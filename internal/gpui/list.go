package gpui

import (
	"math"
	"slices"
	"sort"
)

const (
	listDefaultEstimate = 24
	listDefaultOverdraw = 100
	// listMaxPasses bounds how often a frame re-resolves its offset after
	// measuring newly shown items.
	listMaxPasses = 6
)

// ListState is the retained state of one virtualized List (GPUI's
// ListState): the measured height of each item at the list's content width,
// the scroll anchor and the item keys. The view owns it across frames; one
// state backs one list.
type ListState struct {
	heights   []Pixels
	measured  []bool
	keys      []uint64
	keyed     []bool
	nMeasured int
	width     Pixels
	hasWidth  bool
	// median is the median measured height, recomputed (into scratch) only
	// when a height changed since.
	median      Pixels
	medianDirty bool
	// sinceMedian counts height changes since the median was last computed.
	sinceMedian int
	scratch     []Pixels
	// topsBuf is reused by every frame's item-top computation.
	topsBuf []Pixels

	// anchorIx is the item at the top of the viewport after the last frame
	// and anchorOff the offset's distance below that item's top.
	anchorIx  int
	anchorOff Pixels
	lastOff   Pixels
	hasLast   bool

	content          Pixels
	visStart, visEnd int
}

// NewListState returns an empty state: nothing measured, scrolled to the top.
func NewListState() *ListState { return &ListState{} }

// Invalidate forgets item ix's measured height; the item is measured again
// the next time it is built and counts as the estimate until then.
func (s *ListState) Invalidate(ix int) {
	if ix >= 0 && ix < len(s.measured) {
		s.unset(ix)
	}
}

// InvalidateAll forgets every measured height (after a font or theme change,
// say). The scroll anchor is kept.
func (s *ListState) InvalidateAll() {
	for i := range s.measured {
		s.measured[i] = false
		s.heights[i] = 0
	}
	s.nMeasured, s.medianDirty = 0, true
}

// MeasuredHeight returns item ix's cached border-box height; ok is false
// while the item is unmeasured at the current width.
func (s *ListState) MeasuredHeight(ix int) (Pixels, bool) {
	if ix < 0 || ix >= len(s.measured) || !s.measured[ix] {
		return 0, false
	}
	return s.heights[ix], true
}

// ContentHeight returns the list's content height as of the last frame:
// items, gaps and padding, unmeasured items counted at the estimate.
func (s *ListState) ContentHeight() Pixels { return s.content }

// VisibleRange returns the half-open range of items built in the last frame
// (the viewport plus the overdraw margin).
func (s *ListState) VisibleRange() (start, end int) { return s.visStart, s.visEnd }

func (s *ListState) unset(ix int) {
	if s.measured[ix] {
		s.nMeasured--
		s.medianDirty = true
		s.sinceMedian++
	}
	s.measured[ix], s.heights[ix] = false, 0
}

// set records item ix's height and reports whether it changed.
func (s *ListState) set(ix int, h Pixels) bool {
	if s.measured[ix] && s.heights[ix] == h {
		return false
	}
	s.unset(ix)
	s.measured[ix], s.heights[ix] = true, h
	s.nMeasured++
	s.medianDirty = true
	s.sinceMedian++
	return true
}

// sync fits the cache to count items at content width w, dropping the
// heights a width change or a changed key invalidates.
func (s *ListState) sync(count int, w Pixels, key func(int) uint64) {
	if n := len(s.heights); count < n {
		for i := count; i < n; i++ {
			s.unset(i)
		}
		s.heights, s.measured, s.keys, s.keyed = s.heights[:count], s.measured[:count], s.keys[:count], s.keyed[:count]
	} else if count > n {
		grow := count - n
		s.heights = append(s.heights, make([]Pixels, grow)...)
		s.measured = append(s.measured, make([]bool, grow)...)
		s.keys = append(s.keys, make([]uint64, grow)...)
		s.keyed = append(s.keyed, make([]bool, grow)...)
	}
	if !s.hasWidth || absPixels(s.width-w) > 0.01 {
		s.InvalidateAll()
		s.width, s.hasWidth = w, true
	}
	if key == nil {
		return
	}
	for i := 0; i < count; i++ {
		k := key(i)
		if s.keyed[i] && s.keys[i] != k {
			s.unset(i)
		}
		s.keys[i], s.keyed[i] = k, true
	}
}

// listMedianSmall is the measured-item count up to which the median is
// recomputed after every change; beyond it, after every listMedianEvery
// changes, so an item that changes every frame (a streaming message) does
// not re-sort every height each frame. The median of many items barely moves.
const (
	listMedianSmall = 32
	listMedianEvery = 16
)

// estimate is the height an unmeasured item counts as: the (lower) median
// of the measured heights, which one very tall or very short item does not
// skew, or fallback while none is measured.
func (s *ListState) estimate(fallback Pixels) Pixels {
	if s.nMeasured == 0 {
		return fallback
	}
	if s.medianDirty && (s.nMeasured <= listMedianSmall || s.sinceMedian >= listMedianEvery || s.median == 0) {
		s.sinceMedian = 0
		s.scratch = s.scratch[:0]
		for i, ok := range s.measured {
			if ok {
				s.scratch = append(s.scratch, s.heights[i])
			}
		}
		slices.Sort(s.scratch)
		s.median, s.medianDirty = s.scratch[(len(s.scratch)-1)/2], false
	}
	return s.median
}

// height returns item ix's cached height or est.
func (s *ListState) height(ix int, est Pixels) Pixels {
	if s.measured[ix] {
		return s.heights[ix]
	}
	return est
}

// tops fills the reused buffer with each item's top (content coordinates
// from the border box, so item 0 starts at padTop) and returns it with the
// content height (padding included).
func (s *ListState) tops(gap, padTop, padBottom, fallback Pixels) ([]Pixels, Pixels) {
	n := len(s.heights)
	dst := s.topsBuf[:0]
	defer func() { s.topsBuf = dst }()
	est := s.estimate(fallback)
	y := padTop
	for i := 0; i < n; i++ {
		if i > 0 {
			y += gap
		}
		dst = append(dst, y)
		y += s.height(i, est)
	}
	return dst, y + padBottom
}

// List is a virtualized, vertically scrolling list of items of varying
// height (GPUI's list). Each frame only the items intersecting the viewport
// (plus the overdraw margin) are rendered, laid out, prepainted and painted;
// every other item is represented by its cached height in the ListState, or
// by the estimate while it was never measured.
type List struct {
	StyledMixin
	id            string
	state         *ListState
	count         int
	render        func(ix int, w *Window, cx *App) IntoElement
	key           func(ix int) uint64
	estimate      Pixels
	overdraw      Pixels
	scrollHandle  *ScrollHandle
	showScrollbar bool
}

// NewList returns a list of count items keyed by id whose heights live in
// state; render returns the element of item ix and is called only for the
// items being built this frame. The list scrolls vertically.
func NewList(id string, state *ListState, count int, render func(ix int, w *Window, cx *App) IntoElement) *List {
	if state == nil {
		state = NewListState()
	}
	l := &List{StyledMixin: NewStyledMixin(), id: id, state: state, count: max(0, count), render: render,
		estimate: listDefaultEstimate, overdraw: listDefaultOverdraw}
	l.OverflowYScroll()
	return l
}

// ItemKey sets a per-item key: when item ix's key differs from the key its
// height was measured with, the cached height is dropped. Use a revision
// that changes with the item's content.
func (l *List) ItemKey(key func(ix int) uint64) *List { l.key = key; return l }

// EstimatedItemHeight sets the height unmeasured items count as while no
// item is measured (default 24); afterwards the median measured height is used.
func (l *List) EstimatedItemHeight(px float32) *List { l.estimate = Pixels(max(0, px)); return l }

// Overdraw sets the margin above and below the viewport whose items are
// built too (default 100).
func (l *List) Overdraw(px float32) *List { l.overdraw = Pixels(max(0, px)); return l }

// TrackScroll binds h to the list: h reports and controls the scroll offset,
// and ScrollToItem(ix) reveals item ix.
func (l *List) TrackScroll(h *ScrollHandle) *List { l.scrollHandle = h; return l }

// ShowScrollbar opts the list into a thin overlay scrollbar with primary
// thumb dragging and viewport paging on track presses.
func (l *List) ShowScrollbar() *List { l.showScrollbar = true; return l }

// IntoElement satisfies IntoElement via the listElement adapter.
func (l *List) IntoElement() Element { return listElement{l: l} }

// listElement adapts *List to Element.
type listElement struct{ l *List }

// listLayout is the RequestLayout state: the leaf node and the text style
// items inherit.
type listLayout struct {
	node      *LayoutNode
	textStyle TextStyle
}

// ID returns the list's id.
func (e listElement) ID() (ElementID, bool) {
	if e.l.id == "" {
		return ElementID{}, false
	}
	return ElementID{Path: []ElementPathKey{{Key: e.l.id}}}, true
}

// RequestLayout registers a leaf whose content height is the estimated
// content height at the offered width; the list has no intrinsic width.
func (e listElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	l := e.l
	style := l.ResolvedStyle(DefaultStyle())
	style.TextStyle = l.style.RefineText(w.currentTextStyle())
	node := &LayoutNode{Style: style, Text: "list"}
	gap := style.axisGap(false)
	node.Measure = func(_ string, _ TextStyle, maxWidth float32) (float32, float32) {
		s := l.state
		n := min(l.count, len(s.heights))
		est := s.estimate(l.estimate)
		cached := s.hasWidth && maxWidth > 0 && maxWidth < math.MaxFloat32/2 && absPixels(s.width-Pixels(maxWidth)) <= 0.01
		var h Pixels
		for i := 0; i < l.count; i++ {
			if i > 0 {
				h += gap
			}
			if cached && i < n && s.measured[i] {
				h += s.heights[i]
			} else {
				h += est
			}
		}
		return 0, float32(h)
	}
	return w.registerLayoutNode(node), &listLayout{node: node, textStyle: style.TextStyle}
}

// listPrepaint is the per-frame prepaint state.
type listPrepaint struct {
	style     Style
	textStyle TextStyle
	textSet   TextFields
	children  []*renderNode
	scroll    *scrollbarInfo
}

// paintTextOverride re-asserts the list's own text paint fields, so an
// ancestor's hover/focus text style does not recolour items the list styles.
func (st *listPrepaint) paintTextOverride() (textOverride, bool) {
	return textOverride{set: st.textSet & paintTextFields, ts: st.textStyle}, false
}

func (st *listPrepaint) clipsChildren() bool            { return true }
func (st *listPrepaint) paintOpacity(w *Window) float32 { return st.style.Opacity }
func (st *listPrepaint) lateChildren() []*renderNode    { return st.children }
func (st *listPrepaint) registerScrollbar(w *Window) {
	if st.scroll != nil {
		st.scroll.registerInput(w)
	}
}

func (st *listPrepaint) paintScrollbar(w *Window) {
	if st.scroll != nil {
		st.scroll.paint(w)
	}
}

// listItem is an item built (rendered and laid out) this frame.
type listItem struct {
	el    Element
	node  *LayoutNode
	state any
}

// rawScroll returns the container's stored offset: the handle's when one is
// bound, else the retained one.
func (w *Window) rawScroll(gid GlobalElementID, h *ScrollHandle) Point {
	if h != nil {
		return h.offset
	}
	st, _ := w.elementStates[globalIDKey(gid)].(scrollState)
	return st.offset
}

// storeScroll writes off as the container's offset before syncScroll
// records it, with any handle request consumed.
func (w *Window) storeScroll(gid GlobalElementID, h *ScrollHandle, off Point) {
	if h != nil {
		h.offset, h.request = off, scrollNone
		return
	}
	key := globalIDKey(gid)
	st, _ := w.elementStates[key].(scrollState)
	st.offset = off
	w.elementStates[key] = st
}

// Prepaint resolves the offset against the cached heights, builds and
// measures the items the viewport shows (re-resolving until their heights
// settle), registers the scrollable hitbox and returns the built items as
// render nodes, which the window then prepaints and paints as children.
func (e listElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	l := e.l
	s := l.state
	gid := w.currentElementID
	ls, _ := layoutState.(*listLayout)
	if ls == nil {
		ls = &listLayout{textStyle: l.style.RefineText(DefaultTextStyle())}
	}
	style := l.ResolvedStyle(DefaultStyle())
	st := &listPrepaint{style: style, textStyle: ls.textStyle, textSet: l.style.textSet()}
	pad := style.Padding
	gap := style.axisGap(false)
	content := bounds.Inset(pad)
	cw := maxPixels(0, content.Size.W)
	viewH := bounds.Size.H
	n := l.count
	if l.render == nil {
		n = 0
	}
	s.sync(n, cw, l.key)

	raw := w.rawScroll(gid, l.scrollHandle).Y
	req, reqItem := scrollNone, 0
	if h := l.scrollHandle; h != nil {
		req, reqItem = h.request, h.item
	}
	userDelta := raw - s.lastOff

	built := map[int]*listItem{}
	build := func(ix int) *listItem {
		if it, ok := built[ix]; ok {
			return it
		}
		it := &listItem{}
		if el := IntoElementOf(l.render(ix, w, cx)); el != nil {
			lid, cstate := el.RequestLayout(w, cx)
			it.el, it.state, it.node = el, cstate, w.layoutNodes[lid]
			if it.node != nil {
				Layout(it.node, UnboundedHeight(float32(cw)))
			}
		}
		built[ix] = it
		return it
	}

	w.pushTextStyle(ls.textStyle)
	var tops []Pixels
	var total, limit, off Pixels
	var start, end int
	for pass := 0; ; pass++ {
		tops, total = s.tops(gap, pad.Top, pad.Bottom, l.estimate)
		limit = maxPixels(0, total-viewH)
		est := s.estimate(l.estimate)
		anchored := raw
		if s.hasLast && s.anchorIx < n {
			anchored = tops[s.anchorIx] + s.anchorOff + userDelta
		}
		switch req {
		case scrollTop:
			off = 0
		case scrollBottom:
			off = limit
		case scrollItem:
			off = anchored
			if reqItem >= 0 && reqItem < n {
				off = revealSpan(anchored, tops[reqItem], tops[reqItem]+s.height(reqItem, est), viewH, pad.Top, pad.Bottom)
			}
		default:
			off = anchored
		}
		off = clampf(off, 0, limit)
		lo, hi := off-l.overdraw, off+viewH+l.overdraw
		start = sort.Search(n, func(i int) bool { return tops[i]+s.height(i, est) > lo })
		end = sort.Search(n, func(i int) bool { return tops[i] >= hi })
		end = max(start, end)
		changed := false
		for ix := start; ix < end; ix++ {
			it := build(ix)
			var h Pixels
			if it.node != nil {
				h = it.node.Rect.Size.H
			}
			if s.set(ix, h) {
				changed = true
			}
		}
		if !changed {
			break
		}
		if pass >= listMaxPasses {
			// Heights still moving: place the built items by their
			// measured heights at the offset this pass chose.
			tops, total = s.tops(gap, pad.Top, pad.Bottom, l.estimate)
			limit = maxPixels(0, total-viewH)
			off = clampf(off, 0, limit)
			break
		}
	}
	w.popTextStyle()

	w.storeScroll(gid, l.scrollHandle, Point{Y: off})
	lim := Size{H: limit}
	w.syncScroll(gid, l.scrollHandle, bounds, pad, lim, AxisY, nil)

	// With no items there is nothing to anchor to: the next frame takes the
	// stored offset as it is.
	s.anchorIx = max(0, sort.Search(n, func(i int) bool { return tops[i] > off })-1)
	s.anchorOff = 0
	if n > 0 {
		s.anchorOff = off - tops[s.anchorIx]
	}
	s.lastOff, s.hasLast = off, n > 0
	s.content = total
	s.visStart, s.visEnd = start, end

	hb := w.InsertHitbox(bounds)
	ml := &w.nextFrame.mouseListeners[hb]
	ml.elementID = gid
	ml.scrollable = true
	ml.scrollAxes = AxisY
	ml.scrollMax = lim
	if l.showScrollbar {
		st.scroll = &scrollbarInfo{bounds: bounds, offset: Point{Y: off}, max: lim, axes: AxisY, owner: gid, elastic: true}
	}

	for ix := start; ix < end; ix++ {
		it := built[ix]
		if it == nil || it.el == nil {
			continue
		}
		key := ElementPathKey{Index: ix}
		if local, ok := it.el.ID(); ok && len(local.Path) > 0 {
			key.Key = local.Path[len(local.Path)-1].Key
		}
		cpath := make([]ElementPathKey, 0, len(gid.Path)+1)
		cpath = append(cpath, gid.Path...)
		cpath = append(cpath, key)
		origin := Point{X: content.Origin.X, Y: bounds.Origin.Y + tops[ix] - off}
		st.children = append(st.children, w.buildRenderNode(it.el, GlobalElementID{Path: cpath}, it.node, it.state, origin, cx))
	}
	return st
}

// Paint paints the list's own box (shadows, background, border); items are
// painted by the window's walk.
func (e listElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	if st, ok := prepaintState.(*listPrepaint); ok {
		paintBox(w, bounds, st.style)
		return
	}
	paintBox(w, bounds, e.l.ResolvedStyle(DefaultStyle()))
}

// Gap is StyledMixin.Gap returning *List: the space between items.
func (l *List) Gap(v float32) *List { l.StyledMixin.Gap(v); return l }

// SizeFull is StyledMixin.SizeFull returning *List.
func (l *List) SizeFull() *List { l.StyledMixin.SizeFull(); return l }

// SizeWH is StyledMixin.SizeWH returning *List.
func (l *List) SizeWH(w, h float32) *List { l.StyledMixin.SizeWH(w, h); return l }

// SizeW is StyledMixin.SizeW returning *List.
func (l *List) SizeW(w float32) *List { l.StyledMixin.SizeW(w); return l }

// SizeH is StyledMixin.SizeH returning *List.
func (l *List) SizeH(h float32) *List { l.StyledMixin.SizeH(h); return l }

// Flex1 is StyledMixin.Flex1 returning *List.
func (l *List) Flex1() *List { l.StyledMixin.Flex1(); return l }

// MinH is StyledMixin.MinH returning *List.
func (l *List) MinH(v float32) *List { l.StyledMixin.MinH(v); return l }

// MaxH is StyledMixin.MaxH returning *List.
func (l *List) MaxH(v float32) *List { l.StyledMixin.MaxH(v); return l }

// P is StyledMixin.P returning *List.
func (l *List) P(v float32) *List { l.StyledMixin.P(v); return l }

// Px_ is StyledMixin.Px_ returning *List.
func (l *List) Px_(v float32) *List { l.StyledMixin.Px_(v); return l }

// Py_ is StyledMixin.Py_ returning *List.
func (l *List) Py_(v float32) *List { l.StyledMixin.Py_(v); return l }

// Bg is StyledMixin.Bg returning *List.
func (l *List) Bg(f Fill) *List { l.StyledMixin.Bg(f); return l }

// Rounded is StyledMixin.Rounded returning *List.
func (l *List) Rounded(px float32) *List { l.StyledMixin.Rounded(px); return l }

// Border is StyledMixin.Border returning *List.
func (l *List) Border(w float32, c Rgba) *List { l.StyledMixin.Border(w, c); return l }

// Opacity is StyledMixin.Opacity returning *List.
func (l *List) Opacity(f float32) *List { l.StyledMixin.Opacity(f); return l }

// TextColor is StyledMixin.TextColor returning *List.
func (l *List) TextColor(c Rgba) *List { l.StyledMixin.TextColor(c); return l }
