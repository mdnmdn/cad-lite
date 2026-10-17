package acp

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

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

func opencodeConfigEnv(servers []agentmanager.MCPServer, env []string) ([]string, error) {
	content := os.Getenv("OPENCODE_CONFIG_CONTENT")
	for _, v := range env {
		if value, ok := strings.CutPrefix(v, "OPENCODE_CONFIG_CONTENT="); ok {
			content = value
		}
	}
	config := map[string]json.RawMessage{}
	if content != "" {
		if err := json.Unmarshal([]byte(content), &config); err != nil || config == nil {
			return nil, errors.New("acp: invalid inline OpenCode configuration")
		}
	}
	mcp := map[string]any{}
	if raw, ok := config["mcp"]; ok {
		if err := json.Unmarshal(raw, &mcp); err != nil || mcp == nil {
			return nil, errors.New("acp: invalid inline OpenCode MCP configuration")
		}
	}
	for _, m := range servers {
		e := map[string]any{"type": "remote", "url": m.URL, "enabled": true}
		if m.Token != "" {
			e["headers"] = map[string]string{"Authorization": "Bearer " + m.Token}
		}
		mcp[m.Name] = e
	}
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return nil, err
	}
	config["mcp"] = mcpJSON
	b, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	return []string{"OPENCODE_CONFIG_CONTENT=" + string(b)}, nil
}
