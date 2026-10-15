package ui

import (
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Properties HUD metrics, in logical pixels.
const (
	hudWidth = 184
	// hudAlpha is the shared zoom and Properties background opacity; labels stay opaque.
	hudAlpha = 200
)

// PropsHUDShown reports whether the properties HUD is on.
func (ws *Workspace) PropsHUDShown() bool { return ws.status().hud }

// togglePropsHUD turns the properties HUD on or off.
func (ws *Workspace) togglePropsHUD() {
	s := ws.status()
	s.hud = !s.hud
	s.hudDrag.active = false
}

// propsHUD is the floating, read-only Properties panel at the canvas's top right, level with and
// left of the zoom buttons by default: the same lines as the side panel's Properties section, on a translucent fill that
// covers only its own box. It is a child of the status bar, placed from the canvas's and the bar's
// bounds in the last frame; when either has moved since, it asks for
// another frame. Presses and the wheel on it stay on it, the canvas gets them everywhere else;
// releases go through to the root, so a splitter or slider drag let go over it still ends.
func (ws *Workspace) propsHUD() *gpui.Div {
	st := ws.status()
	b, barTop := ws.canvas.bounds, st.barTop
	p := ws.hudPosition()
	box := ws.hudBox(p)
	top, bottom := ws.hudVerticalRange(box.x, box.w)
	check := gpui.NewCanvas(func(actual gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		h := float32(actual.Size.H)
		if p.height != h || ws.canvas.bounds != b || st.barTop != barTop {
			p.height = h
			w.Refresh()
		}
	})
	check.Absolute().Inset(0)
	fill := uiPanel
	fill.A = hudAlpha
	stop := func(w *gpui.Window) { w.StopPropagation() }
	header := gpui.NewDiv().ID("props-hud-handle").FlexRow().ItemsCenter().Gap(6).FlexShrink(0).
		Tooltip(i18n.T("ui.props.drag_tip")).
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button == gpui.MouseLeft {
				ws.beginPropsDrag(e.Position, w)
			}
			w.StopPropagation()
		}).Child(gpui.NewDiv().SizeWH(10, 3).Rounded(1).Bg(uiMuted),
		gpui.NewText(i18n.T("ui.panel.properties")).FontWeight(700).TextSize(fontSmall).TextColor(uiMuted))
	return gpui.NewDiv().ID("props-hud").Absolute().
		Top(box.y-barTop).Left(box.x).SizeW(box.w).MaxH(bottom-top).FlexCol().Gap(3).Px_(8).Py_(6).Rounded(6).Bg(fill).Border(1, uiBorder).TextSize(12).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { stop(w) }).
		OnScrollWheel(func(e *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) {
			off := p.scroll.Offset()
			off.Y = min(max(off.Y+e.DeltaY, 0), p.scroll.MaxOffset().H)
			p.scroll.SetOffset(off)
			stop(w)
			w.Refresh()
		}).
		Child(
			check, header,
			ws.propsBody().Gap(3).MinH(0).FlexShrink(1).OverflowYScroll().TrackScroll(p.scroll),
		)
}
