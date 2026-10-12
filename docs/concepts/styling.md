# Styling

Source: `gpui/style.go`, `gpui/styled.go`, `gpui/color.go`,
`gpui/geometry.go`.

## Style and StyleRefinement

`Style` is the fully-resolved record the layout engine and painters read:
display, direction, width/height and min/max lengths, margin, padding, gap,
flex grow/shrink, justify/align, overflow, border widths/colour, corner radii,
background and a `TextStyle`. `DefaultStyle()` is the starting point.

Fluent builders do not edit a `Style` directly. They write into a
`StyleRefinement` - the same fields as pointers, where `nil` means "not set" -
and `Refine(base)` overlays the set fields onto a base `Style` at the point of
use. That is how a bare `Div` can distinguish "width: 0" from "width: unset".

`StyledMixin` (embedded in `Div`, `Text`, `StyledText`) holds the refinement
and the Tailwind-flavoured builder methods:

| Group | Methods |
|---|---|
| Size | `SizeWH(w,h)`, `SizeW`, `SizeH`, `SizeFull()`, `MinW/MinH/MaxW/MaxH` |
| Flex | `Flex()`, `FlexRow()`, `FlexCol()`, `Flex1()`, `FlexGrow(v)`, `FlexShrink(v)`, `Gap(v)` |
| Alignment | `ItemsStart/Center/End/Stretch()`, `JustifyStart/Center/End/Between/Around()` |
| Spacing | `P(v)`, `Px_`, `Py_`, `Pt`, `Pr`, `Pb`, `Pl`, `M(v)` (margin: stored, **not laid out**) |
| Paint | `Bg(c)`, `Rounded(px)`, `RoundedAll(tl,tr,br,bl)`, `Border(w, c)` |
| Overflow | `OverflowHidden()`, `OverflowScroll()` |
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
A background with `A == 0` is not painted.

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
- Radii are clipped by the quad's content mask like any primitive, but a
  rounded parent does **not** clip its children to the rounded shape;
  `OverflowHidden` clips to the rectangular bounds.

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
  Limit: rows of a `UniformList` are laid out outside the walk and do not
  inherit.
- **Bold.** Any `FontWeight >= 600` selects a bold face. The font manager
  looks for the family's bold file, then a bold sibling of the default font
  (`NameBold.ttf`), then well-known bold system fonts; if none exists the
  regular outlines are emboldened at raster time with a one-pixel horizontal
  smear (synthetic bold). There are no intermediate weights.
- **Family.** `FontFamily` is looked up by name (case, spaces and dashes
  ignored) in the system font directories by file name, e.g. `"Arial"`,
  `"Menlo"`, `"DejaVu Sans"`; `"monospace"` and `"serif"` map to common
  fonts. Unknown or generic (`"sans-serif"`, `"system-ui"`) families use the
  default font. See [text.md](text.md).
- **Line height.** `TextLineHeight(px)` sets the line height used by both
  measurement and painting.
- **Strikethrough** is drawn as a thin `Underline` primitive across each
  line's width.
- `StyledText` takes a full `TextStyle` per run instead of the builder.

## Hover (and other interaction styles)

There are no `hover:` style refinements. Hover is expressed by registering
`OnHoverStart`/`OnHoverEnd`, storing the state in your own struct, and
rebuilding the element with a different style on the next frame:

```go
type Btn struct{ hovered bool }

func (b *Btn) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
    bg := gpui.Blue500
    if b.hovered {
        bg = gpui.Blue600
    }
    return gpui.NewDiv().ID("btn").P(10).Bg(bg).Rounded(6).
        OnHoverStart(func(w *gpui.Window, _ *gpui.App) { b.hovered = true; w.Refresh() }).
        OnHoverEnd(func(w *gpui.Window, _ *gpui.App) { b.hovered = false; w.Refresh() }).
        Child(gpui.NewText("Save").TextColor(gpui.White).FontWeight(700))
}
```

Give hover-aware elements an `.ID(...)`: hover state is keyed by the element's
global id, so a stable id keeps the state across re-renders. Hover also
re-evaluates after every draw at the last pointer position. See
[events-and-input.md](events-and-input.md#hover) for ordering.

> TODO: this page will gain sections for the features being added to `gpui/`
> (shadows, opacity, scroll, uniform list, tab stops, gradients) once they
> land; until then it documents only what exists today.
