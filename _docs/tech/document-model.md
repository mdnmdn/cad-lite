---
id: tech-document-model
title: Document model
kind: tech
status: current
summary: How a drawing is stored — handles, entities, layers, the header — and the transaction, change-set and undo contract every edit follows.
read_when: you are adding an entity type, changing how edits are recorded, or reading or writing the document from a command or a codec
updated: 2026-10-25
verified: 2026-10-25
code_anchors: [doc/textlimits.go, doc/object.go, doc/properties.go, doc/linetype.go, doc/tables.go, doc/dimarrow.go, doc/dimoverride.go, doc/entities.go, doc/text.go, doc/mtext.go, doc/dimension.go, doc/textfont.go, doc/drawing.go, doc/layerorder.go, doc/txn.go, doc/journal.go, doc/group.go, doc/block.go, doc/insert.go, doc/attribute.go, doc/unknown.go, doc/aci.go, ui/textedit.go]
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
`Clone`. `doc.Entity` adds the graphical part: `Common()` (handle, owner, layer, colour, linetype,
linetype scale, lineweight; see [Property inheritance and resolution](#property-inheritance-and-resolution)), `Bounds`,
`Geometry` (the curves used for snapping, picking and display) and `Transform`. The entity types are
`Line`, `Circle`, `Arc`, `LWPolyline`, `Text`, `MText`, `Dimension`, `Insert` and `AttributeDefinition`. An `LWPolyline` also carries `Widths`, the start and end width per
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

**Dimensions.** `Dimension` (`doc/dimension.go`) is a DIMENSION of kind `DimRotated` (DIMLINEAR: horizontal,
vertical or rotated by `Rotation`), `DimAligned` (parallel to its origins) or `DimArcLength` (an arc length
dimension: `Center`, `Radius`, `StartAngle`, `EndAngle`, `Partial`, `ArcSymbol`). It is non-associative: it keeps
its definition points (`Ext1`, `Ext2`, `DimLine`, `TextPos`; `DefPoints` lists them, with `Center` for an arc
length one) and measures them (`Measurement`, `MeasurementText`: DIMDEC, DIMDSEP, DIMZIN bits 4 and 8, and the
`Text` override where `<>` stands for the value and a single space suppresses it). Its variables come from a
`DimStyle` it references by name, with its own overrides on top (below). `DefaultDimStyle` is AutoCAD's ISO-25.
`Layout` derives the drawing from the fields and the effective style and caches it as `Text` does, keyed on both:
extension lines offset by DIMEXO and extended by DIMEXE; the dimension line (or arc); the arrowheads
(`DimLayout.Arrows`, a `DimArrow` each: kind, tip, direction, size and the shape a renderer draws, an outline
filled or not, strokes with a width for the architectural tick, circles for dots, arcs for the integral sign,
and `LineEnd`, where the line stops so a blank arrow stays empty), whose kinds are AutoCAD's DIMBLK names
(`ArrowKind`, `ArrowKindOf`, `LayoutArrow`; DIMBLK1 and DIMBLK2 with DIMSAH; a block of the drawing's own draws
as closed filled); and the text as an internal middle-centre `Text` in DIMTXSTY, kept readable (angle in
(-90°, 90°]). Fit follows AutoCAD (`linearPlace`): the arrows fit in 2.5 arrow lengths, the text in its length
along the line plus a gap each side, text centred on the line needs both; DIMTIX keeps the text inside,
otherwise DIMATFIT moves both, the arrows first, the text first, or whichever fits best; arrows outside get
stubs, and DIMTOFL keeps the line between the extension lines; text outside goes past the second extension line
(past the first for DIMJUST 1) with the line run out to it. Placement across the line is DIMTAD (centred, which
breaks the line around the text; above in the reading sense, also for JIS; away from the origins; below), and
text that is horizontal by DIMTIH or DIMTOH, or rotated by `TextAngle`, on a line that is not is centred whatever
DIMTAD says, as in AutoCAD. Along the line it is DIMJUST: centred, next to either extension line, or over either
one, aligned with it. An arc length dimension keeps its text at the middle of the arc and moves only its arrows.
Its extension lines are parallel to the arc's bisector under 90° and radial from 90°, and its arc symbol is an
arc stroke before or above the text (the font has no ⌒). Unless `TextMoved`, the text sits where the effective style
puts it (`TextMiddle`, which writers store); `Recompute` moves `DimLine` to the Ext2 end of a linear
dimension line, puts a whole-arc dimension's origins on the arc's ends and refreshes `TextPos`. `Transform`
maps the points (and the arc), so a scale changes the measurement and not the style sizes, then calls
`Recompute`; a reflection leaves the text readable. `Lines` is the geometry without the text, for snaps;
`Geometry` adds the text strokes and strokes that make filled arrowheads and dots read solid and give
the tick its width. The renderer fills arrowheads itself from `Layout` at the top level; the strokes
serve block parts, which carry curves only, and snaps.

**Linetypes.** The drawing has a LTYPE table of `doc.Linetype` records (`Name`, `Description`, `Pattern`
with DXF 49 semantics: a positive length is a dash, a negative one a gap, 0 a dot, in drawing units at
scale 1; `Raw` keeps a complex linetype's record for writers). CONTINUOUS is always in it. `Linetypes` and
`Linetype` (any case) read it, `LinetypePattern` gives the pattern a resolved linetype draws with (solid
for a name the table lacks), `Drawing.LoadLinetype` is the readers' entry and `Txn.SetLinetype`,
`LoadLibraryLinetype` and `RemoveLinetype` the undoable edits (a linetype in use, current, or CONTINUOUS
cannot be removed). `LibraryLinetypes` is a built-in copy of acadiso.lin's simple linetypes in
millimetres: ACAD_ISO02W100 to ACAD_ISO15W100 and BORDER, CENTER, DASHDOT, DASHED, DIVIDE, DOT, HIDDEN and
PHANTOM with their 2 and X2 variants.

**Dimension styles.** The drawing has a DIMSTYLE table (`DimStyles`, `DimStyle` by name in any case,
`ActiveDimStyle` for `CurrentDimStyle`), holding ISO-25 in a new drawing. A record carries DIMSCALE,
DIMASZ, DIMTXT, DIMEXO, DIMEXE, DIMGAP, DIMDEC, DIMDSEP, DIMZIN, DIMTAD (`TextVertical`, `DimText*`),
DIMJUST (`DimJust*`), DIMTIH, DIMTOH, DIMTXSTY, DIMBLK, DIMSAH, DIMBLK1, DIMBLK2, DIMCLRD, DIMCLRE,
DIMCLRT (ACIs, 0 ByBlock), DIMATFIT (`DimFit*`), DIMTIX and DIMTOFL; `DimStyleExtra` keeps a read
record's other variables for writers. `Drawing.LoadDimStyles` is the readers' entry; `Txn.SetDimStyle`
and `RemoveDimStyle` are the undoable edits.

**Colour.** An entity's `Color` is an AutoCAD Color Index from 1 to 255, `doc.ByLayer` (256) or
`doc.ByBlock` (0); how it resolves is below. `doc.ACIRGB` gives an index's RGB and `doc.NearestACI` the index
nearest an RGB (a true colour read from a file has no other place to go: the document has no true colour).

**Layers.** `doc.Layer` has a name, a colour index, the `Off`, `Locked` and `Frozen` flags, `NoPlot`,
`Linetype` (a name; "" is CONTINUOUS), `Lineweight` (an `int`: hundredths of a mm, 0 the default,
`int(LineweightZero)` 0.00 mm; `LineweightFromLayer` and `Lineweight.LayerValue` convert) and
`Description`. Whether a layer's entities are drawn, snapped and picked is one rule in
`doc/layerstate.go`: `Layer.Visible` (not off, not frozen) and `Layer.Editable` (visible and unlocked),
reached for an entity as `Drawing.EntityVisible` and `EntityEditable`; render, snap and selection all
call them, so a new layer flag is honoured by changing that file. `Drawing.RemoveLayer` deletes an
unreferenced layer (never "0" or the current one). Layer "0"
always exists. `Drawing.CurrentLayer` is the layer new entities go on. Layer property edits are either
direct (`Drawing.AddLayer`, writing a field, `RemoveLayer`), not journalled, or undoable through
`Txn.AddLayer`, `ModifyLayer` (which cannot rename) and `RemoveLayer`; layer order uses the transaction
contract below. A host that changes layer metadata calls `Session.MarkModified` after the
actual edit to record unsaved changes without an entity undo step; the counter belongs to the
[command system](command-system.md). `Drawing.Layer` matches a name exactly (case-sensitive) through a name-keyed index
that `New`, `AddLayer` and `RemoveLayer` keep in step with the table, so render, snap and selection
can look a layer up per entity per frame. A layer's `Name` must not change while it is in the table;
a lookup that finds a renamed record rebuilds the index rather than return it under its old name.

**Current properties and global defaults.** `Drawing.CurrentLayer` (CLAYER), `CurrentColor` (CECOLOR,
encoded as an entity colour; `New` sets ByLayer), `CurrentLinetype` (CELTYPE), `CurrentLinetypeScale`
(CELTSCALE), `CurrentLineweight` (CELWEIGHT) and `CurrentDimStyle` (DIMSTYLE) are what new entities and
dimensions get; `LTScale` (LTSCALE) and `LWDefault` (LWDEFAULT, not saved: AutoCAD keeps it per user)
are global defaults, and `Header.InsUnits` (INSUNITS) rides along. `Current` returns them all as
`CurrentProps`; `Txn.SetCurrent` changes them undoably, after validating them (an unknown layer, linetype
or dimension style, a colour outside 0–256, a bad scale or lineweight, INSUNITS outside 0–24); the
fields may also be written directly, without undo. A `CurrentProps` built from a literal must set every
field, `InsUnits` included, or `SetCurrent` writes the zero value; start from `Current`. The system
variables write through it (see below).

**Header.** `doc.Header` holds `HandSeed`, `InsUnits` (journalled through `CurrentProps.InsUnits`), `Version` (the file format version the
drawing saves as: the one a reader found or SAVEAS chose, "" to let the writer choose; see
[File I/O](file-io.md)) and, in `Extra`, every header variable the reader does not model, kept as
read. `Version` is not journalled: changing it is not an undo step.

**Unknown entities and preserved records.** `doc.Unknown` keeps an entity of a class the document
does not model. `Type` is the class name and `Raw` the groups after its `0 CLASS` line in file order,
including 5, 8, 62 and 330; `EntityCommon` is a parsed view of `Raw`, and the writer emits `Raw`. It
has no geometry, so it is never drawn, picked, snapped to or selected, including through a group. `Bounds` is the box of its
point groups, a hint only, and `Drawing.Extents` ignores it. It is never editable: `Txn.Modify` does
not call the edit function, `Txn.Erase` does nothing and `Txn.Add` returns 0. Only `Drawing.Load`
creates one. `Drawing.Preserved` holds the table records and blocks the document does not model, as
`doc.RawRecord` groups; the user cannot change them. [File I/O](file-io.md) says what the DXF codec
keeps and writes.

**Storage order.** `Drawing.Handles` returns entity handles in ascending order for stable storage and
spatial queries. Layer stacking is separate, as described below. Erasing is O(1): it leaves the handle in the order slice as a tombstone, and `Handles` sorts
and drops tombstones and duplicates on its next call (or the drawing compacts once tombstones
outnumber live entities). The slice `Handles` returns is shared and valid until the next edit.

## The edit contract

1. Every modification happens inside a transaction: `Drawing.Begin` opens one; only one may be open,
   and a second `Begin` returns `doc.ErrTxnOpen`.
2. `Txn.Add` assigns a handle, model-space ownership and the current properties to the unset ones:
   the current layer to an empty layer, `CurrentColor` to colour 0, `CurrentLinetype` to an empty
   linetype, `CurrentLinetypeScale` to a scale of 0 and `CurrentLineweight` to a ByLayer lineweight.
   An entity whose colour is ByBlock is therefore added with CECOLOR BYBLOCK or with `Txn.AddCopy`,
   which keeps every property but the layer as it is: commands that copy entities (COPY, MIRROR,
   ARRAY, OFFSET) use it, so a copy keeps ByLayer and ByBlock values. `Txn.Modify` snapshots an
   entity before calling the edit function. `Txn.Erase` removes one.
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
9. Table and current-property edits (`Txn.AddLayer`, `ModifyLayer`, `RemoveLayer`, `SetLinetype`,
   `LoadLibraryLinetype`, `RemoveLinetype`, `SetDimStyle`, `RemoveDimStyle`, `SetCurrent`,
   `ImportBlockSupport`) snapshot the layers, linetypes, dimension styles, imported STYLE records and
   current properties at the transaction's first such edit. The commit records the snapshot with the
   entity states in the same journal step (nothing when the tables ended as they began) and publishes
   `ChangeSet.Support`. When the change affects how existing entities draw (a layer, linetype or
   dimension style record, an imported STYLE or LTYPE record, LTSCALE or LWDEFAULT), every block
   reference and dimension is also reported modified, once, since their drawing depends on the
   tables, and `TakeEdits` reports them once per call; a change of only CLAYER, CECOLOR, CELTYPE,
   CELTSCALE, CELWEIGHT, DIMSTYLE or INSUNITS reports none, so it costs nothing per entity. Undo,
   redo and abort put back only the records and fields that step changed (a layer field by field),
   so a direct edit made since to anything else, a layer's other fields included, survives.
10. `Txn.TakeEdits` returns the handles `Add`, `Modify` and `Erase` touched since `Begin` or the previous
   call, and starts a new list. It publishes and journals nothing; the command session uses it to show
   a command's work before the commit (see the command system).

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
`Txn.ImportBlockSupport` snapshots and imports the source's missing layers, linetypes, dimension styles
(with their unmodelled groups and the arrow blocks they name, nested blocks included) and preserved
STYLE and LTYPE records, keeping the destination's record where both have one of the same name, remaps imported record
handles and references, and restores that support state with the same import undo step.
`ChangeSet.Support` identifies these and every other table change (contract item 9). The layer
commands (-LAYER, LAYER, PURGE) edit layers through `Txn.ModifyLayer`, and the layer panel moves to it
too, so every layer edit is an undo step. Removing an imported current layer through undo selects existing layer 0,
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
Layer 0 and ByBlock appearance inherit the insertion's layer, colour, linetype and lineweight
(`Insert.Parts`, through `Drawing.Resolve`); explicit child layers retain visibility and appearance. Unsupported imported content stays preserved with a warning.

## Property inheritance and resolution

Every display property resolves from the nearest level that sets it: the entity, then the enclosing
block reference (ByBlock), then the layer (ByLayer), then the drawing's global defaults. Changing a
level changes everything that inherits from it. `Drawing.Resolve(common, parent)` is the one place
this happens and returns an `Appearance` (effective layer, colour 1–255, linetype name, linetype
scale, lineweight width); `parent` is the resolved appearance of the enclosing reference, so nested
references resolve ByBlock through the chain, and `Insert.Parts` uses it for every definition entity.
Each `BlockPart` also carries `Scale`, the accumulated scale of every reference around it (the
square root of `|det|` of the product of their transforms; it multiplies the part's linetype scale,
as in AutoCAD), and `Kind`: `PartCurve` for geometry and a dimension's dimension, extension and arc
lines, `PartText` for text, attributes and a dimension's text and arc symbol, `PartSolid` for a
dimension's arrowheads. A dimension inside a block comes out as four parts (extension lines in
DIMCLRE, dimension lines in DIMCLRD, arrowheads in DIMCLRD, text in DIMCLRT), so one linetype
treatment applies per part.
`ResolveEntity` is the model-space case and `ResolveColorIn` resolves a part's colour (a dimension's
DIMCLRD, DIMCLRE, DIMCLRT) against its entity's appearance. Block definitions hold their entities
unresolved: ByBlock and layer 0 stay symbolic in the definition and resolve per reference, so one
definition draws differently in each. `Insert.Explode` is the exception: it resolves a ByBlock child to a
concrete colour and ignores linetype and lineweight inheritance (backlog G88).

| Property | Entity field and encoding (zero value) | ByBlock | ByLayer | Global default |
|---|---|---|---|---|
| Colour | `Color`: 1–255, `ByLayer` 256, `ByBlock` 0 (0 is "unset" to `Txn.Add`) | the reference's colour; 7 outside a block | `Layer.Color` | 7 for a missing layer |
| Linetype | `Linetype`: "" or `LinetypeByLayer`, `LinetypeByBlock`, a table name ("" ByLayer) | the reference's; CONTINUOUS outside | `Layer.Linetype`, "" CONTINUOUS | CONTINUOUS |
| Linetype scale | `LinetypeScale`, 0 reads 1 (`LtScale`) | not inherited | not inherited | times `LTScale` (LTSCALE) |
| Lineweight | `Lineweight` type: `LineweightByLayer` 0, `LineweightByBlock` -2, `LineweightDefault` -3, `LineweightZero` (0.00 mm) -4, else hundredths (ByLayer) | the reference's; Default outside | `Layer.Lineweight`, 0 Default, -4 0.00 mm | Default is `LWDefault` (LWDEFAULT, 0.25 mm unset) |

An entity on layer "0" inside a block takes the reference's layer. The DXF lineweight values (-1
ByLayer, -2 ByBlock, -3 Default, 0 and up hundredths) convert with `Lineweight.DXF` and
`LineweightFromDXF`; `StandardLineweights` lists AutoCAD's widths.

**Dimensions** inherit their variables from the DIMSTYLE record they name (`StyleName`), with their
own `Overrides` (`DimOverrides`: a `DimVar` bit set saying which variables, and their values) on top;
unset means inherit, as AutoCAD's DSTYLE overrides are. `Dimension.Style` returns the effective style
(the record of the drawing holding the dimension, or ISO-25 values for a name the table lacks or a
dimension in no drawing); `SetStyle(d, s)` names `s.Name` and overrides exactly the variables where
`s` differs from that record; `DimOverrides.With` and `Without` set and clear single variables, and
`DimStyleDiff` compares two styles. Editing the record (`Txn.SetDimStyle`) changes every dimension
that does not override what changed.

**Where each level is edited.** The entity level is the entity's own fields, edited through `Txn.Modify`;
the Properties panel and the agents' property tools do it through `cmdsys/props`, the UI-free model
described in the [command system](command-system.md#property-model) (its `Source` says which level a value
comes from, and `Value.Clear` drops the level's own value so the property inherits again: ByLayer for a
colour, linetype or lineweight, 1 for a linetype scale, removing a dimension override; the command system
owns those semantics). The layer level is `Txn.ModifyLayer`. The global and current level is
`Txn.SetCurrent`, and the DIMSTYLE record is `Txn.SetDimStyle`: the system variables CLAYER, CECOLOR,
CELTYPE, CELTSCALE, CELWEIGHT, LTSCALE, DIMSTYLE and the DIM* variables write through them
(`cmdsys/propvars.go`, `Vars.SetTx`), so SETVAR, the panel and the commands cannot disagree and each edit is
undoable. `CurrentProps.InsUnits` makes INSUNITS journalled in `doc`, but the INSUNITS variable still
writes the header directly and is not undoable, as are TEXTSIZE, TEXTSTYLE and DIMARCSYM, session
variables outside the document (backlog G90).

**Rendering.** The renderer takes appearance from `Drawing.Resolve` and `Insert.Parts` and nothing else:
it never reads a layer's colour or an entity's linetype itself. [Rendering](rendering.md) owns how the
resolved colour, linetype pattern and lineweight are drawn.

**Caches.** A dimension's layout is keyed on its fields and its effective style, so a style edit,
committed, undone or aborted, redraws it at the next `Layout`; text and block references rebuild on
their own keys. Entity edits report the entity in the change set; table edits (layers, linetypes,
dimension styles, current properties) report `ChangeSet.Support`, and those that change how existing
entities draw also every block reference and dimension as modified (contract item 9), so a listener
that caches appearance per entity (the renderer) refreshes on `Support`, where a layer colour may
have changed any entity on it.

**Undo.** Entity properties live in the entity, so `Txn.Modify` journals them like geometry (the
snapshot is the whole entity, overrides included). Layers, linetypes, dimension styles, the current
properties, the global defaults and INSUNITS are journalled by the table edits of contract item 9: one
snapshot at a transaction's first table edit, one journal step with the entity changes of the same
command, and undo, redo and abort restore only what that step changed. The direct forms
(`Drawing.AddLayer`, writing a field) are not journalled and are for file readers and hosts that want no
undo step; a host that uses one calls `Session.MarkModified` so the drawing counts as changed.

## Layer order

`Drawing.Layers` is the canonical front-to-back layer order, shared by the layer lists and drawing
stacking. `Txn.SetLayerOrder` accepts an exact permutation of every existing layer name, including 0.
Invalid, duplicate or missing names leave the table unchanged. A real net change creates one undo
step and publishes `ChangeSet.LayerOrder`; a no-op publishes and journals nothing. Snapshots store
names only. Undo, redo and abort reorder surviving named layers within their current slots, preserving
later added layers, direct property edits and the current layer. Imported support restores before
order restoration. `Drawing.LoadLayerOrder` supplies the same validation for file readers without
creating undo history. Layer property edits are direct or go through `Txn.ModifyLayer` (contract item 9).

`Drawing.Handles` remains ascending storage/index order. The renderer sorts a copied candidate list
back to front by the owning entity's layer, then by ascending handle. INSERT is atomic for this order:
its root layer positions the complete reference, while child layers keep visibility/appearance and
children keep authored internal order. Nearest picking is unchanged; ties choose the front layer,
then the newest entity within that layer.

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
