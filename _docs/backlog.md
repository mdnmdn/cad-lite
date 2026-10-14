# Backlog

Every open question, known gap and deferred item in go-draw, one line each. An entry names the
document it affects, so resolving it tells you what to update. Resolving or reprioritising an entry
is a product decision; add lines freely, and remove one only when the work lands and the affected
document says so.

## Open decisions and risks

From the specification ([`inbox/completed/draft-specs-v0.1.md`](inbox/completed/draft-specs-v0.1.md)).

| id | Item | Affects |
|---|---|---|
| O1 | DWG library: Open Design Alliance or LibreDWG. Licensing (commercial versus GPLv3) drives the whole project; decide before M4 | [File I/O](tech/file-io.md) |
| O2 | Application licence; depends on O1 | [File I/O](tech/file-io.md) |
| O3 | SHX fonts: substitute TrueType in v1, or implement an SHX parser | [Product overview](product/overview.md) |
| O4 | WASM and DWG: cgo libraries do not run in a browser, so the browser build is DXF-only | [File I/O](tech/file-io.md) |
| R1 | Large drawings on a CPU rasteriser: every pointer move re-rasterises the window. Mitigation is base-layer caching (G13) | [Rendering](tech/rendering.md) |
| R2 | Numerical robustness of TRIM, EXTEND and OFFSET on near-tangent geometry; there is no regression corpus for TRIM and EXTEND, and `geom.Intersect` reports no points for overlapping collinear or concentric pieces | [Geometry kernel](tech/geometry-kernel.md) |
| R3 | Look and feel: functional, not native-looking; acceptable for v1 | [UI framework](tech/ui-framework.md) |

## Milestones

| # | Milestone | Spike covers | Missing |
|---|---|---|---|
| M1 | Kernel, model, DXF read | Kernel, document, spatial index, DXF read for LINE, CIRCLE, ARC, polylines, layers, handles | Unknown objects in newer classes, blocks, text, other entities (G1, G7) |
| M2 | Viewer | Pan, zoom, layers on and off | Linetypes, lineweights, text, hatch (G7) |
| M3 | Editing core | LINE, PLINE, CIRCLE, ARC, RECTANG, MOVE, COPY, ERASE, ROTATE, SCALE, snaps, selection, undo and redo, DXF save (R12) | Lossless round trip for newer classes (G2) |
| M4 | DWG | Reader and writer interfaces | The library (O1) |
| M5 | Full v1 command set | The commands in the drawing and editing document | G8, G9 |
| M6 | Annotation and output | None | Dimensions, MTEXT, layouts, SVG and PDF export (G7, G17) |
| M7 | Browser build | The binary builds for `js/wasm` and renders headlessly | G12 |

## Known gaps

| id | Gap | Affects |
|---|---|---|
| G1 | Unknown objects in OBJECTS and CLASSES, unmodelled header variables and paper-space (code 67) entities are not preserved or separated; unknown entities of classes R12 lacks cannot be saved until G2 | [DXF files](features/dxf-files.md), [File I/O](tech/file-io.md) |
| G2 | Writing newer DXF versions (AC1015+: handles and owners, BLOCK_RECORD, CLASSES, OBJECTS) so unknown SPLINE, MTEXT, HATCH and custom entities survive a save; a binary DXF reader | [File I/O](tech/file-io.md) |
| G3 | R-tree: the spatial index is a linear scan | [Geometry kernel](tech/geometry-kernel.md) |
| G4 | Pointer move re-rasterises the whole window; no base-layer cache | [Rendering](tech/rendering.md), [Drawing canvas](features/canvas.md) |
| G5 | No large-drawing benchmark and no DXF corpus test | [Testing](tech/testing.md) |
| G6 | HiDPI, clipboard, IME and window resize are not exercised in the real-window harness | [Operations](tech/operations.md) |
| G7 | Unsupported content: linetypes, lineweights, TEXT, MTEXT, HATCH, INSERT and blocks, dimensions, ELLIPSE, SPLINE; the extrusion direction (code 210) is ignored | [Document model](tech/document-model.md), [File I/O](tech/file-io.md) |
| G8 | Missing commands and options: STRETCH, ARRAY, MATCHPROP; FILLET and CHAMFER on arcs, circles and polylines; polyline vertex fillet; chamfer by angle; the NO-TRIM and Multiple modes; the Undo, Multiple and layer options of OFFSET | [Drawing and editing commands](features/drawing-and-editing.md) |
| G9 | Grips and a properties palette; the properties panel is read-only | [Drawing canvas](features/canvas.md) |
| G10 | Polar tracking, object snap tracking, and the Perpendicular, Tangent and Extension snaps | [Command system](tech/command-system.md) |
| G11 | PLINE arc segments | [Drawing and editing commands](features/drawing-and-editing.md) |
| G12 | Browser build: file open and save need browser APIs; the file registry is operating-system based | [Operations](tech/operations.md), [DXF files](features/dxf-files.md) |
| G13 | Base-layer caching, the answer to R1 | [Rendering](tech/rendering.md) |
| G15 | The operating system cursor is not hidden over the canvas; it needs a platform API. The crosshair can linger when the pointer leaves through the window edge, because the platform sends no leave event | [Drawing canvas](features/canvas.md) |
| G16 | Layer table edits are not journalled, so they are not undoable and the render cache is cleared on every command-line submission (D11) | [Document model](tech/document-model.md), [Decision register](tech/decisions.md) |
| G17 | Paper space and layouts, view rotation, UCS | [Product overview](product/overview.md) |
| G18 | Native open and save dialogs | [DXF files](features/dxf-files.md) |
| G20 | Command-line autocomplete, history recall and F1 help | [Command line](features/command-line.md) |

## Further items

New lines are appended below this heading.

| id | Item | Affects |
|---|---|---|
| G21 | Toolbar tooltips and keyboard hints | [Drawing canvas](features/canvas.md) |
| G22 | The layer-visible, layer-hidden and layer-current icons are drawn but unused; the layers panel shows `on` and `off` text | [Drawing canvas](features/canvas.md) |
| G23 | The view-pan, view-zoom-extents, measure-angle, measure-radius and measure-id-point icons have no toolbar buttons | [Drawing canvas](features/canvas.md) |
| G24 | A Go render helper so the icon sheets use oksvg, the application's rasterizer, instead of resvg | [Operations](tech/operations.md) |
| G25 | measure-radius may need a redraw after review | [UI framework](tech/ui-framework.md) |
| G26 | The agent panel prompt is a single-line field; a multi-line prompt needs `TextInput` support | [Agent panel](features/agent-panel.md) |
| G27 | Pick a model, and persist the transcript with the drawing file (sessions persist per project in `.go-draw/agent/`) | [Agent panel](features/agent-panel.md) |
| G28 | Live smoke tests of the opencode and Codex backends against the real CLIs; only fakes run in CI (Claude Code is verified live) | [Agent integration](tech/agent-integration.md) |
| G29 | Alternative to image results: a small CLI, found through an environment variable, that talks to the running instance and writes a PNG for agents whose harness drops MCP images | [Agent integration](tech/agent-integration.md) |
| G30 | Agent token usage: `TurnEnd` has no usage field, so usage only reaches the debug log and the panel cannot show it | [Agent panel](features/agent-panel.md) |
| G31 | Live tests of agent session resume and of killing the Claude process on close | [Agent integration](tech/agent-integration.md) |
| G32 | Handle-taking MCP tools accept unknown-entity handles and silently do nothing; they should return an error (`parseHandles` in `cadmcp/tools.go`) | [Agent integration](tech/agent-integration.md) |
| G33 | A live test of `ResumeID` against the real CLIs | [Agent integration](tech/agent-integration.md) |
| G34 | Delete and rename stored agent sessions from the history list | [Agent panel](features/agent-panel.md) |
| G35 | The command history keeps the last prompt text after LINE ends (cosmetic, `cmdsys`) | [Command line](features/command-line.md) |
| G36 | The point prompts of TRIM, EXTEND and BREAK snap the click to nearby points, and the pick radius is a fixed 8 pixels because `Context` cannot see `Session.Aperture` | [Drawing and editing commands](features/drawing-and-editing.md) |
| G37 | OFFSET, FILLET and CHAMFER cannot pick entities created earlier in the same command (the index updates on commit), and their remembered settings are process-wide rather than per session | [Drawing and editing commands](features/drawing-and-editing.md) |
| G38 | Confirm the macOS signing identity and the bundle identifier (`it.synesthesia.go-draw`) | [Operations](tech/operations.md) |
| G39 | Windows Authenticode signing is not wired; unsigned executables trigger SmartScreen | [Operations](tech/operations.md) |
| G41 | UI/UX review findings in `inbox/review-ux.md`, to be triaged | [Drawing canvas](features/canvas.md) |
| G42 | Settings: persist the agent panel width and the side panel visibility | [Settings](features/settings.md) |
| G43 | Settings: a key-binding editor | [Settings](features/settings.md) |
| G44 | gpui has no pointer capture, so the UI-scale slider applies only on release instead of live while dragging | [Settings](features/settings.md) |
| G45 | Tab traversal: no Tab binding to FocusNext, so the panels cannot be walked by keyboard | [UI framework](tech/ui-framework.md) |
