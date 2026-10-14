package agentmanager

import "strings"

// PermissionPolicy may answer a request itself; ok=false sends it to the UI.
type PermissionPolicy func(PermissionRequest) (optionID string, ok bool)

// AllowMCP returns a PermissionPolicy that allows (allow_once preferred) requests whose call
// belongs to the MCP server named server. The backend-set Call.MCPServer is authoritative: when it
// is set only an exact match is allowed. Without it, only the exact Claude title "mcp__server__tool"
// qualifies. A call whose Kind is anything but other or read (execute, edit, delete, move, fetch,
// ...) is never auto-allowed, whatever its title says.
func AllowMCP(server string) PermissionPolicy {
	prefix := "mcp__" + server + "__"
	return func(r PermissionRequest) (string, bool) {
		if server == "" {
			return "", false
		}
		if k := r.Call.Kind; k != nil && *k != KindOther && *k != KindRead {
			return "", false
		}
		if r.Call.MCPServer != "" {
			if r.Call.MCPServer != server {
				return "", false
			}
		} else {
			t := ""
			if r.Call.Title != nil {
				t = *r.Call.Title
			}
			tool, ok := strings.CutPrefix(t, prefix)
			if !ok || !validTool(tool) {
				return "", false
			}
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

// validTool reports whether s is a non-empty bare MCP tool name (no spaces or separators).
func validTool(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
