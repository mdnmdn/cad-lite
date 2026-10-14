package gpui

import "math"

// infPixels stands in for an unbounded axis length. It is only ever compared
// against or clamped — never used as the result of arithmetic that could
// overflow into a NaN.
const infPixels = Pixels(math.MaxFloat32)

// Available is the size constraint handed to a layout pass. A false Bounded
// flag means that axis is indefinite (max-content sizing).
type Available struct {
	W, H     Pixels
	BoundedW bool
	BoundedH bool
}

// Definite returns a constraint with both axes bounded.
func Definite(w, h float32) Available {
	return Available{W: Pixels(w), H: Pixels(h), BoundedW: true, BoundedH: true}
}

// UnboundedHeight returns a constraint with a definite width and an indefinite
// height — what a scroll container offers its children.
func UnboundedHeight(availW float32) Available {
	return Available{W: Pixels(availW), BoundedW: true, BoundedH: false}
}

// UnboundedWidth returns a constraint with a definite height and an indefinite
// width.
func UnboundedWidth(availH float32) Available {
	return Available{H: Pixels(availH), BoundedH: true, BoundedW: false}
}

// LayoutNode is the per-frame box tree the layout engine consumes. It is built
// by the element pipeline each frame and discarded afterwards.
type LayoutNode struct {
	Style    Style
	Children []*LayoutNode

	// Measure sizes a leaf that carries text. maxWidth is the wrap width;
	// pass math.MaxFloat32 for max-content (no wrapping).
	Measure func(text string, ts TextStyle, maxWidth float32) (w, h float32)
	Text    string

	// Outputs, written by Layout.
	Rect        Bounds
	ContentSize Size

	state nodeState
}

// ScrollMax returns how far this node can scroll on each axis.
func (n *LayoutNode) ScrollMax() (Pixels, Pixels) {
	if n == nil {
		return 0, 0
	}
	return maxPixels(0, n.ContentSize.W-n.Rect.Size.W), maxPixels(0, n.ContentSize.H-n.Rect.Size.H)
}

// Layout sizes the tree and assigns every node its parent-relative border box.
// The root is forced to available on each bounded axis whose Length is not
// LengthPx; every other Auto axis sizes to its content. It returns the root's
// border box.
func Layout(n *LayoutNode, available Available) Bounds {
	if n == nil {
		return Bounds{}
	}
	resetLayoutCache(n)
	var f forcedSize
	if available.BoundedW && !n.Style.Width.IsPx() {
		f.w, f.hasW = available.W, true
	}
	if available.BoundedH && !n.Style.Height.IsPx() {
		f.h, f.hasH = available.H, true
	}
	size := measureNode(n, available, f)
	n.Rect = Bounds{Size: size}
	arrangeNode(n)
	return n.Rect
}

// forcedSize carries border-box sizes the parent has already decided for a
// node (stretch, flex resolution). A forced axis is definite; any other axis
// sizes to content within the available space.
type forcedSize struct {
	w, h       Pixels
	hasW, hasH bool
}

func forcedAxes(main Pixels, hasMain bool, cross Pixels, hasCross bool, isRow bool) forcedSize {
	if isRow {
		return forcedSize{w: main, hasW: hasMain, h: cross, hasH: hasCross}
	}
	return forcedSize{w: cross, hasW: hasCross, h: main, hasH: hasMain}
}

type measureKey struct {
	avail  Available
	forced forcedSize
}

type measureEntry struct {
	key  measureKey
	size Size
}

// nodeState is the per-node measure cache. It is only valid for one Layout
// call: sizes depend solely on (available, forced), so repeated measurements
// during flex resolution are free.
type nodeState struct {
	cache []measureEntry
}

func resetLayoutCache(n *LayoutNode) {
	n.state.cache = n.state.cache[:0]
	for _, c := range n.Children {
		if c != nil {
			resetLayoutCache(c)
		}
	}
}

func measureNode(n *LayoutNode, avail Available, f forcedSize) Size {
	if n == nil || !n.Style.Display {
		return Size{}
	}
	key := measureKey{avail: avail, forced: f}
	for _, e := range n.state.cache {
		if e.key == key {
			return e.size
		}
	}
	var size Size
	if len(n.Children) == 0 {
		size = measureLeaf(n, avail, f)
	} else {
		size = layoutFlex(n, avail, f, false)
	}
	n.state.cache = append(n.state.cache, measureEntry{key: key, size: size})
	return size
}

// ownSize resolves a node's border-box size on one axis when it is definite:
// forced by the parent, or a LengthPx preferred size.
func ownSize(forced Pixels, hasForced bool, pref, lo, hi Length) (Pixels, bool) {
	if hasForced {
		return forced, true
	}
	if pref.IsPx() {
		return clampLength(pref.Val, lo, hi), true
	}
	return 0, false
}

func measureLeaf(n *LayoutNode, avail Available, f forcedSize) Size {
	s := n.Style
	padW, padH := s.Padding.Horizontal(), s.Padding.Vertical()
	w, hasW := ownSize(f.w, f.hasW, s.Width, s.MinWidth, s.MaxWidth)
	h, hasH := ownSize(f.h, f.hasH, s.Height, s.MinHeight, s.MaxHeight)
	var mw, mh Pixels
	if n.Measure != nil && n.Text != "" {
		wrap := infPixels
		switch {
		case hasW:
			wrap = maxPixels(0, w-padW)
		case avail.BoundedW:
			wrap = maxPixels(0, avail.W-padW)
		}
		a, b := n.Measure(n.Text, s.TextStyle, float32(wrap))
		mw, mh = Pixels(a), Pixels(b)
	}
	if !hasW {
		w = clampLength(mw+padW, s.MinWidth, s.MaxWidth)
	}
	if !hasH {
		h = clampLength(mh+padH, s.MinHeight, s.MaxHeight)
	}
	return Size{W: w, H: h}
}

// minContentWidth is a node's min-content width contribution: its LengthPx
// width if set, otherwise the narrowest width its content can take without
// overflowing (the longest word for text).
func minContentWidth(n *LayoutNode) Pixels {
	if n == nil || !n.Style.Display {
		return 0
	}
	s := n.Style
	if s.Width.IsPx() {
		return clampLength(s.Width.Val, s.MinWidth, s.MaxWidth)
	}
	var v Pixels
	switch {
	case len(n.Children) == 0:
		if n.Measure != nil && n.Text != "" {
			w, _ := n.Measure(n.Text, s.TextStyle, 0)
			v = Pixels(w)
		}
	default:
		isRow := s.Direction == DirectionRow
		k := 0
		for _, c := range n.Children {
			if c == nil || !c.Style.Display || c.Style.Position == PositionAbsolute {
				continue
			}
			k++
			var cw Pixels
			if c.Style.isScrollContainer() && !c.Style.Width.IsPx() {
				cw = resolveBound(c.Style.MinWidth, 0)
			} else {
				cw = minContentWidth(c)
			}
			ms, me, _, _ := c.Style.margins(true)
			cw += ms + me
			if isRow && !s.Wrap {
				v += cw
			} else {
				v = maxPixels(v, cw)
			}
		}
		if isRow && !s.Wrap && k > 1 {
			v += s.mainGap() * Pixels(k-1)
		}
	}
	return clampLength(v+s.Padding.Horizontal(), s.MinWidth, s.MaxWidth)
}

// flexItem is one visible child's working state during a flex pass.
type flexItem struct {
	node              *LayoutNode
	align             Align
	fillCross         bool
	forcedCross       Pixels
	hasForcedCross    bool
	cross, mainPos    Pixels
	crossMin, crossMx Pixels
	crossPos          Pixels
	// Margins: fixed main/cross start and end (auto edges count 0) and which
	// edges are auto.
	mStart, mEnd, cStart, cEnd Pixels
	autoMS, autoME             bool
	autoCS, autoCE             bool
}

// margins returns the left/right margins when horizontal, else the
// top/bottom ones, with auto edges reported separately and counted as 0.
func (s Style) margins(horizontal bool) (start, end Pixels, autoStart, autoEnd bool) {
	m, a := s.Margin, s.MarginAuto
	if horizontal {
		start, end, autoStart, autoEnd = m.Left, m.Right, a.Left, a.Right
	} else {
		start, end, autoStart, autoEnd = m.Top, m.Bottom, a.Top, a.Bottom
	}
	if autoStart {
		start = 0
	}
	if autoEnd {
		end = 0
	}
	return start, end, autoStart, autoEnd
}

// layoutFlex runs the flexbox algorithm for a container. With place=false it
// only returns the border-box size; with place=true (called from arrange with
// the node's final size forced) it also writes the children's parent-relative
// Rects, arranges them recursively and sets ContentSize.
func layoutFlex(n *LayoutNode, avail Available, f forcedSize, place bool) Size {
	if n.Style.Wrap {
		return layoutWrapped(n, avail, f, place)
	}
	s := n.Style
	isRow := s.Direction == DirectionRow
	pad := s.Padding

	defW, hasW := ownSize(f.w, f.hasW, s.Width, s.MinWidth, s.MaxWidth)
	defH, hasH := ownSize(f.h, f.hasH, s.Height, s.MinHeight, s.MaxHeight)

	// Inner (content-box) space per axis: definite, or a maximum, or neither.
	innerOf := func(def Pixels, hasDef bool, av Pixels, bounded bool, padding Pixels) (Pixels, bool, bool) {
		if hasDef {
			return maxPixels(0, def-padding), true, true
		}
		if bounded {
			return maxPixels(0, av-padding), false, true
		}
		return 0, false, false
	}
	innerW, defInnerW, boundInnerW := innerOf(defW, hasW, avail.W, avail.BoundedW, pad.Horizontal())
	innerH, defInnerH, boundInnerH := innerOf(defH, hasH, avail.H, avail.BoundedH, pad.Vertical())

	var mainInner, crossInner Pixels
	var mainDef, crossDef, mainBounded, crossBounded bool
	if isRow {
		mainInner, mainDef, mainBounded = innerW, defInnerW, boundInnerW
		crossInner, crossDef, crossBounded = innerH, defInnerH, boundInnerH
	} else {
		mainInner, mainDef, mainBounded = innerH, defInnerH, boundInnerH
		crossInner, crossDef, crossBounded = innerW, defInnerW, boundInnerW
	}

	var items []flexItem
	for _, c := range n.Children {
		if c == nil {
			continue
		}
		if !c.Style.Display {
			if place {
				c.Rect = Bounds{}
				c.ContentSize = Size{}
			}
			continue
		}
		if c.Style.Position == PositionAbsolute {
			continue
		}
		items = append(items, flexItem{node: c})
	}
	k := len(items)
	var gapsTotal Pixels
	if k > 1 {
		gapsTotal = s.mainGap() * Pixels(k-1)
	}

	// The space an item sees on the cross axis (a maximum unless definite,
	// less its cross margins) and on the main axis (unbounded: base sizes are
	// max-content).
	childAvail := func(main Pixels, mainBound bool, crossMargin Pixels) Available {
		a := Available{}
		cross := maxPixels(0, crossInner-crossMargin)
		if isRow {
			a.W, a.BoundedW = main, mainBound
			a.H, a.BoundedH = cross, crossBounded
		} else {
			a.H, a.BoundedH = main, mainBound
			a.W, a.BoundedW = cross, crossBounded
		}
		return a
	}

	st := &flexState{
		base: make([]Pixels, k), hyp: make([]Pixels, k),
		min: make([]Pixels, k), max: make([]Pixels, k),
		grow: make([]float32, k), shrink: make([]float32, k),
		target: make([]Pixels, k),
	}
	contrib := make([]Pixels, k)
	var marginsMain Pixels

	// Step 1: flex base size, automatic minimum, hypothetical main size.
	for i := range items {
		it := &items[i]
		cs := it.node.Style
		it.align = cs.AlignSelf
		if it.align == AlignSelfAuto {
			it.align = s.AlignItems
		}
		it.mStart, it.mEnd, it.autoMS, it.autoME = cs.margins(isRow)
		it.cStart, it.cEnd, it.autoCS, it.autoCE = cs.margins(!isRow)
		marginsMain += it.mStart + it.mEnd
		cm := it.cStart + it.cEnd
		unboundedMain := childAvail(0, false, cm)
		crossLen := cs.resolveCross(isRow)
		it.crossMin = resolveBound(cs.resolveMinCross(isRow), 0)
		it.crossMx = resolveBound(cs.resolveMaxCross(isRow), infPixels)
		it.fillCross = crossLen.IsFlex() || (crossLen.IsAuto() && it.align == AlignStretch && !it.autoCS && !it.autoCE)
		if it.fillCross && crossDef {
			it.forcedCross, it.hasForcedCross = clampLen(crossInner-cm, it.crossMin, it.crossMx), true
		}
		baseForced := forcedAxes(0, false, it.forcedCross, it.hasForcedCross, isRow)

		maxMain := resolveBound(cs.resolveMaxMain(isRow), infPixels)
		var minMain Pixels
		switch {
		case cs.resolveMinMain(isRow).IsPx():
			minMain = cs.resolveMinMain(isRow).Val
		case cs.isScrollContainer():
			minMain = 0
		case cs.resolveMain(isRow).IsPx():
			minMain = cs.resolveMain(isRow).Val
		case isRow:
			minMain = minContentWidth(it.node)
		default:
			minMain = measureNode(it.node, unboundedMain, baseForced).H
		}
		minMain = minPixels(minMain, maxMain)

		mainLen := cs.resolveMain(isRow)
		var base Pixels
		switch {
		case mainLen.IsFlex():
			base = 0
			st.grow[i], st.shrink[i] = float32(mainLen.Val), 1
		case cs.FlexBasis.IsPx():
			base = cs.FlexBasis.Val
			st.grow[i], st.shrink[i] = cs.FlexGrow, cs.FlexShrink
		case mainLen.IsPx():
			base = mainLen.Val
			st.grow[i], st.shrink[i] = cs.FlexGrow, cs.FlexShrink
		default:
			base = measureNode(it.node, unboundedMain, baseForced).axis(isRow)
			st.grow[i], st.shrink[i] = cs.FlexGrow, cs.FlexShrink
		}
		st.base[i], st.min[i], st.max[i] = base, minMain, maxMain
		st.hyp[i] = clampLen(base, minMain, maxMain)
		contrib[i] = st.hyp[i]
		if mainLen.IsFlex() || cs.FlexBasis.IsPx() {
			content := mainLen.Val
			if !mainLen.IsPx() {
				content = measureNode(it.node, unboundedMain, baseForced).axis(isRow)
			}
			contrib[i] = maxPixels(contrib[i], clampLen(content, minMain, maxMain))
		}
	}

	// Step 2: the container's inner main size.
	M := mainInner
	if !mainDef {
		var sum, mins Pixels
		for i := range items {
			sum += contrib[i]
			mins += st.min[i]
		}
		M = sum + gapsTotal + marginsMain
		if mainBounded && M > mainInner {
			M = maxPixels(mainInner, mins+gapsTotal+marginsMain)
		}
		mp := s.mainPadding(isRow)
		M = maxPixels(0, clampLength(M+mp, s.resolveMinMain(isRow), s.resolveMaxMain(isRow))-mp)
	}

	// Step 3: resolve flexible lengths.
	resolveFlex(st, M, gapsTotal+marginsMain)

	// Step 4: item cross sizes at their final main sizes, then the line.
	for i := range items {
		it := &items[i]
		cs := it.node.Style
		crossLen := cs.resolveCross(isRow)
		switch {
		case crossLen.IsPx():
			it.cross = clampLen(crossLen.Val, it.crossMin, it.crossMx)
		case it.hasForcedCross:
			it.cross = it.forcedCross
		default:
			ff := forcedAxes(st.target[i], true, 0, false, isRow)
			it.cross = clampLen(measureNode(it.node, childAvail(st.target[i], true, it.cStart+it.cEnd), ff).axis(!isRow), it.crossMin, it.crossMx)
		}
	}
	lineCross := crossInner
	if !crossDef {
		lineCross = 0
		for i := range items {
			lineCross = maxPixels(lineCross, items[i].cross+items[i].cStart+items[i].cEnd)
		}
		cp := s.crossPadding(isRow)
		lineCross = maxPixels(0, clampLength(lineCross+cp, s.resolveMinCross(isRow), s.resolveMaxCross(isRow))-cp)
		for i := range items {
			it := &items[i]
			if it.fillCross {
				it.cross = clampLen(lineCross-it.cStart-it.cEnd, it.crossMin, it.crossMx)
			}
		}
	}

	var size Size
	borderMain := M + s.mainPadding(isRow)
	borderCross := lineCross + s.crossPadding(isRow)
	if isRow {
		size = Size{W: borderMain, H: borderCross}
		if hasW {
			size.W = defW
		}
		if hasH {
			size.H = defH
		}
	} else {
		size = Size{W: borderCross, H: borderMain}
		if hasW {
			size.W = defW
		}
		if hasH {
			size.H = defH
		}
	}
	if !place {
		return size
	}

	// Step 5: main-axis placement.
	var used Pixels
	autos := 0
	for i := range items {
		used += st.target[i]
		if items[i].autoMS {
			autos++
		}
		if items[i].autoME {
			autos++
		}
	}
	free := M - used - gapsTotal - marginsMain
	var perAuto Pixels
	if autos > 0 && free > 0 {
		perAuto = free / Pixels(autos)
		free = 0
	}
	mode := s.Justify
	if k <= 1 || free <= 0 {
		switch mode {
		case JustifySpaceBetween:
			mode = JustifyStart
		case JustifySpaceAround:
			mode = JustifyCenter
		}
	}
	pos := s.mainPaddingStart(isRow) + firstOffset(mode, free, k)
	between := betweenOffset(mode, free, k)
	for i := range items {
		it := &items[i]
		pos += it.mStart
		if it.autoMS {
			pos += perAuto
		}
		it.mainPos = pos
		pos += st.target[i] + it.mEnd + s.mainGap() + between
		if it.autoME {
			pos += perAuto
		}
	}

	// Step 6: cross-axis placement, child rects, recursion, content size.
	extentMain, extentCross := size.axis(isRow), size.axis(!isRow)
	for i := range items {
		it := &items[i]
		cfree := lineCross - it.cStart - it.cEnd - it.cross
		var off Pixels
		switch {
		case it.autoCS || it.autoCE:
			if cfree > 0 {
				switch {
				case it.autoCS && it.autoCE:
					off = cfree / 2
				case it.autoCS:
					off = cfree
				}
			}
		case it.align == AlignCenter:
			off = cfree / 2
		case it.align == AlignEnd:
			off = cfree
		}
		it.crossPos = s.crossPaddingStart(isRow) + it.cStart + off

		c := it.node
		if isRow {
			c.Rect = Bounds{Origin: Point{X: it.mainPos, Y: it.crossPos}, Size: Size{W: st.target[i], H: it.cross}}
		} else {
			c.Rect = Bounds{Origin: Point{X: it.crossPos, Y: it.mainPos}, Size: Size{W: it.cross, H: st.target[i]}}
		}
		arrangeNode(c)

		endMain := it.mainPos + st.target[i] + it.mEnd
		endCross := it.crossPos + it.cross + it.cEnd
		if c.Style.Overflow == OverflowVisible {
			endMain += maxPixels(0, c.ContentSize.axis(isRow)-c.Rect.Size.axis(isRow))
			endCross += maxPixels(0, c.ContentSize.axis(!isRow)-c.Rect.Size.axis(!isRow))
		}
		extentMain = maxPixels(extentMain, endMain+s.mainPaddingEnd(isRow))
		extentCross = maxPixels(extentCross, endCross+s.crossPaddingEnd(isRow))
	}
	n.ContentSize = mapAxis(extentMain, extentCross, isRow)
	layoutAbsoluteChildren(n)
	return size
}

func layoutAbsoluteChildren(n *LayoutNode) {
	size := n.Rect.Size
	isRow := n.Style.Direction == DirectionRow
	extentMain, extentCross := n.ContentSize.axis(isRow), n.ContentSize.axis(!isRow)
	for _, c := range n.Children {
		if c == nil || !c.Style.Display || c.Style.Position != PositionAbsolute {
			continue
		}
		layoutAbsolute(c, size)
		end := c.Rect.Max()
		mr, mb := c.Style.Margin.Right, c.Style.Margin.Bottom
		if c.Style.MarginAuto.Right {
			mr = 0
		}
		if c.Style.MarginAuto.Bottom {
			mb = 0
		}
		end.X += mr
		end.Y += mb
		if c.Style.Overflow == OverflowVisible {
			end.X += maxPixels(0, c.ContentSize.W-c.Rect.Size.W)
			end.Y += maxPixels(0, c.ContentSize.H-c.Rect.Size.H)
		}
		extentMain = maxPixels(extentMain, Size{W: end.X, H: end.Y}.axis(isRow))
		extentCross = maxPixels(extentCross, Size{W: end.X, H: end.Y}.axis(!isRow))
	}
	n.ContentSize = mapAxis(extentMain, extentCross, isRow)
}

func absoluteAxis(parent Pixels, start, end, pref, lo, hi Length, ms, me Pixels) (Pixels, Pixels, bool) {
	available := parent - ms - me
	if start.IsPx() {
		available -= start.Val
	}
	if end.IsPx() {
		available -= end.Val
	}
	available = maxPixels(0, available)
	if pref.IsPx() {
		return available, 0, false
	}
	if start.IsPx() && end.IsPx() {
		return available, clampLength(available, lo, hi), true
	}
	return available, 0, false
}

func absoluteOrigin(parent, size Pixels, start, end Length, ms, me Pixels) Pixels {
	if start.IsPx() {
		return start.Val + ms
	}
	if end.IsPx() {
		return parent - end.Val - me - size
	}
	return ms
}

func layoutAbsolute(n *LayoutNode, parent Size) {
	s := n.Style
	ml, mr, _, _ := s.margins(true)
	mt, mb, _, _ := s.margins(false)
	aw, fw, hasW := absoluteAxis(parent.W, s.Left, s.Right, s.Width, s.MinWidth, s.MaxWidth, ml, mr)
	ah, fh, hasH := absoluteAxis(parent.H, s.Top, s.Bottom, s.Height, s.MinHeight, s.MaxHeight, mt, mb)
	size := measureNode(n, Definite(float32(aw), float32(ah)), forcedSize{w: fw, h: fh, hasW: hasW, hasH: hasH})
	n.Rect = Bounds{Origin: Point{
		X: absoluteOrigin(parent.W, size.W, s.Left, s.Right, ml, mr),
		Y: absoluteOrigin(parent.H, size.H, s.Top, s.Bottom, mt, mb),
	}, Size: size}
	arrangeNode(n)
}

// arrangeNode lays out n's children inside n.Rect, which the caller has set.
func arrangeNode(n *LayoutNode) {
	if n == nil || !n.Style.Display {
		return
	}
	if len(n.Children) == 0 {
		n.ContentSize = n.Rect.Size
		return
	}
	sz := n.Rect.Size
	layoutFlex(n, Definite(float32(sz.W), float32(sz.H)), forcedSize{w: sz.W, h: sz.H, hasW: true, hasH: true}, true)
}

func firstOffset(mode Justify, free Pixels, k int) Pixels {
	switch mode {
	case JustifyEnd:
		return free
	case JustifyCenter:
		return free / 2
	case JustifySpaceAround:
		if free >= 0 && k > 0 {
			return (free / Pixels(k)) / 2
		}
		return free / 2
	default:
		return 0
	}
}

func betweenOffset(mode Justify, free Pixels, k int) Pixels {
	if k < 2 || free <= 0 {
		return 0
	}
	switch mode {
	case JustifySpaceBetween:
		return free / Pixels(k-1)
	case JustifySpaceAround:
		return free / Pixels(k)
	default:
		return 0
	}
}

// flexState is the working set of CSS §9.7 flexible-length resolution.
type flexState struct {
	base, hyp, min, max []Pixels
	grow, shrink        []float32
	target              []Pixels
}

// resolveFlex implements CSS flexbox §9.7.
func resolveFlex(st *flexState, M Pixels, gapsTotal Pixels) {
	k := len(st.base)
	if k == 0 {
		return
	}
	var S Pixels
	for i := range st.base {
		S += st.hyp[i]
	}
	S += gapsTotal

	grow := S <= M
	frozen := make([]bool, k)
	for i := range st.base {
		switch {
		case grow && st.grow[i] == 0:
			frozen[i] = true
		case grow && st.base[i] > st.hyp[i]:
			frozen[i] = true
		case !grow && st.shrink[i] == 0:
			frozen[i] = true
		case !grow && st.base[i] < st.hyp[i]:
			frozen[i] = true
		}
		if frozen[i] {
			st.target[i] = st.hyp[i]
		}
	}

	var initialFree Pixels
	for i := range st.base {
		if frozen[i] {
			initialFree -= st.hyp[i]
		} else {
			initialFree -= st.base[i]
		}
	}
	initialFree = M - initialFree - gapsTotal

	viol := make([]Pixels, k)
	for iter := 0; iter <= k; iter++ {
		allFrozen := true
		for i := range frozen {
			if !frozen[i] {
				allFrozen = false
				break
			}
		}
		if allFrozen {
			break
		}

		free := M - gapsTotal
		var sumf float32
		for i := range st.base {
			if !frozen[i] {
				st.target[i] = st.base[i]
			}
			if frozen[i] {
				free -= st.target[i]
			} else {
				free -= st.base[i]
				if grow {
					sumf += st.grow[i]
				} else {
					sumf += st.shrink[i]
				}
			}
		}
		if sumf < 1 {
			cand := initialFree * Pixels(sumf)
			if absPixels(cand) < absPixels(free) {
				free = cand
			}
		}

		if free != 0 && sumf != 0 {
			if grow {
				for i := range st.base {
					if frozen[i] {
						continue
					}
					st.target[i] = st.base[i] + free*Pixels(st.grow[i]/sumf)
				}
			} else {
				scaled := make([]Pixels, k)
				var sumScaled Pixels
				for i := range st.base {
					if frozen[i] {
						continue
					}
					scaled[i] = Pixels(st.shrink[i]) * st.base[i]
					sumScaled += scaled[i]
				}
				if sumScaled != 0 {
					for i := range st.base {
						if frozen[i] {
							continue
						}
						st.target[i] = st.base[i] - absPixels(free)*(scaled[i]/sumScaled)
					}
				}
			}
		}

		var total Pixels
		for i := range st.base {
			if frozen[i] {
				viol[i] = 0
				continue
			}
			t := clampLen(st.target[i], st.min[i], st.max[i])
			viol[i] = t - st.target[i]
			st.target[i] = t
			total += viol[i]
		}

		if total == 0 {
			for i := range st.base {
				frozen[i] = true
			}
			break
		}
		froze := false
		want := Pixels(0)
		if total > 0 {
			want = 1
		} else if total < 0 {
			want = -1
		}
		for i := range st.base {
			if frozen[i] {
				continue
			}
			if (want > 0 && viol[i] > 0) || (want < 0 && viol[i] < 0) {
				frozen[i] = true
				froze = true
			}
		}
		if !froze {
			for i := range st.base {
				frozen[i] = true
			}
		}
	}
}

func (s Size) axis(isRow bool) Pixels {
	if isRow {
		return s.W
	}
	return s.H
}

func mapAxis(main, cross Pixels, isRow bool) Size {
	if isRow {
		return Size{W: main, H: cross}
	}
	return Size{W: cross, H: main}
}

func clampLength(v Pixels, lo, hi Length) Pixels {
	if lo.IsPx() {
		v = maxPixels(v, lo.Val)
	}
	if hi.IsPx() {
		v = minPixels(v, hi.Val)
	}
	return maxPixels(v, 0)
}

// resolveBound turns a Length bound into a concrete Pixels bound. autoBound
// supplies the value used when the Length is content-based (0 for a minimum,
// +infinity for a maximum).
func resolveBound(l Length, autoBound Pixels) Pixels {
	if l.IsPx() {
		return l.Val
	}
	return autoBound
}

func clampLen(v, lo, hi Pixels) Pixels {
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return maxPixels(v, 0)
}

func absPixels(v Pixels) Pixels {
	if v < 0 {
		return -v
	}
	return v
}
