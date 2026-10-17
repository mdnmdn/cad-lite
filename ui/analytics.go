package ui

import (
	"agentmanager"
	"cad-lite/analytics"
)

// SetAnalytics installs a nonblocking recorder for the workspace and all existing and new drawings.
// nil disables events. The host owns the recorder's lifetime.
func (ws *Workspace) SetAnalytics(recorder analytics.Recorder) {
	ws.analytics = recorder
	for _, tab := range ws.tabs {
		tab.session.SetAnalytics(recorder)
	}
}

func (ws *Workspace) track(name string, properties analytics.Properties) {
	analytics.Record(ws.analytics, name, properties)
}

func (p *AgentPanel) trackEvent(t *agentTab, event agentmanager.Event) {
	properties := analytics.Properties{"harness": string(p.harnessOf(t))}
	switch ev := event.(type) {
	case agentmanager.SessionStarted:
		t.analyticsMessages = nil
		p.ws.track("agent_session_started", properties)
	case agentmanager.TextDelta:
		if ev.Text != "" && !t.analyticsMessages[ev.MessageID] && len(t.analyticsMessages) < 128 {
			if t.analyticsMessages == nil {
				t.analyticsMessages = make(map[string]bool)
			}
			t.analyticsMessages[ev.MessageID] = true
			properties["role"] = "assistant"
			p.ws.track("agent_message", properties)
		}
	case agentmanager.ToolCallStart:
		properties["kind"] = string(ev.Call.Kind)
		if ev.Call.MCPServer == "cad" {
			properties["tool"] = ev.Call.MCPTool
		}
		p.ws.track("agent_tool_used", properties)
	case agentmanager.ToolCallUpdate:
		if ev.Update.Status != nil && *ev.Update.Status == agentmanager.ToolFailed {
			p.ws.track("error", analytics.Properties{"source": "agent", "code": "tool_failed"})
		}
	case agentmanager.TurnEnd:
		t.analyticsMessages = nil
		properties["reason"] = string(ev.Reason)
		p.ws.track("agent_turn_finished", properties)
		if ev.Err != "" {
			p.ws.track("error", analytics.Properties{"source": "agent", "code": "turn_failed"})
		}
	case agentmanager.Exited:
		t.analyticsMessages = nil
		if ev.Err != "" {
			p.ws.track("error", analytics.Properties{"source": "agent", "code": "process_exited"})
		}
	case agentmanager.Log:
		if ev.Level == "error" {
			p.ws.track("error", analytics.Properties{"source": "agent", "code": "backend_log"})
		}
	}
}
