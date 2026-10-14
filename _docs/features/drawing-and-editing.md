---
id: feat-drawing-and-editing
title: Drawing and editing commands
kind: feature
status: current
summary: Every built-in command with its prompts and aliases, how selection, object snaps and ortho behave for the user, and undo.
read_when: you are adding or changing a command, an alias, or what a click does while a command runs
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [cmdsys/commands/draw.go, cmdsys/commands/modify.go, cmdsys/commands/view.go, cmdsys/commands/layer.go, cmdsys/commands/inquiry.go, cmdsys/commands/files.go, cmdsys/commands/undo.go, cmdsys/commands/register.go, cmdsys/commands/measure.go, cmdsys/commands/construct.go, cmdsys/commands/trim.go, cmdsys/commands/select.go, cmdsys/select.go, cmdsys/selectopts.go, cmdsys/snap.go]
depends_on: [tech-command-system, tech-document-model]
review_cycle: monthly
---

# Drawing and editing commands

## Purpose

The drafter draws and edits geometry with AutoCAD's commands, prompts and aliases, using clicks
made exact by object snaps and ortho, or typed coordinates. Every command undoes as one step.

## Behaviour

### Commands

This table is the owner of the command list and the default aliases. The prompts and messages
quoted here are the English text. Each command also has Italian names, registered as aliases in every
language (`LINEA`, `CERCHIO`, ...); the `aliases` and `name` entries of `i18n/locales/it.toml` own them. A leading `_`
on a typed keyword forces the English one.

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
| MIRROR | MI | Selection, two points of the mirror line, then `Erase source objects [Yes/No] <No>`; the reflection is added on the source's layer and colour, arcs stay counter-clockwise and bulges flip. Two equal points print `The two points of the mirror line must differ.` |
| OFFSET | O | `Specify offset distance [Through]`: a typed distance, two points, or `Through` (the distance is the one just used, and the side point sets it per pick). Then loops `Select object to offset [Exit]` and `Specify point on side to offset [Exit]`; Enter exits. Lines, circles, arcs and open or closed polylines; the copy lands on the source's layer. Polyline corners mitre, arc segments take the new radius, segments that collapse are dropped; a radius that would reach zero creates nothing and prints why |
| FILLET | F | `Select first object [Radius]`, `Select second object`. Two lines, trimmed or extended to the corner and joined by a tangent arc; the picked sides are kept. Radius 0 makes a sharp corner. The radius is remembered. Parallel lines print `Lines are parallel.` and a radius that does not fit prints `Radius is too large for the lines.`, both leaving the drawing unchanged; other objects print `Fillet supports two lines.` |
| CHAMFER | CHA | `Select first line [Distance]`, `Select second line`. Two lines, trimmed to the corner and joined by a bevel line; the two distances are asked for by the `Distance` keyword and remembered. Distances 0 and 0 make a sharp corner; too large prints `Chamfer distances are too large for the lines.` |
| TRIM | TR | `Select object to trim [cuTting edges]`, repeated until Enter. Without a chosen edge set every visible entity is a cutting edge; `T` asks `Select cutting edges <select all>` and the picks become the edge set. A pick removes the piece of the object between the two nearest cuts, or from the one cut to the end. Lines, circles, arcs and polylines (a polyline's bulges are kept). Prints `Object does not intersect an edge.` when no edge cuts it |
| EXTEND | EX | `Select object to extend`, repeated until Enter. The end nearest the pick grows along its line or its own circle to the nearest visible entity ahead of it. Lines, arcs and open polylines (the end segment grows); a closed polyline prints `Cannot extend a closed polyline.` and no boundary ahead prints `No boundary edge in the extension of that end.` |
| BREAK | BR | `Select object`, `Specify second break point [First point]`; the part between the two points is removed and the object splits in two, or at one point when both are equal. `F` re-asks the first point. A circle loses its arc counter-clockwise from the first point to the second |
| JOIN | J | `Select source object or multiple objects to join at once`, then `Select objects to join to source` unless several were selected. Collinear lines become one line, arcs of one circle one arc (a circle when they close it), and lines, arcs and open polylines that meet end to end become one polyline. Prints `N objects joined.` and how many `cannot be joined.` |
| EXPLODE | X | Selection; each polyline becomes its lines and arcs, on its layer; other objects print `N objects cannot be exploded.` |
| ID | — | One point; prints its coordinates |
| LIST | LI | Prints each selected entity's type, layer, handle and geometry |
| NEW | — | Replaces the drawing with an empty one |
| OPEN, SAVE | — | A DXF path; see [DXF files](dxf-files.md) |
| OPTIONS | OP | Opens the [settings](settings.md) panel; prints `Settings are not available.` where the host has none |
| SETVAR | SETV | A system variable name (`?` lists), then its value; a variable typed by name at `Command:` asks for the value too; see [Command line](command-line.md) |
| SCRIPT, RSCRIPT, DELAY | SCR | Runs a script file (the nested script replaces the caller); restarts it; accepts a delay and does not wait |
| AI | AGENT | `Ask the agent` takes free text (Space does not submit): text opens the [agent panel](agent-panel.md) and sends it; an empty answer toggles the panel |

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
- **Typed options** at `Select objects:` (and in `SELECT`): `W` window, `C` crossing, `BOX`, `ALL`,
  `L` last (the newest visible object), `P` previous selection, `F` fence, `WP` and `CP` polygon,
  `A` and `R` to switch between adding and removing (the prompt reads `Remove objects:`), `M`, `AU`,
  `SI` (ends the prompt after the first selection), `U` (undoes the last operation); Enter ends. W,
  C and BOX ask `Specify first corner:` and `Specify opposite corner:`; WP, CP and F take points
  until Enter (`U` drops the last one; Enter closes a polygon). A typed point picks the object under
  it, or starts an automatic box when it is empty. Each operation reports `N found`, then
  `N found, M total`, `N found, K duplicate` or `N found, K removed, M total`; other text prints
  `Invalid selection` with the list of options. `ALL` includes objects on layers that are off but not
  frozen or locked ones. Clicks reach the corner and polygon prompts as points. `SELECT` runs the
  prompt and leaves the set as the previous selection for the next command's `P`; the set is not left
  highlighted.

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
`transformCmd` for ROTATE and SCALE, all sharing `selecting`), measurement (`areaCmd`, `measureGeomCmd`, with the area formulas in `geom/area.go`) in `measure.go`, the construct commands in `construct.go` (`mirrorCmd`, `offsetCmd`, `cornerCmd` for FILLET and CHAMFER, on `geom/offset.go` and `geom/fillet.go`), the trim family in `trim.go` (`trimCmd`, `extendCmd`, `breakCmd`, `joinCmd`, `explodeCmd`, on `geom/split.go`), and one file per remaining group: `view.go` (ZOOM, `ZoomExtents`), `layer.go`, `inquiry.go` (DIST, ID, LIST),
`files.go` (NEW, OPEN, SAVE), `undo.go` (UNDO, REDO). `register.go` holds `Register`, which calls one `registerX` per
file, and `aliases.go` the `Aliases` text; `base.go` holds the helpers every command shares (`base`, `selecting`,
`immediate`). Snaps, picking and box selection are
`cmdsys.FindSnap`, `cmdsys.Pick` and `cmdsys.SelectBox`; the UI forwards clicks and drags to
`Session.Click` and `Session.SelectBox` from `ui/canvas.go`.

OFFSET, FILLET and CHAMFER pick against the spatial index, which follows the document on commit, so
an entity created earlier in the same command is not pickable until the command ends. Their
remembered distance, radius and chamfer distances are package variables in `construct.go`, shared by
every session in the process. The point-picking prompts of the trim family pick within a fixed 8
pixels, because a command's `Context` does not carry `Session.Aperture`, and the picking clicks
snap like any point click.

## Failure

- A zero or negative radius or scale factor prints `Requires a positive and nonzero value.` and asks
  again.
- An empty selection ends the command without a change.
- `LAYER` with a colour outside 1–255 prints the valid range and changes nothing.

## Related docs

- [Command system](../tech/command-system.md) — the framework under every command
- [Command line](command-line.md) — how commands are typed and answered
- [Drawing canvas](canvas.md) — highlights, markers and the crosshair on screen

## Next steps

- STRETCH, ARRAY and MATCHPROP.
- FILLET and CHAMFER on arcs, circles and polylines; the TRIM, Multiple and Undo options of OFFSET, FILLET and CHAMFER.
- Grip editing and a properties palette.
- Polar tracking, object snap tracking, and the Perpendicular, Tangent and Extension snaps.
- PLINE arc segments.
