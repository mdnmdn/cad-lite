package gpui

func (s Style) axisGap(horizontal bool) Pixels {
	l := s.RowGap
	if horizontal {
		l = s.ColumnGap
	}
	if l.IsPx() {
		return l.Val
	}
	return s.Gap
}

func (s Style) mainGap() Pixels { return s.axisGap(s.Direction == DirectionRow) }

func wrappedHypothetical(c *LayoutNode, avail Available, isRow bool) Pixels {
	s := c.Style
	maxMain := resolveBound(s.resolveMaxMain(isRow), infPixels)
	var minMain Pixels
	switch {
	case s.resolveMinMain(isRow).IsPx():
		minMain = s.resolveMinMain(isRow).Val
	case s.isScrollContainer():
	case s.resolveMain(isRow).IsPx():
		minMain = s.resolveMain(isRow).Val
	case isRow:
		minMain = minContentWidth(c)
	default:
		minMain = measureNode(c, avail, forcedSize{}).H
	}
	minMain = minPixels(minMain, maxMain)
	var base Pixels
	switch main := s.resolveMain(isRow); {
	case main.IsFlex():
	case s.FlexBasis.IsPx():
		base = s.FlexBasis.Val
	case main.IsPx():
		base = main.Val
	default:
		base = measureNode(c, avail, forcedSize{}).axis(isRow)
	}
	ms, me, _, _ := s.margins(isRow)
	return clampLen(base, minMain, maxMain) + ms + me
}

func wrapLines(s Style, kids []*LayoutNode, limit Pixels, bounded bool, cross Pixels, crossBounded bool) []*LayoutNode {
	isRow := s.Direction == DirectionRow
	lineStyle := s
	lineStyle.Wrap = false
	lineStyle.Width, lineStyle.Height = Auto(), Auto()
	lineStyle.MinWidth, lineStyle.MinHeight, lineStyle.MaxWidth, lineStyle.MaxHeight = Auto(), Auto(), Auto(), Auto()
	lineStyle.Padding, lineStyle.Margin, lineStyle.MarginAuto = Edges{}, Edges{}, AutoEdges{}
	lineStyle.Position, lineStyle.Overflow = PositionRelative, OverflowVisible
	lineStyle.Gap = s.mainGap()
	lineStyle.RowGap, lineStyle.ColumnGap = Auto(), Auto()
	var lines []*LayoutNode
	var used Pixels
	for _, c := range kids {
		if c == nil || !c.Style.Display || c.Style.Position == PositionAbsolute {
			continue
		}
		cs, ce, _, _ := c.Style.margins(!isRow)
		a := Available{}
		if isRow {
			a.H, a.BoundedH = maxPixels(0, cross-cs-ce), crossBounded
		} else {
			a.W, a.BoundedW = maxPixels(0, cross-cs-ce), crossBounded
		}
		size := wrappedHypothetical(c, a, isRow)
		if len(lines) == 0 || (bounded && len(lines[len(lines)-1].Children) > 0 && used+s.mainGap()+size > limit) {
			lines = append(lines, &LayoutNode{Style: lineStyle})
			used = 0
		}
		line := lines[len(lines)-1]
		if len(line.Children) > 0 {
			used += s.mainGap()
		}
		line.Children = append(line.Children, c)
		used += size
	}
	return lines
}

func wrappedInner(def Pixels, hasDef bool, available Pixels, bounded bool, padding Pixels, maximum Length) (Pixels, bool) {
	if hasDef {
		return maxPixels(0, def-padding), true
	}
	if maximum.IsPx() && (!bounded || maximum.Val < available) {
		available, bounded = maximum.Val, true
	}
	return maxPixels(0, available-padding), bounded
}

func layoutWrapped(n *LayoutNode, avail Available, f forcedSize, place bool) Size {
	s := n.Style
	isRow := s.Direction == DirectionRow
	w, hasW := ownSize(f.w, f.hasW, s.Width, s.MinWidth, s.MaxWidth)
	h, hasH := ownSize(f.h, f.hasH, s.Height, s.MinHeight, s.MaxHeight)
	innerW, boundW := wrappedInner(w, hasW, avail.W, avail.BoundedW, s.Padding.Horizontal(), s.MaxWidth)
	innerH, boundH := wrappedInner(h, hasH, avail.H, avail.BoundedH, s.Padding.Vertical(), s.MaxHeight)
	limit, crossLimit, mainDef, crossDef, mainBound, crossBound := innerH, innerW, hasH, hasW, boundH, boundW
	if isRow {
		limit, crossLimit, mainDef, crossDef, mainBound, crossBound = innerW, innerH, hasW, hasH, boundW, boundH
	}
	lines := wrapLines(s, n.Children, limit, mainBound, crossLimit, crossBound)
	M := limit
	lineAvail := func(main Pixels, mainBounded bool) Available {
		if isRow {
			return Available{W: main, BoundedW: mainBounded, H: crossLimit, BoundedH: crossBound}
		}
		return Available{H: main, BoundedH: mainBounded, W: crossLimit, BoundedW: crossBound}
	}
	if !mainDef {
		M = 0
		for _, line := range lines {
			M = maxPixels(M, measureNode(line, lineAvail(0, false), forcedSize{}).axis(isRow))
		}
		if mainBound {
			M = minPixels(M, limit)
		}
		M = maxPixels(0, clampLength(M+s.mainPadding(isRow), s.resolveMinMain(isRow), s.resolveMaxMain(isRow))-s.mainPadding(isRow))
		lines = wrapLines(s, n.Children, M, true, crossLimit, crossBound)
	}
	crossGap := s.axisGap(!isRow)
	crossSizes := make([]Pixels, len(lines))
	var naturalCross Pixels
	for i, line := range lines {
		crossSizes[i] = measureNode(line, lineAvail(M, true), forcedAxes(M, true, 0, false, isRow)).axis(!isRow)
		naturalCross += crossSizes[i]
	}
	if len(lines) > 1 {
		naturalCross += crossGap * Pixels(len(lines)-1)
	}
	C := crossLimit
	if !crossDef {
		C = maxPixels(0, clampLength(naturalCross+s.crossPadding(isRow), s.resolveMinCross(isRow), s.resolveMaxCross(isRow))-s.crossPadding(isRow))
	}
	size := mapAxis(M+s.mainPadding(isRow), C+s.crossPadding(isRow), isRow)
	if !place {
		return size
	}
	for _, c := range n.Children {
		if c != nil && !c.Style.Display {
			c.Rect, c.ContentSize = Bounds{}, Size{}
		}
	}
	free := C - naturalCross
	mode := JustifyStart
	switch s.AlignContent {
	case AlignContentCenter:
		mode = JustifyCenter
	case AlignContentEnd:
		mode = JustifyEnd
	case AlignContentSpaceBetween:
		mode = JustifySpaceBetween
	case AlignContentSpaceAround:
		mode = JustifySpaceAround
	}
	if free <= 0 || len(lines) <= 1 {
		if mode == JustifySpaceBetween {
			mode = JustifyStart
		}
		if mode == JustifySpaceAround {
			mode = JustifyCenter
		}
	}
	pos := s.crossPaddingStart(isRow) + firstOffset(mode, free, len(lines))
	between := betweenOffset(mode, free, len(lines))
	var stretch Pixels
	if s.AlignContent == AlignContentStretch && free > 0 && len(lines) > 0 {
		stretch = free / Pixels(len(lines))
	}
	extentMain, extentCross := size.axis(isRow), size.axis(!isRow)
	for i, line := range lines {
		lc := crossSizes[i] + stretch
		line.Rect = Bounds{Size: mapAxis(M, lc, isRow)}
		layoutFlex(line, lineAvail(M, true), forcedAxes(M, true, lc, true, isRow), true)
		for _, c := range line.Children {
			if isRow {
				c.Rect.Origin.X += s.Padding.Left
				c.Rect.Origin.Y += pos
			} else {
				c.Rect.Origin.Y += s.Padding.Top
				c.Rect.Origin.X += pos
			}
		}
		extentMain = maxPixels(extentMain, line.ContentSize.axis(isRow)+s.mainPadding(isRow))
		extentCross = maxPixels(extentCross, pos+line.ContentSize.axis(!isRow)+s.crossPaddingEnd(isRow))
		pos += lc + crossGap + between
	}
	n.ContentSize = mapAxis(extentMain, extentCross, isRow)
	layoutAbsoluteChildren(n)
	return size
}
