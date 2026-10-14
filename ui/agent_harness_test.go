package ui_test

import (
	"os"
	"path/filepath"
	"testing"
)

// onScreen reports whether an element with the given id is hit anywhere in the window.
func (r *agentRig) onScreen(id string) bool {
	_, ok := (&tbRig{t: r.t, hw: r.hw}).boundsAnywhere(id)
	return ok
}

// snap saves the frame under _tmp/harness for a visual check.
func (r *agentRig) snap(name string) {
	r.t.Helper()
	if err := os.MkdirAll("../_tmp/harness", 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := r.hw.SavePNG(filepath.Join("../_tmp/harness", name)); err != nil {
		r.t.Fatal(err)
	}
}

func TestAgentHarnessChipsUntilStarted(t *testing.T) {
	r := openAgent(t)
	if !r.onScreen("agent-pick-claude") || r.onScreen("agent-harness") {
		t.Fatal("a fresh tab shows the harness chips and no harness in the header")
	}
	r.snap("fresh.png")

	r.start()
	r.hw.Window().Refresh()
	r.hw.Pixels()
	if r.onScreen("agent-pick-claude") || r.onScreen("agent-pick-codex") {
		t.Fatal("a live tab still shows the harness chips")
	}
	if !r.onScreen("agent-harness") {
		t.Fatal("a live tab does not name its harness in the header")
	}
	if !r.onScreen("agent-stop") || !r.onScreen("agent-model") {
		t.Fatal("the row lost the Stop button or the model chip")
	}
	r.snap("live.png")
}

func TestAgentRestoredTabHidesHarnessChips(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	r.say("hello", 0)
	r.nextCfg()
	r.turn(r.fs, "claude-sid-1", "hi there")
	r.until("reply", func() bool { return r.hasText(0, "hi there") && !r.ws.AgentPanel().Transcript().Running() })
	r.ws.CloseAgent()

	r2 := newRig(t, dir, nil, true)
	r2.hw.Window().Refresh()
	r2.hw.Pixels()
	if r2.ws.AgentPanel().Running() {
		t.Fatal("restoring started a session")
	}
	if r2.onScreen("agent-pick-claude") {
		t.Fatal("a restored tab shows the harness chips")
	}
	if !r2.onScreen("agent-harness") || !r2.onScreen("agent-start") {
		t.Fatal("a restored tab lacks the harness in the header or the Start button")
	}
	r2.snap("restored.png")
}
