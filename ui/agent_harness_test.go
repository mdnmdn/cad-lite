package ui_test

import (
	"agentmanager"
	"cad-lite/settings"
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
	c := r.ws.Settings()
	c.AgentMode = settings.AgentAdvanced
	c.DefaultAgent = "claude"
	r.ws.SetSettings(c, r.hw.Window())
	r.ws.AgentPanel().Configure(agentmanager.Claude, "/nonexistent/claude")
	r.hw.Pixels()
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

func TestAgentHistoryTabHidesHarnessChips(t *testing.T) {
	r := openAgent(t)
	r.chat(0, "hello", "claude-sid-1", "hi there")
	p, w := r.ws.AgentPanel(), r.hw.Window()
	id := p.Tabs()[0].SessionID
	p.CloseTab(0, w)
	p.OpenSession(id, w)
	r.hw.Pixels()
	if p.Running() || r.onScreen("agent-pick-claude") {
		t.Fatal("opening history started a process or showed harness chips")
	}
	if !r.onScreen("agent-harness") || !r.onScreen("agent-start") {
		t.Fatal("history tab lacks harness or Start")
	}
	r.snap("reopened-history.png")
}
