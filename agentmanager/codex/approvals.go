package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"agentmanager"
	"agentmanager/jsonrpc"
)

// ask is a parked server->client approval request.
type ask struct {
	id      json.RawMessage
	method  string
	params  map[string]any
	options []agentmanager.PermissionOption
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func opt(id, name string, k agentmanager.PermissionKind) agentmanager.PermissionOption {
	return agentmanager.PermissionOption{ID: id, Name: name, Kind: k}
}

func allowDeny() []agentmanager.PermissionOption {
	return []agentmanager.PermissionOption{
		opt("accept", "Allow", agentmanager.AllowOnce),
		opt("acceptForSession", "Allow for this session", agentmanager.AllowAlways),
		opt("decline", "Deny", agentmanager.RejectOnce),
	}
}

// HandleRequest implements jsonrpc.Handler; it runs on the reader goroutine.
func (s *session) HandleRequest(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
	p := map[string]any{}
	_ = json.Unmarshal(params, &p)
	a := &ask{id: id, method: method, params: p}
	var title string
	var kind agentmanager.ToolKind
	server := ""
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		title = str(p, "command")
		if argv, ok := p["command"].([]any); ok {
			parts := make([]string, 0, len(argv))
			for _, x := range argv {
				if v, ok := x.(string); ok {
					parts = append(parts, v)
				}
			}
			title = strings.Join(parts, " ")
		}
		if title == "" {
			title = "Run a command"
		}
		kind, a.options = agentmanager.KindExecute, allowDeny()
	case "item/fileChange/requestApproval", "applyPatchApproval":
		title = "Apply a file change"
		if r := str(p, "grantRoot"); r != "" {
			title = "Allow writes under " + r
		}
		kind, a.options = agentmanager.KindEdit, allowDeny()
	case "item/permissions/requestApproval":
		title = "Grant permissions"
		if r := str(p, "reason"); r != "" {
			title += ": " + r
		}
		kind, a.options = agentmanager.KindOther, allowDeny()
	case "mcpServer/elicitation/request":
		if !elicitationNeedsNoContent(p) {
			_ = c.Reply(id, a.result(""))
			return true
		}
		title = str(p, "message")
		if title == "" {
			title = "An MCP server asks to continue"
		}
		server = str(p, "serverName")
		kind = agentmanager.KindOther
		a.options = []agentmanager.PermissionOption{
			opt("accept", "Allow", agentmanager.AllowOnce),
			opt("decline", "Deny", agentmanager.RejectOnce),
		}
	case "item/tool/requestUserInput":
		_ = c.Reply(id, map[string]any{"answers": map[string]any{}})
		return true
	default:
		return false
	}

	itemID := str(p, "itemId")
	if itemID == "" {
		itemID = str(p, "callId")
	}
	key := string(id)
	if itemID == "" {
		itemID = key
	}
	raw, _ := json.Marshal(p)
	st := agentmanager.ToolPending
	req := agentmanager.PermissionRequest{
		RequestID: key,
		Call: agentmanager.ToolCallPatch{
			ID: itemID, Title: &title, MCPServer: server, Kind: &kind, Status: &st, RawInput: raw,
		},
		Options: a.options,
	}
	if s.policy != nil {
		if optID, ok := s.policy(req); ok && a.hasOption(optID) {
			_ = c.Reply(id, a.result(optID))
			return true
		}
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = c.Reply(id, a.result(""))
		return true
	}
	s.asks[key] = a
	s.mu.Unlock()
	s.q.Push(req)
	return true
}

func (a *ask) hasOption(id string) bool {
	for _, o := range a.options {
		if o.ID == id {
			return true
		}
	}
	return false
}

func elicitationNeedsNoContent(p map[string]any) bool {
	switch str(p, "mode") {
	case "url":
		return true
	case "form":
		schema, _ := p["requestedSchema"].(map[string]any)
		req, _ := schema["required"].([]any)
		return len(req) == 0
	}
	return false
}

// result builds the reply for the ask; choice is the option id, "" cancels.
func (a *ask) result(choice string) any {
	switch a.method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		d := "cancel"
		switch choice {
		case "accept", "acceptForSession", "decline":
			d = choice
		}
		return map[string]any{"decision": d}
	case "execCommandApproval", "applyPatchApproval":
		var d any = "abort"
		switch choice {
		case "accept":
			d = "approved"
		case "acceptForSession":
			d = "approved_for_session"
		case "decline":
			d = map[string]any{"denied": map[string]any{"rejection": "declined by the user"}}
		}
		return map[string]any{"decision": d}
	case "item/permissions/requestApproval":
		granted := map[string]any{}
		if choice == "accept" || choice == "acceptForSession" {
			for _, k := range []string{"network", "fileSystem"} {
				if perms, ok := a.params["permissions"].(map[string]any); ok && perms[k] != nil {
					granted[k] = perms[k]
				}
			}
		}
		scope := "turn"
		if choice == "acceptForSession" {
			scope = "session"
		}
		return map[string]any{"permissions": granted, "scope": scope}
	case "mcpServer/elicitation/request":
		action := "cancel"
		if choice == "accept" || choice == "decline" {
			action = choice
		}
		var content any
		if str(a.params, "mode") == "form" && action == "accept" {
			content = map[string]any{}
		}
		return map[string]any{"action": action, "content": content, "_meta": nil}
	}
	return map[string]any{}
}

// RespondPermission implements agentmanager.Session.
func (s *session) RespondPermission(requestID string, ans agentmanager.PermissionAnswer) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	a := s.asks[requestID]
	if a == nil {
		s.mu.Unlock()
		return fmt.Errorf("codex: no pending permission request %q", requestID)
	}
	choice := ans.OptionID
	if ans.Cancelled {
		choice = ""
	} else if !a.hasOption(choice) {
		s.mu.Unlock()
		return fmt.Errorf("codex: unknown option %q for request %q", choice, requestID)
	}
	delete(s.asks, requestID)
	s.mu.Unlock()
	if err := s.conn.Reply(a.id, a.result(choice)); err != nil {
		if errors.Is(err, jsonrpc.ErrClosed) {
			return agentmanager.ErrClosed
		}
		return err
	}
	return nil
}
