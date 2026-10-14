package ui

import (
	"fmt"
	"math"

	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// The zoom slider covers 1e-3 to 1e3 pixels per model unit (0.1% to
// 100000%), logarithmically; a scale outside it pins the thumb to the end.
const (
	zoomSliderMinExp = -3
	zoomSliderMaxExp = 3
	// zoomButtonFactor is the scale step of the - and + buttons.
	zoomButtonFactor = 1.25
	zoomSliderWidth  = 110
)

// zoomToSlider maps a view scale to the slider's value, its decimal log.
func zoomToSlider(scale float64) float64 {
	return SnapSlider(math.Log10(math.Max(scale, render.MinScale)), zoomSliderMinExp, zoomSliderMaxExp, 0)
}

// sliderToZoom is the inverse of zoomToSlider.
func sliderToZoom(v float64) float64 {
	return math.Max(render.MinScale, math.Min(render.MaxScale, math.Pow(10, v)))
}

// zoomPercent formats a view scale as a percentage, where 100% is one
// screen pixel per model unit.
func zoomPercent(scale float64) string {
	p := scale * 100
	switch {
	case p >= 100:
		return fmt.Sprintf("%.0f%%", p)
	case p >= 10:
		return fmt.Sprintf("%.1f%%", p)
	}
	return fmt.Sprintf("%.2g%%", p)
}

// setZoom sets the view scale about the canvas centre. A drag puts the view
// it started from on the ZOOM Previous stack once; a click on a button or a
// key press is a step of its own.
func (ws *Workspace) setZoom(scale float64, drag bool) {
	v := ws.view
	if v.Width <= 0 || v.Height <= 0 {
		return
	}
	h := v.History()
	if !drag || !ws.zoomSaved {
		h.Save(*v)
		ws.zoomSaved = drag
	}
	v.ZoomAt(v.Width/2, v.Height/2, math.Max(render.MinScale, math.Min(render.MaxScale, scale))/v.Scale)
	if !drag {
		h.Settle(*v)
	}
}

// zoomControls is the status bar's zoom group: - and + buttons around the
// slider, and the scale as a percentage.
func (ws *Workspace) zoomControls() *gpui.Div {
	step := func(id, label, tip string, f float64) *gpui.Div {
		return gpui.NewDiv().ID("status-zoom-" + id).Px_(6).Rounded(3).Tooltip(tip).FlexShrink(0).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				ws.setZoom(ws.view.Scale*f, false)
				w.Refresh()
			}).
			Child(gpui.NewText(label).FontWeight(700))
	}
	slider := NewSlider("status-zoom-slider", &ws.zoom, SliderOptions{
		Value: zoomToSlider(ws.view.Scale), Min: zoomSliderMinExp, Max: zoomSliderMaxExp,
		OnChange: func(v float64, _ *gpui.Window) { ws.setZoom(sliderToZoom(v), ws.zoom.Dragging()) },
		OnCommit: func(_ float64, _ *gpui.Window) {
			ws.zoomSaved = false
			ws.view.History().Settle(*ws.view)
		},
	}).SizeW(zoomSliderWidth).FlexShrink(0).Tooltip(i18n.T("ui.status.zoom_slider_tip"))
	return gpui.NewDiv().ID("status-zoom").FlexRow().ItemsCenter().Gap(2).FlexShrink(0).Children(
		step("out", "−", i18n.T("ui.status.zoom_out_tip"), 1/zoomButtonFactor),
		slider,
		step("in", "+", i18n.T("ui.status.zoom_in_tip"), zoomButtonFactor),
		gpui.NewDiv().ID("status-zoom-readout").SizeW(58).Tooltip(i18n.T("ui.status.zoom_readout_tip")).
			Child(gpui.NewText(zoomPercent(ws.view.Scale)).TextColor(uiMuted)),
	)
}
