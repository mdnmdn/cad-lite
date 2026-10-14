package acp

import (
	"encoding/json"

	"agentmanager"
)

// OpenCode returns the Opener for `opencode acp`.
//
// The argv is exactly `acp` plus Config.Args; models are chosen over the wire. Permissions are real
// round trips (no skip flag). MCP servers go in session/new; when the agent does not advertise http
// MCP support they are injected through OPENCODE_CONFIG_CONTENT instead (process restarted once).
func OpenCode() agentmanager.Opener {
	return opener{
		cmd:      Command{Path: "opencode", Args: []string{"acp"}, Env: []string{"OPENCODE_DISABLE_CLAUDE_CODE=1"}},
		authHint: "authenticate the CLI first (`opencode auth login`)",
		envMCP:   opencodeConfigEnv,
	}
}

func opencodeConfigEnv(servers []agentmanager.MCPServer) ([]string, error) {
	mcp := map[string]any{}
	for _, m := range servers {
		e := map[string]any{"type": "remote", "url": m.URL, "enabled": true}
		if m.Token != "" {
			e["headers"] = map[string]string{"Authorization": "Bearer " + m.Token}
		}
		mcp[m.Name] = e
	}
	b, err := json.Marshal(map[string]any{"mcp": mcp})
	if err != nil {
		return nil, err
	}
	return []string{"OPENCODE_CONFIG_CONTENT=" + string(b)}, nil
}
