# `gpui` API contract

This file is the **coordination contract** for the Go port of GPUI. Every file in
package `gpui` (and `ebitenplatform`) must conform to the signatures below.
If an implementation needs a signature to change, change it **here first**, then
in the code, so the other agents stay in sync.

Module: `cad-lite` (library at `internal/gpui/`). Core package `cad-lite/internal/gpui` must depend only on the Go
standard library. `golang.org/x/image` and `github.com/go-text/typesetting` are
allowed **only** in `font.go` / `text.go`. `github.com/hajimehoshi/ebiten/v2`
is allowed **only** in `ebitenplatform/`.

Units: all geometry is in **logical pixels** (`float32`), origin top-left, +y down.
`Pixels` is a distinct float32 type used where GPUI distinguishes logical px.

---

## 1. `geometry.go`

```go
type Pixels float32
type Point struct{ X, Y Pixels }
type Size struct{ W, H Pixels }
type Bounds struct{ Origin Point; Size Size }
type Edges struct{ Top, Right, Bottom, Left Pixels }
type Corners struct{ TopLeft, TopRight, BottomLeft, BottomRight Pixels }

func (b Bounds) Min() Point
func (b Bounds) Max() Point
func (b Bounds) Width() Pixels
func (b Bounds) Height() Pixels
func (b Bounds) Contains(p Point) bool
func (b Bounds) Intersects(o Bounds) bool
func (b Bounds) Translate(d Point) Bounds
func (b Bounds) Inset(e Edges) Bounds
func (b Bounds) Union(o Bounds) Bounds
```

## 2. `color.go`

```go
type Rgba struct{ R, G, B, A uint8 } // straight (non-premultiplied)
type Hsla struct{ H, S, L, A float32 }

func RgbaOf(r, g, b, a uint8) Rgba
func ParseHex(s string) (Rgba, error)      // "#rrggbb", "#rrggbbaa", "#rgb"
func (c Rgba) Premultiply() Rgba           // returns premultiplied-encoded Rgba
func (c Rgba) ToHsla() Hsla
func (c Hsla) ToRgba() Rgba
func (c Rgba) Opacity(f float32) Rgba      // multiplies alpha by f

// Package-level common colors (straight alpha, a=255):
var (
    White, Black, Transparent Rgba
    Gray50, Gray100, Gray200, Gray300, Gray400, Gray500, Gray600, Gray700, Gray800, Gray900 Rgba
    Blue500, Blue600, Red500, Green600, Yellow400 Rgba
)
```

## 3. `style.go` + `styled.go`

```go
type LengthKind uint8
const (
    LengthAuto LengthKind = iota // content-based; also the automatic min size
    LengthPx
    LengthFlex                    // flex-basis:0 + flex-grow:Val (CSS `flex: Val`)
)
type Length struct {
    Kind LengthKind
    Val  Pixels
}
func Auto() Length
func Px(v float32) Length
func Flex(g float32) Length

type Position uint8
const (PositionRelative Position = iota; PositionAbsolute)

type Direction uint8
const (DirectionColumn Direction = iota; DirectionRow)

type AlignContent uint8
const (
    AlignContentStretch AlignContent = iota
    AlignContentStart
    AlignContentCenter
    AlignContentEnd
    AlignContentSpaceBetween
    AlignContentSpaceAround
)

type Justify uint8
const (
    JustifyStart Justify = iota
    JustifyCenter
    JustifyEnd
    JustifySpaceBetween
    JustifySpaceAround
)

type Align uint8
const (
    AlignStretch Align = iota
    AlignStart
    AlignCenter
    AlignEnd
)
const AlignSelfAuto Align = 255

type OverflowKind uint8
const (
    OverflowVisible OverflowKind = iota
    OverflowHidden
    OverflowScroll
)

// Style is the resolved, per-element style used by layout and paint.
type Style struct {
    Display  bool // false => hidden (Display:none), skipped entirely
    Direction Direction
    Wrap bool // false (default) keeps one flex line
    AlignContent AlignContent // Stretch (default); wrapped-line cross distribution
    RowGap, ColumnGap Length // Auto falls back to Gap; Px overrides physical axis
    Position Position // Relative (default) participates in flex; Absolute does not
    Top, Right, Bottom, Left Length // Auto (default) or Px; only for Absolute

    Width, Height             Length
    MinWidth, MinHeight       Length // LengthAuto => automatic minimum size
    MaxWidth, MaxHeight       Length // LengthAuto => no maximum

    Margin, Padding Edges
    MarginAuto      AutoEdges // per-edge `margin: auto` (overrides Margin on that edge)
    Gap            Pixels // main-axis gutter between adjacent children

    FlexGrow   float32 // default 0
    FlexShrink float32 // default 1
    FlexBasis  Length  // default Auto (= the main size); Px(v) overrides it

    Justify   Justify
    AlignItems Align
    AlignSelf Align  // AlignSelfAuto means "inherit AlignItems"
    Overflow  OverflowKind

    BorderWidths Edges
    BorderColor  Rgba
    CornerRadii  Corners
    Background   Background // solid color or linear gradient; transparent => no quad
    BoxShadows   []BoxShadow
    Opacity      float32    // default 1; multiplies the alpha of the whole subtree at paint
    ScrollAxes   Axes       // axes OverflowScroll scrolls; default AxesBoth

    TextStyle TextStyle
}

// AutoEdges marks which margin edges are `auto`.
type AutoEdges struct{ Top, Right, Bottom, Left bool }

// Axes is a bit set of axes.
type Axes uint8
const (
    AxisX Axes = 1 << iota
    AxisY
    AxesBoth = AxisX | AxisY
)

// StyleRefinement is the partial/fluent form; Refine(base) fills unset fields
// from base. All fields are pointers so "unset" is distinguishable.
type StyleRefinement struct {
    Display   *bool
    Direction *Direction
    Wrap *bool
    AlignContent *AlignContent
    RowGap, ColumnGap *Length
    Position *Position
    Top, Right, Bottom, Left *Length
    Width, Height             *Length
    MinWidth, MinHeight       *Length
    MaxWidth, MaxHeight       *Length
    Margin, Padding           *Edges
    MarginAuto                *AutoEdges
    Gap                       *Pixels
    FlexGrow, FlexShrink      *float32
    FlexBasis                 *Length
    Justify                   *Justify
    AlignItems                *Align
    AlignSelf                 *Align
    Overflow                  *OverflowKind
    BorderWidths              *Edges
    BorderColor               *Rgba
    CornerRadii               *Corners
    Background                *Background
    BoxShadows                *[]BoxShadow
    Opacity                   *float32
    ScrollAxes                *Axes
    TextStyle                 *TextStyle
    TextSet                   TextFields // which TextStyle fields are explicit
}
func (r *StyleRefinement) Refine(base Style) Style

// TextFields is a bit set naming the TextStyle fields a refinement sets. The
// text builders (TextColor, TextSize, FontWeight, FontFamily, TextLineHeight,
// TextStrikethrough) set TextStyle's field and the matching bit. Refine copies
// only set fields onto base.TextStyle; TextStyle non-nil with TextSet == 0 is
// the legacy "replace the whole style" form.
type TextFields uint8
const (
    TextFieldFamily TextFields = 1 << iota
    TextFieldSize
    TextFieldWeight
    TextFieldColor
    TextFieldLineHeight
    TextFieldStrikethrough
)

// Text style cascade: TextStyle is inherited. During RequestLayout the window
// keeps a text-style stack; a Div pushes its TextSet fields refined over the
// current top (default: DefaultTextStyle()) for its children. Text, Str,
// StyledText and TextInput resolve own explicit fields over the inherited
// style; the result is stored in the element's LayoutNode.Style.TextStyle,
// which is what Measure and Paint use. StyledText runs whose FontSize is 0
// use the resolved style of the StyledText itself.

type TextStyle struct {
    FontFamily  string // "" => default family
    FontSize    Pixels // default 14
    FontWeight  int    // 400 normal, 700 bold
    Color       Rgba   // default black
    LineHeight  Pixels // 0 => 1.2 * FontSize
    Strikethrough bool
}

func DefaultTextStyle() TextStyle
```

`styled.go` provides the fluent builder. `Styled` is implemented by `*Div` and
`*Text`. Every method returns the receiver for chaining.

```go
type Styled interface {
    StyleRef() *StyleRefinement
}

// Every one of these exists on both *Div and *Text (via a shared embedded struct
// `StyledMixin` that holds a StyleRefinement and the methods):
func (m *StyledMixin) StyleRef() *StyleRefinement
func (m *StyledMixin) SizeFull() *T                 // w=100%,h=100%  (Px 100% not modeled: use Flex(1) w/ parent)
func (m *StyledMixin) SizeWH(w, h float32) *T
func (m *StyledMixin) SizeW(w float32) *T
func (m *StyledMixin) SizeH(h float32) *T
func (m *StyledMixin) FlexGrow(v float32) *T
func (m *StyledMixin) FlexShrink(v float32) *T
func (m *StyledMixin) Flex1() *T                    // GPUI flex_1: `flex: 1 1 0%` (grow 1, shrink 1, basis Px(0))
func (m *StyledMixin) FlexAuto() *T                 // `flex: 1 1 auto` (grow 1, shrink 1, basis Auto = content)
func (m *StyledMixin) FlexNone() *T                 // `flex: none` (grow 0, shrink 0, basis Auto)
func (m *StyledMixin) FlexBasis(v float32) *T       // definite flex-basis in px
func (m *StyledMixin) Flex() *T                     // display:flex, default DirectionColumn
func (m *StyledMixin) FlexRow() *T                  // Direction=Row
func (m *StyledMixin) FlexCol() *T                  // Direction=Column
func (m *StyledMixin) Absolute() *T
func (m *StyledMixin) Relative() *T // restores in-flow layout; offsets are ignored
func (m *StyledMixin) Top(v float32) *T
func (m *StyledMixin) Right(v float32) *T
func (m *StyledMixin) Bottom(v float32) *T
func (m *StyledMixin) Left(v float32) *T
func (m *StyledMixin) Inset(v float32) *T // all four offsets
func (m *StyledMixin) InsetX(v float32) *T // left and right
func (m *StyledMixin) InsetY(v float32) *T // top and bottom
func (m *StyledMixin) FlexWrap() *T
func (m *StyledMixin) FlexNoWrap() *T
func (m *StyledMixin) RowGap(v float32) *T
func (m *StyledMixin) ColumnGap(v float32) *T
func (m *StyledMixin) ContentStretch() *T / ContentStart / ContentCenter / ContentEnd / ContentBetween / ContentAround
func (m *StyledMixin) Gap(v float32) *T // resets RowGap and ColumnGap to Auto
func (m *StyledMixin) P(v float32) *T
func (m *StyledMixin) Px_(v float32) *T             // padding-left  (named to avoid clash with Px())
func (m *StyledMixin) Py_(v float32) *T
func (m *StyledMixin) Pt(v float32) *T
func (m *StyledMixin) Pr(v float32) *T
func (m *StyledMixin) Pb(v float32) *T
func (m *StyledMixin) M(v float32) *T
func (m *StyledMixin) Mx(v float32) *T / My / Mt / Mr / Mb / Ml   // per-edge margins (clear `auto` on those edges)
func (m *StyledMixin) MAuto() *T / MxAuto / MyAuto / MtAuto / MrAuto / MbAuto / MlAuto // `margin: auto` edges
func (m *StyledMixin) Bg(f Fill) *T                 // Rgba, Hsla or a Background (e.g. LinearGradient)
func (m *StyledMixin) Opacity(f float32) *T         // subtree opacity, see §3b
func (m *StyledMixin) Shadow(s ...BoxShadow) *T     // replaces the shadow list
func (m *StyledMixin) ShadowNone() *T
func (m *StyledMixin) Shadow2xs() *T / ShadowXs / ShadowSm / ShadowMd / ShadowLg / ShadowXl / Shadow2xl
func (m *StyledMixin) TextColor(c Rgba) *T
func (m *StyledMixin) TextSize(px float32) *T
func (m *StyledMixin) FontWeight(w int) *T
func (m *StyledMixin) TextStrikethrough() *T
func (m *StyledMixin) Rounded(px float32) *T
func (m *StyledMixin) RoundedAll(tl, tr, br, bl float32) *T
func (m *StyledMixin) Border(w float32, c Rgba) *T
func (m *StyledMixin) ItemsStart() *T
func (m *StyledMixin) ItemsCenter() *T
func (m *StyledMixin) ItemsEnd() *T
func (m *StyledMixin) ItemsStretch() *T
func (m *StyledMixin) JustifyStart() *T
func (m *StyledMixin) JustifyCenter() *T
func (m *StyledMixin) JustifyEnd() *T
func (m *StyledMixin) JustifyBetween() *T
func (m *StyledMixin) JustifyAround() *T
func (m *StyledMixin) OverflowHidden() *T
func (m *StyledMixin) OverflowScroll() *T          // scrolls both axes
func (m *StyledMixin) OverflowXScroll() *T         // Overflow=Scroll, ScrollAxes=AxisX
func (m *StyledMixin) OverflowYScroll() *T         // Overflow=Scroll, ScrollAxes=AxisY
func (m *StyledMixin) MinW(v float32) *T
func (m *StyledMixin) MinH(v float32) *T
func (m *StyledMixin) MaxW(v float32) *T
func (m *StyledMixin) MaxH(v float32) *T
```

The margin and flex-basis builders above that are not declared in
`div.go`/`text_elem.go` (`Mx`..`Ml`, `*Auto`, `FlexAuto`, `FlexNone`,
`FlexBasis`) are forwarded for `*Div` and `*Text` in `styled_layout.go`.

> **Note on `SizeFull()`.** CSS `100%` is not modeled. `SizeFull()` sets
> `Width=Flex(1)` and `Height=Flex(1)` when the parent is a flex container of the
> matching direction, and additionally sets `MinWidth/MinHeight` to `Px(0)` so the
> automatic minimum size does not block shrinking. Document this deviation.

Per-axis overflow follows CSS: when one axis scrolls the other is clipped
too (`visible` computes to `auto`), so a single `Overflow` kind drives
clipping and layout and `ScrollAxes` restricts which axes scroll: wheel
input and programmatic offsets (`ScrollHandle`, `Window.SetScrollOffset`)
are clamped to 0 on the other axis (`OverflowHidden` scrolls on neither). *Div, *Text, *UniformList and *List forward the common methods so a chain
keeps its concrete type.

## 3b. `background.go`, `shadow.go` — fills, gradients, shadows, opacity

```go
// Fill is anything usable as a background: Rgba, Hsla, Background.
type Fill interface{ ToBackground() Background }
func (c Rgba) ToBackground() Background
func (c Hsla) ToBackground() Background

type ColorSpace uint8
const (ColorSpaceSrgb ColorSpace = iota; ColorSpaceOklab)
func (c ColorSpace) String() string // "sRGB", "Oklab"

// ColorStop is GPUI's LinearColorStop (the type is renamed because Go
// cannot have a type and a function with one name).
type ColorStop struct {
    Color      Rgba
    Percentage float32 // 0..1 along the gradient line
}
func LinearColorStop(c Rgba, percentage float32) ColorStop

// Gradient is a two-stop linear gradient. Angle is in degrees with CSS
// semantics: 0 = towards the top, 90 = towards the right.
type Gradient struct {
    Angle float32
    Stops [2]ColorStop
    Space ColorSpace
}

type Background struct {
    Kind     BackgroundKind // None, Solid, LinearGradient
    Solid    Rgba
    Gradient Gradient
}
func SolidBackground(c Rgba) Background
func LinearGradient(angle float32, from, to ColorStop) Background
func (b Background) ColorSpace(cs ColorSpace) Background // builder (GPUI .color_space)
func (b Background) Opacity(f float32) Background          // multiplies every alpha
func (b Background) IsTransparent() bool
func (b Background) ToBackground() Background

type BoxShadow struct {
    Color        Rgba
    Offset       Point
    BlurRadius   Pixels // gaussian sigma = BlurRadius/2 (CSS)
    SpreadRadius Pixels
    Inset        bool
}
func NewBoxShadow(offsetX, offsetY float32, color Rgba) BoxShadow
func (s BoxShadow) WithBlur(r float32) BoxShadow
func (s BoxShadow) WithSpread(r float32) BoxShadow
func (s BoxShadow) AsInset() BoxShadow
```

Gradient geometry is GPUI's shader: direction `(cos(a-90°), sin(a-90°))`
with the short side expanded to the long one, `t` projected through the
quad's center and remapped by the stop percentages, then clamped;
interpolation of straight-alpha colors in sRGB or Oklab.

Shadow presets match GPUI (`styled.rs`, Tailwind v4): `2xs` (0,1) blur 0
a.05; `xs` (0,1) blur 2 a.05; `sm` (0,1) blur 3 a.1 + (0,1) blur 2 spread −1
a.1; `md` (0,4) blur 6 spread −1 + (0,2) blur 4 spread −2, a.1; `lg` (0,10)
blur 15 spread −3 + (0,4) blur 6 spread −4, a.1; `xl` (0,20) blur 25 spread
−5 + (0,8) blur 10 spread −6, a.1; `2xl` (0,25) blur 50 spread −12 a.25.

Div paint order: outset shadows, background, inset shadows, border, then the
children. `Opacity` is honoured by *Div, *UniformList, *Text and
*StyledText (elements whose prepaint state resolves a paint scope; a text
leaf returns one only when its opacity is below 1). Opacity is GPUI's element opacity: the window keeps a running
product of the ancestors' `Opacity` and multiplies the alpha of every
primitive painted inside the subtree (overlapping children are not
composited as a group, exactly as in GPUI). Opacity 1 changes nothing.

### Wrapped flex lines

FlexWrap enables wrapping; FlexNoWrap restores the default single-line layout.
These builders, RowGap, ColumnGap and ContentStretch/Start/Center/End/Between/Around
preserve Div and Text chains. Wrap-reverse is unsupported.

Gap keeps its original main-axis meaning for no-wrap containers. With Wrap it
also separates lines. RowGap overrides vertical spacing; ColumnGap overrides
horizontal spacing (both Length Auto by default, Px overrides; Flex acts as Auto).
Gap resets both overrides to Auto, so the last call setting an axis wins.
Cross-axis gaps and AlignContent are ignored in no-wrap containers.

Wrap greedily collects visible in-flow children by hypothetical outer main size
(including fixed margins; auto margins count as zero) plus main gap. Exact fits
stay on the line; an oversized item forms a singleton. The wrapping limit is a
definite main size, otherwise bounded available main space, further capped by
MaxWidth/MaxHeight. An indefinite main axis without a maximum forms one line.
Auto main size is the largest collected line's content contribution, capped by
the limit, with its own min/max applied. All lines resolve flex sizing against
that final main size, with independent grow/shrink, auto margins and Justify.
A wrapped row's automatic min-content width is the largest child's contribution.

Natural cross size is the sum of line cross sizes plus cross gaps. With a final
cross size, AlignContent distributes remaining space: Start, Center, End,
SpaceBetween, SpaceAround, or Stretch (default, adds positive free space equally
to lines). This applies even with one wrapped line. Items align/stretch within
each resulting line via AlignItems/AlignSelf and auto margins. Negative free
space is not stretched; SpaceBetween falls back to Start and SpaceAround to
Center. Absolute children do not form lines and anchor once to the final parent.

### Absolute positioning

`Absolute`, `Relative`, `Top`, `Right`, `Bottom`, `Left`, `Inset`, `InsetX`,
`InsetY` preserve fluent chains on Div and Text; other elements inherit mixin
methods. Position defaults to PositionRelative, meaning normal flex flow.
Relative offsets do not translate the box. PositionAbsolute children are
excluded from flex sizing, automatic minimums, gaps, alignment and justification.
They are arranged after their immediate parent's final size is known, relative
to its border box (padding does not shift their origin). They retain tree/scene
paint order, with no separate stacking context or z-index.

Offsets are logical pixels, default Auto (unset); negative values are allowed.
A LengthFlex offset acts as unset. On an axis, an explicit Px size wins;
otherwise two set edges stretch to the nonnegative remaining space, respecting
Min/Max. With fewer set edges the size is content-based, measured within the
remaining parent space. Flex lengths on absolute sizes act as Auto; flex grow,
shrink and basis have no effect. The leading edge wins when both edges and a
size are set; a sole trailing edge anchors the sized box there. With neither
edge set the origin is zero, not a CSS static position. Fixed margins inset the
margin box; auto margins count as zero. Absolute descendants can enlarge
positive scrollable overflow but never the parent's content-based size.
Percentages, relative translation and viewport/fixed positioning are unsupported.

## 4. `layout.go` — the flexbox engine

The engine is a plain function-based two-pass layout. It knows nothing about
elements, scenes or windows. Nodes are built by the element pipeline and
discarded after each frame.

```go
type Available struct {
    W, H      Pixels
    BoundedW  bool // false => W unbounded (max-content sizing)
    BoundedH  bool
}
func Definite(w, h float32) Available
func UnboundedHeight(availW float32) Available   // {W: availW, BoundedW:true, BoundedH:false}
func UnboundedWidth(availH float32) Available

type LayoutNode struct {
    Style    Style
    Children []*LayoutNode

    // Measure overrides leaf sizing. Only used when len(Children)==0.
    // maxWidth is the wrap width (math.MaxFloat32 for max-content);
    // maxWidth <= 0 requests the min-content size (text: the longest
    // unbreakable word, with the height of wrapping at that width).
    Measure func(text string, ts TextStyle, maxWidth float32) (w, h float32)
    Text    string // non-empty + Measure != nil => leaf sized by Measure

    // Outputs, written by Layout:
    Rect        Bounds // border box, parent-relative
    ContentSize Size   // scrollable extent from the padding-box origin (>= Rect size)
}

// Layout runs measure+arrange. available is the constraint for the root node.
func Layout(n *LayoutNode, available Available) Bounds

// ScrollMax returns max(0, ContentSize - Rect size) per axis.
func (n *LayoutNode) ScrollMax() (w, h Pixels)
```

Semantics (from the CSS flexbox spec + Taffy's `flexbox.rs`):

- By default one flex line, LTR; Wrap uses the wrapped-line rules in section 3.
  `Display:false` children are
  skipped entirely (zero rect, no gap). Sizes are border-box (padding
  included); leaves include their padding too. `Rect` is the border box;
  margins sit outside it.
- **Margins** (CSS semantics, no collapsing): an item's *outer* size is its
  border box plus its margins on that axis. Main-axis margins are fixed space
  subtracted (like gaps) before flexing and justification and are added to
  the container's content-based size and to min-content contributions;
  cross-axis margins reduce the space the item may stretch into and are added
  to the line's cross size. Negative margins are allowed. The root node's
  margins are ignored.
- **Auto margins** (`MarginAuto`): on the main axis, positive free space
  (after flexing) is split equally between all auto margins of the line
  before `justify-content`, which then sees no free space; with no positive
  free space they are 0. On the cross axis, an item with an auto cross margin
  does not stretch; the free cross space goes to its auto margin(s) (both:
  centered), overriding `align-self`; negative free space puts it at the
  start.
- **Rects are parent-relative**: a child's `Rect.Origin` is measured from its
  parent's border-box origin. The window adds the parent origins when it
  builds the render tree.
- Available space is a *maximum*, never a size. An `Auto` axis sizes to its
  content (fit-content: max-content capped by a bounded available length);
  only the parent can force a size (stretch, flex grow/shrink). The root is
  the exception: `Layout` forces the root to `available` on every bounded axis
  whose `Length` is not `LengthPx` (a window root fills the window).
- The effective main gap (Gap, overridden by RowGap/ColumnGap on its axis)
  is a fixed gutter: `gapsTotal = mainGap*(k-1)` per line, subtracted before
  `justify-content` sees free space, always re-added between items.
- Flex base size: `LengthFlex(g)` main length→base 0, grow g, shrink 1;
  otherwise a `Px` `FlexBasis` wins (`Flex1()` = basis `Px(0)`), else
  `LengthPx`→val, else
  `LengthAuto`→max-content main size, measured with an unbounded main axis and
  the cross size the item will get (the container's inner cross if it
  stretches and that is definite, else the cross space as a maximum).
- Automatic minimum size (`Min*=LengthAuto`, main axis only): **0 if the node
  is a scroll container** (`Overflow != Visible`); otherwise a definite
  `LengthPx` preferred size, else the min-content size, capped by a definite
  `Max*`. Min-content width: text = longest word (`Measure(..., 0)`), no-wrap row =
  sum of children's min-content widths + gaps, wrapped row and column = max,
  plus padding; absolute and hidden children are excluded;
  scroll-container children contribute 0 unless they have a `LengthPx` width.
  Min-content height = content height at the given width. So a wrapping text
  item shrinks down to its longest word without `MinW(0)`. The cross axis has
  no automatic minimum.
- Hypothetical main size = clamp(base, min, max), floored at 0.
- Resolve flexible lengths per CSS §9.7 (freeze inflexible items, iterate,
  fix min/max violations, freeze violators, repeat until all frozen) against
  each line's final inner main size. For the default no-wrap case that size is the definite main size if
  there is one; otherwise the sum of the items' max-content contributions
  (for a `LengthFlex` length or a `Px` basis smaller than the content: the
  clamped max-content size) + margins + gaps, capped by a bounded available
  main length.
- No-wrap cross size: if the container's cross is definite, `lineCross = innerCross`;
  else `lineCross = max_i(itemCross)` where each item's cross is measured at its
  final main size (content height, not the available height). `align-items:
  stretch` applies only on the cross axis and only when the child's cross
  `Length` is `LengthAuto`; it sets the child's cross size to `lineCross`
  (clamped). A `LengthFlex` cross length (e.g. `SizeFull`) fills `lineCross`
  regardless of alignment (the POC's `100%`). Wrapped lines first measure their
  natural cross sizes and then apply AlignContent as specified in section 3.
- `justify-content` offsets with the fallback: if `k<=1 || free<=0`, then
  `SpaceBetween→Start` and `SpaceAround→Center`. `SpaceBetween` adds
  `free/(k-1)` after every item but the last; `SpaceAround` starts at
  `free/k/2` and adds `free/k` between items.
- `Overflow` never changes descendant layout. It only affects (a) the automatic
  minimum size rule, (b) clipping, (c) content-size propagation.
- Scroll container: children are measured with an **unbounded main axis**
  (their base sizes are max-content). `ContentSize.main = max(usedMain,
  max_i(childEndAlongMain)+endPaddingMain)`, and the same on the cross axis.
  Non-scroll children with `OverflowVisible` propagate their own
  `ContentSize` upward.
- Text leaf: wrap width = the leaf's final content-box width once the parent
  has sized it; while measuring, the `LengthPx` width if set, else the bounded
  available width, else `math.MaxFloat32` (max-content). Text paints wrapped
  at its laid-out content width (`bounds.W - horizontal padding`), so paint and
  layout agree.

## 5. `element.go`, `div.go`, `text_elem.go`, `canvas.go`

`canvas.go` — the port of GPUI's `canvas()`: a leaf that paints through a
callback with its laid-out bounds. It embeds `StyledMixin` (SizeFull by
default) and does not clip.

```go
type Canvas struct{ StyledMixin /* paint callback */ }
func NewCanvas(paint func(bounds Bounds, w *Window, cx *App)) *Canvas
func (c *Canvas) IntoElement() Element
```

```go
// Element is the per-frame UI node. Everything is rebuilt each frame.
type Element interface {
    ID() (ElementID, bool)
    RequestLayout(w *Window, cx *App) (LayoutID, any)
    Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any
    Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any)
}

type IntoElement interface{ IntoElement() Element }

type ParentElement interface {
    Child(children ...IntoElement) ParentElement
    Children(children ...IntoElement) ParentElement
}

// ElementID identifies an element across frames. Stable ids keep element state
// (scroll offsets, hover) alive; missing ids fall back to the child index path.
type ElementID struct {
    Path []ElementPathKey
}
type ElementPathKey struct {
    Index  int
    Key    string // "" when keyed by index
}

// GlobalElementID is the fully-resolved id used as a map key for retained state.
type GlobalElementID struct {
    Path []ElementPathKey
}
func (a GlobalElementID) Equal(b GlobalElementID) bool

// LayoutID indexes into the Window's per-frame layout engine.
type LayoutID int

// AnyElement erases an Element for the children slice.
type AnyElement struct{ elem Element }
func NewAnyElement(e IntoElement) AnyElement
func (a AnyElement) Element() Element

// Normalize converts anything IntoElement into an Element.
func IntoElementOf(e IntoElement) Element

// ParentElementChildren is implemented by Elements that have child elements
// (custom containers outside the package). The window walks the returned
// children after the element itself, exactly as for a Div: each child is
// prepainted and painted with its own laid-out bounds. The element's
// RequestLayout must return the state from Window.RequestLayoutWithChildren
// (or a value embedding *ChildLayout) so the walk can pair children with
// their layout nodes.
type ParentElementChildren interface {
    Element
    ElementChildren() []AnyElement
}

// ChildLayout is the RequestLayout state produced by
// Window.RequestLayoutWithChildren: the node plus each child's LayoutID and
// RequestLayout state (in children order, nil children skipped).
type ChildLayout struct {
    Node *LayoutNode
    // unexported: child ids and states
}

// On *Window (window.go):
// RegisterLayoutNode registers a caller-built LayoutNode (leaf or with
// already-registered children) for this frame and returns its id.
func (w *Window) RegisterLayoutNode(n *LayoutNode) LayoutID
// LayoutNode returns the node registered under id this frame, or nil.
func (w *Window) LayoutNode(id LayoutID) *LayoutNode
// RequestLayoutWithChildren runs RequestLayout on every child, builds a
// node with style and those children, registers it and returns its id plus
// the *ChildLayout state to return from the element's RequestLayout. The
// children inherit the enclosing text style overridden by the fields of
// style.TextStyle that are non-zero and differ from DefaultTextStyle (so a
// DefaultStyle() container inherits everything).
func (w *Window) RequestLayoutWithChildren(style Style, children []AnyElement, cx *App) (LayoutID, *ChildLayout)
```

### `div.go`

```go
func Div() *Div

type Div struct {
    StyledMixin
    id         string
    children   []AnyElement
    // listeners (all optional, nil = none)
    onMouseDown  func(*MouseDownEvent, *Window, *App)
    onMouseUp    func(*MouseUpEvent, *Window, *App)
    onMouseMove  func(*MouseMoveEvent, *Window, *App)
    onClick      func(*ClickEvent, *Window, *App)
    onScrollWheel func(*ScrollWheelEvent, *Window, *App)
    onPinch      func(*PinchEvent, *Window, *App)
    onFileDrop   func(*FileDropEvent, *Window, *App)
    onImagePaste func(*ImagePasteEvent, *Window, *App)
    onKeyDown    func(*KeyDownEvent, *Window, *App)
    onKeyUp      func(*KeyUpEvent, *Window, *App)
    onHoverStart func(*Window, *App)
    onHoverEnd   func(*Window, *App)
    tooltip      string
    onAction     map[ActionKey]func(Action, *Window, *App)
    focusHandle  *FocusHandle
    keyContext   string
    tracksFocus  bool
    tabIndex     int
}

func (d *Div) ID(id string) *Div
func (d *Div) Child(children ...IntoElement) *Div
func (d *Div) Children(children ...IntoElement) *Div
func (d *Div) OnMouseDown(f func(*MouseDownEvent, *Window, *App)) *Div
func (d *Div) OnMouseUp(f func(*MouseUpEvent, *Window, *App)) *Div
func (d *Div) OnMouseMove(f func(*MouseMoveEvent, *Window, *App)) *Div
func (d *Div) OnClick(f func(*ClickEvent, *Window, *App)) *Div
func (d *Div) OnScrollWheel(f func(*ScrollWheelEvent, *Window, *App)) *Div
func (d *Div) OnPinch(f func(*PinchEvent, *Window, *App)) *Div
func (d *Div) OnFileDrop(f func(*FileDropEvent, *Window, *App)) *Div
func (d *Div) OnImagePaste(f func(*ImagePasteEvent, *Window, *App)) *Div // focus path, like OnKeyDown
func (d *Div) OnKeyDown(f func(*KeyDownEvent, *Window, *App)) *Div
func (d *Div) OnKeyUp(f func(*KeyUpEvent, *Window, *App)) *Div
func (d *Div) OnHoverStart(f func(*Window, *App)) *Div
func (d *Div) OnHoverEnd(f func(*Window, *App)) *Div
// Tooltip shows text in a bubble once the pointer has rested on the div for
// TooltipStyle.Delay (see "Tooltips" below). Empty text removes it.
func (d *Div) Tooltip(text string) *Div
func (d *Div) OnAction(action Action, f func(Action, *Window, *App)) *Div // keyed by ActionKeyOf(action)
func (d *Div) TrackFocus(h *FocusHandle) *Div
func (d *Div) KeyContext(ctx string) *Div
func (d *Div) TabIndex(i int) *Div   // also makes the div a tab stop
func (d *Div) TabStop(b bool) *Div
// Interaction style refinements (GPUI .hover/.focus/.focus_visible/.active). f gets
// an empty StyledMixin and returns it; the set fields override the base
// style at paint. Applied in the order base, Focus, FocusVisible, Hover, Active.
// They are paint-time only: layout fields set in them are ignored. Their
// TextColor/TextStrikethrough reach descendant Text/StyledText through a
// paint-time text-override stack (fields a text element, or a nearer
// Div/UniformList, sets itself win); layout-affecting text fields (size,
// weight, family, line height) are ignored for descendants.
func (d *Div) Active(f func(*StyledMixin) *StyledMixin) *Div
func (d *Div) Hover(f func(*StyledMixin) *StyledMixin) *Div
func (d *Div) Focus(f func(*StyledMixin) *StyledMixin) *Div
func (d *Div) FocusVisible(f func(*StyledMixin) *StyledMixin) *Div
// TrackScroll binds a ScrollHandle to this scroll container.
func (d *Div) TrackScroll(h *ScrollHandle) *Div
// ShowScrollbar opts a scroll container into an overlay scrollbar (GPUI
// divs paint none; Zed's ui crate adds them).
func (d *Div) ShowScrollbar() *Div
```

Focus semantics:

- A div with `TrackFocus(h)` is focusable: a mouse-down on it (or on a
  descendant that is not itself focusable) focuses it — the innermost
  focusable element on the hit path wins, after that element's
  `OnMouseDown` ran, unless propagation was stopped before reaching it or
  an `OnMouseDown` handler on the path already moved focus (an explicit
  focus change in a handler is never overridden).
- `TabIndex(i)` without `TrackFocus` gives the div an implicit focus handle,
  retained per GlobalElementID (GPUI's stateful-div focus handle).
- Tab stops: the effective tab index is the div's `TabIndex` if set, else
  the handle's; the div is a stop if `TabIndex`/`TabStop(true)` was called or
  the handle has `SetTabStop(true)`.
- Hover style is resolved at paint from the hit test of the frame being
  built at the last pointer position; a hover-styled element entering or
  leaving the hover set triggers a redraw.

Tooltips (`tooltip.go`; GPUI's `.tooltip(|window, cx| Tooltip::text(..))`,
reduced to plain text):

```go
type TooltipStyle struct {
    Background, Border, Foreground Rgba
    BorderWidth, FontSize, PaddingX, PaddingY, CornerRadius float32
    MaxWidth float32        // 0 = window width only
    Delay    time.Duration // hover delay
    Offset   float32       // gap below the pointer
}
func DefaultTooltipStyle() TooltipStyle      // dark; Delay 500ms
func (a *App) SetTooltipStyle(s TooltipStyle) // refreshes all windows
func (a *App) TooltipStyle() TooltipStyle
```

- The tooltip target is the innermost element with `Tooltip` on the hit path
  under the pointer. A pointer move on a new target starts a `Window.After(Delay)`
  timer; a move within the same pending target restarts it ("rested"). When it
  fires the bubble becomes visible at the pointer position of that moment.
  Time comes from the app clock, so headless tests use
  `HeadlessPlatform.Advance`.
- A visible bubble stays while the pointer stays on the target (it does not
  follow the pointer). It hides on leaving the target, mouse down (any button),
  scroll wheel and key down; after a press/scroll/key it does not return until
  the pointer has left the target.
- It is painted after the whole tree (above everything, regardless of tree
  order), registers no hitboxes or key nodes, so it never takes input. Placed
  `Offset` below the pointer with its left edge at the pointer, flipped above
  the pointer if it would overflow the bottom, then clamped inside
  `Window.Bounds()`; its width is capped at min(`MaxWidth`, window width).
- Text wraps at that cap and the bubble is sized to the wrapped height (every
  line is inside the bubble); `"\n"` in the text is a hard line break.
- Style values are logical pixels inside the tree, so `Window.UIScale` applies.
  Text uses the default font family; the style has no font family field.

Tooltips (`tooltip.go`; GPUI's `.tooltip(|window, cx| Tooltip::text(..))`,
reduced to plain text):

```go
type TooltipStyle struct {
    Background, Border, Foreground Rgba
    BorderWidth, FontSize, PaddingX, PaddingY, CornerRadius float32
    MaxWidth float32        // 0 = window width only
    Delay    time.Duration // hover delay
    Offset   float32       // gap below the pointer
}
func DefaultTooltipStyle() TooltipStyle      // dark; Delay 500ms
func (a *App) SetTooltipStyle(s TooltipStyle) // refreshes all windows
func (a *App) TooltipStyle() TooltipStyle
```

- The tooltip target is the innermost element with `Tooltip` on the hit path
  under the pointer. A pointer move on a new target starts a `Window.After(Delay)`
  timer; a move within the same pending target restarts it ("rested"). When it
  fires the bubble becomes visible at the pointer position of that moment.
  Time comes from the app clock, so headless tests use
  `HeadlessPlatform.Advance`.
- A visible bubble stays while the pointer stays on the target (it does not
  follow the pointer). It hides on leaving the target, mouse down (any button),
  scroll wheel and key down; after a press/scroll/key it does not return until
  the pointer has left the target.
- It is painted after the whole tree (above everything, regardless of tree
  order), registers no hitboxes or key nodes, so it never takes input. Placed
  `Offset` below the pointer with its left edge at the pointer, flipped above
  the pointer if it would overflow the bottom, then clamped inside
  `Window.Bounds()`; its width is capped at min(`MaxWidth`, window width).
- Text wraps at that cap and the bubble is sized to the wrapped height (every
  line is inside the bubble); `"\n"` in the text is a hard line break.
- Style values are logical pixels inside the tree, so `Window.UIScale` applies.
  Text uses the default font family; the style has no font family field.

### `scroll.go`

```go
// ScrollHandle exposes and controls a scroll container's offset (GPUI's
// ScrollHandle / UniformListScrollHandle). Offsets are POSITIVE distances
// scrolled (GPUI's are negative).
type ScrollHandle struct{ /* shared state */ }
func NewScrollHandle() *ScrollHandle
func (h *ScrollHandle) Offset() Point
func (h *ScrollHandle) SetOffset(p Point)      // clamped at the next frame; caller refreshes
func (h *ScrollHandle) MaxOffset() Size        // as of the last frame
func (h *ScrollHandle) Bounds() Bounds         // container border box, last frame
func (h *ScrollHandle) ScrollToItem(ix int)    // child ix (Div) / item ix (UniformList, List), next frame
func (h *ScrollHandle) ScrollToTop()
func (h *ScrollHandle) ScrollToBottom()
```

Pinch semantics: `OnPinch` listeners run along the hit path at the event
position, innermost first, until one calls `Window.StopPropagation()`. A pinch
never scrolls containers.

File drop semantics: `OnFileDrop` listeners run along the hit path at the drop
position, innermost first, until one calls `Window.StopPropagation()`, as
pinch does. A drop no listener takes is ignored.

Image paste semantics: `OnImagePaste` listeners run along the focus path (the
focused element and its ancestors, as key events), innermost first, until one
calls `Window.StopPropagation()`; with nothing focused the path is the root
key node, as for keys. A paste no listener takes is ignored.

Scroll semantics: `OnScrollWheel` listeners run first (bubble, innermost
first); if one calls `Window.StopPropagation()` no container scrolls. Then
wheel deltas go to scroll containers on the hit path,
innermost first. A container takes the components on its `ScrollAxes`
(a container scrolling X only maps `DeltaY` to X when `DeltaX == 0`, as
GPUI does); if its offset changed the event stops, else it bubbles to the
next container. Offsets are clamped to `[0, ScrollMax]` before the frame
uses them. Hitboxes are clipped to the enclosing scroll/clip containers, so
scrolled-out children are not hit. With `ShowScrollbar()`, a scroll
container with overflow on a scrolling axis paints a thin overlay thumb
(6px, 2px inset, gray α≈0.55) after its children.

Scrollbar tracks share the painted thumb geometry, including the 20px minimum
thumb and shortened tracks when both axes overflow. Hitboxes cover the 6px
track strips; they are registered after container descendants, under later
siblings, and clipped to the container and enclosing rectangular clips. No
overflow or a nonpositive track length produces no scrollbar input region.

Primary thumb press captures the container GlobalElementID and axis. Movement
changes offset proportionally by pointer delta times scroll maximum divided by
thumb travel, clamped to the current extent, even outside the container. Primary
release ends capture anywhere; secondary release does not. Pressing the track
outside the thumb pages one viewport toward the pointer, with no held-repeat.
Small tracks whose thumb fills the track have no drag travel.

The consumed primary gesture does not dispatch content mouse handlers, focus
content, or start Active styles. Its immediately following matching ClickEvent
is suppressed; another input event/new press clears stale click suppression.
Scrollbar input updates retained scroll state and the attached ScrollHandle
and redraws automatically. Stable explicit container IDs preserve capture
across redraws; capture cancels if the owner/scrollbar disappears, its track
is fully clipped, geometry/max changes, or programmatic scrolling changes the
tracked offset. Geometry changes cancel rather than retarget the drag. Div
and UniformList share this behavior; no generic drag/drop API is introduced.
A `List` is the exception: its extent moves while items are measured, so with
an unchanged track its drag re-bases (pointer, offset, maximum) instead of
cancelling.

### Pressed interaction style

`Div.Active(f)` refines paint while the primary (MouseLeft) button is held
inside the element. Primary down captures Active-enabled elements on the
current topmost hit path, including ancestors; occluded siblings do not activate.
This state is independent of mouse-handler bubbling/StopPropagation and does
not require an OnClick or OnMouseDown handler. Primary up clears all captured
state even outside the target or when a handler stops propagation. Secondary
buttons do not start or end primary active state, but their reported pointer
position can still cancel a press that has left the hit path.

Keys are GlobalElementIDs across redraws; stable explicit IDs are recommended.
Movement or redraw cancels a captured element when it is no longer on the hit
path, hidden, removed or no longer has Active. Re-entering does not restore it;
a new primary down is required. Nested ancestors can remain active when the
pointer leaves only the child. Reconciliation uses the new frame before paint
so geometry/occlusion changes cannot paint stale pressed state.

Interaction precedence is base, Focus, FocusVisible, Hover, then Active.
Active is paint-only like other interaction refinements: layout fields and
text metrics are ignored; colour/strikethrough reach descendant text and
opacity multiplies subtree painting. Press/release/cancellation automatically
request redraws. Active(nil) clears the refinement.

### `uniform_list.go`

```go
// UniformList is a virtualized, vertically scrolling list of count rows of
// equal height. The row height is measured each frame by laying out item 0
// at the list's content width; only the rows intersecting the viewport are
// rendered (render is called once per frame with [start, end)).
type UniformList struct {
    StyledMixin
    // unexported
}
func NewUniformList(id string, count int, render func(start, end int, w *Window, cx *App) []IntoElement) *UniformList
func (u *UniformList) TrackScroll(h *ScrollHandle) *UniformList
func (u *UniformList) ShowScrollbar() *UniformList
func (u *UniformList) IntoElement() Element
```

*UniformList forwards SizeFull, SizeWH, SizeW, SizeH, Flex1, MinH, MaxH, P,
Px_, Py_, Bg, Rounded, Border, Opacity, TextColor to keep the chain typed.
Rows get GlobalElementIDs keyed by their absolute index (stable while
scrolling) and are laid out with `Definite(contentWidth, rowHeight)`. With an
`Auto` height the list sizes to `count * rowHeight` (+ padding). Overflow is
Scroll on Y; padding is honoured. Item 0's measurement and the rows inherit
the text style enclosing the list, refined by the list's own text fields.

### `list.go`

```go
// ListState is the retained state of one virtualized List (GPUI's
// ListState): the measured height of each item at the list's content
// width, the scroll anchor and the item keys. The view owns it across
// frames; one state backs one list.
type ListState struct{ /* unexported */ }
func NewListState() *ListState
func (s *ListState) Invalidate(ix int)                  // forget item ix's height; re-measured when next built
func (s *ListState) InvalidateAll()                     // forget every height (font or theme change); keeps the anchor
func (s *ListState) MeasuredHeight(ix int) (Pixels, bool) // border-box height, ok false while unmeasured
func (s *ListState) ContentHeight() Pixels              // items + gaps + padding as of the last frame, estimates included
func (s *ListState) VisibleRange() (start, end int)     // items built in the last frame, [start, end)

// List is a virtualized, vertically scrolling list of items of varying
// height (GPUI's list). Each frame only the items intersecting the
// viewport (plus the overdraw margin) are rendered, laid out, prepainted
// and painted; every other item is represented by its cached height, or by
// the estimate while it was never measured.
type List struct {
    StyledMixin
    // unexported
}
func NewList(id string, state *ListState, count int, render func(ix int, w *Window, cx *App) IntoElement) *List
func (l *List) ItemKey(key func(ix int) uint64) *List   // a changed key drops ix's cached height
func (l *List) EstimatedItemHeight(px float32) *List    // estimate before any item is measured (default 24)
func (l *List) Overdraw(px float32) *List               // extra build margin above and below the viewport (default 100)
func (l *List) TrackScroll(h *ScrollHandle) *List
func (l *List) ShowScrollbar() *List
func (l *List) IntoElement() Element
```

*List forwards Gap, SizeFull, SizeWH, SizeW, SizeH, Flex1, MinH, MaxH, P,
Px_, Py_, Bg, Rounded, Border, Opacity, TextColor to keep the chain typed.

Measurement and caching:

- An item is measured by building it (`render(ix)`), running its
  `RequestLayout` and laying it out with `UnboundedHeight(contentWidth)`; its
  border-box height is cached under its index. Every built item is
  re-measured, so a visible item whose content changed corrects itself in the
  same frame.
- The cache belongs to one content width: a width change drops every height.
  A count change keeps the heights of the indices that remain and adds
  unmeasured ones. With `ItemKey`, an index whose key differs from the key it
  was measured with is unmeasured again. `Invalidate`/`InvalidateAll` drop
  heights explicitly.
- An unmeasured item counts as the (lower) median of the measured heights at this
  width, or `EstimatedItemHeight` while none is measured. The content height
  is padding + item heights + `Gap` between items; it sets the scroll extent
  and, with an `Auto` height, the list's own height. The list has no
  intrinsic width: give it one or let the parent stretch it.
- `render` returning nil makes an empty, zero-height item (gaps still apply).
- An item's own margins are not applied: the item is laid out as a root at
  the content width. Wrap it in a `Div` with padding to inset it.
- With more than 32 measured items the median is refreshed every 16 height
  changes rather than after each, so the estimate may lag slightly.

Scrolling (Y only; the `ScrollHandle` contract above holds):

- The list keeps its position anchored to the first item at the top of the
  viewport and the distance into it, so items above the viewport that change
  height (measured for the first time, invalidated, re-measured) do not move
  the visible content. Wheel, scrollbar and `SetOffset` deltas since the last
  frame are applied on top of the anchor. A frame with no items drops the
  anchor; the next frame starts from the stored offset as it is.
- `ScrollToBottom` reaches the true end even through unmeasured items: the
  list measures the items the new offset shows and resolves the request again
  until the heights stop changing (a few passes at most). `ScrollToTop` and
  `ScrollToItem(ix)` resolve the same way, `ScrollToItem` with the
  minimum-move rule of a Div child.
- Items get GlobalElementIDs keyed by their absolute index (or their own id),
  as UniformList rows, so hover, clicks, tooltips and element state follow an
  item while it is built. An item outside the built range has no element
  state, hitbox or focus.
- The scrollbar thumb reflects the estimated content height. A thumb drag on a
  List survives extent changes caused by measuring: instead of cancelling,
  the drag re-bases on the current pointer and offset (Div and UniformList
  drags still cancel, I1).
- Items inherit the text style enclosing the list, refined by the list's own
  text fields, as UniformList rows do.

### `text_elem.go`

```go
// Text is a leaf that renders a single run of styled text.
type Text struct {
    StyledMixin
    text string
}
// NewText is named NewText rather than Text because Go does not allow a
// type and a package-level function to share a name.
func NewText(s string) *Text
func (t *Text) Text(s string) *Text

// StyledText renders multiple runs of differing styles within one paragraph.
type StyledText struct {
    StyledMixin
    runs []TextRun
}
type TextRun struct {
    Text  string
    Style TextStyle
}
func NewStyledText(runs ...TextRun) *StyledText
```

Both implement `Element`. `Text` sizes itself by calling the window's text
shaper (`w.shapeLine` / `w.measureText`) with the wrap width supplied by the
layout constraint. Painting emits `GlyphSprite`s into the scene. Lines
advance by `ResolvedLineHeight()`; each line's glyph box (ascent + descent)
is centred in its line box (half-leading), so the baseline is
`top + (lineHeight - (ascent + descent))/2 + ascent`. `Opacity` below 1
makes Prepaint return a paint scope carrying it.

Also provide `func (s String) IntoElement() Element` equivalent: a helper
`func Str(s string) IntoElement` returning a `*Text` so `Child(Str("hi"))` works.

## 6. `scene.go`, `raster.go`, `bounds.go`

```go
// ContentMask is the clip applied to a primitive: a rect, optionally with
// rounded corners (from an OverflowHidden/Scroll container with Rounded).
// Zero CornerRadii = plain rect clip; infinite = whole window.
type ContentMask struct {
    Bounds      Bounds
    CornerRadii Corners
}

type BackgroundKind uint8
const (
    BackgroundNone BackgroundKind = iota
    BackgroundSolid
    BackgroundLinearGradient
)

type Quad struct {
    Order         uint32
    Bounds        Bounds
    ContentMask   ContentMask
    Background    BackgroundKind
    Fill          Rgba
    BorderColor   Rgba
    BorderWidths  Edges
    CornerRadii   Corners
    Gradient      Gradient // used when Background == BackgroundLinearGradient
}

// Shadow is a blurred rounded rect. Outset: Bounds/CornerRadii are the
// shadow shape (element bounds offset and grown by spread). Inset: the
// shadow fills Clip/ClipRadii (the element shape) minus the blurred
// Bounds/CornerRadii hole.
type Shadow struct {
    Order       uint32
    Bounds      Bounds
    CornerRadii Corners
    ContentMask ContentMask
    Color       Rgba
    BlurRadius  Pixels
    Inset       bool
    Clip        Bounds
    ClipRadii   Corners
}

type GlyphSprite struct {
    Order       uint32
    Bounds      Bounds      // destination rect in window space
    ContentMask ContentMask
    Mask        *GlyphMask  // grayscale alpha mask
    Color       Rgba
    // Image, when set, is a colour picture drawn instead of Mask: its pixels
    // are scaled into Bounds (bilinear, 1:1 when the sizes match) and Color's
    // alpha multiplies them (opaque white draws the image as is).
    Image       *image.NRGBA
}

type GlyphMask struct {
    Pix  []uint8 // 1 byte per pixel, alpha only, row-major, Stride = W
    W, H int
    // Ink bounds in font pixels (y-up) relative to the pen position. The
    // mask covers exactly this rect expanded by one pixel on every side,
    // screen-oriented: row 0 is the top of the ink. GlyphSprite.Bounds is
    // derived from these at paint time.
    MinX, MinY, MaxX, MaxY float32 // logical px (device ink / Scale)
    // Scale is the device scale the mask was rasterized at (W, H and Pix are
    // in device px); 0 means 1. X0, Y0 are the mask's top-left in device px
    // relative to the pen position / baseline (y-down), valid when Scale > 0.
    Scale  float32
    X0, Y0 int
}

type Underline struct {
    Order       uint32
    Bounds      Bounds
    ContentMask ContentMask
    Color       Rgba
    Thickness   Pixels
}

// Stroke is an anti-aliased constant-width polyline with round joins and
// caps. Coverage is the union of the segment capsules (max per pixel), so
// joints and self-crossings blend once. Width < 1 device px draws a 1 device
// px line with alpha scaled by the width; Width 0 is a full-alpha hairline.
// Points are window-space logical px, read at raster time (do not mutate
// them before the frame is presented).
type Stroke struct {
    Order       uint32
    ContentMask ContentMask
    Points      []Point
    Closed      bool
    Width       Pixels
    Color       Rgba
}

type Scene struct {
    Quads   []Quad
    Shadows []Shadow
    Sprites []GlyphSprite
    Underlines []Underline
    Strokes []Stroke
}
func (s *Scene) InsertShadow(sh Shadow)
func (s *Scene) InsertStroke(st Stroke)

func (s *Scene) Clear()
func (s *Scene) InsertQuad(q Quad)
func (s *Scene) InsertSprite(g GlyphSprite)
func (s *Scene) InsertUnderline(u Underline)
// PushLayer records a z-order boundary; all subsequently inserted primitives
// get their Order bumped. Called during prepaint for stacking contexts.
func (s *Scene) PushLayer()
```

### `raster.go`

```go
// Rasterizer paints a Scene into an RGBA buffer.
type Rasterizer struct {
    pix   []byte // premultiplied RGBA, 4*W*H
    w, h  int
}
func NewRasterizer(w, h int) *Rasterizer
func (r *Rasterizer) Size() (w, h int)
func (r *Rasterizer) Pixels() []byte            // premultiplied, tightly packed
func (r *Rasterizer) Reset(clear Rgba)          // fill with premultiplied clear color
func (r *Rasterizer) Render(s *Scene)           // draw all primitives in Order
func (r *Rasterizer) Image() *image.NRGBA       // non-premultiplied copy (for png.Encode)
```

`Render` draws every primitive in scene order: quads, shadows, strokes,
underlines and sprites are merged by `Order` (each kind stably sorted; on equal
`Order`, shadow < quad < stroke < underline < sprite). Since Paint* hand out
increasing auto orders and `PushLayer` jumps the counter, a later
background (an overlay, a layer) covers earlier text, as in GPUI. Shadow coverage is an analytic approximation:
`min(Φ(−sdf/σ), separable box blur)` with σ = BlurRadius/2 (exact for
square corners, close for rounded ones), Φ from a lookup table, the pixel
loop bounded by the shape grown by 3σ. Gradient fills use a 256-entry
premultiplied color table per quad.

Clipping: each primitive is intersected with its `ContentMask.Bounds` and drawn
with that clip rect; when `ContentMask.CornerRadii` is non-zero every
pixel's coverage is also multiplied by the anti-aliased rounded-rect mask
(evaluated only in the corner squares, so the pixel loops stay bounded by
the primitive and the clip rect). Corner radii are drawn by an anti-aliased rounded-rect
mask; borders are drawn as four strips (or as a rounded-rect stroke). Text is
drawn by alpha-blending the `GlyphMask` into `GlyphSprite.Bounds` with
bilinear sampling; when the device-space bounds size equals the mask size
(masks rasterized at the window scale) the mask is blitted 1:1, snapped to
whole device pixels, so text stays crisp on HiDPI. A sprite with `Image` is
drawn the same way from the image's premultiplied colours instead of a tinted
mask.

`raster.go` may import `image`, `image/draw`, `image/color`, `math`, but **not**
`golang.org/x/*` or ebiten.

### `bounds.go`

```go
// Hitbox is a hit-testable region registered during prepaint.
type Hitbox struct {
    Bounds  Bounds
    Order   uint32 // paint order; higher = later = on top
    // listeners index into the frame's listener arrays (nil = none)
    hasMouseListeners bool
}

type BoundsTree struct {
    boxes []Hitbox
}
func (t *BoundsTree) Clear()
func (t *BoundsTree) Insert(h Hitbox) int
// HitTest returns the index of the deepest, topmost box containing p, or -1.
func (t *BoundsTree) HitTest(p Point) int
func (t *BoundsTree) Len() int
```

Hit test order: among boxes containing `p`, prefer the one with the highest
`Order`; ties broken by the later insert.

## 7. `events.go`

```go
type DispatchPhase uint8
const (
    Capture DispatchPhase = iota
    Bubble
)

type Modifiers uint8
const (
    Shift   Modifiers = 1 << iota
    Control
    Alt
    Platform // GPUI convention: Cmd on macOS, Super/Win (ebiten KeyMeta) on
             // Windows/Linux. Physical Ctrl is always Control.
    Function
)

type MouseButton uint8
const (
    MouseLeft MouseButton = iota
    MouseMiddle
    MouseRight
)

type MouseDownEvent struct {
    Position Point
    Button   MouseButton
    Modifiers Modifiers
    ClickCount int
}
type MouseUpEvent struct {
    Position Point
    Button   MouseButton
    Modifiers Modifiers
    ClickCount int
}
type MouseMoveEvent struct {
    Position Point
    Modifiers Modifiers
}
type ScrollWheelEvent struct {
    Position Point
    DeltaX, DeltaY Pixels
    Modifiers Modifiers
    // Precise is true when the deltas are pixel-precise (a trackpad or another
    // smooth-scroll device) and false when they are discrete line/notch steps
    // (a mouse wheel). Backends that cannot tell leave it false.
    Precise bool
}
// PinchEvent is one step of a trackpad pinch (magnify) gesture at Position.
// Delta is the magnification as a fraction: +0.1 is 10% bigger (zoom in);
// the step's scale factor is 1+Delta. Sent only by backends that can see the
// gesture (macOS in the ebiten backend); a browser pinch is a Ctrl+wheel
// ScrollWheelEvent instead. DispatchEvent divides Position by the UI scale.
type PinchEvent struct {
    Position Point
    Delta float32
    Modifiers Modifiers
}
// FileDropEvent is files or folders dragged from the OS and dropped on the
// window at Position. Paths are absolute paths in the real file system,
// sorted by name; a backend that cannot see real paths (a browser) sends no
// event. DispatchEvent divides Position by the UI scale.
type FileDropEvent struct {
    Position Point
    Paths []string
    Modifiers Modifiers
}
// ImagePasteEvent is an image the user pasted (the paste shortcut while the
// clipboard holds no text but an image). Data is the encoded file and MIME its
// type (image/png in practice). It goes to the focused element like a key.
type ImagePasteEvent struct {
    Data []byte
    MIME string
}
type ClickEvent struct {
    Up *MouseUpEvent
}
type KeyDownEvent struct {
    Keystroke Keystroke
    // PreferCharacterInput is true for events that should go to a focused text
    // input rather than to keybindings (i.e. printable chars).
    PreferCharacterInput bool
}
type KeyUpEvent struct {
    Keystroke Keystroke
}
type ModifiersChangedEvent struct{ Modifiers Modifiers }

// Keystroke mirrors gpui's platform/keystroke.rs.
// Key is a lower-case symbolic name: "enter", "escape", "backspace", "a", "1",
// "left", "up", "space", "tab", "delete", ...
// KeyChar holds the printable character when the key produces one.
type Keystroke struct {
    Modifiers Modifiers
    Key       string
    KeyChar   *string
}
func ParseKeystroke(s string) (Keystroke, error) // "cmd-shift-p", "enter", "ctrl-a"
func (k Keystroke) String() string
func (k Keystroke) Equals(o Keystroke) bool // exact: modifiers, key and KeyChar
// Matches reports whether typed satisfies this (binding) keystroke, as GPUI's
// should_match: equal modifiers and either the same key (case-insensitive,
// KeyChar ignored) or typed.KeyChar equal to this key (so a "?" binding
// matches a typed "?" whatever the physical key).
func (k Keystroke) Matches(typed Keystroke) bool

// PlatformInput is any of the events above; the dispatcher type-switches.
type PlatformInput interface{ isPlatformInput() }
```

All event structs get `func (...) isPlatformInput() {}`.

## 8. `keymap.go`

```go
// Action is any named, cloneable, comparable command.
type Action interface {
    Name() string
    Clone() Action
    Equals(other Action) bool
}

// NewAction builds a simple named action (a value type for the POC).
func NewAction(name string) Action

type KeyBinding struct {
    Action    Action
    Keystrokes []Keystroke
    Context   string // "" => matches any context; supports "&&", "||", "!" of
                     // simple key=value tokens and bare keys
}

func NewKeyBinding(keystrokeStr string, action Action, context string) KeyBinding

type Keymap struct {
    bindings []KeyBinding
}
func NewKeymap() *Keymap
func (k *Keymap) AddBindings(bindings ...KeyBinding)
// MatchKeystroke finds the binding whose last keystroke Matches typed and whose
// context predicate is satisfied by any entry in ctxStack. Returns nil if none.
func (k *Keymap) MatchKeystroke(typed Keystroke, ctxStack []string) Action

// ActionKey is the identity OnAction handlers are registered under: the
// action's concrete type (GPUI's TypeId) plus, only for NewAction values,
// the name. NewAction("a") and NewAction("b") are therefore distinct, while
// all values of a user-defined action type share one key (a zero value may
// register the handler).
type ActionKey struct {
    Type reflect.Type
    Name string
}
func ActionKeyOf(a Action) ActionKey
```

Key-down dispatch order (window.go):

1. If the event is character input (`PreferCharacterInput` with a `KeyChar`)
   and the focused input handler accepts text, the keymap is consulted
   first; when a binding matches and an `OnAction` handler on the focus path
   ran, dispatch ends. Otherwise the text goes to the input handler.
2. Otherwise `OnKeyDown` bubbles over the focus path, then (unless stopped)
   the keymap is matched and the action dispatched along the same path.

```go

// KeyContext is a predicate. "editor && !readonly" is parsed into tokens.
type keyContextPredicate struct{ /* ... */ }
func parseKeyContext(s string) *keyContextPredicate
func (p *keyContextPredicate) matches(ctxStack []string) bool
```

`App` owns the keymap: `cx.Keymap().AddBindings(...)`, `cx.Keymap()` returns
`*Keymap`.

## 9. `focus.go`

```go
type FocusID uint64

// FocusHandle is a refcounted, copyable handle to a focusable element.
// The zero value is invalid; obtain one from App.NewFocusHandle.
type FocusHandle struct {
    id       FocusID
    app      *App
    tabIndex int
    tabStop  bool
}

func (h *FocusHandle) ID() FocusID
func (h *FocusHandle) IsValid() bool
func (h *FocusHandle) Focus(w *Window)
func (h *FocusHandle) Blur(w *Window)
func (h *FocusHandle) IsFocused(w *Window) bool
func (h *FocusHandle) ContainsFocused(w *Window) bool
func (h *FocusHandle) SetTabIndex(i int) *FocusHandle // chainable (GPUI .tab_index)
func (h *FocusHandle) SetTabStop(b bool) *FocusHandle  // chainable (GPUI .tab_stop)
func (h *FocusHandle) TabIndex() int
func (h *FocusHandle) TabStop() bool
// IsFocusVisible reports IsFocused && the window's last input was the
// keyboard (CSS :focus-visible).
func (h *FocusHandle) IsFocusVisible(w *Window) bool
```

Tab-order data lives on the handle *pointer*: copies made by
`Window.Focus()` carry no tab info.

`App` holds a `focusMap` from `FocusID` to a refcount. Dropping the last handle
releases the id during `flushEffects`. `Window` stores `focus FocusID` (0 =
none). `App.NewFocusHandle() *FocusHandle` allocates.

## 10. `input.go` — the textbox plumbing

```go
// IntRange is a half-open [Start, End) range of UTF-16 code unit offsets.
type IntRange struct{ Start, End int }

type UTF16Selection struct {
    Range        IntRange
    Reversed     bool
    IsNil        bool
}

// PlatformInputHandler is what a focused text field must provide. This is the
// trimmed-down gpui::EntityInputHandler.
type PlatformInputHandler interface {
    // TextForRange returns the text in the given UTF-16 range.
    TextForRange(rng IntRange) (string, bool)
    SelectedTextRange() (UTF16Selection, bool)
    MarkedTextRange() (IntRange, bool)
    UnmarkText()
    // ReplaceTextInRange replaces the given range (nil range = insert at
    // caret) with text and moves the caret to the end of the inserted text.
    ReplaceTextInRange(rng *IntRange, text string)
    BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool)
    CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool)
    SetSelectedTextRange(rng IntRange)
    TextLengthUTF16() (int, bool)
    AcceptsTextInput() bool
}

// TextInput is a ready-to-use single-line text field view.
type TextInput struct {
    // unexported
}
func NewTextInput(placeholder string) *TextInput
// Text returns the current value.
func (t *TextInput) Text() string
func (t *TextInput) SetText(s string)
// SetPlaceholder replaces the placeholder text and refreshes the window if
// the field has been rendered.
func (t *TextInput) SetPlaceholder(s string)
func (t *TextInput) FocusHandle() *FocusHandle
// Render returns the element to render; it emits TextSubmitted on Enter. It
// returns IntoElement (not Element) so it composes directly inside Child.
func (t *TextInput) Render(w *Window, cx *App) IntoElement
func (t *TextInput) OnSubmit(f func(string, *Window, *App)) *TextInput
func (t *TextInput) OnChange(f func(string, *Window, *App)) *TextInput
// OnEscape replaces Escape's default (blur) while any callback is registered:
// the callbacks run and the field keeps focus.
func (t *TextInput) OnEscape(f func(*Window, *App)) *TextInput
// Style applies f to the field's box div after the defaults (white Bg, 1px
// Gray300 border, Rounded 6, P 8), so f may override background, border,
// padding, text colour/size and a focused border via Div.Focus. The text
// colour also colours the caret. A later call replaces the earlier function;
// a nil result from f keeps the div unchanged.
func (t *TextInput) Style(f func(*Div) *Div) *TextInput
// PlaceholderColor sets the placeholder colour (default Gray500).
func (t *TextInput) PlaceholderColor(c Rgba) *TextInput
// SelectionColor sets the selection highlight fill (default Blue500 at 25%).
func (t *TextInput) SelectionColor(c Rgba) *TextInput
// Multiline makes the field a multi-line editor that grows from one line up
// to maxLines visible lines (values below 1 count as 1) and then scrolls.
func (t *TextInput) Multiline(maxLines int) *TextInput
```

Theming follows GPUI's element-styling model rather than per-property setters:
the field's box is a `Div`, so `Style` hands it to the caller. Only the two
colours that are not div properties (placeholder text, selection fill) get
setters. With no `Style`, `PlaceholderColor` or `SelectionColor` call the
rendering is unchanged.

`TextInput` implements `PlatformInputHandler` internally. It is **not** an
`Entity[T]` for the POC — it is a plain struct the root view owns by value, so
`cx.Notify()` on the owning entity re-renders it. Document this simplification
relative to GPUI's entity-per-textbox design.

### Single-line selection and editing

TextInput keeps public offsets in UTF-16 code units, with internal byte/rune
boundaries. TextForRange returns false for negative, reversed, out-of-range or
surrogate-interior endpoints. SetSelectedTextRange and explicit replacement
ranges clamp each endpoint to [0, length] and snap down to the previous rune
boundary. SetSelectedTextRange sorts endpoints and sets Reversed when the
clamped/snapped first endpoint is greater than the second (collapsed ranges are
not reversed). Explicit replacement ranges are sorted after normalization.
SetText collapses to end; SetText and selection setters refresh the rendered field.

Left/Right move one Unicode rune (never half a surrogate pair). Without Shift,
a nonempty selection collapses to its start/end respectively. Home/End move to
0/document end. With Shift these keys preserve the opposite selection endpoint
as anchor, including reversal when crossing it. Backspace/Delete remove a
selection or one whole rune. Typing/paste use the current selection via the
existing PlatformInputHandler path. Nil ReplaceTextInRange remains insertion
at the active caret; copy/cut use TextForRange as before. Enter submits; Escape
blurs. Editing keys consume their handled events; character keymap precedence
and the textbox context remain unchanged.

Primary mouse press positions the nearest caret; Shift-primary extends from
the existing anchor. ClickCount 2 selects a run of Unicode letters/numbers/marks
or underscore, whitespace, or punctuation/other symbols. ClickCount >=3 selects
the whole single line. Double-click drag extends by those word-class runs;
triple-click drag keeps the whole line selected. Selection capture follows
TextInput identity and its GlobalElementID across redraws, continues outside
bounds (x clamped to line endpoints), and ends on primary release anywhere.
Secondary release does not end it. Removal/replacement, blur, changed field
bounds or text/programmatic selection changes cancel capture; re-entry does not start a new capture. Mouse-down still
bubbles normally; captured movement/release are consumed. Scrollbar capture
has precedence; no generic capture API is introduced.

Rendering uses one unwrapped, clipped text line. Embedded CR/LF display as
spaces while stored text stays unchanged. Focused nonempty selection paints a
light blue rectangle behind selected glyphs; the 2px caret remains at the active
endpoint. Highlight/caret use the laid-out label origin and inherited TextStyle,
clipped to the content box. Full-line shaped cluster advances map x positions;
internal caret stops within a multi-rune ligature interpolate evenly across its
rune boundaries. Fallback metrics count runes, not UTF-16 units. These are logical
metrics using existing shaping APIs; no scale API change. Grapheme navigation,
BiDi visual-order selection, word-arrow shortcuts, horizontal autoscroll,
select-all and undo remain unsupported.

### Multi-line fields

`Multiline(maxLines)` switches a field to multi-line editing; without it every
rule above holds unchanged (and resting renders are pixel-identical). A
multi-line field lays its box out as a column; the text area is one line tall
when empty and grows by a line height per visual line up to `maxLines`, after
which it keeps that height and scrolls vertically. Lines break at `\n` and wrap
softly at the text area's width (the same Unicode line breaking as `Text`);
without a shaper only `\n` breaks lines. A caret at a soft-wrap boundary shows
at the start of the next visual line.

Keys: Shift+Enter inserts `\n`; Enter runs the `OnSubmit` callbacks when any
is registered and otherwise inserts `\n`. Up/Down move to the nearest caret
stop on the previous/next visual line, keeping the x of the first vertical move
while vertical moves chain; Up on the first line goes to 0, Down on the last to
the end, and either forgets that x. Home/End go to the start/end of the visual line (End on a soft-wrapped
line stops before its last rune, so the caret stays on that line). Shift extends
with all of them. Triple-click selects the logical line (between `\n`s).
Double-click, drag, Shift-click and `CharacterIndexForPoint` map the point to
the visual line under it and the nearest stop on that line; `BoundsForRange`
returns the caret rect on its visual line.

Any edit or caret move scrolls the text area so the caret line is visible; the
mouse wheel over the field scrolls it within its content (and is consumed only
when it moved). Text arriving through `ReplaceTextInRange` (typing, paste)
keeps `\n` and turns `\r\n` and `\r` into `\n` in a multi-line field; a
single-line field turns each of them into a space. `SetText` stores its
argument unchanged. Copy and cut keep newlines; IME and the rest of the
`PlatformInputHandler` surface use the same UTF-16 offsets across lines.

## 11. `entity.go`, `context.go`, `app.go`, `subscription.go`

```go
// EntityID mirrors gpui's slotmap key: index + generation.
type EntityID struct {
    index      uint32
    generation uint32
}
func (id EntityID) IsNil() bool
func (id EntityID) String() string

// Entity is a cheap, copyable handle to state stored in the App.
type Entity[T any] struct {
    id  EntityID
    app *App
}
// NewEntity allocates the slot before build runs, so Context.Observe and
// Context.OnRelease called from build attach to the new entity (Read
// returns nil until build returns).
func NewEntity[T any](cx *App, build func(cx *Context[T]) T) Entity[T]
func (e Entity[T]) ID() EntityID
func (e Entity[T]) Read(cx *App) *T
func (e Entity[T]) Update(cx *App, f func(state *T, cx *Context[T]))
func (e Entity[T]) Downgrade() WeakEntity[T]
func (e Entity[T]) Notify(cx *App)
func (e Entity[T]) SubscribeEvent[E any](cx *App, f func(ev *E)) *Subscription
// Clone returns a new strong reference (refcount +1). Release drops one;
// when the count reaches 0 the entity is released at the end of the current
// effect flush (Release outside both Update and a flush flushes
// immediately; inside an observer, event handler or other effect it only
// schedules). Go has no Drop:
// plain copies of an Entity, Context.Entity() and WeakEntity.Upgrade() are
// borrowed (uncounted) handles and must not be Released.
func (e Entity[T]) Clone() Entity[T]
func (e Entity[T]) Release(cx *App)

type WeakEntity[T any] struct {
    id  EntityID
    app *App
}
func (w WeakEntity[T]) Upgrade() (Entity[T], bool) // false once released

// EntityView adapts an entity whose *T has a Render method into a window
// root. It holds a strong reference (Clone) that Window.Close releases.
func EntityView[T any, P interface{ *T; EntityRoot }](e Entity[T]) EntityRoot
```

Releasing an entity frees its slot (stale handles read nil, Upgrade fails),
runs its `OnRelease` callbacks, drops the observers and event listeners
registered on it, and closes the subscriptions it created through
`Context.Observe`.

```go

// Context is the per-entity view handed to Render and Update.
type Context[T any] struct {
    App   *App
    entity EntityID
}
func (c *Context[T]) Notify()
func (c *Context[T]) Emit[E any](event *E)
func (c *Context[T]) Entity() Entity[T]
func (c *Context[T]) WeakEntity() WeakEntity[T]
func (c *Context[T]) Observe(other Entity[T], f func(Entity[T], *App)) *Subscription
func (c *Context[T]) Subscribe(other Entity[T], f func(*E)) *Subscription  // generic over E
func (c *Context[T]) Spawn(f func() ) *Task
func (c *Context[T]) Defer(f func(*App))
func (c *Context[T]) OnRelease(f func(*App)) *Subscription // runs when this entity is released
func (c *Context[T]) Listener[E any](f func(state *T, ev *E, w *Window, cx *App)) func(*E, *Window, *App)

type Task struct{ /* ... */ }
func (t *Task) Cancel()
func (t *Task) Detach()
```

```go
type App struct {
    // unexported
}
func NewApp() *App
func (a *App) Update(f func(*App))   // reentrancy-guarded; outermost runs flushEffects
func (a *App) Quit()           // goroutine-safe (atomic flag); Platform.Quit must be too
func (a *App) IsQuitting() bool // goroutine-safe
func (a *App) OpenWindow(o WindowOptions, build func(*Window, *App) EntityRoot) error
func (a *App) Windows() []*Window
func (a *App) Notify(id EntityID)       // runs observers; invalidates windows that rendered id
func (a *App) RefreshWindows()         // queues effectRefreshWindows: every window redraws
func (a *App) Defer(f func(*App))
func (a *App) Observe[T any](e Entity[T], f func(Entity[T], *App)) *Subscription
func (a *App) Subscribe[T any, E any](e Entity[T], f func(Entity[T], *E)) *Subscription
func (a *App) SetGlobal(v any)
func (a *App) Global(out any) bool
func (a *App) UpdateGlobal(f func(any))
func (a *App) Keymap() *Keymap
func (a *App) NewFocusHandle() *FocusHandle
func (a *App) Platform() Platform
// OnOpenPaths sets the handler for the OS asking the app to open files with no
// window position (macOS: a file opened from Finder or dropped on the Dock
// icon). It replaces any earlier handler; nil removes it. Requests that came
// while no handler was set are delivered to f at once, in order.
func (a *App) OnOpenPaths(f func(paths []string, cx *App))
// OpenPaths hands an open-files request to the OnOpenPaths handler inside
// App.Update, or queues it until one is set. Platforms call it on the UI
// goroutine; tests call it to inject the request. Empty requests are dropped.
func (a *App) OpenPaths(paths []string)
func (a *App) flushEffects()   // unexported
```

`EntityRoot` is the root-view interface a window renders:

```go
// EntityRoot is implemented by any view type via the Render method.
type EntityRoot interface {
    Render(w *Window, cx *App) Element
}
```

Provide a helper so plain functions work as roots:

```go
type RenderFunc func(w *Window, cx *App) Element
func (f RenderFunc) Render(w *Window, cx *App) Element { return f(w, cx) }
```

Effects (unexported enum):

```go
type effectKind uint8
const (
    effectNotify effectKind = iota
    effectEmit
    effectDefer
    effectRefreshWindows
)
type effect struct {
    kind     effectKind
    entity   EntityID
    handler  func(*App)
    event    any
    eventType reflect.Type
}
```

`flushEffects` drains `pendingEffects` in a loop. On `effectNotify` it runs the
observers registered for that `EntityID`; `effectRefreshWindows` marks every
window invalid. Entities whose refcount reached 0 are released after the
queue drains (their release callbacks may queue more effects, which are
drained too). After the queue empties it calls
`scheduleFrame()` on every window whose `invalid` flag is set. Nested
`Update` calls only flush at the outermost (`pendingUpdates` counter); the
flush itself counts as an `Update`, so `Update`/`Release` from a callback
running inside it never starts a nested flush. `Window.DispatchEvent` runs
inside `App.Update` (as GPUI dispatches inside `cx.update`), so effects
queued by handlers flush once, after dispatch.

`Subscription`:

```go
type Subscription struct {
    close func()
}
func (s *Subscription) Close()
func (s *Subscription) IsClosed() bool
```

## 12. `window.go`

```go
type WindowOptions struct {
    Width  float32 // logical px; default 800
    Height float32 // logical px; default 600
    Title  string
    Resizable bool
}

type Window struct {
    // unexported: platform window, renderedFrame, nextFrame, layoutEngine,
    // focus, mouse position, invalid flag, root EntityRoot, scale factor,
    // text shaper cache, element state map, scroll offsets.
}
func (w *Window) Bounds() Bounds          // in logical px: platform size / UIScale
func (w *Window) ScaleFactor() float32    // effective raster scale: platform device scale * UIScale
// SetUIScale zooms the whole UI: layout sees platform size / f logical px,
// the scene rasterizes at device scale * f (text included), and DispatchEvent
// divides pointer positions by f, so platforms need no change. f is clamped to
// [0.25, 4]; the default 1 changes nothing (renders are pixel-identical). It
// invalidates the window. ScrollWheelEvent deltas are not divided.
func (w *Window) SetUIScale(f float32)
func (w *Window) UIScale() float32
func (w *Window) Refresh()                // mark dirty, request frame
func (w *Window) RequestFocus(h *FocusHandle)
func (w *Window) Focus() *FocusHandle
func (w *Window) Blur()
func (w *Window) StopPropagation()
func (w *Window) HandleInput(h PlatformInputHandler)
func (w *Window) InsertHitbox(b Bounds) int
func (w *Window) PaintQuad(q Quad)
func (w *Window) PaintUnderline(u Underline)
func (w *Window) PaintGlyph(g GlyphSprite)
// PaintImage draws img scaled into b (window space, logical pixels), clipped
// to the current content mask, with the current opacity. nil draws nothing.
func (w *Window) PaintImage(b Bounds, img *image.NRGBA)
func (w *Window) PaintShadow(s Shadow)
func (w *Window) PaintStroke(s Stroke)    // unset ContentMask = current clip; opacity applied
// FocusNext / FocusPrev move focus along the rendered frame's tab stops,
// ordered by tab index (ties: tree order), wrapping. With no focused stop,
// Next goes to the first and Prev to the last. They do not bind Tab: apps
// bind "tab"/"shift-tab" to actions that call them, as in GPUI.
func (w *Window) FocusNext()
func (w *Window) FocusPrev()
// FocusVisible reports whether the last input was a key press (true) or a
// mouse press (false); see FocusHandle.IsFocusVisible.
func (w *Window) FocusVisible() bool
func (w *Window) PushLayer()
func (w *Window) PushClip(b Bounds)                // rect content mask, intersected with the current one
func (w *Window) PushContentMask(m ContentMask)    // rect + corner radii, intersected
func (w *Window) PopClip()
func (w *Window) CurrentClip() Bounds
func (w *Window) CurrentContentMask() ContentMask  // what Paint* fill an unset mask with
// Element state not accessed (ElementState/SetElementState, scroll state,
// implicit TabIndex focus handles, whose focus id is then released) between
// the end of one Draw and the end of the next is dropped, as GPUI drops
// state not accessed during a frame.
func (w *Window) ElementState(id GlobalElementID, out any) bool   // retained state lookup
func (w *Window) SetElementState(id GlobalElementID, v any)
func (w *Window) DispatchEvent(input PlatformInput, cx *App)
func (w *Window) Draw(cx *App)            // full pipeline; returns nothing
func (w *Window) Scene() *Scene
func (w *Window) SetBackground(c Rgba)   // clear color for rasterization; default opaque white
// HitTest returns the GlobalElementID of the element owning the topmost
// hitbox of the rendered frame at p. Unlike a hitbox index it is stable across
// frames (as long as the element keeps its path), so platforms can compare a
// press and a release that straddle a redraw. Compare with Equal.
func (w *Window) HitTest(p Point) (id GlobalElementID, ok bool)
// Close removes the window from App.Windows, releases the entities it
// retains (an EntityView root) and calls PlatformWindow.Quit. Idempotent.
func (w *Window) Close()
```

Clipping in the pipeline: an `OverflowHidden`/`OverflowScroll` element's
children (not the element's own background, border or shadows, as in GPUI)
are painted inside a content mask of its bounds; if it has `CornerRadii`
the mask is rounded with them. Nested masks intersect; each corner of the
intersection keeps the radius of the mask(s) that contribute it.

Entity tracking: every entity read (`Entity.Read`/`Update`) while a window
draws is recorded on that window; `App.Notify(id)` (and `Context.Notify`)
marks every window that rendered `id` in its last frame invalid, so it
redraws at the end of the effect flush without an explicit `Refresh`.

Event dispatch (GPUI semantics):

- Each hitbox records its parent hitbox (the nearest ancestor element that
  registered one). A mouse event's *hit path* is the ancestor chain of the
  topmost hitbox under the pointer. Element listeners (`OnMouseDown`,
  `OnClick`, `OnKeyDown`, ...) are bubble-phase, as in GPUI (GPUI's
  `capture_*` variants are not ported, so the capture phase has no
  listeners): they run innermost-first over the path, each exactly once,
  until `StopPropagation`. Siblings that are not on the path are never
  invoked.
- Key events follow the *focus path*: the ancestor chain of the key node whose
  `FocusID` is focused. With nothing focused (or the focused node not in the
  frame) the path is the root element's key node only, matching GPUI's
  dispatch-from-root. Keymap context stack and `OnAction` use the same path.
- Hover: after every mouse move *and after every Draw* (using the last known
  pointer position) the hovered set — hit-path elements with hover callbacks,
  keyed by GlobalElementID — is diffed against the previous one. Callbacks are
  looked up in the current rendered frame when fired (an element no longer in
  the frame gets no `OnHoverEnd`). Ends fire first, leaves-first (deepest
  first); then starts, root-first.

`Frame` (unexported fields, but exposed for tests via accessors):

```go
type Frame struct {
    scene       Scene
    hitboxes    BoundsTree
    // mouseListeners[i] pairs with hitboxes[i]
    mouseListeners []mouseListener
    keyNodes       []keyNode
    inputHandlers  []PlatformInputHandler
    elementStates  map[GlobalElementID]any
    order          uint32
}
type mouseListener struct {
    onMouseDown, onMouseUp, onMouseMove func(any, *Window, *App)
    onClick      func(*ClickEvent, *Window, *App)
    onScrollWheel func(*ScrollWheelEvent, *Window, *App)
    onPinch      func(*PinchEvent, *Window, *App)
    onFileDrop   func(*FileDropEvent, *Window, *App)
    keyContext   string
}
type keyNode struct {
    keyContext string
    focusID    FocusID
    onKeyDown  func(*KeyDownEvent, *Window, *App)
    onKeyUp    func(*KeyUpEvent, *Window, *App)
    onImagePaste func(*ImagePasteEvent, *Window, *App)
    onAction   map[ActionKey]func(Action, *Window, *App)
}
func (f *Frame) SceneRef() *Scene
func (f *Frame) HitboxCount() int
func (f *Frame) HitTest(p Point) int
```

### The draw pipeline (must follow this order)

1. `w.nextFrame.clear()`
2. `root.Render(w, cx)` → `Element` tree.
3. Build a `LayoutNode` tree: recursively `RequestLayout` each element, collect
   child `LayoutID`s, call the layout engine with the window's available size.
4. Walk the tree assigning final `Bounds` (root at origin, window size).
5. `Prepaint` each element in tree order: registers hitboxes, listeners, scroll
   state, input handlers; returns per-element state.
6. `Paint` each element in tree order: emits quads/glyphs/underlines with the
   scene order incremented per primitive.
7. Swap `nextFrame` into `renderedFrame`.
8. If there is a `PlatformWindow`: re-read its `ScaleFactor()` (the device scale;
   the effective scale is that times `UIScale()`), scale the scene
   from logical to device px, rasterize (cleared to the background color) and
   call `Present(pix, round(w*scale), round(h*scale))`; then call
   `SetInputHandler(focusedInputHandler())` (nil when none is focused).

`Prepaint` is where `Interactivity` decides hover/active/focus style refinements
and where the content mask (for `Overflow`) is pushed onto a stack on the window.

## 12b. Clock, timers and animation (`animation.go` / `timer.go`)

```go
// Clock supplies nondecreasing UI time. Implementations run on the UI goroutine.
type Clock interface { Now() time.Time }
func NewAppWithClock(p Platform, clock Clock) *App
func (a *App) Tick()
func (w *Window) FrameTime() time.Time
func (w *Window) RequestNextFrame()
func (w *Window) After(delay time.Duration, f func(*Window, *App)) *Timer
// Timer is a window-owned one-shot callback; Cancel and Done are nil-safe.
type Timer struct { /* unexported */ }
func (t *Timer) Cancel()
func (t *Timer) Done() bool

type Easing func(float32) float32
func Linear(p float32) float32
func EaseIn(p float32) float32
func EaseOut(p float32) float32
func EaseInOut(p float32) float32
func NewAnimation(duration time.Duration, easing Easing) *Animation
type Animation struct { /* unexported */ }
func (a *Animation) Repeat() *Animation
func (a *Animation) Progress(w *Window) float32
func (a *Animation) Cancel()
func (a *Animation) Restart()
func (a *Animation) Done() bool
```

Existing NewApp/NewAppWithPlatform signatures stay unchanged. They use the
platform's optional Clock when present, otherwise time.Now. NewAppWithClock
explicitly overrides that source; nil chooses the normal fallback. Zero-valued
time.Time is a valid clock/frame/start time. Clock time must be nondecreasing.
All new APIs, including cancellation and Tick, run on the UI goroutine; they
create no background callbacks/goroutines. Entity callbacks must upgrade weak
handles at invocation (use Listener), never retain entity state across reentry.

FrameTime is sampled once per draw and stable during render/layout/paint.
RequestNextFrame coalesces independently of dirty invalidation; a next-frame-only
request becomes eligible strictly after the request/last frame timestamp.
Repeated reads/Tick at frozen time do not consume/rearm it. Refresh still requests
ordinary redraws, even at unchanged time. Draw consumes prior invalidation at
entry, preserves invalidation created during/post draw, and never recursively
renders. RequestNextFrame inside Render survives draw completion.

After bases its deadline on Clock.Now at the call, not the prior FrameTime.
Nonpositive delays mean due on the next Tick, never inline. Timers are one-shot;
Done becomes true before invocation, on Cancel, or owner close; Cancel is
idempotent. Nil callbacks return a done timer. Timer callback state changes
redraw the owner automatically unless closed. No repeating timer API; reschedule
After explicitly. Window.Close cancels its timers and queued frames.

Tick snapshots open windows and all due timers at entry, sorted globally by
deadline then insertion order. Cancel/Close before a due callback prevents its
invocation. Callbacks execute inside one App.Update; timers added by callbacks,
even zero-delay/cross-window, wait for a later Tick. Each snapshot window gets
at most one draw, including a callback's manual Draw (tracked by frame serial).
Nested Tick is ignored; repeated Tick within one outer App.Update coalesces.
A later outer Tick at the same timestamp can process new due timers or dirty
state. Custom/stub platforms call App.Tick from their UI loop if they do not
support deadline wakeups; timers never fall back to an immediate wakeup spin.
FrameRequester callbacks pump this same bounded Tick and consume eligibility,
not unconditional drawing. Earliest deadlines are recomputed/cleared on
Cancel/Close. A due timer at zero-valued time wakes by ordinary RequestFrame.

Animation is a persistent view-owned sampler bound to the first window sampled.
It starts on its first Progress call, uses FrameTime, and requests another frame
while running; callers do not call Refresh from Render. Easing input/output
is clamped to [0,1], nil means Linear; NaN easing results fall back to raw progress.
Presets use quadratic ease-in/out and symmetric quadratic ease-in-out. Finite
animation completes at 1; duration <=0 completes immediately even with Repeat.
Positive repeats use elapsed modulo duration with no catch-up callbacks. Cancel
freezes the last sample (0 before first sample) and marks Done; Restart resets
start/completion/cancellation so the next sample starts at 0. Repeat/Restart
while idle require caller Refresh to wake the view. Closing the bound window
stops scheduling and makes Done true. Sample one Animation in one window; using
another window panics. Grapheme/text/style transitions and async executors are
outside this milestone.

## 13. `platform.go`

```go
// Platform is the OS/backend abstraction. Implemented by
// ebitenplatform and by the headless test platform.
type Platform interface {
    // Run blocks until Quit. onLaunch is called once on the main thread
    // before the first frame.
    Run(onLaunch func(*App)) error
    OpenWindow(o WindowOptions) (PlatformWindow, error)
    Quit()
    Name() string // "macos", "windows", "linux", "headless"
}

// PlatformWindow is one OS window. All methods are called on the main thread.
type PlatformWindow interface {
    Bounds() (w, h Pixels)      // logical px
    ScaleFactor() float32
    RequestFrame()              // ask the platform to draw soon
    // Present hands over a frame: premultiplied RGBA, 4*w*h. pix is the
    // window's reusable raster buffer and is only valid until the next
    // Window.Draw; a platform that needs it later (or off the main thread)
    // must copy it.
    Present(pix []byte, w, h int)
    // InputHandler is installed/removed as focus changes.
    SetInputHandler(h PlatformInputHandler)
    TakeInputHandler() PlatformInputHandler
    Quit()
}

// RequestFrameCallback is registered by the Window on its PlatformWindow.
type FrameRequester interface {
    OnRequestFrame(f func()) // callbacks must be queued, never invoked inline by RequestFrame
}

// Optional: replace the timed wakeup; zero clears. RequestFrame remains separate.
type FrameDeadlineRequester interface { RequestFrameAt(deadline time.Time) }
```

The `App` wires `platform.OpenWindow` → creates a `Window` → calls
`window.RequestFrame()` when dirty → the platform calls back into
`Window.DispatchEvent` / `Window.Draw`.

## 14. `headless.go` (in package `gpui`, no external deps)

```go
// RenderToPNG builds root, lays it out at w x h, paints, rasterizes, and
// writes a PNG to path. Used by visual tests and the todo app's -png flag.
func RenderToPNG(path string, w, h int, scale float32, root EntityRoot, opts ...RenderOption) error

type RenderOption func(*renderConfig)
func WithBackgroundColor(c Rgba) RenderOption
func WithKeymap(k *Keymap) RenderOption
func WithTitle(t string) RenderOption

// RenderToImage is the same but returns the image (non-premultiplied, hence
// *image.NRGBA: *image.RGBA is premultiplied by definition).
func RenderToImage(w, h int, scale float32, root EntityRoot, opts ...RenderOption) (*image.NRGBA, error)

// HeadlessPlatform implements Platform with no window; used by RenderToPNG
// and by tests that need to drive frames and dispatch synthetic input.
type HeadlessPlatform struct{ /* ... */ }
func NewHeadlessPlatform() *HeadlessPlatform
func (p *HeadlessPlatform) Run(onLaunch func(*App)) error
func (p *HeadlessPlatform) OpenWindow(o WindowOptions) (PlatformWindow, error)
func (p *HeadlessPlatform) Quit()
func (p *HeadlessPlatform) Name() string

// HeadlessWindow is the PlatformWindow returned by HeadlessPlatform; it keeps
// the last presented pixels and exposes helpers for tests.
type HeadlessWindow struct{ /* ... */ }
func (w *HeadlessWindow) Pixels() []byte
func (w *HeadlessWindow) Image() *image.NRGBA
func (w *HeadlessWindow) SavePNG(path string) error
func (w *HeadlessWindow) Dispatch(input PlatformInput)  // feed synthetic input, then draw if invalidated

// Additions (implemented):
func (p *HeadlessPlatform) SetScaleFactor(s float32)     // scale of windows opened afterwards
func (p *HeadlessPlatform) SetBackgroundColor(c Rgba)    // clear color of windows opened afterwards
func (p *HeadlessPlatform) Windows() []*HeadlessWindow
func (p *HeadlessPlatform) Now() time.Time
func (p *HeadlessPlatform) Advance(d time.Duration) error
func (w *HeadlessWindow) Window() *Window
```

Semantics: `w`,`h` are logical px; the output image is `round(w*scale) x
round(h*scale)` device px. Default background is opaque white. HeadlessWindow
draws lazily: `Pixels()` /
`Image()` / `Dispatch` draw if the window is invalid; `Window.Draw` rasterizes
and `Present`s to the HeadlessWindow, which keeps the pixels. `HeadlessWindow` implements
no `FrameRequester`; `RequestFrame` only sets a pending flag, so redraws never
re-enter an in-flight event dispatch. Text shaping is not
headless-specific: every window created through `App.OpenWindow` gets the
App's shared `Shaper` (one `FontManager` per App), which falls back to the
bundled Go Regular/Bold faces when no system font exists;
`Window.SetTextMeasurer` overrides it and `Window.TextMeasurer()` returns it.

Headless time starts at Unix epoch UTC and changes only through Advance.
Negative Advance fails without changing time; each call pumps each bound App
once. Advance(0) can fire newly due zero-delay timers but cannot advance an
animation frame. Pixels/Image/Dispatch preserve frozen animation time and do
not consume timed next-frame requests; they still flush ordinary invalidation.
With an explicit NewAppWithClock override, advance that injected clock and call
App.Tick; HeadlessPlatform.Advance only advances its own clock, not the override.

`RenderToPNG` uses `image/png` from the standard library.

## 15. `font.go` + `text.go`

```go
// Font is an opaque loaded font face.
type Font struct{ /* ... */ }

// FontManager loads and caches fonts, and resolves the default family.
type FontManager struct{ /* ... */ }
func NewFontManager() *FontManager
// DefaultFontPath resolves the platform's default sans font. Honors the
// GPUI_FONT environment variable. Candidate lists per runtime.GOOS.
// It returns "" when none exists or when system fonts are disabled (always on
// js/wasm; elsewhere when GPUI_NO_SYSTEM_FONTS is non-empty); the FontManager
// then resolves the default font to the bundled Go Regular face (and bold to
// Go Bold), from golang.org/x/image/font/gofont. System fonts stay preferred
// everywhere else, so existing renders are unchanged.
func DefaultFontPath() string
// NoSystemFontsEnv names GPUI_NO_SYSTEM_FONTS: when non-empty, system font
// discovery (default, named families, fallback) is skipped.
const NoSystemFontsEnv = "GPUI_NO_SYSTEM_FONTS"
// Face resolves family by name against the system font directories (file-name
// match, regular and bold files, cached; generic names "monospace", "serif",
// "sans-serif"); unknown or "" families use the default font, except the
// generic "monospace" and "ui-monospace", which resolve to the bundled Go Mono
// and Go Mono Bold (golang.org/x/image/font/gofont) when no system monospace
// font is found or system fonts are disabled.
func (m *FontManager) Face(family string, size float32, weight int) (*Font, error)
// Fallback returns a face (same size and weight) from a system fallback font
// that has a glyph for r, or nil when none does.
func (m *FontManager) Fallback(r rune, size float32, weight int) *Font
func (m *FontManager) DefaultFace(size float32, weight int) (*Font, error)

// Shaper shapes text into positioned glyphs.
type Shaper struct{ /* ... */ }
func NewShaper(m *FontManager) *Shaper

// ShapedLine is the result of shaping one line of text.
type ShapedLine struct {
    Width  Pixels
    Ascent Pixels
    Descent Pixels
    Glyphs []ShapedGlyph
}
type ShapedGlyph struct {
    GlyphID uint32
    X, Y    Pixels   // baseline-relative pen position
    Advance Pixels
    Cluster int      // byte offset into the source string
    Face    *Font    // fallback face that owns GlyphID; nil = the line's face
}

// ShapeLine shapes text at a given size. It does not wrap.
func (s *Shaper) ShapeLine(text string, face *Font) ShapedLine
// ShapeWrap shapes text, wrapping at wrapWidth (math.MaxFloat32 = no wrap),
// returning one ShapedLine per visual line.
func (s *Shaper) ShapeWrap(text string, face *Font, wrapWidth float32) []ShapedLine
// ShapeWrap and Measure treat "\n" (and "\r\n") as hard line breaks.
// Measure uses the default 1.2*size line height; MeasureWrapped (TextMeasurer)
// uses ts.ResolvedLineHeight(), the same height Text paints with.
// Measure returns the natural (w,h) of wrapped text.
func (s *Shaper) Measure(text string, face *Font, wrapWidth float32) (w, h Pixels)
// GlyphMask rasterizes a single glyph into a grayscale mask at the given size.
func (s *Shaper) GlyphMask(face *Font, glyphID uint32, size float32) *GlyphMask
// GlyphMaskScaled rasterizes at size*scale device px (scale is part of the
// cache key); sprite bounds stay logical (mask W/H divided by scale).
func (s *Shaper) GlyphMaskScaled(face *Font, glyphID uint32, size, scale float32) *GlyphMask
// ShapeWrappedScaled is ShapeWrapped with masks rasterized at scale. Window
// uses it (via an optional interface on the TextMeasurer) at its ScaleFactor.
func (s *Shaper) ShapeWrappedScaled(text string, ts TextStyle, maxWidth, scale float32) []ShapedLineOut
```

`font.go`/`text.go` may import `github.com/go-text/typesetting/...` and
`golang.org/x/image/font/sfnt`, `golang.org/x/image/font/gofont/{goregular,gobold}`, `golang.org/x/image/vector`, `golang.org/x/image/math/fixed`.

Default font resolution order (first existing path wins), overridable with the
`GPUI_FONT` env var:

- darwin: `/System/Library/Fonts/SFNS.ttf`, `/System/Library/Fonts/Helvetica.ttc`,
  `/Library/Fonts/Arial.ttf`
- windows: `%WINDIR%\Fonts\segoeui.ttf`, `arial.ttf`, `seguisb.ttf`
- linux: `/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
  `/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf`,
  `/usr/share/fonts/TTF/DejaVuSans.ttf`, `~/.fonts/DejaVuSans.ttf`

## 16. `clipboard.go`

```go
// Clipboard reads/writes the system clipboard via OS tools.
func ClipboardRead() (string, error)
func ClipboardWrite(s string) error
// ClipboardAvailable reports whether an OS clipboard tool was found.
func ClipboardAvailable() bool
// ClipboardReadImage returns the image on the system clipboard as PNG bytes
// and its MIME type ("image/png"), or ErrNoClipboardImage when it holds none.
// It blocks (up to ~3s); UI code calls it off the main loop.
func ClipboardReadImage() (data []byte, mime string, err error)
var ErrNoClipboardImage error
```

Image read tools, each tried in turn like the text ones:
- darwin: `osascript -e 'the clipboard as «class PNGf»'`, whose `«data PNGf<hex>»`
  answer is decoded.
- windows: PowerShell `System.Windows.Forms.Clipboard.GetImage()` saved as
  PNG and written to stdout as base64.
- linux: `wl-paste --no-newline --type image/png` (only with `WAYLAND_DISPLAY`),
  then `xclip -selection clipboard -t image/png -o`; `xsel` has no images.
- The answer must start with the PNG signature, else it counts as no image.

- darwin: `pbpaste` / `pbcopy`
- windows: `powershell -NoProfile -Command Get-Clipboard` /
  `Set-Clipboard -Value ...` (pass via stdin to avoid quoting bugs)
- linux: `wl-paste`/`wl-copy` only when `WAYLAND_DISPLAY` is set, then
  `xclip`, then `xsel`; a tool that is missing or fails falls through to the
  next one.
- Every tool runs via `exec.CommandContext` with a ~1s timeout.
- PowerShell commands force UTF-8 (`[Console]::OutputEncoding` /
  `InputEncoding`) and read/write raw text (no added trailing newline).

## 17. `ebitenplatform/`

```go
package ebitenplatform

import (
    "image"

    "cad-lite/internal/gpui"
)

type Platform struct{ /* ... */ }
func New(opts ...Option) gpui.Platform     // gpui.Platform is an interface

// Option configures a Platform at construction.
type Option func(*Platform)

// WithSnapshotKey binds the frame-snapshot shortcut to keystroke, written in
// keymap syntax ("cmd-shift-s"); "" disables the shortcut. It panics on a
// malformed keystroke (a construction-time constant, like NewKeyBinding).
func WithSnapshotKey(keystroke string) Option

// DefaultSnapshotKey is the default binding: "cmd-shift-s" on darwin,
// "ctrl-shift-s" elsewhere.
func DefaultSnapshotKey() string

// WithWindowIcon sets the window icon (the title bar and taskbar on Windows
// and Linux) through ebiten.SetWindowIcon. imgs are the same picture at several
// sizes, 16 to 256 pixels, and the OS picks the closest; an empty slice leaves
// the default icon. macOS ignores it: the Dock icon is the .app bundle's
// AppIcon.icns.
func WithWindowIcon(imgs []image.Image) Option
```

Snapshot: when the bound keystroke is pressed, the last presented frame
(device resolution) is copied on the game loop and encoded off it, to
`./snapshot-YYYYMMDD-HHMMSS.png` (a `-N` suffix is added if the name
exists); the path (or the error) is logged to stderr. The keystroke is
consumed: neither its key-down nor its key-up nor any text input is
dispatched to the app. A printable snapshot key only fires under the
shortcut predicate (see input mapping below).

Implements `gpui.Platform`:

- `Run(onLaunch)` → `ebiten.RunGame(g)` where `g` implements
  `Update() error`, `Draw(screen *ebiten.Image)`, `Layout(ow, oh int) (int, int)`.
- `Layout` returns **device pixels** (`outside * Monitor().DeviceScaleFactor()`)
  so the rasterizer and `CursorPosition` share one coordinate space.
- `Update` polls input (see below) and forwards to the window's
  `DispatchEvent`, wrapped in `app.Update`.
- `Draw` calls `screen.WritePixels(pix)` with premultiplied RGBA; skips the
  upload when not dirty. `ebiten.SetScreenClearedEveryFrame(false)` at startup.
- `Present(pix, w, h)` keeps a reference to the buffer (no copy) for the
  next `Draw`; this is safe because `Window.Draw` only runs inside `Update`,
  on the same goroutine as `Draw`.
- Idle cost: `Update` polls input every tick but only draws when
  `RequestFrame` was called (or the size/scale changed); `Draw` uploads only
  a fresh frame, so with nothing changing ebiten skips presenting.
- `RequestFrame` sets a dirty flag that `Update`/`Draw` observe; the Window
  calls it from `flushEffects`.
- Clean quit: return `ebiten.Termination` from `Update` when `app.IsQuitting()`.
- Window title/size: `ebiten.SetWindowTitle`, `ebiten.SetWindowSize` (DIP).
- `ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)`.

Input mapping in `Update`:

- `inpututil.IsMouseButtonJustPressed` → `MouseDownEvent`; `JustReleased` →
  `MouseUpEvent` + a `ClickEvent` if `Window.HitTest` yields the same
  `GlobalElementID` at press and release (press state tracked per button);
  `ebiten.CursorPosition()` → `Position` each tick, synthesized into
  `MouseMoveEvent` when it changed.
- `ebiten.Wheel()` → `ScrollWheelEvent` (note: ebiten's wheel y is typically
  inverted relative to scroll-down-positive; normalize so positive `DeltaY`
  means scroll down / content moves up). `Precise` is set from the raw
  values: ebiten exposes no device kind, so the backend classifies each
  event (a fractional or two-axis delta is precise; an integral one-axis delta
  is a notch unless it follows a precise event within 250 ms). Precise deltas
  are 10 px per ebiten unit (GLFW already scales macOS precise deltas by 0.1),
  notches 20 px per line; under `GOOS=js` ebiten's units are CSS pixels and
  are passed through unscaled, with integral deltas of 50 or more taken as
  notches. A pinch that a browser delivers as Ctrl+wheel arrives as a
  precise event with `Control` set. The js backend reads `WheelEvent.ctrlKey`
  in a passive capture listener (`wheelctrl_js.go`) and ORs `Control` into the
  wheel event, because a browser pinch has `ctrlKey` but no Control keydown,
  which is the only source ebiten derives its Control state from; ebiten's own
  wheel listener already calls `preventDefault`, so the page does not zoom.
- macOS pinch: GLFW drops `NSEventTypeMagnify`, so `pinch_darwin.go` installs
  an `NSEvent` local monitor (`addLocalMonitorForEventsMatchingMask:handler:`,
  `NSEventMaskMagnify`) through `purego/objc`. The first `Update` schedules the install on the main
  queue (`dispatch_async_f` on `_dispatch_main_q` from libSystem, once), so
  AppKit is only touched on the main thread. The block
  runs on the main thread and only pushes to a mutex-guarded `pinchQueue`
  (position from `locationInWindow` flipped by the content view height, the
  `magnification` value, modifiers from `modifierFlags`); `Update` drains it
  into `PinchEvent`s. Any failure to install logs once and leaves pinch
  unsupported. Smart magnify (double-tap) is not handled. Other OSes have no
  native source; Windows precision touchpads deliver a pinch as Ctrl+wheel.
- File drops: `ebiten.DroppedFiles()` (non-nil only on the tick of a drop) →
  one `FileDropEvent` at the cursor position, which GLFW moves to the drop
  point before reporting the drop on every desktop OS. ebiten hands over a
  virtual `fs.FS` rooted at the deepest directory holding every dropped path,
  whose entries implement `ebiten.AbsPather` on desktops (ebiten 2.10+);
  `droppedPaths` walks it back to the dropped paths. A root entry that is a
  file is a dropped file; a directory is a dropped folder unless it lists
  fewer entries than the real directory, in which case it is only the way to
  dropped paths further down (files dropped from several folders at once) and
  is walked. An entry with no `AbsPath` (a browser drop, whose files exist
  only in memory) is skipped, so `GOOS=js` sends no event. Drop order is lost:
  the virtual file system sorts by name.
- macOS open-documents: GLFW installs no handler for the `odoc` Apple event
  (a file opened from Finder, or dropped on the Dock icon), so
  `openfiles_darwin.go` registers one through `purego/objc` on
  `NSAppleEventManager` for `kCoreEventClass`/`kAEOpenDocuments`. `Run`
  arranges it on the main thread before `ebiten.RunGame`: it installs the
  handler and again from an `NSApplicationWillFinishLaunchingNotification`
  observer, because `NSApplication` installs its own Apple event handlers while
  launching and the `odoc` of a launch-by-document is dispatched right after
  that notification, inside GLFW's initial `[NSApp run]`. The handler reads each
  item's `fileURLValue` path and pushes the batch to a mutex-guarded queue;
  `Update` drains it into `App.OpenPaths`. Other OSes pass files to open as
  command-line arguments, which is the app's business. A failure to install
  logs once and leaves the event unhandled (AppKit then reports that the
  document cannot be opened).
- `ebiten.AppendInputChars` → `KeyDownEvent{PreferCharacterInput:true}` per
  rune, always — except while a shortcut modifier is held: Cmd on darwin,
  Super elsewhere (`ModPlatform`), or Ctrl unless it is AltGr (Ctrl+Alt on
  Windows/Linux). Alt/Option alone never suppresses text (Option-produced
  characters like `@ # [ ]` on non-US Mac layouts must reach text input).
  Printable keys are dispatched as plain `KeyDownEvent`s (for keybindings)
  only under that same shortcut predicate.
- Special keys via `inpututil.IsKeyJustPressed` (only valid in `Update`):
  `KeyEnter→"enter"`, `KeyEscape→"escape"`, `KeyBackspace`, `KeyDelete`,
  `KeyLeft/Right/Up/Down`, `KeySpace→"space"`, `KeyTab→"tab"`, letters/digits.
- Modifiers (GPUI convention, identical on every OS): `KeyControl` →
  `Control`, `KeyMeta` → `ModPlatform` (Cmd on macOS, Super/Win elsewhere),
  `KeyShift` → `Shift`, `KeyAlt` → `Alt`. Keybindings that should be
  "Cmd on mac, Ctrl elsewhere" must be registered per OS by the app.
- Clipboard via `gpui.ClipboardRead/Write` for cmd-c/v/x on darwin and
  ctrl-c/v/x elsewhere. The OS tools run on a goroutine; results are posted
  back over a channel and applied in a later `Update` (paste → character
  input; cut deletes the selection only if it is unchanged and the write
  succeeded). A paste whose text read fails or is empty tries
  `gpui.ClipboardReadImage` on the same goroutine and, when an image comes
  back, dispatches an `ImagePasteEvent`.

## 18. `examples/todo/`

```go
package main

type Todo struct {
    Title string
    Done  bool
}
type TodoApp struct {
    todos   []Todo
    input   *gpui.TextInput
    nextID  int
}
func NewTodoApp() *gpui.TodoApp  // or just construct in main
func (t *TodoApp) Render(w *gpui.Window, cx *gpui.App) gpui.Element
```

`Render` returns:

```
Div().FlexCol().SizeFull().Gap(12).P(16).Bg(Gray50)
  ├─ Text("Todos").TextSize(24).FontWeight(700)
  ├─ Div().FlexRow().Gap(8).ItemsCenter()
  │    ├─ input.Render(w, cx)      // Flex(1)
  │    └─ Div().ID("add").OnClick(add).P(8).Bg(Blue500).Rounded(6).Child(Text("Add").TextColor(White))
  └─ Div().ID("list").Flex(1).OverflowScroll().FlexCol().Gap(4)
       └─ per todo: Div().ID(fmt.Sprintf("todo-%d", i)).FlexRow().ItemsCenter().Gap(8).P(8).Bg(White).Rounded(6)
             .OnClick(toggle)
             ├─ Text(done? "☑":"☐")  (or a small quad indicator)
             └─ Text(title).TextStrikethrough(if done)
```

`main`:

```go
func main() {
    flags: -png PATH, -width, -height, -smoke MS
    if *png != "" { gpui.RenderToPNG(...); return }
    err := ebitenplatform.New().Run(func(cx *gpui.App) {
        cx.Keymap().AddBindings(
            gpui.NewKeyBinding("enter", gpui.NewAction("add-todo"), "textbox"),
            gpui.NewKeyBinding("escape", gpui.NewAction("blur"), "textbox"),
        )
        cx.OpenWindow(opts, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot { return todoApp })
    })
}
```

---

## Conventions every agent must follow

1. **Package `gpui` imports only stdlib** (+ `x/image` and `go-text/typesetting`
   in `font.go`/`text.go`). No ebiten in `gpui`.
2. **No comments unless the code is genuinely non-obvious**; prefer clear names.
   (A short package doc comment and a README paragraph per file group are fine.)
3. **All exported identifiers get a doc comment** — this is a library.
4. `gofmt -l .` must be clean. `go vet ./...` must pass.
5. Tests live in `*_test.go`, package `gpui_test` (external test package)
   unless internal access is required.
6. Prefer `float32` for geometry; convert to `int` only at raster/pixel boundaries.
7. Never hold a `*T` from an entity across a call that can re-enter the same
   entity — upgrade the weak handle at invoke time instead (see
   the package function `gpui.Listener(ctx, f)`).
