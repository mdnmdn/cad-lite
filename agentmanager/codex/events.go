package codex

import (
	"encoding/json"
	"strings"

	"agentmanager"
	"agentmanager/jsonrpc"
)

// streamState is touched only by the reader goroutine.
type streamState struct {
	text, thought map[string]bool // item ids that streamed deltas
	tools         map[string]bool // item ids already announced
}

func newStreamState() streamState {
	return streamState{text: map[string]bool{}, thought: map[string]bool{}, tools: map[string]bool{}}
}

// HandleNotification implements jsonrpc.Handler; it runs on the reader goroutine.
func (s *session) HandleNotification(_ *jsonrpc.Conn, method string, params json.RawMessage) {
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	if p == nil {
		p = map[string]any{}
	}
	if id := str(p, "threadId"); id != "" {
		s.mu.Lock()
		own := s.threadID
		s.mu.Unlock()
		if id != own {
			return
		}
	}
	switch method {
	case "item/agentMessage/delta":
		if d := str(p, "delta"); d != "" {
			id := str(p, "itemId")
			s.st.text[id] = true
			s.q.Push(agentmanager.TextDelta{MessageID: id, Text: d})
		}
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		if d := str(p, "delta"); d != "" {
			id := str(p, "itemId")
			s.st.thought[id] = true
			s.q.Push(agentmanager.ThoughtDelta{MessageID: id, Text: d})
		}
	case "item/started", "item/completed":
		item, _ := p["item"].(map[string]any)
		s.item(item, method == "item/completed")
	case "turn/plan/updated":
		s.plan(p)
	case "turn/completed":
		s.turnCompleted(p)
	case "error":
		msg := str(p, "message")
		switch e := p["error"].(type) {
		case string:
			msg = e
		case map[string]any:
			msg = str(e, "message")
		}
		if msg == "" {
			msg = "codex error"
		}
		if r, _ := p["willRetry"].(bool); r {
			msg += " (retrying)"
		}
		s.logEvent("error", msg)
	case "serverRequest/resolved":
		if raw, ok := p["requestId"]; ok {
			b, _ := json.Marshal(raw)
			s.mu.Lock()
			delete(s.asks, string(b))
			s.mu.Unlock()
		}
	}
}

func (s *session) turnCompleted(p map[string]any) {
	turn, _ := p["turn"].(map[string]any)
	end := agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	switch str(turn, "status") {
	case "interrupted":
		end.Reason = agentmanager.StopCancelled
	case "failed":
		end.Reason = agentmanager.StopFailed
		if e, ok := turn["error"].(map[string]any); ok {
			end.Err = str(e, "message")
		}
		if end.Err == "" {
			end.Err = "turn failed"
		}
	}
	s.mu.Lock()
	s.turnActive, s.turnID, s.cancelPending = false, "", false
	s.asks = map[string]*ask{}
	s.mu.Unlock()
	s.q.Push(end)
}

func (s *session) plan(p map[string]any) {
	steps, _ := p["plan"].([]any)
	var entries []agentmanager.PlanEntry
	for _, x := range steps {
		m, _ := x.(map[string]any)
		st := "pending"
		switch str(m, "status") {
		case "inProgress", "in_progress":
			st = "in_progress"
		case "completed":
			st = "completed"
		}
		entries = append(entries, agentmanager.PlanEntry{Content: str(m, "step"), Status: st})
	}
	s.q.Push(agentmanager.PlanUpdate{Entries: entries})
}

func toolStatus(item map[string]any) agentmanager.ToolStatus {
	switch str(item, "status") {
	case "completed":
		return agentmanager.ToolCompleted
	case "failed", "declined":
		return agentmanager.ToolFailed
	}
	return agentmanager.ToolInProgress
}

func (s *session) item(item map[string]any, done bool) {
	id := str(item, "id")
	switch str(item, "type") {
	case "agentMessage":
		if done && !s.st.text[id] {
			if t := str(item, "text"); t != "" {
				s.q.Push(agentmanager.TextDelta{MessageID: id, Text: t})
			}
		}
	case "reasoning":
		if done && !s.st.thought[id] {
			var parts []string
			for _, k := range []string{"summary", "content"} {
				if arr, ok := item[k].([]any); ok {
					for _, x := range arr {
						if t, ok := x.(string); ok && t != "" {
							parts = append(parts, t)
						}
					}
				}
				if len(parts) > 0 {
					break
				}
			}
			if len(parts) > 0 {
				s.q.Push(agentmanager.ThoughtDelta{MessageID: id, Text: strings.Join(parts, "\n")})
			}
		}
	case "commandExecution":
		call := agentmanager.ToolCall{ID: id, Title: str(item, "command"), Kind: agentmanager.KindExecute}
		call.RawInput = marshal(map[string]any{"command": item["command"], "cwd": item["cwd"]})
		patch := agentmanager.ToolCallPatch{ID: id}
		if out := str(item, "aggregatedOutput"); out != "" {
			patch.Output = &out
		}
		failed := false
		if code, ok := item["exitCode"].(float64); ok && code != 0 {
			failed = true
		}
		s.tool(call, patch, item, done, failed)
	case "fileChange":
		var paths, diffs []string
		changes, _ := item["changes"].([]any)
		for _, c := range changes {
			m, _ := c.(map[string]any)
			if pth := str(m, "path"); pth != "" {
				paths = append(paths, pth)
			}
			if d := str(m, "diff"); d != "" {
				diffs = append(diffs, d)
			}
		}
		title := "Edit files"
		if len(paths) > 0 {
			title = "Edit " + strings.Join(paths, ", ")
		}
		call := agentmanager.ToolCall{ID: id, Title: title, Kind: agentmanager.KindEdit, RawInput: marshal(item["changes"])}
		patch := agentmanager.ToolCallPatch{ID: id}
		if len(diffs) > 0 {
			out := strings.Join(diffs, "\n")
			patch.Output = &out
		}
		s.tool(call, patch, item, done, false)
	case "mcpToolCall":
		server, tool := str(item, "server"), str(item, "tool")
		call := agentmanager.ToolCall{
			ID: id, Title: server + ": " + tool, Kind: agentmanager.KindOther,
			RawInput: marshal(item["arguments"]), MCPServer: server, MCPTool: tool,
		}
		patch := agentmanager.ToolCallPatch{ID: id, MCPServer: server}
		failed := false
		if e, ok := item["error"].(map[string]any); ok && e != nil {
			out := str(e, "message")
			patch.Output = &out
			failed = true
		} else if r, ok := item["result"].(map[string]any); ok {
			patch.RawOutput = marshal(r)
			var texts []string
			if blocks, ok := r["content"].([]any); ok {
				for _, b := range blocks {
					if m, ok := b.(map[string]any); ok && str(m, "type") == "text" {
						texts = append(texts, str(m, "text"))
					}
				}
			}
			if len(texts) > 0 {
				out := strings.Join(texts, "\n")
				patch.Output = &out
			}
		}
		s.tool(call, patch, item, done, failed)
	}
}

// tool announces the call on first sight and patches its status on completion.
func (s *session) tool(call agentmanager.ToolCall, patch agentmanager.ToolCallPatch, item map[string]any, done, failed bool) {
	if !s.st.tools[call.ID] {
		s.st.tools[call.ID] = true
		call.Status = agentmanager.ToolInProgress
		s.q.Push(agentmanager.ToolCallStart{Call: call})
	}
	if !done {
		return
	}
	st := toolStatus(item)
	if failed {
		st = agentmanager.ToolFailed
	}
	patch.Status = &st
	s.q.Push(agentmanager.ToolCallUpdate{Update: patch})
}

func marshal(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil
	}
	return b
}
