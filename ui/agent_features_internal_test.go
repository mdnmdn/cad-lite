package ui

import (
	"testing"
	"time"

	"agentmanager"
)

func TestAgentMarkdownParsedOncePerRevision(t *testing.T) {
	r := newLongTranscriptRig(t, 3)
	tab := r.p.cur
	last := 5*2 + 3 // the last turn's agent message
	c, ok := tab.md[last]
	if !ok || c.rev != tab.tr.Revision(last) {
		t.Fatalf("agent message not parsed and cached: %+v", c)
	}
	// A steady frame reuses the cached parse: plant a sentinel and see it survive.
	sentinel := []mdBlock{{kind: mdRule}}
	tab.md[last] = mdCached{rev: c.rev, blocks: sentinel}
	r.frame()
	if got := tab.md[last].blocks; len(got) != 1 || got[0].kind != mdRule {
		t.Fatal("steady frame re-parsed the message")
	}
	// More text bumps the revision and re-parses (the rig's last turn is still running, so end it:
	// while a turn runs the re-parse is throttled, see TestAgentStreamingParseIsThrottled).
	tab.tr.Apply(agentmanager.TextDelta{MessageID: "m2", Text: "\n\n**more**"})
	tab.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn})
	r.frame()
	if got := tab.md[last]; got.rev != tab.tr.Revision(last) || got.blocks[0].kind == mdRule {
		t.Fatalf("changed message not re-parsed: %+v", got)
	}
}

func TestAgentCopyFeedbackExpires(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	got := make(chan string, 1)
	r.p.clip = func(s string) error { got <- s; return nil }
	w := r.hw.Window()
	r.p.copyText(r.p.cur, 3, "text", w)
	if s := <-got; s != "text" {
		t.Fatalf("clipboard got %q", s)
	}
	r.frame()
	if !r.p.copiedNow(3) {
		t.Fatal("no check mark right after copying")
	}
	if err := r.plat.Advance(agentCopiedFor + 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	r.frame()
	if r.p.copiedNow(3) {
		t.Fatal("check mark still shown after it expired")
	}
}

func TestAgentCopyFailureIsReported(t *testing.T) {
	r := newLongTranscriptRig(t, 1)
	done := make(chan struct{})
	r.p.clip = func(string) error { defer close(done); return errNoClip }
	r.p.copyText(r.p.cur, 3, "text", r.hw.Window())
	<-done
	time.Sleep(10 * time.Millisecond)
	r.frame()
	if r.p.copiedNow(3) {
		t.Fatal("check mark kept after the clipboard write failed")
	}
}

type clipErr string

func (e clipErr) Error() string { return string(e) }

const errNoClip = clipErr("no clipboard tool")

func TestWorkingPhaseSteps(t *testing.T) {
	t0 := time.UnixMilli(1_000_000)
	seen := map[int]bool{}
	for k := 0; k < agentWorkingSteps; k++ {
		seen[workingPhase(t0.Add(time.Duration(k)*time.Second/agentWorkingFPS))] = true
	}
	if len(seen) != agentWorkingSteps {
		t.Fatalf("phases over one cycle = %d, want %d", len(seen), agentWorkingSteps)
	}
	if workingPhase(t0) != workingPhase(t0.Add(time.Second/agentWorkingFPS-time.Millisecond)) {
		t.Fatal("the pose changed within one step")
	}
	if workingX(0) != 0 || workingX(agentWorkingSteps/2) != 1 || workingX(agentWorkingSteps-1) >= 1 {
		t.Fatal("crosshair does not sweep out and back")
	}
}

// benchWorking draws frames of a panel whose turn is running, with the animation on or off.
func benchWorking(b *testing.B, on bool) {
	r := newLongTranscriptRig(b, 20)
	r.p.cur.tr.AddUser("working")
	r.p.cur.state = agentReady
	prev := agentWorkingAnim
	agentWorkingAnim = on
	b.Cleanup(func() { agentWorkingAnim = prev; r.p.cur.state = agentIdle })
	r.frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.frame()
	}
}

func BenchmarkAgentFrameWorkingAnimOn(b *testing.B)  { benchWorking(b, true) }
func BenchmarkAgentFrameWorkingAnimOff(b *testing.B) { benchWorking(b, false) }
