package acp

import (
	"encoding/json"
	"fmt"

	"agentmanager"
	"agentmanager/jsonrpc"
)

// idKey keys the outstanding-permission table so numeric 7 and string "7" differ.
func idKey(id json.RawMessage) string {
	var str string
	if json.Unmarshal(id, &str) == nil {
		return "s:" + str
	}
	return "n:" + string(id)
}

type permissionParams struct {
	ToolCall toolFields `json:"toolCall"`
	Options  []struct {
		OptionID string `json:"optionId"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
	} `json:"options"`
}

func selected(optionID string) map[string]any {
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}

var cancelledOutcome = map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}

// handlePermission answers via the policy, or parks the request until RespondPermission.
func (s *session) handlePermission(c *jsonrpc.Conn, id, params json.RawMessage) {
	var p permissionParams
	if err := json.Unmarshal(params, &p); err != nil {
		_ = c.ReplyError(id, jsonrpc.CodeInvalidParams, "bad session/request_permission params")
		return
	}
	req := agentmanager.PermissionRequest{RequestID: idKey(id), Call: s.patchFor(p.ToolCall)}
	for _, o := range p.Options {
		req.Options = append(req.Options, agentmanager.PermissionOption{ID: o.OptionID, Name: o.Name, Kind: agentmanager.PermissionKind(o.Kind)})
	}
	if pol := s.cfg.AutoApprove; pol != nil {
		if opt, ok := pol(req); ok {
			_ = c.Reply(id, selected(opt))
			title := req.Call.ID
			if req.Call.Title != nil {
				title = *req.Call.Title
			}
			s.logEvent("info", fmt.Sprintf("auto-approved %s (option %s)", title, opt))
			return
		}
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = c.Reply(id, cancelledOutcome)
		return
	}
	s.perms[req.RequestID] = append(json.RawMessage(nil), id...)
	s.mu.Unlock()
	s.q.Push(req)
}

// RespondPermission implements agentmanager.Session.
func (s *session) RespondPermission(requestID string, ans agentmanager.PermissionAnswer) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	id, ok := s.perms[requestID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("acp: no pending permission request %q", requestID)
	}
	if !ans.Cancelled && ans.OptionID == "" {
		s.mu.Unlock()
		return fmt.Errorf("acp: permission answer needs an option id or Cancelled")
	}
	delete(s.perms, requestID)
	s.mu.Unlock()
	if ans.Cancelled {
		return s.conn.Reply(id, cancelledOutcome)
	}
	return s.conn.Reply(id, selected(ans.OptionID))
}

// cancelPermissions replies cancelled to every outstanding request.
func (s *session) cancelPermissions() {
	s.mu.Lock()
	ids := make([]json.RawMessage, 0, len(s.perms))
	for _, id := range s.perms {
		ids = append(ids, id)
	}
	s.perms = map[string]json.RawMessage{}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.conn.Reply(id, cancelledOutcome)
	}
}
