# go-gpui documentation

go-gpui is a proof-of-concept port of Zed's GPUI to Go: a retained-state,
immediate-mode-element UI core (`gpui`, stdlib-only apart from `x/image` and
`go-text/typesetting`) plus an ebiten window backend (`gpui/ebitenplatform`).
Everything is drawn by a CPU rasterizer, so the same tree renders to a window
or to a PNG with no GPU.

These docs describe what the code in `gpui/` does today. The internal API
contract is [`_docs/api-contract.md`](../_docs/api-contract.md) and the
internal docs index is [`_docs/index.md`](../_docs/index.md); they are
intentionally separate from these user docs: where the two disagree, these
pages follow the code and call the difference out.

## Start here

- [../README.md](../README.md) - project overview, quick start, what is and is
  not supported.
- [getting-started.md](getting-started.md) - tutorial: install, first window,
  state and click handlers, entities, keybindings, `TextInput`, PNG rendering,
  headless tests.
- [architecture.md](architecture.md) - package layout, dependency rules, the
  frame pipeline, scene order and content masks, element-state lifecycle,
  effect flush and `Update` nesting, paint-time text overrides, threading
  model, performance notes.

## Concepts

How each subsystem works and why.

- [concepts/app-and-entities.md](concepts/app-and-entities.md) - `App`,
  `Entity`, `Context`, weak handles, subscriptions, the re-entrancy rule.
- [concepts/elements.md](concepts/elements.md) - the `Element` interface,
  `Div`, `Text`, `StyledText`, writing a custom element.
- [concepts/layout.md](concepts/layout.md) - the flexbox engine as
  implemented, and its limits.
- [concepts/styling.md](concepts/styling.md) - `Style`, `StyledMixin`,
  colours, borders, radii, gradients, shadows, opacity, hover/focus/active styles,
  margins and flex shorthands, text style inheritance, fonts.
- [concepts/events-and-input.md](concepts/events-and-input.md) - mouse and
  keyboard dispatch, keymap, actions, focus, `TextInput`, clipboard.
- [concepts/text.md](concepts/text.md) - font discovery, shaping, wrapping,
  glyph masks.
- [concepts/platform.md](concepts/platform.md) - `Platform` /
  `PlatformWindow`, the ebiten loop, headless rendering, writing a backend.

## Guides

Task-oriented pages.

- [guides/cookbook.md](guides/cookbook.md) - compiling recipes: button with
  hover, toggle and checkbox, list with selection, scrollable panel,
  virtualized list, modal overlay, form with `TextInput`, keyboard shortcuts
  and key contexts, tab navigation, card with shadow and gradient, custom
  element with children.
- [guides/testing.md](guides/testing.md) - headless tests, synthetic mouse
  and key events, pixel assertions, `RenderToPNG` snapshots and golden images,
  benchmarks, `-race`.
- [guides/porting-from-gpui.md](guides/porting-from-gpui.md) - Rust GPUI to
  Go mapping table and the known differences.
- [guides/troubleshooting.md](guides/troubleshooting.md) - no text, blurry
  text, slow frames, key bindings not firing, focus, display errors, entity
  release.
- [guides/contributing.md](guides/contributing.md) - contract-first workflow,
  verification commands, test and example conventions, commits, where things
  live.

## Reference

Exported API grouped by area; see [reference/README.md](reference/README.md)
for the index.

- [reference/geometry-and-color.md](reference/geometry-and-color.md) -
  `Pixels`, `Point`, `Size`, `Bounds`, `Edges`, `Corners`, `Length`, colours,
  gradients, shadows.
- [reference/style-builders.md](reference/style-builders.md) - every fluent
  builder on `Div`, `Text`, `StyledMixin` and `UniformList`.
- [reference/elements.md](reference/elements.md) - `Div`, `Text`,
  `StyledText`, `Str`, `TextInput`, `UniformList`, `ScrollHandle`, custom
  elements.
- [reference/events.md](reference/events.md) - event types, listeners,
  `Keystroke`, `KeyBinding`, `Keymap`, `Action`, focus.
- [reference/app-window.md](reference/app-window.md) - `App`, `Entity`,
  `WeakEntity`, `Context`, `Subscription`, `Window`.
- [reference/platform.md](reference/platform.md) - `Platform`,
  `HeadlessPlatform`, `RenderToPNG`, scene, `ebitenplatform`, clipboard.
- [reference/text-and-fonts.md](reference/text-and-fonts.md) -
  `FontManager`, `Shaper`, `TextStyle`, metrics.

## Examples

- [examples.md](examples.md) - index of the 17 programs under `examples/`:
  what each shows, which gpui-ce example it was ported from, what was left
  out. Run any of them with `go run ./examples/<name>`; every one accepts
  `-png PATH` to render a frame without a window.

## Suggested reading order

1. [../README.md](../README.md), then [getting-started.md](getting-started.md).
2. Run and skim a few [examples](examples.md) (`counter`, `todo`, `styling`).
3. [concepts/elements.md](concepts/elements.md),
   [concepts/styling.md](concepts/styling.md) and
   [concepts/layout.md](concepts/layout.md) to build UIs; keep the
   [style builder reference](reference/style-builders.md) open.
4. [concepts/events-and-input.md](concepts/events-and-input.md) and
   [concepts/app-and-entities.md](concepts/app-and-entities.md) for
   interaction and state.
5. [guides/cookbook.md](guides/cookbook.md) for working patterns, and
   [guides/testing.md](guides/testing.md) to test them headlessly.
6. Coming from Rust GPUI: [guides/porting-from-gpui.md](guides/porting-from-gpui.md)
   first, then the concepts.
7. Something not working: [guides/troubleshooting.md](guides/troubleshooting.md).
8. [architecture.md](architecture.md), [concepts/text.md](concepts/text.md)
   and [concepts/platform.md](concepts/platform.md) for internals and
   backends.
9. Contributing: [guides/contributing.md](guides/contributing.md), then
   [`_docs/index.md`](../_docs/index.md).

## Conventions used in these pages

- File references such as `gpui/window.go` are relative to the repository
  root.
- Geometry is `float32` (`gpui.Pixels`) in logical pixels. Device pixels only
  appear at the raster boundary.
- Code snippets are checked to compile against the current API. Fences marked
  `text` are listings or fragments, not compilable code.
