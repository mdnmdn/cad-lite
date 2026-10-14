package agentmanager

import "strings"

// PermissionPolicy may answer a request itself; ok=false sends it to the UI.
type PermissionPolicy func(PermissionRequest) (optionID string, ok bool)

// AllowMCP returns a PermissionPolicy that allows (allow_once preferred) requests whose call
// belongs to the MCP server named server: Call.MCPServer==server when the backend set it,
// otherwise a Title in the naming the backends produce: "mcp__server__tool" (Claude),
// "server_tool", "server.tool", "server:tool" or "server/tool" (ACP agents, Codex).
func AllowMCP(server string) PermissionPolicy {
	prefixes := []string{"mcp__" + server + "__"}
	for _, sep := range []string{"_", ".", ":", "/"} {
		prefixes = append(prefixes, server+sep)
	}
	return func(r PermissionRequest) (string, bool) {
		if server == "" {
			return "", false
		}
		match := r.Call.MCPServer == server
		t := ""
		if r.Call.Title != nil {
			t = strings.TrimSpace(*r.Call.Title)
		}
		for _, p := range prefixes {
			if !match && t != "" && strings.HasPrefix(t, p) {
				match = true
				break
			}
		}
		if !match {
			return "", false
		}
		for _, want := range []PermissionKind{AllowOnce, AllowAlways} {
			for _, o := range r.Options {
				if o.Kind == want {
					return o.ID, true
				}
			}
		}
		return "", false
	}
}
