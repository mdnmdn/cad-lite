---
id: tech-document-model
title: Document model
kind: tech
status: current
summary: How a drawing is stored — handles, entities, layers, the header — and the transaction, change-set and undo contract every edit follows.
read_when: you are adding an entity type, changing how edits are recorded, or reading or writing the document from a command or a codec
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [doc/object.go, doc/entities.go, doc/drawing.go, doc/txn.go, doc/journal.go, doc/unknown.go, doc/aci.go]
depends_on: [tech-architecture, tech-geometry]
review_cycle: quarterly
---

# Document model

## What it is

`doc` is the document database. Its model mirrors the DWG/DXF object model so that a drawing read
from a file can be written back unchanged: every object has a persistent 64-bit handle, objects refer
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
`Line`, `Circle`, `Arc` and `LWPolyline`. An `LWPolyline` also carries `Widths`, the start and end width per
segment; only the DWG reader fills it, and the DXF codec and the renderer ignore it until polyline widths land. Arc angles are radians; the DXF codec converts degrees.

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
and not journalled.

**Header.** `doc.Header` holds `HandSeed`, `InsUnits` and, in `Extra`, every header variable the
reader does not model, kept as read.

**Unknown entities and preserved records.** `doc.Unknown` keeps an entity of a class the document
does not model. `Type` is the class name and `Raw` the groups after its `0 CLASS` line in file order,
including 5, 8, 62 and 330; `EntityCommon` is a parsed view of `Raw`, and the writer emits `Raw`. It
has no geometry, so it is never drawn, picked, snapped to or selected. `Bounds` is the box of its
point groups, a hint only, and `Drawing.Extents` ignores it. It is never editable: `Txn.Modify` does
not call the edit function, `Txn.Erase` does nothing and `Txn.Add` returns 0. Only `Drawing.Load`
creates one. `Drawing.Preserved` holds the table records and blocks the document does not model, as
`doc.RawRecord` groups; the user cannot change them. [File I/O](file-io.md) says what the DXF codec
keeps and writes.

**Draw order.** `Drawing.Handles` returns entity handles in ascending order, so newer entities draw
on top.

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
