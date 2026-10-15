---
id: feat-canvas
title: Drawing canvas
kind: feature
status: current
summary: The canvas and the chrome around it — pan and zoom, grid, rulers, crosshair and pickbox, highlights, selection boxes, snap markers, the viewport label and zoom buttons, the title bar, the ribbon, the layers and properties panels, the layer popover and properties HUD, resizing and collapsing the panels, and the status bar.
read_when: you are changing pan, zoom, the crosshair, what the canvas draws, a panel or the status bar
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [ui/canvas.go, ui/ruler.go, ui/ruler_canvas.go, ui/workspace.go, ui/titlebar.go, ui/recent.go, ui/toolbar_layout.go, ui/tooltip.go, ui/sidepanel.go, ui/statusbar.go, ui/chrome.go, ui/layout.go, ui/cmdarea.go, ui/filedialog.go, ui/statusbar_zoom.go, ui/icon.go, ui/brand.go, ui/layerpop.go, ui/hud.go, ui/textedit.go, ui/textedit_view.go, ui/hud_drag.go]
depends_on: [tech-rendering, tech-command-system, tech-ui-framework]
review_cycle: monthly
---

# Drawing canvas

## Purpose

The canvas is where the drawing is seen and where clicks become points, picks and boxes. Around it,
a title bar and a ribbon hold the commands, a strip of [document tabs](document-tabs.md) over it switches
drawings, a side panel lists the layers and describes the
selection, and a status bar shows the cursor position, the drafting toggles and the view scale. The view behaves like AutoCAD's model space: dark
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
| Trackpad pinch (macOS native `gpui.PinchEvent`) | Zoom about the pinch position by factor `1+Delta` per step |
| Middle-button drag | Pan |
| Double middle click | Zoom to extents |

The [wheel setting](settings.md) `auto`, `zoom` or `pan` (`Workspace.WheelMode`) changes the table: `auto` is
it as written, `zoom` makes a precise vertical scroll zoom like a notch, and `pan` makes every unmodified scroll
pan, notch or precise, with only Ctrl or Cmd zooming. `Workspace.WheelZoom` makes a precise vertical scroll zoom as a mouse wheel does, for users who
want the plain wheel to always zoom (an accelerated mouse that the heuristic reads as a trackpad
is the usual reason). Scale is clamped to `render.MinScale` and `render.MaxScale`; panning is
unbounded. ebiten exposes no pinch, so the macOS backend reads it from an `NSEvent` magnify monitor and sends `gpui.PinchEvent`; Windows precision touchpads and browsers deliver it as Ctrl+wheel (the browser backend sets Control from `WheelEvent.ctrlKey`). Linux desktop has no pinch.

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
| Grid | F7 or the GRID flag toggles it. Lines sit every `GRIDUNIT` (x and y), widened by powers of ten until at least 16 px apart; every `GRIDMAJOR`-th (5) line is stronger. It is omitted when more than 400 lines would show |
| Entities | Visible entities, on layers that are on, in their layer or entity colour; ACI 7 contrasts with the background. One logical pixel wide, or with the LWT toggle on as wide as the layer's lineweight (1 mm is 3.8 px), never thinner than one pixel. TEXT and MTEXT draw in the built-in single-line font whatever their style; text under 3 to 6 pixels high (by zoom band) draws as one line per row |
| Hover highlight | The entity under the pickbox, in a lighter blue and thicker. Absent while a command asks for a point, while a box is dragged, and for selected entities |
| Selection highlight | Selected entities, in blue and thicker |
| Command preview | The rubber band and ghosts of the running command. What the command has made so far (LINE's segments, OFFSET's and COPY's copies, TEXT's lines, a TRIM pick) is not preview: it draws as entities, in their colours, from the moment it is made |
| Selection box | A **window** (left to right) is blue; a **crossing** (right to left) is green. It appears after the pointer moves 4 px from the press |
| UCS icon | Red X axis pointing right and green Y axis pointing up, each with an arrowhead and a `X` or `Y` letter at its tip, in the bottom-left corner, a fixed 40 px long |
| Crosshair and pickbox | Full-height and full-width hairlines through the cursor, with a small square pickbox at the cursor when no point is being asked for, and at the prompts that pick an object by a click (`Prompt.Pick`: AREA `Object`, MEASUREGEOM Radius and Angle), which do not snap. While a point is asked for, the crosshair sits on the snapped point and there is no pickbox |
| Snap marker | While a point is asked for and a snap applies: a square for endpoint, a triangle for midpoint, a circle for centre, a diamond for quadrant, an X for intersection, an hourglass for nearest, a right angle for perpendicular, a circle on a line for tangent |
| Rulers | On by default. A 20 px ruler along the top edge (X) and one along the left edge (Y, as wide as the labels at the ends of the visible range need, rounded up to three characters, so it keeps its width while panning until the labels gain or lose digits), in model units, on the panel colour with a border-coloured edge toward the drawing. Labelled ticks every 1, 2 or 5 times a power of ten, the smallest at least 80 px apart (X) or 48 px (Y); unlabelled minor ticks split that step into fifths (quarters for a 2) when they are at least 6 px apart. A line in the accent colour marks the cursor's position (the snapped one while a point is asked for) on each ruler. Labels are muted text, 11 px, with the decimals their step needs (five at the deepest zoom, where the Y step is 5e-5), so no two read the same |

The rulers lie over the drawing: the view still spans the whole canvas, so a ruler changes no model
coordinate, and a click, a snap, a box or the wheel on a ruler acts on the drawing under it. The viewport
label, the zoom buttons and the UCS icon move inside the rulers while they show. `Workspace.toggleRulers`
switches them; the choice is not saved.

The crosshair, pickbox and the status-bar coordinates show only while a real pointer position is
known and over the canvas. Before the pointer first moves in the window, and after it leaves the
canvas (the toolbar, a panel, outside the window), there is no crosshair and the coordinates are dimmed (they keep the last
position). The pointer is not hidden: the operating system cursor shows beside the crosshair.

The canvas is framed by a 1 px border in the theme's border colour (`uiBorder`), painted over the
drawing along the inside edge of the canvas area, so it reads as a viewport apart from the ribbon,
side panel and command line.

Two overlays float over the canvas, and a third at startup. At the top left a monospace, muted caption `[-][Top][2D Wireframe]`
(`ui.canvas.viewport_label`) names the view, the only one Cad-lite has. At the top right a rounded
compact 36 by 100 px column of three 30 px icon buttons zooms in and out by a step of 1.25 about the canvas centre and zooms
to extents; each is a step of its own for ZOOM Previous, has a tooltip, and a press on the column
never reaches the canvas, so it neither picks nor answers a point prompt.

The **start panel** is a card centred near the top of the canvas, headed `Recent drawings`, listing up to six
recent drawings (format, name, folder; greyed when the file was not found) and an `Open another drawing…` row that
shows the Open dialog. It shows only when the settings file was loaded at startup (never in a `-png` render), the
drawing is empty and untitled, and the recent list is not empty. It goes away for good on the first action: a
command (typed, from a button or a script), a press on the canvas, typing in the command line, Escape, or any
file opened. A press on the card never reaches the canvas. The lists and what clicking an entry does are in
[DXF files](dxf-files.md).

### Pointer

- **Left click** answers the point prompt when one is open; otherwise it picks, and a drag starts a
  box. Shift-click removes from the selection. What the click does inside a command belongs to
  [Drawing and editing commands](drawing-and-editing.md).
- **Right click** is Enter.
- **Double left click** on editable TEXT or MTEXT while idle opens the basic TEXTEDIT modal. During
  a command it keeps the command's point or selection behavior and opens no editor. The modal owns
  keyboard and pointer input, including drawing shortcuts, file drops and the wheel; Tab and
  Shift+Tab cycle its fields and buttons. Opening it ends any splitter, command-history or zoom
  drag; pointer movement in the modal cannot continue those background drags. Its body scrolls on small windows while Apply and Cancel
  stay visible. Escape cancels. The text-editing contract is in
  [Drawing and editing commands](drawing-and-editing.md).
- A press on the canvas moves keyboard focus to the command line.

### Layers panel

The column's top section is headed `Layers` (semibold, normal case, behind a disclosure chevron) with a
`+` icon button at its right that runs `LAYER` and a chevron that folds the whole column, where `name=colour` creates a layer. Each layer shows its colour swatch, its name, a lock switch (an open or closed padlock) and an `on`, `off` or `frozen` button (frozen is set by `-LAYER Freeze`; the button toggles off and on only). The current layer's name
is bold. Clicking a row makes that layer current, except a frozen layer, which cannot be current (as
`-LAYER Set` refuses it; the row's tooltip says to thaw it). Clicking the button toggles the layer off or on;
entities on an off layer are neither drawn nor selectable. Clicking the padlock locks or unlocks the layer:
a locked layer is drawn but its entities cannot be selected. The switches never make their layer current.
Changing the current layer, visibility or lock marks the drawing modified, so `CLOSE` and `QUIT` ask to save.
Selecting the current layer again, or trying to select a frozen layer, leaves that state unchanged.
Layer metadata edits do not create an undo step.

### Layer popover

Pressing `Layer <current>` in the status bar (AutoCAD's Layer drop-down) opens a popover above it: a filter
field that has the keyboard, the layers whose name contains the filter (ignoring case), at most ten rows
before the list scrolls, and a `New layer…` row. The rows are the Layers panel's rows (swatch, name, lock,
visibility), with the same effects, and the two always agree. Typing filters and highlights the first match;
Up and Down move the highlight; Enter makes the only match current, or else the highlighted one, and closes
the popover; a click on a row does the same. The switches act without closing it. `New layer…` closes it and
runs `LAYER` on the command line; with a filter that names no layer, even ignoring case (so `wall` is not
offered beside `Wall`), it reads `New layer "<filter>"` and creates that layer and makes it current at once,
marking the drawing modified.
Enter or a click on a frozen layer does nothing. Escape, a press anywhere outside and a second press on the
status bar item close it without effect (the press does not reach what is under it), and the keyboard goes
back to the command line; switching drawing tabs and opening the settings close it too. The popover opens
with the current layer highlighted.

### Properties panel

Below a splitter, under a `Properties` heading with its own disclosure chevron, the panel is read-only. With nothing selected it reads `No selection`. Otherwise it shows the count and a line per entity
class (`LINE × 3`); a single selected entity also shows its layer and handle.

### Properties HUD

The `PROPS` chip in the status bar shows a small read-only panel floating over the canvas at its top right,
left of and level with the zoom buttons by default, with the same lines as the Properties section
(`No selection` included). It is 184 px wide, narrowing when space requires, with 12 px body text and a
`Properties` header with a short grip and a drag tooltip. Both HUD backgrounds use the panel colour at
200/255 opacity (about 78%); their text and icons stay opaque. Presses stay on the HUD; the wheel
scrolls overflowing Properties content without zooming the canvas. Other releases go through so an
existing splitter or slider drag still ends.

Dragging the header moves Properties freely within the canvas. Releasing within 28 px of a corner
slot snaps it there with a 16 px canvas/ruler margin. The top-right slot clears the zoom column by
8 px; the top-left sits below the viewport label, and the bottom-left clears the UCS icon. Narrow
spaces and tall content retain these clearances by limiting the panel's height and scrolling its body.
Positions follow canvas resize, ruler changes and selection-height changes. A
panel narrower than 112 px or with less than 32 px of usable height is hidden and its drag ends;
the PROPS toggle and session position are retained, so it reappears when space returns.
During the drag a
transparent window-sized input layer and root move/release routing prevent canvas selection or pan
and end the drag over other controls. Hiding Properties, switching drawings or opening TEXTEDIT ends
the drag. Visibility remains shared across tabs and off at start; position and corner are remembered
per drawing for this session only. Neither is saved. The lines are worked out once per selection and
drawing change (`selectionSummary`), not per frame; `ui/hud_bench_test.go` measures whole-frame cost.

Both sections scroll when their rows do not fit. The column defaults to a sixth of the window wide, between
220 and 340 px, on the panel colour; with the agent panel open it sits between the canvas column and the
agent column, and both run from under the ribbon down to the status bar, so the command area spans only the
canvas.

### Resizing and collapsing

Every area can be resized with a splitter and folded away; a folded area keeps its size, so unfolding it
restores what it had. Splitters are 5 px hit areas with the 1 px border line in the middle; the line takes the
accent colour while the pointer is over it or a drag is in progress (gpui has no cursor styles, so the pointer
shape does not change). A drag follows the pointer through the root element's mouse move and up, so it
continues outside the splitter. A double click on a splitter or on the command handle resets that size to
automatic. Every tooltip names the drag and the reset.

| Area | Splitter | Limits | Fold |
|---|---|---|---|
| Layers and Properties column | its left edge (`split-side`) | 180 to 640 px | the chevron at the right of the Layers header folds the column to a 30 px strip with an expand button |
| Agent column | its left edge (`split-agent`) | 280 to 800 px | its `>` button hides the panel; the title bar button, F2 and `AI` bring it back |
| Layers and Properties sections | the line between them (`split-layers`) | from one row to all the rows | the chevron before each title folds that section to its header; the other takes the freed space |
| Command history | the bar on its top edge | one line to 480 px, and the canvas keeps at least 420 px of the window | the chevron at the right of the bar leaves only the input row |
| Ribbon | none | none | the chevron button in the title bar hides the button groups |

The canvas column never gets narrower than 240 px: when the columns want more (a saved width, a small window,
the agent panel opened), the agent column gives way first, then the Layers column, down to their minimums
(`columnWidths`). Saved widths are not changed by that; a larger window gets them back.

The Layers section uses only the space its rows need: its height is the header plus the rows, and Properties
takes the rest. Dragging its splitter sets a cap that can only be below the rows (the splitter stops where
the rows end, since more would be empty space); the list then scrolls, and a double click fits it to its rows
again. With many layers the list stops where Properties keeps its header and a 56 px body. With Properties
folded the list may take everything but its header. The command history keeps its fixed default height (96 px,
less in a short window) rather than fitting its lines, so the canvas does not move as lines appear.

The sizes and the folded flags are saved in the settings file (`layout` table, see [Settings](settings.md))
shortly after each change and when the app closes, and applied at startup.

### Title bar

A 44 px row on the background colour. From the left: the brand, a divider, the document chip, a
divider, Open with a small chevron beside it and Save, a divider, Undo and Redo; at the right end the ribbon toggle (a chevron that hides and shows the ribbon's button groups), the agent panel toggle and the gear
`Settings` button, which opens the [settings](settings.md) panel (also Ctrl+, or `OPTIONS`). Every
button is icon-only (28 px, an 18 px icon) with its tooltip. The document chip is a rounded box with
the file name, or `Untitled` while the drawing has no file, and a dot before the name while the drawing has unsaved
changes (`Session.Dirty`); its tooltip gives the path. The agent button is filled while the panel is
open and carries a dot while a session is live (amber when a background tab wants attention). Open and Save show the
system file dialog first (see [DXF files](dxf-files.md)); right-clicking Save is Save as. The chevron opens the
Open menu of recent files and folders, a dropdown under the Open button that closes on a choice, a press
anywhere else or Escape. Undo and
Redo run `U` and `REDO` and show the command tooltips (see Tooltips); Open, Save, Settings and the
agent button show the same kind of tooltip, their description from `ui.toolbar.open_tip`, `save_tip`,
`settings_tip` and `agent_tip` (Save's names Save as and its keys).

The brand depends on the OS (`Workspace.Brand`, chosen by `ui.BrandForOS`): Linux and the web show the
application icon and the bold `Cad-lite` wordmark, macOS shows the icon alone (32 px, the 32 and 64 px
window icons drawn 1:1 at UI scale 1 and 2), and Windows shows nothing, so the document chip starts at
the row's left padding (the window frame carries icon and title). `CADLITE_BRAND=mac|win|text`
forces a mode for `-png` renders. On a narrow window the document chip shrinks and clips its text
before anything else moves.

### Ribbon

Three panels, each with its name (`Draw`, `Modify`, `Utilities`) centred under its buttons in muted
12 px text and a vertical divider after it; on one row the ribbon is 84 px high. The title bar's ribbon toggle
hides the panels (the commands stay on the command line) and shows them again. **Draw** has large buttons (icon over label) for Line,
Polyline, Circle and Arc, then a column of two small ones (icon left of label), Rectangle over Polygon. **Modify** is a grid of small
buttons in two rows and seven columns: Move, Rotate, Mirror, Offset, Chamfer, Extend, Join over Copy,
Scale, Erase, Fillet, Trim, Break, Explode. **Utilities** is two columns of two: Distance, Area and ID point,
Zoom. Labels are the commands' display names in the interface language; a button runs its command as if typed, shows
the command's tooltip (see Tooltips) and, while its command runs, takes the soft accent fill and text. A command
without a button (LAYER, LIST, NEW, MEASUREGEOM) is still reached by typing it.

The ribbon reflows with the window width: on a wide window the groups share one row, and as the window
narrows whole groups wrap onto the next row. A group is never split across rows and nothing clips or overlaps. When the
three groups do not fit one row (`ribbonWidth` estimates the width from the labels), Utilities moves before Modify so
it sits beside Draw, and the ribbon takes two rows rather than three. With English labels, the
groups fit on one row at 1100 px; at 900 px, Draw and Utilities are the first row and Modify the second.

### Tooltips

Every clickable control carries a tooltip that appears after the pointer rests on it for 500 ms
(`gpui.Div.Tooltip`); it hides when the pointer leaves the control and on a click, and stays hidden
until the pointer leaves. Ribbon and title bar command buttons, Open, Save and the gear show two
lines: the command's display name with its keyboard hint in brackets, then its description —
`Line (L)`, `Undo (Ctrl+Z / U)`, `Save (Ctrl+S / QSAVE)`, `Linea (L)` in Italian. The hint is
composed, never written in the catalog: the key comes from the binding in `Bindings` (the Cmd form on
macOS, the Ctrl form elsewhere) and the typed part is the command's shortest registered alias
(`Registry.AliasesOf`), or the command itself when no alias is shorter, so a hint always names
something that works. The agent button, the layer rows (make current), the lock switches, the
visibility pills and the colour swatches, the document chip, the viewport label, the zoom buttons, the layer `+`,
the command history handle and its collapse button, the splitters, the panel chevrons and the recent commands button, the Open menu chevron and its rows, the layout tabs, the status bar toggles, the layer item and the popover's New layer row, and the coordinates readout, and
every control of the settings and agent panels. The text lives in the catalog (`*_tip` keys) in both
languages. The bubble follows the theme (`tooltipStyle`), wraps at 320 px and grows to the wrapped height.

### Status bar

| Part | Shows |
|---|---|
| Layout tabs | `Model` is the selected tab. `Layout1` and `Layout2` are disabled (paper space is not supported) and say so in their tooltip |
| Coordinates | The cursor as `x, y, 0.0000` in four decimals, monospace: the snapped point while a point is asked for, otherwise the pointer; dimmed and holding the last position while the pointer is not over the canvas |
| MODEL | A static label for the space being edited, with a tooltip |
| GRID, RULER, SNAP, ORTHO, POLAR, OSNAP, LWT, PROPS | Switch chips in monospace capitals (captions follow the language: `GRIGLIA`, `ORTO`, `POLARE`, `SNAP OGG.`, `SPESS`, `PROPR`, `RIGHELLI` in Italian). Muted when off; the soft accent fill, border and text when on. GRID is F7 (`Session.SetGrid`), SNAP is the snap grid, F9 (`Session.SetSnapGrid`), ORTHO is F8, POLAR is polar tracking, F10 (`Session.SetPolar`: the rubber band locks to multiples of `POLARANG` within 5 degrees; ortho and polar exclude each other), OSNAP is F3 (`Session.SnapOn`), and LWT draws layer lineweights (`LWDISPLAY`). All start as the settings say, with POLAR and LWT off. RULER shows or hides the canvas rulers (`Workspace.Rulers`, `toggleRulers`). PROPS shows the properties HUD (above) and starts off |
| Right side | `Layer <current>` (a press opens the layer popover), the entity count (`10 entities`), the drawing unit from `INSUNITS` (`mm`, or `unitless`; the tooltip gives `1 px = <size> units`), then the zoom slider and the scale as a percentage |
| Zoom | The slider is logarithmic from 0.1% to 100000% (scale 1e-3 to 1e3; a scale outside pins the thumb to an end), zooms about the canvas centre and updates the view while it is dragged. The thumb and percentage follow wheel, pinch and command zoom. A drag, a key press or a canvas zoom button is one step for ZOOM Previous (100% is one screen pixel per model unit) |

There is no plot scale in Cad-lite, so the unit stands where an AutoCAD status bar shows `1:50`. The status bar is
at least 32 px high; when the window is too narrow for the tabs and the right side together, the right side wraps onto a second
line at the right.

## Contract

- View: `render.View` (`ZoomAt`, `PanBy`, `Fit`, `PixelSize`), one per document tab; the canvas, the
  panels and the status bar show the active tab's view, session and drawing.
- Session state read: `Session.Cursor`, `Session.Snap`, `Session.CurrentPrompt`, `Session.Selection`,
  `Session.Aperture`, `Session.SnapOn`, `Session.Ortho`, `Session.Polar`, `Session.Grid`,
  `Session.SnapGrid`, `Session.LineweightDisplay`, `Session.Units`; written through `Session.Click`,
  `Session.SelectBox`, `Session.Hover` and `Session.Submit`.
- Document: reads layers and entities; the layer panel and the layer popover write `Layer.Off`,
  `Layer.Locked` and `Drawing.CurrentLayer` directly, outside a transaction.

## Implementation

`ui/textedit.go` holds the text-editing draft, target snapshot, validation and one-transaction Apply;
`ui/textedit_view.go` builds the modal from existing `TextInput` fields, with multiline contents for
MTEXT. `Workspace.withTextEditor` mounts it after the other overlays. TEXTEDIT reaches it through
the session's host callback; idle double-click uses `beginTextEditAt` and clears the canvas drag.
Apply checks the drawing, active command, target properties and layer eligibility again. A changed
or removed target reports an error; changing the active tab or replacing the drawing discards the
draft. The normal transaction listeners update the index, dirty counter and render cache.

`ui.Workspace` in `ui/workspace.go` is the root view: `titleBar` (`ui/titlebar.go`), `ribbon`, then a row of the
canvas column (the document tab strip `tabStrip` of `ui/doctabs.go`, then `canvasElement` over `commandArea`), `sidePanel` (`ui/sidepanel.go`) and the agent panel, then
`statusBar` (`ui/statusbar.go`), with 1 px dividers or splitters between (`Workspace.chrome`).
`ui/layout.go` holds `layoutState` (sizes, folded flags, the drag in progress), the pure sizing functions
`columnWidths`, `sideRange`, `agentRange`, `layersHeight`, `layersLimits` and `resizeSpan`, the `splitter`
and `collapseButton` elements, and `layoutSettings`, `applyLayout` and `persistLayout`, which move the layout
to and from `settings.Layout`; `sidePanel` measures its own height from a `gpui.ScrollHandle` of the previous frame. `ui/toolbar_layout.go` builds the ribbon:
`Workspace.ribbonGroups` lists the groups, `ribbonLarge`, `ribbonSmall` and `smallColumns` build the
buttons around `ui.NewIcon` (`ui/icon.go`) with `Workspace.commandTip` as the tooltip, and `Workspace.ribbon` puts each group in
a no-shrink `Div` inside a `FlexWrap` row, so gpui wraps between groups only. `ui/chrome.go` holds
the type scale, `iconButton`, `monoText` and the dividers shared by the chrome; `mix` in `ui/theme.go` derives the soft
accent fill. `Workspace.fileButton` (`ui/filedialog.go`) builds Open and Save with `openTip` and `saveTip`.
The rulers are `ui/ruler.go`, one implementation shared with `render_png`: `rulerStyle` places the X ruler at the top
(`canvasRulers`, theme colours, minor ticks and cursor marker) or at the bottom (`snapshotRulers`, fixed colours, the MCP
image), `rulerSteps` and `minorStep` choose the steps, `lines` adds bands and ticks to the display list and `labels` the
tick labels as absolute `Div`s; `ui/ruler_canvas.go` holds the toggle (`Rulers`, `SetRulers`, `toggleRulers`), `rulerInsets`
for the overlays, and the canvas's `rulerItems` and `rulerLabels`. `rulerFrame` works the canvas rulers' layout
out once per view and keeps it in `canvasState`, since the overlays, the ticks and the labels all ask for it every
frame. `exact` in `rulerStyle` (the canvas's) gives labels their step's decimals and the stable left width; without
it (`render_png`) labels have at most four decimals and the left ruler fits its longest label, which keeps the
MCP image's pinned pixels.
`ui/tooltip.go` composes the button tooltips: `commandTip`, `keyHint` (shortcut, then `typedName`),
`shortcutTable` and `keystrokeLabel` (binding to `Ctrl+Shift+S`), and `commandKeys`, the commands a key
runs directly, from which `Workspace.Render` also registers the key handlers.
`Workspace.openButtons` (`ui/recent.go`) adds the chevron and a zero-width marker before Open whose paint callback
records the button's position, because `openMenuOverlay` is mounted on the root after the chrome (gpui paints in
tree order and has no z-index) and must hang under it. `startPanel` is the last canvas overlay.
`toggleChip` and `layoutTab` in `ui/statusbar.go` build the status bar. `layerRow` and `propsBody` in
`ui/sidepanel.go` are the layer row and the Properties lines that the side panel, the layer popover and the HUD
share. `ui/layerpop.go` holds the status bar's overlay state (`statusState`, beside the workspace like the
settings state), `layerItem`, `layerPopover` and `statusOverlays`, which hangs the HUD and the popover from the
status bar as absolute children: gpui gives every div a hitbox over its whole box and delivers a press only to the
topmost one and its ancestors, so no overlay is larger than what it shows, except the popover's backdrop.
The state holds the app and session weakly, and the filter callbacks hold the workspace weakly, so an
opened popover cannot retain its window and drawing. Layer controls call `Session.MarkModified` once
for an actual persistent metadata change; selecting the unchanged current layer does not call it.
A zero-size marker records the bar's top and a zero-width one the layer item's position while painting; the
HUD (`ui/hud.go`) is placed from the canvas's bounds of the previous frame and asks for another frame when
they moved. `ui/hud_drag.go` constrains its placement around canvas chrome, keeps session-keyed
positions weakly in the status state, and handles free movement and corner snapping. During a drag,
`statusOverlays` adds the transparent release layer and the workspace routes move/up events; the
Properties body's scroll handle consumes the wheel before it can reach the canvas.
User-visible text goes through `i18n.T`
(`ui.*` keys); `Workspace.Render` installs the themed tooltip style (`tooltipStyle` in `ui/theme.go`) on the app
whenever the theme changes. `ui/brand.go` holds the title bar brand: `Brand`, `BrandForOS`, and a colour bitmap
element that paints `appicon.Image` as runs of device-pixel quads (gpui has no colour image primitive). `Bindings`
registers the key bindings for the `cad` key context.

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

`Workspace.viewportLabel`, `zoomPill` and `ucsLabels` in `ui/canvas.go` are the canvas overlays: absolutely
positioned children of the canvas `Div` (the UCS axes themselves are in the display list, `ucsIcon`). The pill
stops mouse down, up and wheel from bubbling to the canvas. `paintCanvas` copies `Session.LineweightDisplay`
to `render.Builder.Lineweights` each frame.

`ui/statusbar_zoom.go` holds the zoom controls: `Workspace.zoomControls` builds them around
`NewSlider` (`ui/slider.go`) with the `SliderState` in `Workspace.zoom`; `zoomToSlider` and
`sliderToZoom` map the scale to its decimal logarithm, and `Workspace.setZoom` and `zoomExtents` zoom about the canvas
centre or to the drawing and keep the `render.ViewStack` (`View.History`) step. The workspace root forwards mouse move and
up to the slider state, so a drag continues while the pointer is off the slider, as gpui has no
pointer capture. `paintFrame` in `ui/canvas.go` draws the canvas border.

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
- `Layout1` and `Layout2` do nothing: there is no paper space.
- Lineweights come from the layer only; an entity has no lineweight of its own yet.

## Related docs

- [Rendering](../tech/rendering.md) — the view, the display list and the frame order
- [Command system](../tech/command-system.md) — the session, snaps and the input pipeline behind the clicks
- [UI framework](../tech/ui-framework.md) — the theme colours and the gpui boundary
- [Command line](command-line.md) — the command box under the canvas, the keyboard shortcuts and Space, Enter and Escape

## Next steps

- Hide the operating system cursor over the canvas, which needs a platform API.
- Base-layer caching so a pointer move redraws only the overlay.
- Grips on selected entities and an editable properties palette.
- Paper space layouts behind the `Layout1` and `Layout2` tabs.
