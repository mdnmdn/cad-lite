# AGENTS.md

Guidance for AI agents working on the `internal/gpui` library.

This library is the UI framework under the Cad-lite CAD application (repo root
`AGENTS.md`, docs at the root `_docs/INDEX.md`). It keeps its own docs and
rules: everything below applies to changes inside `internal/gpui/`. All paths
here are relative to `internal/gpui/`, and the commands run from this
directory (`go test ./...` covers the library and its examples). The CAD
application imports it as `cad-lite/internal/gpui`.

## What this is

POC port of Zed's GPUI (Rust) to Go. The UI core lives in package `gpui`
(stdlib + `x/image` + `go-text/typesetting`, no cgo) and renders with a CPU
rasterizer; the display backend lives in `ebitenplatform` (ebiten).

Start at `_docs/index.md`. Contract: `_docs/api-contract.md`. Status, open work
and process: `_docs/roadmap.md`. Repo map and where to add things:
`_docs/project-structure.md`. User docs: `docs/README.md`.

## Current status (2026-10-12)

- **Works:**
  - Flexbox layout with wrapping and absolute positioning, including margins (auto margins too), gaps, min/max and grow/shrink/basis.
  - Elements: `Div`, `Text`, `StyledText`, `TextInput` with selection and Shift navigation, the virtualized `UniformList`, scroll containers with thumb dragging and track paging, `Canvas` with `Stroke` polylines.
  - Styling: borders, rounded corners and clips, gradients, box shadows, opacity, hover/focus/focus-visible/active styles.
  - Text: style inheritance, font family lookup with system fallback, HarfBuzz shaping and wrapping.
  - Entities with observe, emit and release; keymap, actions, key contexts, tab order.
  - Headless platform and PNG output.
  - Ebiten backend, with a snapshot key (`cmd/ctrl-shift-s`).
- **Examples:** 17 under `examples/`. 14 are ports of gpui-ce examples; each file's header comment says what was not ported.
- **Missing:** animation, images/SVG, grid, colour emoji, IME, multi-window, drag and drop. Priorities are in `_docs/roadmap.md` §3.
- **Performance:** a full todo frame at scale 2 takes about 1.2 ms (`go test . -run X -bench Draw`). Treat a clear regression as a bug.

## Rules

1. **Contract first.** If a signature must change, edit
   `_docs/api-contract.md` before the code.
2. **Dependency boundaries.** `gpui` imports only stdlib (+ `x/image` and
   `go-text/typesetting` in `font.go`/`text.go`). Ebiten only in
   `ebitenplatform/`.
3. **Style.** No comments unless non-obvious; every exported identifier gets a
   doc comment. `float32` geometry; `int` only at raster boundaries.
4. **Verification before done.** `gofmt -l .` clean, `go vet ./...`,
   `go test ./...` pass. Run `go mod tidy` after adding imports.
5. **Tests** live in `*_test.go`, package `gpui_test` unless internal
   access is required. Bug fixes get a regression test that fails first.
6. **Element pattern.** Elements are per-frame values implementing `Element`
   (`ID/RequestLayout/Prepaint/Paint`). Because Go has no overloading, fluent
   builders (`ID(string) *Div`) coexist with the `Element` interface
   (`ID() (ElementID, bool)`) through an adapter type (`divElement`). Follow
   the same pattern for new elements.
7. **Entity safety.** Never hold a `*T` from an entity across a call that can
   re-enter the same entity — upgrade the weak handle at invoke time
   (package function `gpui.Listener(ctx, f)`).
8. **Docs ride with the change.** Behaviour or API changes update `docs/`
   (concepts + `docs/reference/`, which lists every export) and
   `_docs/roadmap.md` in the same change. Doc snippets must compile (check in a
   package under the repo root's `_tmp/`, e.g. `go vet ./_tmp/docchk/...`; the
   library is `internal`, so no separate module).
9. **Rendering changes are checked visually.** Render affected examples with
   `-png` before and after (into `_tmp/`), compare, and look at the PNGs.
   Existing renders should stay pixel-identical unless the change is meant to
   alter them; say which ones changed and why.

## Working in this environment

- **No real window.** The agent shell has no GPU/window-server access: ebiten
  fails with `NSGL: failed to find a suitable pixel format` or
  `MTLCreateSystemDefaultDevice returned 0`. Don't try to fix this. Verify
  with headless tests (`HeadlessPlatform`, `HeadlessWindow.Dispatch`) and
  `-png` renders. Only the user can check interactive behaviour, so say what was
  not run.
- **Parallel agents.** Several agents may edit the repo at once. Stay inside the
  files your brief gives you. If a fix belongs in someone else's file, stop
  and report it instead of editing. A transient build break caused by another
  agent's in-flight edit usually clears in a minute or two, so retry.
- **Scratch.** External sources (e.g. gpui-ce clones), experiment modules and
  before/after PNGs go in `_tmp/` (gitignored; skipped by `./...`). Never inside
  tracked dirs.
- **Commits.** Small, descriptive messages, no AI attribution trailer. Don't
  commit unless asked; the coordinating agent commits after verification.

## Gotchas (behaviour that surprises)

- **Text style and scrolling:**
  - Text style cascades from `Div` text builders. Hover/focus text refinements reach descendants at paint time, but only colour and strikethrough.
  - Scroll offsets are positive distances (GPUI uses negative). Programmatic scrolling only moves the container's scroll axes.
- **Flex:** `Flex1()` is `flex: 1 1 0%` (equal split). Use `FlexAuto()`/`FlexGrow(1)` for content-based growth. `Flex()` is column-direction; use `FlexRow`/`FlexCol` explicitly.
- **Keys and actions:**
  - A bare-key binding (`"space"`, `"a"`) beats typing into a `TextInput` when a handler consumes it. Scope such bindings with `"!textbox"`.
  - `NewAction("x")` values are keyed by name (`ActionKeyOf`).
- **Entities:**
  - `Release()` during an effect flush only schedules.
  - `DispatchEvent` runs inside `App.Update`, and observers fire when the outermost `Update` returns.
  - Only `NewEntity`/`Clone` handles are counted; never `Release` a plain copy.
- **Element state** is pruned when not accessed during a frame.
- **Pointer behaviour:** mouse-down focuses the innermost `TrackFocus` element unless a handler already changed focus. Hitboxes are clipped to rectangles, not rounded corners.
- **`Text` builders:** `Text.Bg`/`Border` exist, but `Text` paints no box. Wrap the text in a `Div`.

## Layout

- `internal/gpui/` (this directory): the core. Geometry, colour, style, layout, elements (div, text, uniform list, text input), background/shadow, scroll, scene, raster, events, keymap, focus, entity/context/app, window pipeline, font/text shaper, platform interfaces plus a stub, headless platform, clipboard, and `bench_test.go`.
- `ebitenplatform/`: the ebiten backend implementing `gpui.Platform`, with input mapping, clipboard on a goroutine, and the snapshot key.
- `examples/`: 17 runnable apps, each `examples/<name>/main.go`, listed below. Index: `docs/examples.md`.
  - **Original:** counter, gallery, todo.
  - **gpui-ce ports:** actions_and_keybinds, border_and_shadow_backgrounds, focus_visible, gradient, hello_world, interactive_elements, layout, opacity, scrollable, styling, tab_stop, text, tree, uniform_list.
  - **Conventions:**
    - Flags `-png PATH -width -height -smoke MS`, plus per-example seeding flags.
    - PNG mode seeds a representative state.
    - The header doc comment names the gpui-ce source and lists `Not ported:` items.
- `docs/`: user documentation. Getting started, architecture, `concepts/`, `guides/` (testing, cookbook, porting from GPUI, troubleshooting, contributing), `reference/`, and the examples index.
- `_docs/`: `index.md` (entry point to all docs), `api-contract.md`, `roadmap.md` (status, done with commit hashes, prioritized open work), `project-structure.md`.
- `_tmp/`: gitignored scratch.
