---
id: tech-testing
title: Testing and performance
kind: tech
status: current
summary: What each level of test covers and where it lives, how UI behaviour is tested without a display, and the performance targets.
read_when: you are adding a feature or fixing a bug and need to know which test to write and where
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [geom/geom_test.go, doc/doc_test.go, cmdsys/commands/commands_test.go, cmdsys/commands/inflight_test.go, fileio/dxf/dxf_test.go, ui/workspace_test.go, ui/ruler_test.go, ui/hud_test.go, ui/layerpop_test.go, ui/cmdcomplete_test.go, ui/settings_tabs_test.go, ui/agent_review_test.go, ui/agent_review_internal_test.go, tools/gui/main.go, tools/gui/gui_test.go, agentmanager/store/store_test.go, cmdsys/panic_test.go, cmdsys/commands/nonfinite_test.go, cmdsys/commands/i18n_test.go, render/view_test.go, fileio/format_test.go, cadmcp/safety_test.go, ui/agent_quit_test.go, fileio/dwg/read_test.go, fileio/dwg/fuzz_test.go, spatial/bench_test.go]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# Testing and performance

## What it is

Every layer is tested without a window, a display or a GPU, and one level runs the real window on a
virtual display. The core layers are plain Go; the UI is driven through gpui's headless platform,
which dispatches real input events and rasterizes real frames. Tests live beside the package they test, in the external `_test` package unless they need
unexported access.

## Levels

| Level | What | Where and how |
|---|---|---|
| Unit | Geometry: transforms, arc bounds, closest points, bulges, chord tolerance, intersections | `geom/geom_test.go`, table-driven |
| Unit | Areas, offsets, fillets and chamfers, trim spans, splitting, joining and extending | `geom/area_test.go`, `geom/offset_test.go`, `geom/fillet_test.go`, `geom/split_test.go` |
| Unit | Transactions, abort, change listeners, undo and redo | `doc/doc_test.go`; a random-edit test undoes 200 transactions and compares with the original, then redoes them and compares with the result |
| Command | Every built-in command end to end | `cmdsys/commands/commands_test.go`: scripts fed to a headless `Session`, then the document inspected |
| Command | MIRROR, OFFSET, FILLET, CHAMFER; TRIM, EXTEND, BREAK, JOIN, EXPLODE; the measurement commands; the AI command | `cmdsys/commands/construct_test.go`, `construct_more_test.go`, `trim_test.go`, `measure_test.go`, `agent_test.go` |
| Command | Input parsing, snaps, ortho, window and crossing selection | the same file, calling `ParseInput`, `FindSnap`, `Session.Click` and `Session.SelectBox` |
| Command | Geometry placed during a command stays visible and pickable; commit and panic recovery leave no stale spatial-index entries | `cmdsys/commands/inflight_test.go`, with one undo step for the whole command |
| Round trip | DXF write then read; reading a newer-version file; malformed input; truncated files, a missing `EOF` and non-finite numbers | `fileio/dxf/dxf_test.go` |
| Unit | Atomic save: a failed write keeps the original, a save keeps the mode; a format with only a reader refuses to save and names the writable extension | `fileio/format_test.go` |
| Unit | DWG bit codes (BS, BL, BD, DD, MC, SMC, MS, H, TV, TU, string streams) at every bit offset, CRC-16, CRC-32 and the page checksum, both LZ77 variants, the section map, the object map and the class table | `fileio/dwg/bits_test.go`, `crc_test.go`, `lz_test.go`, `container_test.go` |
| Round trip | DWG fixtures of R14 to R2018: layers, entity counts, exact coordinates, units and extents against ACadSharp's DXF export; DWG to DXF and back; hand-built R2000 and R2004 records for what no fixture holds (2D POLYLINE, mirrored entities, paper space, damaged objects) | `fileio/dwg/read_test.go`, `roundtrip_test.go`, `synth_test.go`, `parity_test.go` |
| Robustness | Truncated and bit-flipped DWG files, damaged decoded object data, claimed sizes, and `FuzzRead` (the fuzz target for the DWG reader) | `fileio/dwg/robust_test.go`, `errors_test.go`, `fuzz_test.go` |
| Robustness | A panicking command leaves the session usable; typed `nan` and `inf` are rejected and never reach the view; the view ignores non-finite boxes and zooms | `cmdsys/panic_test.go`, `cmdsys/commands/nonfinite_test.go`, `render/view_test.go` |
| Agent safety | Scripts cannot reach NEW, OPEN or SAVE; a panicking tool leaves the drawing usable; quitting during a session start closes the late session; the strict `AllowMCP` match | `cadmcp/safety_test.go`, `ui/agent_quit_test.go`, `agentmanager/queue_test.go`, `agentmanager/acp/mcpof_test.go` |
| Unit | The agent conversation store: JSONL records, the open-tab set per drawing, the model and drawing of a record, copying between projects | `agentmanager/store/store_test.go`, `drawing_test.go`, in the nested module (`just check-agentmanager`) |
| UI | The agent panel's model picker (choice before start, live switch, restart without a setter, refusal, per-harness default) and the tabs of each drawing (OPEN, SAVEAS in and out of the project, untitled first save, a legacy store) | `ui/agent_model_test.go`, `ui/agent_drawing_test.go`, with fake sessions; the backends' model wire is tested in `agentmanager/acp/models_test.go`, `codex/models_test.go` and `claude/claude_test.go` against the fake agent |
| UI | Agent prompt images (a drop on the composer attaches and never opens a drawing, a drop on the canvas still does, paste, chip removal, refusal without vision) and opencode's free models (ordering, the free default, a settings default and *Default* winning over it) | `ui/agent_image_test.go`, `ui/agent_free_test.go`, `ui/agent_free_internal_test.go`; the backends' image wire in `agentmanager/acp/images_test.go`, `claude/images_test.go`, `codex/images_test.go` against the fake agent's image echo |
| UI | Typed commands, clicks, right-click Enter, Space as Enter, Escape, box drag, Ctrl+Z, middle-drag pan, wheel zoom, the selection highlight | `ui/workspace_test.go`: a `ui.Workspace` in a headless window, driven by `HeadlessWindow.Dispatch` |
| UI | Ruler labels and layout reuse, HUD release propagation, layer popover filtering and lifecycle, layer dirty tracking, autocomplete and command recall, settings tabs | `ui/ruler_test.go`, `hud_test.go`, `layerpop_test.go`, `cmdcomplete_test.go`, `settings_tabs_test.go`, with headless input and renders |
| UI | Agent Stop and Cancel, ordered and paused queues, bounded markdown parsing, redraw rate and asynchronous copy | `ui/agent_review_test.go`, `agent_review_internal_test.go`, using fake sessions and headless input |
| Real window | The real ebiten window under Xvfb with Mesa llvmpipe software GL: xdotool sends the input, screenshots are compared, and the drawing is saved on exit (`-save-on-exit`) for DXF assertions such as `expect-count LINE 3` | `tools/gui`, scenarios in `tools/gui/scenarios/` (`smoke`, `line`, `snap`, `panzoom`, `toolbar`, `agent_panel`, `gestures`, `settings`); `just gui-test` runs them all and is gated by `CADLITE_GUI_TEST=1`; `just gui-run` runs chosen ones; scenario language in `tools/gui/README.md` |
| Visual | Whole-window renders | `go run ./cmd/cad -png PATH [-script …] [-hover x,y]`, looked at by a person or an agent |

The real-window level is the only one that runs the platform layer (ebiten, OS events, GPU). It does
not cover HiDPI scales, the clipboard, IME composition or window resizing.

A bug fix gets a regression test that fails before the fix.

`internal/gpui` has its own suite and rules (`internal/gpui/AGENTS.md`).

## Writing a command test

Script the command with AutoCAD script rules — single spaces between inputs, two in a row for Enter —
and inspect the drawing. A command that ends by itself needs no trailing Enter; an extra one repeats
the command. Clicks and boxes take model points, so a test never depends on pixel positions.

## Testing a language

The default test language is English. A test of Italian output sets `i18n.Set(i18n.Italian)`,
restores English in `t.Cleanup`, and sets `i18n.MissingHandler` to fail on a missing key; the helper
is `italian` in `cmdsys/commands/i18n_test.go`, which also covers the Italian aliases, keywords and
prompts, and the English output of an agent caller. `go run ./i18n/cmd/i18ncheck -scan
cmdsys,fileio,settings -strict` reports string literals at user-facing call sites that have no
catalog key and must stay clean.

## Symbol integration tests

`doc/block_test.go` covers metadata undo/redo/abort, overlapping group membership, affine bounds,
nearest points, intersections, depth/expansion budgets and transactional library support. Import undo
preserves unrelated layer edits. `cmdsys/commands/block_test.go` exercises the public command flow,
group selection, hidden/locked members, ALL, attributes, libraries, cancellation and Italian aliases.
`fileio/dxf/symbols_test.go` repeats native R12/R2000 symbol round trips and checks opaque preservation.
`render/block_test.go` verifies inherited colour, child-layer toggles, bounds/index and cache invalidation.

## Performance targets

| Scenario | Target |
|---|---|
| Open a 10 MB DXF (about 100k entities) | First frame in under 2 s |
| Pan and zoom on 100k entities | 60 fps while interacting |
| Full re-render after a zoom settles | Under 100 ms |
| Snap candidate search | Under 2 ms per pointer move |
| Idle, nothing changing | About 0% CPU: no frame is drawn |

gpui draws only invalidated frames. `spatial/bench_test.go`, `BenchmarkQuery`, measures viewport
queries on 10k and 100k entities through the uniform-grid index; the large-drawing file-open and
full-frame targets have no end-to-end benchmark.

## Related docs

- [Operations](operations.md) — the commands that run these tests
- [Command system](command-system.md) — the script rules the command tests rely on
- [Backlog](../backlog.md) — the missing large-drawing benchmarks and corpus tests
