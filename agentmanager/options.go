package agentmanager

// PermissionPolicy may answer a request itself; ok=false sends it to the UI.
type PermissionPolicy func(PermissionRequest) (optionID string, ok bool)

// AllowMCP returns a PermissionPolicy that allows (allow_once preferred) requests whose call has MCPServer==server.
func AllowMCP(server string) PermissionPolicy {
	panic("todo: WP1")
}
