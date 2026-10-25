package ui

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf16"

	"cad-lite/assets/icons"
	"cad-lite/cmdsys/props"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// The Properties panel is the side panel's editable property grid: cmdsys/props builds the sheet
// (the selection, a layer, or the defaults when nothing is selected) and applies each edit as one
// undo step; this file lays the sheet out as collapsible categories of label | value rows with one
// shared text field, and propsmenu.go holds the drop-down lists.

// Properties panel metrics, in logical pixels.
const (
	propRowH     = 22
	propFont     = 12
	propLabelMin = 64
	propLabelMax = 140
	propPadX     = 8
	propCtlGap   = 6
)

// propsPanel is one tab's Properties panel state: what the sheet shows and the cached sheet.
type propsPanel struct {
	// layer is the layer a click on its row in the Layers section targets ("" for none), and
	// layerGen the selection generation at that click: a selection change switches back.
	layer    string
	layerGen int
	// sel is the selection the class filter and the polyline vertex were chosen for; a new one
	// resets both. selGen counts its changes; selFrame is the frame it was last compared in.
	sel      []doc.Handle
	selGen   int
	selFrame int
	class    string
	vertex   int
	// folded holds the folded categories, by Category.ID.
	folded map[string]bool
	// rev counts the changes that can alter a sheet: document changes, variable changes and
	// Modified moving (direct layer edits); mods is the Modified count last seen.
	rev, mods int
	cache     propsCache
}

// propsCache is the last sheet and what it was built from.
type propsCache struct {
	valid  bool
	d      *doc.Drawing
	target props.Target
	selGen int
	rev    int
	lang   i18n.Lang
	active string
	sheet  props.Sheet
}

// propsEdit is the Properties panel's editing state, shared by the tabs: the text field, the row it
// edits, the error and hint shown, the choice controls' focus handles and positions, the open
// drop-down and the label fitting cache.
type propsEdit struct {
	field *gpui.TextInput
	// tab is the tab the edit or the drop-down belongs to; another active tab drops them.
	tab *docTab
	// id is the row being typed into ("" for none); initial is the text it started with.
	id, initial string
	focusWanted bool
	// errID is the row whose last edit failed, with err its message.
	errID, err string
	// note is the hint under the header (not undoable).
	note string
	// handles focus the choice controls; focusWant asks the next frame to focus one.
	handles   map[string]*gpui.FocusHandle
	focusWant string
	// anchors are the controls' bounds in the last frame, which place the drop-down.
	anchors map[string]gpui.Bounds
	menu    propsMenu
	fits    map[fitKey]string
	// app is the App of the last frame, which makes focus handles.
	app *gpui.App
}

type fitKey struct {
	s    string
	w    int
	bold bool
}

// pedit returns the panel's editing state, made on first use.
func (ws *Workspace) pedit() *propsEdit {
	if ws.pe != nil {
		return ws.pe
	}
	pe := &propsEdit{handles: map[string]*gpui.FocusHandle{}, anchors: map[string]gpui.Bounds{}, fits: map[fitKey]string{}}
	f := gpui.NewTextInput("")
	trackField(f)
	f.Style(func(d *gpui.Div) *gpui.Div {
		b := uiAccent
		if pe.errID != "" && pe.errID == pe.id {
			b = uiErr
		}
		return d.Bg(uiField).Border(1, b).Rounded(4).P(0).Px_(4).Py_(1).TextColor(uiText).TextSize(propFont)
	}).PlaceholderColor(uiMuted).SelectionColor(uiSelect)
	f.OnSubmit(func(_ string, w *gpui.Window, _ *gpui.App) {
		if ws.commitPropEdit(w) {
			ws.focusCommandLine(w)
		}
	})
	f.OnEscape(func(w *gpui.Window, _ *gpui.App) { ws.cancelPropEdit(w) })
	f.OnBlur(func(_ string, w *gpui.Window, _ *gpui.App) { ws.commitPropEdit(w) })
	f.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) bool {
		if e.Keystroke.Key != "tab" || e.Keystroke.Modifiers&^gpui.Shift != 0 {
			return false
		}
		ws.propTab(e.Keystroke.Modifiers&gpui.Shift != 0, w)
		return true
	})
	pe.field = f
	ws.pe = pe
	return pe
}

// propsPanelShown reports whether the side panel shows the Properties section expanded, which
// hides the properties HUD.
func (ws *Workspace) propsPanelShown() bool { return !ws.lay.sideOff && !ws.lay.propsOff }

// focusCommandLine gives the keyboard back to the command line.
func (ws *Workspace) focusCommandLine(w *gpui.Window) {
	if fh := ws.input.FocusHandle(); fh != nil {
		w.RequestFocus(fh)
	}
	w.Refresh()
}

// syncSel compares the session's selection with pp.sel, once per frame while a frame is being
// built (the selection cannot change during one) and on every call otherwise, and moves to a new
// generation when it changed: the class filter and the vertex reset.
func (ws *Workspace) syncSel() {
	pp := &ws.pp
	if ws.building && pp.selFrame == ws.frameN {
		return
	}
	if sel := ws.session.Selection().Handles(); !slices.Equal(sel, pp.sel) {
		pp.sel = slices.Clone(sel)
		pp.selGen++
		pp.class, pp.vertex = "", 0
	}
	if ws.building {
		pp.selFrame = ws.frameN
	}
}

// propsTarget works out what the sheet shows: the targeted layer while the selection has not
// changed since its row was clicked, else the selection, else the defaults.
func (ws *Workspace) propsTarget() props.Target {
	pp := &ws.pp
	ws.syncSel()
	if pp.layer != "" && (pp.layerGen != pp.selGen || ws.session.Drawing().Layer(pp.layer) == nil) {
		pp.layer = ""
	}
	switch {
	case pp.layer != "":
		return props.Target{Kind: props.TargetLayer, Layer: pp.layer}
	case len(pp.sel) > 0:
		return props.Target{Kind: props.TargetSelection, Handles: pp.sel, Class: pp.class, Vertex: pp.vertex}
	}
	return props.Target{Kind: props.TargetDefaults}
}

// sameTarget reports whether a and b are the same target, their handles being the selection of
// generations ga and gb (pp.sel is replaced, never changed in place, so equal generations mean
// equal handles).
func sameTarget(a, b props.Target, ga, gb int) bool {
	return a.Kind == b.Kind && a.Layer == b.Layer && a.Class == b.Class && a.Vertex == b.Vertex && ga == gb
}

// propsSheet returns the active tab's sheet, built again only when the target, the drawing, the
// variables, the language or the running command changed since the last call: the panel asks for
// it every frame.
func (ws *Workspace) propsSheet() *props.Sheet {
	pp := &ws.pp
	s := ws.session
	if m := s.Modified(); m != pp.mods {
		pp.mods = m
		pp.rev++
	}
	tg := ws.propsTarget()
	d := s.Drawing()
	c := &pp.cache
	if c.valid && c.d == d && c.rev == pp.rev && c.lang == i18n.Current() && c.active == s.Active() && sameTarget(c.target, tg, c.selGen, pp.selGen) {
		return &c.sheet
	}
	sh := props.Build(s, tg)
	if tg.Kind == props.TargetSelection && tg.Class != "" && !slices.ContainsFunc(sh.Types, func(t props.TypeCount) bool { return t.Class == tg.Class }) {
		pp.class, tg.Class = "", ""
		sh = props.Build(s, tg)
	}
	if tg.Kind == props.TargetSelection {
		pp.vertex = sh.Target.Vertex
		tg.Vertex = pp.vertex
	}
	*c = propsCache{valid: true, d: d, target: tg, selGen: pp.selGen, rev: pp.rev, lang: i18n.Current(), active: s.Active(), sheet: sh}
	return &c.sheet
}

// targetLayer shows layer l's properties in the panel: a click on its row in the Layers section.
func (ws *Workspace) targetLayer(l *doc.Layer, w *gpui.Window) {
	ws.commitPropEdit(w)
	ws.closePropMenu(w, false)
	pp := &ws.pp
	ws.syncSel()
	pp.layer, pp.layerGen = l.Name, pp.selGen
	w.Refresh()
}

// clearLayerTarget drops the layer target (Escape, the × beside its title); it reports whether
// there was one.
func (ws *Workspace) clearLayerTarget(w *gpui.Window) bool {
	if ws.pp.layer == "" {
		return false
	}
	ws.cancelPropEdit(nil)
	ws.closePropMenu(nil, false)
	ws.pp.layer = ""
	if w != nil {
		w.Refresh()
	}
	return true
}

// errText is an edit error in the current language.
func errText(err error) string {
	var pe *props.Error
	if errors.As(err, &pe) {
		return pe.Text()
	}
	return err.Error()
}

// applyProp applies v to row p of the current sheet: a selector row moves the vertex shown, any
// other goes through props.Apply as one undo step. A failure is shown on the row.
func (ws *Workspace) applyProp(p props.Property, v props.Value, w *gpui.Window) bool {
	pe := ws.pedit()
	pp := &ws.pp
	pe.note = ""
	if p.Selector {
		pp.vertex = v.Int
		pe.errID, pe.err = "", ""
		refresh(w)
		return true
	}
	r, err := props.Apply(ws.session, ws.propsSheet().Target, p.ID, v)
	if err != nil {
		pe.errID, pe.err = p.ID, errText(err)
		refresh(w)
		return false
	}
	pe.errID, pe.err = "", ""
	if r.NotUndoable {
		pe.note = i18n.T("ui.props.not_undoable_tip")
	}
	ws.builder.SyncLayers()
	refresh(w)
	return true
}

// textKind reports whether a row of kind k is edited by typing (otherwise by a drop-down).
func textKind(k props.Kind) bool {
	switch k {
	case props.KindNumber, props.KindInt, props.KindLength, props.KindAngle, props.KindText, props.KindMultiText:
		return true
	}
	return false
}

// rowEditable reports whether row p of sheet sh takes edits.
func rowEditable(sh *props.Sheet, p props.Property) bool { return sh.Editable && !p.ReadOnly }

// startPropEdit opens the text field on row p, holding its current value, all selected.
func (ws *Workspace) startPropEdit(p props.Property, w *gpui.Window) {
	pe := ws.pedit()
	if pe.id == p.ID {
		return
	}
	if !ws.commitPropEdit(w) {
		return
	}
	ws.closePropMenu(w, false)
	text := p.Display
	switch {
	case p.Value.Varies:
		text = ""
	case p.Kind == props.KindText || p.Kind == props.KindMultiText:
		text = p.Value.Str
	}
	pe.tab, pe.id, pe.initial, pe.focusWanted = ws.docTab, p.ID, text, true
	if pe.errID != p.ID {
		pe.errID, pe.err = "", ""
	}
	pe.field.SetText(text)
	pe.field.SetSelectedTextRange(gpui.IntRange{Start: 0, End: len(utf16.Encode([]rune(text)))})
	w.Refresh()
}

// commitPropEdit applies what the field holds to the row it edits, when it changed, in the tab the
// edit was opened in (which is the active one unless a tab switch got past flushPropEdit). It
// reports false when the value is refused: the field stays open with the error on the row. w may
// be nil.
func (ws *Workspace) commitPropEdit(w *gpui.Window) bool {
	pe := ws.pe
	if pe == nil || pe.id == "" {
		return true
	}
	id := pe.id
	if pe.tab != ws.docTab {
		if ws.tabIndex(pe.tab) < 0 {
			pe.id = ""
			return true
		}
		cur := ws.docTab
		ws.docTab = pe.tab
		defer func() { ws.docTab = cur }()
	}
	text := pe.field.Text()
	sh := ws.propsSheet()
	p, ok := sh.Find(id)
	if text == pe.initial || !ok || !rowEditable(sh, p) {
		pe.id = ""
		if pe.errID == id {
			pe.errID, pe.err = "", ""
		}
		return true
	}
	v, err := props.Parse(ws.session, p, text)
	if err != nil {
		pe.errID, pe.err = id, errText(err)
		refresh(w)
		return false
	}
	if !ws.applyProp(p, v, w) {
		return false
	}
	pe.id = ""
	return true
}

// flushPropEdit commits the open field, if any, before something that must see its value or
// would otherwise drop it: a tab switch or close, Save, Save As, Open, New, any command started
// from the UI, quitting. A refused value keeps the field open with its error (a tab switch then
// drops it). Once committed the keyboard goes back to the command line. w may be nil.
func (ws *Workspace) flushPropEdit(w *gpui.Window) {
	pe := ws.pe
	if pe == nil || pe.id == "" {
		return
	}
	if ws.commitPropEdit(w) && w != nil {
		ws.focusCommandLine(w)
	}
}

// refresh asks w, when set, for a new frame.
func refresh(w *gpui.Window) {
	if w != nil {
		w.Refresh()
	}
}

// cancelPropEdit closes the field without applying it and gives the keyboard back to the command
// line (when w is set).
func (ws *Workspace) cancelPropEdit(w *gpui.Window) {
	pe := ws.pe
	if pe == nil {
		return
	}
	if pe.errID == pe.id {
		pe.errID, pe.err = "", ""
	}
	pe.id = ""
	if w != nil {
		ws.focusCommandLine(w)
	}
}

// propTabOrder is the editable rows in display order, folded categories left out.
func (ws *Workspace) propTabOrder(sh *props.Sheet) []props.Property {
	var out []props.Property
	for _, c := range sh.Categories {
		if ws.pp.folded[c.ID] {
			continue
		}
		for _, p := range c.Properties {
			if rowEditable(sh, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// propTab answers Tab (back for Shift+Tab) in the panel: the current edit is committed (a refused
// one keeps the keyboard) and the next editable row gets it, its field open or its control focused.
func (ws *Workspace) propTab(back bool, w *gpui.Window) {
	pe := ws.pedit()
	cur := pe.id
	if cur == "" {
		for id, fh := range pe.handles {
			if fh.IsFocused(w) {
				cur = id
			}
		}
	}
	if !ws.commitPropEdit(w) {
		return
	}
	order := ws.propTabOrder(ws.propsSheet())
	if len(order) == 0 {
		return
	}
	i := slices.IndexFunc(order, func(p props.Property) bool { return p.ID == cur })
	switch {
	case i < 0 && back:
		i = len(order) - 1
	case i < 0:
		i = 0
	case back:
		i = (i - 1 + len(order)) % len(order)
	default:
		i = (i + 1) % len(order)
	}
	ws.focusPropRow(order[i], w)
}

// focusPropRow gives row p the keyboard: its field opens, or its drop-down control is focused.
func (ws *Workspace) focusPropRow(p props.Property, w *gpui.Window) {
	if textKind(p.Kind) {
		ws.startPropEdit(p, w)
		return
	}
	pe := ws.pedit()
	pe.tab, pe.focusWant = ws.docTab, p.ID
	w.Refresh()
}

// validateProps drops an edit or a drop-down the sheet no longer allows: another tab, a command
// started, the row gone.
func (ws *Workspace) validateProps(sh *props.Sheet) {
	pe := ws.pe
	if pe == nil {
		return
	}
	if pe.id != "" {
		if _, ok := sh.Find(pe.id); !ok || pe.tab != ws.docTab || !sh.Editable {
			ws.cancelPropEdit(nil)
		}
	}
	if m := pe.menu.id; m != "" {
		_, ok := sh.Find(m)
		if pe.menu.tab != ws.docTab || !sh.Editable || !ok && m != propTypesMenu {
			ws.closePropMenu(nil, false)
		}
	}
}

// fit returns s cut with an ellipsis to fit maxW at the panel's type size.
func (pe *propsEdit) fit(w *gpui.Window, s string, maxW float32, bold bool) string {
	k := fitKey{s, int(maxW), bold}
	if r, ok := pe.fits[k]; ok {
		return r
	}
	m := w.TextMeasurer()
	if m == nil || maxW <= 0 {
		return s
	}
	ts := gpui.TextStyle{FontSize: propFont, FontWeight: 400}
	if bold {
		ts.FontWeight = 700
	}
	fits := func(t string) bool {
		tw, _ := m.MeasureWrapped(t, ts, 1e9)
		return float32(tw) <= maxW
	}
	out := s
	if !fits(s) {
		r := []rune(s)
		lo, hi := 0, len(r)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if fits(strings.TrimRight(string(r[:mid]), " ") + "…") {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		out = strings.TrimRight(string(r[:lo]), " ") + "…"
	}
	if len(pe.fits) > 4000 {
		clear(pe.fits)
	}
	pe.fits[k] = out
	return out
}

// propLabel is a row's label in the current language.
func propLabel(p props.Property) string {
	if p.Name != "" {
		return p.Name
	}
	return i18n.T(p.Label)
}

// propLabelTip is a row label's tooltip: the whole label, the system variable it edits and the
// row's own tip.
func propLabelTip(p props.Property) string {
	t := propLabel(p)
	if p.Var != "" {
		t += " (" + p.Var + ")"
	}
	if p.Tip != "" {
		t += "\n" + i18n.T(p.Tip)
	}
	return t
}

// propsGrid is the Properties section's body, width wide: the header strip (the object-type
// filter, the layer's title with its ×, or the defaults title), the busy and not-undoable notes,
// then each category's header and, unless folded, its rows.
func (ws *Workspace) propsGrid(width float32, w *gpui.Window, cx *gpui.App) *gpui.Div {
	sh := ws.propsSheet()
	ws.validateProps(sh)
	pe := ws.pedit()
	pe.app = cx
	inner := max(0, width-2*propPadX-4)
	labelW := min(propLabelMax, max(propLabelMin, inner*0.36))
	ctlW := max(0, inner-labelW-propCtlGap)

	col := gpui.NewDiv().FlexCol().Px_(propPadX).Pb(8).TextSize(propFont)
	col.Child(ws.propsHeader(sh, inner, w))
	if !sh.Editable {
		col.Child(propsNote("props-busy", i18n.T("ui.props.busy_tip"), uiWarn))
	}
	if pe.note != "" {
		col.Child(propsNote("props-note", pe.note, uiMuted))
	}
	for i, c := range sh.Categories {
		folded := ws.pp.folded[c.ID]
		col.Child(ws.propsCategoryHeader(c, folded, i == 0, w))
		if folded {
			continue
		}
		for _, p := range c.Properties {
			col.Child(ws.propRow(sh, p, labelW, ctlW, w, cx))
		}
	}
	// The panel's keys stay in it: drawing actions must not run while a field has the keyboard.
	for _, a := range append([]gpui.Action{actionUndo, actionRedo}, commandKeyActions()...) {
		col.OnAction(a, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	// Plain keys (Delete erasing the selection) stop here; Ctrl, Cmd and function-key shortcuts go
	// on to the window's bindings (Save, Ctrl+Tab, F8...), which flush the open field first, while
	// undo, redo and the command keys stop above.
	col.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		if f := w.Focus(); f != nil && ws.input.FocusHandle() != nil && f.ID() != ws.input.FocusHandle().ID() && !shortcutKey(e.Keystroke) {
			w.StopPropagation()
		}
	})
	if pe.focusWant != "" {
		if fh := pe.handles[pe.focusWant]; fh != nil {
			w.RequestFocus(fh)
		}
		pe.focusWant = ""
	}
	return col
}

// shortcutKey reports whether k is a window shortcut rather than typing: a Ctrl or Cmd chord, or a
// function key.
func shortcutKey(k gpui.Keystroke) bool {
	if k.Modifiers&(gpui.Control|gpui.ModPlatform) != 0 {
		return true
	}
	return len(k.Key) >= 2 && len(k.Key) <= 3 && k.Key[0] == 'f' && k.Key[1] >= '1' && k.Key[1] <= '9'
}

// commandKeyActions are the actions of commandKeys.
func commandKeyActions() []gpui.Action {
	var out []gpui.Action
	for _, a := range commandKeys {
		out = append(out, a)
	}
	return out
}

// propsNote is a one-line hint under the panel header.
func propsNote(id, text string, c gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().ID(id).Py_(3).Child(gpui.NewText(text).TextSize(fontSmall - 1).TextColor(c))
}

// propTypesMenu is the drop-down id of the object-type filter.
const propTypesMenu = "#types"

// propsHeader is the strip above the categories: for a selection the object-type filter
// ("All (3)", "Line (2)"), for a layer its title and the × that drops it, for the defaults their
// title.
func (ws *Workspace) propsHeader(sh *props.Sheet, inner float32, w *gpui.Window) *gpui.Div {
	pe := ws.pedit()
	strip := gpui.NewDiv().ID("props-header").FlexRow().ItemsCenter().Gap(6).MinH(propRowH + 4).Pb(2)
	switch sh.Target.Kind {
	case props.TargetLayer:
		strip.Child(gpui.NewText(pe.fit(w, sh.Title, inner-28, true)).FontWeight(700).Flex1().MinW(0),
			propsIconButton("props-layer-close", i18n.T("ui.props.layer_close_tip"), icons.AppCancel, func(w *gpui.Window) { ws.clearLayerTarget(w) }))
	case props.TargetDefaults:
		strip.Child(gpui.NewText(sh.Title).TextColor(uiMuted).Flex1().MinW(0))
	default:
		label := i18n.T("ui.props.filter_all", sh.Count)
		for _, t := range sh.Types {
			if t.Class == sh.Target.Class {
				label = i18n.T("ui.props.filter_class", t.Label, t.Count)
			}
		}
		if sh.Target.Class == "" {
			total := 0
			for _, t := range sh.Types {
				total += t.Count
			}
			label = i18n.T("ui.props.filter_all", total)
			if len(sh.Types) == 1 {
				label = i18n.T("ui.props.filter_class", sh.Types[0].Label, total)
			}
		}
		ctl := ws.choiceControl(propTypesMenu, "props-filter", len(sh.Types) > 1 && sh.Editable, nil, pe.fit(w, label, inner-40, true), false, i18n.T("ui.props.filter_tip"))
		ctl.Flex1().FontWeight(700)
		strip.Child(ctl)
	}
	return strip
}

// propsIconButton is a small icon button of the panel.
func propsIconButton(id, tip string, icon icons.Name, act func(*gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).FlexRow().ItemsCenter().JustifyCenter().SizeWH(18, 18).Rounded(4).FlexShrink(0).
		Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { act(w) }).
		Child(NewIcon(icon, 12, uiMuted))
}

// propsCategoryHeader is a category's title with its disclosure chevron; a click folds it.
func (ws *Workspace) propsCategoryHeader(c props.Category, folded, first bool, w *gpui.Window) *gpui.Div {
	icon, tip := icons.AppChevronDown, i18n.T("ui.props.fold_tip")
	if folded {
		icon, tip = icons.AppChevronRight, i18n.T("ui.props.unfold_tip")
	}
	h := gpui.NewDiv().ID("props-cat-"+c.ID).FlexRow().ItemsCenter().Gap(4).SizeH(propRowH).FlexShrink(0).Px_(2).Rounded(4).
		Bg(uiPanelAlt).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			pp := &ws.pp
			if pp.folded == nil {
				pp.folded = map[string]bool{}
			}
			pp.folded[c.ID] = !pp.folded[c.ID]
			w.Refresh()
		}).
		Child(NewIcon(icon, 12, uiMuted), gpui.NewText(i18n.T(c.Title)).FontWeight(700).TextColor(uiText))
	if !first {
		h.Mt(4)
	}
	return h
}

// propRow is one row: the label (cut with an ellipsis, the whole label and its variable in the
// tooltip) and the value's control, labelW and ctlW wide. An inherited value is muted with a tip
// that says where it comes from; an overridden dimension variable has a marker; a clearable value
// a clear button; a refused edit an error outline with the message as the tip.
func (ws *Workspace) propRow(sh *props.Sheet, p props.Property, labelW, ctlW float32, w *gpui.Window, cx *gpui.App) *gpui.Div {
	pe := ws.pedit()
	editable := rowEditable(sh, p)
	label := gpui.NewDiv().ID("props-label-" + p.ID).SizeW(labelW).FlexShrink(0).OverflowHidden().Tooltip(propLabelTip(p)).
		Child(gpui.NewText(pe.fit(w, propLabel(p), labelW-2, false)).TextColor(uiMuted))
	row := gpui.NewDiv().ID("props-row-" + p.ID).FlexRow().ItemsCenter().Gap(propCtlGap).SizeH(propRowH).FlexShrink(0).Child(label)

	clear := editable && p.Clearable
	room := ctlW
	if clear {
		room -= 18 + 2
	}
	var ctl *gpui.Div
	switch {
	case pe.id == p.ID && pe.tab == ws.docTab:
		field := pe.field.Render(w, cx)
		if pe.focusWanted {
			if fh := pe.field.FocusHandle(); fh != nil {
				pe.focusWanted = false
				w.RequestFocus(fh)
			}
		}
		ctl = gpui.NewDiv().ID("props-edit-" + p.ID).FlexCol().JustifyCenter().Flex1().MinW(0).SizeH(propRowH - 2).Child(field)
	case textKind(p.Kind) || !editable:
		ctl = ws.valueControl(sh, p, editable, room, w)
	default:
		ctl = ws.choiceControl(p.ID, "props-ctl-"+p.ID, true, &p.Sample, pe.fit(w, p.Display, room-sampleWidth(p.Sample, sampleFieldW)-29, false), propMuted(p), valueTip(p))
	}
	if pe.errID == p.ID && pe.err != "" {
		ctl.Border(1, uiErr).Tooltip(pe.err)
	}
	if p.Overridden {
		row.Child(gpui.NewDiv().ID("props-over-"+p.ID).SizeWH(3, 14).Rounded(1).FlexShrink(0).Bg(uiAccent).Ml(-propCtlGap + 1).Mr(-3).
			Tooltip(i18n.T("ui.props.overridden_tip")))
	}
	row.Child(ctl.Flex1().MinW(0))
	if clear {
		row.Child(propsIconButton("props-clear-"+p.ID, i18n.T("ui.props.clear_override")+"\n"+i18n.T("ui.props.clear_override_tip"), icons.AppCancel, func(w *gpui.Window) {
			ws.cancelPropEdit(nil)
			ws.applyProp(p, props.ClearValue(), w)
		}))
	}
	return row
}

// propMuted reports whether row p's value is inherited (drawn muted).
func propMuted(p props.Property) bool { return p.Source != props.SourceOwn && p.Inherited != "" }

// valueTip is a value's tooltip: where an inherited value comes from, else the value itself.
func valueTip(p props.Property) string {
	if propMuted(p) {
		return p.Inherited
	}
	return p.Display
}

// sampleWidth is the room a sample takes beside its text, its strips stripW wide.
func sampleWidth(s props.Sample, stripW float32) float32 {
	switch {
	case s.HasArrow, s.Linetype != "", s.Lineweight != 0:
		return stripW + 5
	case s.ACI > 0:
		return sampleSwatch + 5
	}
	return 0
}

// valueControl is a typed value: a click opens the field on it. Read-only (or while a command
// runs) it is plain text. A multi-line text shows its first line and a … button that opens
// TEXTEDIT on a single text.
func (ws *Workspace) valueControl(sh *props.Sheet, p props.Property, editable bool, room float32, w *gpui.Window) *gpui.Div {
	pe := ws.pedit()
	text := p.Display
	more := p.Kind == props.KindMultiText && editable && !p.Value.Varies && sh.Count == 1
	if p.Kind == props.KindMultiText {
		text, _, _ = strings.Cut(text, "\n")
	}
	tw := room - 12 - sampleWidth(p.Sample, sampleFieldW)
	if more {
		tw -= 20
	}
	col := uiText
	if propMuted(p) || !editable {
		col = uiMuted
	}
	ctl := gpui.NewDiv().ID("props-ctl-"+p.ID).FlexRow().ItemsCenter().Gap(5).SizeH(propRowH-2).Px_(5).Rounded(4).
		Border(1, gpui.RgbaOf(0, 0, 0, 0)).Tooltip(valueTip(p))
	if s := sampleElement("props-sample-"+p.ID, p.Sample, sampleFieldW); s != nil {
		ctl.Child(s)
	}
	ctl.Child(gpui.NewText(pe.fit(w, text, tw, false)).TextColor(col).Flex1().MinW(0))
	if !editable {
		return ctl
	}
	ctl.Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			if p.Kind == props.KindMultiText && strings.Contains(p.Value.Str, "\n") && more {
				ws.openPropText(sh, w)
				return
			}
			ws.startPropEdit(p, w)
		})
	if more {
		ctl.Child(gpui.NewDiv().ID("props-more-"+p.ID).FlexRow().ItemsCenter().JustifyCenter().SizeWH(18, 16).Rounded(4).FlexShrink(0).
			Bg(uiPanelAlt).Tooltip(i18n.T("ui.props.edit_text")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				w.StopPropagation()
				ws.openPropText(sh, w)
			}).
			Child(gpui.NewText("…").TextColor(uiText)))
	}
	return ctl
}

// openPropText opens TEXTEDIT on the sheet's single text.
func (ws *Workspace) openPropText(sh *props.Sheet, w *gpui.Window) {
	if !ws.commitPropEdit(w) {
		return
	}
	d := ws.session.Drawing()
	for _, h := range sh.Target.Handles {
		if e := d.Entity(h); e != nil && (sh.Target.Class == "" || e.Class() == sh.Target.Class) {
			ws.openTextEditor(ws.docTab, h)
			return
		}
	}
}

// choiceControl is a drop-down's closed field, element id elemID, opening drop-down menuID: the
// sample, the text (muted when inherited) and a chevron. It takes the keyboard on Tab: Enter,
// Space or Down opens it, Escape gives the keyboard back to the command line. Its bounds are
// recorded while painting so the drop-down hangs from it.
func (ws *Workspace) choiceControl(menuID, elemID string, enabled bool, sample *props.Sample, text string, muted bool, tip string) *gpui.Div {
	pe := ws.pedit()
	col := uiText
	if muted || !enabled {
		col = uiMuted
	}
	mark := gpui.NewCanvas(func(b gpui.Bounds, _ *gpui.Window, _ *gpui.App) { pe.anchors[menuID] = b })
	mark.Absolute().Inset(0)
	ctl := gpui.NewDiv().ID(elemID).Relative().FlexRow().ItemsCenter().Gap(5).SizeH(propRowH-2).Px_(5).Rounded(4).
		Border(1, gpui.RgbaOf(0, 0, 0, 0)).Tooltip(tip).Child(mark)
	if sample != nil {
		if s := sampleElement(elemID+"-sample", *sample, sampleFieldW); s != nil {
			ctl.Child(s)
		}
	}
	ctl.Child(gpui.NewText(text).TextColor(col).Flex1().MinW(0))
	if !enabled {
		return ctl
	}
	if pe.menu.id == menuID {
		ctl.Bg(uiPanelAlt).Border(1, uiAccent)
	}
	fh := pe.handles[menuID]
	if fh == nil && pe.app != nil {
		fh = pe.app.NewFocusHandle()
		pe.handles[menuID] = fh
	}
	if fh != nil {
		ctl.TrackFocus(fh).Focus(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, uiAccent) })
	}
	return ctl.Child(NewIcon(icons.AppChevronDown, 12, uiMuted)).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button == gpui.MouseLeft {
				ws.togglePropMenu(menuID, w)
			}
		}).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
			switch e.Keystroke.Key {
			case "enter", "space", "down":
				ws.openPropMenu(menuID, w)
			case "escape":
				ws.focusCommandLine(w)
			case "tab":
				ws.propTab(e.Keystroke.Modifiers&gpui.Shift != 0, w)
			default:
				return
			}
			w.StopPropagation()
		})
}
