package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"agentmanager"
	"cad-lite/analytics"
	"cad-lite/doc"
)

type analyticsEvent struct {
	name       string
	properties analytics.Properties
}
type analyticsRecorder struct{ events []analyticsEvent }

func (r *analyticsRecorder) Track(name string, properties analytics.Properties) {
	r.events = append(r.events, analyticsEvent{name, properties})
}

func TestWorkspaceAnalyticsFollowsNewDrawingsAndAgentEvents(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	r := &analyticsRecorder{}
	ws.SetAnalytics(r)
	ws.Session().Script("LINE 0,0 10,0  ")
	ws.Session().Run("NEW")
	ws.Session().Script("CIRCLE 0,0 5 ")
	commands := 0
	for _, event := range r.events {
		if event.name == "command_used" {
			commands++
		}
	}
	if commands < 3 {
		t.Fatalf("new drawing did not inherit recorder: %d commands", commands)
	}
	p := ws.AgentPanel()
	p.hist.off = true
	t.Cleanup(ws.CloseAgent)
	tab := p.cur
	p.userMessage(tab, "private prompt must not be sent", nil)
	p.apply(tab, agentmanager.TextDelta{MessageID: "one", Text: "private response"})
	p.apply(tab, agentmanager.TextDelta{MessageID: "one", Text: "more private response"})
	p.apply(tab, agentmanager.TextDelta{MessageID: "two", Text: "second private response"})
	p.apply(tab, agentmanager.ToolCallStart{Call: agentmanager.ToolCall{Kind: agentmanager.KindRead, MCPServer: "cad", MCPTool: "drawing_info", Title: "private file path", RawInput: json.RawMessage(`{"path":"/private/drawing"}`)}})
	p.apply(tab, agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: "private error containing a secret key"})
	messages, tools, failures := 0, 0, 0
	for _, event := range r.events {
		switch event.name {
		case "agent_message":
			messages++
		case "agent_tool_used":
			tools++
		case "error":
			failures++
		}
		encoded, _ := json.Marshal(event.properties)
		if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
			t.Fatalf("event captured private payload: %s", encoded)
		}
	}
	if messages != 3 || tools != 1 || failures != 1 {
		t.Fatalf("unexpected agent counts: messages=%d tools=%d errors=%d", messages, tools, failures)
	}
}
