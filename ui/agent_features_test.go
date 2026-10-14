package ui_test

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"agentmanager"
	"agentmanager/transcript"
)

// clipRecorder stands in for the OS clipboard.
type clipRecorder struct {
	mu   sync.Mutex
	text []string
}

func (c *clipRecorder) write(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.text = append(c.text, s)
	return nil
}

func (c *clipRecorder) got() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.text)
}

// frame redraws the window.
func (r *agentRig) frame() {
	r.hw.Window().Refresh()
	r.hw.Pixels()
}

// reply plays an agent message and the end of the turn.
func (r *agentRig) reply(s *fakeSession, text string) {
	r.t.Helper()
	n := len(r.ws.AgentPanel().Transcript().Blocks())
	s.events <- agentmanager.TextDelta{MessageID: "m" + text[:min(4, len(text))], Text: text}
	s.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("reply", func() bool {
		return len(r.ws.AgentPanel().Transcript().Blocks()) > n && !r.ws.AgentPanel().Transcript().Running()
	})
	r.frame()
}

func TestAgentCopyButtonsCopySource(t *testing.T) {
	r := openAgent(t)
	clip := &clipRecorder{}
	r.ws.AgentPanel().SetClipboard(clip.write)
	r.start()
	r.say("draw **two** circles", 0)
	md := "# Done\n\nTwo circles, `r=5`:\n\n- one\n- two"
	r.reply(r.fs, md)
	// Block 0 is the user message, block 1 the agent's.
	r.click(r.pos("agent-copy-1"))
	r.click(r.pos("agent-copy-0"))
	r.until("clipboard", func() bool { return len(clip.got()) == 2 })
	if got := clip.got(); got[0] != md || got[1] != "draw **two** circles" {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestAgentLinkOpensURL(t *testing.T) {
	r := openAgent(t)
	var opened []string
	r.ws.AgentPanel().SetLinkOpener(func(u string) error { opened = append(opened, u); return nil })
	r.start()
	r.say("docs?", 0)
	r.reply(r.fs, "See [the manual](https://example.com/manual) for **details**.")
	r.click(r.pos("agent-md-1-link-1"))
	if len(opened) != 1 || opened[0] != "https://example.com/manual" {
		t.Fatalf("opened = %q", opened)
	}
}

func TestAgentToolParamsCollapsedUntilClicked(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("box", 0)
	r.fs.events <- agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "t1", Title: "cad add_rect",
		Kind: agentmanager.KindEdit, Status: agentmanager.ToolInProgress, RawInput: json.RawMessage(`{"x":0,"y":0,"w":10,"h":20}`)}}
	r.until("tool card", func() bool { return len(r.ws.AgentPanel().Transcript().Blocks()) == 2 })
	r.frame()
	if r.onScreen("agent-tool-params-1") {
		t.Fatal("tool parameters shown before the card was opened")
	}
	r.click(r.pos("agent-tool-1"))
	r.frame()
	if !r.onScreen("agent-tool-params-1") {
		t.Fatal("clicking the card header did not show the parameters")
	}
	r.click(r.pos("agent-tool-1"))
	r.frame()
	if r.onScreen("agent-tool-params-1") {
		t.Fatal("a second click did not hide the parameters")
	}
}

func TestAgentCancelReplacesSendWhileRunning(t *testing.T) {
	r := openAgent(t)
	r.start()
	if !r.onScreen("agent-send") || r.onScreen("agent-cancel") {
		t.Fatal("idle composer should show Send only")
	}
	r.say("hello", 0)
	r.frame()
	if r.onScreen("agent-send") || !r.onScreen("agent-cancel") {
		t.Fatal("while a turn runs the composer shows Cancel instead of Send")
	}
	r.click(r.pos("agent-cancel"))
	r.fs.mu.Lock()
	n := r.fs.cancels
	r.fs.mu.Unlock()
	if n != 1 || !r.ws.AgentPanel().Running() {
		t.Fatalf("cancels = %d, session live = %v; Cancel cancels the turn and keeps the session", n, r.ws.AgentPanel().Running())
	}
}

func TestAgentQueueSendsInOrderAsTurnsEnd(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	// Typing while the agent works turns the button into Queue, with Cancel still there.
	r.click(r.inputPos())
	r.typ("two")
	r.frame()
	if !r.onScreen("agent-send") || !r.onScreen("agent-cancel") {
		t.Fatal("with text typed during a turn, Queue and Cancel are both shown")
	}
	r.click(r.pos("agent-send"))
	r.typ("three")
	r.enter()
	r.typ("dropped later")
	r.enter()
	if got := p.Queued(); !slices.Equal(got, []string{"two", "three", "dropped later"}) {
		t.Fatalf("queue = %q", got)
	}
	r.frame()
	if !r.onScreen("agent-queued-0") || !r.onScreen("agent-queued-2") {
		t.Fatal("queued prompts are not shown as chips")
	}
	r.click(r.pos("agent-queue-x-2"))
	if got := p.Queued(); !slices.Equal(got, []string{"two", "three"}) {
		t.Fatalf("after removing the third chip queue = %q", got)
	}
	if got := r.fs.promptList(); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("prompts before the turn ended = %q", got)
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("second prompt", func() bool { return len(r.fs.promptList()) == 2 })
	if got := p.Queued(); !slices.Equal(got, []string{"three"}) {
		t.Fatalf("queue after one turn = %q", got)
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("third prompt", func() bool { return len(r.fs.promptList()) == 3 })
	if got := r.fs.promptList(); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("prompts = %q", got)
	}
	if len(p.Queued()) != 0 {
		t.Fatalf("queue not empty: %q", p.Queued())
	}
}

func TestAgentQueueDroppedWhenSessionEnds(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	if len(p.Queued()) != 1 {
		t.Fatalf("queue = %q", p.Queued())
	}
	r.fs.events <- agentmanager.Exited{}
	r.until("session end", func() bool { return !p.Running() })
	if len(p.Queued()) != 0 {
		t.Fatalf("queue survived the session: %q", p.Queued())
	}
	noted := false
	for _, b := range p.Transcript().Blocks() {
		if n, ok := b.(transcript.Note); ok && strings.Contains(n.Text, "1 queued prompt dropped") {
			noted = true
		}
	}
	if !noted {
		t.Fatal("no note about the dropped prompt")
	}
}

func TestAgentQueuedImagesGoWithTheirPrompt(t *testing.T) {
	r := openAgent(t)
	s := withImageSessions(r, true)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.AttachFiles([]string{pngFile(t, t.TempDir(), "a.png", 40, 30)}, r.hw.Window())
	p.Send("look", r.hw.Window())
	if len(p.Queued()) != 1 || len(p.Attachments()) != 0 {
		t.Fatalf("queue %q, composer images %d", p.Queued(), len(p.Attachments()))
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("queued prompt", func() bool { return len(r.fs.promptList()) == 2 })
	imgs := s.images()
	if len(imgs) == 0 || len(imgs[len(imgs)-1]) != 1 {
		t.Fatalf("images sent = %v", imgs)
	}
}

func TestAgentAICommandQueuesWhileRunning(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	r.click(r.pos("agent-collapse"))
	r.typ("AI")
	r.enter()
	r.typ("and a circle")
	r.enter()
	r.frame()
	if got := r.ws.AgentPanel().Queued(); !slices.Equal(got, []string{"and a circle"}) {
		t.Fatalf("AI command while busy: queue = %q", got)
	}
}

func TestAgentWorkingAnimationOnlyWhileRunning(t *testing.T) {
	r := openAgent(t)
	r.start()
	if r.onScreen("agent-working") {
		t.Fatal("animation shown while idle")
	}
	r.say("one", 0)
	r.frame()
	if !r.onScreen("agent-working") {
		t.Fatal("no animation while the turn runs")
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("turn end", func() bool { return !r.ws.AgentPanel().Transcript().Running() })
	r.frame()
	if r.onScreen("agent-working") {
		t.Fatal("animation still shown after the turn ended")
	}
}

func TestAgentUserMessageIsInset(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("hello", 0)
	r.reply(r.fs, "A plain reply that is long enough to fill the row.")
	tb := &tbRig{t: t, hw: r.hw}
	u, ok1 := tb.boundsAnywhere("agent-user-0")
	l, ok2 := tb.boundsAnywhere("agent-list")
	if !ok1 || !ok2 {
		t.Fatal("user message or list not on screen")
	}
	if inset := u.x0 - l.x0; inset < 40 || inset > 60 {
		t.Fatalf("user box inset %v from the list edge, want about 40 + padding", inset)
	}
}
