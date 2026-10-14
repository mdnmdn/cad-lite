# API reference

The exported API of package `gpui` (and `ebitenplatform`), grouped by
area. The lists were generated from `go doc -all ./gpui` and checked against the
code, then annotated by hand; for a signature or doc comment that is not here,
`go doc cad-lite/internal/gpui <Name>` is authoritative. The normative design contract is
[`_docs/api-contract.md`](../../_docs/api-contract.md); where it and the code
disagree these pages follow the code.

| Page | Covers |
|---|---|
| [geometry-and-color.md](geometry-and-color.md) | `Pixels`, `Point`, `Size`, `Bounds`, `Edges`, `Corners`, `Axes`; `Length` (`Auto`, `Px`, `Flex`) and `Available`; layout enums; `Rgba`, `Hsla`, palette; `Fill`, `Background`, `LinearGradient`, `ColorSpace`; `BoxShadow`. |
| [style-builders.md](style-builders.md) | Every fluent builder on `*Div`, `*Text`, `StyledMixin` and `*UniformList` with signature and meaning, grouped by layout, sizing, spacing, flex, overflow/scroll, background/border/radius/shadow/opacity, text and interaction styles; `Style` and `StyleRefinement`. |
| [elements.md](elements.md) | `Element`, `IntoElement`, `AnyElement`, ids; `Div`, `Text`, `StyledText`, `Str`, `TextInput`, `UniformList`, `ScrollHandle`, `Canvas`; custom elements, `LayoutNode` and `Layout`. |
| [events.md](events.md) | Every input event type and its fields, listener builders, hit testing types; `Keystroke`, `KeyBinding`, `Keymap`, `Action`, `ActionKey`; `FocusHandle` and the window focus API; key dispatch order. |
| [app-window.md](app-window.md) | `App`, `Entity`, `WeakEntity`, `Context`, `Listener`/`Emit`/`Subscribe`, `Subscription`, `SubscriberSet`, `Task`, `WindowOptions`, and every `Window` method. |
| [platform.md](platform.md) | `Platform` / `PlatformWindow` / `PlatformInputHandler`; `HeadlessPlatform` and `HeadlessWindow`; `RenderToPNG` / `RenderToImage` and `RenderOption`s; `Scene`, primitives and `Rasterizer`; `ebitenplatform.New` and options; clipboard. |
| [text-and-fonts.md](text-and-fonts.md) | `TextStyle`, `TextRun`, `FontManager`, `Font`, `Shaper`, `TextMeasurer`, shaped line types, `GlyphMask`, font discovery. |

## Finding a name

| If you are looking for | Page |
|---|---|
| `NewDiv`, `NewText`, `NewStyledText`, `NewTextInput`, `NewUniformList`, `NewScrollHandle`, `NewCanvas`, `Str` | [elements](elements.md) |
| `Bg`, `P`, `Gap`, `Flex1`, `Rounded`, `Shadow*`, `Hover`, `TextColor` | [style builders](style-builders.md) |
| `OnClick`, `OnKeyDown`, `OnAction`, `KeyContext`, `TabIndex`, `TrackFocus` | [elements](elements.md#mouse-and-keyboard-listeners) (builders) and [events](events.md) (payloads) |
| `NewKeyBinding`, `NewAction`, `ParseKeystroke`, `Keymap` | [events](events.md#keymap-and-bindings) |
| `NewEntity`, `Listener`, `ObserveOn`, `Subscribe`, `Emit` | [app and window](app-window.md) |
| `Refresh`, `FocusNext`, `HitTest`, `PaintQuad`, `RegisterLayoutNode` | [app and window](app-window.md#window) |
| `NewHeadlessPlatform`, `RenderToPNG`, `WithKeymap` | [platform](platform.md) |
| `DefaultFontPath`, `FontManager`, `Shaper`, `TextStyle` | [text and fonts](text-and-fonts.md) |
| `Rgba`, `MustParseHex`, `LinearGradient`, `BoxShadow`, `Edges`, `Px` | [geometry and colour](geometry-and-color.md) |

## Conventions in these pages

- Signature blocks fenced as `text` are listings, not compilable code; fences
  marked `go` are complete declarations checked to compile.
- Pixel arguments are `float32` logical pixels; geometry structs use `Pixels`
  (`float32`).
- Handlers have the shape `func(*Event, *Window, *App)`; events are passed to
  `Dispatch` / `DispatchEvent` by value and to handlers by pointer.
- "Chain" columns and notes about `*StyledMixin` are explained in
  [style builders](style-builders.md#how-the-builders-are-organised).

Related: [concept pages](../concepts/), [guides](../guides/cookbook.md),
[docs index](../README.md).
