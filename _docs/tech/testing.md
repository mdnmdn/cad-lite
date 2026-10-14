---
id: tech-testing
title: Testing and performance
kind: tech
status: current
summary: What each level of test covers and where it lives, how UI behaviour is tested without a display, and the performance targets.
read_when: you are adding a feature or fixing a bug and need to know which test to write and where
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [geom/geom_test.go, doc/doc_test.go, cmdsys/commands/commands_test.go, fileio/dxf/dxf_test.go, ui/workspace_test.go, tools/gui/main.go, tools/gui/gui_test.go, agentmanager/store/store_test.go, cmdsys/panic_test.go, cmdsys/commands/nonfinite_test.go, render/view_test.go, fileio/format_test.go, cadmcp/safety_test.go, ui/agent_quit_test.go]
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
| Command | MIRROR, OFFSET, FILLET, CHAMFER; TRIM, EXTEND, BREAK, JOIN, EXPLODE; the measurement commands; the AI command | `cmdsys/commands/construct_test.go`, `trim_test.go`, `measure_test.go`, `agent_test.go` |
| Command | Input parsing, snaps, ortho, window and crossing selection | the same file, calling `ParseInput`, `FindSnap`, `Session.Click` and `Session.SelectBox` |
| Round trip | DXF write then read; reading a newer-version file; malformed input; truncated files, a missing `EOF` and non-finite numbers | `fileio/dxf/dxf_test.go` |
| Unit | Atomic save: a failed write keeps the original, a save keeps the mode | `fileio/format_test.go` |
| Robustness | A panicking command leaves the session usable; typed `nan` and `inf` are rejected and never reach the view; the view ignores non-finite boxes and zooms | `cmdsys/panic_test.go`, `cmdsys/commands/nonfinite_test.go`, `render/view_test.go` |
| Agent safety | Scripts cannot reach NEW, OPEN or SAVE; a panicking tool leaves the drawing usable; quitting during a session start closes the late session; the strict `AllowMCP` match | `cadmcp/safety_test.go`, `ui/agent_quit_test.go`, `agentmanager/queue_test.go`, `agentmanager/acp/mcpof_test.go` |
| Unit | The agent conversation store: JSONL records, the open-tab set, per-project directories | `agentmanager/store/store_test.go`, in the nested module (`just check-agentmanager`) |
| UI | Typed commands, clicks, right-click Enter, Space as Enter, Escape, box drag, Ctrl+Z, middle-drag pan, wheel zoom, the selection highlight | `ui/workspace_test.go`: a `ui.Workspace` in a headless window, driven by `HeadlessWindow.Dispatch` |
| Real window | The real ebiten window under Xvfb with Mesa llvmpipe software GL: xdotool sends the input, screenshots are compared, and the drawing is saved on exit (`-save-on-exit`) for DXF assertions such as `expect-count LINE 3` | `tools/gui`, scenarios in `tools/gui/scenarios/` (`smoke`, `line`, `snap`, `panzoom`, `toolbar`, `agent_panel`, `gestures`, `settings`); `just gui-test` runs them all and is gated by `GODRAW_GUI_TEST=1`; `just gui-run` runs chosen ones; scenario language in `tools/gui/README.md` |
| Visual | Whole-window renders | `go run ./cmd/cad -png PATH [-script …] [-hover x,y]`, looked at by a person or an agent |

The real-window level is the only one that runs the platform layer (ebiten, OS events, GPU). It does
not cover HiDPI scales, the clipboard, IME composition or window resizing.

A bug fix gets a regression test that fails before the fix.

`internal/gpui` has its own suite and rules (`internal/gpui/AGENTS.md`).

## Writing a command test

Script the command with AutoCAD script rules — single spaces between inputs, two in a row for Enter —
and inspect the drawing. A command that ends by itself needs no trailing Enter; an extra one repeats
the command. Clicks and boxes take model points, so a test never depends on pixel positions.

## Performance targets

| Scenario | Target |
|---|---|
| Open a 10 MB DXF (about 100k entities) | First frame in under 2 s |
| Pan and zoom on 100k entities | 60 fps while interacting |
| Full re-render after a zoom settles | Under 100 ms |
| Snap candidate search | Under 2 ms per pointer move |
| Idle, nothing changing | About 0% CPU: no frame is drawn |

The spike meets the idle target (gpui draws only invalidated frames). The large-drawing targets need
the R-tree and base-layer caching listed in the backlog; there is no benchmark for them yet.

## Related docs

- [Operations](operations.md) — the commands that run these tests
- [Command system](command-system.md) — the script rules the command tests rely on
- [Backlog](../backlog.md) — the missing large-drawing benchmarks and corpus tests
