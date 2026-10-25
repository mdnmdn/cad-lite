package ui

import (
	"fmt"
	"strconv"
	"strings"

	"cad-lite/cmdsys/props"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// The Properties panel's drop-down lists: one at a time, mounted on the root after the chrome
// (gpui paints in tree order) over a transparent backdrop that closes it on a press outside, and
// hung under its control from the bounds the control recorded while painting, or above it when
// the window has no room below.

// Drop-down metrics, in logical pixels.
const (
	propMenuRows   = 12
	propMenuFilter = 10 // more options than this get a filter field
	propMenuMinW   = 190
	aciCell        = 11
	aciGap         = 1
)

// propsMenu is the open drop-down: the row (or propTypesMenu) it edits, the highlighted item, and
// whether it shows the full ACI grid instead of the list.
type propsMenu struct {
	id          string
	tab         *docTab
	hi          int
	aci         bool
	filter      *gpui.TextInput
	focus       *gpui.FocusHandle
	focusWanted bool
	list        *gpui.ScrollHandle
}

// menuItem is one entry of a drop-down.
type menuItem struct {
	label   string
	tip     string
	sample  props.Sample
	current bool
	load    bool
	pick    func(*gpui.Window)
}

// sameValue reports whether an option's value is the row's value (names without regard to case).
func sameValue(a, b props.Value) bool {
	return a.Num == b.Num && a.Int == b.Int && a.Bool == b.Bool && strings.EqualFold(a.Str, b.Str) && !a.Varies && !b.Varies
}

// rowOptions are row p's choices: its Options, or Yes and No for a switch.
func rowOptions(p props.Property) []props.Option {
	if p.Kind == props.KindBool && len(p.Options) == 0 {
		return []props.Option{{Value: props.BoolValue(true), Label: i18n.T("ui.props.yes")}, {Value: props.BoolValue(false), Label: i18n.T("ui.props.no")}}
	}
	return p.Options
}

// propMenuItems returns drop-down id's items, and the row for a property drop-down.
func (ws *Workspace) propMenuItems(id string) ([]menuItem, props.Property, bool) {
	sh := ws.propsSheet()
	if id == propTypesMenu {
		total := 0
		for _, t := range sh.Types {
			total += t.Count
		}
		set := func(class string) func(*gpui.Window) {
			return func(w *gpui.Window) {
				ws.pp.class, ws.pp.vertex = class, 0
				ws.closePropMenu(w, true)
			}
		}
		items := []menuItem{{label: i18n.T("ui.props.filter_all", total), current: sh.Target.Class == "", pick: set("")}}
		for _, t := range sh.Types {
			items = append(items, menuItem{label: i18n.T("ui.props.filter_class", t.Label, t.Count), current: sh.Target.Class == t.Class, pick: set(t.Class)})
		}
		return items, props.Property{}, true
	}
	p, ok := sh.Find(id)
	if !ok {
		return nil, p, false
	}
	var items []menuItem
	for _, o := range rowOptions(p) {
		tip := o.Label
		if o.Load {
			tip = i18n.T("ui.props.load_linetype")
		}
		items = append(items, menuItem{label: o.Label, tip: tip, sample: o.Sample, load: o.Load, current: sameValue(o.Value, p.Value),
			pick: func(w *gpui.Window) {
				ws.closePropMenu(w, false)
				if ws.applyProp(p, o.Value, w) {
					ws.focusCommandLine(w)
				}
			}})
	}
	return items, p, true
}

// openPropMenu opens drop-down id with its current item highlighted, the keyboard in it.
func (ws *Workspace) openPropMenu(id string, w *gpui.Window) {
	if !ws.commitPropEdit(w) {
		return
	}
	pe := ws.pedit()
	m := &pe.menu
	if m.list == nil {
		m.list = gpui.NewScrollHandle()
		m.filter = themeBareField(gpui.NewTextInput(i18n.T("ui.props.filter_placeholder")), propFont, false)
		m.filter.OnChange(func(_ string, w *gpui.Window, _ *gpui.App) {
			m.hi = 0
			m.list.ScrollToTop()
			w.Refresh()
		})
		m.filter.OnSubmit(func(_ string, w *gpui.Window, _ *gpui.App) { ws.pickPropMenu(w) })
		m.filter.OnEscape(func(w *gpui.Window, _ *gpui.App) { ws.closePropMenu(w, true) })
	}
	items, _, ok := ws.propMenuItems(id)
	if !ok {
		return
	}
	m.id, m.tab, m.aci, m.hi, m.focusWanted = id, ws.docTab, false, 0, true
	m.filter.SetText("")
	for i, it := range items {
		if it.current {
			m.hi = i
		}
	}
	m.list.ScrollToItem(m.hi)
	w.Refresh()
}

// togglePropMenu opens drop-down id, or closes it when it is open.
func (ws *Workspace) togglePropMenu(id string, w *gpui.Window) {
	if ws.pe != nil && ws.pe.menu.id == id {
		ws.closePropMenu(w, true)
		return
	}
	ws.openPropMenu(id, w)
}

// closePropMenu closes the drop-down; with focusCmd the keyboard goes back to the command line.
func (ws *Workspace) closePropMenu(w *gpui.Window, focusCmd bool) {
	if ws.pe == nil || ws.pe.menu.id == "" {
		return
	}
	ws.pe.menu.id = ""
	if w == nil {
		return
	}
	if focusCmd {
		ws.focusCommandLine(w)
	} else {
		w.Refresh()
	}
}

// propMenuMatches returns the items whose label contains the filter, ignoring case.
func (m *propsMenu) matches(items []menuItem) []menuItem {
	if m.filter == nil || len(items) <= propMenuFilter {
		return items
	}
	f := strings.ToLower(strings.TrimSpace(m.filter.Text()))
	if f == "" {
		return items
	}
	var out []menuItem
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.label), f) {
			out = append(out, it)
		}
	}
	return out
}

// pickPropMenu picks the highlighted item (Enter).
func (ws *Workspace) pickPropMenu(w *gpui.Window) {
	m := &ws.pedit().menu
	items, _, ok := ws.propMenuItems(m.id)
	if !ok {
		return
	}
	if l := m.matches(items); m.hi >= 0 && m.hi < len(l) {
		l[m.hi].pick(w)
	}
}

// moveMenuHighlight moves the highlight by delta, stopping at either end.
func (ws *Workspace) moveMenuHighlight(delta int, w *gpui.Window) {
	m := &ws.pedit().menu
	items, _, _ := ws.propMenuItems(m.id)
	n := len(m.matches(items))
	if n == 0 {
		return
	}
	m.hi = min(max(m.hi+delta, 0), n-1)
	m.list.ScrollToItem(m.hi)
	w.Refresh()
}

// propsMenuOverlay is the open drop-down over the window with its backdrop, or an empty box.
func (ws *Workspace) propsMenuOverlay(w *gpui.Window, cx *gpui.App) *gpui.Div {
	pe := ws.pe
	if pe == nil || pe.menu.id == "" || pe.menu.tab != ws.docTab {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	m := &pe.menu
	items, p, ok := ws.propMenuItems(m.id)
	if !ok {
		m.id = ""
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	a := pe.anchors[m.id]
	winW, winH := float32(w.Bounds().Size.W), float32(w.Bounds().Size.H)
	menu := gpui.NewDiv().ID("props-menu").FlexCol().Gap(2).P(4).Rounded(6).Bg(uiPanelAlt).Border(1, uiBorder).TextSize(propFont)
	var width, height float32
	if m.aci {
		width, height = ws.aciGrid(menu, p)
	} else {
		width, height = ws.menuList(menu, m, items, p, float32(a.Size.W), w, cx)
	}
	width = min(width, winW-16)
	left := max(8, min(float32(a.Origin.X), winW-width-8))
	top := float32(a.Origin.Y+a.Size.H) + 2
	if top+height > winH-8 {
		top = max(8, float32(a.Origin.Y)-2-height)
	}
	focus := m.focus
	if m.filter != nil && len(items) > propMenuFilter && !m.aci {
		focus = m.filter.FocusHandle()
	}
	if m.focus == nil {
		m.focus = cx.NewFocusHandle()
		focus = m.focus
	}
	if m.focusWanted && focus != nil {
		m.focusWanted = false
		w.RequestFocus(focus)
	}
	menu.Absolute().Left(left).Top(top).SizeW(width).TrackFocus(m.focus).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
			switch e.Keystroke.Key {
			case "up":
				ws.moveMenuHighlight(-1, w)
			case "down":
				ws.moveMenuHighlight(1, w)
			case "enter":
				if !m.aci {
					ws.pickPropMenu(w)
				}
			case "escape", "tab":
				ws.closePropMenu(w, true)
			}
			w.StopPropagation()
		})
	for _, act := range append([]gpui.Action{actionUndo, actionRedo}, commandKeyActions()...) {
		menu.OnAction(act, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	return gpui.NewDiv().ID("props-menu-backdrop").Absolute().Inset(0).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			w.StopPropagation()
			ws.closePropMenu(w, true)
		}).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(menu)
}

// menuList fills menu with the filter field (above propMenuFilter items), the matching items and,
// for a colour, the More colors item; it returns the menu's width and height.
func (ws *Workspace) menuList(menu *gpui.Div, m *propsMenu, items []menuItem, p props.Property, anchorW float32, w *gpui.Window, cx *gpui.App) (float32, float32) {
	pe := ws.pe
	width := max(anchorW, propMenuMinW)
	height := float32(8)
	if len(items) > propMenuFilter {
		menu.Child(gpui.NewDiv().ID("props-menu-filter").FlexCol().JustifyCenter().SizeH(24).FlexShrink(0).Px_(6).Rounded(4).Bg(uiField).Border(1, uiBorder).
			Child(m.filter.Render(w, cx)))
		height += 24 + 2
	}
	shown := m.matches(items)
	m.hi = min(m.hi, max(len(shown)-1, 0))
	sw := float32(0)
	for _, it := range items {
		sw = max(sw, sampleWidth(it.sample, sampleListW))
	}
	rows := min(max(len(shown), 1), propMenuRows)
	list := gpui.NewDiv().ID("props-menu-list").FlexCol().OverflowYScroll().TrackScroll(m.list).SizeH(float32(rows) * propRowH).FlexShrink(0)
	for i, it := range shown {
		idx := indexOfItem(items, it)
		row := gpui.NewDiv().ID("props-opt-" + strconv.Itoa(idx)).FlexRow().ItemsCenter().Gap(0).SizeH(propRowH).FlexShrink(0).Px_(6).Rounded(4).
			Tooltip(it.tip).
			Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(uiBorder) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { it.pick(w) })
		if i == m.hi {
			row.Bg(activeFill())
		}
		if sw > 0 {
			slot := gpui.NewDiv().FlexRow().ItemsCenter().SizeW(sw).FlexShrink(0)
			if s := sampleElement("props-opt-"+strconv.Itoa(idx)+"-sample", it.sample, sampleListW); s != nil {
				slot.Child(s)
			}
			row.Child(slot)
		}
		col := uiText
		if it.load {
			col = uiMuted
		}
		t := gpui.NewText(pe.fit(w, it.label, width-sw-30, it.current)).TextColor(col).Flex1().MinW(0)
		if it.current {
			t.FontWeight(700).TextColor(uiOn)
		}
		row.Child(t)
		list.Child(row)
	}
	if len(shown) == 0 {
		list.Child(gpui.NewText(i18n.T("ui.layers.no_match", m.filter.Text())).TextColor(uiMuted).Px_(6).Py_(4))
	}
	menu.Child(list)
	height += float32(rows) * propRowH
	if p.Kind == props.KindColor {
		menu.Child(gpui.NewDiv().SizeH(1).FlexShrink(0).Bg(uiBorder),
			menuRow("props-more-colors", i18n.T("ui.props.more_colors_tip"), func(w *gpui.Window) {
				m.aci = true
				w.Refresh()
			}).SizeH(propRowH).Py_(0).Px_(6).Gap(0).FlexShrink(0).Child(gpui.NewDiv().SizeW(sw).FlexShrink(0), gpui.NewText(i18n.T("ui.props.more_colors")).TextColor(uiText)))
		height += propRowH + 5
	}
	return width, height
}

// indexOfItem is it's index among items, by label (labels are unique within a drop-down).
func indexOfItem(items []menuItem, it menuItem) int {
	for i := range items {
		if items[i].label == it.label {
			return i
		}
	}
	return -1
}

// aciGrid fills menu with the full ACI palette as AutoCAD's Select Color index tab lays it out:
// 1–9, then 10–249 in 24 columns of ten shades, then the greys 250–255. A click applies the index.
// It returns the menu's width and height.
func (ws *Workspace) aciGrid(menu *gpui.Div, p props.Property) (float32, float32) {
	cur := -1
	if !p.Value.Varies {
		cur = p.Value.Int
	}
	cell := func(n int) *gpui.Div {
		c := gpui.NewDiv().ID(fmt.Sprintf("props-aci-%d", n)).SizeWH(aciCell, aciCell).FlexShrink(0).Bg(sampleColor(n)).Border(1, uiBorder).
			Tooltip(i18n.T("ui.props.aci.n", n)).
			Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(1, uiText) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				ws.closePropMenu(w, false)
				if ws.applyProp(p, props.IntValue(n), w) {
					ws.focusCommandLine(w)
				}
			})
		if n == cur {
			c.Border(2, uiAccent)
		}
		return c
	}
	row := func(from, to int) *gpui.Div {
		r := gpui.NewDiv().FlexRow().Gap(aciGap).FlexShrink(0)
		for n := from; n <= to; n++ {
			r.Child(cell(n))
		}
		return r
	}
	grid := gpui.NewDiv().FlexCol().Gap(aciGap).FlexShrink(0)
	for r := 0; r < 10; r++ {
		line := gpui.NewDiv().FlexRow().Gap(aciGap)
		for c := 0; c < 24; c++ {
			line.Child(cell(10 + c*10 + r))
		}
		grid.Child(line)
	}
	menu.Gap(6).P(8).Child(menuHeading(i18n.T("ui.props.more_colors_title")).Px_(0).Pt(0).Pb(0), row(1, 9), grid, row(250, 255))
	const step = aciCell + aciGap
	return 24*step - aciGap + 18, 16 + 14 + 6 + step + 6 + 10*step + 6 + step
}
