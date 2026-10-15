package ui

import (
	"fmt"
	"slices"
	"weak"

	"cad-lite/assets/icons"
	"cad-lite/doc"
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
		layers.Child(ws.layerRow("layer-", d, l, false, func(w *gpui.Window) {
			ws.makeLayerCurrent(l)
			w.Refresh()
		}))
	}
	add := iconButton("layer-add", i18n.T("ui.panel.layer_add_tip"), icons.AppAdd, false, func(w *gpui.Window) { ws.run("LAYER", w) })
	add.SizeWH(24, 24)
	fold := collapseButton("side-collapse", i18n.T("ui.panel.side_collapse_tip"), icons.AppChevronRight, func(w *gpui.Window) { ws.toggleFold(&ws.lay.sideOff, w) })

	props := ws.propsBody().Px_(14).Pb(8)

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

// layerRow is one layer of a layer list, shared by the side panel and the status bar's layer
// popover so both look and act alike: the colour swatch, the name (bold and accented when the
// layer is current), reorder arrows, the lock switch and the visibility switch. idPrefix starts every element id
// ("layer-" in the panel, so the row is "layer-<name>" and its switches "layer-vis-<name>" and
// "layer-lock-<name>"). A click on the row runs pick, except on a frozen layer, which cannot be
// current (as -LAYER Set refuses it; the row's tooltip says so); the switches flip their flag
// without picking. hi marks the popover's keyboard highlight.
func (ws *Workspace) layerRow(idPrefix string, d *doc.Drawing, l *doc.Layer, hi bool, pick func(*gpui.Window)) *gpui.Div {
	swatch := gpui.NewDiv().SizeWH(12, 12).Rounded(2).FlexShrink(0).Bg(gpuiColor(render.ACI(l.Color))).Tooltip(i18n.T("ui.layers.swatch_tip", l.Color))
	name := gpui.NewText(l.Name).Flex1().MinW(0)
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
	flip := func(f func()) func(*gpui.ClickEvent, *gpui.Window, *gpui.App) {
		return func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			f()
			ws.session.MarkModified()
			w.StopPropagation()
			w.Refresh()
		}
	}
	vis := gpui.NewDiv().ID(idPrefix + "vis-" + l.Name).Px_(6).Rounded(3).FlexShrink(0).Bg(uiPanelAlt).Tooltip(i18n.T("ui.layers.visibility_tip")).
		OnClick(flip(func() { l.Off = !l.Off })).
		Child(gpui.NewText(state).TextColor(stateCol))
	lockIcon, lockCol := icons.LayerUnlocked, uiMuted
	if l.Locked {
		lockIcon, lockCol = icons.LayerLocked, uiOn
	}
	lock := gpui.NewDiv().ID(idPrefix+"lock-"+l.Name).FlexRow().ItemsCenter().JustifyCenter().SizeWH(20, 20).Rounded(3).FlexShrink(0).
		Tooltip(i18n.T("ui.layers.lock_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(flip(func() { l.Locked = !l.Locked })).
		Child(NewIcon(lockIcon, 14, lockCol))
	tip := i18n.T("ui.layers.current_tip")
	if l.Frozen {
		tip = i18n.T("ui.layers.frozen_current_tip")
	}
	row := gpui.NewDiv().ID(idPrefix+l.Name).FlexRow().ItemsCenter().Gap(4).Px_(6).SizeH(layerRowHeight).FlexShrink(0).Rounded(4).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			if !l.Frozen {
				pick(w)
			}
		}).
		Child(swatch, ws.layerOrderButtons(idPrefix, d, l), name, lock, vis)
	if hi {
		row.Bg(activeFill())
	}
	return row
}

// propsSummary is what the Properties lines say about a selection, before translation.
type propsSummary struct {
	// d, handles and mods are what it was worked out from: the drawing (weakly, as the cache
	// lives in statusTable), the selection and the session's modified counter.
	d       weak.Pointer[doc.Drawing]
	handles []doc.Handle
	mods    int
	valid   bool

	n             int
	classes       []string
	counts        []int
	layer, handle string
}

// selectionSummary returns the summary of the current selection, worked out again only when the
// drawing, the selection or the drawing's contents changed since the last call: the side panel
// and the HUD ask for it every frame.
func (ws *Workspace) selectionSummary() *propsSummary {
	c := &ws.status().props
	d, sel, mods := ws.session.Drawing(), ws.session.Selection(), ws.session.Modified()
	if c.valid && c.d == weak.Make(d) && c.mods == mods && slices.Equal(c.handles, sel.Handles()) {
		return c
	}
	*c = propsSummary{d: weak.Make(d), handles: append(c.handles[:0], sel.Handles()...), mods: mods, valid: true, n: sel.Len()}
	for _, h := range sel.Handles() {
		e := d.Entity(h)
		if e == nil {
			continue
		}
		if i := slices.Index(c.classes, e.Class()); i >= 0 {
			c.counts[i]++
		} else {
			c.classes, c.counts = append(c.classes, e.Class()), append(c.counts, 1)
		}
	}
	if sel.Len() == 1 {
		if e := d.Entity(sel.Handles()[0]); e != nil {
			c.layer, c.handle = e.Common().Layer, e.Handle().String()
		}
	}
	return c
}

// propsBody is what the Properties section shows for the selection: "No selection", or the count,
// the count per object type and, for a single object, its layer and handle. The side panel and the
// properties HUD both show it.
func (ws *Workspace) propsBody() *gpui.Div {
	props := gpui.NewDiv().FlexCol().Gap(4)
	c := ws.selectionSummary()
	if c.n == 0 {
		return props.Child(gpui.NewText(i18n.T("ui.props.none")).TextColor(uiMuted))
	}
	props.Child(gpui.NewText(i18n.Plural("ui.props.selected", c.n)).FontWeight(700))
	for i, cl := range c.classes {
		props.Child(gpui.NewText(fmt.Sprintf("%s × %d", cl, c.counts[i])).TextColor(uiMuted))
	}
	if c.handle != "" {
		props.Child(gpui.NewText(i18n.T("ui.props.layer", c.layer)).TextColor(uiMuted),
			gpui.NewText(i18n.T("ui.props.handle", c.handle)).TextColor(uiMuted))
	}
	return props
}
