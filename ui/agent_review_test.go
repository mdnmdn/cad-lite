package ui_test

import (
	"errors"
	"slices"
	"testing"

	"agentmanager"
	"cad-lite/assets/icons"
	"cad-lite/ui"
)

// Regressions from the agent-side review.

// H1: the header Stop during a turn must end the turn, so a later Send starts a session instead of
// queueing forever.
func TestAgentHeaderStopMidTurnEndsTurn(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Stop(r.hw.Window())
	if p.Transcript().Running() {
		t.Fatal("Stop during a turn left the turn running")
	}
	p.Send("two", r.hw.Window())
	if len(p.Queued()) != 0 {
		t.Fatalf("Send after Stop was queued: %q", p.Queued())
	}
	r.until("new session", func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.more) == 1 })
}

// H1: the event channel closing mid-turn ends the turn too.
func TestAgentEventsClosedMidTurnEndsTurn(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	r.fs.Close()
	r.until("session end", func() bool { return !p.Running() })
	if p.Transcript().Running() {
		t.Fatal("closed event channel left the turn running")
	}
	if len(p.Queued()) != 0 {
		t.Fatalf("queue survived the session: %q", p.Queued())
	}
}

// H2: a failed prompt moves the queue on, and a direct Send never overtakes queued prompts.
func TestAgentFailedPromptKeepsQueueOrder(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	p.Send("three", r.hw.Window())
	r.fs.mu.Lock()
	r.fs.promptErr = errors.New("boom")
	r.fs.mu.Unlock()
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	// "two" fails, so "three" goes next (and fails too, the error is still set).
	r.until("queue drained", func() bool { return len(r.fs.promptList()) == 3 })
	if len(p.Queued()) != 0 || p.Transcript().Running() {
		t.Fatalf("queue %q, running %v after failed prompts", p.Queued(), p.Transcript().Running())
	}
	r.fs.mu.Lock()
	r.fs.promptErr = nil
	r.fs.mu.Unlock()
	p.Send("four", r.hw.Window())
	if got := r.fs.promptList(); !slices.Equal(got, []string{"one", "two", "three", "four"}) {
		t.Fatalf("prompts = %q", got)
	}
}

// H2: with prompts still queued (paused), a direct Send appends and the order holds.
func TestAgentDirectSendAppendsToQueue(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	p.Cancel(r.hw.Window())
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopCancelled}
	r.until("turn end", func() bool { return !p.Transcript().Running() })
	p.Send("three", r.hw.Window())
	r.until("two sent", func() bool { return len(r.fs.promptList()) == 2 })
	if got := r.fs.promptList(); !slices.Equal(got, []string{"one", "two"}) || !slices.Equal(p.Queued(), []string{"three"}) {
		t.Fatalf("prompts %q, queue %q", got, p.Queued())
	}
}

// L1: a duplicate TurnEnd does not send two queued prompts.
func TestAgentDuplicateTurnEndSendsOne(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	p.Send("three", r.hw.Window())
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("events drained", func() bool { return len(r.fs.events) == 0 })
	for i := 0; i < 10; i++ {
		r.tick()
	}
	if got := r.fs.promptList(); !slices.Equal(got, []string{"one", "two"}) || !slices.Equal(p.Queued(), []string{"three"}) {
		t.Fatalf("prompts %q, queue %q", got, p.Queued())
	}
}

// M4: Cancel with prompts queued pauses the queue until the user resumes it.
func TestAgentCancelPausesQueueUntilResumed(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("one", 0)
	p := r.ws.AgentPanel()
	p.Send("two", r.hw.Window())
	r.frame()
	r.click(r.pos("agent-cancel"))
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopCancelled}
	r.until("turn end", func() bool { return !p.Transcript().Running() })
	for i := 0; i < 10; i++ {
		r.tick()
	}
	r.frame()
	if got := r.fs.promptList(); len(got) != 1 || !slices.Equal(p.Queued(), []string{"two"}) {
		t.Fatalf("after Cancel: prompts %q, queue %q", got, p.Queued())
	}
	r.click(r.pos("agent-queue-resume"))
	r.until("resumed", func() bool { return len(r.fs.promptList()) == 2 })
	if len(p.Queued()) != 0 {
		t.Fatalf("queue after resume: %q", p.Queued())
	}
}

func TestAgentHeaderStopHasDistinctIcon(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.say("hello", 0)
	r.frame()
	bounds, ok := (&tbRig{t: t, hw: r.hw}).boundsAnywhere("agent-stop")
	if !ok {
		t.Fatal("header Stop absent")
	}
	stop, cancel := ui.IconMask(icons.AppStop, 14), ui.IconMask(icons.AppCancel, 14)
	if stop == nil || cancel == nil || slices.Equal(stop.Pix, cancel.Pix) {
		t.Fatal("Stop and Cancel must have distinct rasterized icons")
	}
	found := false
	for _, sprite := range r.hw.Window().Scene().Sprites {
		if sprite.Mask == stop && float32(sprite.Bounds.Origin.X) >= bounds.x0 &&
			float32(sprite.Bounds.Origin.X+sprite.Bounds.Size.W) <= bounds.x1 &&
			float32(sprite.Bounds.Origin.Y) >= bounds.y0 &&
			float32(sprite.Bounds.Origin.Y+sprite.Bounds.Size.H) <= bounds.y1 {
			found = true
		}
	}
	if !found {
		t.Fatal("header Stop did not paint its square icon")
	}
	r.snap("stop-cancel.png")
	r.click(r.pos("agent-stop"))
	if r.ws.AgentPanel().Running() || r.ws.AgentPanel().Transcript().Running() {
		t.Fatal("header Stop must still end the session and turn")
	}
}
