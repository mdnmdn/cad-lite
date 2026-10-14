---
id: tech-testing
title: Testing and performance
kind: tech
status: current
summary: What each level of test covers and where it lives, how UI behaviour is tested without a display, and the performance targets.
read_when: you are adding a feature or fixing a bug and need to know which test to write and where
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [geom/geom_test.go, doc/doc_test.go, cmdsys/commands/commands_test.go, fileio/dxf/dxf_test.go, ui/workspace_test.go]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# Testing and performance

## What it is

Every layer is tested without a window, a display or a GPU. The core layers are plain Go; the UI is
driven through gpui's headless platform, which dispatches real input events and rasterizes real
frames. Tests live beside the package they test, in the external `_test` package unless they need
unexported access.

## Levels

| Level | What | Where and how |
|---|---|---|
| Unit | Geometry: transforms, arc bounds, closest points, bulges, chord tolerance, intersections | `geom/geom_test.go`, table-driven |
| Unit | Transactions, abort, change listeners, undo and redo | `doc/doc_test.go`; a random-edit test undoes 200 transactions and compares with the original, then redoes them and compares with the result |
| Command | Every built-in command end to end | `cmdsys/commands/commands_test.go`: scripts fed to a headless `Session`, then the document inspected |
| Command | Input parsing, snaps, ortho, window and crossing selection | the same file, calling `ParseInput`, `FindSnap`, `Session.Click` and `Session.SelectBox` |
| Round trip | DXF write then read; reading a newer-version file; malformed input | `fileio/dxf/dxf_test.go` |
| UI | Typed commands, clicks, right-click Enter, Space as Enter, Escape, box drag, Ctrl+Z, middle-drag pan, wheel zoom, the selection highlight | `ui/workspace_test.go`: a `ui.Workspace` in a headless window, driven by `HeadlessWindow.Dispatch` |
| Visual | Whole-window renders | `go run ./cmd/cad -png PATH [-script …] [-hover x,y]`, looked at by a person or an agent |

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
