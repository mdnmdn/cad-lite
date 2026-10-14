# Backlog

Every open question, known gap and deferred item in Cad-lite, one line each. An entry names the
document it affects, so resolving it tells you what to update. Resolving or reprioritising an entry
is a product decision; add lines freely, and remove one only when the work lands and the affected
document says so.

## Open decisions and risks

From the specification ([`inbox/completed/draft-specs-v0.1.md`](inbox/completed/draft-specs-v0.1.md)).

| id | Item | Affects |
|---|---|---|
| O2 | Application licence. DWG no longer constrains it (the reader is pure Go from an MIT-licensed port, D24); the ACadSharp notice must ship with the binaries | [File I/O](tech/file-io.md) |
| O3 | SHX fonts: substitute TrueType in v1, or implement an SHX parser | [Product overview](product/overview.md) |
| R1 | Large drawings on a CPU rasteriser: every pointer move re-rasterises the window. Mitigation is base-layer caching (G13) | [Rendering](tech/rendering.md) |
| R2 | Numerical robustness of TRIM, EXTEND and OFFSET on near-tangent geometry; there is no regression corpus for TRIM and EXTEND, and `geom.Intersect` reports no points for overlapping collinear or concentric pieces | [Geometry kernel](tech/geometry-kernel.md) |
| R3 | Look and feel: functional, not native-looking; acceptable for v1 | [UI framework](tech/ui-framework.md) |

## Milestones

| # | Milestone | Spike covers | Missing |
|---|---|---|---|
| M1 | Kernel, model, DXF read | Kernel, document, spatial index, DXF read for LINE, CIRCLE, ARC, polylines, layers, handles | Unknown objects in newer classes, blocks, text, other entities (G1, G7) |
| M2 | Viewer | Pan, zoom, layers on and off | Linetypes, lineweights, text, hatch (G7) |
| M3 | Editing core | LINE, PLINE, CIRCLE, ARC, RECTANG, MOVE, COPY, ERASE, ROTATE, SCALE, snaps, selection, undo and redo, DXF save (R12) | Lossless round trip for newer classes (G2) |
| M4 | DWG | DWG reader, pure Go, R14 to R2018: layers, units, LINE, CIRCLE, ARC and polylines | A writer (G60) and the rest of the entities (G61) |
| M5 | Full v1 command set | The commands in the drawing and editing document | G8, G9 |
| M6 | Annotation and output | None | Dimensions, MTEXT, layouts, SVG and PDF export (G7, G17) |
| M7 | Browser build | The binary builds for `js/wasm` and renders headlessly | G12 |

## Known gaps

| id | Gap | Affects |
|---|---|---|
| G1 | Unknown objects in OBJECTS and CLASSES, unmodelled header variables and paper-space (code 67) entities are not preserved or separated; unknown entities of classes R12 lacks cannot be saved until G2 | [DXF files](features/dxf-files.md), [File I/O](tech/file-io.md) |
| G2 | Writing newer DXF versions (AC1015+: handles and owners, BLOCK_RECORD, CLASSES, OBJECTS) so unknown SPLINE, MTEXT, HATCH and custom entities survive a save; a binary DXF reader | [File I/O](tech/file-io.md) |
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
| G16 | Layer table edits are not journalled, so they are not undoable. Typed layer commands and side-panel creation still do not mark the drawing modified; panel/popover switches, current-layer changes and named popover creation call `Session.MarkModified` (the render cache detects layer changes by comparing the layer table, D27) | [Document model](tech/document-model.md), [Decision register](tech/decisions.md) |
| G17 | Paper space and layouts, view rotation, UCS | [Product overview](product/overview.md) |
| G18 | Typed OPEN and SAVEAS never show the native dialog (FILEDIA is a stored variable with no effect); the dialogs are reached only from the buttons and shortcuts | [DXF files](features/dxf-files.md) |
| G20 | Command line: F1 help listing the commands, and Up/Down recall of earlier answers at a command's prompt (AutoComplete and recall at `Command:` exist) | [Command line](features/command-line.md) |

## Further items

New lines are appended below this heading.

| id | Item | Affects |
|---|---|---|
| G22 | The layer-visible, layer-hidden and layer-current icons are drawn but unused; the layers panel shows `on` and `off` text | [Drawing canvas](features/canvas.md) |
| G23 | The view-pan, view-zoom-extents, measure-angle, measure-radius and measure-id-point icons have no toolbar buttons | [Drawing canvas](features/canvas.md) |
| G24 | A Go render helper so the icon sheets use oksvg, the application's rasterizer, instead of resvg | [Operations](tech/operations.md) |
| G25 | measure-radius may need a redraw after review | [UI framework](tech/ui-framework.md) |
| G28 | Live smoke tests of the opencode and Codex backends against the real CLIs; only fakes run in CI (Claude Code is verified live) | [Agent integration](tech/agent-integration.md) |
| G29 | Alternative to image results: a small CLI, found through an environment variable, that talks to the running instance and writes a PNG for agents whose harness drops MCP images | [Agent integration](tech/agent-integration.md) |
| G30 | Agent token usage: `TurnEnd` has no usage field, so usage only reaches the debug log and the panel cannot show it | [Agent panel](features/agent-panel.md) |
| G31 | Live tests of agent session resume and of killing the Claude process on close | [Agent integration](tech/agent-integration.md) |
| G32 | Handle-taking MCP tools accept unknown-entity handles and silently do nothing; they should return an error (`parseHandles` in `cadmcp/tools.go`) | [Agent integration](tech/agent-integration.md) |
| G33 | A live test of `ResumeID` against the real CLIs | [Agent integration](tech/agent-integration.md) |
| G34 | Delete and rename stored agent sessions from the history list | [Agent panel](features/agent-panel.md) |
| G35 | The command history keeps the last prompt text after LINE ends (cosmetic, `cmdsys`) | [Command line](features/command-line.md) |
| G36 | The pick radius of the TRIM, EXTEND and BREAK object prompts is a fixed 8 pixels because `Context` cannot see `Session.Aperture` (they no longer snap the pick: `Prompt.Pick`) | [Drawing and editing commands](features/drawing-and-editing.md) |
| G37 | The remembered settings of OFFSET, FILLET and CHAMFER are process-wide rather than per session (picking entities made earlier in the same command works since in-flight edits reach the spatial index) | [Drawing and editing commands](features/drawing-and-editing.md) |
| G38 | Confirm the macOS signing identity and the bundle identifier (`it.synesthesia.cad-lite`) | [Operations](tech/operations.md) |
| G39 | Windows Authenticode signing is not wired; unsigned executables trigger SmartScreen | [Operations](tech/operations.md) |
| G41 | UI/UX review findings in `inbox/review-ux.md`, to be triaged | [Drawing canvas](features/canvas.md) |
| G42 | Settings: persist the agent panel width and the side panel visibility | [Settings](features/settings.md) |
| G43 | Settings: a key-binding editor | [Settings](features/settings.md) |
| G44 | gpui has no pointer capture, so the UI-scale slider applies only on release instead of live while dragging | [Settings](features/settings.md) |
| G45 | Tab traversal: no Tab binding to FocusNext, so the panels cannot be walked by keyboard | [UI framework](tech/ui-framework.md) |
| G48 | `tools/gui` runs `RemoveAll` on `-out/<scenario>`; refuse paths outside `_tmp/` unless `-force` | [Testing](tech/testing.md) |
| G49 | The `cad` MCP server has no cap on batch length and no `WriteTimeout` (only `IdleTimeout` and the header timeout are set) | [Agent integration](tech/agent-integration.md) |
| G50 | SAVE has no path allow-list (agents cannot run it, humans can); the window close button skips the "Save changes" question (G59) | [DXF files](features/dxf-files.md) |
| G51 | gpui `text.go` uses the deprecated `ClusterIndex` (staticcheck SA1019); fixing it needs a `go-text/typesetting` bump | [UI framework](tech/ui-framework.md) |
| G52 | Promote the random-script fuzz to a `go test -fuzz` target with `NaN` and `1e308` tokens in the pool, and add a fuzz target for the DXF reader | [Testing](tech/testing.md) |
| G53 | The Italian terms marked `(verify)` in `i18n/locales/it.toml` (36 entries) need a review by a CAD-fluent Italian speaker; `cadmcp` relies on the session being English for agents, so an agent never sees them | [Drawing and editing commands](features/drawing-and-editing.md), [Agent integration](tech/agent-integration.md) |
| G54 | Updates: the checksum list lives in the same release as the archive, so it detects corruption but not a tampered release; sign the archives (minisign or cosign) and verify in `updater` | [Updates](features/updates.md) |
| G55 | Updates: delta updates and a rollback to the `.old` copy are not implemented | [Updates](features/updates.md) |
| G56 | Updates: an available update shows only as the gear badge; a status-bar notice is not wired | [Updates](features/updates.md) |
| G57 | Updates: a real install and relaunch on Windows and macOS is untested (tests simulate the layouts on Linux); macOS in `/Applications` for a non-admin user falls back to the release page | [Updates](features/updates.md) |
| G58 | Codex: a withdrawn permission request (`serverRequest/resolved`) is dropped silently; the agent panel keeps showing the prompt until it is answered and the answer fails. Emit a withdrawal event and remove the prompt | [Agent panel](features/agent-panel.md), [Agent integration](tech/agent-integration.md) |
| G59 | The window's close button quits without the "Save changes" question: gpui has no close-request callback the app can veto; add one to `Platform`/`PlatformWindow` and ask when `Session.Dirty()` | [DXF files](features/dxf-files.md), [UI framework](tech/ui-framework.md) |
| G60 | DWG writing: the object map, classes, the section container (R2000 first, then R2004) and the bit-coded records of every object the document holds; until then a drawing opened from DWG saves as DXF | [File I/O](tech/file-io.md), [DXF files](features/dxf-files.md) |
| G61 | DWG entities that are skipped and counted today: TEXT and MTEXT, INSERT and blocks (BLOCK_HEADER and the block definitions), POINT, ELLIPSE, SPLINE, HATCH, DIMENSION, LEADER and MULTILEADER, 3D entities; they need document support first (G7) | [File I/O](tech/file-io.md), [Document model](tech/document-model.md) |
| G62 | DWG paper space, layouts and viewports are counted but not read (G17) | [File I/O](tech/file-io.md) |
| G63 | DWG R13 (`AC1012`) is accepted as R14 but untested for lack of a sample; R12 and older (`AC1009` and before) have a different, unrelated format and are refused | [File I/O](tech/file-io.md) |
| G64 | DWG linetype definitions, text styles and the other tables are not read, so a layer's linetype is a name only; the R12 DXF writer drops it (D7); true colours map to the nearest ACI index and entity lineweights and linetypes are not kept | [File I/O](tech/file-io.md), [Document model](tech/document-model.md) |
| G65 | The DWG reader mirrors entities whose extrusion is (0,0,-1) and the DXF reader does not (code 210 is ignored, G7); make the two agree | [File I/O](tech/file-io.md) |
| G66 | DWG code pages 857, 861, 864, 869 and the UTF-16 one decode as Windows-1252 | [File I/O](tech/file-io.md) |
| G67 | DWG: read the R2018+ header variables beyond `$INSUNITS` that the document could use ($CECOLOR, $LTSCALE, $PDMODE), and `AcDb:SummaryInfo` | [File I/O](tech/file-io.md) |
| G68 | Images in agent prompts are verified only against the fake agents: the Codex `inputModalities` field and image item shape, opencode's answer to an image on a text-only model, and the opencode model ids (`opencode/big-pickle`, `*-free`) are assumptions | [Agent integration](tech/agent-integration.md), [Agent panel](features/agent-panel.md) |
| G69 | Images are decoded and scaled on the UI goroutine when attached, so a very large file stalls the panel briefly; move it off the goroutine | [Agent panel](features/agent-panel.md) |
| G70 | OpenCode auto-install is run for real only on Linux; macOS and Windows terminals and mise there, the aqua backend with a reachable GitHub API, and a real `opencode auth login` are untested. The direct-download fallback has HTTPS only, because opencode publishes no checksum list | [Agent integration](tech/agent-integration.md) |
| G71 | The Windows installer's shell integration, the macOS open-documents handler and `Info.plist` document types, and a real file drop on the window are untested on their platforms | [DXF files](features/dxf-files.md), [Operations](tech/operations.md) |
| G72 | Flaky agentmanager tests under parallel package runs: `claude` TestOpenMissingBinary (leaked temp MCP config) and `harnesses` TestDiscoverReportsPathsAndVersions (empty version); both pass alone and with `-p 1` | [Testing](tech/testing.md) |
| G75 | `geom.CutPolyline` accepts a cut overshooting by 1e-9 and emits a reversed ulp-long segment when a cut uses a whole side; `geom.NormAngle` returns exactly 2π for a tiny negative angle. `cadmcp` works around both | [Geometry kernel](tech/geometry-kernel.md) |
| G76 | `run_script` is not covered by the MCP number caps: a command fed huge finite values (SCALE 1e300) can store geometry that `get_drawing` cannot encode as JSON | [Agent integration](tech/agent-integration.md), [Command system](tech/command-system.md) |
| G77 | gpui rasteriser: a box shadow is computed over its whole area, including the part a solid element covers, and translucent fills have no fast path (about 10 ms per frame at 2x for a window-sized one); the settings card and the restart dialog avoid or still pay it | [UI framework](tech/ui-framework.md) |
| G78 | The settings AI tab looks up the agent programs on PATH and stats files every frame (`ui/agent_binary.go`, `ui/agent_install.go`); cache until discovery runs again | [Settings](features/settings.md) |
| G79 | TEXTSIZE and TEXTSTYLE are session variables, not saved in the drawing header as AutoCAD does; needs a session-to-drawing sync and header variables in the DXF writer | [Drawing and editing commands](features/drawing-and-editing.md), [File I/O](tech/file-io.md) |
| G80 | The Hershey font notice (`doc/fonts/NOTICE`) is not shipped with the release archives and installers, and there is no About or licences view | [Operations](tech/operations.md) |
| G82 | Headless `-png` in the light theme draws colour 7 near-white, as if the render cache were built before the theme is applied | [Operations](tech/operations.md), [Rendering](tech/rendering.md) |

## Next features

Requested next, in no fixed order.

| id | Item | Affects |
|---|---|---|
| N8 | Agent panel: a ring that fills with the context-window usage, and a Compact button that asks the agent to compact its context | [Agent panel](features/agent-panel.md) |
| N9 | Agent drawing API, from [`inbox/Cad-lite-improvement-suggestions.txt`](inbox/Cad-lite-improvement-suggestions.txt) (suggested order: N9 to N14, then the rest): native TEXT and MTEXT, entity, renderer, DXF and MCP `add_text`/`update_text`; TEXT is writable in R12, MTEXT needs G2 (G7, G61, O3) | [Document model](tech/document-model.md), [Agent integration](tech/agent-integration.md) |
| N10 | MCP batch tool: many operations in one call and one undo step, client ids mapped to handles, validated before commit and rolled back on failure; lands with the batch cap of G49 and the unknown-handle errors of G32 | [Agent integration](tech/agent-integration.md) |
| N11 | Groups first, then reusable symbols as blocks. Groups: named or unnamed collections of independent objects managed by GROUP/UNGROUP, with whole-group selection by default and collective edits; an object can belong to multiple groups. Blocks: a shared named definition with geometry and a placement base point (for example, Chair); INSERT creates references with individual position, rotation and scale. Definition edits update all references in that drawing; support instance attributes and reuse across drawings through block libraries (G7, G61, G2) | [Document model](tech/document-model.md), [Drawing and editing commands](features/drawing-and-editing.md), [File I/O](tech/file-io.md) |
| N12 | Entity tags (key/value metadata) journalled with the entity, a tag filter in `list_entities`, export through XDATA | [Document model](tech/document-model.md), [Agent integration](tech/agent-integration.md) |
| N13 | Structured edits by handle: `update_entity` for geometry, layer and colour, polyline vertex insert, remove and move; per-entity lineweight and linetype (G64) | [Agent integration](tech/agent-integration.md), [Document model](tech/document-model.md) |
| N14 | Handle-based TRIM, EXTEND, OFFSET, FILLET and CHAMFER tools returning created, replaced and deleted handles; needs the commands factored into functions | [Agent integration](tech/agent-integration.md), [Drawing and editing commands](features/drawing-and-editing.md) |
| N15 | Dimensions (associative) and boundary-area annotations; HATCH as a separate object filling a bounded area with a pattern. Include diagonal wall hatching such as ANSI31, adjustable pattern angle and scale (line spacing for user-defined patterns), and custom `.pat` definitions. Hatches are associative by default: changing boundary geometry updates the fill (G7, G2) | [Document model](tech/document-model.md), [Drawing and editing commands](features/drawing-and-editing.md), [Rendering](tech/rendering.md) |
| N16 | Clearance and collision checks: region overlap, minimum distance, door swings and clearance envelopes; depends on N11 and N12 (R2) | [Geometry kernel](tech/geometry-kernel.md), [Agent integration](tech/agent-integration.md) |
| N17 | MCP geometry: arcs by centre, radius and angles, polyline bulges, width/height and rounded rectangles, polygons; ellipse and spline need new entities | [Agent integration](tech/agent-integration.md) |
| N18 | Measurements and validation with units: minimum separation, angle, point in region, intersections; open boundaries, duplicates, tiny segments, self-intersections | [Agent integration](tech/agent-integration.md), [Geometry kernel](tech/geometry-kernel.md) |
| N19 | Output: an agent-safe save with one path policy (G50), SVG and PDF export, `render_png` file output, backgrounds, layer filters and named views (G29) | [DXF files](features/dxf-files.md), [Agent integration](tech/agent-integration.md) |
| N20 | Queries and schemas: containment, proximity, tag and group filters, detail levels; a capabilities tool, a uniform response envelope and a drawing revision; later dry runs, checkpoints, idempotency keys, UCS, asset libraries, layer styles, layouts and schedules (suggestions 16 to 24) | [Agent integration](tech/agent-integration.md) |
| N21 | Agent panel: follow the end of the transcript only when the view is already at the bottom, so a user who scrolled up is not pulled down by each streamed delta, debug log line or empty turn end | [Agent panel](features/agent-panel.md) |
| N22 | Reference images: insert a raster image (for example, a photo or scan of a blueprint) into the drawing, position, scale and rotate it for tracing against CAD geometry; define how image data or references persist when saving and reopening drawings | [Drawing canvas](features/canvas.md), [Document model](tech/document-model.md), [File I/O](tech/file-io.md), [Rendering](tech/rendering.md) |
| N23 | Reorder layers in the layers panel and status-bar layer popover, preserving the chosen order when saving and reopening; settle whether this controls only the layer-list order or also the drawing stacking order | [Drawing canvas](features/canvas.md), [Document model](tech/document-model.md), [File I/O](tech/file-io.md), [Rendering](tech/rendering.md) |
| N24 | Compact canvas HUDs: reduce the footprint of both the zoom controls and the properties HUD, with semitransparent backgrounds on both. Make the properties HUD draggable and able to snap to any of the four canvas corners with a small inset margin | [Drawing canvas](features/canvas.md), [UI framework](tech/ui-framework.md) |
