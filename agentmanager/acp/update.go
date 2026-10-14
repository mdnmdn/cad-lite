package acp

import (
	"encoding/json"
	"strings"

	"agentmanager"
	"agentmanager/jsonrpc"
)

// HandleRequest implements jsonrpc.Handler. Everything but permission asks is method-not-found
// (the Conn replies), so fs/*, terminal/* and vendor methods each get exactly one error reply.
func (s *session) HandleRequest(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
	if method == "session/request_permission" {
		s.handlePermission(c, id, params)
		return true
	}
	return false
}

// HandleNotification implements jsonrpc.Handler.
func (s *session) HandleNotification(_ *jsonrpc.Conn, method string, params json.RawMessage) {
	if method == "session/update" {
		s.handleUpdate(params)
	}
}

type toolFields struct {
	ToolCallID string          `json:"toolCallId"`
	Title      *string         `json:"title"`
	Kind       *string         `json:"kind"`
	Status     *string         `json:"status"`
	Content    json.RawMessage `json:"content"`
	RawInput   json.RawMessage `json:"rawInput"`
	RawOutput  json.RawMessage `json:"rawOutput"`
}

type updateFields struct {
	toolFields
	SessionUpdate string `json:"sessionUpdate"`
	MessageID     string `json:"messageId"`
	Entries       []struct {
		Content string `json:"content"`
		Status  string `json:"status"`
	} `json:"entries"`
}

func (s *session) handleUpdate(params json.RawMessage) {
	s.mu.Lock()
	loading := s.loading
	s.mu.Unlock()
	if loading {
		return // history replay from session/load
	}
	var wrap struct {
		Update json.RawMessage `json:"update"`
	}
	raw := params
	if json.Unmarshal(params, &wrap) == nil && len(wrap.Update) > 0 {
		raw = wrap.Update
	}
	var u updateFields
	if json.Unmarshal(raw, &u) != nil {
		return
	}
	switch u.SessionUpdate {
	case "agent_message_chunk", "agent_thought_chunk":
		var ch struct {
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &ch) != nil || ch.Content.Type != "text" || ch.Content.Text == "" {
			return
		}
		if u.SessionUpdate == "agent_message_chunk" {
			s.q.Push(agentmanager.TextDelta{MessageID: u.MessageID, Text: ch.Content.Text})
		} else {
			s.q.Push(agentmanager.ThoughtDelta{MessageID: u.MessageID, Text: ch.Content.Text})
		}
	case "tool_call":
		s.toolStart(u.toolFields)
	case "tool_call_update":
		s.q.Push(agentmanager.ToolCallUpdate{Update: s.patchFor(u.toolFields)})
	case "plan":
		entries := make([]agentmanager.PlanEntry, 0, len(u.Entries))
		for _, e := range u.Entries {
			st := e.Status
			if st != "in_progress" && st != "completed" {
				st = "pending"
			}
			entries = append(entries, agentmanager.PlanEntry{Content: e.Content, Status: st})
		}
		s.q.Push(agentmanager.PlanUpdate{Entries: entries})
	}
}

func normKind(k *string) *agentmanager.ToolKind {
	if k == nil {
		return nil
	}
	v := agentmanager.ToolKind(*k)
	switch v {
	case agentmanager.KindRead, agentmanager.KindEdit, agentmanager.KindDelete, agentmanager.KindMove,
		agentmanager.KindSearch, agentmanager.KindExecute, agentmanager.KindThink, agentmanager.KindFetch:
	default:
		v = agentmanager.KindOther // includes switch_mode and unknown kinds
	}
	return &v
}

func normStatus(st *string) *agentmanager.ToolStatus {
	if st == nil {
		return nil
	}
	v := agentmanager.ToolStatus(*st)
	switch v {
	case agentmanager.ToolPending, agentmanager.ToolInProgress, agentmanager.ToolCompleted, agentmanager.ToolFailed:
		return &v
	}
	return nil
}

// mcpOf matches a tool title against the configured MCP server names (opencode names tools
// "<server>_<tool>"; other agents use "mcp__server__tool", "server.tool", "server:tool", "server/tool").
func (s *session) mcpOf(title string) (server, tool string) {
	title = strings.TrimSpace(title)
	for _, m := range s.cfg.MCPServers {
		for _, p := range []string{"mcp__" + m.Name + "__", m.Name + "_", m.Name + ".", m.Name + ":", m.Name + "/"} {
			if m.Name != "" && strings.HasPrefix(title, p) && len(title) > len(p) {
				return m.Name, title[len(p):]
			}
		}
	}
	return "", ""
}

func (s *session) toolStart(f toolFields) {
	call := agentmanager.ToolCall{ID: f.ToolCallID, Kind: agentmanager.KindOther, Status: agentmanager.ToolPending, RawInput: rawOrNil(f.RawInput)}
	if f.Title != nil {
		call.Title = *f.Title
	}
	if k := normKind(f.Kind); k != nil {
		call.Kind = *k
	}
	if st := normStatus(f.Status); st != nil {
		call.Status = *st
	}
	call.MCPServer, call.MCPTool = s.mcpOf(call.Title)
	s.tools[call.ID] = toolInfo{call.MCPServer, call.MCPTool}
	s.q.Push(agentmanager.ToolCallStart{Call: call})
	if out := outputOf(f.Content); out != nil || len(rawOrNil(f.RawOutput)) > 0 {
		s.q.Push(agentmanager.ToolCallUpdate{Update: agentmanager.ToolCallPatch{
			ID: call.ID, MCPServer: call.MCPServer, Output: out, RawOutput: rawOrNil(f.RawOutput)}})
	}
}

// patchFor converts partial tool fields into a patch, remembering/recalling the MCP server by id.
func (s *session) patchFor(f toolFields) agentmanager.ToolCallPatch {
	p := agentmanager.ToolCallPatch{ID: f.ToolCallID, Title: f.Title, Kind: normKind(f.Kind), Status: normStatus(f.Status),
		Output: outputOf(f.Content), RawInput: rawOrNil(f.RawInput), RawOutput: rawOrNil(f.RawOutput)}
	if info, ok := s.tools[f.ToolCallID]; ok {
		p.MCPServer = info.server
	}
	if p.MCPServer == "" && f.Title != nil {
		if srv, tool := s.mcpOf(*f.Title); srv != "" {
			p.MCPServer = srv
			s.tools[f.ToolCallID] = toolInfo{srv, tool}
		}
	}
	return p
}

// outputOf joins the text of tool content items; nil when there is none.
func outputOf(content json.RawMessage) *string {
	var items []struct {
		Type    string `json:"type"`
		Path    string `json:"path"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var parts []string
	for _, it := range items {
		switch it.Type {
		case "content":
			if it.Content.Type == "text" && it.Content.Text != "" {
				parts = append(parts, it.Content.Text)
			}
		case "diff":
			parts = append(parts, "diff "+it.Path)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	out := strings.Join(parts, "\n")
	return &out
}
