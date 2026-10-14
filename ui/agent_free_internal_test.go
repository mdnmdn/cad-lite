package ui

import (
	"slices"
	"testing"

	"agentmanager"
)

func modelIDs(ms []agentmanager.ModelInfo) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func TestFreeModel(t *testing.T) {
	for id, want := range map[string]bool{
		"opencode/big-pickle": true, "big-pickle": true, "opencode/grok-code-fast-1-free": true,
		"opencode/kimi-k2.5-free": true, "anthropic/claude-sonnet-4": false, "opencode/free-ish": false,
		"opencode/big-pickle-pro": false, "": false,
	} {
		if got := freeModel(id); got != want {
			t.Errorf("freeModel(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestOrderModelsPutsFreeFirstOnOpenCode(t *testing.T) {
	ms := []agentmanager.ModelInfo{{ID: "anthropic/claude-sonnet-4"}, {ID: "opencode/glm-4.7-free"}, {ID: "openai/gpt-5"},
		{ID: "opencode/big-pickle"}, {ID: "opencode/kimi-k2.5-free"}}
	want := []string{"opencode/big-pickle", "opencode/glm-4.7-free", "opencode/kimi-k2.5-free", "anthropic/claude-sonnet-4", "openai/gpt-5"}
	if got := modelIDs(orderModels(agentmanager.OpenCode, ms)); !slices.Equal(got, want) {
		t.Fatalf("opencode order %q", got)
	}
	if got := modelIDs(orderModels(agentmanager.Claude, ms)); !slices.Equal(got, modelIDs(ms)) {
		t.Fatalf("claude order changed: %q", got)
	}
	if ms[0].ID != "anthropic/claude-sonnet-4" {
		t.Fatal("orderModels sorted the agent's list in place")
	}
}

func TestSuggestedModel(t *testing.T) {
	m := &tabModel{}
	if s := suggestedModel(agentmanager.OpenCode, m); s != opencodeSuggested {
		t.Fatalf("no list: %q", s)
	}
	if s := suggestedModel(agentmanager.Codex, m); s != "" {
		t.Fatalf("codex suggestion %q", s)
	}
	m.offered = []agentmanager.ModelInfo{{ID: "openai/gpt-5"}, {ID: "opencode/glm-4.7-free"}}
	if s := suggestedModel(agentmanager.OpenCode, m); s != "opencode/glm-4.7-free" {
		t.Fatalf("first free: %q", s)
	}
	m.offered = append(m.offered, agentmanager.ModelInfo{ID: "opencode/big-pickle"})
	if s := suggestedModel(agentmanager.OpenCode, m); s != "opencode/big-pickle" {
		t.Fatalf("big-pickle first: %q", s)
	}
	m.offered = []agentmanager.ModelInfo{{ID: "openai/gpt-5"}}
	if s := suggestedModel(agentmanager.OpenCode, m); s != "" {
		t.Fatalf("no free model listed: %q", s)
	}
}
