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

// Layout runs the two-pass flexbox algorithm: measure sizes the tree, then
// arrange assigns final positions. It returns the root's border box.
func Layout(n *LayoutNode, available Available) Bounds {
	if n == nil {
		return Bounds{}
	}
	size := measureNode(n, available)
	n.Rect = Bounds{Size: size}
	arrangeNode(n)
	return n.Rect
}

// nodeState caches the per-node intermediate results that arrange reads back.
// It is recomputed by every measureNode call.
type nodeState struct {
	isRow bool

	base, hyp, min, max   []Pixels
	grow, shrink          []float32
	target                []Pixels
	hypCross, crossTarget []Pixels
	crossMin, crossMax    []Pixels
	mainPos, crossPos     []Pixels
	needReMeasure         []bool

	usedMain, usedCross Pixels
}

func measureNode(n *LayoutNode, avail Available) Size {
	if n == nil || !n.Style.Display {
		n.clearState()
		return Size{}
	}
	if len(n.Children) == 0 {
		n.clearState()
		return measureLeaf(n, avail)
	}
	return measureFlex(n, avail)
}

func (n *LayoutNode) clearState() {
	n.state = nodeState{}
	n.ContentSize = n.Rect.Size
}

func measureLeaf(n *LayoutNode, avail Available) Size {
	var w, h Pixels
	if n.Measure != nil && n.Text != "" {
		wrapW := resolveWrapWidth(n, avail)
		mw, mh := n.Measure(n.Text, n.Style.TextStyle, float32(wrapW))
		w, h = Pixels(mw), Pixels(mh)
	}
	if n.Style.Width.IsPx() {
		w = n.Style.Width.Val
	}
	if n.Style.Height.IsPx() {
		h = n.Style.Height.Val
	}
	w = clampLength(w, n.Style.MinWidth, n.Style.MaxWidth)
	h = clampLength(h, n.Style.MinHeight, n.Style.MaxHeight)
	return Size{W: w, H: h}
}

func resolveWrapWidth(n *LayoutNode, avail Available) Pixels {
	if n.Style.Width.IsPx() {
		return n.Style.Width.Val
	}
	if avail.BoundedW {
		return avail.W
	}
	return infPixels
}

func measureFlex(n *LayoutNode, avail Available) Size {
	isRow := n.Style.Direction == DirectionRow
	pad := n.Style.Padding

	innerW, boundedW := avail.W, avail.BoundedW
	innerH, boundedH := avail.H, avail.BoundedH
	if boundedW {
		innerW = maxPixels(0, innerW-pad.Horizontal())
	}
	if boundedH {
		innerH = maxPixels(0, innerH-pad.Vertical())
	}

	availMain, boundedMain := pickAxis(innerW, boundedW, innerH, boundedH, isRow)
	availCross, boundedCross := pickAxis(innerW, boundedW, innerH, boundedH, !isRow)

	// A scroll container offers its children an indefinite main axis so they
	// size to content and overflow, which is what makes them scrollable.
	childBoundedMain := boundedMain
	if n.Style.isScrollContainer() {
		childBoundedMain = false
	}

	k := len(n.Children)
	gap := n.Style.Gap
	var gapsTotal Pixels
	if k > 1 {
		gapsTotal = gap * Pixels(k-1)
	}

	st := &n.state
	st.isRow = isRow
	st.base = make([]Pixels, k)
	st.hyp = make([]Pixels, k)
	st.min = make([]Pixels, k)
	st.max = make([]Pixels, k)
	st.grow = make([]float32, k)
	st.shrink = make([]float32, k)
	st.target = make([]Pixels, k)
	st.hypCross = make([]Pixels, k)
	st.crossTarget = make([]Pixels, k)
	st.crossMin = make([]Pixels, k)
	st.crossMax = make([]Pixels, k)
	st.mainPos = make([]Pixels, k)
	st.crossPos = make([]Pixels, k)
	st.needReMeasure = make([]bool, k)

	childAvail := Available{
		W:        innerW,
		H:        innerH,
		BoundedW: boundedW,
		BoundedH: boundedH,
	}
	childAvailMain := Available{
		W:        innerW,
		H:        innerH,
		BoundedW: childBoundedMain,
		BoundedH: boundedH,
	}
	if isRow {
		childAvailMain = Available{
			W:        innerW,
			H:        innerH,
			BoundedW: boundedW,
			BoundedH: childBoundedMain,
		}
	}

	// Step 1: per-child flex base size, automatic minimum, hypothetical size.
	for i, child := range n.Children {
		styleMain := child.Style.resolveMain(isRow)
		styleCross := child.Style.resolveCross(isRow)

		minMain := childAutoMin(child, isRow, childAvail, childBoundedMain)
		if child.Style.resolveMinMain(isRow).IsPx() {
			minMain = child.Style.resolveMinMain(isRow).Val
		}
		maxMain := infPixels
		if child.Style.resolveMaxMain(isRow).IsPx() {
			maxMain = child.Style.resolveMaxMain(isRow).Val
		}
		minCross := childAutoMin(child, !isRow, childAvail, boundedCross)
		if child.Style.resolveMinCross(isRow).IsPx() {
			minCross = child.Style.resolveMinCross(isRow).Val
		}
		maxCross := infPixels
		if child.Style.resolveMaxCross(isRow).IsPx() {
			maxCross = child.Style.resolveMaxCross(isRow).Val
		}

		var base Pixels
		switch styleMain.Kind {
		case LengthPx:
			base = styleMain.Val
			st.grow[i] = child.Style.FlexGrow
			st.shrink[i] = child.Style.FlexShrink
		case LengthFlex:
			base = 0
			st.grow[i] = float32(styleMain.Val)
			st.shrink[i] = 1
		default: // LengthAuto
			base = measureNode(child, childAvailMain).axis(isRow)
			st.grow[i] = child.Style.FlexGrow
			st.shrink[i] = child.Style.FlexShrink
		}

		st.min[i], st.max[i] = minMain, maxMain
		st.crossMin[i], st.crossMax[i] = minCross, maxCross
		st.base[i] = base
		st.hyp[i] = clampLen(base, minMain, maxMain)

		// Cross hypothetical size, measured at the child's hypothetical main
		// size so the two axes agree.
		switch styleCross.Kind {
		case LengthPx:
			st.hypCross[i] = clampLen(styleCross.Val, minCross, maxCross)
		default:
			// Flex on the cross axis is treated as Auto so stretch can fill
			// the line.
			m := childAvail
			if isRow {
				m.H = st.hyp[i]
				m.BoundedH = true
			} else {
				m.W = st.hyp[i]
				m.BoundedW = true
			}
			if !boundedCross {
				if isRow {
					m.BoundedH = false
				} else {
					m.BoundedW = false
				}
			}
			st.hypCross[i] = clampLen(measureNode(child, m).axis(!isRow), minCross, maxCross)
		}
	}

	// Step 2: the container's own main size. Sizes are border-box (padding
	// included), matching CSS box-sizing: border-box, so the value handed to
	// resolveFlex is the padding-subtracted content box.
	var borderMain, contentMain Pixels
	mainDefinite := true
	selfMain := n.Style.resolveMain(isRow)
	switch {
	case selfMain.IsPx():
		borderMain = selfMain.Val
	case boundedMain:
		borderMain = availMain + n.Style.mainPadding(isRow)
	default:
		mainDefinite = false
		var sum Pixels
		for i := range n.Children {
			sum += maxPixels(st.base[i], st.min[i])
		}
		borderMain = sum + gapsTotal + n.Style.mainPadding(isRow)
	}
	borderMain = clampLen(borderMain, resolveBound(n.Style.resolveMinMain(isRow), 0), resolveBound(n.Style.resolveMaxMain(isRow), infPixels))
	contentMain = maxPixels(0, borderMain-n.Style.mainPadding(isRow))
	st.usedMain = borderMain

	// Step 3: resolve flexible lengths.
	resolveFlex(st, contentMain, gapsTotal, mainDefinite)

	// Step 4: line cross size and stretch. A definite cross size on the
	// container itself wins over the space the parent offers, otherwise a
	// flex column would stretch to fill its parent instead of its own width.
	var lineCross Pixels
	selfCross := n.Style.resolveCross(isRow)
	switch {
	case selfCross.IsPx():
		lineCross = maxPixels(0, selfCross.Val-n.Style.crossPadding(isRow))
	case boundedCross:
		lineCross = availCross
	default:
		for i := range n.Children {
			lineCross = maxPixels(lineCross, st.hypCross[i])
		}
	}
	lineCross = clampLen(lineCross, resolveBound(n.Style.resolveMinCross(isRow), 0), resolveBound(n.Style.resolveMaxCross(isRow), infPixels))

	for i, child := range n.Children {
		align := child.Style.AlignSelf
		if align == AlignSelfAuto {
			align = n.Style.AlignItems
		}
		if align == AlignStretch && child.Style.resolveCross(isRow).IsAuto() {
			ct := clampLen(lineCross, st.crossMin[i], st.crossMax[i])
			st.crossTarget[i] = ct
			st.needReMeasure[i] = ct != st.hypCross[i]
		} else {
			st.crossTarget[i] = st.hypCross[i]
			st.needReMeasure[i] = false
		}
	}

	// Step 5: main-axis placement.
	var used Pixels
	for i := range n.Children {
		used += st.target[i]
	}
	used += gapsTotal

	free := Pixels(0)
	if mainDefinite {
		free = contentMain - used
	}
	mode := n.Style.Justify
	if k <= 1 || free <= 0 {
		if mode == JustifySpaceBetween {
			mode = JustifyStart
		}
		if mode == JustifySpaceAround {
			mode = JustifyCenter
		}
	}
	first := firstOffset(mode, free, k)
	between := betweenOffset(mode, free, k)

	pos := n.Style.mainPaddingStart(isRow) + first
	for i := range n.Children {
		st.mainPos[i] = pos
		pos += st.target[i] + gap
		if i > 0 {
			pos += between
		}
	}

	// Step 6: cross-axis placement.
	for i, child := range n.Children {
		align := child.Style.AlignSelf
		if align == AlignSelfAuto {
			align = n.Style.AlignItems
		}
		cfree := lineCross - st.crossTarget[i]
		var off Pixels
		switch align {
		case AlignCenter:
			off = cfree / 2
		case AlignEnd:
			off = cfree
		default: // Start, Stretch
			off = 0
		}
		st.crossPos[i] = n.Style.crossPaddingStart(isRow) + off
	}

	// Step 7/8: container size and content size.
	var usedCross Pixels
	if boundedCross {
		usedCross = lineCross
	} else {
		usedCross = lineCross
	}
	usedCross += n.Style.crossPadding(isRow)
	st.usedCross = usedCross

	var size Size
	if isRow {
		size = Size{W: borderMain, H: usedCross}
	} else {
		size = Size{W: usedCross, H: borderMain}
	}

	// ContentSize tracks how far the content scrolls past the border box.
	var extentMain, extentCross Pixels
	extentMain = st.usedMain
	extentCross = st.usedCross
	for i, child := range n.Children {
		endMain := st.mainPos[i] + st.target[i]
		endCross := st.crossPos[i] + st.crossTarget[i]
		if child.Style.Overflow == OverflowVisible {
			if child.Rect.Size.W > 0 || child.Rect.Size.H > 0 || child.ContentSize.W > 0 {
				// Propagate the child's own scrollable overflow. The child has
				// not been arranged yet, so use the size it will get.
				cw, ch := st.target[i], st.crossTarget[i]
				if !isRow {
					cw, ch = ch, cw
				}
				endMain += maxPixels(0, child.ContentSize.W-cw)
				endCross += maxPixels(0, child.ContentSize.H-ch)
			}
		}
		extentMain = maxPixels(extentMain, endMain+n.Style.mainPaddingEnd(isRow))
		extentCross = maxPixels(extentCross, endCross+n.Style.crossPaddingEnd(isRow))
	}
	n.ContentSize = mapAxis(size, extentMain, extentCross, isRow)
	return size
}

func arrangeNode(n *LayoutNode) {
	if n == nil || !n.Style.Display {
		return
	}
	if len(n.Children) == 0 {
		return
	}
	isRow := n.state.isRow
	origin := n.Rect.Origin
	// mainPos/crossPos are already measured from the content-box edge (they
	// start at mainPaddingStart/crossPaddingStart), so padding is not added
	// here a second time.
	ox, oy := origin.X, origin.Y

	for i, child := range n.Children {
		var r Bounds
		if isRow {
			r = Bounds{
				Origin: Point{X: ox + n.state.mainPos[i], Y: oy + n.state.crossPos[i]},
				Size:   Size{W: n.state.target[i], H: n.state.crossTarget[i]},
			}
		} else {
			r = Bounds{
				Origin: Point{X: ox + n.state.crossPos[i], Y: oy + n.state.mainPos[i]},
				Size:   Size{W: n.state.crossTarget[i], H: n.state.target[i]},
			}
		}
		child.Rect = r

		// Re-measure the child against its final box so its own internal
		// state (and ContentSize) reflect the size it actually received.
		tight := Available{W: r.Size.W, H: r.Size.H, BoundedW: true, BoundedH: true}
		if child.Style.isScrollContainer() {
			if isRow {
				tight.BoundedW = false
			} else {
				tight.BoundedH = false
			}
		}
		measureNode(child, tight)
		arrangeNode(child)
	}
}

func firstOffset(mode Justify, free Pixels, k int) Pixels {
	switch mode {
	case JustifyEnd:
		return free
	case JustifyCenter:
		return free / 2
	case JustifySpaceAround:
		if free >= 0 {
			return (free / Pixels(k)) / 2
		}
		return free / 2
	default:
		return 0
	}
}

func betweenOffset(mode Justify, free Pixels, k int) Pixels {
	if k < 2 {
		return 0
	}
	f := maxPixels(0, free)
	switch mode {
	case JustifySpaceBetween:
		return f / Pixels(k-1)
	case JustifySpaceAround:
		return f / Pixels(k)
	default:
		return 0
	}
}

// childAutoMin computes the CSS automatic minimum size (min-width/min-height:
// auto) for a child on the given axis: 0 for scroll containers, otherwise the
// child's min-content size in that axis, capped by its definite preferred size.
func childAutoMin(child *LayoutNode, isRow bool, parentAvail Available, parentBounded bool) Pixels {
	if child.Style.isScrollContainer() {
		return 0
	}
	var m Available = parentAvail
	if isRow {
		m.BoundedW = false
	} else {
		m.BoundedH = false
	}
	_ = parentBounded
	v := measureNode(child, m).axis(isRow)
	pref := child.Style.resolveMain(isRow)
	if pref.IsPx() {
		v = minPixels(v, pref.Val)
	}
	return v
}

// resolveFlex implements CSS flexbox §9.7.
func resolveFlex(st *nodeState, M Pixels, gapsTotal Pixels, mainDefinite bool) {
	k := len(st.base)
	if k == 0 {
		return
	}
	if !mainDefinite {
		for i := range st.base {
			st.target[i] = st.hyp[i]
		}
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
			if frozen[i] {
				free -= st.hyp[i]
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

func pickAxis(w Pixels, bw bool, h Pixels, bh bool, isRow bool) (Pixels, bool) {
	if isRow {
		return w, bw
	}
	return h, bh
}

func (s Size) axis(isRow bool) Pixels {
	if isRow {
		return s.W
	}
	return s.H
}

func mapAxis(size Size, main, cross Pixels, isRow bool) Size {
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
