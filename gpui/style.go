package gpui

// LengthKind discriminates the three ways an axis length can be specified.
type LengthKind uint8

const (
	// LengthAuto is content-based sizing. On a min-length it also denotes the
	// CSS automatic minimum size; on a max-length it denotes "no maximum".
	LengthAuto LengthKind = iota
	// LengthPx is a definite length in logical pixels.
	LengthPx
	// LengthFlex is CSS `flex: Val` — a flex basis of 0 with the given grow
	// factor. It is only meaningful on the main axis of a flex container.
	LengthFlex
)

// Length is a single axis length specification.
type Length struct {
	Kind LengthKind
	Val  Pixels
}

// Auto returns a content-based Length.
func Auto() Length { return Length{Kind: LengthAuto} }

// Px returns a definite length in logical pixels.
func Px(v float32) Length { return Length{Kind: LengthPx, Val: Pixels(v)} }

// Flex returns CSS `flex: g` — flex-basis 0 with flex-grow g and flex-shrink 1.
func Flex(g float32) Length { return Length{Kind: LengthFlex, Val: Pixels(g)} }

// IsPx reports whether the length is a definite pixel length.
func (l Length) IsPx() bool { return l.Kind == LengthPx }

// IsAuto reports whether the length is content-based.
func (l Length) IsAuto() bool { return l.Kind == LengthAuto }

// IsFlex reports whether the length is a flex factor.
func (l Length) IsFlex() bool { return l.Kind == LengthFlex }

// Direction selects a flex container's main axis.
type Direction uint8

const (
	// DirectionColumn stacks children vertically; main axis is height.
	DirectionColumn Direction = iota
	// DirectionRow lays children out horizontally; main axis is width.
	DirectionRow
)

// Justify is the main-axis distribution of a flex container.
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
	JustifySpaceAround
)

// Align is a cross-axis alignment value, used for both align-items and
// align-self.
type Align uint8

const (
	// AlignStretch is the CSS initial value: children fill the line's cross size.
	AlignStretch Align = iota
	AlignStart
	AlignCenter
	AlignEnd
)

// AlignSelfAuto on a child means "inherit the parent's AlignItems".
const AlignSelfAuto Align = 255

// OverflowKind controls clipping and scrollability. It never changes the
// layout of descendants — only painting, scroll extent, and the automatic
// minimum size rule.
type OverflowKind uint8

const (
	// OverflowVisible is the CSS default: no clipping, content propagates into
	// the parent's scrollable overflow.
	OverflowVisible OverflowKind = iota
	// OverflowHidden clips painting and is a scroll container for layout
	// purposes (automatic minimum size becomes 0).
	OverflowHidden
	// OverflowScroll is OverflowHidden plus a scroll offset driven by input.
	OverflowScroll
)

// Axes is a bit set of the two layout axes.
type Axes uint8

const (
	// AxisX is the horizontal axis.
	AxisX Axes = 1 << iota
	// AxisY is the vertical axis.
	AxisY
	// AxesBoth is AxisX | AxisY.
	AxesBoth = AxisX | AxisY
)

// Has reports whether every axis in o is in a.
func (a Axes) Has(o Axes) bool { return a&o == o }

// TextStyle describes how a run of text is rendered. It is opaque to the
// layout engine, which only needs the resulting metrics.
type TextStyle struct {
	FontFamily    string // "" means the platform default family
	FontSize      Pixels // default 14
	FontWeight    int    // 400 normal, 700 bold
	Color         Rgba   // default black
	LineHeight    Pixels // 0 means 1.2 * FontSize
	Strikethrough bool
}

// DefaultTextStyle returns the text style used when none is specified.
func DefaultTextStyle() TextStyle {
	return TextStyle{
		FontSize:   14,
		FontWeight: 400,
		Color:      Black,
	}
}

// ResolvedLineHeight returns the effective line height in pixels.
func (t TextStyle) ResolvedLineHeight() Pixels {
	if t.LineHeight > 0 {
		return t.LineHeight
	}
	return Pixels(float32(t.FontSize) * 1.2)
}

// Style is a fully-resolved per-element style, consumed by layout and paint.
type Style struct {
	Display   bool // false means Display:none; the element is skipped entirely
	Direction Direction

	Width, Height       Length
	MinWidth, MinHeight Length // LengthAuto means the automatic minimum size
	MaxWidth, MaxHeight Length // LengthAuto means no maximum

	Margin, Padding Edges
	Gap             Pixels // main-axis gutter between adjacent children

	FlexGrow   float32 // default 0
	FlexShrink float32 // default 1

	Justify    Justify
	AlignItems Align
	AlignSelf  Align // AlignSelfAuto means inherit AlignItems
	Overflow   OverflowKind

	BorderWidths Edges
	BorderColor  Rgba
	CornerRadii  Corners
	// Background is the fill; a transparent one paints no quad.
	Background Background
	// BoxShadows are painted outside (or, inset, inside) the border box.
	BoxShadows []BoxShadow
	// Opacity multiplies the alpha of everything the element and its
	// descendants paint; 1 is opaque.
	Opacity float32
	// ScrollAxes are the axes an OverflowScroll container scrolls.
	ScrollAxes Axes

	TextStyle TextStyle
}

// DefaultStyle returns the style an element starts from before refinement.
func DefaultStyle() Style {
	return Style{
		Display:    true,
		Direction:  DirectionColumn,
		Width:      Auto(),
		Height:     Auto(),
		MinWidth:   Auto(),
		MinHeight:  Auto(),
		MaxWidth:   Auto(),
		MaxHeight:  Auto(),
		FlexGrow:   0,
		FlexShrink: 1,
		Justify:    JustifyStart,
		AlignItems: AlignStretch,
		AlignSelf:  AlignSelfAuto,
		Overflow:   OverflowVisible,
		Opacity:    1,
		ScrollAxes: AxesBoth,
		TextStyle:  DefaultTextStyle(),
	}
}

// isScrollContainer reports whether this node clips and sizes its children to
// content rather than to its own box.
func (s Style) isScrollContainer() bool { return s.Overflow != OverflowVisible }

func (s Style) resolveMain(isRow bool) Length {
	if isRow {
		return s.Width
	}
	return s.Height
}

func (s Style) resolveCross(isRow bool) Length {
	if isRow {
		return s.Height
	}
	return s.Width
}

func (s Style) resolveMinMain(isRow bool) Length {
	if isRow {
		return s.MinWidth
	}
	return s.MinHeight
}

func (s Style) resolveMaxMain(isRow bool) Length {
	if isRow {
		return s.MaxWidth
	}
	return s.MaxHeight
}

func (s Style) resolveMinCross(isRow bool) Length {
	if isRow {
		return s.MinHeight
	}
	return s.MinWidth
}

func (s Style) resolveMaxCross(isRow bool) Length {
	if isRow {
		return s.MaxHeight
	}
	return s.MaxWidth
}

func (s Style) mainPadding(isRow bool) Pixels {
	if isRow {
		return s.Padding.Horizontal()
	}
	return s.Padding.Vertical()
}

func (s Style) crossPadding(isRow bool) Pixels {
	if isRow {
		return s.Padding.Vertical()
	}
	return s.Padding.Horizontal()
}

func (s Style) mainPaddingStart(isRow bool) Pixels {
	if isRow {
		return s.Padding.Left
	}
	return s.Padding.Top
}

func (s Style) crossPaddingStart(isRow bool) Pixels {
	if isRow {
		return s.Padding.Top
	}
	return s.Padding.Left
}

func (s Style) mainPaddingEnd(isRow bool) Pixels {
	if isRow {
		return s.Padding.Right
	}
	return s.Padding.Bottom
}

func (s Style) crossPaddingEnd(isRow bool) Pixels {
	if isRow {
		return s.Padding.Bottom
	}
	return s.Padding.Right
}

// StyleRefinement is the partial form of Style used by the fluent builder.
// Every field is a pointer so that "unset" is distinguishable from the zero
// value; Refine fills the unset fields from a base Style.
type StyleRefinement struct {
	Display   *bool
	Direction *Direction

	Width, Height       *Length
	MinWidth, MinHeight *Length
	MaxWidth, MaxHeight *Length

	Margin, Padding *Edges
	Gap             *Pixels

	FlexGrow, FlexShrink *float32

	Justify    *Justify
	AlignItems *Align
	AlignSelf  *Align
	Overflow   *OverflowKind

	BorderWidths *Edges
	BorderColor  *Rgba
	CornerRadii  *Corners
	Background   *Background
	BoxShadows   *[]BoxShadow
	Opacity      *float32
	ScrollAxes   *Axes

	TextStyle *TextStyle
}

// Refine merges the refinement onto base, returning a resolved Style. Fields
// left nil in the refinement keep their value from base. A nil receiver
// returns base unchanged.
func (r *StyleRefinement) Refine(base Style) Style {
	out := base
	if r == nil {
		return out
	}
	if r.Display != nil {
		out.Display = *r.Display
	}
	if r.Direction != nil {
		out.Direction = *r.Direction
	}
	if r.Width != nil {
		out.Width = *r.Width
	}
	if r.Height != nil {
		out.Height = *r.Height
	}
	if r.MinWidth != nil {
		out.MinWidth = *r.MinWidth
	}
	if r.MinHeight != nil {
		out.MinHeight = *r.MinHeight
	}
	if r.MaxWidth != nil {
		out.MaxWidth = *r.MaxWidth
	}
	if r.MaxHeight != nil {
		out.MaxHeight = *r.MaxHeight
	}
	if r.Margin != nil {
		out.Margin = *r.Margin
	}
	if r.Padding != nil {
		out.Padding = *r.Padding
	}
	if r.Gap != nil {
		out.Gap = *r.Gap
	}
	if r.FlexGrow != nil {
		out.FlexGrow = *r.FlexGrow
	}
	if r.FlexShrink != nil {
		out.FlexShrink = *r.FlexShrink
	}
	if r.Justify != nil {
		out.Justify = *r.Justify
	}
	if r.AlignItems != nil {
		out.AlignItems = *r.AlignItems
	}
	if r.AlignSelf != nil {
		out.AlignSelf = *r.AlignSelf
	}
	if r.Overflow != nil {
		out.Overflow = *r.Overflow
	}
	if r.BorderWidths != nil {
		out.BorderWidths = *r.BorderWidths
	}
	if r.BorderColor != nil {
		out.BorderColor = *r.BorderColor
	}
	if r.CornerRadii != nil {
		out.CornerRadii = *r.CornerRadii
	}
	if r.Background != nil {
		out.Background = *r.Background
	}
	if r.BoxShadows != nil {
		out.BoxShadows = *r.BoxShadows
	}
	if r.Opacity != nil {
		out.Opacity = *r.Opacity
	}
	if r.ScrollAxes != nil {
		out.ScrollAxes = *r.ScrollAxes
	}
	if r.TextStyle != nil {
		out.TextStyle = *r.TextStyle
	}
	return out
}

// merge overwrites r's fields with every field set in o.
func (r *StyleRefinement) merge(o *StyleRefinement) {
	if o == nil {
		return
	}
	mergePtr(&r.Display, o.Display)
	mergePtr(&r.Direction, o.Direction)
	mergePtr(&r.Width, o.Width)
	mergePtr(&r.Height, o.Height)
	mergePtr(&r.MinWidth, o.MinWidth)
	mergePtr(&r.MinHeight, o.MinHeight)
	mergePtr(&r.MaxWidth, o.MaxWidth)
	mergePtr(&r.MaxHeight, o.MaxHeight)
	mergePtr(&r.Margin, o.Margin)
	mergePtr(&r.Padding, o.Padding)
	mergePtr(&r.Gap, o.Gap)
	mergePtr(&r.FlexGrow, o.FlexGrow)
	mergePtr(&r.FlexShrink, o.FlexShrink)
	mergePtr(&r.Justify, o.Justify)
	mergePtr(&r.AlignItems, o.AlignItems)
	mergePtr(&r.AlignSelf, o.AlignSelf)
	mergePtr(&r.Overflow, o.Overflow)
	mergePtr(&r.BorderWidths, o.BorderWidths)
	mergePtr(&r.BorderColor, o.BorderColor)
	mergePtr(&r.CornerRadii, o.CornerRadii)
	mergePtr(&r.Background, o.Background)
	mergePtr(&r.BoxShadows, o.BoxShadows)
	mergePtr(&r.Opacity, o.Opacity)
	mergePtr(&r.ScrollAxes, o.ScrollAxes)
	mergePtr(&r.TextStyle, o.TextStyle)
}

func mergePtr[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}
