# AGENTS.md

Guidance for AI agents working in this repo.

## What this is

POC port of Zed's GPUI (Rust) to Go. The UI core lives in package `gpui`
(stdlib-only); the GPU/display backend lives in `gpui/ebitenplatform`
(ebiten, no cgo).

Start at `_docs/index.md`. Contract: `_docs/api-contract.md`. Status, open work
and process: `_docs/roadmap.md`. Repo map and where to add things:
`_docs/project-structure.md`.

## Rules

1. **Contract first.** If a signature must change, edit
   `_docs/api-contract.md` before the code.
2. **Dependency boundaries.** `gpui` imports only stdlib (+ `x/image` and
   `go-text/typesetting` in `font.go`/`text.go`). Ebiten only in
   `gpui/ebitenplatform/`.
3. **Style.** No comments unless non-obvious; every exported identifier gets a
   doc comment. `float32` geometry; `int` only at raster boundaries.
4. **Verification before done.** `gofmt -l .` clean, `go vet ./...`,
   `go test ./...` pass. Run `go mod tidy` after adding imports.
5. **Tests** live in `gpui/*_test.go`, package `gpui_test` unless internal
   access is required.
6. **Element pattern.** Elements are per-frame values implementing `Element`
   (`ID/RequestLayout/Prepaint/Paint`). Because Go has no overloading, fluent
   builders (`ID(string) *Div`) coexist with the `Element` interface
   (`ID() (ElementID, bool)`) through an adapter type (`divElement`). Follow
   the same pattern for new elements.
7. **Entity safety.** Never hold a `*T` from an entity across a call that can
   re-enter the same entity — upgrade the weak handle at invoke time
   (package function `gpui.Listener(ctx, f)`).

## Layout

- `gpui/` — core: geometry, color, style, layout, elements (div, text,
  uniform list, text input), background/shadow, scroll, scene, raster, events,
  keymap, focus, entity/context/app, window pipeline, font/text shaper,
  platform interfaces + stub, headless platform, clipboard.
- `gpui/ebitenplatform/` — ebiten backend implementing `gpui.Platform`.
- `examples/` — 17 runnable apps, each `examples/<name>/main.go` with `-png
  PATH` headless rendering: actions_and_keybinds, border_and_shadow_backgrounds,
  counter, focus_visible, gallery, gradient, hello_world, interactive_elements,
  layout, opacity, scrollable, styling, tab_stop, text, todo, tree,
  uniform_list. Index: `docs/examples.md`.
- `docs/` — user documentation (getting started, examples, architecture,
  concepts); describes what the code does, so update it with behaviour changes.
  Snippets must compile.
- `_docs/` — `index.md` (entry point), `api-contract.md`, `roadmap.md`
  (status, done, open work), `project-structure.md`.
- `_tmp/` — gitignored scratch (e.g. `_tmp/docchk2/`, a throwaway module with
  `replace go-gpui => ../..` for compiling doc snippets). Never commit it.
