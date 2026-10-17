package ui

import (
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// Settings panel geometry in logical pixels.
const (
	settingsWidth = 580
	// settingsMinH is the card's height unless a tab needs more or the window
	// has less; the card's top stays where a card this tall is centred, so the
	// tab strip does not move when a tab switch changes the height.
	settingsMinH    = 600
	settingsMargin  = 24
	settingsRadius  = 10
	themeCardWidth  = 118
	themePreviewH   = 62
	settingsRowGap  = 10
	settingsLabelW  = 130
	settingsBtnSize = 28
)

// settingsTabs are the panel's tabs in strip order: the element id suffix and
// the catalog keys of the label and the tooltip.
var settingsTabs = [settingsTabCount]struct{ id, label, tip string }{
	settingsGeneral: {"general", "ui.settings.tab_general", "ui.settings.tab_general_tip"},
	settingsAI:      {"ai", "ui.settings.tab_ai", "ui.settings.tab_ai_tip"},
	settingsUpdates: {"updates", "ui.settings.tab_updates", "ui.settings.tab_updates_tip"},
}

// settingsButton is the title bar's gear button that opens the panel. While
// a release waits it carries the update badge and opens the Updates tab.
func (ws *Workspace) settingsButton() *gpui.Div {
	_, waiting := ws.UpdateAvailable()
	b := iconButton("tool-settings", ws.settingsTip(), icons.AppSettings, ws.prefs().open, func(w *gpui.Window) {
		if !ws.prefs().open && waiting {
			ws.openSettingsAt(w, settingsUpdates)
			return
		}
		ws.ToggleSettings(w)
	})
	if waiting {
		b.Relative().Child(ws.updateBadge())
	}
	return b
}

// settingsOverlay is the modal layer: nothing when the panel is closed, else
// a dimmed backdrop over the whole window with the panel on it, centred
// across and, vertically, where a settingsMinH card would be centred.
//
// Keep the translucent backdrop outside the opaque card to avoid redundant blending.
func (ws *Workspace) settingsOverlay(cx *gpui.App) gpui.IntoElement {
	p := ws.prefs()
	if !p.open {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	w := ws.win
	if p.focus == nil {
		p.focus = cx.NewFocusHandle()
	}
	if p.focusWanted {
		p.focusWanted = false
		w.RequestFocus(p.focus)
	}
	winH := float32(w.Bounds().Size.H)
	maxW := float32(w.Bounds().Size.W) - 2*settingsMargin
	top := max(settingsMargin, (winH-settingsMinH)/2)
	maxH := winH - top - settingsMargin

	// The title bar, the tabs and the footer keep their size; only the body
	// scrolls, so Close and Reset stay reachable however short the window is.
	// Only the active tab is built.
	body := gpui.NewDiv().ID("settings-body").FlexCol().Gap(18).Px_(20).Pt(16).Pb(16).Flex1().MinH(0).OverflowYScroll().ShowScrollbar().
		TrackScroll(p.bodyScroll)
	switch p.tab {
	case settingsAI:
		body.Child(ws.basicAISection(w, cx))
		if p.cur.AgentMode == settings.AgentAdvanced {
			body.Child(gpui.NewDiv().ID("settings-ai-advanced").FlexCol().Gap(14).P(12).Rounded(8).Border(1, uiBorder).Child(
				sectionTitle(i18n.T("ui.settings.ai_advanced")), ws.agentSection(), ws.discoverySection()))
		}
	case settingsUpdates:
		body.Child(ws.updateSection())
	default:
		body.Child(ws.appearanceSection(), ws.drawingSection())
	}

	card := gpui.NewDiv().ID("settings-card").TrackFocus(p.focus).FlexCol().
		SizeW(settingsWidth).MaxW(maxW).MinH(min(settingsMinH, maxH)).MaxH(maxH).
		Bg(uiPanel).Border(1, uiBorder).Rounded(settingsRadius).TextColor(uiText).TextSize(13).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) { ws.settingsKey(e, w) }).
		Child(ws.settingsHeader(), ws.settingsTabStrip(), body, ws.settingsFooter())

	dim := func() *gpui.Div { return gpui.NewDiv().Bg(uiBackdrop) }
	// The card's rounded corners leave its bounding box uncovered there;
	// these squares under it dim those few pixels.
	corner := func() *gpui.Div {
		return gpui.NewDiv().Absolute().SizeWH(settingsRadius, settingsRadius).Bg(uiBackdrop)
	}
	framed := gpui.NewDiv().Relative().FlexCol().MaxW(maxW).Child(
		corner().Left(0).Top(0), corner().Right(0).Top(0), corner().Left(0).Bottom(0), corner().Right(0).Bottom(0),
		card)
	return gpui.NewDiv().ID("settings-overlay").Absolute().Inset(0).FlexCol().
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.CloseSettings(w) }).
		OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) { p.scale.Move(e.Position, w) }).
		OnMouseUp(func(e *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { p.scale.End(w) }).
		OnScrollWheel(func(e *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { ws.scrollSettings(e, w) }).
		Child(
			dim().FlexShrink(0).SizeH(top),
			gpui.NewDiv().FlexRow().FlexShrink(0).Child(dim().Flex1(), framed, dim().Flex1()),
			dim().Flex1(),
		)
}

// settingsKey handles the keys the panel owns: Escape closes it, Ctrl+Tab and
// Ctrl+Shift+Tab switch tabs (instead of drawings, while it is open).
func (ws *Workspace) settingsKey(e *gpui.KeyDownEvent, w *gpui.Window) {
	k := e.Keystroke
	switch {
	case k.Key == "escape":
		ws.CloseSettings(w)
	case k.Key == "tab" && k.Modifiers&gpui.Control != 0:
		d := settingsTab(1)
		if k.Modifiers&gpui.Shift != 0 {
			d = -1
		}
		ws.setSettingsTab(w, ws.prefs().tab+d)
	default:
		return
	}
	w.StopPropagation()
}

// settingsTabStrip is the row of tabs under the title bar, styled like the
// document tabs: the active one takes the card's colour and joins the body.
func (ws *Workspace) settingsTabStrip() *gpui.Div {
	cur := ws.prefs().tab
	row := gpui.NewDiv().ID("settings-tabs").FlexRow().ItemsEnd().Gap(2).Px_(16).SizeH(docTabHeight).FlexShrink(0).
		Bg(uiBg)
	keys := i18n.T("ui.settings.tab_keys_tip")
	for i, t := range settingsTabs {
		tab := settingsTab(i)
		tip := i18n.T(t.tip) + "\n" + keys
		label := gpui.NewText(i18n.T(t.label)).TextSize(fontUI)
		chip := gpui.NewDiv().ID("settings-tab-"+t.id).FlexRow().ItemsCenter().Gap(6).Px_(14).SizeH(docTabHeight-4).
			RoundedAll(7, 7, 0, 0).Border(1, uiBorder).Tooltip(tip).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.setSettingsTab(w, tab) })
		if tab == cur {
			chip.Bg(uiPanel)
			label.FontWeight(700)
		} else {
			chip.Bg(uiBg).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) })
			label.TextColor(uiMuted)
		}
		chip.Child(label)
		if rel, ok := ws.UpdateAvailable(); ok && tab == settingsUpdates {
			chip.Tooltip(tip + "\n" + i18n.T("ui.update.badge_tip", rel.Version)).Child(dot(7, uiWarn))
		}
		row.Child(chip)
	}
	return row
}

// scrollSettings scrolls the panel's body by a wheel event anywhere over the
// overlay and always consumes it: the window runs every OnScrollWheel handler
// before the scroll containers, so a handler that only stopped propagation
// would freeze the body, and one that let it through would zoom the canvas.
func (ws *Workspace) scrollSettings(e *gpui.ScrollWheelEvent, w *gpui.Window) {
	h := ws.prefs().bodyScroll
	off := h.Offset()
	off.Y = min(max(off.Y+e.DeltaY, 0), h.MaxOffset().H)
	if off != h.Offset() {
		h.SetOffset(off)
		w.Refresh()
	}
	w.StopPropagation()
}

func (ws *Workspace) settingsHeader() *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().Px_(20).Py_(14).FlexShrink(0).
		Child(
			NewIcon(icons.AppSettings, 18, uiOn),
			gpui.NewDiv().Ml(8).Child(gpui.NewText(i18n.T("ui.settings.title")).FontWeight(700).TextSize(16)),
			gpui.NewDiv().Flex1(),
			gpui.NewDiv().ID("settings-close").Px_(10).Py_(4).Rounded(4).Bg(uiPanelAlt).Tooltip(i18n.T("ui.settings.close_tip")).
				Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
				OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.CloseSettings(w) }).
				Child(gpui.NewText(i18n.T("ui.settings.close")).TextSize(12)),
		)
}

func sectionTitle(s string) *gpui.Text {
	return gpui.NewText(s).FontWeight(700).TextSize(11).TextColor(uiMuted)
}

// settingRow is a label on the left and a control on the right.
// tip, when not empty, is the tooltip of the label.
func settingRow(label, tip string, control gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(12).
		Child(gpui.NewDiv().SizeW(settingsLabelW).FlexShrink(0).Tooltip(tip).Child(gpui.NewText(label)), control)
}

func (ws *Workspace) appearanceSection() *gpui.Div {
	p := ws.prefs()
	cards := gpui.NewDiv().FlexRow().FlexWrap().Gap(settingsRowGap)
	for _, t := range Themes() {
		cards.Child(ws.themeCard(t))
	}

	scale := p.cur.UIScale
	if p.scale.Dragging() {
		scale = p.scale.value
	}
	step := func(id, label, tip string, d float64) *gpui.Div {
		return ws.smallButton(id, label, tip, func(w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) { s.UIScale = settings.ClampUIScale(s.UIScale + d) })
		})
	}
	slider := NewSlider("ui-scale", &p.scale, SliderOptions{
		Value: scale, Min: settings.MinUIScale, Max: settings.MaxUIScale, Step: settings.UIScaleStep,
		OnCommit: func(v float64, w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) { s.UIScale = v })
		},
		OnChange: func(v float64, w *gpui.Window) {
			if !p.scale.Dragging() {
				ws.change(w, func(s *settings.Settings) { s.UIScale = v })
			}
		},
	}).Flex1().MinW(0).Tooltip(i18n.T("ui.settings.ui_scale_tip"))
	scaleRow := gpui.NewDiv().FlexCol().Gap(4).Child(
		gpui.NewDiv().FlexRow().ItemsCenter().Child(
			gpui.NewDiv().Flex1().Tooltip(i18n.T("ui.settings.ui_scale_tip")).Child(gpui.NewText(i18n.T("ui.settings.ui_scale"))),
			gpui.NewText(i18n.T("ui.settings.ui_scale_value", int(scale*100+0.5))).FontWeight(700),
		),
		gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Child(
			step("ui-scale-down", "\u2212", i18n.T("ui.settings.ui_scale_down_tip"), -settings.UIScaleStep),
			slider,
			step("ui-scale-up", "+", i18n.T("ui.settings.ui_scale_up_tip"), settings.UIScaleStep),
			ws.smallButton("ui-scale-reset", "100%", i18n.T("ui.settings.ui_scale_reset_tip"), func(w *gpui.Window) {
				ws.change(w, func(s *settings.Settings) { s.UIScale = 1 })
			}).Px_(8),
		),
	)
	langTip := i18n.T("ui.settings.language_tip")
	lang := settingRow(i18n.T("ui.settings.language"), langTip, segmented("lang-", [][2]string{
		{settings.LangAuto, i18n.T("ui.settings.language_auto")}, {settings.LangEnglish, i18n.T("ui.settings.language_en")}, {settings.LangItalian, i18n.T("ui.settings.language_it")},
	}, langTip, p.cur.Language, func(v string, w *gpui.Window) {
		ws.change(w, func(s *settings.Settings) { s.Language = v })
	}).Flex1())
	hint := gpui.NewText(i18n.T("ui.settings.language_hint")).TextSize(11).TextColor(uiMuted)
	return gpui.NewDiv().ID("settings-sec-appearance").FlexCol().Gap(settingsRowGap).
		Child(sectionTitle(i18n.T("ui.settings.section_appearance")), cards, scaleRow,
			sectionTitle(i18n.T("ui.settings.section_language")), lang, hint)
}

// themeCard is a clickable swatch: a miniature of the theme's window with its
// own colours, and its name below.
func (ws *Workspace) themeCard(t Theme) *gpui.Div {
	selected := ws.prefs().cur.Theme == t.Name
	border := uiBorder
	if selected {
		border = uiAccent
	}
	line := func(x, y, w, h float32, c gpui.Rgba) *gpui.Div {
		return gpui.NewDiv().Absolute().Left(x).Top(y).SizeWH(w, h).Bg(c)
	}
	cv := gpuiColor(t.Canvas)
	aci7 := gpui.RgbaOf(255, 255, 255, 255)
	if !t.Dark() {
		aci7 = gpui.RgbaOf(0, 0, 0, 255)
	}
	canvas := gpui.NewDiv().Flex1().MinW(0).Relative().Bg(cv).
		Child(
			line(8, 14, 46, 2, aci7),
			line(8, 14, 2, 26, aci7),
			line(14, 36, 40, 2, gpuiColor(t.Selected)),
			line(30, 8, 1, 46, gpuiColor(t.Crosshair)),
			gpui.NewDiv().Absolute().Left(26).Top(10).SizeWH(8, 8).Border(1, gpuiColor(t.Snap)),
		)
	preview := gpui.NewDiv().FlexCol().SizeWH(themeCardWidth-4, themePreviewH).Rounded(4).OverflowHidden().Bg(t.Bg).
		Child(
			gpui.NewDiv().FlexRow().ItemsCenter().Gap(3).Px_(4).SizeH(10).Bg(t.Panel).FlexShrink(0).Child(
				gpui.NewDiv().SizeWH(14, 4).Rounded(1).Bg(t.On),
				gpui.NewDiv().SizeWH(8, 4).Rounded(1).Bg(t.Muted),
				gpui.NewDiv().SizeWH(8, 4).Rounded(1).Bg(t.Muted),
				gpui.NewDiv().SizeWH(12, 4).Rounded(1).Bg(t.Accent),
			),
			gpui.NewDiv().FlexRow().Flex1().MinH(0).Child(
				canvas,
				gpui.NewDiv().FlexCol().Gap(3).P(3).SizeW(26).Bg(t.Panel).Child(
					gpui.NewDiv().SizeH(3).Rounded(1).Bg(t.Text),
					gpui.NewDiv().SizeH(3).Rounded(1).Bg(t.Muted),
					gpui.NewDiv().SizeH(3).Rounded(1).Bg(t.Muted),
				),
			),
		)
	name := i18n.T("ui.settings.theme." + strings.ReplaceAll(t.Name, "-", "_"))
	label := gpui.NewText(name).FontWeight(700).TextSize(12)
	if selected {
		label.TextColor(uiOn)
	}
	return gpui.NewDiv().ID("theme-"+t.Name).FlexCol().Gap(6).P(2).Rounded(8).Border(2, border).
		Bg(uiPanelAlt).SizeW(themeCardWidth+4).Tooltip(i18n.T("ui.settings.theme_tip", name)).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(2, uiOn) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			ws.change(w, func(s *settings.Settings) { s.Theme = t.Name })
		}).
		Child(preview, gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Px_(4).Pb(4).Child(label))
}

func (ws *Workspace) smallButton(id, label, tip string, f func(*gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).Tooltip(tip).FlexRow().ItemsCenter().JustifyCenter().SizeH(settingsBtnSize).MinW(settingsBtnSize).
		FlexShrink(0).Rounded(4).Bg(uiPanelAlt).Border(1, uiBorder).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) }).
		Child(gpui.NewText(label).TextSize(13))
}

// toggle is an on/off switch.
func toggle(id, tip string, on bool, f func(*gpui.Window)) *gpui.Div {
	track, knob := uiBorder, uiText
	pos := gpui.NewDiv().Flex1()
	if on {
		track = uiAccent
	}
	sw := gpui.NewDiv().ID(id).FlexRow().ItemsCenter().SizeWH(40, 22).P(2).Rounded(11).Bg(track).Tooltip(tip).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) })
	k := gpui.NewDiv().SizeWH(18, 18).Rounded(9).Bg(knob)
	if on {
		return sw.Child(pos, k)
	}
	return sw.Child(k, pos)
}

// segmented is a row of mutually exclusive buttons.
// tip is the tooltip of every button.
func segmented(idPrefix string, options [][2]string, tip, current string, f func(string, *gpui.Window)) *gpui.Div {
	row := gpui.NewDiv().FlexRow().Gap(4).FlexWrap()
	for _, o := range options {
		value, label := o[0], o[1]
		b := gpui.NewDiv().ID(idPrefix+value).Px_(12).Py_(5).Rounded(4).Border(1, uiBorder).Tooltip(tip).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(value, w) })
		if value == current {
			b.Bg(uiAccent).Child(gpui.NewText(label).TextSize(12).FontWeight(700))
		} else {
			b.Bg(uiPanelAlt).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
				Child(gpui.NewText(label).TextSize(12))
		}
		row.Child(b)
	}
	return row
}

func (ws *Workspace) drawingSection() *gpui.Div {
	c := ws.prefs().cur
	flag := func(id, label, tip string, on bool, set func(*settings.Settings, bool)) *gpui.Div {
		return settingRow(label, tip, toggle(id, tip, on, func(w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) { set(s, !on) })
		}))
	}
	wheelTip := i18n.T("ui.settings.wheel_tip")
	wheel := segmented("wheel-", [][2]string{
		{settings.WheelAuto, i18n.T("ui.settings.wheel_auto")}, {settings.WheelZoom, i18n.T("ui.settings.wheel_zoom")}, {settings.WheelPan, i18n.T("ui.settings.wheel_pan")},
	}, wheelTip, c.Wheel, func(v string, w *gpui.Window) { ws.change(w, func(s *settings.Settings) { s.Wheel = v }) })
	hint := map[string]string{
		settings.WheelAuto: i18n.T("ui.settings.wheel_hint_auto"),
		settings.WheelZoom: i18n.T("ui.settings.wheel_hint_zoom"),
		settings.WheelPan:  i18n.T("ui.settings.wheel_hint_pan"),
	}[c.Wheel]
	return gpui.NewDiv().ID("settings-sec-drawing").FlexCol().Gap(settingsRowGap).Child(
		sectionTitle(i18n.T("ui.settings.section_drawing")),
		gpui.NewText(i18n.T("ui.settings.drawing_hint")).TextColor(uiMuted).TextSize(12),
		flag("set-grid", i18n.T("ui.settings.grid"), i18n.T("ui.settings.grid_tip"), c.Grid, func(s *settings.Settings, v bool) { s.Grid = v }),
		flag("set-osnap", i18n.T("ui.settings.osnap"), i18n.T("ui.settings.osnap_tip"), c.OSnap, func(s *settings.Settings, v bool) { s.OSnap = v }),
		flag("set-ortho", i18n.T("ui.settings.ortho"), i18n.T("ui.settings.ortho_tip"), c.Ortho, func(s *settings.Settings, v bool) { s.Ortho = v }),
		settingRow(i18n.T("ui.settings.wheel"), wheelTip, gpui.NewDiv().FlexCol().Gap(4).Flex1().Child(wheel, gpui.NewText(hint).TextSize(11).TextColor(uiMuted))),
	)
}

// agentSection is the "AI AGENT" block: which agent a session starts with
// and which executable each agent runs (agentBinaryRows). How the PATH was
// searched is the discovery section's business.
func (ws *Workspace) agentSection() *gpui.Div {
	c := ws.prefs().cur
	opts := [][2]string{{"", i18n.T("ui.settings.harness_auto")}, {"opencode", "OpenCode"}, {"claude", "Claude"}, {"codex", "Codex"}}
	known := false
	for _, o := range opts {
		known = known || o[0] == c.DefaultAgent
	}
	if !known {
		opts = append(opts, [2]string{c.DefaultAgent, c.DefaultAgent})
	}
	harnessTip := i18n.T("ui.settings.default_harness_tip")
	return gpui.NewDiv().ID("settings-sec-agent").FlexCol().Gap(settingsRowGap).Child(
		sectionTitle(i18n.T("ui.settings.section_agent")),
		settingRow(i18n.T("ui.settings.default_harness"), harnessTip, segmented("agent-", opts, harnessTip, c.DefaultAgent, func(v string, w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) { s.DefaultAgent = v; s.AgentMode = settings.AgentAdvanced })
		}).Flex1()),
		gpui.NewText(i18n.T("ui.settings.agent_auto_hint")).TextSize(11).TextColor(uiMuted),
		ws.agentBinaryRows(),
	)
}

func (ws *Workspace) settingsFooter() *gpui.Div {
	p := ws.prefs()
	where := i18n.T("ui.settings.in_memory")
	if p.path != "" {
		where = i18n.T("ui.settings.saved_to", p.path)
	}
	foot := gpui.NewDiv().FlexCol().Gap(6).
		Child(gpui.NewText(where).TextSize(11).TextColor(uiMuted))
	if p.err != nil {
		foot.Child(gpui.NewText(p.err.Error()).TextSize(11).TextColor(uiErr))
	}
	for _, w := range p.warnings {
		foot.Child(gpui.NewText(w).TextSize(11).TextColor(uiWarn))
	}
	row := gpui.NewDiv().FlexRow().ItemsStart().Gap(12).Px_(20).Pt(10).Pb(14).Child(
		foot.Flex1().MinW(0),
		ws.smallButton("settings-reset", i18n.T("ui.settings.reset"), i18n.T("ui.settings.reset_tip"), func(w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) {
				d := settings.Default()
				d.Extra, d.Window, d.Layout, d.LastUpdateCheck = s.Extra, s.Window, s.Layout, s.LastUpdateCheck
				d.Recent = s.Recent
				*s = d
			})
		}).Px_(12),
	)
	return gpui.NewDiv().ID("settings-footer").FlexCol().FlexShrink(0).
		Child(gpui.NewDiv().SizeH(1).Bg(uiBorder), row)
}
