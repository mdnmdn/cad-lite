# go-draw

go-draw is a 2D drafting application in the spirit of AutoCAD, written in Go. A drafter works from
a command line with typed coordinates and object snaps, on drawings in real units with double
precision, organised in layers, and exchanges them as DXF. The UI runs on an internal Go port of
Zed's GPUI, which rasterises on the CPU, so the application needs no cgo.

**Status: spike.** It implements a slice of the specification: the kernel, the document model, the
core drawing and editing commands, snaps and selection, and DXF read and write. The gaps are
listed in [`_docs/backlog.md`](_docs/backlog.md).

## Quick start

```
go run ./cmd/cad                            # desktop window
go run ./cmd/cad -open plan.dxf             # open a DXF file
go run ./cmd/cad -demo                      # draw the demo plate
go run ./cmd/cad -script "LINE 0,0 100,50"  # run a command script
go run ./cmd/cad -png _tmp/cad.png          # render one frame headlessly
GOOS=js GOARCH=wasm go build -o _tmp/cad.wasm ./cmd/cad
```

The desktop window needs a display and a GPU-capable backend. All flags and the test and lint
commands are in [`_docs/tech/operations.md`](_docs/tech/operations.md).

## What works

- Commands LINE, PLINE, CIRCLE, ARC, RECTANG, MOVE, COPY, ROTATE, SCALE, ERASE, UNDO, REDO, ZOOM,
  LAYER, DIST, ID, LIST, NEW, OPEN and SAVE, with AutoCAD's aliases and prompts.
- Typed points (absolute, relative, polar), direct distance entry, ortho, and the endpoint,
  midpoint, centre, quadrant and intersection object snaps.
- Pick, window and crossing selection; every command undoes as one step.
- Pan, zoom, grid, layers panel and status bar.
- DXF read and write, with warnings for what is not supported.

## Repository layout

| Path | Purpose |
|---|---|
| `geom/` | Geometry kernel: vectors, boxes, lines, arcs, circles, polylines and their operations |
| `doc/` | Document database: handles, entities, layers, transactions and undo |
| `spatial/` | Spatial index for culling, picking and snapping |
| `fileio/` | File format plugins; `fileio/dxf` is the DXF codec |
| `render/` | Backend-neutral view, display list and tessellation; `render/gpuirender` draws it with gpui |
| `cmdsys/` | Command session, input parsing, snaps and selection; `cmdsys/commands` holds the commands |
| `agentmanager/` | Nested module: launches opencode, Claude Code and Codex and speaks to them; imports nothing from go-draw, has its own docs |
| `cadmcp/` | The drawing as MCP tools for those agents ([`_docs/tech/agent-integration.md`](_docs/tech/agent-integration.md)) |
| `ui/` | Application shell: toolbar, canvas, panels, command line, status bar |
| `cmd/cad/` | The executable |
| `internal/gpui/` | The UI framework, with its own docs ([`internal/gpui/README.md`](internal/gpui/README.md)) |
| `_docs/` | The documentation library, starting at [`_docs/INDEX.md`](_docs/INDEX.md) |
| `tools/docs/` | Lints and indexes `_docs/` |

Contributors and agents: read [`AGENTS.md`](AGENTS.md).
