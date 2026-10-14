package acp

import (
	"testing"

	"agentmanager"
)

func TestMCPOfStrict(t *testing.T) {
	s := &session{cfg: agentmanager.Config{MCPServers: []agentmanager.MCPServer{{Name: "cad"}}}}
	k := func(v agentmanager.ToolKind) *agentmanager.ToolKind { return &v }
	for _, c := range []struct {
		title string
		kind  *agentmanager.ToolKind
		want  string
	}{
		{"cad_add_line", k(agentmanager.KindOther), "cad"},
		{"cad_add_line", nil, "cad"},
		{"mcp__cad__add_line", nil, "cad"},
		{"cad/build.sh --all", nil, ""},
		{"cad.sh", nil, ""},
		{"cad:rm -rf ~", nil, ""},
		{"cad_helper --wipe", nil, ""},
		{"cad_Add", nil, ""},
		{"cad_", nil, ""},
		{"cad_build", k(agentmanager.KindExecute), ""},
		{"cad_build", k(agentmanager.KindFetch), ""},
	} {
		if got, _ := s.mcpOf(c.title, c.kind); got != c.want {
			t.Errorf("%q kind %v: server %q, want %q", c.title, c.kind, got, c.want)
		}
	}
}
