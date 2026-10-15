---
id: tech-document-model
title: Document model
kind: tech
status: current
summary: How a drawing is stored — handles, entities, layers, the header — and the transaction, change-set and undo contract every edit follows.
read_when: you are adding an entity type, changing how edits are recorded, or reading or writing the document from a command or a codec
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [doc/textlimits.go, doc/object.go, doc/entities.go, doc/text.go, doc/mtext.go, doc/textfont.go, doc/drawing.go, doc/txn.go, doc/journal.go, doc/group.go, doc/block.go, doc/insert.go, doc/attribute.go, doc/unknown.go, doc/aci.go, ui/textedit.go]
depends_on: [tech-architecture, tech-geometry]
review_cycle: quarterly
---

# Document model

## What it is

`doc` is the document database. Its model mirrors the DWG/DXF object model so that supported content read
from a file can be written back: every object has a persistent 64-bit handle, objects refer
to each other by handle, and the drawing's header variables travel with it. A `doc.Drawing` holds
one document: its header, its layer table, the entities of model space and the undo journal.

## Model

**Handles.** `doc.Handle` is DXF group code 5, written as upper-case hexadecimal. Handles read from a
file are kept. New objects take `Header.HandSeed` and advance it, and loading an object whose handle
is at or above the seed raises the seed past it, so a new handle never collides with a loaded one.
Zero means "no object". The model-space block record is `doc.ModelSpace`, the owner of every entity.

**Objects and entities.** `doc.Object` is anything stored: handle, owner, DXF class name and a deep
`Clone`. `doc.Entity` adds the graphical part: `Common()` (handle, owner, layer, colour), `Bounds`,
`Geometry` (the curves used for snapping, picking and display) and `Transform`. The entity types are
`Line`, `Circle`, `Arc`, `LWPolyline`, `Text`, `MText`, `Insert` and `AttributeDefinition`. An `LWPolyline` also carries `Widths`, the start and end width per
segment; only the DWG reader fills it, and the DXF codec and the renderer ignore it until polyline widths land. Arc angles are radians; the DXF codec converts degrees.

**Text.** `Text` is a single-line TEXT with DXF's fields, all exported: `Insert` and `Align` (the first and
second alignment points, groups 10 and 11), `Height`, `Rotation` and `Oblique` (radians), `WidthFactor`, `Style`,
`HAlign` and `VAlign` (groups 72 and 73, `TextLeft`… and `TextBaseline`…), `Generation` (`TextBackward`,
`TextUpsideDown`) and `Value`, with AutoCAD's `%%` codes. Left-baseline text sits at `Insert`; Aligned and Fit run
the baseline from `Insert` to `Align`, deriving the height or the width factor; every other justification places
the text by `Align` (`Anchor`, `SetAnchor`; `SetJustify` changes the justification around the anchor;
`ParseJustify` and `JustifyName` convert AutoCAD's option names). `NewText` makes left-justified text in the
`Standard` style. `MText` is multiline text: `Insert`, `Height`, `Width` (the wrapping width, 0 for none),
`Attachment` (1 to 9, top left to bottom right), `Rotation`, `LineSpacing`, `Style` and `Value` with MTEXT format
codes (`\P` breaks a paragraph; `MTextPlain` strips the rest, `MTextEscape` escapes plain text for a value);
`NewMText` makes top-left attached text. The document has no text style table: `TextStyles` lists the names a
drawing knows (Standard, the STYLE records kept from its file, the styles its texts name) and `FindTextStyle`
matches one without regard to case. Both
satisfy `doc.Annotation` (`Anchor`, `HitBox`, `Box`, `CharHeight`, `Greek`, `MirrorReadable`).

The manual TEXTEDIT host keeps its draft outside the document and applies it through `Txn.Modify`,
preserving the entity handle and common fields. MTEXT property-only edits preserve raw `Value`;
replacement contents use `MTextEscape` and paragraph separators, as the editing feature describes.

Text geometry is computed in `doc`, without the UI: every style draws with one built-in single-stroke font, Hershey
Roman Simplex (`doc/fonts/romans.jhf`, the design AutoCAD's `romans.shx` comes from; licence acknowledgements in
`doc/fonts/NOTICE`), laid out by `layoutStrokes` in `doc/textfont.go`. `Geometry` returns the glyph strokes as open
polylines, `Bounds` the union of the strokes' box, the text box and the anchor (so the spatial index finds a text for its
insertion snap and for a click anywhere in its box), `Layout` where the line sits (`TextLayout`: the baseline
start AutoCAD writes as group 10, the height, width factor and length in effect, the box from the descenders to the
capitals). Capitals are `Height` tall and descenders a third of it; advance widths are the font's, so `TextAdvance`
gives a string's exact length without a display; it only advances the pen, without building strokes.
MTEXT wraps at spaces by summing each word's advance once (`wrapLine`), so wrapping is linear in the
paragraph's length. `Text.Sanitize` and `MText.Sanitize` clamp the size fields to the ranges in
`doc/textlimits.go` (`TextMaxSize`, AutoCAD's width factor, oblique and line spacing ranges); the file
readers call them, and the layouts apply the same limits, so a text's box is always finite. Characters the font lacks are drawn through their compatibility
decomposition (accented letters as letter and mark) or as `?`. The layout is cached in the entity against a key of
the fields it depends on, so editing a field directly (inside `Txn.Modify`) is enough. `Transform` mirrors TEXT for
real (it toggles `TextUpsideDown`); `MirrorReadable` is MIRROR with `MIRRTEXT` 0. MTEXT has no mirrored form: a
reflection keeps it readable and swaps its attachment.

**Colour.** An entity's `Color` is an AutoCAD Color Index from 1 to 255, or `doc.ByLayer` (256),
which a transaction assigns when the colour is unset: `Drawing.CurrentColor` (`CECOLOR`) when it is
set to 1–255, otherwise `ByLayer`. `doc.ACIRGB` gives an index's RGB and `doc.NearestACI` the index nearest an RGB (a
true colour read from a file has no other place to go: the document has no true colour).

**Layers.** `doc.Layer` has a name, a colour index, the `Off`, `Locked` and `Frozen` flags, `NoPlot`,
`Linetype` (a name; "" is CONTINUOUS), `Lineweight` (hundredths of a mm; 0 is the default) and
`Description`. Whether a layer's entities are drawn, snapped and picked is one rule in
`doc/layerstate.go`: `Layer.Visible` (not off, not frozen) and `Layer.Editable` (visible and unlocked),
reached for an entity as `Drawing.EntityVisible` and `EntityEditable`; render, snap and selection all
call them, so a new layer flag is honoured by changing that file. `Drawing.RemoveLayer` deletes an
unreferenced layer (never "0" or the current one). Layer "0"
always exists. `Drawing.CurrentLayer` is the layer new entities go on. Layer table edits are direct
and not journalled. A host that changes layer metadata calls `Session.MarkModified` after the
actual edit to record unsaved changes without an entity undo step; the counter belongs to the
[command system](command-system.md). `Drawing.Layer` matches a name exactly (case-sensitive) through a name-keyed index
that `New`, `AddLayer` and `RemoveLayer` keep in step with the table, so render, snap and selection
can look a layer up per entity per frame. A layer's `Name` must not change while it is in the table;
a lookup that finds a renamed record rebuilds the index rather than return it under its old name.

**Header.** `doc.Header` holds `HandSeed`, `InsUnits` and, in `Extra`, every header variable the
reader does not model, kept as read.

**Unknown entities and preserved records.** `doc.Unknown` keeps an entity of a class the document
does not model. `Type` is the class name and `Raw` the groups after its `0 CLASS` line in file order,
including 5, 8, 62 and 330; `EntityCommon` is a parsed view of `Raw`, and the writer emits `Raw`. It
has no geometry, so it is never drawn, picked, snapped to or selected, including through a group. `Bounds` is the box of its
point groups, a hint only, and `Drawing.Extents` ignores it. It is never editable: `Txn.Modify` does
not call the edit function, `Txn.Erase` does nothing and `Txn.Add` returns 0. Only `Drawing.Load`
creates one. `Drawing.Preserved` holds the table records and blocks the document does not model, as
`doc.RawRecord` groups; the user cannot change them. [File I/O](file-io.md) says what the DXF codec
keeps and writes.

**Draw order.** `Drawing.Handles` returns entity handles in ascending order, so newer entities draw
on top. Erasing is O(1): it leaves the handle in the order slice as a tombstone, and `Handles` sorts
and drops tombstones and duplicates on its next call (or the drawing compacts once tombstones
outnumber live entities). The slice `Handles` returns is shared and valid until the next edit.

## The edit contract

1. Every modification happens inside a transaction: `Drawing.Begin` opens one; only one may be open,
   and a second `Begin` returns `doc.ErrTxnOpen`.
2. `Txn.Add` assigns a handle, model-space ownership, the current layer when the layer is unset and
   `ByLayer` when the colour is unset. `Txn.Modify` snapshots an entity before calling the edit
   function. `Txn.Erase` removes one.
3. `Txn.Commit` records one journal step holding each touched handle's state before and after, and
   publishes a `doc.ChangeSet` listing the added, modified and erased handles. A transaction that
   changed nothing records nothing.
4. `Txn.Abort` restores every touched entity and publishes the change set of the restoration.
5. `Drawing.Undo` and `Drawing.Redo` apply a step's before or after states and publish the change
   set, so listeners never distinguish an edit from its undo. Neither runs while a transaction is
   open.
6. Committing a new step clears the redo stack.
7. Listeners registered with `Drawing.OnChange` receive every change set. The spatial index, the
   selection and the render cache are the listeners in the application.
8. `Drawing.Load` inserts an entity outside any transaction and journal. Only file readers use it,
   on a drawing nobody else holds yet.
9. `Txn.TakeEdits` returns the handles `Add`, `Modify` and `Erase` touched since `Begin` or the previous
   call, and starts a new list. It publishes and journals nothing; the command session uses it to show
   a command's work before the commit (see the [command system](command-system.md)).

`journal.go` adds the hooks the UNDO commands use, all over the same steps: `Drawing.Mark` pushes a marker at
the current depth and `UndoToMark` undoes back to the latest one (a marker the history has been undone
past is dropped); `BeginGroup` and `EndGroup` merge the transactions committed between them into one undo
step (groups nest; only the outermost end merges); `UndoN` and `RedoN` step several times; `UndoDepth`,
`RedoDepth` and `HasMark` report state. `NameLastStep` and `UndoName` carry the command name of a step, which
the session records when a command ends, so `U` can name what it undid. A group or open transaction refuses `UndoToMark`.

`Drawing.AbortOpen` aborts the open transaction if there is one, and does nothing otherwise. A host uses
it to recover when a panic left a transaction open (`cmdsys.Session.guard`, `cadmcp`).

The command system makes one command one transaction, so one command is one undo step; see the
[command system](command-system.md).

## Groups and block definitions

Groups and named blocks are document objects edited inside `Txn`. Group membership keeps independent
model-space handles; overlapping selectable groups expand selection transitively by default.
`Group` and `Block` read APIs return deep copies. `Txn.AddGroup`, `ModifyGroup`, `EraseGroup`,
`DefineBlock` and `EraseBlock` validate edits before applying them. Entity deletion removes membership
in the same transaction. Metadata snapshots accompany entity snapshots in each journal step, including
abort, grouped undo and redo. Metadata-only changes publish a nonempty `ChangeSet` and mark the drawing dirty.
`Txn.ImportBlockSupport` snapshots supporting layers and preserved STYLE/LTYPE tables, remaps imported
record handles and references, and restores that support state with the same import undo step.
`ChangeSet.Support` identifies these support-table changes; ordinary layer commands retain their
direct-edit policy. Removing an imported current layer through undo selects existing layer 0,
so the current layer always exists; unrelated direct layer property edits survive the undo.

A `Block` holds a case-insensitive name, base point and owned geometry outside model space. An
`Insert` references it with position, rotation and per-axis scale, and owns instance attributes.
Definition replacement updates direct and nested references. Definition changes queue those references
through `TakeEdits` and publish their handles as modified at commit, restoration and undo, so bounds,
spatial queries and cached drawing stay synchronized. New cycles, unresolved definitions, nesting beyond `MaxBlockDepth` and expansion beyond
`MaxBlockEntities` are rejected. The expansion budget counts each visited reference, even one
whose definition has no leaf geometry, and instance attribute work; sums stop at the limit. `Drawing.InsertError` includes the root reference’s
own attributes in that same budget before addition, modification, initialization, geometry resolution
and file-import quarantine. Default attributes are preflighted before initialization, and definition
replacement also checks affected live roots. Validation includes ancestors affected by replacement. Imported invalid
references remain opaque with a warning, preserving their source records.
Layer 0 and ByBlock appearance inherit the insertion's layer and colour; explicit child layers retain
visibility and appearance. Unsupported imported content stays preserved with a warning.

## Rationale

**Snapshots rather than inverse operations.** A journal step stores whole entity states. Undo then
cannot drift from redo, for any entity type, without each type implementing its own inverse. The
cost is memory proportional to the entities touched, which is small next to the drawing itself.

**Handles rather than pointers.** Persisted references are handles because that is what the file
formats store; a pointer graph would have to be rebuilt and kept consistent on every load and undo.

## Related docs

- [Geometry kernel](geometry-kernel.md) — the curves entities are made of
- [File I/O](file-io.md) — how drawings are read and written
- [Backlog](../backlog.md) — unknown-object preservation, blocks, text and the other entity types
