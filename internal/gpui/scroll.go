package gpui

// scrollRequest is a pending programmatic scroll applied at the next frame.
type scrollRequest uint8

const (
	scrollNone scrollRequest = iota
	scrollItem
	scrollTop
	scrollBottom
)

// ScrollHandle exposes and controls one scroll container's offset (GPUI's
// ScrollHandle and UniformListScrollHandle). Bind it with Div.TrackScroll or
// UniformList.TrackScroll. Offsets are positive distances scrolled, unlike
// GPUI's negative offsets. A handle should track one container at a time.
type ScrollHandle struct {
	offset  Point
	max     Size
	bounds  Bounds
	request scrollRequest
	item    int
}

// NewScrollHandle returns a handle at offset zero.
func NewScrollHandle() *ScrollHandle { return &ScrollHandle{} }

// Offset returns the current scroll offset.
func (h *ScrollHandle) Offset() Point { return h.offset }

// SetOffset sets the scroll offset. It is clamped to the scrollable extent
// when the next frame is drawn; the caller refreshes the window (or
// notifies the view) to see it.
func (h *ScrollHandle) SetOffset(p Point) {
	h.offset = p
	h.request = scrollNone
}

// MaxOffset returns the scrollable extent as of the last frame.
func (h *ScrollHandle) MaxOffset() Size { return h.max }

// Bounds returns the container's border box as of the last frame.
func (h *ScrollHandle) Bounds() Bounds { return h.bounds }

// ScrollToItem scrolls the minimum distance that makes child ix of a Div
// (or row ix of a UniformList) fully visible, at the next frame.
func (h *ScrollHandle) ScrollToItem(ix int) { h.request, h.item = scrollItem, ix }

// ScrollToTop scrolls to the start at the next frame.
func (h *ScrollHandle) ScrollToTop() { h.request = scrollTop }

// ScrollToBottom scrolls to the end at the next frame.
func (h *ScrollHandle) ScrollToBottom() { h.request = scrollBottom }

// revealSpan returns the offset along one axis that brings [start, end)
// (content coordinates from the border-box origin) into the viewport of
// length view, padded by padStart/padEnd, moving as little as possible.
func revealSpan(off, start, end, view, padStart, padEnd Pixels) Pixels {
	if end-off > view-padEnd {
		off = end - (view - padEnd)
	}
	if start-off < padStart {
		off = start - padStart
	}
	return off
}

// scrollMaxOn zeroes the components of max on axes that do not scroll, so
// neither input nor programmatic scrolling can move them.
func scrollMaxOn(max Size, axes Axes) Size {
	if !axes.Has(AxisX) {
		max.W = 0
	}
	if !axes.Has(AxisY) {
		max.H = 0
	}
	return max
}

// syncScroll resolves a scroll container's offset for this frame: it
// applies a pending handle request, clamps to max (zero on axes the
// container does not scroll) and records the result in both the retained
// element state and the handle. itemSpan returns the content-space rect of
// item ix for ScrollToItem (ok false if none).
func (w *Window) syncScroll(gid GlobalElementID, h *ScrollHandle, bounds Bounds, pad Edges, max Size, axes Axes, itemSpan func(ix int) (Bounds, bool)) Point {
	max = scrollMaxOn(max, axes)
	key := globalIDKey(gid)
	w.touchState(key)
	st, _ := w.elementStates[key].(scrollState)
	off := st.offset
	if h != nil {
		off = h.offset
		switch h.request {
		case scrollTop:
			off = Point{}
		case scrollBottom:
			off.Y = max.H
		case scrollItem:
			if itemSpan != nil {
				if r, ok := itemSpan(h.item); ok {
					off.X = revealSpan(off.X, r.Origin.X, r.Max().X, bounds.Size.W, pad.Left, pad.Right)
					off.Y = revealSpan(off.Y, r.Origin.Y, r.Max().Y, bounds.Size.H, pad.Top, pad.Bottom)
				}
			}
		}
		h.request = scrollNone
	}
	off = Point{X: clampf(off.X, 0, max.W), Y: clampf(off.Y, 0, max.H)}
	st.offset, st.maxW, st.maxH, st.handle = off, max.W, max.H, h
	w.elementStates[key] = st
	if h != nil {
		h.offset, h.max, h.bounds = off, max, bounds
	}
	return off
}

// applyWheel scrolls the container l by the wheel delta on its axes and
// reports whether its offset changed. A container scrolling X only takes
// DeltaY as horizontal motion when there is no DeltaX, as GPUI does.
func (w *Window) applyWheel(l *MouseListener, e ScrollWheelEvent) bool {
	dx, dy := e.DeltaX, e.DeltaY
	axes := l.scrollAxes
	if !axes.Has(AxisX) {
		dx = 0
	} else if !axes.Has(AxisY) && dx == 0 {
		dx = dy
	}
	if !axes.Has(AxisY) {
		dy = 0
	}
	key := globalIDKey(l.elementID)
	w.touchState(key)
	st, _ := w.elementStates[key].(scrollState)
	st.maxW, st.maxH = l.scrollMax.W, l.scrollMax.H
	old := st.offset
	if st.handle != nil {
		old = st.handle.offset
	}
	nw := Point{X: clampf(old.X+dx, 0, st.maxW), Y: clampf(old.Y+dy, 0, st.maxH)}
	if nw == old {
		return false
	}
	st.offset = nw
	if st.handle != nil {
		st.handle.offset = nw
	}
	w.elementStates[key] = st
	return true
}

// scrollbarInfo is what a scroll container needs to paint its overlay
// scrollbar after its children.
type scrollbarInfo struct {
	bounds Bounds
	offset Point
	max    Size
	axes   Axes
	owner  GlobalElementID
	// elastic marks a List, whose extent moves while items are measured: a
	// thumb drag re-bases instead of cancelling when it does.
	elastic bool
}

const (
	scrollbarThickness = 6
	scrollbarInset     = 2
	scrollbarMinThumb  = 20
)

// scrollbarColor is the overlay thumb color: mid gray, visible on light and
// dark backgrounds.
var scrollbarColor = Rgba{R: 128, G: 128, B: 128, A: 140}

type scrollbarGeometry struct {
	track, thumb      Bounds
	maximum, viewport Pixels
}

func (g scrollbarGeometry) travel(axis Axes) Pixels {
	if axis == AxisY {
		return g.track.Size.H - g.thumb.Size.H
	}
	return g.track.Size.W - g.thumb.Size.W
}

func (s *scrollbarInfo) geometry(axis Axes) (scrollbarGeometry, bool) {
	b := s.bounds
	showY := s.axes.Has(AxisY) && s.max.H > 0.5
	showX := s.axes.Has(AxisX) && s.max.W > 0.5
	if (axis == AxisY && !showY) || (axis == AxisX && !showX) {
		return scrollbarGeometry{}, false
	}
	var track, viewport, maximum, offset Pixels
	var bounds Bounds
	if axis == AxisY {
		track, viewport, maximum, offset = b.Size.H-2*scrollbarInset, b.Size.H, s.max.H, s.offset.Y
		if showX {
			track -= scrollbarThickness + scrollbarInset
		}
		bounds = Bounds{Origin: Point{X: b.Max().X - scrollbarInset - scrollbarThickness, Y: b.Origin.Y + scrollbarInset}, Size: Size{W: scrollbarThickness, H: track}}
	} else {
		track, viewport, maximum, offset = b.Size.W-2*scrollbarInset, b.Size.W, s.max.W, s.offset.X
		if showY {
			track -= scrollbarThickness + scrollbarInset
		}
		bounds = Bounds{Origin: Point{X: b.Origin.X + scrollbarInset, Y: b.Max().Y - scrollbarInset - scrollbarThickness}, Size: Size{W: track, H: scrollbarThickness}}
	}
	if track <= 0 {
		return scrollbarGeometry{}, false
	}
	thumb := minPixels(track, maxPixels(scrollbarMinThumb, track*viewport/(viewport+maximum)))
	pos := (track - thumb) * offset / maximum
	thumbBounds := bounds
	if axis == AxisY {
		thumbBounds.Origin.Y += pos
		thumbBounds.Size.H = thumb
	} else {
		thumbBounds.Origin.X += pos
		thumbBounds.Size.W = thumb
	}
	return scrollbarGeometry{track: bounds, thumb: thumbBounds, maximum: maximum, viewport: viewport}, true
}

// paint uses the same geometry as input, preserving the existing overlay appearance.
func (s *scrollbarInfo) paint(w *Window) {
	for _, axis := range []Axes{AxisY, AxisX} {
		g, ok := s.geometry(axis)
		if !ok {
			continue
		}
		w.PaintQuad(Quad{Bounds: g.thumb, Background: BackgroundSolid, Fill: scrollbarColor, CornerRadii: Corners{}.Uniform(scrollbarThickness / 2)})
	}
}

type scrollbarInput struct {
	owner    GlobalElementID
	axis     Axes
	geometry scrollbarGeometry
	elastic  bool
}

type scrollbarDrag struct {
	ownerKey                              string
	axis                                  Axes
	track                                 Bounds
	maximum, travel                       Pixels
	startPointer, startOffset, lastOffset Pixels
	// lastPointer is the pointer coordinate of the last move, where an
	// elastic drag re-bases.
	lastPointer Pixels
}

func (s *scrollbarInfo) registerInput(w *Window) {
	for _, axis := range []Axes{AxisY, AxisX} {
		g, ok := s.geometry(axis)
		if !ok {
			continue
		}
		key := "#scrollbar-y"
		if axis == AxisX {
			key = "#scrollbar-x"
		}
		path := append(append([]ElementPathKey(nil), s.owner.Path...), ElementPathKey{Key: key})
		w.currentElementID = GlobalElementID{Path: path}
		hb := w.InsertHitbox(g.track)
		w.nextFrame.mouseListeners[hb].scrollbar = &scrollbarInput{owner: s.owner, axis: axis, geometry: g, elastic: s.elastic}
	}
}

func scrollbarCoordinate(p Point, axis Axes) Pixels {
	if axis == AxisY {
		return p.Y
	}
	return p.X
}

func scrollbarOffset(st scrollState, axis Axes) Pixels {
	p := st.offset
	if st.handle != nil {
		p = st.handle.offset
	}
	return scrollbarCoordinate(p, axis)
}

func (w *Window) setScrollbarOffset(input *scrollbarInput, value Pixels) {
	key := globalIDKey(input.owner)
	w.touchState(key)
	st, ok := w.elementStates[key].(scrollState)
	if !ok {
		return
	}
	p := st.offset
	if st.handle != nil {
		p = st.handle.offset
	}
	if input.axis == AxisY {
		p.Y = value
	} else {
		p.X = value
	}
	w.SetScrollOffset(input.owner, p)
	w.Refresh()
}

func (w *Window) beginScrollbar(pos Point) bool {
	i := w.renderedFrame.hitboxes.HitTest(pos)
	if i < 0 {
		return false
	}
	input := w.renderedFrame.mouseListeners[i].scrollbar
	if input == nil {
		return false
	}
	g := input.geometry
	st, ok := w.elementStates[globalIDKey(input.owner)].(scrollState)
	if !ok {
		return false
	}
	w.scrollbarPress = true
	if len(w.active) > 0 {
		clear(w.active)
		w.Refresh()
	}
	offset := scrollbarOffset(st, input.axis)
	if g.thumb.Contains(pos) {
		if travel := g.travel(input.axis); travel > 0 {
			w.scrollbarDrag = &scrollbarDrag{ownerKey: globalIDKey(input.owner), axis: input.axis, track: g.track, maximum: g.maximum, travel: travel, startPointer: scrollbarCoordinate(pos, input.axis), startOffset: offset, lastOffset: offset, lastPointer: scrollbarCoordinate(pos, input.axis)}
		}
	} else {
		delta := g.viewport
		if scrollbarCoordinate(pos, input.axis) < scrollbarCoordinate(g.thumb.Origin, input.axis) {
			delta = -delta
		}
		w.setScrollbarOffset(input, clampf(offset+delta, 0, g.maximum))
	}
	return true
}

func (w *Window) scrollbarFor(frame *Frame, drag *scrollbarDrag) *scrollbarInput {
	for i := range frame.mouseListeners {
		input := frame.mouseListeners[i].scrollbar
		if input != nil && input.axis == drag.axis && globalIDKey(input.owner) == drag.ownerKey && !frame.hitboxes.boxes[i].Bounds.IsEmpty() {
			return input
		}
	}
	return nil
}

func (w *Window) reconcileScrollbarDrag(frame *Frame) {
	drag := w.scrollbarDrag
	if drag == nil {
		return
	}
	input := w.scrollbarFor(frame, drag)
	if input == nil || input.geometry.track != drag.track {
		w.scrollbarDrag = nil
		return
	}
	st, ok := w.elementStates[drag.ownerKey].(scrollState)
	if !ok || (st.handle != nil && st.handle.request != scrollNone) {
		w.scrollbarDrag = nil
		return
	}
	travel, offset := input.geometry.travel(drag.axis), scrollbarOffset(st, drag.axis)
	if input.geometry.maximum == drag.maximum && travel == drag.travel && offset == drag.lastOffset {
		return
	}
	if !input.elastic || travel <= 0 {
		w.scrollbarDrag = nil
		return
	}
	drag.maximum, drag.travel = input.geometry.maximum, travel
	drag.startPointer, drag.startOffset, drag.lastOffset = drag.lastPointer, offset, offset
}

func (w *Window) moveScrollbar(pos Point) {
	w.reconcileScrollbarDrag(&w.renderedFrame)
	drag := w.scrollbarDrag
	if drag == nil {
		return
	}
	input := w.scrollbarFor(&w.renderedFrame, drag)
	drag.lastPointer = scrollbarCoordinate(pos, drag.axis)
	value := clampf(drag.startOffset+(drag.lastPointer-drag.startPointer)*drag.maximum/drag.travel, 0, drag.maximum)
	if value != drag.lastOffset {
		drag.lastOffset = value
		w.setScrollbarOffset(input, value)
	}
}
