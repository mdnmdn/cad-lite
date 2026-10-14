package gpui

import "time"

// TooltipStyle is the look and timing of tooltips shown by Div.Tooltip. Set it
// on the App with SetTooltipStyle; sizes are logical pixels and follow the
// window UIScale like the rest of the element tree.
type TooltipStyle struct {
	// Background, Border and Foreground are the bubble fill, outline and text colours.
	Background, Border, Foreground Rgba
	// BorderWidth is the outline width; zero draws none.
	BorderWidth float32
	// FontSize is the text size.
	FontSize float32
	// PaddingX and PaddingY are the inner padding.
	PaddingX, PaddingY float32
	// CornerRadius rounds the bubble.
	CornerRadius float32
	// MaxWidth wraps longer text; zero means no limit.
	MaxWidth float32
	// Delay is how long the pointer must rest on the element before the bubble shows.
	Delay time.Duration
	// Offset is the gap between the pointer and the bubble.
	Offset float32
}

// DefaultTooltipStyle returns the built-in dark tooltip style.
func DefaultTooltipStyle() TooltipStyle {
	return TooltipStyle{
		Background:   MustParseHex("#1e1e22"),
		Border:       MustParseHex("#4a4a52"),
		Foreground:   MustParseHex("#e8e8ec"),
		BorderWidth:  1,
		FontSize:     12,
		PaddingX:     8,
		PaddingY:     4,
		CornerRadius: 4,
		MaxWidth:     320,
		Delay:        500 * time.Millisecond,
		Offset:       18,
	}
}

// SetTooltipStyle replaces the tooltip style for every window and redraws them.
func (a *App) SetTooltipStyle(s TooltipStyle) {
	a.tooltipStyle = &s
	a.RefreshWindows()
}

// TooltipStyle returns the current tooltip style (DefaultTooltipStyle until set).
func (a *App) TooltipStyle() TooltipStyle {
	if a == nil || a.tooltipStyle == nil {
		return DefaultTooltipStyle()
	}
	return *a.tooltipStyle
}

// Tooltip shows text in a bubble after the pointer rests on the div for the
// style's Delay. It hides on leave, mouse down, scroll and key press, and is
// drawn above everything. An empty text removes the tooltip.
func (d *Div) Tooltip(text string) *Div {
	d.tooltip = text
	return d
}

const tooltipElementKey = "__gpui_tooltip"

// tooltipState is the per-window tooltip tracker.
type tooltipState struct {
	key     string
	text    string
	timer   *Timer
	visible bool
	// dismissed holds the key of an element whose tooltip was hidden by a
	// press, scroll or key; it stays hidden until the pointer leaves.
	dismissed string
	pos       Point
}

// tooltipTarget returns the innermost tooltip text on the hit path at pos.
func (w *Window) tooltipTarget(pos Point) (key, text string) {
	ls := w.renderedFrame.mouseListeners
	for _, i := range w.hitPath(pos) {
		if ls[i].tooltip != "" {
			return globalIDKey(ls[i].elementID), ls[i].tooltip
		}
	}
	return "", ""
}

// updateTooltip re-evaluates the tooltip for the pointer at pos. moved is true
// for a real pointer move, which restarts a pending delay.
func (w *Window) updateTooltip(pos Point, moved bool) {
	t := &w.tooltip
	key, text := w.tooltipTarget(pos)
	if key == "" {
		w.hideTooltip()
		t.dismissed = ""
		return
	}
	if key != t.dismissed {
		t.dismissed = ""
	}
	if key == t.key && key == t.dismissed {
		return
	}
	if key == t.key && t.visible {
		if text != t.text {
			t.text = text
			w.Refresh()
		}
		return
	}
	if key == t.key && !moved && !t.timer.Done() {
		t.text = text
		return
	}
	if key == t.dismissed {
		return
	}
	w.hideTooltip()
	t.key, t.text, t.pos = key, text, pos
	delay := w.app.TooltipStyle().Delay
	t.timer = w.After(delay, func(w *Window, _ *App) {
		if w.tooltip.key == key {
			w.tooltip.visible = true
			w.tooltip.pos = w.mousePosition
		}
	})
}

// hideTooltip cancels a pending tooltip or hides the visible one.
func (w *Window) hideTooltip() {
	t := &w.tooltip
	t.timer.Cancel()
	t.timer = nil
	if t.visible {
		w.Refresh()
	}
	t.visible = false
	t.key, t.text = "", ""
}

// dismissTooltip hides the tooltip until the pointer leaves its element.
func (w *Window) dismissTooltip() {
	if w.tooltip.key == "" {
		return
	}
	key := w.tooltip.key
	w.hideTooltip()
	w.tooltip.dismissed = key
}

// paintTooltip draws the visible tooltip above the finished frame, clamped
// inside the window. It registers no hitboxes, so it never takes input.
func (w *Window) paintTooltip(cx *App) {
	t := &w.tooltip
	if !t.visible || t.text == "" || w.closed {
		return
	}
	st := w.app.TooltipStyle()
	d := NewDiv().ID(tooltipElementKey).Bg(st.Background).Rounded(st.CornerRadius).
		Px_(st.PaddingX).Py_(st.PaddingY).TextSize(st.FontSize).TextColor(st.Foreground)
	if st.BorderWidth > 0 {
		d.Border(st.BorderWidth, st.Border)
	}
	win := w.Bounds()
	maxW := float32(win.Width())
	if st.MaxWidth > 0 {
		maxW = min(maxW, st.MaxWidth)
	}
	d.MaxW(maxW)
	d.Child(NewText(t.text))
	el := IntoElementOf(d)
	lid, state := el.RequestLayout(w, cx)
	node := w.layoutNodes[lid]
	if node == nil {
		return
	}
	Layout(node, Available{})
	if node.Rect.Size.W >= Pixels(maxW) {
		Layout(node, UnboundedHeight(maxW))
	}
	sz := node.Rect.Size
	x, y := t.pos.X, t.pos.Y+Pixels(st.Offset)
	if y+sz.H > win.Height() {
		y = t.pos.Y - Pixels(st.Offset)/3 - sz.H
	}
	x = min(x, win.Width()-sz.W)
	y = min(y, win.Height()-sz.H)
	x, y = max(x, 0), max(y, 0)
	rn := w.buildRenderNode(el, GlobalElementID{Path: []ElementPathKey{{Key: tooltipElementKey}}}, node, state, Point{X: x, Y: y}, cx)
	hb, mb, kb := w.nextFrame.hitboxes.Len(), len(w.nextFrame.mouseListeners), len(w.nextFrame.keyNodes)
	w.prepaintTree(rn, cx, false)
	w.nextFrame.hitboxes.truncate(hb)
	w.nextFrame.mouseListeners = w.nextFrame.mouseListeners[:mb]
	w.nextFrame.keyNodes = w.nextFrame.keyNodes[:kb]
	w.paintTree(rn, cx)
}
