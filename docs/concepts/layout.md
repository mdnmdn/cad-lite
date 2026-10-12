# Layout

Source: `gpui/layout.go` (engine), `gpui/style.go` (inputs), `gpui/div.go` and
`gpui/text_elem.go` (node construction). The engine is a single-line flexbox
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
- **One flex line**, LTR. `Direction` is `DirectionColumn` (default) or
  `DirectionRow`.

## Lengths

`Length` is `Auto()`, `Px(v)` or `Flex(g)`:

| Length | Meaning |
|---|---|
| `Auto()` | Content size. On a min length: the automatic minimum (below). On a max length: no maximum. |
| `Px(v)` | Definite border-box length. |
| `Flex(g)` | CSS `flex: g` - basis 0, grow `g`, shrink 1. Main axis only for flexing; on the **cross** axis it means "fill the line". |

Do not confuse the builders: `Flex1()` is grow 1 / shrink 1 with an **auto**
basis (`flex: 1 1 auto`), while `SizeFull()` sets both axes to `Flex(1)` and
both minimums to `Px(0)` (a stand-in for `100%`). `Flex()`, `FlexCol()` and
`FlexRow()` merely set the direction; they do not change sizes.

## Flex algorithm (per container)

`layoutFlex` follows CSS Flexbox section 9, one line:

1. **Base size and bounds per item.** Main length `Px` -> base = value, uses the
   item's `FlexGrow`/`FlexShrink`. `Flex(g)` -> base 0, grow `g`, shrink 1.
   `Auto` -> base = the item measured with an unbounded main axis (for text:
   unwrapped width, or height at max-content). The hypothetical size clamps the
   base by min/max.
2. **Automatic minimum size** (when `MinWidth`/`MinHeight` is `Auto`): a `Px`
   min wins; a scroll container (`OverflowHidden`/`OverflowScroll`) gets 0; a
   `Px` main size gives that size; otherwise **row**: min-content width (text:
   longest word; container: sum of children along the main axis, max across it;
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
- `Gap` is main-axis only and applies between **visible** children (there is
  no separate row/column gap).
- Padding insets the content box on all four sides.

Verified example (row, width 300, three 40x20 `Px` children, `Gap(10)`,
`JustifySpaceBetween`): x = 0, 130, 260 (free space 160 split into 80 per
gap, plus the 10px gap).

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
painting is clipped to its bounds, and `ContentSize` records the content
extent. `LayoutNode.ScrollMax()` is `max(0, ContentSize - Rect.Size)` per axis.
`OverflowScroll` additionally stores an offset updated by wheel events
(`Window.dispatchScroll`) and clamped to that range; children are shifted by it
when the render tree is built. Give a scroll container an `.ID(...)` so its
offset survives re-renders. Children of an `OverflowVisible` node that spill
past it extend its `ContentSize`.

## Known limits

- **Margins are not implemented.** `Style.Margin` and `M(v)` are stored but the
  layout engine ignores them (no margins on any side, no auto margins).
- **Borders are not part of layout size.** `Border(...)` only paints inside the
  box; the layout engine does not inset content by the border width, so add
  matching padding yourself (`TextInput` does this with `P(8)` plus a 1px
  border).
- No wrap (`flex-wrap`), no `order`, no separate `flex-basis`, no percentages,
  no aspect ratio, no absolute/relative positioning, no row/column gap split,
  no baseline alignment, no RTL.
- `Px`-sized items do not shrink.
- Hit testing and clipping: clipping affects painting only; a child scrolled
  outside its container's bounds still owns its hitbox.
- Layout is recomputed from scratch each frame (with per-call measure
  memoization); there is no incremental layout.

> TODO: this page will gain sections for the features being added to `gpui/`
> (shadows, opacity, scroll, uniform list, tab stops, gradients) once they
> land; until then it documents only what exists today.
