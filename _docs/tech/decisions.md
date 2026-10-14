---
id: tech-decisions
title: Decision register
kind: tech
status: current
summary: Append-only register of structural choices — what was chosen, why, and what it costs.
read_when: you are about to reverse a structural choice, or wonder why something is the way it is
updated: 2026-10-14
verified: 2026-10-14
depends_on: [tech-architecture]
review_cycle: quarterly
---

# Decision register

One row per structural choice a reasonable person might later reverse. A row may name what it
replaced, because the rationale stays true after the replaced thing is gone. Open decisions are not
here; they are in the [backlog](../backlog.md). Rows are never edited away: a reversal is a new row
that cites the old one.

| id | date | decision | why | cost |
|---|---|---|---|---|
| D1 | 2026-10-14 | The UI shell is built on `internal/gpui`, not Dear ImGui through cimgui-go as the specification proposed | No cgo, so the application builds for `js/wasm`; headless rendering, so UI behaviour is tested and screenshotted without a display; the library is in the repository, so the canvas gets the primitives it needs | Widgets are built by hand; the whole window is rasterized on the CPU each frame; no native look |
| D2 | 2026-10-14 | `internal/gpui` lives inside the `go-draw` module, with its own docs and `AGENTS.md` | A canvas need and the library change that serves it land in one commit, under one `go.mod` | The library cannot be imported from another module; doc snippets are checked inside the module, under `_tmp/` |
| D3 | 2026-10-14 | The renderer draws through the gpui scene (`render/gpuirender`), not through a separate ebiten adapter painting offscreen images (the specification's `render/ebitenrender`) | One drawing path for the UI and the canvas, identical headless and in a window, and no direct ebiten use outside the platform package | No base-layer cache: every frame re-rasterizes the drawing, which bounds the drawing size the spike handles smoothly |
| D4 | 2026-10-14 | The file I/O layer is the package `fileio`, not `io` | `io` would shadow the standard library package in every importer | The path differs from the specification's layout |
| D5 | 2026-10-14 | Entities and tables live in package `doc`, not in `doc/entities` and `doc/tables` subpackages | Entities need `EntityCommon` and the drawing needs entities; one package has no import cycle to break | A larger package as entity types are added |
| D6 | 2026-10-14 | The spatial index is a linear scan behind the API an R-tree serves | The spike's drawings are small; callers do not change when the R-tree lands | Query time grows linearly with the drawing |
| D7 | 2026-10-14 | The DXF writer emits R12 (`AC1009`) | No object dictionary, classes or owner chains to get right; every CAD application reads it | Newer data cannot be written; `LWPOLYLINE` becomes `POLYLINE`; the round trip is lossless only for what the document models |
| D8 | 2026-10-14 | Box selection is press-drag-release | It works the same with a mouse, a trackpad and in tests | AutoCAD users expect click-click windows as well |
| D9 | 2026-10-14 | Space submits the command line except at prompts that take text; it is handled in the input's change callback, not as a key binding | A binding would consume every space, including those typed into layer names and paths | A space typed at a text prompt cannot submit it; Enter does |
| D10 | 2026-10-14 | `View` has no rotation | Every spike feature works in an unrotated view, and the camera stays a centre and a scale | A rotated view, as the specification's `View.Rotation` allows, needs the transform threaded through picking and the adapter |
| D11 | 2026-10-14 | Every command-line submission clears the whole render cache | Layer colour edits are not journalled, so no change set reports them | Re-tessellating everything after each submission; harmless at spike sizes, wasteful on large drawings |
