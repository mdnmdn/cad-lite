package ui

import (
	"fmt"

	"cad-lite/cmdsys"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// statusBarMinHeight keeps the status bar the same height whether or not its groups wrap.
const statusBarMinHeight = 32

// layoutTab is a tab of the status bar's layout switcher. Only Model works: paper space layouts
// are not supported, so the others are disabled and say so in their tooltip.
func layoutTab(id, label string, selected bool) *gpui.Div {
	t := gpui.NewDiv().ID("status-tab-" + id).Px_(10).Py_(3).Rounded(6).FlexShrink(0)
	if selected {
		return t.Bg(uiPanelAlt).Tooltip(i18n.T("ui.status.model_tab_tip")).Child(gpui.NewText(label).TextSize(fontUI).FontWeight(700))
	}
	return t.Opacity(0.55).Tooltip(i18n.T("ui.status.layout_tip")).Child(gpui.NewText(label).TextSize(fontUI).TextColor(uiMuted))
}

// toggleChip is a status bar switch: monospace caps, muted while off, and the soft accent fill,
// border and text while on. f flips the state.
func toggleChip(id, label, tip string, on bool, f func()) *gpui.Div {
	chip := gpui.NewDiv().ID("status-"+id).Px_(8).Py_(2).Rounded(6).Border(1, uiBg).FlexShrink(0).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() })
	col := uiMuted
	if on {
		col = uiOn
		chip.Bg(activeFill()).Border(1, activeBorder())
	}
	return chip.Child(monoText(label, fontSmall).FontWeight(700).TextColor(col))
}

// staticChip is a status bar label that is not a switch.
func staticChip(id, label, tip string) *gpui.Div {
	return gpui.NewDiv().ID("status-" + id).Px_(8).Py_(2).FlexShrink(0).Tooltip(tip).
		Child(monoText(label, fontSmall).FontWeight(700).TextColor(uiMuted))
}

// statusBar is the bottom row: the layout tabs and the cursor coordinates at the left; at the
// right the MODEL label, the drafting-aid switches, the current layer, the entity count, the
// drawing unit and the zoom slider. The groups wrap rather than squeeze when the window is narrow.
func (ws *Workspace) statusBar() *gpui.Div {
	s := ws.session
	cur := s.Cursor()
	coordCol := uiText
	if !ws.pointerOnCanvas() {
		coordCol = uiMuted
	}
	coords := monoText(fmt.Sprintf("%.4f, %.4f, %.4f", cur.X, cur.Y, 0.0), fontUI).TextColor(coordCol)
	left := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).FlexShrink(0).Child(
		layoutTab("model", i18n.T("ui.status.model"), true),
		layoutTab("layout1", i18n.T("ui.status.layout1"), false),
		layoutTab("layout2", i18n.T("ui.status.layout2"), false),
		gpui.NewDiv().ID("status-coords").Ml(10).Tooltip(i18n.T("ui.status.coords_tip")).Child(coords),
	)

	units := s.Units()
	if units == "" {
		units = i18n.T("ui.status.unitless")
	}
	chips := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).FlexShrink(0).Child(
		staticChip("model", i18n.T("ui.status.model_chip"), i18n.T("ui.status.model_chip_tip")),
		toggleChip("grid", i18n.T("ui.status.grid"), i18n.T("ui.status.grid_tip"), s.Grid(), func() { s.SetGrid(!s.Grid()) }),
		toggleChip("snapgrid", i18n.T("ui.status.snap"), i18n.T("ui.status.snap_tip"), s.SnapGrid(), func() { s.SetSnapGrid(!s.SnapGrid()) }),
		toggleChip("ortho", i18n.T("ui.status.ortho"), i18n.T("ui.status.ortho_tip"), s.Ortho, func() { s.SetOrtho(!s.Ortho) }),
		toggleChip("polar", i18n.T("ui.status.polar"), i18n.T("ui.status.polar_tip", cmdsys.FormatNumber(s.PolarAngle())), s.Polar(), func() { s.SetPolar(!s.Polar()) }),
		toggleChip("snap", i18n.T("ui.status.osnap"), i18n.T("ui.status.osnap_tip"), s.SnapOn, func() { s.SnapOn = !s.SnapOn }),
		toggleChip("lwt", i18n.T("ui.status.lwt"), i18n.T("ui.status.lwt_tip"), s.LineweightDisplay(), func() { s.SetLineweightDisplay(!s.LineweightDisplay()) }),
	)
	d := s.Drawing()
	info := gpui.NewDiv().FlexRow().ItemsCenter().Gap(14).FlexShrink(0).Child(
		gpui.NewDiv().ID("status-layer").Tooltip(i18n.T("ui.status.layer_tip")).
			Child(gpui.NewText(i18n.T("ui.status.layer", d.CurrentLayer)).TextSize(fontUI).TextColor(uiText)),
		gpui.NewText(i18n.Plural("ui.status.entities", d.Len())).TextSize(fontUI).TextColor(uiMuted),
		gpui.NewDiv().ID("status-units").Tooltip(i18n.T("ui.status.units_tip", units, cmdsys.FormatNumber(ws.view.PixelSize()))).
			Child(gpui.NewText(units).TextSize(fontUI).TextColor(uiMuted)),
	)
	right := gpui.NewDiv().FlexRow().ItemsCenter().Gap(14).FlexShrink(0).MlAuto().Child(chips, info, ws.zoomControls())
	return gpui.NewDiv().ID("statusbar").FlexRow().FlexWrap().ItemsCenter().Gap(6).Px_(12).Py_(3).MinH(statusBarMinHeight).FlexShrink(0).Bg(uiBg).TextSize(fontUI).Child(left, right)
}
