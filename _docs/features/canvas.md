---
id: feat-canvas
title: Drawing canvas
kind: feature
status: current
summary: The canvas and the chrome around it — pan and zoom, grid, crosshair and pickbox, highlights, selection boxes, snap markers, the layers and properties panels and the status bar.
read_when: you are changing pan, zoom, the crosshair, what the canvas draws, a panel or the status bar
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/canvas.go, ui/workspace.go]
depends_on: [tech-rendering, tech-command-system, tech-ui-framework]
review_cycle: monthly
---

# Drawing canvas

## Purpose

The canvas is where the drawing is seen and where clicks become points, picks and boxes. Around it,
a side panel lists the layers and describes the selection, and a status bar shows the cursor
position, the drafting toggles and the view scale. The view behaves like AutoCAD's model space: dark
background, a crosshair that follows the pointer, and snap markers that show what a click will hit.

## Behaviour

### View

- **Pan:** drag with the middle button, or scroll with Shift held (the wheel's horizontal and
  vertical deltas move the drawing).
- **Zoom:** the wheel zooms about the pointer; the model point under the pointer stays under it.
  Scale is clamped to `render.MinScale` and `render.MaxScale`.
- **Zoom to extents:** the first frame after `-open`, `-demo` or `-script` at startup frames the
  whole drawing with a 24 px margin, and `OPEN` does the same when it succeeds. `Ctrl+E` and
  `ZOOM` with `Extents` do it on demand.

### What the canvas draws

Later items draw over earlier ones.

| Item | Rule |
|---|---|
| Grid | F7 or the GRID flag toggles it. Lines sit every power of ten model units that is at least 16 px apart; every tenth line is stronger. It is omitted when more than 400 lines would show |
| Entities | Visible entities, on layers that are on, in their layer or entity colour; ACI 7 contrasts with the background |
| Hover highlight | The entity under the pickbox, in a lighter blue and thicker. Absent while a command asks for a point, while a box is dragged, and for selected entities |
| Selection highlight | Selected entities, in blue and thicker |
| Command preview | The rubber band and ghosts of the running command |
| Selection box | A **window** (left to right) is blue; a **crossing** (right to left) is green. It appears after the pointer moves 4 px from the press |
| UCS icon | Red X and green Y axes in the bottom-left corner, a fixed 36 px long |
| Crosshair and pickbox | Full-height and full-width hairlines through the cursor, with a small square pickbox at the cursor when no point is being asked for. While a point is asked for, the crosshair sits on the snapped point and there is no pickbox |
| Snap marker | While a point is asked for and a snap applies: a square for endpoint, a triangle for midpoint, a circle for centre, a diamond for quadrant, an X for intersection, an hourglass for nearest |

The crosshair and pickbox draw only while the pointer is over the canvas. The pointer is not hidden:
the operating system cursor shows beside the crosshair.

### Pointer

- **Left click** answers the point prompt when one is open; otherwise it picks, and a drag starts a
  box. Shift-click removes from the selection. What the click does inside a command belongs to
  [Drawing and editing commands](drawing-and-editing.md).
- **Right click** is Enter.
- A press on the canvas moves keyboard focus to the command line.

### Layers panel

Each layer shows its colour swatch, its name and an `on` or `off` button. The current layer's name
is bold. Clicking a row makes that layer current. Clicking the button toggles the layer off or on;
entities on an off layer are neither drawn nor selectable.

### Properties panel

With nothing selected it reads `No selection`. Otherwise it shows the count and a line per entity
class (`LINE × 3`); a single selected entity also shows its layer and handle. The panel is read-only.

### Status bar

| Part | Shows |
|---|---|
| Coordinates | The cursor in model units: the snapped point while a point is asked for, otherwise the pointer |
| GRID, OSNAP, ORTHO | Toggles. Bright when on, muted when off; click or press F7, F3, F8 |
| Right side | `Layer <current>`, the entity count, and `1 px = <size> units` |

## Contract

- View: `render.View` (`ZoomAt`, `PanBy`, `Fit`, `PixelSize`), owned by the workspace.
- Session state read: `Session.Cursor`, `Session.Snap`, `Session.CurrentPrompt`, `Session.Selection`,
  `Session.Aperture`, `Session.SnapOn`, `Session.Ortho`; written through `Session.Click`,
  `Session.SelectBox`, `Session.Hover` and `Session.Submit`.
- Document: reads layers and entities; the layer panel writes `Layer.Off` and
  `Drawing.CurrentLayer` directly, outside a transaction.

## Implementation

`ui.Workspace` in `ui/workspace.go` is the root view: toolbar, then the canvas beside `sidePanel`
(layers and properties), then `commandArea`, then `statusBar`. The toolbar buttons run the
commands they name. `Bindings` registers the key bindings for the `cad` key context.

`ui/canvas.go` holds the canvas. `Workspace.canvasElement` wires mouse down, move, up and wheel
to `Session.Click`, `Session.SelectBox`, `Workspace.pointerMoved` and the view. `pointerMoved`
updates the pan, the box state and the hover pick (`cmdsys.Pick`, with the aperture scaled by
`View.PixelSize`). `Workspace.paintCanvas` runs on each paint: it sizes the view, applies the
pending zoom to extents, assembles the display list in the order of the table above
(`gridItems`, `render.Builder.Build`, the highlights, `Session.Overlay`, `ucsIcon`,
`cursorItems`, `snapMarker`) and hands it to `gpuirender.Painter`. `Workspace.FitOnFirstFrame`
arms the zoom to extents; `cmd/cad` calls it after `-open`, `-demo`, `-script` and `-png`.

Colours are the `col*` and `ui*` variables in `ui/theme.go`.

## Failure

- A canvas with no size yet (the first layout pass) skips the pending zoom to extents until it has
  one.
- Zoom stops at the scale limits; panning is unbounded.
- A grid that would need more than 400 lines in either direction is skipped, not thinned.
- The layer panel's toggle and current-layer click are not undoable and leave the selection
  alone.

## Related docs

- [Rendering](../tech/rendering.md) — the view, the display list and the frame order
- [Command system](../tech/command-system.md) — the session, snaps and the input pipeline behind the clicks
- [UI framework](../tech/ui-framework.md) — the theme colours and the gpui boundary
- [Command line](command-line.md) — the keyboard shortcuts and Space, Enter and Escape

## Next steps

- Hide the operating system cursor over the canvas, which needs a platform API.
- Base-layer caching so a pointer move redraws only the overlay.
- Grips on selected entities and an editable properties palette.
