package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"time"
	"unicode"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/transcript"
	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// agentPanelWidth is the docked panel's default width for a window of the given width: a quarter of it,
// between 300 and 510 px.
func agentPanelWidth(window float32) float32 { return min(510, max(300, window/4)) }

// agentOutputLimit caps the tool output shown in a block, in runes.
const agentOutputLimit = 280

// agentParamsLimit caps a tool card's expanded parameters, in runes.
const agentParamsLimit = 2000

// agentUserInset is the left margin of a user message box, so the conversation reads as a chat.
const agentUserInset = 40

// toolParams is a tool call's input as indented JSON (raw when it is not JSON), clipped to
// agentParamsLimit; "" for none.
func toolParams(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "{}" {
		return ""
	}
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(s), "", "  ") == nil {
		s = buf.String()
	}
	if r := []rune(s); len(r) > agentParamsLimit {
		s = string(r[:agentParamsLimit]) + "\n..."
	}
	return s
}

// agentTip is the agent button's tooltip.
func (ws *Workspace) agentTip() string {
	return toolTip(i18n.CommandName("AI"), ws.keyHint(actionAgent, "AI"), i18n.T("ui.toolbar.agent_tip"))
}

// agentToggle is the title bar's button that shows and hides the agent panel; it carries a dot
// while an agent session is live (amber when a background tab wants attention).
func (ws *Workspace) agentToggle() *gpui.Div {
	on := ws.agent != nil && ws.agent.visible
	b := iconButton("tool-agent", ws.agentTip(), icons.AppAgent, on, func(w *gpui.Window) { ws.AgentPanel().Toggle(w) })
	if ws.agent != nil {
		if live, attn := ws.agent.Live(); attn {
			b.Relative().Child(gpui.NewDiv().Absolute().Top(3).Right(3).Child(dot(8, uiWarn)))
		} else if live {
			b.Relative().Child(gpui.NewDiv().Absolute().Top(3).Right(3).Child(dot(8, uiOK)))
		}
	}
	return b
}

// button is a panel button; tip is its tooltip and glyph is an optional leading icon.
func (p *AgentPanel) button(id, label, tip string, primary bool, f func(*gpui.Window), glyph ...icons.Name) *gpui.Div {
	bg := uiPanelAlt
	if primary {
		bg = uiAccent
	}
	b := gpui.NewDiv().ID(id).Px_(10).Py_(4).Rounded(4).Bg(bg).FlexShrink(0).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) })
	if len(glyph) > 0 {
		b.FlexRow().ItemsCenter().Gap(4).Child(NewIcon(glyph[0], 14, uiText))
	}
	return b.Child(gpui.NewText(label).TextSize(12))
}

// upperFirst returns s with its first letter in upper case.
func upperFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// statusPill is the header's rounded state label: a coloured dot and the session state.
func (p *AgentPanel) statusPill() *gpui.Div {
	col := uiMuted
	switch t := p.cur; t.state {
	case agentStarting:
		col = uiWarn
	case agentReady:
		col = uiOK
		if t.tr.Running() {
			col = uiOn
		}
	}
	var mark gpui.IntoElement = dot(7, col)
	if p.cur.tr.Running() && p.cur.state == agentReady && agentWorkingAnim {
		mark = workingIndicator()
	}
	return gpui.NewDiv().ID("agent-status").FlexRow().ItemsCenter().Gap(6).Px_(10).Py_(3).Rounded(12).Bg(uiPanelAlt).FlexShrink(0).
		Tooltip(i18n.T("ui.agent.status_tip")).
		Child(mark, gpui.NewText(upperFirst(p.statusLine())).TextSize(fontSmall).TextColor(uiMuted))
}

// noCLICard is the amber card shown while no agent CLI is on PATH: what is missing, where it
// was looked for, and a Start button that explains why nothing starts.
func (p *AgentPanel) noCLICard() *gpui.Div {
	title := i18n.T("ui.agent.no_cli_title")
	choices := harnesses.All
	if p.ws.prefs().cur.AgentMode == settings.AgentBasic && (!p.explicit || p.selected == agentmanager.OpenCode) {
		choices = []agentmanager.Harness{agentmanager.OpenCode}
		title = i18n.T("ui.agent.no_cli_basic_title")
	}
	var names []string
	for _, h := range choices {
		names = append(names, string(h))
	}
	start := gpui.NewDiv().ID("agent-start").Px_(14).Py_(6).Rounded(6).Bg(uiBg).Border(1, uiBorder).FlexShrink(0).
		Tooltip(i18n.T("ui.agent.start_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.Start(w) }).
		Child(gpui.NewText(i18n.T("ui.agent.start")).TextSize(fontUI))
	return gpui.NewDiv().ID("agent-no-cli").FlexRow().ItemsCenter().Gap(12).P(12).Rounded(10).
		Bg(mix(uiPanel, uiWarn, 0.12)).Border(1, mix(uiPanel, uiWarn, 0.45)).
		Child(
			NewIcon(icons.AppWarning, 22, uiWarn),
			gpui.NewDiv().FlexCol().Gap(2).Flex1().MinW(0).Child(
				gpui.NewText(title).TextSize(fontUI+1).FontWeight(700),
				gpui.NewText(i18n.T("ui.agent.no_cli_hint", strings.Join(names, ", "))).TextSize(fontSmall).TextColor(uiMuted)),
			start)
}

// keycap is a small outlined key with an icon on it.
func keycap(icon icons.Name) *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().JustifyCenter().SizeWH(22, 20).Rounded(5).Border(1, uiBorder).Bg(uiPanel).
		Child(NewIcon(icon, 12, uiMuted))
}

// composerButton is the composer's primary button: an icon and a label on the primary fill.
func composerButton(id, label, tip string, icon icons.Name, f func(*gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).FlexRow().ItemsCenter().Gap(8).Px_(16).Py_(7).Rounded(8).Bg(uiPrimary).FlexShrink(0).
		Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Opacity(0.88) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) }).
		Child(NewIcon(icon, 16, uiOnPrimary), gpui.NewText(label).TextSize(fontUI).FontWeight(700).TextColor(uiOnPrimary))
}

// composer is the message box: the input on top; below it the send hint and the Send button.
// While a turn runs Send becomes Cancel, which cancels the turn (the header's Stop ends the
// session); once something is typed it becomes Queue, with Cancel beside it as an icon button.
func (p *AgentPanel) composer(input gpui.IntoElement) *gpui.Div {
	t := p.cur
	send := func(w *gpui.Window) { p.Send(p.input.Text(), w) }
	foot := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		keycap(icons.AppKeyEnter),
		gpui.NewText(i18n.T("ui.agent.hint_send")).TextSize(fontSmall).TextColor(uiMuted),
		gpui.NewDiv().Flex1())
	typed := strings.TrimSpace(p.input.Text()) != "" || len(t.images) > 0
	switch {
	case !t.tr.Running():
		foot.Child(composerButton("agent-send", i18n.T("ui.agent.send"), i18n.T("ui.agent.send_tip"), icons.AppSend, send))
	case typed:
		cancel := iconButton("agent-cancel", i18n.T("ui.agent.cancel_tip"), icons.AppCancel, false, p.Cancel)
		foot.Child(cancel, composerButton("agent-send", i18n.T("ui.agent.queue.button"), i18n.T("ui.agent.queue.button_tip"), icons.AppSend, send))
	default:
		foot.Child(composerButton("agent-cancel", i18n.T("ui.agent.cancel"), i18n.T("ui.agent.cancel_tip"), icons.AppCancel, p.Cancel))
	}
	box := gpui.NewDiv().ID("agent-composer").FlexCol().Gap(8).P(12).Rounded(10).Border(1, uiBorder).Bg(uiField)
	if len(t.images) > 0 {
		box.Child(p.imageChips())
	}
	return box.Child(gpui.NewDiv().ID("agent-input").MinH(36).FlexCol().Child(input), foot)
}

func (p *AgentPanel) render(w *gpui.Window, cx *gpui.App, width float32) *gpui.Div {
	if p.ws.prefs().cur.AgentMode == settings.AgentBasic {
		p.ws.refreshOllamaModels(w, false)
	}
	p.win = w
	p.frames++
	p.probe()
	if q := p.deferred; q != "" {
		p.deferred = ""
		p.sendFromCommand(q, w)
	}
	t := p.cur
	input := p.input.Render(w, cx)

	// locked: the tab's harness is fixed once it started or when it holds a stored conversation;
	// the header then names it and the harness chips are not shown.
	locked := t.state != agentIdle || t.rec != nil
	cur := p.harnessOf(t)
	if t.state != agentIdle && t.harness != "" {
		cur = t.harness
	}

	collapse := iconButton("agent-collapse", i18n.T("ui.agent.collapse_tip"), icons.AppChevronRight, false, p.Toggle)
	collapse.SizeWH(28, 28)
	head := gpui.NewDiv().FlexRow().ItemsCenter().Gap(10).Child(
		gpui.NewText(i18n.T("ui.agent.title")).FontWeight(700).TextSize(fontTitle))
	if locked && cur != "" {
		head.Child(gpui.NewDiv().ID("agent-harness").FlexShrink(0).Tooltip(i18n.T("ui.agent.harness_tip")).
			Child(gpui.NewText("· " + string(cur)).TextSize(fontTitle).TextColor(uiMuted)))
	}
	head.Child(
		p.statusPill(),
		gpui.NewDiv().Flex1(),
		collapse,
	)
	tabs := gpui.NewDiv().FlexRow().FlexWrap().ItemsCenter().Gap(4)
	for i, ti := range p.Tabs() {
		i, ti := i, ti
		tabDot := gpui.NewDiv().SizeWH(6, 6).Rounded(3).FlexShrink(0)
		switch {
		case ti.Attention:
			tabDot.Bg(uiWarn)
		case ti.Working:
			tabDot.Bg(uiAccent)
		case ti.Live:
			tabDot.Bg(uiOK)
		}
		label := gpui.NewDiv().ID(fmt.Sprintf("agent-tab-%d", i)).FlexRow().ItemsCenter().Gap(4).Tooltip(i18n.T("ui.agent.tab_tip")).Child(tabDot, gpui.NewText(ti.Title).TextSize(fontSmall)).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.SwitchTab(i, w) })
		closeBtn := gpui.NewDiv().ID(fmt.Sprintf("agent-tab-close-%d", i)).Px_(3).Tooltip(i18n.T("ui.agent.close_tab_tip")).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.CloseTab(i, w) }).
			Child(gpui.NewText("x").TextSize(11).TextColor(uiMuted))
		chip := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Pl(8).Pr(2).Py_(2).Rounded(10).Bg(uiPanelAlt).Child(label, closeBtn)
		if ti.Active {
			chip.Bg(activeFill())
		}
		tabs.Child(chip)
	}
	tabs.Child(p.button("agent-new", i18n.T("ui.agent.new_tab"), i18n.T("ui.agent.new_tab_tip"), false, p.NewTab))
	histLabel := i18n.T("ui.agent.history")
	tabs.Child(p.button("agent-history", histLabel, i18n.T("ui.agent.history_tip"), false, func(w *gpui.Window) {
		p.ensureLoaded()
		p.hist.menu = !p.hist.menu
		p.modelMenu = false
		p.syncSessions()
		w.Refresh()
	}))

	var picker gpui.IntoElement
	if len(p.choices) == 0 {
		picker = p.noCLICard()
	} else {
		row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(4)
		if !locked && p.ws.prefs().cur.AgentMode == settings.AgentAdvanced {
			for _, h := range p.choices {
				h := h
				chip := gpui.NewDiv().ID("agent-pick-" + string(h)).Px_(8).Py_(2).Rounded(10).Bg(uiPanelAlt).Tooltip(i18n.T("ui.agent.pick_tip")).
					Child(gpui.NewText(string(h)).TextSize(fontSmall))
				if h == cur {
					chip.Bg(activeFill())
				}
				chip.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.selected = h; w.Refresh() })
				row.Child(chip)
			}
		}
		row.Child(p.modelChip())
		row.Child(gpui.NewDiv().Flex1())
		if t.state == agentIdle {
			row.Child(p.button("agent-start", i18n.T("ui.agent.start"), i18n.T("ui.agent.start_tip"), true, p.Start))
		} else {
			row.Child(p.button("agent-stop", i18n.T("ui.agent.stop"), i18n.T("ui.agent.stop_tip"), false, p.Stop, icons.AppStop))
		}
		picker = row
	}

	top := gpui.NewDiv().FlexCol().Gap(10).P(16).Pb(12).Child(head, tabs, picker)
	if p.spike != nil {
		top.Child(p.harnessSpikeControls(w, cx))
	}
	if p.ws.installWanted(len(p.choices) == 0) {
		top.Child(p.ws.agentInstallBlock("agent"))
	}
	if p.hist.menu {
		top.Child(p.historyMenu())
	}

	p.checkLook(w)
	p.reportClipboard()
	list := p.transcriptList()
	if v := t.tr.Version(); v != t.shownVersion {
		t.shownVersion = v
		t.scroll.ScrollToBottom()
	}

	marker := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		if p.modelPanel != b {
			p.modelPanel = b
			if p.modelMenu {
				w.Refresh()
			}
		}
	})
	marker.Absolute().Inset(0)
	panel := gpui.NewDiv().ID("agent-panel").Relative().FlexCol().FlexShrink(0).SizeW(width).MinW(0).MinH(0).OverflowHidden().Bg(uiPanel).
		OnFileDrop(p.onFileDrop).OnImagePaste(p.onImagePaste).Child(
		top,
		hDivider(),
		list,
		hDivider(),
		p.composerArea(input),
		marker,
	)
	if p.modelMenu {
		below := float32(p.modelAnchor.Y-p.modelPanel.Origin.Y) + 28
		available := max(0, float32(p.modelPanel.Size.H)-below-12)
		panel.Child(p.modelMenuView(w, cx).Absolute().Left(16).Right(16).Top(below).MaxH(min(320, available)))
	}
	return panel
}

// composerArea is the bottom of the panel: the queued prompts, if any, above the composer.
func (p *AgentPanel) composerArea(input gpui.IntoElement) *gpui.Div {
	area := gpui.NewDiv().FlexCol().FlexShrink(0).Gap(8).P(12)
	if len(p.cur.queue) > 0 {
		area.Child(p.queueChips())
	}
	return area.Child(p.composer(input))
}

// historyMenu lists the project's stored conversations, newest first.
func (p *AgentPanel) historyMenu() *gpui.Div {
	menu := gpui.NewDiv().ID("agent-history-menu").FlexCol().Gap(2).P(4).Rounded(6).Bg(uiPanelAlt).Border(1, uiBorder)
	if len(p.hist.sessions) == 0 {
		return menu.Child(gpui.NewText(i18n.T("ui.agent.history_empty")).TextSize(12).TextColor(uiMuted))
	}
	open := map[string]bool{}
	for _, t := range p.Tabs() {
		open[t.SessionID] = true
	}
	now := time.Now()
	for i, m := range p.hist.sessions {
		if i == agentHistoryShown {
			break
		}
		m := m
		title := m.Title
		if title == "" {
			title = i18n.T("ui.agent.history_untitled")
		}
		sub := string(m.Harness) + ", " + relTime(m.Updated, now)
		if m.Drawing != "" && m.Drawing != p.hist.drawing {
			sub = clip(m.Drawing, 16) + ", " + sub
		}
		if open[m.ID] {
			sub += ", " + i18n.T("ui.agent.history_open")
		}
		menu.Child(gpui.NewDiv().ID("agent-sess-"+m.ID).FlexRow().ItemsCenter().Gap(6).Px_(6).Py_(3).Rounded(4).Tooltip(i18n.T("ui.agent.session_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.OpenSession(m.ID, w) }).
			Child(gpui.NewText(clip(title, 30)).TextSize(12).Flex1().MinW(0), gpui.NewText(sub).TextSize(10).TextColor(uiMuted).FlexShrink(0)))
	}
	return menu
}

func statusColor(s agentmanager.ToolStatus) gpui.Rgba {
	switch s {
	case agentmanager.ToolCompleted:
		return uiOK
	case agentmanager.ToolFailed:
		return uiErr
	case agentmanager.ToolInProgress:
		return uiWarn
	}
	return uiMuted
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return string(r)
}

func (p *AgentPanel) card() *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(4).P(8).Rounded(6).Bg(uiPanelAlt).Border(1, uiBorder)
}

// agentLook is what a transcript block's height depends on besides its content and the width.
type agentLook struct {
	lang  i18n.Lang
	theme string
	scale float32
}

// checkLook drops every transcript's cached block heights when the language, theme or UI scale
// changed since the last frame: labels, fonts and sizes may differ, and blocks off-screen would
// keep the old heights until scrolled into view.
func (p *AgentPanel) checkLook(w *gpui.Window) {
	cur := agentLook{lang: i18n.Current(), theme: CurrentTheme().Name, scale: w.UIScale()}
	if cur == p.look {
		return
	}
	p.look = cur
	for _, s := range append(p.sets(), p.agentSet) {
		if s == nil {
			continue
		}
		for _, t := range s.tabs {
			t.list.InvalidateAll()
		}
	}
}

// agentHintKey is the list key of the empty-transcript hint; block keys are revisions shifted left.
const agentHintKey = ^uint64(0)

// transcriptList is the virtualized transcript: only the blocks near the viewport are built, the
// others count by their measured height (gpui.List). A block's key is its transcript revision plus
// its expanded bit, so a block that changed off-screen is measured again.
func (p *AgentPanel) transcriptList() *gpui.List {
	t := p.cur
	blocks := t.tr.Blocks()
	if v := t.tr.Version(); !t.shownOK || t.shownFor != v {
		t.shown = t.shown[:0]
		for i, b := range blocks {
			if agentBlockShown(b) {
				t.shown = append(t.shown, i)
			}
		}
		t.shownFor, t.shownOK = v, true
	}
	shown := t.shown
	hint := len(blocks) == 0 && len(p.choices) > 0
	count := len(shown)
	if hint {
		count = 1
	}
	render := func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
		if hint {
			return gpui.NewText(i18n.T("ui.agent.empty_hint")).TextColor(uiMuted)
		}
		i := shown[ix]
		return p.block(i, blocks[i])
	}
	key := func(ix int) uint64 {
		if hint {
			return agentHintKey
		}
		i := shown[ix]
		k := t.tr.Revision(i) << 1
		if t.expanded[i] {
			k |= 1
		}
		return k
	}
	return gpui.NewList("agent-list", t.list, count, render).ItemKey(key).
		Flex1().MinH(0).Gap(8).P(12).TrackScroll(t.scroll)
}

// agentBlockShown reports whether a transcript block is drawn: debug notes are not.
func agentBlockShown(b transcript.Block) bool {
	n, ok := b.(transcript.Note)
	return !ok || n.Level != "debug"
}

func (p *AgentPanel) block(i int, b transcript.Block) gpui.IntoElement {
	switch b := b.(type) {
	case transcript.UserMsg:
		head := gpui.NewDiv().FlexRow().ItemsCenter().JustifyBetween().Child(
			gpui.NewText(i18n.T("ui.agent.you")).FontWeight(700).TextSize(11), p.copyButton(i, b.Text))
		d := gpui.NewDiv().ID(fmt.Sprintf("agent-user-%d", i)).FlexCol().Gap(2).P(8).Rounded(6).Bg(uiAccent).Child(head)
		if b.Text != "" {
			d.Child(gpui.NewText(b.Text))
		}
		if len(b.Attachments) > 0 {
			if p.thumbs == nil {
				p.thumbs = map[string]*image.NRGBA{}
			}
			d.Child(attachmentRow(b.Attachments, p.thumbs))
		}
		// The inset is the wrapper's padding: a list item's own margin is not laid out.
		return gpui.NewDiv().FlexCol().Pl(agentUserInset).Child(d)
	case transcript.AgentMsg:
		md := p.mdView(i, fontUI, false).render(p.markdownOf(i, b.Text))
		return gpui.NewDiv().FlexCol().Gap(2).MinW(0).Child(md,
			gpui.NewDiv().FlexRow().Child(p.copyButton(i, b.Text)))
	case transcript.Thought:
		open := p.cur.expanded[i]
		mark := "> " + i18n.T("ui.agent.thinking")
		if open {
			mark = "v " + i18n.T("ui.agent.thinking")
		}
		d := gpui.NewDiv().ID(fmt.Sprintf("agent-thought-%d", i)).FlexCol().Gap(2).Tooltip(i18n.T("ui.agent.thought_tip")).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				p.cur.expanded[i] = !p.cur.expanded[i]
				w.Refresh()
			}).
			Child(gpui.NewText(mark).TextSize(11).TextColor(uiMuted))
		if open {
			d.Child(p.mdView(i, 12, true).render(p.markdownOf(i, b.Text)))
		}
		return d
	case transcript.ToolBlock:
		title := b.Call.Title
		if title == "" {
			title = string(b.Call.Kind)
		}
		head := gpui.NewDiv().ID(fmt.Sprintf("agent-tool-%d", i)).FlexRow().ItemsCenter().Gap(6).Child(
			gpui.NewDiv().SizeWH(8, 8).Rounded(4).Bg(statusColor(b.Status)).FlexShrink(0),
			gpui.NewText(title).Flex1().MinW(0).FontWeight(700).TextSize(12),
			gpui.NewText(string(b.Status)).TextSize(11).TextColor(statusColor(b.Status)).FlexShrink(0))
		c := p.card().Child(head)
		if params := toolParams(b.Call.RawInput); params != "" {
			open := p.cur.expanded[i]
			chev := icons.AppChevronRight
			if open {
				chev = icons.AppChevronDown
			}
			head.Child(NewIcon(chev, 12, uiMuted)).Tooltip(i18n.T("ui.agent.tool_params_tip")).
				OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
					p.cur.expanded[i] = !p.cur.expanded[i]
					w.Refresh()
				})
			if open {
				c.Child(gpui.NewDiv().ID(fmt.Sprintf("agent-tool-params-%d", i)).P(6).Rounded(4).Bg(codeFill()).MinW(0).
					Child(gpui.NewText(params).FontFamily(monoFamily).TextSize(11).TextColor(uiText)))
			}
		}
		if b.Output != "" {
			c.Child(gpui.NewText(clip(b.Output, agentOutputLimit)).TextSize(11).TextColor(uiMuted))
		}
		return c
	case transcript.PermissionBlock:
		title := i18n.T("ui.agent.tool_default")
		if b.Call.Title != nil {
			title = *b.Call.Title
		}
		c := p.card().Child(gpui.NewText(i18n.T("ui.agent.permission")).FontWeight(700).TextSize(11).TextColor(uiWarn),
			gpui.NewText(title).TextSize(12))
		if b.Answered {
			return c.Child(gpui.NewText(i18n.T("ui.agent.answered")).TextSize(11).TextColor(uiMuted))
		}
		row := gpui.NewDiv().FlexRow().FlexWrap().Gap(4)
		for _, o := range b.Options {
			o := o
			row.Child(p.button("agent-perm-"+b.RequestID+"-"+o.ID, o.Name, i18n.T("ui.agent.permission_tip"), o.Kind == agentmanager.AllowOnce || o.Kind == agentmanager.AllowAlways,
				func(w *gpui.Window) { p.Respond(b.RequestID, o.ID, w) }))
		}
		return c.Child(row)
	case transcript.Plan:
		c := p.card().Child(gpui.NewText(i18n.T("ui.agent.plan")).FontWeight(700).TextSize(11).TextColor(uiMuted))
		for _, e := range b.Entries {
			mark, col := "[ ]", uiMuted
			switch e.Status {
			case "completed":
				mark, col = "[x]", uiOK
			case "in_progress":
				mark, col = "[~]", uiWarn
			}
			c.Child(gpui.NewDiv().FlexRow().Gap(6).Child(
				gpui.NewText(mark).TextSize(12).TextColor(col).FlexShrink(0),
				gpui.NewText(e.Content).TextSize(12).Flex1().MinW(0)))
		}
		return c
	case transcript.Note:
		col := uiMuted
		switch b.Level {
		case "error":
			col = uiErr
		case "warn":
			col = uiWarn
		case "debug":
			return nil
		}
		return gpui.NewText(b.Text).TextSize(12).TextColor(col)
	}
	return nil
}
