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
}

const (
	scrollbarThickness = 6
	scrollbarInset     = 2
	scrollbarMinThumb  = 20
)

// scrollbarColor is the overlay thumb color: mid gray, visible on light and
// dark backgrounds.
var scrollbarColor = Rgba{R: 128, G: 128, B: 128, A: 140}

// paint draws one thumb per scrolling axis that has overflow.
func (s *scrollbarInfo) paint(w *Window) {
	b := s.bounds
	showY := s.axes.Has(AxisY) && s.max.H > 0.5
	showX := s.axes.Has(AxisX) && s.max.W > 0.5
	var r Corners
	r = r.Uniform(scrollbarThickness / 2)
	if showY {
		track := b.Size.H - 2*scrollbarInset
		if showX {
			track -= scrollbarThickness + scrollbarInset
		}
		content := b.Size.H + s.max.H
		thumb := minPixels(track, maxPixels(scrollbarMinThumb, track*b.Size.H/content))
		pos := (track - thumb) * s.offset.Y / s.max.H
		if track > 0 {
			w.PaintQuad(Quad{
				Bounds: Bounds{
					Origin: Point{X: b.Max().X - scrollbarInset - scrollbarThickness, Y: b.Origin.Y + scrollbarInset + pos},
					Size:   Size{W: scrollbarThickness, H: thumb},
				},
				Background: BackgroundSolid, Fill: scrollbarColor, CornerRadii: r,
			})
		}
	}
	if showX {
		track := b.Size.W - 2*scrollbarInset
		if showY {
			track -= scrollbarThickness + scrollbarInset
		}
		content := b.Size.W + s.max.W
		thumb := minPixels(track, maxPixels(scrollbarMinThumb, track*b.Size.W/content))
		pos := (track - thumb) * s.offset.X / s.max.W
		if track > 0 {
			w.PaintQuad(Quad{
				Bounds: Bounds{
					Origin: Point{X: b.Origin.X + scrollbarInset + pos, Y: b.Max().Y - scrollbarInset - scrollbarThickness},
					Size:   Size{W: thumb, H: scrollbarThickness},
				},
				Background: BackgroundSolid, Fill: scrollbarColor, CornerRadii: r,
			})
		}
	}
}
