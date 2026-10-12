# AGENTS.md

Guidance for AI agents working in this repo.

## What this is

POC port of Zed's GPUI (Rust) to Go. The UI core lives in package `gpui`
(stdlib-only); the GPU/display backend lives in `gpui/ebitenplatform`
(ebiten, no cgo). Contract: `_docs/00-api-contract.md`. Plan: `_docs/plan.md`.

## Rules

1. **Contract first.** If a signature must change, edit
   `_docs/00-api-contract.md` before the code.
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
   (`Context.Listener`).

## Layout

- `gpui/` — core: geometry, color, style, layout, elements (div, text),
  scene, raster, events, keymap, focus, entity/context/app, window pipeline,
  font/text shaper, platform interfaces + stub.
- `gpui/ebitenplatform/` — ebiten backend implementing `gpui.Platform`.
- `examples/todo/` — demo app; supports `-png PATH` headless rendering.
- `_docs/` — contract and plan.
