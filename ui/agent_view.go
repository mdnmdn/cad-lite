package ui

import (
	"fmt"
	"strings"
	"time"

	"agentmanager"
	"agentmanager/transcript"
	"go-draw/internal/gpui"
)

// agentPanelWidth is the docked panel width in logical pixels.
const agentPanelWidth = 340

// agentOutputLimit caps the tool output shown in a block, in runes.
const agentOutputLimit = 280

func (ws *Workspace) agentToggle() *gpui.Div {
	on := ws.agent != nil && ws.agent.visible
	c := uiText
	dot := gpui.NewDiv().SizeWH(6, 6).Rounded(3).FlexShrink(0)
	if ws.agent != nil {
		if live, attn := ws.agent.Live(); attn {
			dot.Bg(uiWarn)
		} else if live {
			dot.Bg(uiOK)
		}
	}
	b := gpui.NewDiv().ID("tool-agent").FlexCol().ItemsCenter().Gap(2).Px_(10).Py_(4).Rounded(4).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.AgentPanel().Toggle(w) }).
		Child(gpui.NewDiv().FlexRow().ItemsCenter().Gap(3).Child(gpui.NewText("AI").FontWeight(700).TextSize(14).TextColor(c), dot), gpui.NewText("Agent").TextSize(10))
	if on {
		b.Bg(uiAccent)
	}
	return b
}

func (p *AgentPanel) button(id, label string, primary bool, f func(*gpui.Window)) *gpui.Div {
	bg := uiPanelAlt
	if primary {
		bg = uiAccent
	}
	return gpui.NewDiv().ID(id).Px_(10).Py_(4).Rounded(4).Bg(bg).FlexShrink(0).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) }).
		Child(gpui.NewText(label).TextSize(12))
}

func (p *AgentPanel) render(w *gpui.Window, cx *gpui.App) *gpui.Div {
	p.win = w
	p.probe()
	if q := p.deferred; q != "" {
		p.deferred = ""
		p.sendFromCommand(q, w)
	}
	t := p.cur
	input := p.input.Render(w, cx)

	head := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		gpui.NewText("AGENT").FontWeight(700).TextSize(11).TextColor(uiMuted),
		gpui.NewDiv().Flex1(),
		gpui.NewText(p.statusLine()).TextSize(11).TextColor(uiMuted),
	)
	tabs := gpui.NewDiv().FlexRow().FlexWrap().ItemsCenter().Gap(4)
	for i, ti := range p.Tabs() {
		i, ti := i, ti
		dot := gpui.NewDiv().SizeWH(6, 6).Rounded(3).FlexShrink(0)
		switch {
		case ti.Attention:
			dot.Bg(uiWarn)
		case ti.Working:
			dot.Bg(uiAccent)
		case ti.Live:
			dot.Bg(uiOK)
		}
		label := gpui.NewDiv().ID(fmt.Sprintf("agent-tab-%d", i)).FlexRow().ItemsCenter().Gap(4).Child(dot, gpui.NewText(ti.Title).TextSize(12)).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.SwitchTab(i, w) })
		closeBtn := gpui.NewDiv().ID(fmt.Sprintf("agent-tab-close-%d", i)).Px_(3).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.CloseTab(i, w) }).
			Child(gpui.NewText("x").TextSize(11).TextColor(uiMuted))
		chip := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Pl(8).Pr(2).Py_(2).Rounded(10).Bg(uiPanelAlt).Child(label, closeBtn)
		if ti.Active {
			chip.Bg(uiAccent)
		}
		tabs.Child(chip)
	}
	tabs.Child(p.button("agent-new", "+", false, p.NewTab))
	histLabel := "History"
	tabs.Child(p.button("agent-history", histLabel, false, func(w *gpui.Window) {
		p.ensureLoaded()
		p.hist.menu = !p.hist.menu
		p.syncSessions()
		w.Refresh()
	}))

	picker := gpui.NewDiv().FlexRow().ItemsCenter().Gap(4)
	locked := t.state != agentIdle || t.rec != nil
	cur := p.harnessOf(t)
	for _, h := range p.choices {
		h := h
		chip := gpui.NewDiv().ID("agent-pick-" + string(h)).Px_(8).Py_(2).Rounded(10).Bg(uiPanelAlt).
			Child(gpui.NewText(string(h)).TextSize(12))
		if h == cur {
			chip.Bg(uiAccent)
		}
		if !locked {
			chip.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.selected = h; w.Refresh() })
		} else {
			chip.Opacity(0.6)
		}
		picker.Child(chip)
	}
	if len(p.choices) == 0 {
		picker.Child(gpui.NewText("No agent CLI on PATH (opencode, claude, codex)").TextSize(12).TextColor(uiWarn))
	}
	picker.Child(gpui.NewDiv().Flex1())
	if t.state == agentIdle {
		picker.Child(p.button("agent-start", "Start", true, p.Start))
	} else {
		picker.Child(p.button("agent-stop", "Stop", false, p.Stop))
	}

	top := gpui.NewDiv().FlexCol().Gap(6).P(10).Child(head, tabs, picker)
	if p.hist.menu {
		top.Child(p.historyMenu())
	}

	list := gpui.NewDiv().ID("agent-list").FlexCol().Flex1().MinH(0).Gap(8).P(8).OverflowYScroll().TrackScroll(t.scroll)
	blocks := t.tr.Blocks()
	if len(blocks) == 0 {
		list.Child(gpui.NewText("Describe a drawing and the agent will draw it with the same commands you use.").TextColor(uiMuted))
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

	send := p.button("agent-send", "Send", true, func(w *gpui.Window) { p.Send(p.input.Text(), w) })
	row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		gpui.NewDiv().Flex1().MinW(0).FlexCol().Child(input), send)
	if t.tr.Running() {
		row.Child(p.button("agent-cancel", "Cancel", false, p.Cancel))
	}

	return gpui.NewDiv().ID("agent-panel").FlexCol().SizeW(agentPanelWidth).FlexShrink(0).Bg(uiPanel).
		Child(
			top,
			gpui.NewDiv().SizeH(1).Bg(uiBorder),
			list,
			gpui.NewDiv().SizeH(1).Bg(uiBorder),
			gpui.NewDiv().FlexCol().P(8).Child(row),
		)
}

// historyMenu lists the project's stored conversations, newest first.
func (p *AgentPanel) historyMenu() *gpui.Div {
	menu := gpui.NewDiv().ID("agent-history-menu").FlexCol().Gap(2).P(4).Rounded(6).Bg(uiPanelAlt).Border(1, uiBorder)
	if len(p.hist.sessions) == 0 {
		return menu.Child(gpui.NewText("No earlier conversations in this project").TextSize(12).TextColor(uiMuted))
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
			title = "(empty)"
		}
		sub := string(m.Harness) + ", " + relTime(m.Updated, now)
		if open[m.ID] {
			sub += ", open"
		}
		menu.Child(gpui.NewDiv().ID("agent-sess-"+m.ID).FlexRow().ItemsCenter().Gap(6).Px_(6).Py_(3).Rounded(4).
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
			Child(gpui.NewText("You").FontWeight(700).TextSize(11), gpui.NewText(b.Text))
	case transcript.AgentMsg:
		return gpui.NewText(b.Text)
	case transcript.Thought:
		open := p.cur.expanded[i]
		mark := "> Thinking"
		if open {
			mark = "v Thinking"
		}
		d := gpui.NewDiv().ID(fmt.Sprintf("agent-thought-%d", i)).FlexCol().Gap(2).
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
		title := "a tool"
		if b.Call.Title != nil {
			title = *b.Call.Title
		}
		c := p.card().Child(gpui.NewText("Permission requested").FontWeight(700).TextSize(11).TextColor(uiWarn),
			gpui.NewText(title).TextSize(12))
		if b.Answered {
			return c.Child(gpui.NewText("answered").TextSize(11).TextColor(uiMuted))
		}
		row := gpui.NewDiv().FlexRow().FlexWrap().Gap(4)
		for _, o := range b.Options {
			o := o
			row.Child(p.button("agent-perm-"+b.RequestID+"-"+o.ID, o.Name, o.Kind == agentmanager.AllowOnce || o.Kind == agentmanager.AllowAlways,
				func(w *gpui.Window) { p.Respond(b.RequestID, o.ID, w) }))
		}
		return c.Child(row)
	case transcript.Plan:
		c := p.card().Child(gpui.NewText("Plan").FontWeight(700).TextSize(11).TextColor(uiMuted))
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
