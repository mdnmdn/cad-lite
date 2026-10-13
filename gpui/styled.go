package gpui

// StyledMixin holds a StyleRefinement plus the fluent, Tailwind-like builder
// methods that configure it. Element types such as *Div and *Text embed it and
// thereby expose every method below via Go method promotion; callers chain
// normally and recover the concrete type through their own variable.
type StyledMixin struct {
	style StyleRefinement
}

// NewStyledMixin returns an empty refinement — every field unset.
func NewStyledMixin() StyledMixin { return StyledMixin{} }

// StyleRef returns the underlying partial style for refinement.
func (m *StyledMixin) StyleRef() *StyleRefinement { return &m.style }

// ResolvedStyle merges the refinement onto base.
func (m *StyledMixin) ResolvedStyle(base Style) Style { return m.style.Refine(base) }

func (m *StyledMixin) setDisplay(v bool) *StyledMixin {
	m.style.Display = &v
	return m
}

// Hidden makes the element take up no space and paint nothing (Display:none).
func (m *StyledMixin) Hidden() *StyledMixin { return m.setDisplay(false) }

// Visible undoes Hidden.
func (m *StyledMixin) Visible() *StyledMixin { return m.setDisplay(true) }

func (m *StyledMixin) setWidth(l Length) *StyledMixin {
	m.style.Width = &l
	return m
}

func (m *StyledMixin) setHeight(l Length) *StyledMixin {
	m.style.Height = &l
	return m
}

// SizeFull fills both axes. The POC's stand-in for CSS `100%`: it sets both
// axes to Flex(1) and both minimums to Px(0), so a child of a flex parent
// fills it without the automatic minimum size blocking the shrink.
func (m *StyledMixin) SizeFull() *StyledMixin {
	m.style.Width = lengthPtr(Flex(1))
	m.style.Height = lengthPtr(Flex(1))
	m.style.MinWidth = lengthPtr(Px(0))
	m.style.MinHeight = lengthPtr(Px(0))
	return m
}

// SizeWH sets both axes to definite pixel lengths.
func (m *StyledMixin) SizeWH(w, h float32) *StyledMixin {
	m.style.Width = lengthPtr(Px(w))
	m.style.Height = lengthPtr(Px(h))
	return m
}

// SizeW sets the width to a definite pixel length.
func (m *StyledMixin) SizeW(w float32) *StyledMixin { return m.setWidth(Px(w)) }

// SizeH sets the height to a definite pixel length.
func (m *StyledMixin) SizeH(h float32) *StyledMixin { return m.setHeight(Px(h)) }

// FlexGrow sets the flex-grow factor.
func (m *StyledMixin) FlexGrow(v float32) *StyledMixin {
	m.style.FlexGrow = &v
	return m
}

// FlexShrink sets the flex-shrink factor.
func (m *StyledMixin) FlexShrink(v float32) *StyledMixin {
	m.style.FlexShrink = &v
	return m
}

// Flex1 is GPUI's flex_1, CSS `flex: 1 1 0%`: grow and shrink from a zero
// basis, so siblings with Flex1 share the space equally regardless of their
// content (each still stops at its automatic minimum size).
func (m *StyledMixin) Flex1() *StyledMixin {
	m.style.FlexGrow = float32Ptr(1)
	m.style.FlexShrink = float32Ptr(1)
	m.style.FlexBasis = lengthPtr(Px(0))
	return m
}

// FlexAuto is CSS `flex: 1 1 auto`: grow and shrink from the content (or
// main) size, so free space is added on top of each item's content.
func (m *StyledMixin) FlexAuto() *StyledMixin {
	m.style.FlexGrow = float32Ptr(1)
	m.style.FlexShrink = float32Ptr(1)
	m.style.FlexBasis = lengthPtr(Auto())
	return m
}

// FlexNone is CSS `flex: none`: neither grow nor shrink, auto basis.
func (m *StyledMixin) FlexNone() *StyledMixin {
	m.style.FlexGrow = float32Ptr(0)
	m.style.FlexShrink = float32Ptr(0)
	m.style.FlexBasis = lengthPtr(Auto())
	return m
}

// FlexBasis sets a definite flex base size in pixels.
func (m *StyledMixin) FlexBasis(v float32) *StyledMixin {
	m.style.FlexBasis = lengthPtr(Px(v))
	return m
}

// Flex turns the element into a flex container stacking children vertically.
func (m *StyledMixin) Flex() *StyledMixin {
	m.style.Display = boolPtr(true)
	m.style.Direction = dirPtr(DirectionColumn)
	return m
}

// FlexRow makes the element a flex container with a horizontal main axis.
func (m *StyledMixin) FlexRow() *StyledMixin {
	m.style.Display = boolPtr(true)
	m.style.Direction = dirPtr(DirectionRow)
	return m
}

// FlexCol makes the element a flex container with a vertical main axis.
func (m *StyledMixin) FlexCol() *StyledMixin {
	m.style.Display = boolPtr(true)
	m.style.Direction = dirPtr(DirectionColumn)
	return m
}

// Gap sets the main-axis gutter between adjacent children.
func (m *StyledMixin) Gap(v float32) *StyledMixin {
	g := Pixels(v)
	m.style.Gap = &g
	a := Auto()
	m.style.RowGap, m.style.ColumnGap = &a, &a
	return m
}

// P sets padding on all four edges.
func (m *StyledMixin) P(v float32) *StyledMixin {
	m.style.Padding = edgesAll(v)
	return m
}

// Px_ sets padding on the left and right edges.
func (m *StyledMixin) Px_(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Left, e.Right = Pixels(v), Pixels(v)
	m.style.Padding = &e
	return m
}

// Py_ sets padding on the top and bottom edges.
func (m *StyledMixin) Py_(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Top, e.Bottom = Pixels(v), Pixels(v)
	m.style.Padding = &e
	return m
}

// Pt sets top padding.
func (m *StyledMixin) Pt(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Top = Pixels(v)
	m.style.Padding = &e
	return m
}

// Pr sets right padding.
func (m *StyledMixin) Pr(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Right = Pixels(v)
	m.style.Padding = &e
	return m
}

// Pb sets bottom padding.
func (m *StyledMixin) Pb(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Bottom = Pixels(v)
	m.style.Padding = &e
	return m
}

// Pl sets left padding.
func (m *StyledMixin) Pl(v float32) *StyledMixin {
	e := m.style.Padding.unwrap()
	e.Left = Pixels(v)
	m.style.Padding = &e
	return m
}

// M sets margin on all four edges.
func (m *StyledMixin) M(v float32) *StyledMixin {
	m.style.Margin = edgesAll(v)
	m.style.MarginAuto = &AutoEdges{}
	return m
}

// setMargin sets the selected margin edges to v and clears their auto flag.
func (m *StyledMixin) setMargin(v float32, top, right, bottom, left bool) *StyledMixin {
	e := m.style.Margin.unwrap()
	var a AutoEdges
	if m.style.MarginAuto != nil {
		a = *m.style.MarginAuto
	}
	p := Pixels(v)
	if top {
		e.Top, a.Top = p, false
	}
	if right {
		e.Right, a.Right = p, false
	}
	if bottom {
		e.Bottom, a.Bottom = p, false
	}
	if left {
		e.Left, a.Left = p, false
	}
	m.style.Margin, m.style.MarginAuto = &e, &a
	return m
}

// setMarginAuto marks the selected margin edges auto.
func (m *StyledMixin) setMarginAuto(top, right, bottom, left bool) *StyledMixin {
	var a AutoEdges
	if m.style.MarginAuto != nil {
		a = *m.style.MarginAuto
	}
	a.Top = a.Top || top
	a.Right = a.Right || right
	a.Bottom = a.Bottom || bottom
	a.Left = a.Left || left
	m.style.MarginAuto = &a
	return m
}

// Mx sets the left and right margins.
func (m *StyledMixin) Mx(v float32) *StyledMixin { return m.setMargin(v, false, true, false, true) }

// My sets the top and bottom margins.
func (m *StyledMixin) My(v float32) *StyledMixin { return m.setMargin(v, true, false, true, false) }

// Mt sets the top margin.
func (m *StyledMixin) Mt(v float32) *StyledMixin { return m.setMargin(v, true, false, false, false) }

// Mr sets the right margin.
func (m *StyledMixin) Mr(v float32) *StyledMixin { return m.setMargin(v, false, true, false, false) }

// Mb sets the bottom margin.
func (m *StyledMixin) Mb(v float32) *StyledMixin { return m.setMargin(v, false, false, true, false) }

// Ml sets the left margin.
func (m *StyledMixin) Ml(v float32) *StyledMixin { return m.setMargin(v, false, false, false, true) }

// MAuto makes every margin auto (centers the element in its parent).
func (m *StyledMixin) MAuto() *StyledMixin { return m.setMarginAuto(true, true, true, true) }

// MxAuto makes the left and right margins auto.
func (m *StyledMixin) MxAuto() *StyledMixin { return m.setMarginAuto(false, true, false, true) }

// MyAuto makes the top and bottom margins auto.
func (m *StyledMixin) MyAuto() *StyledMixin { return m.setMarginAuto(true, false, true, false) }

// MtAuto makes the top margin auto.
func (m *StyledMixin) MtAuto() *StyledMixin { return m.setMarginAuto(true, false, false, false) }

// MrAuto makes the right margin auto.
func (m *StyledMixin) MrAuto() *StyledMixin { return m.setMarginAuto(false, true, false, false) }

// MbAuto makes the bottom margin auto.
func (m *StyledMixin) MbAuto() *StyledMixin { return m.setMarginAuto(false, false, true, false) }

// MlAuto makes the left margin auto.
func (m *StyledMixin) MlAuto() *StyledMixin { return m.setMarginAuto(false, false, false, true) }

// Bg sets the background fill: an Rgba, an Hsla, or a Background such as
// LinearGradient(...).
func (m *StyledMixin) Bg(f Fill) *StyledMixin {
	var b Background
	if f != nil {
		b = f.ToBackground()
	}
	m.style.Background = &b
	return m
}

// Opacity sets the element's opacity (0..1); it multiplies the alpha of
// everything the element and its descendants paint.
func (m *StyledMixin) Opacity(f float32) *StyledMixin {
	f = clamp32(f, 0, 1)
	m.style.Opacity = &f
	return m
}

// Shadow replaces the element's box shadows.
func (m *StyledMixin) Shadow(shadows ...BoxShadow) *StyledMixin {
	s := append([]BoxShadow(nil), shadows...)
	m.style.BoxShadows = &s
	return m
}

// ShadowNone removes every box shadow.
func (m *StyledMixin) ShadowNone() *StyledMixin { return m.Shadow() }

// Shadow2xs applies GPUI's shadow_2xs preset.
func (m *StyledMixin) Shadow2xs() *StyledMixin { return m.Shadow(shadow2xs...) }

// ShadowXs applies GPUI's shadow_xs preset.
func (m *StyledMixin) ShadowXs() *StyledMixin { return m.Shadow(shadowXs...) }

// ShadowSm applies GPUI's shadow_sm preset.
func (m *StyledMixin) ShadowSm() *StyledMixin { return m.Shadow(shadowSm...) }

// ShadowMd applies GPUI's shadow_md preset.
func (m *StyledMixin) ShadowMd() *StyledMixin { return m.Shadow(shadowMd...) }

// ShadowLg applies GPUI's shadow_lg preset.
func (m *StyledMixin) ShadowLg() *StyledMixin { return m.Shadow(shadowLg...) }

// ShadowXl applies GPUI's shadow_xl preset.
func (m *StyledMixin) ShadowXl() *StyledMixin { return m.Shadow(shadowXl...) }

// Shadow2xl applies GPUI's shadow_2xl preset.
func (m *StyledMixin) Shadow2xl() *StyledMixin { return m.Shadow(shadow2xl...) }

// TextColor sets the text color inherited by descendants.
func (m *StyledMixin) TextColor(c Rgba) *StyledMixin {
	t := m.textStyle()
	t.Color = c
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldColor
	return m
}

// TextSize sets the font size inherited by descendants.
func (m *StyledMixin) TextSize(px float32) *StyledMixin {
	t := m.textStyle()
	t.FontSize = Pixels(px)
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldSize
	return m
}

// FontWeight sets the font weight inherited by descendants.
func (m *StyledMixin) FontWeight(w int) *StyledMixin {
	t := m.textStyle()
	t.FontWeight = w
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldWeight
	return m
}

// FontFamily sets the font family inherited by descendants.
func (m *StyledMixin) FontFamily(name string) *StyledMixin {
	t := m.textStyle()
	t.FontFamily = name
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldFamily
	return m
}

// TextStrikethrough strikes through descendants' text.
func (m *StyledMixin) TextStrikethrough() *StyledMixin {
	t := m.textStyle()
	t.Strikethrough = true
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldStrikethrough
	return m
}

// TextLineHeight sets the line height inherited by descendants.
func (m *StyledMixin) TextLineHeight(px float32) *StyledMixin {
	t := m.textStyle()
	t.LineHeight = Pixels(px)
	m.style.TextStyle = &t
	m.style.TextSet |= TextFieldLineHeight
	return m
}

// Rounded sets all four corner radii.
func (m *StyledMixin) Rounded(px float32) *StyledMixin {
	c := Corners{}.Uniform(px)
	m.style.CornerRadii = &c
	return m
}

// RoundedAll sets the corner radii individually. The parameter order is
// top-left, top-right, bottom-right, bottom-left.
func (m *StyledMixin) RoundedAll(tl, tr, br, bl float32) *StyledMixin {
	c := Corners{
		TopLeft:     Pixels(tl),
		TopRight:    Pixels(tr),
		BottomRight: Pixels(br),
		BottomLeft:  Pixels(bl),
	}
	m.style.CornerRadii = &c
	return m
}

// Border sets a uniform border width and color.
func (m *StyledMixin) Border(w float32, c Rgba) *StyledMixin {
	e := Edges{}.All(w)
	m.style.BorderWidths = &e
	m.style.BorderColor = &c
	return m
}

// ItemsStart aligns children to the start of the cross axis.
func (m *StyledMixin) ItemsStart() *StyledMixin {
	m.style.AlignItems = alignPtr(AlignStart)
	return m
}

// ItemsCenter centers children on the cross axis.
func (m *StyledMixin) ItemsCenter() *StyledMixin {
	m.style.AlignItems = alignPtr(AlignCenter)
	return m
}

// ItemsEnd aligns children to the end of the cross axis.
func (m *StyledMixin) ItemsEnd() *StyledMixin {
	m.style.AlignItems = alignPtr(AlignEnd)
	return m
}

// ItemsStretch stretches children to fill the line's cross size.
func (m *StyledMixin) ItemsStretch() *StyledMixin {
	m.style.AlignItems = alignPtr(AlignStretch)
	return m
}

// JustifyStart packs children at the start of the main axis.
func (m *StyledMixin) JustifyStart() *StyledMixin {
	m.style.Justify = justifyPtr(JustifyStart)
	return m
}

// JustifyCenter centers children on the main axis.
func (m *StyledMixin) JustifyCenter() *StyledMixin {
	m.style.Justify = justifyPtr(JustifyCenter)
	return m
}

// JustifyEnd packs children at the end of the main axis.
func (m *StyledMixin) JustifyEnd() *StyledMixin {
	m.style.Justify = justifyPtr(JustifyEnd)
	return m
}

// JustifyBetween spreads children with equal space between them.
func (m *StyledMixin) JustifyBetween() *StyledMixin {
	m.style.Justify = justifyPtr(JustifySpaceBetween)
	return m
}

// JustifyAround spreads children with half-size gaps at each end.
func (m *StyledMixin) JustifyAround() *StyledMixin {
	m.style.Justify = justifyPtr(JustifySpaceAround)
	return m
}

// OverflowHidden clips painting and makes the element a scroll container for
// layout purposes.
func (m *StyledMixin) OverflowHidden() *StyledMixin {
	m.style.Overflow = overflowPtr(OverflowHidden)
	return m
}

// OverflowScroll is OverflowHidden plus a scroll offset driven by input on
// both axes.
func (m *StyledMixin) OverflowScroll() *StyledMixin { return m.overflowScroll(AxesBoth) }

// OverflowXScroll makes the element a scroll container that scrolls
// horizontally. As in CSS, the other axis is clipped too.
func (m *StyledMixin) OverflowXScroll() *StyledMixin { return m.overflowScroll(AxisX) }

// OverflowYScroll makes the element a scroll container that scrolls
// vertically. As in CSS, the other axis is clipped too.
func (m *StyledMixin) OverflowYScroll() *StyledMixin { return m.overflowScroll(AxisY) }

func (m *StyledMixin) overflowScroll(a Axes) *StyledMixin {
	m.style.Overflow = overflowPtr(OverflowScroll)
	m.style.ScrollAxes = &a
	return m
}

// MinW sets a minimum width.
func (m *StyledMixin) MinW(v float32) *StyledMixin {
	m.style.MinWidth = lengthPtr(Px(v))
	return m
}

// MinH sets a minimum height.
func (m *StyledMixin) MinH(v float32) *StyledMixin {
	m.style.MinHeight = lengthPtr(Px(v))
	return m
}

// MaxW sets a maximum width.
func (m *StyledMixin) MaxW(v float32) *StyledMixin {
	m.style.MaxWidth = lengthPtr(Px(v))
	return m
}

// MaxH sets a maximum height.
func (m *StyledMixin) MaxH(v float32) *StyledMixin {
	m.style.MaxHeight = lengthPtr(Px(v))
	return m
}

func (m *StyledMixin) textStyle() TextStyle {
	if m.style.TextStyle != nil {
		return *m.style.TextStyle
	}
	return DefaultTextStyle()
}

func (e *Edges) unwrap() Edges {
	if e == nil {
		return Edges{}
	}
	return *e
}

func lengthPtr(l Length) *Length               { return &l }
func float32Ptr(v float32) *float32            { return &v }
func boolPtr(v bool) *bool                     { return &v }
func dirPtr(v Direction) *Direction            { return &v }
func alignPtr(v Align) *Align                  { return &v }
func justifyPtr(v Justify) *Justify            { return &v }
func overflowPtr(v OverflowKind) *OverflowKind { return &v }

func edgesAll(v float32) *Edges {
	e := Edges{}.All(v)
	return &e
}
