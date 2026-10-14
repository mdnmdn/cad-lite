package ui

import (
	"fmt"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// sidePanelWidth is the default width of the Layers and Properties column for a window of the
// given width: a sixth of it, between 220 and 340 px.
func sidePanelWidth(window float32) float32 { return min(340, max(220, window/6)) }

// sectionHeader is a section heading (semibold, normal case): the disclosure chevron and the
// title, which fold the section when clicked, and an optional control at the right.
func (ws *Workspace) sectionHeader(id, title string, off bool, collapseTip, expandTip string, flag *bool, right ...gpui.IntoElement) *gpui.Div {
	icon, tip := icons.AppChevronDown, collapseTip
	if off {
		icon, tip = icons.AppChevronRight, expandTip
	}
	head := gpui.NewDiv().ID(id).FlexRow().ItemsCenter().Gap(4).SizeH(24).Px_(4).Rounded(5).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.toggleFold(flag, w) }).
		Child(NewIcon(icon, 14, uiMuted), gpui.NewText(title).FontWeight(700).TextSize(fontTitle))
	h := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).SizeH(sectionHeaderHeight).FlexShrink(0).Px_(8).Child(head, gpui.NewDiv().Flex1())
	for _, r := range right {
		h.Child(r)
	}
	return h
}

// sidePanel is the right-hand column, width wide: the Layers section with its add button, a
// splitter, and the Properties section. The Layers section is as tall as its rows need (or the
// height the user dragged it to, if shorter) and scrolls when the rows do not fit; Properties
// takes the rest. Either section folds to its header and the whole column folds to a strip.
func (ws *Workspace) sidePanel(width float32) *gpui.Div {
	if ws.lay.sideOff {
		expand := collapseButton("side-expand", i18n.T("ui.panel.side_expand_tip"), icons.AppChevronLeft, func(w *gpui.Window) { ws.toggleFold(&ws.lay.sideOff, w) })
		return gpui.NewDiv().ID("side-strip").FlexCol().ItemsCenter().SizeW(width).FlexShrink(0).Bg(uiPanel).Pt(6).Child(expand)
	}
	d := ws.session.Drawing()
	layers := gpui.NewDiv().FlexCol().Gap(layerRowGap).Px_(10).Pb(layerListPad)
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
		layers.Child(gpui.NewDiv().ID("layer-"+l.Name).FlexRow().ItemsCenter().Gap(8).Px_(6).SizeH(layerRowHeight).FlexShrink(0).Rounded(4).Tooltip(i18n.T("ui.layers.current_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				d.CurrentLayer = l.Name
				w.Refresh()
			}).
			Child(swatch, name, vis))
	}
	add := iconButton("layer-add", i18n.T("ui.panel.layer_add_tip"), icons.AppAdd, false, func(w *gpui.Window) { ws.run("LAYER", w) })
	add.SizeWH(24, 24)
	fold := collapseButton("side-collapse", i18n.T("ui.panel.side_collapse_tip"), icons.AppChevronRight, func(w *gpui.Window) { ws.toggleFold(&ws.lay.sideOff, w) })

	props := gpui.NewDiv().FlexCol().Gap(4).Px_(14).Pb(8)
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

	l := &ws.lay
	layersH := layersHeight(ws.sectionAvail(), len(d.Layers()), l.layersH, l.layersOff, l.propsOff)
	layersSec := gpui.NewDiv().FlexCol().SizeH(layersH).FlexShrink(1).MinH(sectionHeaderHeight).Child(
		ws.sectionHeader("layers-toggle", i18n.T("ui.panel.layers"), l.layersOff, i18n.T("ui.panel.layers_collapse_tip"), i18n.T("ui.panel.layers_expand_tip"), &l.layersOff, add, fold))
	if !l.layersOff {
		layersSec.Child(layers.ID("layers-scroll").Flex1().MinH(0).OverflowYScroll())
	}
	propsSec := gpui.NewDiv().FlexCol().MinH(sectionHeaderHeight).Child(
		ws.sectionHeader("props-toggle", i18n.T("ui.panel.properties"), l.propsOff, i18n.T("ui.panel.props_collapse_tip"), i18n.T("ui.panel.props_expand_tip"), &l.propsOff))
	if l.propsOff {
		propsSec.SizeH(sectionHeaderHeight).FlexShrink(0)
	} else {
		propsSec.Flex1().MinH(sectionHeaderHeight + minPropsBody).Child(props.ID("props-scroll").Flex1().MinH(0).OverflowYScroll())
	}
	var between gpui.IntoElement = gpui.NewDiv().SizeH(splitterSize).FlexShrink(0).FlexCol().JustifyCenter().Child(gpui.NewDiv().SizeH(1).Bg(uiBorder))
	if !l.layersOff && !l.propsOff {
		between = ws.splitter("split-layers", splitLayers, true, i18n.T("ui.panel.section_splitter_tip"),
			func() float32 { return layersH }, func() { l.layersH = 0 })
	}
	col := gpui.NewDiv().ID("side-panel").FlexCol().SizeW(width).FlexShrink(0).Bg(uiPanel).Pt(sideTopPad).
		OverflowYScroll().TrackScroll(l.col).
		Child(layersSec, between, propsSec)
	if l.propsOff {
		col.Child(gpui.NewDiv().Flex1())
	}
	return col
}
