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

- Set them on the **`Text`**: `NewText("x").TextSize(18).FontWeight(700)
  .TextColor(gpui.Gray600)`. The builders exist on `Div` too, but text style
  does **not** cascade from a `Div` to its descendants today: each `Text`
  starts from `DefaultStyle()`, so `Div.TextColor(...)` has no visible effect.
- **Bold.** Any `FontWeight >= 600` selects a bold face. The font manager
  looks for a bold sibling file of the default font (`NameBold.ttf`), then
  well-known bold system fonts; if none exists the regular outlines are
  emboldened at raster time with a one-pixel horizontal smear (synthetic
  bold). Weights between 600 and 900 are all "bold"; there are no intermediate
  weights.
- **Family.** `FontFamily` is accepted but not honoured: every family resolves
  to the platform default font (see [text.md](text.md)).
- **Line height.** `TextLineHeight(px)` changes where lines are painted, but
  the measured height of wrapped text is currently always
  `lines * 1.2 * FontSize`, so a custom line height can overlap or leave a
  gap relative to the layout box.
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
