# Styling

Source: `style.go`, `styled.go`, `color.go`,
`geometry.go`.

## Style and StyleRefinement

`Style` is the fully-resolved record the layout engine and painters read:
display, direction, position and offsets, width/height and min/max lengths, margin (with per-edge
auto flags), padding, gap, flex grow/shrink/basis, justify/align, overflow,
border widths/colour, corner radii, `Background`, `BoxShadows`, `Opacity` and a
`TextStyle`. `DefaultStyle()` is the starting point.

Fluent builders do not edit a `Style` directly. They write into a
`StyleRefinement` - the same fields as pointers, where `nil` means "not set" -
and `Refine(base)` overlays the set fields onto a base `Style` at the point of
use. That is how a bare `Div` can distinguish "width: 0" from "width: unset".

`StyledMixin` (embedded in `Div`, `Text`, `StyledText`, `UniformList`) holds the refinement
and the Tailwind-flavoured builder methods. Only `Div` and `UniformList` paint
a box: `Text`/`StyledText` accept `Bg`/`Border`/`Shadow` but draw none of them
(padding, size, margins, text style and opacity do apply):

| Group | Methods |
|---|---|
| Size | `SizeWH(w,h)`, `SizeW`, `SizeH`, `SizeFull()`, `MinW/MinH/MaxW/MaxH` |
| Flex | `Flex()`, `FlexRow()`, `FlexCol()`, `Flex1()`, `FlexAuto()`, `FlexNone()`, `FlexBasis(v)`, `FlexGrow(v)`, `FlexShrink(v)`, `Gap(v)`, `FlexWrap()`, `FlexNoWrap()`, `RowGap(v)`, `ColumnGap(v)` |
| Position | `Absolute()`, `Relative()`, `Top/Right/Bottom/Left(v)`, `Inset(v)`, `InsetX(v)`, `InsetY(v)`; see [layout](layout.md#absolute-positioning) |
| Line alignment | `ContentStart/Center/End/Stretch/Between/Around()`; requires `FlexWrap()` |
| Alignment | `ItemsStart/Center/End/Stretch()`, `JustifyStart/Center/End/Between/Around()` |
| Spacing | `P(v)`, `Px_`, `Py_`, `Pt`, `Pr`, `Pb`, `Pl`; margins `M(v)`, `Mx`, `My`, `Mt`, `Mr`, `Mb`, `Ml` and auto margins `MAuto`, `MxAuto`, `MyAuto`, `MtAuto`, `MrAuto`, `MbAuto`, `MlAuto` |
| Paint | `Bg(fill)`, `Rounded(px)`, `RoundedAll(tl,tr,br,bl)`, `Border(w, c)`, `Opacity(f)`, `Shadow(...)`, `ShadowNone()`, `Shadow2xs()`, `ShadowXs/Sm/Md/Lg/Xl()`, `Shadow2xl()` |
| Overflow | `OverflowHidden()`, `OverflowScroll()`, `OverflowXScroll()`, `OverflowYScroll()` |
| Visibility | `Hidden()`, `Visible()` |
| Text | `TextColor`, `TextSize`, `FontWeight`, `FontFamily`, `TextStrikethrough`, `TextLineHeight` |

Anything without a builder (`AlignSelf`, per-edge borders, per-edge margin,
`Direction` beyond row/column) is reachable through `StyleRef()`, which returns
the `*StyleRefinement`; assign a pointer:

```go
self := gpui.AlignEnd
d.StyleRef().AlignSelf = &self
bw := gpui.Edges{Bottom: 2}
d.StyleRef().BorderWidths = &bw
bc := gpui.Gray400
d.StyleRef().BorderColor = &bc
```

Later calls overwrite earlier ones for the same property. Padding builders
compose (`P(8).Pl(16)` keeps top/right/bottom 8).

## Colours

`Rgba{R,G,B,A uint8}` is straight (non-premultiplied). Constructors and
helpers:

```go
c, err := gpui.ParseHex("#336699cc") // "#rgb", "#rgba", "#rrggbb", "#rrggbbaa"
bg := gpui.MustParseHex("#fff")      // panics on bad input
x := gpui.RgbaOf(10, 20, 30, 255)
g := gpui.Gray(128)
half := c.Opacity(0.5)               // scales alpha
h := gpui.Blue500.ToHsla()           // Hsla{H,S,L,A}; Hsla.ToRgba() converts back
```

Predefined values: `White`, `Black`, `Transparent`, `Gray50` ... `Gray900`
(Material-style), `Blue500/600`, `Red500/600`, `Green600`, `Yellow400`.
The rasterizer premultiplies at draw time and composites source-over.
A fully transparent background is not painted.

## Borders and corner radii

- `Border(w, c)` sets a uniform width and colour; per-edge widths go through
  `StyleRef().BorderWidths`.
- `Rounded(px)` sets all four radii; `RoundedAll(tl, tr, br, bl)` (top-left,
  top-right, bottom-right, bottom-left) sets them individually.
- The background is a rounded-rect quad; the border is a **separate** quad
  drawn as the ring between the outer rounded rect and the inner one (inset by
  the border widths, radii reduced accordingly). Both are anti-aliased.
- The border is painted inside the element's bounds and is **not** included in
  layout size: content is not inset by it. Add padding at least as large as
  the border width.
- With `OverflowHidden`/`OverflowScroll`, children are clipped to the parent's
  bounds rounded by its radii; the parent's own background, border and shadows
  are not clipped. Hitboxes are clipped to rectangles only, not to rounded
  corners.

## Text styles

`TextStyle{FontFamily, FontSize, FontWeight, Color, LineHeight, Strikethrough}`
with defaults 14px, weight 400, black, line height `1.2 * FontSize`
(`ResolvedLineHeight()`).

- Set them on a `Text` (`NewText("x").TextSize(18).FontWeight(700)
  .TextColor(gpui.Gray600)`) or on any ancestor `Div`: text style **cascades**.
  During `RequestLayout` the window keeps a text-style stack; a `Div` pushes
  its explicitly set fields (`TextColor`, `TextSize`, `FontWeight`,
  `FontFamily`, `TextLineHeight`, `TextStrikethrough`) over the inherited
  style, and `Text`, `Str`, `StyledText` and `TextInput` resolve own explicit
  fields over inherited over `DefaultTextStyle()`. Only fields that were set
  are overridden (`StyleRefinement.TextSet` bit set), so a child's
  `TextColor` keeps the ancestor's size. `StyledText` runs with `FontSize == 0`
  use the resolved style of the `StyledText`; other runs are used as given.
  Rows of a `UniformList` inherit the style enclosing the list refined by the
  list's own text fields, as do the children of a custom container built
  with `Window.RequestLayoutWithChildren` (its `style.TextStyle` overrides
  the fields that are non-zero and differ from `DefaultTextStyle()`).
- **Bold.** Any `FontWeight >= 600` selects a bold face. The font manager
  looks for the family's bold file, then a bold sibling of the default font
  (`NameBold.ttf`), then well-known bold system fonts; if none exists the
  regular outlines are emboldened at raster time with a one-pixel horizontal
  smear (synthetic bold). There are no intermediate weights.
- **Family.** `FontFamily` is looked up by name (case, spaces and dashes
  ignored) in the system font directories by file name, e.g. `"Arial"`,
  `"Menlo"`, `"DejaVu Sans"`; `"monospace"`, `"ui-monospace"`, `"serif"` and
  `"cursive"` map to common fonts. Unknown or generic (`"sans-serif"`,
  `"system-ui"`) families use the default font. See [text.md](text.md).
- **Line height.** `TextLineHeight(px)` sets the line height used by both
  measurement and painting.
- **Strikethrough** is drawn as a thin `Underline` primitive across each
  line's width.
- `StyledText` takes a full `TextStyle` per run instead of the builder.

## Backgrounds and gradients

`Bg` takes a `Fill`: `Rgba`, `Hsla` or a `Background` (`background.go`).
`LinearGradient(angle, from, to)` builds a two-stop gradient; the angle is in
degrees with CSS semantics (0 points up, 90 right) and each stop is
`LinearColorStop(color, percentage)` with the percentage in 0..1 along the
gradient line. Exactly two stops are supported.

```go
grad := gpui.LinearGradient(90,
    gpui.LinearColorStop(gpui.Blue500, 0),
    gpui.LinearColorStop(gpui.Red500, 1)).ColorSpace(gpui.ColorSpaceOklab)
card := gpui.NewDiv().SizeWH(120, 60).Bg(grad).Rounded(8)
```

Interpolation defaults to sRGB (`ColorSpaceSrgb`); `.ColorSpace(ColorSpaceOklab)`
switches to Oklab. `Background.Opacity(f)` scales every stop's alpha. Only
the background can be a gradient: borders and shadows are solid colours. The
rasterizer samples a 256-entry LUT per quad (`gradientMapper`).

## Shadows

`Shadow(...BoxShadow)` replaces the element's shadows; the presets
(`ShadowSm()`, `ShadowMd()` ... `Shadow2xl()`) copy GPUI's `shadow_*` values.
Build your own with a fluent chain:

```go
s := gpui.NewBoxShadow(0, 4, gpui.RgbaOf(0, 0, 0, 64)).WithBlur(12).WithSpread(1)
d := gpui.NewDiv().Rounded(8).Shadow(s, s.AsInset())
```

`BoxShadow{Color, Offset, BlurRadius, SpreadRadius, Inset}`: the CSS blur
radius is twice the gaussian sigma; spread grows (negative: shrinks) the shape
and the corner radii. Outset shadows paint before the background, inset
shadows after it and clipped to the border box (`paintBox` in
`shadow.go`). Shadows follow the element's corner radii.

## Opacity

`Opacity(f)` (clamped to 0..1) multiplies the alpha of **every primitive** the
element and its descendants paint (quads, borders, gradients, shadows, glyphs);
nested opacities multiply. It is honoured on `Div`, `UniformList`, `Text`
and `StyledText`. It is not a group layer: where two children overlap
inside a translucent parent, both are blended individually and the overlap is
denser than a flattened layer would be.

## Interaction styles: hover, focus, focus-visible, active

`Div` has GPUI's state-dependent refinements:

```go
btn := gpui.NewDiv().ID("save").TabIndex(0).P(8).Bg(gpui.Blue500).
    Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Blue600) }).
    Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(2, gpui.Gray400) }).
    FocusVisible(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(2, gpui.Green600) }).
    Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray700) })
```

- `Hover` applies while the pointer is over the div's hitbox (it follows the
  hit path, so a hovered descendant counts).
- `Focus` applies while the div's focus handle is focused. The handle comes
  from `TrackFocus(h)` or, for `TabIndex(i)` divs without one, an implicit
  handle retained per element id.
- `FocusVisible` additionally requires the last input to be the keyboard
  (`Window.FocusVisible()`: key press sets it, mouse press clears it), like CSS
  `:focus-visible`.
- `Active` captures primary-button presses on the hit path, including ancestors.
  Release or leaving cancels it; returning while held does not reactivate it.
  Stable IDs retain it across redraws; hidden/removed/occluded elements lose it.
  Secondary buttons do not activate it, and no handler is needed.
- Precedence: base style, then `Focus`, then `FocusVisible`, then `Hover`, then `Active`.

They are resolved at paint (`paintOpacity` in `div.go`), so layout
fields set in them (size, padding, margin, flex) have **no effect**. Their
`TextColor` and `TextStrikethrough` do reach descendant text: while painting
the div's subtree the window keeps a paint-time text-override stack, and
`Text`/`StyledText` apply it over the style resolved at layout, except for
fields the text element (or a nearer ancestor `Div`/`UniformList`) sets
itself. Text fields that affect layout (`TextSize`, `FontWeight`,
`FontFamily`, `TextLineHeight`) in an interaction style are ignored for
descendant text. The paint-time override is pushed only by `Div` and
`UniformList`: text inside a custom `RequestLayoutWithChildren` container does
not stop an outer div's hover colour from applying. Visual-only properties (`Bg`, `Border`, `Rounded`, `Shadow`,
`Opacity`, text colour) are the intended use. `Active` applies last while a primary press stays on the hit path.

The older manual pattern still works and is the way to change layout or text
on hover: store `hovered` in your view, set it from
`OnHoverStart`/`OnHoverEnd`, call `w.Refresh()`, and rebuild with a different
style (see `examples/counter/main.go`). Give hover-aware elements an
`.ID(...)`; hover re-evaluates after every draw at the last pointer position.
See [events-and-input.md](events-and-input.md#hover).

## Flex shorthands and margins

Layout semantics are in [layout.md](layout.md); the builders are:

| Builder | CSS | Notes |
|---|---|---|
| `Flex1()` | `flex: 1 1 0%` | grow 1, shrink 1, basis 0: siblings share space equally |
| `FlexAuto()` | `flex: 1 1 auto` | free space added on top of content size |
| `FlexNone()` | `flex: none` | no grow, no shrink |
| `FlexBasis(v)` | `flex-basis: Npx` | definite base size |
| `Mx/My/Mt/Mr/Mb/Ml(v)` | `margin-*` | outside the border box; negative allowed |
| `MAuto()`, `MxAuto()`, `MlAuto()` ... | `margin: auto` | absorbs free main-axis space (`MlAuto` on the last row child right-aligns it) |

A pixel margin on an edge clears that edge's auto flag and vice versa.

## Text style inheritance

Text style cascades. The `Text*`/`Font*` builders on a `Div` record **only the
fields they set** (`StyleRefinement.TextSet`, a `TextFields` bit set:
`TextFieldFamily`, `TextFieldSize`, `TextFieldWeight`, `TextFieldColor`,
`TextFieldLineHeight`, ...); descendants resolve explicit fields over the
nearest ancestor's over `DefaultTextStyle()`.

```go
gpui.NewDiv().FontFamily("Menlo").TextSize(12).TextColor(gpui.Gray600).Child(
    gpui.NewText("inherits all three"),
    gpui.NewText("keeps family+size").TextColor(gpui.Red500))
```

Rows built by a `UniformList` render callback inherit too: the list captures
the text style at `RequestLayout` (enclosing style refined by its own
`TextColor` etc.) and pushes it while measuring item 0 and laying out rows.

## Font family and fallback

`FontFamily(name)` resolves by file name in the system font directories
(`FontManager.Face`); unknown or generic names use the default font (override
the default with `GPUI_FONT`). Glyphs the chosen face lacks are drawn from
system fallback fonts (`FontManager.Fallback`), so CJK and symbols usually
render; colour emoji do not (bitmap fonts: tofu). Details in [text.md](text.md).

See also: [reference/style-builders.md](../reference/style-builders.md) (every builder with signature and meaning), [reference/geometry-and-color.md](../reference/geometry-and-color.md) and the [cookbook](../guides/cookbook.md#card-with-shadow-and-gradient).
