package ui

import (
	"encoding/json"
	"time"

	"agentmanager"
	"agentmanager/store"
	"agentmanager/transcript"
)

// SeedAgentDemo opens the agent panel on a canned conversation, for -png
// renders and screenshots; no agent runs.
func SeedAgentDemo(ws *Workspace) {
	p := ws.AgentPanel()
	p.hist.loaded, p.hist.off = true, true
	p.Show()
	p.Configure(agentmanager.Claude, "")
	str := func(s string) *string { return &s }
	p.Seed(func(t *transcript.T) {
		t.AddUser("Draw a 100 by 60 plate with a 10 mm hole in each corner")
		t.Apply(agentmanager.ThoughtDelta{Text: "Rectangle first, then four circles inset by 10."})
		t.Apply(agentmanager.PlanUpdate{Entries: []agentmanager.PlanEntry{
			{Content: "Draw the outline", Status: "completed"},
			{Content: "Add four corner holes", Status: "in_progress"},
			{Content: "Zoom to extents", Status: "pending"},
		}})
		t.Apply(agentmanager.TextDelta{Text: "I'll start with the outline on its own layer."})
		t.Apply(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "1", Title: "cad add_rect", Kind: agentmanager.KindEdit,
			Status: agentmanager.ToolInProgress, RawInput: json.RawMessage(`{}`)}})
		done := agentmanager.ToolCompleted
		t.Apply(agentmanager.ToolCallUpdate{Update: agentmanager.ToolCallPatch{ID: "1", Status: &done, Output: str(`{"new_handles":["2A"]}`)}})
		t.Apply(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "2", Title: "bash: ls", Kind: agentmanager.KindExecute, Status: agentmanager.ToolPending}})
		t.Apply(agentmanager.PermissionRequest{RequestID: "p1", Call: agentmanager.ToolCallPatch{ID: "2", Title: str("Run shell command: ls")},
			Options: []agentmanager.PermissionOption{
				{ID: "a", Name: "Allow once", Kind: agentmanager.AllowOnce},
				{ID: "r", Name: "Reject", Kind: agentmanager.RejectOnce},
			}})
	})
	second := newAgentTab()
	second.tr.AddUser("Add a centre line")
	second.tr.Apply(agentmanager.TextDelta{Text: "Done."})
	second.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn})
	second.attention = true
	third := newAgentTab()
	p.tabs = append(p.tabs, second, third)
	p.hist.menu = true
	p.hist.sessions = []store.Meta{
		{ID: "d1", Harness: "claude", Title: "Draw a 100 by 60 plate with a 10 mm hole in each corner", Updated: time.Now().Add(-2 * time.Minute)},
		{ID: "d2", Harness: "codex", Title: "Add a centre line", Updated: time.Now().Add(-26 * time.Hour)},
	}
}
