# Project structure

The library lives at `internal/gpui/` inside the `go-draw` module (see the
root `go.mod`); its import path is `go-draw/internal/gpui`. Paths below are
relative to `internal/gpui/`. Re-check with `ls . ebitenplatform examples`
when adding files.

## Tree

```
internal/gpui/           core package (package gpui), library root
  AGENTS.md              rules for AI agents working on the library
  README.md              project overview, quick start
  _docs/                 internal docs (contract, roadmap, structure, index)
  docs/                  user documentation (index: docs/README.md)
  examples/              17 runnable programs, one dir each with main.go
  ebitenplatform/        ebiten backend
  *.go                   the core package itself
(repo root)
  go.mod, go.sum         module go-draw (ebiten, x/image, go-text/typesetting)
  .gitignore             ignores *.png, /bin/, .DS_Store, /_tmp/
  _tmp/                  gitignored scratch (never commit)
```

### `_docs/`

| File | Responsibility |
|------|----------------|
| `index.md` | entry point listing every doc and reading orders |
| `api-contract.md` | normative API contract; edit before changing signatures |
| `roadmap.md` | status, done milestones, open work, limitations, process |
| `project-structure.md` | this file |

### `docs/`

`docs/README.md` is the index of user documentation (architecture, getting
started, examples index, `concepts/*`, plus any reference/guide pages added
later). See [index.md](index.md) for the current list.

### `internal/gpui/` (package `gpui`)

Core types and geometry:

| File | Responsibility |
|------|----------------|
| `geometry.go` | `Pixels`, `Point`, `Size`, `Bounds`-adjacent geometry, edges, corners |
| `bounds.go` | `Hitbox`, bounds helpers, hit testing primitives |
| `color.go` | `Rgba`, `Hsla`, named colours, conversions, Oklab helpers |
| `background.go` | `Fill`/`Background`, linear gradients, colour spaces |
| `shadow.go` | `BoxShadow` |

Style and layout:

| File | Responsibility |
|------|----------------|
| `style.go` | `Style`, `StyleRefinement`, lengths, `TextFields`, overflow, display |
| `styled.go` | `StyledMixin`: Tailwind-like fluent builders shared by `Div`/`Text` |
| `styled_layout.go` | margin, position, wrap/gap/alignment and flex-basis builders/forwarders for `*Div` and `*Text` |
| `layout.go` | flexbox engine: `LayoutNode`, measure, resolve, absolute positioning |
| `layout_wrap.go` | wrapped line formation, per-line sizing and align-content |

Elements:

| File | Responsibility |
|------|----------------|
| `element.go` | `Element` interface, `IntoElement`, per-frame context types |
| `div.go` | `Div` box element, interaction handlers, style states, scroll wiring |
| `text_elem.go` | `Text`, `StyledText`, `TextRun`, `Str` |
| `text.go` | text shaper (HarfBuzz via go-text), glyph cache, wrapping, measurement |
| `font.go` | system font discovery, families, fallback, bundled faces |
| `input.go` | `TextInput` single-line field, rune-safe UTF-16 selection and internal capture (`PlatformInputHandler`) |
| `uniform_list.go` | virtualized equal-height `UniformList` |
| `scroll.go` | `ScrollHandle`, scroll requests, scrollbar geometry, capture and paging |
| `canvas.go` | `Canvas`: leaf element painting through a callback (GPUI `canvas()`) |

Painting:

| File | Responsibility |
|------|----------------|
| `scene.go` | `Scene` primitives (quads, shadows, glyph sprites, underlines), content masks |
| `stroke.go` | `Stroke` polyline primitive, `Window.PaintStroke`, its rasterization |
| `raster.go` | CPU `Rasterizer`: Scene to premultiplied RGBA |

Window, events and application:

| File | Responsibility |
|------|----------------|
| `window.go` | `Window`: draw pipeline (layout, prepaint, paint, raster, present), hit test, focus/hover state |
| `events.go` | input event types, dispatch phases, modifiers |
| `keymap.go` | `Action`, `Keystroke`, `Keymap`, key contexts |
| `focus.go` | `FocusHandle`, `FocusID`, tab stops, focus-visible modality |
| `entity.go` | `Entity[T]`, weak handles, slotmap IDs, release |
| `context.go` | `Context`: per-entity view passed to `Render`/`Update` |
| `app.go` | `App`, `EntityRoot`, `OpenWindow`, update/effect flush |
| `subscription.go` | `Subscription` handles |

Platform:

| File | Responsibility |
|------|----------------|
| `platform.go` | `Platform`, `PlatformWindow`, `PlatformInputHandler` interfaces + stub |
| `headless.go` | `HeadlessPlatform`/`HeadlessWindow`, `RenderToPNG`, `RenderToImage`, `RenderOption`s |
| `clipboard.go` | `ClipboardRead`/`ClipboardWrite` via OS tools with timeouts |

Tests (package `gpui_test` unless noted internal), grouped by area:

| Area | Files |
|------|-------|
| Geometry/bounds/scene/raster | `bounds_test.go`*, `scene_test.go`*, `raster_test.go`*, `paint_order_test.go`, `paint_features_test.go`, `bench_test.go` |
| Layout | `layout_test.go`, `layout_wrap_test.go`, `layout_margin_test.go`, `styled_wrap_test.go` |
| Text/fonts | `text_test.go`*, `text_features_test.go`*, `text_elem_test.go`, `text_cascade_test.go`, `font_test.go`*, `glyph_fix_test.go`* |
| Events/input/keymap/focus | `events_test.go`, `dispatch_test.go`, `hover_test.go`, `active_test.go`, `input_test.go`, `input_selection_test.go`, `keymap_test.go`, `action_key_test.go`, `scroll_focus_test.go`, `scroll_stop_test.go`, `scrollbar_input_test.go` |
| Entities/app | `entity_release_test.go`, `subscription_test.go`, `custom_element_test.go`, `core_test.go`* |
| Headless/clipboard | `headless_test.go`, `clipboard_test.go`, `clipboard_internal_test.go`* |
| Review regressions | `review_fixes_test.go`, `review_fixes_internal_test.go`* |

`*` = internal package `gpui` (needs unexported access).

### `ebitenplatform/`

| File | Responsibility |
|------|----------------|
| `platform.go` | `New()`, `Run`, ebiten `Game` (`Update`/`Draw`/`Layout`), window, present buffer |
| `input.go` | ebiten keys/mouse/wheel/chars to gpui events; modifier mapping |
| `clipboard.go` | clipboard operations applied on the main loop |
| `snapshot.go` | `Option`s, `WithSnapshotKey`, frame PNG snapshot |
| `*_test.go` | `clipboard_test.go`, `input_test.go`, `snapshot_test.go` |

### `examples/`

`counter`, `actions_and_keybinds`, `border_and_shadow_backgrounds`,
`focus_visible`, `gallery`, `gradient`, `hello_world`, `interactive_elements`,
`layout`, `opacity`, `scrollable`, `styling`, `tab_stop`, `text`, `todo`,
`tree`, `uniform_list` - each is `examples/<name>/main.go`. Index with notes:
`docs/examples.md`.

## Dependency boundaries

- `gpui` imports only the standard library, plus `golang.org/x/image` and
  `github.com/go-text/typesetting` (confined to `font.go`/`text.go`).
- Ebiten (`github.com/hajimehoshi/ebiten/v2`) is imported only in
  `ebitenplatform/`.
- `ebitenplatform` imports `gpui`; `gpui` never imports it.
- `examples/*` import `gpui` and (for the interactive path) `ebitenplatform`.
- Tests may import only what the package under test may import.

## In-package dependency layers

Lower layers must not depend on higher ones:

1. geometry, colour, background, shadow (`geometry.go`, `color.go`, `background.go`, `shadow.go`)
2. style (`style.go`, `styled.go`, `styled_layout.go`)
3. layout (`layout.go`)
4. elements (`element.go`, `div.go`, `text_elem.go`, `uniform_list.go`, `input.go`) with fonts/shaper (`font.go`, `text.go`)
5. scene and raster (`scene.go`, `raster.go`, `bounds.go`)
6. window (`window.go`, `events.go`, `focus.go`, `keymap.go`, `scroll.go`)
7. app, context, entity (`app.go`, `context.go`, `entity.go`, `subscription.go`)
8. platform (`platform.go`, `headless.go`, `clipboard.go`) and `ebitenplatform`

In practice `window.go` and `div.go` reference each other (elements receive a
`*Window`); the layering is a guide, not an enforced rule.

## Where to add things

- New element: new file `<name>.go`; per-frame value implementing
  `Element` (`ID/RequestLayout/Prepaint/Paint`) through an adapter type like
  `divElement`; fluent builders via `StyledMixin`; contract section first;
  tests; a `docs/concepts/elements.md` note.
- New style property: field in `style.go` (and the refinement/merge),
  builder in `styled.go`, layout use in `layout.go` or paint use in `div.go`/
  `scene.go`, contract, tests, `docs/concepts/styling.md`.
- New platform: new package `<name>platform/` implementing
  `gpui.Platform`; no imports of ebiten elsewhere; mirror
  `ebitenplatform` tests using the headless platform.
- New example: `examples/<name>/main.go` with flags `-png`, `-width`,
  `-height`, `-smoke`; header comment states what is ported and "Not
  ported" items; add to `docs/examples.md`, `AGENTS.md` list and the
  roadmap port table.
- New doc: user-facing under `docs/` (link from `docs/README.md`); internal
  under `_docs/`; always add a row to [index.md](index.md).

## Naming conventions

- Files: lower snake case, one concept per file; tests as `<file>_test.go` or a
  topic name (`paint_order_test.go`); internal-package tests mention
  `internal` in the file name when practical (`clipboard_internal_test.go`).
- Tests: `TestXxx` in `gpui_test` by default; table-driven where cheap.
- Examples: directory name equals the gpui-ce example name where ported;
  common flags `-png PATH`, `-width`, `-height`, `-smoke MS`, plus
  seeding flags (`-focus`, `-scroll`, `-vscroll`, `-hscroll`, `-oklab`).
- Docs: kebab-case `.md`.

## Scratch and generated output

- `_tmp/` is gitignored scratch: rendered PNGs, a clone of gpui-ce
  (`_tmp/gpui-ce-src`), a throwaway module `_tmp/docchk2/` with
  doc-snippet packages (`go run ./_tmp/<name>` from the repo root, which may import
  `go-draw/internal/gpui`). Never commit it.
- `*.png` is gitignored everywhere (snapshots, `-png` outputs), as is `/bin/`.
