package ui

import (
	"fmt"
	"strings"

	"agentmanager"
	"agentmanager/transcript"
	"go-draw/internal/gpui"
)

// agentPanelWidth is the docked panel width in logical pixels.
const agentPanelWidth = 340

// agentOutputLimit caps the tool output shown in a block, in runes.
const agentOutputLimit = 280

func (ws *Workspace) agentToggle() *gpui.Div {
	on := ws.agent.visible
	c := uiText
	b := gpui.NewDiv().ID("tool-agent").FlexCol().ItemsCenter().Gap(2).Px_(10).Py_(4).Rounded(4).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.agent.Toggle(w) }).
		Child(gpui.NewText("AI").FontWeight(700).TextSize(14).TextColor(c), gpui.NewText("Agent").TextSize(10))
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
	input := p.input.Render(w, cx)

	head := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		gpui.NewText("AGENT").FontWeight(700).TextSize(11).TextColor(uiMuted),
		gpui.NewDiv().Flex1(),
		gpui.NewText(p.statusLine()).TextSize(11).TextColor(uiMuted),
	)
	picker := gpui.NewDiv().FlexRow().ItemsCenter().Gap(4)
	for _, h := range p.choices {
		h := h
		chip := gpui.NewDiv().ID("agent-pick-" + string(h)).Px_(8).Py_(2).Rounded(10).Bg(uiPanelAlt).
			Child(gpui.NewText(string(h)).TextSize(12))
		if h == p.selected {
			chip.Bg(uiAccent)
		}
		if p.state == agentIdle {
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
	if p.state == agentIdle {
		picker.Child(p.button("agent-start", "Start", true, p.Start))
	} else {
		picker.Child(p.button("agent-stop", "Stop", false, p.Stop))
	}

	list := gpui.NewDiv().ID("agent-list").FlexCol().Flex1().MinH(0).Gap(8).P(8).OverflowYScroll().TrackScroll(p.scroll)
	blocks := p.tr.Blocks()
	if len(blocks) == 0 {
		list.Child(gpui.NewText("Describe a drawing and the agent will draw it with the same commands you use.").TextColor(uiMuted))
	}
	for i, b := range blocks {
		if el := p.block(i, b); el != nil {
			list.Child(el)
		}
	}
	if v := p.tr.Version(); v != p.shownVersion {
		p.shownVersion = v
		p.scroll.ScrollToBottom()
	}

	send := p.button("agent-send", "Send", true, func(w *gpui.Window) { p.Send(p.input.Text(), w) })
	row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Child(
		gpui.NewDiv().Flex1().MinW(0).FlexCol().TextColor(gpui.Gray900).Child(input), send)
	if p.tr.Running() {
		row.Child(p.button("agent-cancel", "Cancel", false, p.Cancel))
	}

	return gpui.NewDiv().ID("agent-panel").FlexCol().SizeW(agentPanelWidth).FlexShrink(0).Bg(uiPanel).
		Child(
			gpui.NewDiv().FlexCol().Gap(6).P(10).Child(head, picker),
			gpui.NewDiv().SizeH(1).Bg(uiBorder),
			list,
			gpui.NewDiv().SizeH(1).Bg(uiBorder),
			gpui.NewDiv().FlexCol().P(8).Child(row),
		)
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
		open := p.expanded[i]
		mark := "> Thinking"
		if open {
			mark = "v Thinking"
		}
		d := gpui.NewDiv().ID(fmt.Sprintf("agent-thought-%d", i)).FlexCol().Gap(2).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.expanded[i] = !p.expanded[i]; w.Refresh() }).
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
