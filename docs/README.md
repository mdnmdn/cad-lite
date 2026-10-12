# go-gpui documentation

go-gpui is a proof-of-concept port of Zed's GPUI to Go: a retained-state,
immediate-mode-element UI core (`gpui`, stdlib-only apart from `x/image` and
`go-text/typesetting`) plus an ebiten window backend (`gpui/ebitenplatform`).
Everything is drawn by a CPU rasterizer, so the same tree renders to a window
or to a PNG with no GPU.

These docs describe what the code in `gpui/` does today. The internal API
contract and plan live in `_docs/` and are intentionally separate: where the
two disagree, these pages follow the code and call the difference out.

## Contents

- [architecture.md](architecture.md) - package layout, dependency rules, the
  frame pipeline, threading model, performance notes.
- Concepts
  - [concepts/app-and-entities.md](concepts/app-and-entities.md) - `App`,
    `Entity`, `Context`, weak handles, subscriptions, the re-entrancy rule.
  - [concepts/elements.md](concepts/elements.md) - the `Element` interface,
    `Div`, `Text`, `StyledText`, writing a custom element.
  - [concepts/layout.md](concepts/layout.md) - the flexbox engine as
    implemented, and its limits.
  - [concepts/styling.md](concepts/styling.md) - `Style`, `StyledMixin`,
    colours, borders, radii, text styles, hover.
  - [concepts/events-and-input.md](concepts/events-and-input.md) - mouse and
    keyboard dispatch, keymap, actions, focus, `TextInput`, clipboard.
  - [concepts/text.md](concepts/text.md) - font discovery, shaping, wrapping,
    glyph masks.
  - [concepts/platform.md](concepts/platform.md) - `Platform` /
    `PlatformWindow`, the ebiten loop, headless rendering, writing a backend.

Getting started, the examples index and the repository README are written
separately.

## Conventions used in these pages

- File references such as `gpui/window.go` are relative to the repository
  root.
- Geometry is `float32` (`gpui.Pixels`) in logical pixels. Device pixels only
  appear at the raster boundary.
- Code snippets are checked to compile against the current API.
