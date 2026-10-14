---
id: tech-architecture
title: Architecture
kind: tech
status: current
summary: The layers of Cad-lite, the dependency rule between them, where each layer lives, and how an edit flows from input to pixels.
read_when: you are adding a package, wondering where code belongs, or tempted to import the UI from the core
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [go.mod, cadmcp/dispatcher.go, cmd/cad/main.go, ui/workspace.go, i18n/i18n.go]
depends_on: [prod-overview]
review_cycle: quarterly
---

# Architecture

## Shape

Cad-lite is one Go module, `cad-lite`, holding a layered CAD core, a UI shell and the UI framework it is
built on. The core is pure Go, headless-testable and independent of any rendering backend; only the
UI shell and one rendering adapter touch the graphics stack.

```
┌─────────────────────────────────────────────────────────────┐
│ UI shell (ui/)    toolbar · canvas · layers · command line   │
├──────────────────────────────┬──────────────────────────────┤
│ Commands (cmdsys/)           │ Renderer (render/)            │
│ prompts · snaps · selection  │ view · display list · builder │
│                              │ render/gpuirender (adapter)   │
├──────────────────────────────┴──────────────────────────────┤
│ Document database (doc/)  entities · layers · handles        │
│                           transactions · undo journal        │
├─────────────────────────────────────────────────────────────┤
│ File I/O plugins (fileio/)   DXF · DWG (read)                 │
├──────────────────────────────┬──────────────────────────────┤
│ Geometry kernel (geom/)      │ Spatial index (spatial/)      │
└──────────────────────────────┴──────────────────────────────┘
```

`i18n` is a leaf beside the core: the English and Italian message catalogs, language detection,
plurals, localized command keywords and command aliases. It imports nothing of the project, and
`cmdsys`, `fileio`, `settings`, `cadmcp` and `ui` may import it.

The UI framework under the shell is `internal/gpui`, with `internal/gpui/ebitenplatform` as its display backend.

`appenv` is a third leaf: it reads the `CADLITE_*` environment variables, falling back to the old
`GODRAW_*` names. `settings`, `updater`, `cmd/cad` and the tests use it.

`updater` is a second stdlib-only leaf: it finds, downloads, verifies and installs GitHub releases and
restarts the application. Only `ui` and `cmd/cad` use it; it imports nothing of the project. See
[Updates](../features/updates.md).

AI agents attach on the side: `cadmcp` exposes the drawing as MCP tools on top of `cmdsys`, and the
nested module `agentmanager` runs the agent CLIs and the MCP server; `ui` hosts the chat panel that
joins them. `agentmanager` imports nothing from Cad-lite. See [Agent integration](agent-integration.md).

`cmd/cad` wires the layers together and chooses the platform: an ebiten window, or a headless
window that renders one frame to PNG. Every package's purpose is listed in the
[code map](code-map.md), which is generated from the package doc comments.

## Rules

1. **Dependencies point down.** A package imports only packages in layers below it. `geom` imports
   nothing of the project; `spatial` and `doc` import `geom`; `fileio` imports `doc`; `render`
   imports `doc`; `cmdsys` imports `doc`, `render` and `spatial`; `ui` imports everything below it. `i18n` imports nothing of the project (it is a leaf, builds for `js/wasm`) and is imported by `cmdsys`, `cmdsys/commands`, `fileio`, `settings`, `ui` and `cmd/cad`. `cadmcp` imports `cmdsys` and `doc`; `ui` imports `cadmcp`; nothing in `agentmanager` imports Cad-lite.
2. **The core never sees the screen.** `geom`, `doc`, `spatial`, `fileio`, `render` and `cmdsys`
   never import `internal/gpui`, ebiten or any graphics package. Only `render/gpuirender`, `ui` and
   `cmd/` import `internal/gpui`, and only `cmd/` imports `internal/gpui/ebitenplatform`.
   `assets/icons` is a data package (embedded SVGs, imports only `embed`) that only `ui` imports,
   together with the SVG rasterizer; see [UI framework](ui-framework.md).
3. **No cgo in the default build.** Everything that builds without tags is pure Go, so the
   application compiles for `js/wasm`; DWG is read by pure Go code (`fileio/dwg`, decision D24). A
   future cgo dependency would live behind a build tag in its own `fileio` plugin.
4. **Model space is `float64`.** No `float32` in `geom`, `doc`, `spatial` or `cmdsys`. Narrowing to
   `float32` happens once, in `render/gpuirender`, after the camera-relative transform.
5. **The document changes only inside a transaction.** Commands never mutate entities directly;
   the one exception is a file reader filling a new drawing with `Drawing.Load`, outside the
   journal. See the [document model](document-model.md).
6. **The selection, the view and the snap settings belong to the session, not the document.** They
   are not saved and not undoable.

## Data flow of an edit

1. Input (keyboard, mouse) reaches `ui.Workspace`, which forwards it to `cmdsys.Session`: typed text
   to `Session.Submit`, clicks to `Session.Click`, pointer moves to `Session.Hover`, box drags to
   `Session.SelectBox`.
2. The session parses the input for the active prompt and calls the active command's `Input`.
3. The command changes the document through the transaction from `Context.Tx`.
4. When the command ends, the session commits the transaction; `doc` records one journal step and
   publishes a `ChangeSet`.
5. Change listeners update the spatial index (`spatial.Index.Apply`), prune the selection and drop
   the render cache entries of the touched handles (`render.Builder.Invalidate`).
6. The workspace refreshes the window; on the next frame the canvas rebuilds the display list from
   the cache and paints it through `render/gpuirender`.

## Rationale

**Why one module with the UI framework inside it.** The CAD application and `internal/gpui` evolve
together: a canvas need (strokes, a canvas element, a command-line hook) lands in the library and its
user in one commit. The cost is that the library is not importable from other modules; the decision
register records it.

**Why the adapter sits under `render/`.** The display list is the seam between "what to draw" and
"how to draw it". Keeping every gpui call in one small package means a different backend (a GPU
renderer with retained buffers, an SVG or PDF exporter) consumes the same display list without
touching the core.

## Related docs

- [Code map](code-map.md) — every package and its purpose
- [UI framework](ui-framework.md) — the boundary with `internal/gpui`
- [Decision register](decisions.md) — why the layers are where they are
