# Reference: geometry, colour, fills and shadows

Source: `geometry.go`, `color.go`, `background.go`,
`shadow.go`, the `Length`/`Available` types in `layout.go` and the
enums in `style.go`. All of it is plain data with no `App` or `Window`
dependency.

Back to the [reference index](README.md). Concept pages:
[layout](../concepts/layout.md), [styling](../concepts/styling.md).

## Geometry

All lengths are `float32` logical pixels (`Pixels`). Device pixels only appear
at the raster boundary. The origin is the top-left corner and +y points down.

| Type | Definition | Notes |
|---|---|---|
| `Pixels` | `float32` | A logical (device-independent) length. Builders take plain `float32`; geometry structs use `Pixels`. |
| `Point` | `{X, Y Pixels}` | Position. |
| `Size` | `{W, H Pixels}` | Extent. |
| `Bounds` | `{Origin Point; Size Size}` | Axis-aligned rectangle. |
| `Edges` | `{Top, Right, Bottom, Left Pixels}` | Padding, margin and border widths. |
| `Corners` | `{TopLeft, TopRight, BottomLeft, BottomRight Pixels}` | Per-corner radii. Note the field order differs from `RoundedAll(tl, tr, br, bl)`. |
| `AutoEdges` | `{Top, Right, Bottom, Left bool}` | Which margin edges are `auto`. |
| `Axes` | `uint8` bit set: `AxisX`, `AxisY`, `AxesBoth` | `Axes.Has(o)` reports whether every axis in `o` is set. Used for `Style.ScrollAxes`. |

### Constructors and methods

```text
func NewBounds(x, y, w, h float32) Bounds

func (b Bounds) Min() Point                     // top-left (== Origin)
func (b Bounds) Max() Point                     // bottom-right
func (b Bounds) Center() Point
func (b Bounds) Width() Pixels
func (b Bounds) Height() Pixels
func (b Bounds) IsEmpty() bool                  // width <= 0 or height <= 0
func (b Bounds) Contains(p Point) bool          // half-open: right/bottom edge is outside
func (b Bounds) ContainsBounds(o Bounds) bool
func (b Bounds) Intersects(o Bounds) bool
func (b Bounds) Intersection(o Bounds) Bounds   // zero Bounds when disjoint
func (b Bounds) Union(o Bounds) Bounds
func (b Bounds) Translate(d Point) Bounds
func (b Bounds) Inset(e Edges) Bounds           // shrink by edge lengths, size clamped at 0

func (e Edges) All(v float32) Edges             // all four edges = v (the receiver is ignored)
func (e Edges) Horizontal() Pixels              // Left + Right
func (e Edges) Vertical() Pixels                // Top + Bottom
func (e Edges) Sum() Pixels                     // Horizontal + Vertical

func (c Corners) Uniform(v float32) Corners     // all four radii = v (the receiver is ignored)
func (c Corners) Max() Pixels                   // largest radius
func (c Corners) IsEmpty() bool                 // Max() <= 0

func (a Axes) Has(o Axes) bool
```

`Edges{}.All(4)` and `Corners{}.Uniform(8)` are the usual spellings.

## Lengths and layout constraints

```text
type LengthKind uint8      // LengthAuto, LengthPx, LengthFlex
type Length struct { Kind LengthKind; Val Pixels }

func Auto() Length          // content-based
func Px(v float32) Length   // definite logical pixels
func Flex(g float32) Length // CSS `flex: g` (basis 0, grow g, shrink 1)

func (l Length) IsAuto() bool
func (l Length) IsPx() bool
func (l Length) IsFlex() bool
```

| Kind | Meaning |
|---|---|
| `LengthAuto` | Content-based. On a min-length it is the CSS automatic minimum size; on a max-length it means "no maximum". |
| `LengthPx` | Definite length in logical pixels. |
| `LengthFlex` | `flex: Val`; only meaningful on the main axis of a flex container. |

`Style.Width`/`Height`/`MinWidth`/... are `Length`s; the fluent builders
(`SizeW`, `MinH`, ...) set them for you
([style-builders.md](style-builders.md)).

```text
type Available struct { W, H Pixels; BoundedW, BoundedH bool }

func Definite(w, h float32) Available          // both axes bounded
func UnboundedHeight(availW float32) Available // definite width, indefinite height (scroll container)
func UnboundedWidth(availH float32) Available  // definite height, indefinite width
```

`Available` is the size constraint handed to `Layout`; a `false` bounded flag
means that axis is indefinite (max-content sizing). `Layout` itself and
`LayoutNode` are listed in [elements.md](elements.md#layout-engine-types).

## Layout enums

| Type | Values | Meaning |
|---|---|---|
| `Position` | `PositionRelative` (zero value), `PositionAbsolute` | Normal flex flow or out-of-flow anchoring to the immediate parent box. Relative offsets are ignored. |
| `Direction` | `DirectionColumn` (zero value), `DirectionRow` | Flex main axis. |
| `AlignContent` | `AlignContentStretch` (zero value), `AlignContentStart`, `AlignContentCenter`, `AlignContentEnd`, `AlignContentSpaceBetween`, `AlignContentSpaceAround` | Cross-axis distribution of wrapped lines, including a single line. Ignored without Wrap. |
| `Justify` | `JustifyStart`, `JustifyCenter`, `JustifyEnd`, `JustifySpaceBetween`, `JustifySpaceAround` | Main-axis distribution. |
| `Align` | `AlignStretch` (zero value), `AlignStart`, `AlignCenter`, `AlignEnd`; `AlignSelfAuto` (= 255) | Cross-axis alignment (`AlignItems`, `AlignSelf`). `AlignSelfAuto` on a child means "inherit the parent's `AlignItems`". |
| `OverflowKind` | `OverflowVisible` (zero value), `OverflowHidden`, `OverflowScroll` | Clipping and scrollability. Never changes how descendants are laid out. |

## Colour

```text
type Rgba struct { R, G, B, A uint8 }          // straight (non-premultiplied)
type Hsla struct { H, S, L, A float32 }        // H in degrees [0,360); S, L, A in [0,1]

func RgbaOf(r, g, b, a uint8) Rgba
func Gray(v uint8) Rgba                        // opaque grey
func ParseHex(s string) (Rgba, error)          // "#rgb", "#rgba", "#rrggbb", "#rrggbbaa"
func MustParseHex(s string) Rgba               // panics on error; for literals

func (c Rgba) Opacity(f float32) Rgba          // alpha *= f (f clamped to [0,1])
func (c Rgba) Premultiply() Rgba               // RGB scaled by alpha
func (c Rgba) IsTransparent() bool
func (c Rgba) ToHsla() Hsla
func (c Rgba) ToBackground() Background        // solid fill

func (c Hsla) ToRgba() Rgba
func (c Hsla) ToBackground() Background
```

Predefined values:

| Group | Names |
|---|---|
| Basics | `White`, `Black`, `Transparent` |
| Greys (light to dark) | `Gray50` `#fafafa`, `Gray100` `#f5f5f5`, `Gray200` `#eeeeee`, `Gray300` `#e0e0e0`, `Gray400` `#bdbdbd`, `Gray500` `#9e9e9e`, `Gray600` `#757575`, `Gray700` `#616161`, `Gray800` `#424242`, `Gray900` `#212121` |
| Accents | `Blue500` `#2196f3`, `Blue600` `#1e88e5`, `Red500` `#f44336`, `Red600` `#e53935`, `Green600` `#43a047`, `Yellow400` `#ffca28` |

The palette is deliberately small; define your own with `MustParseHex`.
`Image()` / `NRGBAAt` results are straight alpha, while `HeadlessWindow.Pixels`
and the rasterizer buffer are premultiplied
([platform.md](platform.md#headlessplatform-and-headlesswindow)).

## Fills and gradients

```text
type Fill interface{ ToBackground() Background }   // Rgba, Hsla and Background all satisfy it

type BackgroundKind uint8   // BackgroundNone, BackgroundSolid, BackgroundLinearGradient
type Background struct { Kind BackgroundKind; Solid Rgba; Gradient Gradient }

func SolidBackground(c Rgba) Background
func LinearGradient(angle float32, from, to ColorStop) Background  // 0 = towards top, 90 = towards right

func (b Background) ColorSpace(cs ColorSpace) Background  // interpolate in cs
func (b Background) Opacity(f float32) Background         // multiply every colour's alpha
func (b Background) IsTransparent() bool
func (b Background) ToBackground() Background

type ColorStop struct { Color Rgba; Percentage float32 }   // Percentage 0..1 along the gradient line
func LinearColorStop(c Rgba, percentage float32) ColorStop

type Gradient struct { Angle float32; Stops [2]ColorStop; Space ColorSpace }

type ColorSpace uint8       // ColorSpaceSrgb (default), ColorSpaceOklab
func (c ColorSpace) String() string   // "sRGB" or "Oklab"
```

`Bg(f Fill)` accepts any of these. Gradients have exactly two stops, and
gradient borders and shadows are not supported (the first stop is used).

## Box shadows

```text
type BoxShadow struct {
    Color        Rgba
    Offset       Point
    BlurRadius   Pixels   // CSS blur radius; gaussian sigma is half of it
    SpreadRadius Pixels   // grows (negative: shrinks) the shape
    Inset        bool     // draw inside the element
}

func NewBoxShadow(offsetX, offsetY float32, color Rgba) BoxShadow // unblurred outset shadow
func (s BoxShadow) WithBlur(r float32) BoxShadow
func (s BoxShadow) WithSpread(r float32) BoxShadow
func (s BoxShadow) AsInset() BoxShadow
```

Apply with `Shadow(...BoxShadow)` or one of the presets `ShadowNone`,
`Shadow2xs`, `ShadowXs`, `ShadowSm`, `ShadowMd`, `ShadowLg`, `ShadowXl`,
`Shadow2xl` ([style-builders.md](style-builders.md#background-border-radius-shadow-opacity)).

```go
package snippet

import "go-draw/internal/gpui"

var glow = gpui.NewBoxShadow(0, 0, gpui.Blue500.Opacity(0.4)).WithBlur(12).WithSpread(2)

var brand = gpui.LinearGradient(135,
	gpui.LinearColorStop(gpui.Blue500, 0),
	gpui.LinearColorStop(gpui.MustParseHex("#7e57c2"), 1),
).ColorSpace(gpui.ColorSpaceOklab)

func chip() *gpui.Div {
	return gpui.NewDiv().Px_(12).Py_(6).Rounded(999).Bg(brand).Shadow(glow)
}
```
