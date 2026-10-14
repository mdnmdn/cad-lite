package claude_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/claude"
	"agentmanager/mcp"
)

// TestLiveClaude drives the installed claude CLI through the backend: one plain turn, then a turn
// that calls an MCP tool of an in-process server (auto-allowed through AllowMCP). Needs auth and
// network; run with AGENTMANAGER_LIVE_CLAUDE=1.
func TestLiveClaude(t *testing.T) {
	if os.Getenv("AGENTMANAGER_LIVE_CLAUDE") != "1" {
		t.Skip("set AGENTMANAGER_LIVE_CLAUDE=1 to run against the real claude CLI")
	}
	cwd := t.TempDir()
	var calls atomic.Int32
	srv := mcp.NewServer(mcp.Options{Name: "cad", Version: "0"})
	srv.Register(mcp.Tool{
		Name:        "ping",
		Description: "Returns the word pong. Call it when asked to ping.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
			calls.Add(1)
			return mcp.Text("pong-from-cad"), nil
		},
	})
	ep, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	s, err := claude.New().Open(context.Background(), agentmanager.Config{
		Harness:      agentmanager.Claude,
		Cwd:          cwd,
		MCPServers:   []agentmanager.MCPServer{{Name: "cad", URL: ep.URL, Token: ep.Token}},
		AutoApprove:  agentmanager.AllowMCP("cad"),
		SystemPrompt: "Be extremely brief.",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	type turn struct {
		started                bool
		text                   strings.Builder
		thoughts, tools, perms int
		mcpStart               agentmanager.ToolCall
		mcpUpdate              agentmanager.ToolCallPatch
		permCall               agentmanager.ToolCallPatch
		failedTool             bool
		end                    agentmanager.TurnEnd
	}
	answer := agentmanager.PermissionAnswer{Cancelled: true}
	run := func(prompt string) turn {
		t.Helper()
		if err := s.Prompt(context.Background(), prompt); err != nil {
			t.Fatal(err)
		}
		var tr turn
		deadline := time.After(180 * time.Second)
		for {
			select {
			case ev, ok := <-s.Events():
				if !ok {
					t.Fatal("events closed")
				}
				t.Logf("%T %+v", ev, ev)
				switch e := ev.(type) {
				case agentmanager.SessionStarted:
					tr.started = true
				case agentmanager.TextDelta:
					tr.text.WriteString(e.Text)
				case agentmanager.ThoughtDelta:
					tr.thoughts++
				case agentmanager.ToolCallStart:
					tr.tools++
					if e.Call.MCPServer != "" {
						tr.mcpStart = e.Call
					}
				case agentmanager.ToolCallUpdate:
					if e.Update.Status != nil && *e.Update.Status == agentmanager.ToolFailed {
						tr.failedTool = true
					}
					if e.Update.MCPServer != "" {
						tr.mcpUpdate = e.Update
					}
				case agentmanager.PermissionRequest:
					tr.perms++
					if err := s.RespondPermission(e.RequestID, answer); err != nil {
						t.Errorf("RespondPermission: %v", err)
					}
					tr.permCall = e.Call
				case agentmanager.Exited:
					t.Fatalf("exited: %s", e.Err)
				case agentmanager.TurnEnd:
					tr.end = e
					return tr
				}
			case <-deadline:
				t.Fatal("timeout")
			}
		}
	}

	a := run("Reply with exactly the single word: pong")
	if !a.started || s.ID() == "" {
		t.Error("no SessionStarted")
	}
	if a.end.Reason != agentmanager.StopEndTurn {
		t.Errorf("turn 1 end %+v", a.end)
	}
	if !strings.Contains(strings.ToLower(a.text.String()), "pong") {
		t.Errorf("turn 1 text %q", a.text.String())
	}
	if strings.Count(strings.ToLower(a.text.String()), "pong") > 1 {
		t.Errorf("text duplicated (partial dedupe failed?): %q", a.text.String())
	}

	b := run("Call the mcp__cad__ping tool once and tell me what it returned.")
	if b.end.Reason != agentmanager.StopEndTurn {
		t.Errorf("turn 2 end %+v", b.end)
	}
	if calls.Load() == 0 || b.mcpStart.MCPServer != "cad" || b.mcpStart.MCPTool != "ping" {
		t.Errorf("mcp calls=%d start=%+v", calls.Load(), b.mcpStart)
	}
	if b.perms != 0 {
		t.Errorf("AllowMCP did not auto-allow: %d permission requests", b.perms)
	}
	if b.mcpUpdate.Output == nil || !strings.Contains(*b.mcpUpdate.Output, "pong-from-cad") {
		t.Errorf("mcp result update %+v", b.mcpUpdate)
	}
	// Permission round trip through the real control protocol: allow, then deny.
	answer = agentmanager.PermissionAnswer{OptionID: "allow"}
	c := run("Use the Bash tool to run exactly: touch live-allow.txt")
	if c.perms == 0 || c.permCall.Kind == nil || *c.permCall.Kind != agentmanager.KindExecute {
		t.Errorf("allow turn: perms=%d call=%+v", c.perms, c.permCall)
	}
	if _, err := os.Stat(filepath.Join(cwd, "live-allow.txt")); err != nil || c.failedTool {
		t.Errorf("allowed tool did not run: stat=%v failed=%v", err, c.failedTool)
	}
	answer = agentmanager.PermissionAnswer{OptionID: "deny"}
	d := run("Use the Bash tool to run exactly: touch live-deny.txt")
	if d.perms == 0 || !d.failedTool || d.end.Reason != agentmanager.StopEndTurn {
		t.Errorf("deny turn: perms=%d failed=%v end=%+v", d.perms, d.failedTool, d.end)
	}
	if _, err := os.Stat(filepath.Join(cwd, "live-deny.txt")); err == nil {
		t.Error("denied tool ran")
	}

	// Interrupt mid-turn, then reuse the same process.
	if err := s.Prompt(context.Background(), "Write the numbers 1 to 2000, one per line, no other text."); err != nil {
		t.Fatal(err)
	}
	cancelled := false
	for end := time.After(120 * time.Second); ; {
		var ev agentmanager.Event
		select {
		case ev = <-s.Events():
		case <-end:
			t.Fatal("timeout waiting for cancelled turn")
		}
		if _, ok := ev.(agentmanager.TextDelta); ok && !cancelled {
			cancelled = true
			if err := s.Cancel(); err != nil {
				t.Fatal(err)
			}
		}
		if te, ok := ev.(agentmanager.TurnEnd); ok {
			if te.Reason != agentmanager.StopCancelled {
				t.Errorf("interrupted turn ended %+v", te)
			}
			break
		}
	}
	if e := run("Reply with exactly the single word: pong"); e.end.Reason != agentmanager.StopEndTurn || !strings.Contains(strings.ToLower(e.text.String()), "pong") {
		t.Errorf("prompt after interrupt: %q %+v", e.text.String(), e.end)
	}
}
