package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/transcript"
	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// agentPanelWidth is the docked panel's default width for a window of the given width: a quarter of it,
// between 300 and 510 px.
func agentPanelWidth(window float32) float32 { return min(510, max(300, window/4)) }

// agentOutputLimit caps the tool output shown in a block, in runes.
const agentOutputLimit = 280

// agentToggle is the title bar's button that shows and hides the agent panel; it carries a dot
// while an agent session is live (amber when a background tab wants attention).
func (ws *Workspace) agentToggle() *gpui.Div {
	on := ws.agent != nil && ws.agent.visible
	b := iconButton("tool-agent", i18n.T("ui.toolbar.agent_tip"), icons.AppAgent, on, func(w *gpui.Window) { ws.AgentPanel().Toggle(w) })
	if ws.agent != nil {
		if live, attn := ws.agent.Live(); attn {
			b.Relative().Child(gpui.NewDiv().Absolute().Top(3).Right(3).Child(dot(8, uiWarn)))
		} else if live {
			b.Relative().Child(gpui.NewDiv().Absolute().Top(3).Right(3).Child(dot(8, uiOK)))
		}
	}
	return b
}

// button is a panel button; tip is its tooltip.
func (p *AgentPanel) button(id, label, tip string, primary bool, f func(*gpui.Window)) *gpui.Div {
	bg := uiPanelAlt
	if primary {
		bg = uiAccent
	}
	return gpui.NewDiv().ID(id).Px_(10).Py_(4).Rounded(4).Bg(bg).FlexShrink(0).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) }).
		Child(gpui.NewText(label).TextSize(12))
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
	return gpui.NewDiv().ID("agent-status").FlexRow().ItemsCenter().Gap(6).Px_(10).Py_(3).Rounded(12).Bg(uiPanelAlt).FlexShrink(0).
		Tooltip(i18n.T("ui.agent.status_tip")).
		Child(dot(7, col), gpui.NewText(upperFirst(p.statusLine())).TextSize(fontSmall).TextColor(uiMuted))
}

// noCLICard is the amber card shown while no agent CLI is on PATH: what is missing, where it
// was looked for, and a Start button that explains why nothing starts.
func (p *AgentPanel) noCLICard() *gpui.Div {
	var names []string
	for _, h := range harnesses.All {
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
				gpui.NewText(i18n.T("ui.agent.no_cli_title")).TextSize(fontUI+1).FontWeight(700),
				gpui.NewText(i18n.T("ui.agent.no_cli_hint", strings.Join(names, ", "))).TextSize(fontSmall).TextColor(uiMuted)),
			start)
}

// keycap is a small outlined key with an icon on it.
func keycap(icon icons.Name) *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().JustifyCenter().SizeWH(22, 20).Rounded(5).Border(1, uiBorder).Bg(uiPanel).
		Child(NewIcon(icon, 12, uiMuted))
}

// composer is the message box: the input on top; below it the send hint and the Send button
// (or Cancel while a turn runs).
func (p *AgentPanel) composer(input gpui.IntoElement) *gpui.Div {
	t := p.cur
	send := gpui.NewDiv().ID("agent-send").FlexRow().ItemsCenter().Gap(8).Px_(16).Py_(7).Rounded(8).Bg(uiPrimary).FlexShrink(0).
		Tooltip(i18n.T("ui.agent.send_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Opacity(0.88) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.Send(p.input.Text(), w) }).
		Child(NewIcon(icons.AppSend, 16, uiOnPrimary), gpui.NewText(i18n.T("ui.agent.send")).TextSize(fontUI).FontWeight(700).TextColor(uiOnPrimary))
	foot := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		keycap(icons.AppKeyEnter),
		gpui.NewText(i18n.T("ui.agent.hint_send")).TextSize(fontSmall).TextColor(uiMuted),
		gpui.NewDiv().Flex1())
	if t.tr.Running() {
		foot.Child(p.button("agent-cancel", i18n.T("ui.agent.cancel"), i18n.T("ui.agent.cancel_tip"), false, p.Cancel))
	}
	foot.Child(send)
	return gpui.NewDiv().ID("agent-composer").FlexCol().Gap(8).P(12).Rounded(10).Border(1, uiBorder).Bg(uiField).
		Child(gpui.NewDiv().SizeH(36).FlexCol().Child(input), foot)
}

func (p *AgentPanel) render(w *gpui.Window, cx *gpui.App, width float32) *gpui.Div {
	p.win = w
	p.probe()
	if q := p.deferred; q != "" {
		p.deferred = ""
		p.sendFromCommand(q, w)
	}
	t := p.cur
	input := p.input.Render(w, cx)

	collapse := iconButton("agent-collapse", i18n.T("ui.agent.collapse_tip"), icons.AppChevronRight, false, p.Toggle)
	collapse.SizeWH(28, 28)
	head := gpui.NewDiv().FlexRow().ItemsCenter().Gap(10).Child(
		gpui.NewText(i18n.T("ui.agent.title")).FontWeight(700).TextSize(fontTitle),
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
		p.syncSessions()
		w.Refresh()
	}))

	locked := t.state != agentIdle || t.rec != nil
	cur := p.harnessOf(t)
	var picker gpui.IntoElement
	if len(p.choices) == 0 {
		picker = p.noCLICard()
	} else {
		row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(4)
		for _, h := range p.choices {
			h := h
			chip := gpui.NewDiv().ID("agent-pick-" + string(h)).Px_(8).Py_(2).Rounded(10).Bg(uiPanelAlt).Tooltip(i18n.T("ui.agent.pick_tip")).
				Child(gpui.NewText(string(h)).TextSize(fontSmall))
			if h == cur {
				chip.Bg(activeFill())
			}
			if !locked {
				chip.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.selected = h; w.Refresh() })
			} else {
				chip.Opacity(0.6)
			}
			row.Child(chip)
		}
		row.Child(gpui.NewDiv().Flex1())
		if t.state == agentIdle {
			row.Child(p.button("agent-start", i18n.T("ui.agent.start"), i18n.T("ui.agent.start_tip"), true, p.Start))
		} else {
			row.Child(p.button("agent-stop", i18n.T("ui.agent.stop"), i18n.T("ui.agent.stop_tip"), false, p.Stop))
		}
		picker = row
	}

	top := gpui.NewDiv().FlexCol().Gap(10).P(16).Pb(12).Child(head, tabs, picker)
	if p.hist.menu {
		top.Child(p.historyMenu())
	}

	list := gpui.NewDiv().ID("agent-list").FlexCol().Flex1().MinH(0).Gap(8).P(12).OverflowYScroll().TrackScroll(t.scroll)
	blocks := t.tr.Blocks()
	if len(blocks) == 0 && len(p.choices) > 0 {
		list.Child(gpui.NewText(i18n.T("ui.agent.empty_hint")).TextColor(uiMuted))
	}
	for i, b := range blocks {
		if el := p.block(i, b); el != nil {
			list.Child(el)
		}
	}
	if v := t.tr.Version(); v != t.shownVersion {
		t.shownVersion = v
		t.scroll.ScrollToBottom()
	}

	return gpui.NewDiv().ID("agent-panel").FlexCol().FlexShrink(0).SizeW(width).MinW(0).Bg(uiPanel).Child(
		top,
		hDivider(),
		list,
		hDivider(),
		gpui.NewDiv().FlexCol().P(12).Child(p.composer(input)),
	)
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

func (p *AgentPanel) block(i int, b transcript.Block) gpui.IntoElement {
	switch b := b.(type) {
	case transcript.UserMsg:
		return gpui.NewDiv().FlexCol().Gap(2).P(8).Rounded(6).Bg(uiAccent).
			Child(gpui.NewText(i18n.T("ui.agent.you")).FontWeight(700).TextSize(11), gpui.NewText(b.Text))
	case transcript.AgentMsg:
		return gpui.NewText(b.Text)
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
			d.Child(gpui.NewText(b.Text).TextSize(12).TextColor(uiMuted))
		}
		return d
	case transcript.ToolBlock:
		title := b.Call.Title
		if title == "" {
			title = string(b.Call.Kind)
		}
		c := p.card().Child(gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
			gpui.NewDiv().SizeWH(8, 8).Rounded(4).Bg(statusColor(b.Status)).FlexShrink(0),
			gpui.NewText(title).Flex1().MinW(0).FontWeight(700).TextSize(12),
			gpui.NewText(string(b.Status)).TextSize(11).TextColor(statusColor(b.Status)).FlexShrink(0)))
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
