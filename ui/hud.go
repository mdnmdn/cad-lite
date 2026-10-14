package ui

import (
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Properties HUD metrics, in logical pixels.
const (
	hudWidth = 220
	// hudRight is the HUD's distance from the canvas's right edge: it sits left of the zoom
	// buttons (overlayInset, their 48 px column and a gap).
	hudRight = overlayInset + 48 + 8
	// hudAlpha is the opacity of the HUD's fill (of 255). Nearly opaque: it only has to let the
	// drawing show through faintly, and its box is small, so the blend costs little.
	hudAlpha = 230
)

// PropsHUDShown reports whether the properties HUD is on.
func (ws *Workspace) PropsHUDShown() bool { return ws.status().hud }

// togglePropsHUD turns the properties HUD on or off.
func (ws *Workspace) togglePropsHUD() { s := ws.status(); s.hud = !s.hud }

// propsHUD is the floating, read-only Properties panel at the canvas's top right, level with and
// left of the zoom buttons (below the ruler when it shows): the same lines as the side panel's Properties section, on a nearly opaque fill that
// covers only its own box. It is a child of the status bar, placed from the canvas's and the bar's
// bounds in the last frame (winW is the window's width); when either has moved since, it asks for
// another frame. Presses and the wheel on it stay on it, the canvas gets them everywhere else;
// releases go through to the root, so a splitter or slider drag let go over it still ends.
func (ws *Workspace) propsHUD(winW float32) *gpui.Div {
	st := ws.status()
	b, barTop := ws.canvas.bounds, st.barTop
	check := gpui.NewCanvas(func(_ gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		if ws.canvas.bounds != b || st.barTop != barTop {
			w.Refresh()
		}
	})
	_, ruler := ws.rulerInsets()
	fill := uiPanel
	fill.A = hudAlpha
	stop := func(w *gpui.Window) { w.StopPropagation() }
	return gpui.NewDiv().ID("props-hud").Absolute().
		Top(float32(b.Origin.Y)+ruler+overlayInset-barTop).Right(winW-float32(b.Origin.X+b.Size.W)+hudRight).
		SizeW(hudWidth).FlexCol().Gap(6).Px_(12).Py_(8).Rounded(8).Bg(fill).Border(1, uiBorder).TextSize(fontUI).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { stop(w) }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { stop(w) }).
		Child(
			gpui.NewDiv().SizeWH(0, 0).Child(check),
			gpui.NewText(i18n.T("ui.panel.properties")).FontWeight(700).TextSize(fontSmall).TextColor(uiMuted),
			ws.propsBody(),
		)
}
