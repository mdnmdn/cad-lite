package ui

import (
	"fmt"
	"time"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// mdCached is a block's parsed Markdown, the transcript revision it was parsed at and when.
type mdCached struct {
	rev    uint64
	at     time.Time
	blocks []mdBlock
}

// agentParseEvery is how often a message still streaming in is re-parsed: each delta would
// otherwise re-parse the whole message.
const agentParseEvery = 100 * time.Millisecond

// agentCopiedFor is how long a copy button shows its check mark.
const agentCopiedFor = 1500 * time.Millisecond

// markdown returns block i's parsed text, parsing only when the block changed since; while a turn
// runs, at most once per agentParseEvery (at frame time now), so a streaming message shows its
// newest text a little late rather than costing a full parse per delta. A finished turn catches up
// at once.
func (t *agentTab) markdown(i int, text string, now time.Time) []mdBlock {
	rev := t.tr.Revision(i)
	c, ok := t.md[i]
	if ok && (c.rev == rev || (t.tr.Running() && now.Sub(c.at) < agentParseEvery)) {
		return c.blocks
	}
	bs := parseMarkdown(text)
	t.md[i] = mdCached{rev: rev, at: now, blocks: bs}
	return bs
}

// markdownOf is the active tab's block i parsed, at the current frame time.
func (p *AgentPanel) markdownOf(i int, text string) []mdBlock {
	var now time.Time
	if p.win != nil {
		now = p.win.FrameTime()
	}
	return p.cur.markdown(i, text, now)
}

// mdView returns the renderer for block i of the active tab.
func (p *AgentPanel) mdView(i int, size float32, muted bool) *mdView {
	return &mdView{id: fmt.Sprintf("agent-md-%d", i), size: size, muted: muted, open: p.openLink}
}

// openLink opens a transcript link; a link that cannot be opened (not https, say) is reported in
// the transcript.
func (p *AgentPanel) openLink(url string) {
	if p.openURL == nil {
		return
	}
	if hasBidi(url) {
		p.note(p.cur, i18n.T("ui.agent.link_failed", url))
		return
	}
	if err := p.openURL(url); err != nil {
		p.note(p.cur, i18n.T("ui.agent.link_failed", url))
		if p.win != nil {
			p.win.Refresh()
		}
	}
}

// copyButton is a message's copy button: it copies text (the message's source) to the clipboard
// and shows a check mark for agentCopiedFor.
func (p *AgentPanel) copyButton(i int, text string) *gpui.Div {
	t := p.cur
	icon, col, tip := icons.AppCopy, uiMuted, i18n.T("ui.agent.copy_tip")
	if p.copiedNow(i) {
		icon, col, tip = icons.AppCheck, uiOK, i18n.T("ui.agent.copied")
	}
	return gpui.NewDiv().ID(fmt.Sprintf("agent-copy-%d", i)).FlexRow().ItemsCenter().JustifyCenter().SizeWH(20, 20).
		Rounded(4).FlexShrink(0).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.copyText(t, i, text, w) }).
		Child(NewIcon(icon, 14, col))
}

// copiedNow reports whether block i of the active tab was copied less than agentCopiedFor ago.
func (p *AgentPanel) copiedNow(i int) bool {
	at, ok := p.cur.copied[i]
	return ok && p.win != nil && p.win.FrameTime().Sub(at) < agentCopiedFor
}

// copyText writes text to the clipboard off the UI goroutine (the OS tool can block) and starts
// the check-mark feedback. A failed write reaches clipErr, which the next frame reports in the
// tab's transcript (the feedback timer guarantees that frame).
func (p *AgentPanel) copyText(t *agentTab, i int, text string, w *gpui.Window) {
	if p.clip == nil {
		return
	}
	clip, errs := p.clip, p.clipErrs()
	seq := p.clipSeq.Add(1)
	p.clipBusy.Add(1)
	go func() {
		defer p.clipBusy.Add(-1)
		// Writes run one at a time; one that a later click superseded while it waited is skipped,
		// so the clipboard ends with the last text copied.
		p.clipMu.Lock()
		defer p.clipMu.Unlock()
		if p.clipSeq.Load() != seq {
			return
		}
		if err := clip(text); err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()
	t.copied[i] = w.FrameTime()
	w.After(agentCopiedFor, func(*gpui.Window, *gpui.App) {})
	w.Refresh()
}

// clipIdle reports whether no clipboard write is pending or running.
func (p *AgentPanel) clipIdle() bool { return p.clipBusy.Load() == 0 }

// clipErrs returns the channel failed clipboard writes report on.
func (p *AgentPanel) clipErrs() chan error {
	if p.clipErr == nil {
		p.clipErr = make(chan error, 1)
	}
	return p.clipErr
}

// reportClipboard notes a failed clipboard write in the active tab, on the UI goroutine.
func (p *AgentPanel) reportClipboard() {
	select {
	case err := <-p.clipErrs():
		clear(p.cur.copied)
		p.note(p.cur, i18n.T("ui.agent.copy_failed", err))
	default:
	}
}
