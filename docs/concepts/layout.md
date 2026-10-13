# Layout

Source: `gpui/layout.go` (engine), `gpui/style.go` (inputs), `gpui/div.go` and
`gpui/text_elem.go` (node construction). The engine is a flexbox with optional wrapping
modelled on CSS Flexbox / Taffy and is independent of elements: it operates on
a `LayoutNode` tree.

```go
type LayoutNode struct {
    Style    Style
    Children []*LayoutNode
    Measure  func(text string, ts TextStyle, maxWidth float32) (w, h float32)
    Text     string
    Rect        Bounds // output, parent-relative border box
    ContentSize Size   // output, extent of the content (for scrolling)
}
func Layout(n *LayoutNode, available Available) Bounds
```

`Window.Draw` calls `Layout(root, Definite(windowW, windowH))`. You can also
call it directly (handy for tests):

```go
root := &gpui.LayoutNode{Style: gpui.DefaultStyle()}
root.Style.Direction = gpui.DirectionRow
fixed := &gpui.LayoutNode{Style: gpui.DefaultStyle()}
fixed.Style.Width = gpui.Px(50)
grow := &gpui.LayoutNode{Style: gpui.DefaultStyle()}
grow.Style.FlexGrow = 1
grow2 := &gpui.LayoutNode{Style: gpui.DefaultStyle()}
grow2.Style.Width = gpui.Flex(2)
root.Children = []*gpui.LayoutNode{fixed, grow, grow2}
gpui.Layout(root, gpui.Definite(350, 100))
// fixed.Rect = {{0 0} {50 100}}   grow.Rect = {{50 0} {100 100}}
// grow2.Rect = {{150 0} {200 100}}
```

## Core semantics

- **Border-box sizes.** A node's width/height include its padding. Leaves
  include their padding too (text content is inset by it).
- **Parent-relative rects.** `Rect.Origin` is measured from the parent's
  origin. `Window` adds ancestor origins (and subtracts scroll offsets) when it
  builds the render tree, so elements see absolute bounds.
- **Available space is a maximum, never a size.** An axis with an `Auto`
  length sizes to its content (max-content), capped by the bounded available
  length. `Available.BoundedW/H == false` means "no cap" (used while measuring
  base sizes). Helpers: `Definite`, `UnboundedHeight`, `UnboundedWidth`.
- **The root** is forced to the window size on each bounded axis unless its
  length is a `Px`.
- **`Display: false`** children (`Hidden()`) are skipped: zero rect, no gap.
- **One flex line by default**, optional wrapping with `FlexWrap()`, LTR. `Direction` is `DirectionColumn` (default) or
  `DirectionRow`.

## Lengths

`Length` is `Auto()`, `Px(v)` or `Flex(g)`:

| Length | Meaning |
|---|---|
| `Auto()` | Content size. On a min length: the automatic minimum (below). On a max length: no maximum. |
| `Px(v)` | Definite border-box length. |
| `Flex(g)` | CSS `flex: g` - basis 0, grow `g`, shrink 1. Main axis only for flexing; on the **cross** axis it means "fill the line". |

Do not confuse the builders:

| Builder | CSS | Effect |
|---|---|---|
| `Flex1()` | `flex: 1 1 0%` (GPUI `flex_1`) | grow 1, shrink 1, `FlexBasis = Px(0)`: siblings share the space **equally**, whatever their content (each still stops at its automatic minimum size, i.e. its min-content) |
| `FlexAuto()` | `flex: 1 1 auto` | grow 1, shrink 1, auto basis: free space is added **on top of** each item's content size |
| `FlexGrow(1)` | `flex-grow: 1` | the same as `FlexAuto()` with the default shrink 1 |
| `FlexNone()` | `flex: none` | grow 0, shrink 0, auto basis |
| `FlexBasis(v)` | `flex-basis: v px` | a definite base size that overrides the main size |

`SizeFull()` sets both axes to `Flex(1)` and both minimums to `Px(0)` (a
stand-in for `100%`). `Flex()`, `FlexCol()` and `FlexRow()` merely set the
direction; they do not change sizes.

`Style.FlexBasis` is `Auto()` by default (the base size is the main size or
the content). In a content-sized container a basis-0 item still contributes
its content size, so a `Flex1()` child does not collapse a row that has no
definite width.

## Flex algorithm (per container)

Without wrapping, `layoutFlex` follows CSS Flexbox section 9, one line.
Wrapped containers form lines first and run flex resolution on each line
([wrapped flex lines](#wrapped-flex-lines)):

1. **Base size and bounds per item.** Main length `Flex(g)` -> base 0, grow
   `g`, shrink 1. Otherwise a `Px` `FlexBasis` is the base (`Flex1()` sets
   `Px(0)`); else a `Px` main length; else (`Auto`) the item measured with an
   unbounded main axis (for text: unwrapped width, or height at max-content).
   Non-`Flex` items use their `FlexGrow`/`FlexShrink`. The hypothetical size
   clamps the base by min/max.
2. **Automatic minimum size** (when `MinWidth`/`MinHeight` is `Auto`): a `Px`
   min wins; a scroll container (`OverflowHidden`/`OverflowScroll`) gets 0; a
   `Px` main size gives that size; otherwise **row**: min-content width (text:
   longest word; no-wrap row: sum of children, wrapped row/column: max;
   a `Px` width counts as itself), **column**: measured content height.
   Consequence: items refuse to shrink below their content/declared size
   unless you set `MinW(0)`/`MinH(0)` or make them scroll containers. A `Px`-
   sized item **never shrinks** (two 80px children in a 100px row overflow).
3. **Container inner main size.** Definite if the container's own length is
   `Px` or it was forced by its parent; otherwise the sum of contributions,
   capped by the available space but not below the sum of minimums.
4. **Resolve flexible lengths** (`resolveFlex`): grow if the hypothetical sum
   fits in the container, else shrink; iteratively freeze items that violate
   min/max, with shrink weighted by `shrink * base size`, and the "sum of
   factors < 1" rule limiting how much free space is distributed.
5. **Cross sizes.** `Px` cross length wins. An item whose cross length is
   `Flex(..)`, or `Auto` with effective `AlignStretch`, fills the line. Others
   are measured at their final main size (text re-wraps here).
6. **Line cross size** is the container's inner cross size if definite,
   otherwise the largest item cross size.
7. **Main-axis placement** with `Justify`; **cross-axis placement** with
   `AlignItems`/`AlignSelf`; then each child is arranged recursively with its
   final size forced.

### Justify and align

- `Justify`: `JustifyStart` (default), `Center`, `End`, `SpaceBetween`,
  `SpaceAround`. When there is a single item or no free space,
  `SpaceBetween` behaves like `Start` and `SpaceAround` like `Center`. With negative free
  space (overflow), `Center` and `End` shift the items toward the leading side
  (as in CSS "unsafe" alignment).
- `AlignItems`: `AlignStretch` (default), `Start`, `Center`, `End`. A child's
  `AlignSelf` overrides it unless it is `AlignSelfAuto` (the default). There
  is no `AlignSelf` fluent builder; set `StyleRef().AlignSelf`.
- `Gap` applies between **visible in-flow** children on a line. With wrapping
  it also separates lines. `ColumnGap` overrides horizontal spacing; `RowGap`
  overrides vertical spacing. Calling `Gap` resets both overrides.
- Padding insets the content box on all four sides.

### Margins

`Style.Margin` (builders `M`, `Mx`, `My`, `Mt`, `Mr`, `Mb`, `Ml`) works as in
CSS flexbox, without collapsing. `Rect` stays the border box; margins sit
outside it:

- main-axis margins are fixed space, subtracted like gaps before flexing and
  justification, and they add to a content-sized container (and to
  min-content widths);
- cross-axis margins shrink the space an item stretches into and add to the
  line's cross size when the container is content-sized;
- negative margins are allowed (e.g. `Mt(-8)` pulls an item up over its
  previous sibling); the root node's margins are ignored.

**Auto margins** (`Style.MarginAuto`, builders `MAuto`, `MxAuto`, `MyAuto`,
`MtAuto`, `MrAuto`, `MbAuto`, `MlAuto`; setting a pixel margin on an edge
clears its auto flag):

- main axis: positive free space left after flexing is split equally among
  all auto margins on the line, before `Justify` (which then sees none).
  `MlAuto()` on the last child of a row pushes it to the right edge;
  `MxAuto()` centers a single child.
- cross axis: an item with an auto cross margin does not stretch; the free
  cross space goes to its auto margin(s) (both: centered), overriding
  `AlignSelf`/`AlignItems`.

```go
gpui.NewDiv().FlexRow().Child(
    gpui.NewText("Title"),
    gpui.NewDiv().MlAuto().Child(gpui.NewText("right-aligned")),
)
```

Verified example (row, width 300, three 40x20 `Px` children, `Gap(10)`,
`JustifySpaceBetween`): x = 0, 130, 260 (free space 160 split into 80 per
gap, plus the 10px gap).

### Wrapped flex lines

`FlexWrap()` allows row items to continue on another row, or column items to
continue in another column. `FlexNoWrap()` restores the default. Lines form
before flex grow/shrink: an item's hypothetical main size (basis or natural
size, clamped by min/max), fixed margins and the main gap must fit. Exact fits
stay together; oversized items occupy a line alone and may shrink if their
minimum allows it. Hidden and absolute children do not form lines.

The wrapping limit is the definite main size, otherwise bounded available space,
with a pixel maximum also providing a limit when available space is indefinite.
Without a limit there is one line. Auto main size uses the largest line's
content contribution; flexing resolves each line against the final parent size.
`Flex1`, `FlexAuto`, justification and auto margins operate independently on
each line. A wrapped row's min-content width is its widest child's contribution.
Text is measured again at its resolved width so line heights reflect wrapping.

`Gap(v)` retains its main-axis behavior without wrapping and separates both
items and lines with wrapping. `ColumnGap(v)` overrides physical horizontal
gaps; `RowGap(v)` overrides physical vertical gaps, including an explicit zero.
These fields use `Length`: `Auto` falls back to `Gap`, `Px` overrides. A later
`Gap` call resets both overrides; later axis setters replace only their axis.
The cross gap has no effect without wrapping.

```go
func chips() *gpui.Div {
    return gpui.NewDiv().SizeW(180).FlexRow().FlexWrap().Gap(8).
        RowGap(12).ContentStart().Child(
            gpui.NewDiv().SizeWH(80, 24).Bg(gpui.Blue500),
            gpui.NewDiv().SizeWH(80, 24).Bg(gpui.Green600),
            gpui.NewDiv().SizeWH(80, 24).Bg(gpui.Red500),
        )
}
```

`ContentStart`, `ContentCenter`, `ContentEnd`, `ContentBetween`, `ContentAround`
and `ContentStretch` distribute wrapped lines on the cross axis, including a
single wrapped line. The default is stretch: positive free space is divided
equally among lines. `Items*`/`AlignSelf` align children within their own line;
auto cross margins override this alignment. A content-sized cross axis is the
sum of natural line sizes and cross gaps, clamped by its min/max. Negative free
space does not stretch lines; between falls back to start and around to center.
These content builders have no effect without `FlexWrap`. Wrap-reverse is absent.

### Absolute positioning

`Absolute()` removes an element from flex flow: it contributes no size, minimum,
gap or alignment to its parent. `Relative()` restores normal flow; offsets do
not translate an in-flow element. Every absolute child anchors to its immediate
parent's border box, even when that parent has padding. Nested offsets remain
parent-relative. Later siblings paint above earlier siblings, including ordinary
flex children; there is no z-index.

```go
func badge() *gpui.Div {
    return gpui.NewDiv().SizeWH(120, 80).P(12).Bg(gpui.Gray100).Child(
        gpui.NewText("Card"),
        gpui.NewDiv().Absolute().Top(4).Right(4).P(4).Bg(gpui.Red500).
            Child(gpui.NewText("New").TextColor(gpui.White)),
    )
}
```

`Top`, `Right`, `Bottom`, `Left` accept logical pixels (negative values allowed).
`Inset` sets all edges, `InsetX` sets left/right and `InsetY` sets top/bottom.
Unset offsets are `Auto`; reset an edge via `StyleRef().Top = &autoLength` where
`autoLength := gpui.Auto()`. Opposite set edges stretch an auto-sized axis,
clamped by its minimum and maximum. A definite pixel size wins; with two edges
and a size, left/top wins. A sole right/bottom offset anchors the sized box to
that edge. With no offsets the box starts at zero, plus any fixed leading margin.

Otherwise auto sizes use content measured within the remaining parent space.
Flex sizes act as Auto on absolute elements; grow/shrink/basis and parent
alignment do not apply. Fixed margins participate in the offset calculations;
auto margins count as zero. Absolute content can extend positive scrollable
overflow without enlarging a content-sized parent. Percentage offsets, CSS
static-position placement, relative translation and fixed positioning are absent.

### Text in layout

Text leaves are `LayoutNode`s with `Measure` set. The wrap width passed to
`Measure` is, in order: the forced width minus padding; else the available
width minus padding when bounded; else unbounded. Min-content width (`Measure`
with `maxWidth <= 0`) is the width of the longest whitespace-separated word.
At paint time the text wraps to the laid-out content width plus a 0.01 epsilon,
so it matches what layout measured. A `StyledText` measures each run
independently and sums widths / takes the max height.

### Scrolling

`OverflowHidden` and `OverflowScroll` make a node a scroll container: its
automatic minimum size is 0 (so it can be flexed smaller than its content),
painting of its **children** is clipped to its bounds (rounded by its
`CornerRadii` when it has `Rounded(...)`; its own background, border and
shadows are not clipped), and `ContentSize` records the content
extent. `LayoutNode.ScrollMax()` is `max(0, ContentSize - Rect.Size)` per axis.
`OverflowScroll` additionally stores an offset updated by wheel events and, with
`ShowScrollbar`, thumb dragging and track presses
(`Window.dispatchScroll`) and clamped to that range; children are shifted by it
when the render tree is built. The range is 0 on axes outside the
container's `ScrollAxes` (and on both axes of an `OverflowHidden` node), so
`ScrollHandle.SetOffset`, `ScrollToItem` and `Window.SetScrollOffset` cannot
move a non-scrolling axis either. Give a scroll container an `.ID(...)` so its
offset survives re-renders. Children of an `OverflowVisible` node that spill
past it extend its `ContentSize`.

## Known limits

- **Borders are not part of layout size.** `Border(...)` only paints inside the
  box; the layout engine does not inset content by the border width, so add
  matching padding yourself (`TextInput` does this with `P(8)` plus a 1px
  border).
- No wrap-reverse, no `order`, no margin collapsing, no percentages
  (flex-basis is `Auto` or pixels only),
  no aspect ratio, no relative translation or fixed positioning,
  no baseline alignment, no RTL.
- `Px`-sized items do not shrink.
- Hit testing and clipping: hitboxes are clipped to the enclosing clip
  rectangles (not to rounded corners).
- Layout is recomputed from scratch each frame (with per-call measure
  memoization); there is no incremental layout.

See also: [reference/style-builders.md](../reference/style-builders.md) (layout, sizing, spacing and flex builders), [reference/geometry-and-color.md](../reference/geometry-and-color.md) (`Length`, `Available`, enums) and the [cookbook](../guides/cookbook.md#modal-overlay-with-paint-order) for an absolute modal overlay.
