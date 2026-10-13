# Reference: style builders

Source: `gpui/styled.go` (the mixin), `gpui/styled_layout.go` and
`gpui/div.go` / `gpui/text_elem.go` / `gpui/uniform_list.go` (the per-type
forwarders), `gpui/style.go` (`Style`, `StyleRefinement`).

Back to the [reference index](README.md). Concepts:
[styling](../concepts/styling.md), [layout](../concepts/layout.md).

## How the builders are organised

`StyledMixin` holds a `StyleRefinement` (a `Style` whose every field is a
pointer, so "unset" differs from zero) and the Tailwind-style builder methods
that fill it. `Div`, `Text`, `StyledText` and `UniformList` embed it.

Go method promotion has a catch: a method promoted from the embedded mixin
returns `*StyledMixin`, which ends the typed chain (you could no longer call
`.Child(...)` or `.OnClick(...)`). To avoid that, `*Div` re-declares every
mixin builder returning `*Div`, and `*Text` re-declares most of them returning
`*Text`; `*UniformList` forwards a short list. The third column below says
where the chain is preserved; "mixin only" means the call is only reachable as
a promoted method on the embedded field (for example `st.StyledMixin.P(4)` on
a `*StyledText`, or `list.StyledMixin.Gap(4)`), and it returns `*StyledMixin`.

Rules that apply to every builder:

- Each call sets one field of the refinement; the last call to set a field
  wins. Builders never read other fields, so call order only matters for
  fields they share (`Flex1` then `FlexBasis(40)` leaves basis 40;
  `FlexBasis(40)` then `Flex1` leaves basis 0).
- Pixel arguments are `float32` logical pixels.
- `Flex()`, `FlexRow()` and `FlexCol()` set the direction and also set
  `Display` to visible (so they undo an earlier `Hidden()`). A `Div` already
  lays children out as a column by default, so `Flex()` mostly documents
  intent; it is vertical, unlike CSS and Rust GPUI where `flex()` alone gives
  a row.
- `Text` and `StyledText` are leaves: padding, margins, size, text style and
  opacity take effect, but `Bg`/`Border`/`Shadow` paint no box.
- Text-style builders (`TextColor`, `TextSize`, `FontWeight`, `FontFamily`,
  `TextLineHeight`, `TextStrikethrough`) set the element's own style on a
  `Text` and the inherited style for descendants on a `Div` / `UniformList`.

## Every builder, grouped

### Layout: display and direction

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `Flex()` | Make the element a flex container stacking children vertically (column). Unlike CSS/Rust `flex()`, the default direction is column. | Div, Text |
| `FlexRow()` | Flex container with a horizontal main axis. | Div, Text |
| `FlexCol()` | Flex container with a vertical main axis. | Div, Text |
| `Hidden()` | Display:none: no layout space, no paint, no hit testing. | Div, Text |
| `Visible()` | Undo `Hidden()`. | Div, Text |
| `Gap(v float32)` | Main gap; with wrapping, also line gap. Resets RowGap/ColumnGap overrides. | Div, Text |

### Layout: wrapping and line alignment

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `FlexWrap()` | Enable wrapped flex lines. | Div, Text |
| `FlexNoWrap()` | Restore one flex line. | Div, Text |
| `RowGap(v float32)` | Override physical vertical gap, including zero. | Div, Text |
| `ColumnGap(v float32)` | Override physical horizontal gap, including zero. | Div, Text |
| `ContentStretch()` | Divide positive cross free space equally among wrapped lines (default). | Div, Text |
| `ContentStart()` | Pack wrapped lines at cross start. | Div, Text |
| `ContentCenter()` | Center wrapped lines. | Div, Text |
| `ContentEnd()` | Pack wrapped lines at cross end. | Div, Text |
| `ContentBetween()` | Distribute cross free space between wrapped lines. | Div, Text |
| `ContentAround()` | Distribute cross free space around wrapped lines. | Div, Text |

`RowGap`/`ColumnGap` default to `Auto` (fallback to Gap), with `Px` overrides.
Gap resets both overrides, so the last call setting each axis wins. Content
alignment and cross gaps are ignored without wrapping. StyledText and
UniformList inherit these builders without preserving their concrete chain.
See [wrapped flex lines](../concepts/layout.md#wrapped-flex-lines).

### Layout: position

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `Absolute()` | Remove from flex flow and anchor to immediate parent border box. | Div, Text |
| `Relative()` | Restore normal flow; offsets are ignored. | Div, Text |
| `Top(v float32)` | Absolute top offset in logical pixels. | Div, Text |
| `Right(v float32)` | Absolute right offset in logical pixels. | Div, Text |
| `Bottom(v float32)` | Absolute bottom offset in logical pixels. | Div, Text |
| `Left(v float32)` | Absolute left offset in logical pixels. | Div, Text |
| `Inset(v float32)` | Set all four absolute offsets. | Div, Text |
| `InsetX(v float32)` | Set left and right absolute offsets. | Div, Text |
| `InsetY(v float32)` | Set top and bottom absolute offsets. | Div, Text |

Offsets default to `Auto`. Opposite offsets stretch an auto-sized axis subject
to min/max; a pixel size and leading offset take precedence. See
[absolute positioning](../concepts/layout.md#absolute-positioning) for sizing,
margins, overflow and limitations. StyledText and UniformList inherit these
methods from the mixin without preserving their concrete chain.

### Layout: alignment

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `ItemsStart()` | Cross-axis `align-items: flex-start`. | Div, Text |
| `ItemsCenter()` | Cross-axis centre. | Div, Text |
| `ItemsEnd()` | Cross-axis `flex-end`. | Div, Text |
| `ItemsStretch()` | Cross-axis stretch (the default). | Div, Text |
| `JustifyStart()` | Main-axis: pack at the start (default). | Div, Text |
| `JustifyCenter()` | Main-axis: centre. | Div, Text |
| `JustifyEnd()` | Main-axis: pack at the end. | Div, Text |
| `JustifyBetween()` | Main-axis: equal space between children. | Div, Text |
| `JustifyAround()` | Main-axis: half-size gaps at both ends. | Div, Text |

### Sizing

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `SizeWH(w, h float32)` | Definite pixel width and height. | Div, Text, UniformList |
| `SizeW(w float32)` | Definite pixel width. | Div, Text, UniformList |
| `SizeH(h float32)` | Definite pixel height. | Div, Text, UniformList |
| `SizeFull()` | Fill both axes: both lengths become `Flex(1)` and both minimums `Px(0)` (the POC's stand-in for `100%`; there are no percentages). | Div, Text, UniformList |
| `MinW(v float32)` | Minimum width in pixels. | Div, Text |
| `MinH(v float32)` | Minimum height in pixels. | Div, Text, UniformList |
| `MaxW(v float32)` | Maximum width in pixels. | Div, Text |
| `MaxH(v float32)` | Maximum height in pixels. | Div, Text, UniformList |

### Spacing: padding

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `P(v float32)` | Padding on all four edges. | Div, Text, UniformList |
| `Px_(v float32)` | Left and right padding (the trailing underscore avoids the `Px` length constructor). | Div, Text, UniformList |
| `Py_(v float32)` | Top and bottom padding. | Div, Text, UniformList |
| `Pt(v float32)` | Top padding. | Div, Text |
| `Pr(v float32)` | Right padding. | Div, Text |
| `Pb(v float32)` | Bottom padding. | Div, Text |
| `Pl(v float32)` | Left padding. | Div, Text |

### Spacing: margin

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `M(v float32)` | Margin on all four edges. | Div, Text |
| `Mx(v float32)` | Left and right margin. | Div, Text |
| `My(v float32)` | Top and bottom margin. | Div, Text |
| `Mt(v float32)` | Top margin (negative values pull the item over its previous sibling). | Div, Text |
| `Mr(v float32)` | Right margin. | Div, Text |
| `Mb(v float32)` | Bottom margin. | Div, Text |
| `Ml(v float32)` | Left margin. | Div, Text |
| `MAuto()` | All four margins auto (centres a single child on both axes). | Div, Text |
| `MxAuto()` | Left and right margins auto. | Div, Text |
| `MyAuto()` | Top and bottom margins auto. | Div, Text |
| `MtAuto()` | Top margin auto. | Div, Text |
| `MrAuto()` | Right margin auto (pushes a main-axis neighbour left). | Div, Text |
| `MbAuto()` | Bottom margin auto. | Div, Text |
| `MlAuto()` | Left margin auto (on the last child of a row it pushes it to the right edge). | Div, Text |

### Flex item

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `Flex1()` | `flex: 1 1 0%`: grow 1, shrink 1, basis `Px(0)`. Siblings with `Flex1` share space equally, whatever their content. | Div, Text, UniformList |
| `FlexAuto()` | `flex: 1 1 auto`: grow and shrink from the content or main size. | Div, Text |
| `FlexNone()` | `flex: none`: neither grow nor shrink, auto basis. | Div, Text |
| `FlexGrow(v float32)` | Flex-grow factor. | Div, Text |
| `FlexShrink(v float32)` | Flex-shrink factor. | Div, Text |
| `FlexBasis(v float32)` | Definite flex base size in pixels. | Div, Text |

### Overflow and scrolling

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `OverflowHidden()` | Clip the children to the box (rounded by the corner radii) and make the element a scroll container for layout purposes (minimum size 0). No input scrolling. | Div, Text |
| `OverflowScroll()` | `OverflowHidden` plus a wheel-driven offset on both axes. | Div, Text |
| `OverflowXScroll()` | Scroll horizontally; the other axis clips. | Div |
| `OverflowYScroll()` | Scroll vertically; the other axis clips. | Div |

### Background, border, radius, shadow, opacity

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `Bg(f Fill)` | Background fill: an `Rgba`, `Hsla` or `Background` (e.g. `LinearGradient`). A text leaf paints no box, so wrap `Text` in a `Div` for a visible background. | Div, Text, UniformList |
| `Border(w float32, c Rgba)` | Uniform border width and colour. Painted inside the box; layout does not inset content for it. | Div, Text, UniformList |
| `Rounded(px float32)` | All four corner radii. | Div, Text, UniformList |
| `RoundedAll(tl, tr, br, bl float32)` | Individual radii, clockwise from top-left. | Div, Text |
| `Shadow(shadows ...BoxShadow)` | Replace the box shadows. | Div |
| `ShadowNone()` | Remove every box shadow. | Div |
| `Shadow2xs()` | Preset `shadow_2xs`. | Div |
| `ShadowXs()` | Preset `shadow_xs`. | Div |
| `ShadowSm()` | Preset `shadow_sm`. | Div |
| `ShadowMd()` | Preset `shadow_md`. | Div |
| `ShadowLg()` | Preset `shadow_lg`. | Div |
| `ShadowXl()` | Preset `shadow_xl`. | Div |
| `Shadow2xl()` | Preset `shadow_2xl`. | Div |
| `Opacity(f float32)` | Opacity 0..1 (clamped) multiplied into everything the element and its subtree paint; nested opacities multiply. | Div, UniformList |

### Text style (inherited by descendants on a Div)

| Builder | Meaning | Returns the concrete type on |
|---|---|---|
| `TextColor(c Rgba)` | Text colour. | Div, Text, UniformList |
| `TextSize(px float32)` | Font size in pixels (default 14). | Div, Text |
| `FontWeight(w int)` | Weight; 400 regular, 700 bold (600 and above selects a bold face). | Div, Text |
| `FontFamily(name string)` | Font family looked up by file name; `""`, `sans-serif` and unknown names use the default font. | Div, Text |
| `TextLineHeight(px float32)` | Line height in pixels (0 means 1.2 x font size). | Div, Text |
| `TextStrikethrough()` | Strike through descendants' text. | Div, Text |

### Interaction styles (Div only)

These take a closure that receives an empty `*StyledMixin` and returns it. The
refinement is merged over the base style at paint time, so only paint-affecting
fields matter: layout fields (size, padding, flex, gap) are ignored, and only
text colour and strikethrough cascade to descendant text.

| Builder | Meaning |
|---|---|
| `Hover(f func(*StyledMixin) *StyledMixin) *Div` | Applied while the pointer is over the div or a descendant. Applied before `Active`. |
| `Focus(f func(*StyledMixin) *StyledMixin) *Div` | Applied while the div's focus handle is focused. Applied first. |
| `FocusVisible(f func(*StyledMixin) *StyledMixin) *Div` | Applied while focused and the last input was the keyboard (CSS `:focus-visible`). Applied after `Focus`, before `Hover` and `Active`. |
| `Active(f func(*StyledMixin) *StyledMixin) *Div` | Primary press on the hit path, including ancestors; applied last. Clears on primary release or leave; re-entry needs a new press. |

Active tracks GlobalElementID across redraws (use stable IDs) and cancels when
the element disappears, loses Active, or leaves the current hit path. Secondary
buttons do not activate it. StopPropagation does not block its state transitions,
and no click handler is required. Press/release/cancellation redraw automatically.
`Active(nil)` removes the refinement. See [pressed styles](../concepts/events-and-input.md#pressed-styles).

```go
package snippet

import "go-gpui/gpui"

func focusRing() *gpui.Div {
	return gpui.NewDiv().ID("f").TabIndex(0).P(8).Border(1, gpui.Gray300).
		Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(2, gpui.Blue500) }).
		FocusVisible(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray100) }).
		Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray200) }).
		Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray300) })
}
```

### Other `Div`-only builders that affect style or layout

| Builder | Meaning |
|---|---|
| `ShowScrollbar() *Div` | Paint a thin overlay scrollbar when content overflows a scrolling axis (also on `*UniformList`). |
| `TabIndex(i int)` / `TabStop(b bool)` | Focus order; see [events.md](events.md#focus). |
| `Child(...IntoElement)` / `Children(...)` | Append children; see [elements.md](elements.md#div). |

### `Text` extras

`(*Text).Text(s string) *Text` replaces the string. See
[elements.md](elements.md#text).

## What has no builder

Set these through `StyleRef()` (returns `*StyleRefinement`) or by building a
`Style` for a custom element ([elements.md](elements.md#custom-elements)):

- `AlignSelf` (a per-child override of `AlignItems`);
- `ScrollAxes` beyond what the `Overflow*Scroll` builders pick.

There is no builder for wrap-reverse, percentages, grid,
cursor styles or italic. Absolute positioning and negative
margins can overlap siblings
([cookbook](../guides/cookbook.md#modal-overlay-with-paint-order)).

## `Style` and `StyleRefinement`

```text
func DefaultStyle() Style       // visible, column, auto sizes, shrink 1, stretch, opacity 1

type Style struct {
    Display bool; Direction Direction; Position Position
    Top, Right, Bottom, Left Length
    Width, Height, MinWidth, MinHeight, MaxWidth, MaxHeight Length
    Margin, Padding Edges; MarginAuto AutoEdges; Gap Pixels
    Wrap bool; AlignContent AlignContent; RowGap, ColumnGap Length
    FlexGrow, FlexShrink float32; FlexBasis Length
    Justify Justify; AlignItems, AlignSelf Align; Overflow OverflowKind
    BorderWidths Edges; BorderColor Rgba; CornerRadii Corners
    Background Background; BoxShadows []BoxShadow; Opacity float32
    ScrollAxes Axes
    TextStyle TextStyle
}

type StyleRefinement struct { /* the same fields as pointers, plus TextSet TextFields */ }
func (r *StyleRefinement) Refine(base Style) Style          // nil fields keep base; nil receiver returns base
func (r *StyleRefinement) RefineText(base TextStyle) TextStyle

func NewStyledMixin() StyledMixin
func (m *StyledMixin) StyleRef() *StyleRefinement
func (m *StyledMixin) ResolvedStyle(base Style) Style       // merge the refinement onto base
```

`TextFields` (`TextFieldFamily`, `TextFieldSize`, `TextFieldWeight`,
`TextFieldColor`, `TextFieldLineHeight`, `TextFieldStrikethrough`) names the
text fields a refinement sets explicitly; see
[text-and-fonts.md](text-and-fonts.md#textstyle).

Enums and value types used by `Style` are in
[geometry-and-color.md](geometry-and-color.md).

## Which builders exist on which type (summary)

| Type | Chain-preserving builders |
|---|---|
| `*Div` | Every mixin builder, plus `Hover`, `Focus`, `FocusVisible`, `Active`, `ShowScrollbar`, events, focus and child methods. |
| `*Text` | All mixin builders except `Opacity`, `OverflowXScroll`, `OverflowYScroll`, `Shadow` and the shadow presets; plus `Text(s)`. |
| `*UniformList` | `Bg`, `Border`, `Flex1`, `MaxH`, `MinH`, `Opacity`, `P`, `Px_`, `Py_`, `Rounded`, `ShowScrollbar`, `SizeFull`, `SizeH`, `SizeW`, `SizeWH`, `TextColor`, `TrackScroll`. |
| `*StyledText` | None: it only embeds the mixin. Style it through a variable or wrap it in a `Div`. |
