package ui

import (
	"fmt"
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// agentQueued is a prompt typed while a turn ran, with the images that were in the composer.
type agentQueued struct {
	text string
	imgs []*agentImage
}

// agentQueueChipChars caps a queued prompt's chip text, in runes.
const agentQueueChipChars = 40

// enqueue adds the composer's prompt to t's queue and clears the composer.
func (p *AgentPanel) enqueue(t *agentTab, text string, imgs []*agentImage) {
	t.queue = append(t.queue, agentQueued{text: text, imgs: imgs})
	p.input.SetText("")
	t.images = nil
}

// Queued returns the active tab's queued prompts, in the order they will be sent.
func (p *AgentPanel) Queued() []string {
	var out []string
	for _, q := range p.cur.queue {
		out = append(out, q.text)
	}
	return out
}

// Unqueue removes the active tab's queued prompt i.
func (p *AgentPanel) Unqueue(i int, w *gpui.Window) {
	t := p.cur
	if i < 0 || i >= len(t.queue) {
		return
	}
	t.queue = append(t.queue[:i:i], t.queue[i+1:]...)
	if len(t.queue) == 0 {
		t.queuePaused = false
	}
	if w != nil {
		w.Refresh()
	}
}

// ResumeQueue sends the active tab's next queued prompt after a Cancel paused the queue.
func (p *AgentPanel) ResumeQueue(w *gpui.Window) {
	t := p.cur
	t.queuePaused = false
	if w != nil {
		p.win = w
	}
	p.nextQueued(t)
	if w != nil {
		w.Refresh()
	}
}

// nextQueued sends t's first queued prompt once no turn runs and the queue is not paused: on the
// live session, or by starting one (a model switch ends the session after the turn, so the next
// prompt starts it again).
func (p *AgentPanel) nextQueued(t *agentTab) {
	if t.queueDraining {
		return
	}
	t.queueDraining = true
	defer func() { t.queueDraining = false }()
	// A failed Prompt calls back here; the guard leaves its successor to this loop. Bound the
	// batch so a long queue of failed sends gives the UI a chance to handle input between ticks.
	for i := 0; i < agentEventBatch; i++ {
		if len(t.queue) == 0 || t.tr.Running() || t.queuePaused {
			return
		}
		q := t.queue[0]
		t.queue = t.queue[1:]
		p.userMessage(t, q.text, q.imgs)
		switch t.state {
		case agentReady:
			p.prompt(t, q.text, q.imgs)
		case agentStarting:
			t.queued, t.queuedImages = q.text, q.imgs
		case agentIdle:
			t.queued, t.queuedImages = q.text, q.imgs
			if p.win != nil {
				p.startTab(t, p.win)
			}
		}
	}
}

// dropQueue empties t's queue with a note, when its session ends.
func (p *AgentPanel) dropQueue(t *agentTab) {
	t.queuePaused = false
	if n := len(t.queue); n > 0 {
		t.queue = nil
		p.note(t, i18n.Plural("ui.agent.queue.dropped", n))
	}
}

// queueChips shows the active tab's queued prompts above the composer, each removable.
func (p *AgentPanel) queueChips() *gpui.Div {
	row := gpui.NewDiv().ID("agent-queue").FlexRow().FlexWrap().ItemsCenter().Gap(4)
	if p.cur.queuePaused {
		row.Child(gpui.NewDiv().ID("agent-queue-resume").FlexRow().ItemsCenter().Gap(4).Px_(8).Py_(2).Rounded(10).
			Bg(uiPrimary).Tooltip(i18n.T("ui.agent.queue.resume_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Opacity(0.88) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.ResumeQueue(w) }).
			Child(NewIcon(icons.AppSend, 12, uiOnPrimary), gpui.NewText(i18n.T("ui.agent.queue.resume")).TextSize(fontSmall).TextColor(uiOnPrimary)))
	}
	for i, q := range p.cur.queue {
		i := i
		label := clip(strings.ReplaceAll(q.text, "\n", " "), agentQueueChipChars)
		if n := len(q.imgs); n > 0 {
			label += fmt.Sprintf(" +%d", n)
		}
		x := gpui.NewDiv().ID(fmt.Sprintf("agent-queue-x-%d", i)).Px_(3).Rounded(3).Tooltip(i18n.T("ui.agent.queue.remove_tip")).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.Unqueue(i, w) }).
			Child(gpui.NewText("x").TextSize(11).TextColor(uiMuted))
		row.Child(gpui.NewDiv().ID(fmt.Sprintf("agent-queued-%d", i)).FlexRow().ItemsCenter().Gap(4).Pl(8).Pr(2).Py_(2).
			Rounded(10).Bg(uiPanelAlt).Border(1, uiBorder).MinW(0).Tooltip(i18n.T("ui.agent.queue.tip")).
			Child(gpui.NewText(fmt.Sprintf("%d. %s", i+1, label)).TextSize(fontSmall).MinW(0), x))
	}
	return row
}
