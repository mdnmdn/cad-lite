package claude

import (
	"encoding/json"
	"os"

	"agentmanager"
)

type mcpEntry struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// writeMCPConfig writes {"mcpServers":{...}} to a 0600 temp file (a token in argv would show in
// ps) and returns its path.
func writeMCPConfig(servers []agentmanager.MCPServer) (string, error) {
	m := make(map[string]mcpEntry, len(servers))
	for _, s := range servers {
		e := mcpEntry{Type: "http", URL: s.URL}
		if s.Token != "" {
			e.Headers = map[string]string{"Authorization": "Bearer " + s.Token}
		}
		m[s.Name] = e
	}
	b, err := json.Marshal(map[string]any{"mcpServers": m})
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "agentmanager-claude-mcp-*.json")
	if err != nil {
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
