package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentmanager"
)

type line struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	SessionID string `json:"session_id"`
	Model     string `json:"model"`

	Message struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`

	Event json.RawMessage `json:"event"`

	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`

	ToolUseResult json.RawMessage `json:"tool_use_result"`

	IsError    bool            `json:"is_error"`
	Result     json.RawMessage `json:"result"`
	StopReason string          `json:"stop_reason"`
	Usage      json.RawMessage `json:"usage"`

	Log struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"log"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Status    string          `json:"status"`
	Content   json.RawMessage `json:"content"`
}

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
}

type permissionAsk struct {
	Subtype     string            `json:"subtype"`
	ToolName    string            `json:"tool_name"`
	DisplayName string            `json:"display_name"`
	Input       json.RawMessage   `json:"input"`
	Description string            `json:"description"`
	Suggestions []json.RawMessage `json:"permission_suggestions"`
	ToolUseID   string            `json:"tool_use_id"`
}

type suggestion struct {
	Type string `json:"type"`
	Mode string `json:"mode"`
}

// handle processes one stdout line; it runs on the reader goroutine only.
func (s *session) handle(raw []byte) {
	var l line
	if err := json.Unmarshal(raw, &l); err != nil {
		return
	}
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			s.onInit(l)
		}
	case "assistant":
		s.onAssistant(l)
	case "user":
		s.onUser(l)
	case "stream_event":
		s.onStream(l)
	case "control_request":
		s.onControlRequest(l)
	case "control_cancel_request":
		s.mu.Lock()
		delete(s.perms, l.RequestID)
		s.mu.Unlock()
	case "result":
		s.onResult(l)
	case "log":
		level := l.Log.Level
		if level == "" {
			level = "info"
		}
		s.q.Push(agentmanager.Log{Level: level, Message: l.Log.Message})
	}
}

func (s *session) onInit(l line) {
	s.mu.Lock()
	first := !s.started
	s.started = true
	if l.SessionID != "" {
		s.id = l.SessionID
	}
	s.mu.Unlock()
	if first {
		s.q.Push(agentmanager.SessionStarted{SessionID: l.SessionID, Model: l.Model})
	}
}

func (s *session) onStream(l line) {
	var ev streamEvent
	if json.Unmarshal(l.Event, &ev) != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		s.mu.Lock()
		s.curMsg = ev.Message.ID
		s.mu.Unlock()
	case "content_block_delta":
		s.mu.Lock()
		id := s.curMsg
		if id != "" {
			s.streamed[id] = true
		}
		s.mu.Unlock()
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text != "" {
				s.q.Push(agentmanager.TextDelta{MessageID: id, Text: ev.Delta.Text})
			}
		case "thinking_delta":
			if ev.Delta.Thinking != "" {
				s.q.Push(agentmanager.ThoughtDelta{MessageID: id, Text: ev.Delta.Thinking})
			}
		}
	}
}

func (s *session) onAssistant(l line) {
	var blocks []block
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	id := l.Message.ID
	s.mu.Lock()
	streamed := id != "" && s.streamed[id]
	s.mu.Unlock()
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if !streamed && b.Text != "" {
				s.q.Push(agentmanager.TextDelta{MessageID: id, Text: b.Text})
			}
		case "thinking":
			if !streamed && b.Thinking != "" {
				s.q.Push(agentmanager.ThoughtDelta{MessageID: id, Text: b.Thinking})
			}
		case "tool_use":
			s.announceTool(b)
		}
	}
}

func (s *session) announceTool(b block) {
	s.mu.Lock()
	if _, ok := s.tools[b.ID]; ok {
		s.mu.Unlock()
		return
	}
	call := toolCall(b.ID, b.Name, b.Input)
	s.tools[b.ID] = call
	s.mu.Unlock()
	s.q.Push(agentmanager.ToolCallStart{Call: call})
	if b.Name == "TodoWrite" {
		if entries := planEntries(b.Input); entries != nil {
			s.q.Push(agentmanager.PlanUpdate{Entries: entries})
		}
	}
}

func (s *session) onUser(l line) {
	var blocks []block
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_result" || b.ToolUseID == "" {
			continue
		}
		status := agentmanager.ToolCompleted
		if b.IsError {
			status = agentmanager.ToolFailed
		}
		if b.Status == "async_launched" || toolUseResultStatus(l.ToolUseResult) == "async_launched" {
			status = agentmanager.ToolInProgress
		}
		patch := agentmanager.ToolCallPatch{ID: b.ToolUseID, Status: &status, RawOutput: b.Content}
		if out := resultText(b.Content); out != "" {
			patch.Output = &out
		}
		s.mu.Lock()
		patch.MCPServer = s.tools[b.ToolUseID].MCPServer
		s.mu.Unlock()
		s.q.Push(agentmanager.ToolCallUpdate{Update: patch})
	}
}

func toolUseResultStatus(raw json.RawMessage) string {
	var r struct {
		Status string `json:"status"`
	}
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &r) == nil {
		return r.Status
	}
	return ""
}

// resultText flattens a tool_result content (string or text blocks) to text.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var bs []block
	if json.Unmarshal(raw, &bs) != nil {
		return ""
	}
	var parts []string
	for _, b := range bs {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (s *session) onResult(l line) {
	s.mu.Lock()
	running := s.turn
	interrupted := s.interrupted
	s.turn = false
	s.interrupted = false
	s.streamed = map[string]bool{}
	s.curMsg = ""
	s.mu.Unlock()
	s.log("result subtype=%s usage=%s", l.Subtype, string(l.Usage))
	if !running {
		return
	}
	end := agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	switch {
	case interrupted:
		end.Reason = agentmanager.StopCancelled
	case l.Subtype == "error_max_turns":
		end.Reason = agentmanager.StopMaxTurns
	case l.IsError || (l.Subtype != "" && l.Subtype != "success"):
		end.Reason = agentmanager.StopFailed
		end.Err = resultText(l.Result)
		if end.Err == "" {
			end.Err = l.Subtype
		}
	case l.StopReason == "max_tokens":
		end.Reason = agentmanager.StopMaxTokens
	case l.StopReason == "refusal":
		end.Reason = agentmanager.StopRefusal
	}
	s.q.Push(end)
}

func (s *session) onControlRequest(l line) {
	var ask permissionAsk
	if json.Unmarshal(l.Request, &ask) != nil || ask.Subtype != "can_use_tool" {
		_ = s.conn.SendRaw(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": l.RequestID, "error": "unsupported",
		}})
		return
	}
	s.mu.Lock()
	call, known := s.tools[ask.ToolUseID]
	s.mu.Unlock()
	if !known {
		call = toolCall(ask.ToolUseID, ask.ToolName, ask.Input)
	}
	title := call.Title
	patch := agentmanager.ToolCallPatch{
		ID: ask.ToolUseID, Title: &title, MCPServer: call.MCPServer, Kind: &call.Kind, RawInput: ask.Input,
	}
	opts := []agentmanager.PermissionOption{{ID: "allow", Name: "Allow", Kind: agentmanager.AllowOnce}}
	for i, raw := range ask.Suggestions {
		opts = append(opts, agentmanager.PermissionOption{
			ID: fmt.Sprintf("allow_always:%d", i), Name: suggestionName(raw), Kind: agentmanager.AllowAlways,
		})
	}
	opts = append(opts, agentmanager.PermissionOption{ID: "deny", Name: "Deny", Kind: agentmanager.RejectOnce})
	req := agentmanager.PermissionRequest{RequestID: l.RequestID, Call: patch, Options: opts}

	if s.cfg.AutoApprove != nil {
		if id, ok := s.cfg.AutoApprove(req); ok {
			p := &pending{ask: ask}
			if err := s.respond(l.RequestID, p, agentmanager.PermissionAnswer{OptionID: id}); err == nil {
				return
			}
		}
	}
	s.mu.Lock()
	s.perms[l.RequestID] = &pending{ask: ask}
	s.mu.Unlock()
	s.q.Push(req)
}

func suggestionName(raw json.RawMessage) string {
	var sg suggestion
	_ = json.Unmarshal(raw, &sg)
	if sg.Type == "setMode" && sg.Mode != "" {
		return "Allow and switch to " + sg.Mode
	}
	return "Always allow"
}

// toolCall builds the neutral ToolCall for a tool_use (title "<Name> <path|command|...>").
func toolCall(id, name string, input json.RawMessage) agentmanager.ToolCall {
	c := agentmanager.ToolCall{
		ID: id, Title: name, Kind: toolKind(name), Status: agentmanager.ToolInProgress, RawInput: input,
	}
	if srv, tool, ok := parseMCPName(name); ok {
		c.MCPServer, c.MCPTool = srv, tool
		return c
	}
	var in map[string]any
	if json.Unmarshal(input, &in) == nil {
		for _, k := range []string{"file_path", "path", "command", "pattern", "url"} {
			if v, ok := in[k].(string); ok && v != "" {
				c.Title = name + " " + v
				break
			}
		}
	}
	return c
}

// parseMCPName splits "mcp__<server>__<tool>" (tool names may contain further "__").
func parseMCPName(name string) (server, tool string, ok bool) {
	rest, found := strings.CutPrefix(name, "mcp__")
	if !found {
		return "", "", false
	}
	server, tool, found = strings.Cut(rest, "__")
	if !found || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

func toolKind(name string) agentmanager.ToolKind {
	switch name {
	case "Read", "NotebookRead", "Glob", "LS":
		return agentmanager.KindRead
	case "Edit", "Write", "NotebookEdit", "MultiEdit":
		return agentmanager.KindEdit
	case "Bash", "BashOutput", "KillShell":
		return agentmanager.KindExecute
	case "Grep", "WebSearch":
		return agentmanager.KindSearch
	case "WebFetch":
		return agentmanager.KindFetch
	case "TodoWrite", "ExitPlanMode":
		return agentmanager.KindThink
	}
	return agentmanager.KindOther
}

func planEntries(input json.RawMessage) []agentmanager.PlanEntry {
	var in struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	if json.Unmarshal(input, &in) != nil || in.Todos == nil {
		return nil
	}
	out := make([]agentmanager.PlanEntry, len(in.Todos))
	for i, t := range in.Todos {
		out[i] = agentmanager.PlanEntry{Content: t.Content, Status: t.Status}
	}
	return out
}
