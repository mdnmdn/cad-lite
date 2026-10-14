package codex_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/codex"
	"agentmanager/internal/testutil"
)

type env struct {
	log, argv string
	cfg       agentmanager.Config
}

func setup(t *testing.T, scenario string) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{log: filepath.Join(dir, "log"), argv: filepath.Join(dir, "argv")}
	sc := filepath.Join(dir, "sc.json")
	if err := os.WriteFile(sc, []byte(scenario), 0o600); err != nil {
		t.Fatal(err)
	}
	e.cfg = agentmanager.Config{
		Harness: agentmanager.Codex, Binary: testutil.BuildFake(t), Cwd: dir,
		Args: []string{"--scenario", sc},
		Env:  []string{"FAKEAGENT_LOG=" + e.log, "FAKEAGENT_ARGV=" + e.argv},
	}
	return e
}

func (e *env) open(t *testing.T) agentmanager.Session {
	t.Helper()
	s, err := codex.New().Open(context.Background(), e.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func next(t *testing.T, s agentmanager.Session) agentmanager.Event {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			t.Fatal("events closed")
		}
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for event")
	}
	return nil
}

// until collects events up to and including the first one satisfying stop.
func until(t *testing.T, s agentmanager.Session, stop func(agentmanager.Event) bool) []agentmanager.Event {
	t.Helper()
	var evs []agentmanager.Event
	for {
		ev := next(t, s)
		evs = append(evs, ev)
		if stop(ev) {
			return evs
		}
	}
}

func isTurnEnd(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.TurnEnd); return ok }

func isPerm(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok }

func logLines(t *testing.T, e *env) string {
	b, _ := os.ReadFile(e.log)
	return string(b)
}

func TestHandshakeAndArgv(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	e.cfg.Model = "gpt-x"
	e.cfg.SystemPrompt = `be "brief"`
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: "http://127.0.0.1:1/mcp", Token: "sekret"}}
	e.cfg.Env = append(e.cfg.Env, "EXTRA=1")
	s := e.open(t)
	ev := next(t, s)
	if ss, ok := ev.(agentmanager.SessionStarted); !ok || ss.SessionID != "t-1" || ss.Model != "fake-model" {
		t.Fatalf("first event %#v", ev)
	}
	if s.ID() != "t-1" {
		t.Fatalf("ID %q", s.ID())
	}
	b, _ := os.ReadFile(e.argv)
	argv := string(b)
	for _, want := range []string{
		"-c\nmcp_servers.cad={url=\"http://127.0.0.1:1/mcp\", bearer_token_env_var=\"GODRAW_MCP_TOKEN\", default_tools_approval_mode=\"approve\", tool_timeout_sec=120}\n",
		"-c\nsandbox_mode=\"read-only\"\n", "-c\napproval_policy=\"on-request\"\n",
		"-c\nmodel=\"gpt-x\"\n", "-c\ndeveloper_instructions=\"be \\\"brief\\\"\"\n",
		"app-server\n--listen\nstdio://\n", "GODRAW_MCP_TOKEN=sekret\n", "EXTRA=1\n",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv dump missing %q:\n%s", want, argv)
		}
	}
	if strings.Index(argv, "-c\n") > strings.Index(argv, "app-server") {
		t.Error("-c overrides must precede the subcommand")
	}
	if strings.Contains(strings.SplitN(argv, "ENV\n", 2)[0], "sekret") {
		t.Error("token leaked into argv")
	}
	log := logLines(t, e)
	for _, want := range []string{`"method":"initialize"`, `"experimentalApi":true`, `"method":"initialized"`, `"method":"thread/start"`} {
		if !strings.Contains(log, want) {
			t.Errorf("client frames missing %s:\n%s", want, log)
		}
	}
}

func TestResume(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	e.cfg.ResumeID = "t-old"
	s := e.open(t)
	if ss := next(t, s).(agentmanager.SessionStarted); ss.SessionID != "t-old" {
		t.Fatalf("session id %q", ss.SessionID)
	}
	log := logLines(t, e)
	if !strings.Contains(log, `"method":"thread/resume"`) || !strings.Contains(log, `"threadId":"t-old"`) || !strings.Contains(log, `"excludeTurns":true`) {
		t.Fatalf("resume frame missing:\n%s", log)
	}
}

func TestHandshakeTimeout(t *testing.T) {
	defer codex.SetHandshakeTimeout(300 * time.Millisecond)()
	e := setup(t, `{"builtin":"no-handshake"}`)
	start := time.Now()
	_, err := codex.New().Open(context.Background(), e.cfg)
	if err == nil || !strings.Contains(err.Error(), "initialize") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatal("too slow")
	}
}

func TestHandshakeStderrTail(t *testing.T) {
	e := setup(t, `{"steps":[{"op":"stderr","frame":{"text":"no auth"}},{"op":"exit","code":1}]}`)
	_, err := codex.New().Open(context.Background(), e.cfg)
	if err == nil || !strings.Contains(err.Error(), "no auth") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamingTurn(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	s := e.open(t)
	next(t, s)
	if err := s.Prompt(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	evs := until(t, s, isTurnEnd)
	want := []agentmanager.Event{
		agentmanager.ThoughtDelta{MessageID: "r1", Text: "thinking"},
		agentmanager.TextDelta{MessageID: "m1", Text: "Hello"},
		agentmanager.TextDelta{MessageID: "m1", Text: " world"},
		agentmanager.ThoughtDelta{MessageID: "r2", Text: "plan it"},
		agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn},
	}
	if len(evs) != len(want) {
		t.Fatalf("events %#v", evs)
	}
	for i := range want {
		if evs[i] != want[i] {
			t.Errorf("event %d = %#v want %#v", i, evs[i], want[i])
		}
	}
	if !strings.Contains(logLines(t, e), `"text_elements":[]`) {
		t.Error("turn/start input shape")
	}
	if err := s.Prompt(context.Background(), "again"); err != nil {
		t.Fatalf("second prompt: %v", err)
	}
	until(t, s, isTurnEnd)
}

func TestPromptWhileRunning(t *testing.T) {
	e := setup(t, `{"builtin":"hold-turn"}`)
	s := e.open(t)
	next(t, s)
	if err := s.Prompt(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Prompt(context.Background(), "b"); err == nil {
		t.Fatal("expected error for concurrent prompt")
	}
}

func TestToolItemsAndForeignThread(t *testing.T) {
	e := setup(t, `{"builtin":"mcp-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	evs := until(t, s, isTurnEnd)
	var starts []agentmanager.ToolCall
	var updates []agentmanager.ToolCallPatch
	var logs []agentmanager.Log
	var plan *agentmanager.PlanUpdate
	for _, ev := range evs {
		switch x := ev.(type) {
		case agentmanager.ToolCallStart:
			starts = append(starts, x.Call)
		case agentmanager.ToolCallUpdate:
			updates = append(updates, x.Update)
		case agentmanager.Log:
			logs = append(logs, x)
		case agentmanager.PlanUpdate:
			plan = &x
		case agentmanager.TextDelta:
			if x.Text == "SUBAGENT" {
				t.Error("foreign thread delta leaked")
			}
		}
	}
	if len(starts) != 2 || len(updates) != 2 {
		t.Fatalf("starts %v updates %v", starts, updates)
	}
	mc := starts[0]
	if mc.ID != "mc1" || mc.Title != "cad: add_line" || mc.MCPServer != "cad" || mc.MCPTool != "add_line" ||
		mc.Status != agentmanager.ToolInProgress || string(mc.RawInput) != `{"x1":0,"y1":0}` {
		t.Errorf("mcp start %#v", mc)
	}
	u := updates[0]
	if u.ID != "mc1" || u.MCPServer != "cad" || *u.Status != agentmanager.ToolCompleted || *u.Output != "ok line 1" {
		t.Errorf("mcp update %#v", u)
	}
	if starts[1].Kind != agentmanager.KindEdit || starts[1].Title != "Edit a.txt" || *updates[1].Status != agentmanager.ToolFailed || *updates[1].Output != "+hi" {
		t.Errorf("file change %#v %#v", starts[1], updates[1])
	}
	if plan == nil || len(plan.Entries) != 2 || plan.Entries[0] != (agentmanager.PlanEntry{Content: "draw", Status: "in_progress"}) || plan.Entries[1].Status != "pending" {
		t.Errorf("plan %#v", plan)
	}
	if len(logs) != 1 || logs[0].Level != "error" || !strings.Contains(logs[0].Message, "transient") {
		t.Errorf("logs %#v", logs)
	}
	last := evs[len(evs)-1].(agentmanager.TurnEnd)
	if last.Reason != agentmanager.StopFailed || last.Err == "" {
		t.Errorf("turn end %#v", last)
	}
	// the unknown server request got exactly one -32601 reply
	if n := strings.Count(logLines(t, e), `"code":-32601`); n != 1 {
		t.Errorf("-32601 replies = %d\n%s", n, logLines(t, e))
	}
}

func TestApprovalAnswered(t *testing.T) {
	for _, tc := range []struct {
		opt, decision string
		status        agentmanager.ToolStatus
	}{
		{"accept", "accept", agentmanager.ToolCompleted},
		{"acceptForSession", "acceptForSession", agentmanager.ToolCompleted},
		{"decline", "decline", agentmanager.ToolFailed},
	} {
		t.Run(tc.opt, func(t *testing.T) {
			e := setup(t, `{"builtin":"tool-turn"}`)
			s := e.open(t)
			next(t, s)
			s.Prompt(context.Background(), "go")
			evs := until(t, s, isPerm)
			req := evs[len(evs)-1].(agentmanager.PermissionRequest)
			if req.Call.ID != "c1" || *req.Call.Title != "ls -la" || *req.Call.Kind != agentmanager.KindExecute {
				t.Errorf("request %#v", req)
			}
			if len(req.Options) != 3 || req.Options[0].ID != "accept" || req.Options[0].Kind != agentmanager.AllowOnce ||
				req.Options[1].Kind != agentmanager.AllowAlways || req.Options[2].Kind != agentmanager.RejectOnce {
				t.Errorf("options %#v", req.Options)
			}
			if err := s.RespondPermission("nope", agentmanager.PermissionAnswer{OptionID: "accept"}); err == nil {
				t.Error("unknown request id accepted")
			}
			if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{OptionID: "bogus"}); err == nil {
				t.Error("bogus option accepted")
			}
			if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{OptionID: tc.opt}); err != nil {
				t.Fatal(err)
			}
			rest := until(t, s, isTurnEnd)
			if d := rest[0].(agentmanager.TextDelta); d.Text != "decision="+tc.decision {
				t.Errorf("fake saw %q", d.Text)
			}
			up := rest[1].(agentmanager.ToolCallUpdate).Update
			if *up.Status != tc.status || (tc.status == agentmanager.ToolCompleted && *up.Output != "total 0\n") {
				t.Errorf("update %#v", up)
			}
			if !strings.Contains(logLines(t, e), `"result":{"decision":"`+tc.decision+`"}`) {
				t.Errorf("reply bytes:\n%s", logLines(t, e))
			}
		})
	}
}

func TestApprovalPolicy(t *testing.T) {
	e := setup(t, `{"builtin":"tool-turn"}`)
	e.cfg.AutoApprove = func(r agentmanager.PermissionRequest) (string, bool) { return "accept", true }
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	for _, ev := range until(t, s, isTurnEnd) {
		if isPerm(ev) {
			t.Fatal("policy-answered request reached the UI")
		}
	}
	if !strings.Contains(logLines(t, e), `"decision":"accept"`) {
		t.Error("policy answer not sent")
	}
}

func TestApprovalPolicyDeclinesToUI(t *testing.T) {
	e := setup(t, `{"builtin":"tool-turn"}`)
	e.cfg.AutoApprove = agentmanager.AllowMCP("cad")
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	until(t, s, isPerm)
}

func TestCancelWithPendingApproval(t *testing.T) {
	e := setup(t, `{"builtin":"cancel-ask-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	until(t, s, isPerm)
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	evs := until(t, s, isTurnEnd)
	if d := evs[0].(agentmanager.TextDelta); d.Text != "decision=cancel" {
		t.Errorf("ask answered %q", d.Text)
	}
	if te := evs[len(evs)-1].(agentmanager.TurnEnd); te.Reason != agentmanager.StopCancelled {
		t.Errorf("turn end %#v", te)
	}
	log := logLines(t, e)
	if strings.Index(log, `"decision":"cancel"`) > strings.Index(log, `"method":"turn/interrupt"`) {
		t.Errorf("ask must be answered before interrupt:\n%s", log)
	}
	if !strings.Contains(log, `"threadId":"t-1","turnId":"turn-1"`) && !strings.Contains(log, `"turnId":"turn-1","threadId":"t-1"`) {
		t.Errorf("interrupt params:\n%s", log)
	}
}

func TestCancelIdleIsNoop(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	s := e.open(t)
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logLines(t, e), "interrupt") {
		t.Error("idle cancel sent interrupt")
	}
}

func TestCancelThenPromptAgain(t *testing.T) {
	e := setup(t, `{"builtin":"hold-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.TextDelta); return ok })
	s.Cancel()
	if te := until(t, s, isTurnEnd); te[len(te)-1].(agentmanager.TurnEnd).Reason != agentmanager.StopCancelled {
		t.Fatal("not cancelled")
	}
	if err := s.Prompt(context.Background(), "again"); err != nil {
		t.Fatalf("prompt after cancel: %v", err)
	}
}

func TestCancelBeforeTurnAck(t *testing.T) {
	e := setup(t, `{"builtin":"late-ack-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	evs := until(t, s, isTurnEnd)
	if te := evs[len(evs)-1].(agentmanager.TurnEnd); te.Reason != agentmanager.StopCancelled {
		t.Fatalf("turn end %#v", te)
	}
	log := logLines(t, e)
	if n := strings.Count(log, `"method":"turn/interrupt"`); n != 1 {
		t.Fatalf("interrupts = %d\n%s", n, log)
	}
}

func TestCrashMidTurn(t *testing.T) {
	e := setup(t, `{"builtin":"crash-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	ev := next(t, s)
	ex, ok := ev.(agentmanager.Exited)
	if !ok || !strings.Contains(ex.Err, "boom") {
		t.Fatalf("event %#v", ev)
	}
	if _, ok := <-s.Events(); ok {
		t.Fatal("events not closed after Exited")
	}
	if err := s.Prompt(context.Background(), "x"); !errors.Is(err, agentmanager.ErrClosed) {
		t.Errorf("Prompt err %v", err)
	}
	if err := s.Cancel(); !errors.Is(err, agentmanager.ErrClosed) {
		t.Errorf("Cancel err %v", err)
	}
	if err := s.RespondPermission("1", agentmanager.PermissionAnswer{}); !errors.Is(err, agentmanager.ErrClosed) {
		t.Errorf("RespondPermission err %v", err)
	}
}

func TestCloseIsCleanAndIdempotent(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	s := e.open(t)
	next(t, s)
	s.Close()
	s.Close()
	ev := next(t, s)
	if ex, ok := ev.(agentmanager.Exited); !ok || ex.Err != "" {
		t.Fatalf("event %#v", ev)
	}
	if err := s.Prompt(context.Background(), "x"); !errors.Is(err, agentmanager.ErrClosed) {
		t.Errorf("Prompt err %v", err)
	}
}

func TestPermissionShapes(t *testing.T) {
	// Raw frames through steps: legacy approvals, permissions, elicitation, user input.
	steps := []map[string]any{
		{"op": "expect", "match": map[string]any{"method": "initialize"}, "var": "i"},
		{"op": "send", "frame": map[string]any{"id": "$i", "result": map[string]any{}}},
		{"op": "expect", "match": map[string]any{"method": "initialized"}},
		{"op": "expect", "match": map[string]any{"method": "thread/start"}, "var": "t"},
		{"op": "send", "frame": map[string]any{"id": "$t", "result": map[string]any{"thread": map[string]any{"id": "t-9"}}}},
		{"op": "expect", "match": map[string]any{"method": "turn/start"}, "var": "u"},
		{"op": "send", "frame": map[string]any{"id": "$u", "result": map[string]any{"turn": map[string]any{"id": "turn-1"}}}},
		{"op": "send", "frame": map[string]any{"id": 11, "method": "execCommandApproval", "params": map[string]any{"command": []any{"ls", "-l"}}}},
		{"op": "send", "frame": map[string]any{"id": 12, "method": "item/permissions/requestApproval", "params": map[string]any{
			"reason": "net", "permissions": map[string]any{"network": map[string]any{"enabled": true}}}}},
		{"op": "send", "frame": map[string]any{"id": 13, "method": "mcpServer/elicitation/request", "params": map[string]any{
			"mode": "url", "serverName": "cad", "message": "open", "url": "https://x"}}},
		{"op": "send", "frame": map[string]any{"id": 14, "method": "mcpServer/elicitation/request", "params": map[string]any{
			"mode": "form", "requestedSchema": map[string]any{"required": []any{"name"}}}}},
		{"op": "send", "frame": map[string]any{"id": 15, "method": "item/tool/requestUserInput", "params": map[string]any{}}},
		{"op": "send", "frame": map[string]any{"id": 16, "method": "item/fileChange/requestApproval", "params": map[string]any{"itemId": "f1"}}},
		{"op": "send", "frame": map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": "t-9", "requestId": 16}}},
		{"op": "wait-for-reply", "var": "never"},
	}
	b, _ := json.Marshal(map[string]any{"steps": steps})
	e := setup(t, string(b))
	s := e.open(t)
	next(t, s)
	s.Prompt(context.Background(), "go")
	reqs := map[string]agentmanager.PermissionRequest{}
	for len(reqs) < 4 {
		if r, ok := next(t, s).(agentmanager.PermissionRequest); ok {
			reqs[r.RequestID] = r
		}
	}
	if r := reqs["11"]; *r.Call.Title != "ls -l" {
		t.Errorf("legacy title %q", *r.Call.Title)
	}
	if r := reqs["13"]; r.Call.MCPServer != "cad" || len(r.Options) != 2 {
		t.Errorf("elicitation %#v", r)
	}
	for id, opt := range map[string]string{"11": "accept", "12": "acceptForSession", "13": "decline"} {
		if err := s.RespondPermission(id, agentmanager.PermissionAnswer{OptionID: opt}); err != nil {
			t.Fatalf("respond %s: %v", id, err)
		}
	}
	if err := s.RespondPermission("16", agentmanager.PermissionAnswer{OptionID: "accept"}); err == nil {
		t.Error("withdrawn request still answerable")
	}
	if err := s.RespondPermission("15", agentmanager.PermissionAnswer{Cancelled: true}); err == nil {
		t.Error("user input request must not be parked")
	}
	want := map[string]string{
		"11": `{"decision":"approved"}`,
		"12": `{"permissions":{"network":{"enabled":true}},"scope":"session"}`,
		"13": `{"_meta":null,"action":"decline","content":null}`,
		"14": `{"_meta":null,"action":"cancel","content":null}`,
		"15": `{"answers":{}}`,
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := replies(logLines(t, e))
		ok := true
		for id, w := range want {
			if got[id] != w {
				ok = false
			}
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("replies %v want %v", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// replies maps request id to the compact result of each response line the client wrote.
func replies(log string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(log, "\n") {
		var m struct {
			ID     json.Number     `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal([]byte(l), &m) != nil || m.Method != "" || m.Result == nil {
			continue
		}
		var v any
		json.Unmarshal(m.Result, &v)
		b, _ := json.Marshal(v)
		out[m.ID.String()] = string(b)
	}
	return out
}
