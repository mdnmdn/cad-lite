package settings

import (
	"strings"
	"testing"
)

func TestAgentModelsRoundTrip(t *testing.T) {
	s, warn, err := Parse([]byte(`
[agent]
default = "claude"
model_claude = "opus"
model_codex = ""
model_opencode = 3
other = 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.AgentModels["claude"] != "opus" || s.AgentModels["codex"] != "" || len(s.AgentModels) != 2 {
		t.Fatalf("models %v", s.AgentModels)
	}
	if len(warn) != 1 || !strings.Contains(warn[0], "agent.model_opencode") {
		t.Fatalf("warnings %v", warn)
	}
	if s.Extra["agent.other"] == nil {
		t.Fatalf("unknown agent key lost: %v", s.Extra)
	}
	c := s.Clone()
	c.AgentModels["claude"] = "haiku"
	if s.AgentModels["claude"] != "opus" {
		t.Fatal("Clone shares AgentModels")
	}
	c.AgentModels["bad key"] = "x"
	out := string(Marshal(c))
	if !strings.Contains(out, `model_claude = "haiku"`) || strings.Contains(out, "model_codex") || strings.Contains(out, "bad key") {
		t.Fatalf("marshalled:\n%s", out)
	}
	back, warn, err := Parse([]byte(out))
	if err != nil || len(warn) != 0 || back.AgentModels["claude"] != "haiku" || back.DefaultAgent != "claude" {
		t.Fatalf("round trip %v %v %v", back.AgentModels, warn, err)
	}
}
