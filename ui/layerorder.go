package ui

import (
	"cad-lite/assets/icons"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"slices"
)

func (ws *Workspace) layerOrderBusy() bool {
	return ws.session.Active() != "" || ws.session.Drawing().TransactionOpen()
}
func (ws *Workspace) moveLayerOrder(name string, delta int, w *gpui.Window) bool {
	if ws.layerOrderBusy() {
		ws.session.Say("ui.layers.reorder_busy")
		w.Refresh()
		return false
	}
	d := ws.session.Drawing()
	rank := d.LayerRank(name)
	next := rank + delta
	if rank >= len(d.Layers()) || next < 0 || next >= len(d.Layers()) || delta != 1 && delta != -1 {
		return false
	}
	names := make([]string, len(d.Layers()))
	for i, l := range d.Layers() {
		names[i] = l.Name
	}
	names[rank], names[next] = names[next], names[rank]
	state := ws.status()
	highlight := ""
	if state.pop {
		matches := layerMatches(d, ws.layerFilter().Text())
		if state.hi >= 0 && state.hi < len(matches) {
			highlight = matches[state.hi].Name
		}
	}
	tx, err := d.Begin()
	if err != nil {
		ws.session.Say("ui.layers.reorder_busy")
		w.Refresh()
		return false
	}
	if err = tx.SetLayerOrder(names); err != nil {
		tx.Abort()
		ws.session.Say("ui.layers.reorder_invalid")
		w.Refresh()
		return false
	}
	cs := tx.Commit()
	if cs.LayerOrder {
		d.NameLastStep("LAYERORDER")
	}
	if highlight != "" {
		matches := layerMatches(d, ws.layerFilter().Text())
		state.hi = slices.IndexFunc(matches, func(l *doc.Layer) bool { return l.Name == highlight })
		if state.hi < 0 {
			state.hi = 0
		}
		state.list.ScrollToItem(state.hi)
	}
	w.Refresh()
	return cs.LayerOrder
}
func (ws *Workspace) layerOrderButtons(prefix string, d *doc.Drawing, l *doc.Layer) *gpui.Div {
	group := gpui.NewDiv().FlexRow().ItemsCenter().FlexShrink(0)
	rank := d.LayerRank(l.Name)
	for _, delta := range []int{-1, 1} {
		suffix, key, icon := "up-", "ui.layers.move_up_tip", icons.AppChevronUp
		if delta > 0 {
			suffix, key, icon = "down-", "ui.layers.move_down_tip", icons.AppChevronDown
		}
		enabled := rank+delta >= 0 && rank+delta < len(d.Layers()) && !ws.layerOrderBusy()
		color := uiMuted
		if enabled {
			color = uiText
		}
		tip := i18n.T(key)
		if ws.layerOrderBusy() {
			tip = i18n.T("ui.layers.reorder_busy")
		}
		button := gpui.NewDiv().ID(prefix+suffix+l.Name).SizeWH(16, 20).FlexShrink(0).FlexRow().ItemsCenter().JustifyCenter().Rounded(3).Tooltip(tip).Child(NewIcon(icon, 12, color)).OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			w.StopPropagation()
			ws.moveLayerOrder(l.Name, delta, w)
		})
		if enabled {
			button.Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) })
		}
		group.Child(button)
	}
	return group
}
