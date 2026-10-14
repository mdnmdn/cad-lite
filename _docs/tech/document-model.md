---
id: tech-document-model
title: Document model
kind: tech
status: current
summary: How a drawing is stored — handles, entities, layers, the header — and the transaction, change-set and undo contract every edit follows.
read_when: you are adding an entity type, changing how edits are recorded, or reading or writing the document from a command or a codec
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [doc/object.go, doc/entities.go, doc/drawing.go, doc/txn.go, doc/unknown.go]
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
`Line`, `Circle`, `Arc` and `LWPolyline`. Arc angles are radians; the DXF codec converts degrees.

**Colour.** An entity's `Color` is an AutoCAD Color Index from 1 to 255, or `doc.ByLayer` (256),
which a transaction assigns when the colour is unset.

**Layers.** `doc.Layer` has a name, a colour index and the `Off` and `Locked` flags. Layer "0"
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
