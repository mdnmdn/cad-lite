package claude_test

import (
	"context"
	"os"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/claude"
)

// TestLiveClaudeModels asks the installed claude CLI for its models and switches between them
// without sending a prompt (no tokens are spent). Run with AGENTMANAGER_LIVE_CLAUDE=1.
func TestLiveClaudeModels(t *testing.T) {
	if os.Getenv("AGENTMANAGER_LIVE_CLAUDE") != "1" {
		t.Skip("set AGENTMANAGER_LIVE_CLAUDE=1 to run against the real claude CLI")
	}
	s, err := claude.New().Open(context.Background(), agentmanager.Config{Harness: agentmanager.Claude, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mc agentmanager.ModelsChanged
	select {
	case ev := <-s.Events():
		var ok bool
		if mc, ok = ev.(agentmanager.ModelsChanged); !ok {
			t.Fatalf("first event %#v", ev)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no model list")
	}
	if len(mc.Available) == 0 {
		t.Fatal("empty model list")
	}
	t.Logf("models: %+v", mc.Available)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ms := s.(agentmanager.ModelSetter)
	if err := ms.SetModel(ctx, mc.Available[len(mc.Available)-1].ID); err != nil {
		t.Fatal(err)
	}
	if err := ms.SetModel(ctx, "no-such-model-xyz"); err == nil {
		t.Fatal("an unknown model was accepted")
	} else {
		t.Logf("unknown model: %v", err)
	}
}
