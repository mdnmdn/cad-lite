---
id: feat-canvas
title: Drawing canvas
kind: feature
status: current
summary: The canvas and the chrome around it — pan and zoom, grid, crosshair and pickbox, highlights, selection boxes, snap markers, the layers and properties panels and the status bar.
read_when: you are changing pan, zoom, the crosshair, what the canvas draws, a panel or the status bar
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/canvas.go, ui/workspace.go, ui/icon.go, ui/brand.go]
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

Gestures follow AutoCAD for a mouse and macOS for a trackpad. The backend tells them apart with
`gpui.ScrollWheelEvent.Precise` (a heuristic, documented in the [gpui events guide](../../internal/gpui/docs/concepts/events-and-input.md)).

| Input | Effect |
|---|---|
| Mouse wheel, vertical notch | Zoom about the pointer, about 8.7% per notch; the model point under the pointer stays under it |
| Shift + wheel, or a horizontal notch | Pan |
| Trackpad two-finger scroll (precise) | Pan by the deltas in the direction delivered |
| Ctrl or Cmd + wheel, which is how a browser delivers a pinch | Zoom about the pointer, smoothly: factor `exp(-dy/100)` for precise deltas, the notch step otherwise; one event is limited to 200 px |
| Middle-button drag | Pan |
| Double middle click | Zoom to extents |

The [wheel setting](settings.md) `auto`, `zoom` or `pan` (`Workspace.WheelMode`) changes the table: `auto` is
it as written, `zoom` makes a precise vertical scroll zoom like a notch, and `pan` makes every unmodified scroll
pan, notch or precise, with only Ctrl or Cmd zooming. `Workspace.WheelZoom` makes a precise vertical scroll zoom as a mouse wheel does, for users who
want the plain wheel to always zoom (an accelerated mouse that the heuristic reads as a trackpad
is the usual reason). Scale is clamped to `render.MinScale` and `render.MaxScale`; panning is
unbounded. There is no native pinch on desktop: ebiten does not expose one.

- **Resize:** when the canvas size changes (the window is resized, or F2 opens or closes the agent
  panel) the view keeps its centre and changes its scale so the model box that was visible stays
  fully visible; it never clips. The scale is derived from the last one the user chose, so
  closing the panel restores the view exactly. Zooming or running `ZOOM` between the two starts a
  new reference.
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

The crosshair, pickbox and the status-bar coordinates show only while a real pointer position is
known and over the canvas. Before the pointer first moves in the window, and after it leaves the
canvas (the toolbar, a panel, outside the window), there is no crosshair and the coordinates read
`—`. The pointer is not hidden:
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

### Toolbar

Each button is an icon over a 10px label, in two rows of groups separated by a rule. Top row: Line,
Polyline, Circle, Arc, Rect; Dist, Area, ID point; Undo, Redo, Zoom; at the right end the `AI` agent
toggle and the gear `Settings` button, which opens the [settings](settings.md) panel (also Ctrl+, or
`OPTIONS`). Second row: Move, Copy, Rotate, Scale, Erase; Mirror, Offset, Fillet, Chamfer; Trim,
Extend, Break, Join, Explode. A button runs its command as if typed, and the button of the running
command is highlighted. Every button shows its command's icon from the
[icon pipeline](../tech/ui-framework.md); a command without a button (LAYER, LIST, NEW, OPEN, SAVE,
MEASUREGEOM) is still reached by typing it.

The top row starts with a brand that depends on the OS (`Workspace.Brand`, chosen by `ui.BrandForOS`):
macOS shows the application icon (32px, the 32 and 64 px window icons drawn 1:1 at UI scale 1 and
2), Windows shows nothing and the buttons start at the row's left padding (the window frame
carries icon and title), and Linux and the web show the `go-draw` wordmark. `GODRAW_BRAND=mac|win|text`
forces a mode for `-png` renders.

Labels come from the catalog (`ui.toolbar.*`, or the command's display name for the buttons without a
short caption), so they follow the interface language; Italian labels fit the same buttons without
clipping.

### Tooltips

Every clickable control carries a tooltip that appears after the pointer rests on it for 500 ms
(`gpui.Div.Tooltip`): toolbar buttons show the command's tip (`i18n.Command(name).Tip`: title,
what it does, shortcut or alias), the gear and agent buttons, the layer rows (make current), the
visibility pills and the colour swatches, the status bar toggles and the coordinates readout, and
every control of the settings and agent panels. The text lives in the catalog (`*_tip` keys) in both
languages. The bubble follows the theme (`tooltipStyle`), wraps at 320 px and grows to the wrapped height.

### Status bar

| Part | Shows |
|---|---|
| Coordinates | The cursor in model units: the snapped point while a point is asked for, otherwise the pointer; `—` while the pointer is not over the canvas |
| GRID, OSNAP, ORTHO | Toggles, each with a 14px icon (captions follow the language, `GRIGLIA`, `SNAP OGG.`, `ORTO` in Italian). Bright (`uiOn`) when on, muted (`uiMuted`) when off; click or press F7, F3, F8 |
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
(layers and properties), then `commandArea`, then `statusBar`. `Workspace.button` builds a toolbar
button around `ui.NewIcon` (`ui/icon.go`) with the tooltip from `i18n.Command`, and `statusBar` does
the same for the toggles. User-visible text goes through `i18n.T` (`ui.*` keys); `Workspace.Render`
installs the themed tooltip style (`tooltipStyle` in `ui/theme.go`) on the app whenever the theme
changes. `ui/brand.go` holds the toolbar brand: `Brand`, `BrandForOS`, and a colour bitmap element that paints
`appicon.Image` as runs of device-pixel quads (gpui has no colour image primitive). `Bindings` registers the key bindings for the `cad` key context.

`ui/canvas.go` holds the canvas. `Workspace.canvasElement` wires mouse down, move, up and wheel
to `Session.Click`, `Session.SelectBox`, `Workspace.pointerMoved` and the view. `pointerMoved`
updates the pan, the box state and the hover pick (`cmdsys.Pick`, with the aperture scaled by
`View.PixelSize`). `Workspace.paintCanvas` runs on each paint: it sizes the view, applies the
pending zoom to extents, assembles the display list in the order of the table above
(`gridItems`, `render.Builder.Build`, the highlights, `Session.Overlay`, `ucsIcon`,
`cursorItems`, `snapMarker`) and hands it to `gpuirender.Painter`. `Workspace.wheel` maps a scroll event to the table above and `Workspace.resizeView`, called first
in `paintCanvas`, applies the resize rule; a frame that changes the view refreshes the pointer
position and the window. `Workspace.FitOnFirstFrame`
arms the zoom to extents; `cmd/cad` calls it after `-open`, `-demo`, `-script` and `-png`.

Colours are the `col*` and `ui*` variables in `ui/theme.go`.

## Failure

- A desktop window gets no event when the pointer leaves it: the last in-window position stays
  until the pointer enters another element, so the crosshair can linger after the pointer exits
  through the canvas edge of the window.
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
