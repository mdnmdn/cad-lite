package ui

import (
	"fmt"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// sidePanelWidth is the Layers and Properties column's width for a window of the given width: a
// fifth of it, between 220 and 410 px.
func sidePanelWidth(window float32) float32 { return min(410, max(220, window/5)) }

// panelHeader is a section heading (semibold, normal case) with an optional control at its right.
func panelHeader(title string, right gpui.IntoElement) *gpui.Div {
	h := gpui.NewDiv().FlexRow().ItemsCenter().SizeH(30).Px_(16).Child(
		gpui.NewText(title).FontWeight(700).TextSize(fontTitle).Flex1())
	if right != nil {
		h.Child(right)
	}
	return h
}

// sidePanel is the right-hand column: the Layers section with its add button, a divider, and the
// Properties section. Both sections scroll when their rows do not fit.
func (ws *Workspace) sidePanel(window float32) *gpui.Div {
	d := ws.session.Drawing()
	layers := gpui.NewDiv().FlexCol().Gap(2).Px_(10)
	for _, l := range d.Layers() {
		l := l
		swatch := gpui.NewDiv().SizeWH(12, 12).Rounded(2).Bg(gpuiColor(render.ACI(l.Color))).Tooltip(i18n.T("ui.layers.swatch_tip", l.Color))
		name := gpui.NewText(l.Name).Flex1()
		if l.Name == d.CurrentLayer {
			name.FontWeight(700).TextColor(uiOn)
		}
		state := i18n.T("ui.layers.on")
		stateCol := uiText
		if l.Off {
			state, stateCol = i18n.T("ui.layers.off"), uiMuted
		}
		if l.Frozen {
			state, stateCol = i18n.T("ui.layers.frozen"), uiMuted
		}
		vis := gpui.NewDiv().ID("layer-vis-" + l.Name).Px_(6).Rounded(3).Bg(uiPanelAlt).Tooltip(i18n.T("ui.layers.visibility_tip")).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				l.Off = !l.Off
				w.Refresh()
			}).
			Child(gpui.NewText(state).TextColor(stateCol))
		layers.Child(gpui.NewDiv().ID("layer-"+l.Name).FlexRow().ItemsCenter().Gap(8).Px_(6).Py_(4).Rounded(4).Tooltip(i18n.T("ui.layers.current_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				d.CurrentLayer = l.Name
				w.Refresh()
			}).
			Child(swatch, name, vis))
	}
	add := iconButton("layer-add", i18n.T("ui.panel.layer_add_tip"), icons.AppAdd, false, func(w *gpui.Window) { ws.run("LAYER", w) })
	add.SizeWH(26, 26)

	props := gpui.NewDiv().FlexCol().Gap(4).Px_(16)
	sel := ws.session.Selection()
	if sel.Len() == 0 {
		props.Child(gpui.NewText(i18n.T("ui.props.none")).TextColor(uiMuted))
	} else {
		counts := map[string]int{}
		var order []string
		for _, h := range sel.Handles() {
			if e := d.Entity(h); e != nil {
				if counts[e.Class()] == 0 {
					order = append(order, e.Class())
				}
				counts[e.Class()]++
			}
		}
		props.Child(gpui.NewText(i18n.Plural("ui.props.selected", sel.Len())).FontWeight(700))
		for _, c := range order {
			props.Child(gpui.NewText(fmt.Sprintf("%s × %d", c, counts[c])).TextColor(uiMuted))
		}
		if sel.Len() == 1 {
			e := d.Entity(sel.Handles()[0])
			props.Child(gpui.NewText(i18n.T("ui.props.layer", e.Common().Layer)).TextColor(uiMuted),
				gpui.NewText(i18n.T("ui.props.handle", e.Handle().String())).TextColor(uiMuted))
		}
	}
	section := func(id string, header gpui.IntoElement, body *gpui.Div) *gpui.Div {
		return gpui.NewDiv().FlexCol().Flex1().MinH(0).Child(
			header,
			body.ID(id).Flex1().MinH(0).OverflowYScroll())
	}
	return gpui.NewDiv().ID("side-panel").FlexCol().SizeW(sidePanelWidth(window)).FlexShrink(0).Bg(uiPanel).Pt(8).
		Child(
			section("layers-scroll", panelHeader(i18n.T("ui.panel.layers"), add), layers),
			hDivider(),
			gpui.NewDiv().FlexCol().Flex1().MinH(0).Pt(8).Child(
				panelHeader(i18n.T("ui.panel.properties"), nil),
				props.ID("props-scroll").Flex1().MinH(0).OverflowYScroll()),
		)
}
