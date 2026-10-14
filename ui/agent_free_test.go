package ui_test

import (
	"testing"

	"agentmanager"
)

func TestOpenCodeTabStartsOnFreeModel(t *testing.T) {
	r := openAgent(t)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	p.Configure(agentmanager.OpenCode, "/nonexistent/opencode")
	w.Refresh()
	r.hw.Pixels()
	if m := p.Tabs()[0].Model; m != "big-pickle" {
		t.Fatalf("fresh opencode tab label %q", m)
	}
	r.click(r.pos("agent-model"))
	r.tick()
	for _, id := range []string{"agent-model-menu", "agent-model-default", "agent-model-opencode/big-pickle", "agent-model-opencode/big-pickle-free"} {
		if _, ok := (&tbRig{t: t, hw: r.hw}).boundsAnywhere(id); !ok {
			t.Fatalf("%s not in the model menu", id)
		}
	}
	r.click(r.pos("agent-model"))

	r.say("hi", 0)
	if cfg := r.nextCfg(); cfg.Model != "opencode/big-pickle" {
		t.Fatalf("started on %q", cfg.Model)
	}
	// The agent's list arrives: the choice stays and is stored with the conversation.
	r.fs.events <- agentmanager.ModelsChanged{Current: "anthropic/claude-sonnet-4", Available: []agentmanager.ModelInfo{
		{ID: "anthropic/claude-sonnet-4"}, {ID: "opencode/grok-code-fast-1-free"}}}
	r.until("models", func() bool { return len(p.Models()) == 2 })
	if m := p.Tabs()[0].Model; m != "big-pickle" {
		t.Fatalf("label after the list %q", m)
	}
	if ss := p.Sessions(); len(ss) != 1 || ss[0].Model != "opencode/big-pickle" {
		t.Fatalf("stored %+v", ss)
	}

	// Default, picked by the user, is the harness's own model, not the free one.
	p.NewTab(w)
	p.SetModel("", w)
	if m := p.Tabs()[1].Model; m != "default" {
		t.Fatalf("default picked: label %q", m)
	}
	r.say("hello", 1)
	if cfg := r.nextCfg(); cfg.Model != "" {
		t.Fatalf("default tab started on %q", cfg.Model)
	}
}

func TestOpenCodeSettingsDefaultBeatsFreeModel(t *testing.T) {
	r := openAgent(t)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	p.Configure(agentmanager.OpenCode, "/nonexistent/opencode")
	p.SetDefaultModel(agentmanager.OpenCode, "anthropic/claude-sonnet-4", w)
	if m := p.Tabs()[0].Model; m != "claude-sonnet-4" {
		t.Fatalf("label %q", m)
	}
	r.say("hi", 0)
	if cfg := r.nextCfg(); cfg.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("started on %q", cfg.Model)
	}
}

func TestClaudeTabGetsNoFreeModel(t *testing.T) {
	r := openAgent(t)
	if m := r.ws.AgentPanel().Tabs()[0].Model; m != "default" {
		t.Fatalf("claude label %q", m)
	}
}
