package gpui

import "math"

// UniformList is a virtualized, vertically scrolling list of rows of equal
// height (GPUI's uniform_list). Each frame the row height is measured by
// laying out item 0 at the list's content width, and render is called once
// with the half-open range of rows that intersect the viewport; only those
// rows are laid out, prepainted and painted.
type UniformList struct {
	StyledMixin
	id            string
	count         int
	render        func(start, end int, w *Window, cx *App) []IntoElement
	scrollHandle  *ScrollHandle
	showScrollbar bool
}

// NewUniformList returns a list of count rows keyed by id; render returns
// the elements of rows [start, end). The list scrolls vertically and, with
// an Auto height, sizes itself to count rows.
func NewUniformList(id string, count int, render func(start, end int, w *Window, cx *App) []IntoElement) *UniformList {
	u := &UniformList{StyledMixin: NewStyledMixin(), id: id, count: count, render: render}
	u.OverflowYScroll()
	return u
}

// TrackScroll binds h to the list: h reports and controls the scroll
// offset, and ScrollToItem(ix) reveals row ix.
func (u *UniformList) TrackScroll(h *ScrollHandle) *UniformList {
	u.scrollHandle = h
	return u
}

// ShowScrollbar opts the list into a thin overlay scrollbar.
func (u *UniformList) ShowScrollbar() *UniformList {
	u.showScrollbar = true
	return u
}

// IntoElement satisfies IntoElement via the uniformListElement adapter.
func (u *UniformList) IntoElement() Element { return uniformListElement{u: u} }

// uniformListElement adapts *UniformList to Element.
type uniformListElement struct{ u *UniformList }

// uniformListLayout is the RequestLayout state: the leaf node, the text
// style rows inherit (the enclosing one refined by the list's) and a
// per-frame cache of item-0 measurements keyed by width.
type uniformListLayout struct {
	node      *LayoutNode
	textStyle TextStyle
	measured  map[float32]Size
}

// ID returns the list's id.
func (e uniformListElement) ID() (ElementID, bool) {
	if e.u.id == "" {
		return ElementID{}, false
	}
	return ElementID{Path: []ElementPathKey{{Key: e.u.id}}}, true
}

// measureItem lays out item 0 at width (unbounded when width is not a
// positive finite number) and returns its border-box size.
func (e uniformListElement) measureItem(w *Window, cx *App, st *uniformListLayout, width float32) Size {
	u := e.u
	if u.count <= 0 || u.render == nil {
		return Size{}
	}
	if width <= 0 || width >= math.MaxFloat32/2 {
		width = -1
	}
	if s, ok := st.measured[width]; ok {
		return s
	}
	var size Size
	w.pushTextStyle(st.textStyle)
	defer w.popTextStyle()
	items := u.render(0, 1, w, cx)
	if len(items) > 0 {
		if el := IntoElementOf(items[0]); el != nil {
			lid, _ := el.RequestLayout(w, cx)
			if n := w.layoutNodes[lid]; n != nil {
				avail := Available{}
				if width > 0 {
					avail = UnboundedHeight(width)
				}
				size = Layout(n, avail).Size
			}
		}
	}
	st.measured[width] = size
	return size
}

// RequestLayout registers a leaf node whose content size is count rows of
// item 0's height.
func (e uniformListElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	style := e.u.ResolvedStyle(DefaultStyle())
	style.TextStyle = e.u.style.RefineText(w.currentTextStyle())
	node := &LayoutNode{Style: style, Text: "uniform_list"}
	st := &uniformListLayout{node: node, textStyle: style.TextStyle, measured: map[float32]Size{}}
	node.Measure = func(_ string, _ TextStyle, maxWidth float32) (float32, float32) {
		s := e.measureItem(w, cx, st, maxWidth)
		return float32(s.W), float32(s.H) * float32(e.u.count)
	}
	return w.registerLayoutNode(node), st
}

// uniformListPrepaint is the per-frame prepaint state.
type uniformListPrepaint struct {
	bounds    Bounds
	style     Style
	textStyle TextStyle
	textSet   TextFields
	children  []*renderNode
	scroll    *scrollbarInfo
}

// paintTextOverride re-asserts the list's own text paint fields, so an
// ancestor's hover/focus text style does not recolour rows the list styles.
func (st *uniformListPrepaint) paintTextOverride() (textOverride, bool) {
	return textOverride{set: st.textSet & paintTextFields, ts: st.textStyle}, false
}

func (st *uniformListPrepaint) clipsChildren() bool            { return true }
func (st *uniformListPrepaint) paintOpacity(w *Window) float32 { return st.style.Opacity }
func (st *uniformListPrepaint) lateChildren() []*renderNode    { return st.children }
func (st *uniformListPrepaint) paintScrollbar(w *Window) {
	if st.scroll != nil {
		st.scroll.paint(w)
	}
}

// Prepaint measures the row height, resolves the scroll offset, registers
// the list's scrollable hitbox and builds render nodes for the visible
// rows, which the window then prepaints and paints as children.
func (e uniformListElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	u := e.u
	gid := w.currentElementID
	ls, _ := layoutState.(*uniformListLayout)
	if ls == nil {
		ls = &uniformListLayout{textStyle: u.style.RefineText(DefaultTextStyle()), measured: map[float32]Size{}}
	}
	style := u.ResolvedStyle(DefaultStyle())
	st := &uniformListPrepaint{bounds: bounds, style: style, textStyle: ls.textStyle, textSet: u.style.textSet()}
	pad := style.Padding
	content := bounds.Inset(pad)
	contentW := float32(maxPixels(0, content.Size.W))
	itemH := e.measureItem(w, cx, ls, contentW).H

	total := itemH*Pixels(u.count) + pad.Vertical()
	limit := Size{H: maxPixels(0, total-bounds.Size.H)}
	off := w.syncScroll(gid, u.scrollHandle, bounds, pad, limit, AxisY, func(ix int) (Bounds, bool) {
		if ix < 0 || ix >= u.count || itemH <= 0 {
			return Bounds{}, false
		}
		return Bounds{Origin: Point{X: pad.Left, Y: pad.Top + itemH*Pixels(ix)}, Size: Size{W: content.Size.W, H: itemH}}, true
	})

	hb := w.InsertHitbox(bounds)
	ml := &w.nextFrame.mouseListeners[hb]
	ml.elementID = gid
	ml.scrollable = true
	ml.scrollAxes = AxisY
	ml.scrollMax = limit
	if u.showScrollbar {
		st.scroll = &scrollbarInfo{bounds: bounds, offset: off, max: limit, axes: AxisY}
	}

	if itemH <= 0 || u.count <= 0 || u.render == nil {
		return st
	}
	start := int(math.Floor(float64((off.Y - pad.Top) / itemH)))
	end := int(math.Ceil(float64((off.Y - pad.Top + bounds.Size.H) / itemH)))
	start = max(0, min(start, u.count))
	end = max(start, min(end, u.count))
	if start == end {
		return st
	}
	w.pushTextStyle(ls.textStyle)
	defer w.popTextStyle()
	items := u.render(start, end, w, cx)
	for j, it := range items {
		el := IntoElementOf(it)
		if el == nil {
			continue
		}
		ix := start + j
		lid, cstate := el.RequestLayout(w, cx)
		node := w.layoutNodes[lid]
		if node != nil {
			Layout(node, Definite(contentW, float32(itemH)))
		}
		key := ElementPathKey{Index: ix}
		if local, ok := el.ID(); ok && len(local.Path) > 0 {
			key.Key = local.Path[len(local.Path)-1].Key
		}
		cpath := make([]ElementPathKey, 0, len(gid.Path)+1)
		cpath = append(cpath, gid.Path...)
		cpath = append(cpath, key)
		origin := Point{X: content.Origin.X, Y: content.Origin.Y + itemH*Pixels(ix) - off.Y}
		st.children = append(st.children, w.buildRenderNode(el, GlobalElementID{Path: cpath}, node, cstate, origin, cx))
	}
	return st
}

// Paint paints the list's own box (shadows, background, border); rows are
// painted by the window's walk.
func (e uniformListElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	if st, ok := prepaintState.(*uniformListPrepaint); ok {
		paintBox(w, bounds, st.style)
		return
	}
	paintBox(w, bounds, e.u.ResolvedStyle(DefaultStyle()))
}

// SizeFull is StyledMixin.SizeFull returning *UniformList.
func (u *UniformList) SizeFull() *UniformList { u.StyledMixin.SizeFull(); return u }

// SizeWH is StyledMixin.SizeWH returning *UniformList.
func (u *UniformList) SizeWH(w, h float32) *UniformList { u.StyledMixin.SizeWH(w, h); return u }

// SizeW is StyledMixin.SizeW returning *UniformList.
func (u *UniformList) SizeW(w float32) *UniformList { u.StyledMixin.SizeW(w); return u }

// SizeH is StyledMixin.SizeH returning *UniformList.
func (u *UniformList) SizeH(h float32) *UniformList { u.StyledMixin.SizeH(h); return u }

// Flex1 is StyledMixin.Flex1 returning *UniformList.
func (u *UniformList) Flex1() *UniformList { u.StyledMixin.Flex1(); return u }

// MinH is StyledMixin.MinH returning *UniformList.
func (u *UniformList) MinH(v float32) *UniformList { u.StyledMixin.MinH(v); return u }

// MaxH is StyledMixin.MaxH returning *UniformList.
func (u *UniformList) MaxH(v float32) *UniformList { u.StyledMixin.MaxH(v); return u }

// P is StyledMixin.P returning *UniformList.
func (u *UniformList) P(v float32) *UniformList { u.StyledMixin.P(v); return u }

// Px_ is StyledMixin.Px_ returning *UniformList.
func (u *UniformList) Px_(v float32) *UniformList { u.StyledMixin.Px_(v); return u }

// Py_ is StyledMixin.Py_ returning *UniformList.
func (u *UniformList) Py_(v float32) *UniformList { u.StyledMixin.Py_(v); return u }

// Bg is StyledMixin.Bg returning *UniformList.
func (u *UniformList) Bg(f Fill) *UniformList { u.StyledMixin.Bg(f); return u }

// Rounded is StyledMixin.Rounded returning *UniformList.
func (u *UniformList) Rounded(px float32) *UniformList { u.StyledMixin.Rounded(px); return u }

// Border is StyledMixin.Border returning *UniformList.
func (u *UniformList) Border(w float32, c Rgba) *UniformList { u.StyledMixin.Border(w, c); return u }

// Opacity is StyledMixin.Opacity returning *UniformList.
func (u *UniformList) Opacity(f float32) *UniformList { u.StyledMixin.Opacity(f); return u }

// TextColor is StyledMixin.TextColor returning *UniformList.
func (u *UniformList) TextColor(c Rgba) *UniformList { u.StyledMixin.TextColor(c); return u }
