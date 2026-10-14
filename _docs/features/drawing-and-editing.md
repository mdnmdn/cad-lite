---
id: feat-drawing-and-editing
title: Drawing and editing commands
kind: feature
status: current
summary: Every built-in command with its prompts and aliases, how selection, object snaps and ortho behave for the user, and undo.
read_when: you are adding or changing a command, an alias, or what a click does while a command runs
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [cmdsys/commands/draw.go, cmdsys/commands/modify.go, cmdsys/commands/util.go, cmdsys/commands/measure.go, cmdsys/snap.go]
depends_on: [tech-command-system, tech-document-model]
review_cycle: monthly
---

# Drawing and editing commands

## Purpose

The drafter draws and edits geometry with AutoCAD's commands, prompts and aliases, using clicks
made exact by object snaps and ortho, or typed coordinates. Every command undoes as one step.

## Behaviour

### Commands

This table is the owner of the command list and the default aliases.

| Command | Aliases | Prompts and behaviour |
|---|---|---|
| LINE | L | First point, then next points `[Undo]`, `[Close/Undo]` from the third point; each segment is a separate line. Enter ends; Undo removes the last segment; Close draws back to the first point and ends |
| PLINE | PL | Start point, then next points `[Undo]` / `[Close/Undo]`; one polyline is created when the command ends (Enter, Close or Escape) |
| CIRCLE | C | Center, then radius `[Diameter]`; a click gives the distance from the centre |
| ARC | A | Start, second and end point; the arc passes through all three. Collinear points draw nothing |
| RECTANG | REC | Two opposite corners; a closed four-vertex polyline |
| ERASE | E | Erases the selection |
| MOVE | M | Selection, base point, second point |
| COPY | CO, CP | Selection, base point, then second points until Enter, one copy each |
| ROTATE | RO | Selection, base point, rotation angle (typed degrees, or a click measured from the base) |
| SCALE | SC | Selection, base point, scale factor (typed, or the click's distance from the base) |
| UNDO | U | Undoes the last command; `Everything has been undone.` when nothing is left |
| REDO | — | Redoes the last undone command |
| ZOOM | Z | A window by two corners, or `[Extents/Window]`; Enter is Extents |
| LAYER | LA | A layer name makes it current, creating it with colour 7 when new; `name=color` sets its colour (1–255) |
| DIST | DI | Two points; prints distance, angle and the X and Y deltas. `[Multiple points]` (`M`) adds up consecutive segments and prints `Distance = total` on Enter |
| AREA | AA | Corner points until Enter, or `[Object]` to pick one; prints `Area = X, Perimeter = Y` (`Circumference` for a circle). Open polylines and arcs close by their chord; polyline bulges count; a line has no area |
| MEASUREGEOM | MEA | `[Distance/Radius/Angle/ARea]`, Enter is Distance. Radius picks an arc or circle (`Radius = r, Diameter = d`); Angle measures two lines, an arc's included angle, a circle centre and two points, or a vertex and two points (`Angle = n°`); ARea is AREA. One measurement per run |
| ID | — | One point; prints its coordinates |
| LIST | LI | Prints each selected entity's type, layer, handle and geometry |
| NEW | — | Replaces the drawing with an empty one |
| OPEN, SAVE | — | A DXF path; see [DXF files](dxf-files.md) |

Commands that take a selection (ERASE, MOVE, COPY, ROTATE, SCALE, LIST) use the current selection
without asking when there is one, then clear it. Otherwise they prompt `Select objects:`; the user
picks or drags boxes and presses Enter.

### Selection

- With no command running, or at `Select objects:`, a click picks the entity under the pickbox and
  adds it to the selection; Shift-click removes it.
- Dragging a box selects too: left to right is a **window** (blue; entities entirely inside), right
  to left a **crossing** (green; entities inside or touching).
- The entity under the pickbox is highlighted while hovering; selected entities are highlighted in
  blue.
- Entities on locked layers or layers that are off cannot be selected.

### Object snaps and ortho

- While a command asks for a point, a distance or an angle, the cursor snaps to the nearest snap
  point within the aperture (8 pixels) and a marker shows the snap: square endpoint, triangle
  midpoint, circle centre, diamond quadrant, X intersection.
- The snap modes and their priority are owned by the [command system](../tech/command-system.md).
- With ortho on, rubber-band points are constrained horizontal or vertical from the base point; a snap
  overrides ortho; typed coordinates are never changed.

### Undo

Each command, including one cancelled part-way, is one undo step. Undo and redo restore entities
with their handles. Selections, view changes and layer table edits are not undoable.

## Contract

- Commands registered by `commands.Register`; aliases in `commands.Aliases` (`acad.pgp` format).
- Document: `Txn.Add`, `Txn.Modify`, `Txn.Erase`, `Drawing.Undo`, `Drawing.Redo`, the layer table.
- Session state: the selection, `SnapOn`, `Snaps`, `Ortho`, `Aperture`.

## Implementation

The draw commands are in `cmdsys/commands/draw.go` (`lineCmd`, `plineCmd`, `circleCmd`, `arcCmd`,
`rectCmd`), the modify commands in `modify.go` (`eraseCmd`, `moveCmd` for MOVE and COPY,
`transformCmd` for ROTATE and SCALE, all sharing `selecting`), measurement (`areaCmd`, `measureGeomCmd`, with the area formulas in `geom/area.go`) in `measure.go`, and the rest in `util.go`, which also
holds `Register`, `Aliases` and `ZoomExtents`. Snaps, picking and box selection are
`cmdsys.FindSnap`, `cmdsys.Pick` and `cmdsys.SelectBox`; the UI forwards clicks and drags to
`Session.Click` and `Session.SelectBox` from `ui/canvas.go`.

## Failure

- A zero or negative radius or scale factor prints `Value must be positive and nonzero.` and asks
  again.
- An empty selection ends the command without a change.
- `LAYER` with a colour outside 1–255 prints the valid range and changes nothing.

## Related docs

- [Command system](../tech/command-system.md) — the framework under every command
- [Command line](command-line.md) — how commands are typed and answered
- [Drawing canvas](canvas.md) — highlights, markers and the crosshair on screen

## Next steps

- TRIM, EXTEND, OFFSET, FILLET, CHAMFER, MIRROR, STRETCH, BREAK, JOIN, EXPLODE and ARRAY.
- Grip editing and a properties palette.
- Polar tracking, object snap tracking, and the Perpendicular, Tangent and Extension snaps.
- PLINE arc segments.
