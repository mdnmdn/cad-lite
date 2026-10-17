package acp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/acp"
	"agentmanager/internal/testutil"
	"agentmanager/mcp"
)

type env struct {
	t      *testing.T
	dir    string
	log    string
	argv   string
	cfg    agentmanager.Config
	opener agentmanager.Opener
}

func newEnv(t *testing.T, scenario string) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{t: t, dir: dir, log: filepath.Join(dir, "log"), argv: filepath.Join(dir, "argv")}
	sc := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(sc, []byte(scenario), 0o600); err != nil {
		t.Fatal(err)
	}
	e.cfg = agentmanager.Config{
		Harness: agentmanager.OpenCode,
		Binary:  testutil.BuildFake(t),
		Args:    []string{"acp"},
		Cwd:     dir,
		Env:     []string{"FAKEAGENT_SCENARIO=" + sc, "FAKEAGENT_LOG=" + e.log, "FAKEAGENT_ARGV=" + e.argv},
	}
	e.opener = acp.New(acp.Command{})
	return e
}

func builtin(t *testing.T, name string) *env {
	return newEnv(t, `{"builtin":"`+name+`"}`)
}

func (e *env) open() agentmanager.Session {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := e.opener.Open(ctx, e.cfg)
	if err != nil {
		e.t.Fatalf("open: %v", err)
	}
	e.t.Cleanup(func() { s.Close() })
	return s
}

func (e *env) frames() []map[string]any {
	b, _ := os.ReadFile(e.log)
	var out []map[string]any
	for _, l := range strings.Split(string(b), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (e *env) find(method string) map[string]any {
	for _, f := range e.frames() {
		if f["method"] == method {
			return f
		}
	}
	return nil
}

func next(t *testing.T, s agentmanager.Session) (agentmanager.Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		return ev, ok
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return nil, false
}

// until consumes events through the first one satisfying pred and returns all consumed.
func until(t *testing.T, s agentmanager.Session, pred func(agentmanager.Event) bool) []agentmanager.Event {
	t.Helper()
	var got []agentmanager.Event
	for {
		ev, ok := next(t, s)
		if !ok {
			t.Fatalf("events closed before match; got %#v", got)
		}
		got = append(got, ev)
		if pred(ev) {
			return got
		}
	}
}

func untilTurnEnd(t *testing.T, s agentmanager.Session) []agentmanager.Event {
	return until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.TurnEnd); return ok })
}

func prompt(t *testing.T, s agentmanager.Session, text string) {
	t.Helper()
	if err := s.Prompt(context.Background(), text); err != nil {
		t.Fatal(err)
	}
}

func lastTurnEnd(evs []agentmanager.Event) agentmanager.TurnEnd {
	return evs[len(evs)-1].(agentmanager.TurnEnd)
}

func drainToExit(t *testing.T, s agentmanager.Session) agentmanager.Exited {
	t.Helper()
	var ex agentmanager.Exited
	seen := false
	for {
		ev, ok := next(t, s)
		if !ok {
			break
		}
		if e, ok := ev.(agentmanager.Exited); ok {
			ex, seen = e, true
		}
	}
	if !seen {
		t.Fatal("no Exited event")
	}
	return ex
}

func TestHandshakeAndStreamingTurn(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	if s.ID() != "sess_fake" {
		t.Fatalf("ID = %q", s.ID())
	}
	ev, _ := next(t, s)
	if st, ok := ev.(agentmanager.SessionStarted); !ok || st.SessionID != "sess_fake" || st.Model != "fake-1" {
		t.Fatalf("first event %#v", ev)
	}
	prompt(t, s, "hello")
	evs := untilTurnEnd(t, s)
	want := []agentmanager.Event{
		agentmanager.ThoughtDelta{Text: "thinking"},
		agentmanager.TextDelta{Text: "echo: "},
		agentmanager.TextDelta{Text: "hello"},
		agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn},
	}
	if len(evs) != len(want) {
		t.Fatalf("events %#v", evs)
	}
	for i := range want {
		if evs[i] != want[i] {
			t.Fatalf("event %d = %#v, want %#v", i, evs[i], want[i])
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if ex := drainToExit(t, s); ex.Err != "" {
		t.Fatalf("Exited.Err = %q", ex.Err)
	}
	if err := s.Prompt(context.Background(), "x"); !errors.Is(err, agentmanager.ErrClosed) {
		t.Fatalf("Prompt after close = %v", err)
	}
	if err := s.Cancel(); !errors.Is(err, agentmanager.ErrClosed) {
		t.Fatalf("Cancel after close = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("second Close:", err)
	}

	init := e.find("initialize")["params"].(map[string]any)
	if init["protocolVersion"] != float64(1) {
		t.Fatalf("protocolVersion %#v", init["protocolVersion"])
	}
	caps := init["clientCapabilities"].(map[string]any)
	if caps["terminal"] != false || caps["fs"].(map[string]any)["readTextFile"] != false {
		t.Fatalf("clientCapabilities %#v", caps)
	}
	nw := e.find("session/new")["params"].(map[string]any)
	if nw["cwd"] != e.dir {
		t.Fatalf("cwd %#v", nw["cwd"])
	}
	if l, ok := nw["mcpServers"].([]any); !ok || len(l) != 0 {
		t.Fatalf("mcpServers must be an empty array, got %#v", nw["mcpServers"])
	}
	p := e.find("session/prompt")["params"].(map[string]any)
	item := p["prompt"].([]any)[0].(map[string]any)
	if p["sessionId"] != "sess_fake" || item["type"] != "text" || item["text"] != "hello" {
		t.Fatalf("prompt params %#v", p)
	}
}

func TestSystemPromptPrefixesFirstPromptOnly(t *testing.T) {
	e := builtin(t, "echo-turn")
	e.cfg.SystemPrompt = "SYS"
	s := e.open()
	prompt(t, s, "one")
	untilTurnEnd(t, s)
	prompt(t, s, "two")
	untilTurnEnd(t, s)
	var texts []string
	for _, f := range e.frames() {
		if f["method"] == "session/prompt" {
			texts = append(texts, f["params"].(map[string]any)["prompt"].([]any)[0].(map[string]any)["text"].(string))
		}
	}
	if len(texts) != 2 || texts[0] != "SYS\n\none" || texts[1] != "two" {
		t.Fatalf("prompts %q", texts)
	}
}

func TestModelSelectedOverTheWire(t *testing.T) {
	e := builtin(t, "echo-turn")
	e.cfg.Model = "fancy-2"
	s := e.open()
	ev, _ := next(t, s)
	if st := ev.(agentmanager.SessionStarted); st.Model != "fancy-2" {
		t.Fatalf("model %q", st.Model)
	}
	p := e.find("session/set_config_option")["params"].(map[string]any)
	if p["configId"] != "model" || p["value"] != "fancy-2" || p["sessionId"] != "sess_fake" {
		t.Fatalf("set_config_option %#v", p)
	}
}

func TestPromptWhileRunningAndCancelMidTurn(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	if err := s.Cancel(); err != nil {
		t.Fatalf("idle Cancel must be a no-op: %v", err)
	}
	prompt(t, s, "hang")
	if err := s.Prompt(context.Background(), "second"); err == nil {
		t.Fatal("second prompt while running must fail")
	}
	until(t, s, func(ev agentmanager.Event) bool {
		d, ok := ev.(agentmanager.TextDelta)
		return ok && d.Text == "working"
	})
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	evs := untilTurnEnd(t, s)
	if te := lastTurnEnd(evs); te.Reason != agentmanager.StopCancelled {
		t.Fatalf("turn end %#v", te)
	}
	prompt(t, s, "again")
	evs = untilTurnEnd(t, s)
	if te := lastTurnEnd(evs); te.Reason != agentmanager.StopEndTurn {
		t.Fatalf("turn end %#v", te)
	}
}

func TestToolCallPermissionAnswered(t *testing.T) {
	e := builtin(t, "tool-turn")
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: "http://127.0.0.1:1/mcp", Token: "tok"}}
	s := e.open()
	prompt(t, s, "draw")
	evs := until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
	var start agentmanager.ToolCallStart
	for _, ev := range evs {
		if st, ok := ev.(agentmanager.ToolCallStart); ok {
			start = st
		}
	}
	c := start.Call
	if c.ID != "call_1" || c.Title != "cad_add_line" || c.Kind != agentmanager.KindOther || c.Status != agentmanager.ToolPending ||
		c.MCPServer != "cad" || c.MCPTool != "add_line" || !strings.Contains(string(c.RawInput), `"x2":10`) {
		t.Fatalf("tool call %#v", c)
	}
	req := evs[len(evs)-1].(agentmanager.PermissionRequest)
	if req.RequestID != "s:p1" || req.Call.ID != "call_1" || req.Call.MCPServer != "cad" || len(req.Options) != 3 ||
		req.Options[0] != (agentmanager.PermissionOption{ID: "once", Name: "Allow once", Kind: agentmanager.AllowOnce}) {
		t.Fatalf("request %#v", req)
	}
	if err := s.RespondPermission("s:nope", agentmanager.PermissionAnswer{OptionID: "once"}); err == nil {
		t.Fatal("unknown request id must error")
	}
	if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{OptionID: "once"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{OptionID: "once"}); err == nil {
		t.Fatal("answering twice must error")
	}
	evs = untilTurnEnd(t, s)
	var statuses []agentmanager.ToolStatus
	var out string
	for _, ev := range evs {
		if u, ok := ev.(agentmanager.ToolCallUpdate); ok {
			if u.Update.MCPServer != "cad" {
				t.Fatalf("update lost MCPServer: %#v", u.Update)
			}
			if u.Update.Status != nil {
				statuses = append(statuses, *u.Update.Status)
			}
			if u.Update.Output != nil {
				out = *u.Update.Output
			}
		}
	}
	if len(statuses) != 2 || statuses[0] != agentmanager.ToolInProgress || statuses[1] != agentmanager.ToolCompleted || out != "ok" {
		t.Fatalf("statuses %v output %q", statuses, out)
	}
	if te := lastTurnEnd(evs); te.Reason != agentmanager.StopEndTurn {
		t.Fatalf("turn end %#v", te)
	}
	for _, f := range e.frames() {
		if f["id"] == "p1" {
			b, _ := json.Marshal(f["result"])
			if string(b) != `{"outcome":{"optionId":"once","outcome":"selected"}}` {
				t.Fatalf("permission reply %s", b)
			}
			return
		}
	}
	t.Fatal("no permission reply logged")
}

func TestPermissionRejectAndCancelledAnswer(t *testing.T) {
	e := builtin(t, "tool-turn")
	s := e.open()
	prompt(t, s, "draw")
	evs := until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
	req := evs[len(evs)-1].(agentmanager.PermissionRequest)
	if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{OptionID: "reject"}); err != nil {
		t.Fatal(err)
	}
	evs = untilTurnEnd(t, s)
	if u := evs[0].(agentmanager.ToolCallUpdate).Update; u.Status == nil || *u.Status != agentmanager.ToolFailed {
		t.Fatalf("update %#v", u)
	}
	prompt(t, s, "draw")
	evs = until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
	req = evs[len(evs)-1].(agentmanager.PermissionRequest)
	if err := s.RespondPermission(req.RequestID, agentmanager.PermissionAnswer{Cancelled: true}); err != nil {
		t.Fatal(err)
	}
	if te := lastTurnEnd(untilTurnEnd(t, s)); te.Reason != agentmanager.StopCancelled {
		t.Fatalf("turn end %#v", te)
	}
}

func TestPolicyAutoAllowsMCP(t *testing.T) {
	e := builtin(t, "tool-turn")
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: "http://127.0.0.1:1/mcp"}}
	var asked atomic.Int32
	allow := agentmanager.AllowMCP("cad")
	e.cfg.AutoApprove = func(r agentmanager.PermissionRequest) (string, bool) {
		asked.Add(1)
		return allow(r)
	}
	s := e.open()
	prompt(t, s, "draw")
	evs := untilTurnEnd(t, s)
	for _, ev := range evs {
		if _, ok := ev.(agentmanager.PermissionRequest); ok {
			t.Fatal("policy-answered request must not reach the UI")
		}
	}
	if asked.Load() != 1 {
		t.Fatalf("policy asked %d times", asked.Load())
	}
	if te := lastTurnEnd(evs); te.Reason != agentmanager.StopEndTurn {
		t.Fatalf("turn end %#v", te)
	}
}

func TestPolicyDeclineGoesToUI(t *testing.T) {
	e := builtin(t, "tool-turn")
	e.cfg.AutoApprove = agentmanager.AllowMCP("other")
	s := e.open()
	prompt(t, s, "draw")
	until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
}

func TestCancelWithPendingPermissionOrdering(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	prompt(t, s, "tool please")
	until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	if te := lastTurnEnd(untilTurnEnd(t, s)); te.Reason != agentmanager.StopCancelled {
		t.Fatalf("turn end %#v", te)
	}
	prompt(t, s, "hello")
	untilTurnEnd(t, s)
	s.Close()
	reply, cancel := -1, -1
	for i, f := range e.frames() {
		if f["id"] == "p1" {
			b, _ := json.Marshal(f["result"])
			if string(b) != `{"outcome":{"outcome":"cancelled"}}` {
				t.Fatalf("reply %s", b)
			}
			reply = i
		}
		if f["method"] == "session/cancel" {
			if _, hasID := f["id"]; hasID {
				t.Fatal("session/cancel must be a notification")
			}
			cancel = i
		}
	}
	if reply < 0 || cancel < 0 || reply > cancel {
		t.Fatalf("permission reply at %d must precede session/cancel at %d", reply, cancel)
	}
}

func TestAgentCrashMidTurn(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	prompt(t, s, "crash")
	evs := until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.Exited); return ok })
	for _, ev := range evs {
		if _, ok := ev.(agentmanager.TurnEnd); ok {
			t.Fatalf("crashed turn must be abandoned, got %#v", evs)
		}
	}
	ex := evs[len(evs)-1].(agentmanager.Exited)
	if !strings.Contains(ex.Err, "boom") || !strings.Contains(ex.Err, "exit status 3") {
		t.Fatalf("Exited.Err = %q", ex.Err)
	}
	if _, ok := <-s.Events(); ok {
		t.Fatal("events must close after Exited")
	}
	if err := s.Prompt(context.Background(), "x"); !errors.Is(err, agentmanager.ErrClosed) {
		t.Fatalf("Prompt = %v", err)
	}
	if err := s.RespondPermission("s:p1", agentmanager.PermissionAnswer{Cancelled: true}); !errors.Is(err, agentmanager.ErrClosed) {
		t.Fatalf("RespondPermission = %v", err)
	}
}

func TestPromptErrorFailsTurnOnly(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	prompt(t, s, "fail")
	te := lastTurnEnd(untilTurnEnd(t, s))
	if te.Reason != agentmanager.StopFailed || te.Err != "model exploded" {
		t.Fatalf("turn end %#v", te)
	}
	prompt(t, s, "ok")
	if te := lastTurnEnd(untilTurnEnd(t, s)); te.Reason != agentmanager.StopEndTurn {
		t.Fatalf("turn end %#v", te)
	}
}

func TestLargeLine(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	prompt(t, s, "big")
	for _, ev := range untilTurnEnd(t, s) {
		if d, ok := ev.(agentmanager.TextDelta); ok {
			if len(d.Text) != 1<<20 {
				t.Fatalf("len %d", len(d.Text))
			}
			return
		}
	}
	t.Fatal("no delta")
}

func TestUnknownInboundRequestGetsMethodNotFoundOnce(t *testing.T) {
	e := builtin(t, "echo-turn,probe")
	s := e.open()
	prompt(t, s, "hello")
	untilTurnEnd(t, s)
	s.Close()
	n := 0
	for _, f := range e.frames() {
		if f["id"] == "probe1" {
			n++
			if f["error"].(map[string]any)["code"] != float64(-32601) {
				t.Fatalf("reply %#v", f)
			}
		}
	}
	if n != 1 {
		t.Fatalf("probe replied %d times", n)
	}
}

func TestMCPServerPassedInSessionNewAndCalled(t *testing.T) {
	var calls atomic.Int32
	srv := mcp.NewServer(mcp.Options{Name: "cad", Version: "0"})
	srv.Register(mcp.Tool{Name: "add_line", Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
		calls.Add(1)
		return mcp.Text("line added " + string(args)), nil
	}})
	ep, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	e := builtin(t, "tool-turn")
	e.cfg.Env = append(e.cfg.Env, "FAKEAGENT_MCP_TOOL=add_line", `FAKEAGENT_MCP_ARGS={"n":1}`)
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: ep.URL, Token: ep.Token}}
	e.cfg.AutoApprove = agentmanager.AllowMCP("cad")
	lg := &capture{}
	e.cfg.Logger = lg
	s := e.open()
	prompt(t, s, "draw a line")
	evs := untilTurnEnd(t, s)
	var out string
	for _, ev := range evs {
		if u, ok := ev.(agentmanager.ToolCallUpdate); ok && u.Update.Output != nil {
			out = *u.Update.Output
		}
	}
	if out != `line added {"n":1}` || calls.Load() != 1 {
		t.Fatalf("output %q calls %d", out, calls.Load())
	}
	s.Close()

	list := e.find("session/new")["params"].(map[string]any)["mcpServers"].([]any)
	if len(list) != 1 {
		t.Fatalf("mcpServers %#v", list)
	}
	m := list[0].(map[string]any)
	h := m["headers"].([]any)[0].(map[string]any)
	if m["type"] != "http" || m["name"] != "cad" || m["url"] != ep.URL || h["name"] != "Authorization" || h["value"] != "Bearer "+ep.Token {
		t.Fatalf("entry %#v", m)
	}
	if lg.len() == 0 {
		t.Fatal("expected debug frames")
	}
	if strings.Contains(lg.all(), ep.Token) {
		t.Fatal("token leaked into the log")
	}
}

func TestOpenCodePresetEnvFallbackWhenNoHTTP(t *testing.T) {
	var calls atomic.Int32
	srv := mcp.NewServer(mcp.Options{Name: "cad", Version: "0"})
	srv.Register(mcp.Tool{Name: "add_line", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		calls.Add(1)
		return mcp.Text("fine"), nil
	}})
	ep, _ := srv.Start()
	defer srv.Close()

	e := builtin(t, "tool-turn,nohttp")
	e.opener = acp.OpenCode()
	e.cfg.Env = append(e.cfg.Env, "FAKEAGENT_MCP_TOOL=add_line",
		`OPENCODE_CONFIG_CONTENT={"provider":{"ollama":{"options":{"apiKey":"{env:OLLAMA_KEY}"}}},"model":"ollama/gpt-oss:120b","mcp":{"existing":{"type":"remote","url":"http://localhost/existing"}}}`)
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: ep.URL, Token: ep.Token}}
	e.cfg.AutoApprove = agentmanager.AllowMCP("cad")
	s := e.open()
	prompt(t, s, "draw")
	untilTurnEnd(t, s)
	if calls.Load() != 1 {
		t.Fatalf("calls %d", calls.Load())
	}
	argv, _ := os.ReadFile(e.argv)
	a := string(argv)
	if !strings.HasPrefix(a, "ARGV\nacp\nENV\n") && !strings.HasPrefix(a, "ARGV\nacp\n") {
		t.Fatalf("argv dump %q", a[:40])
	}
	var cfgLine string
	for _, l := range strings.Split(a, "\n") {
		if strings.HasPrefix(l, "OPENCODE_CONFIG_CONTENT=") {
			cfgLine = strings.TrimPrefix(l, "OPENCODE_CONFIG_CONTENT=")
		}
	}
	var got struct {
		Provider map[string]json.RawMessage `json:"provider"`
		Model    string                     `json:"model"`
		MCP      map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			Enabled bool              `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(cfgLine), &got); err != nil {
		t.Fatalf("config env %q: %v", cfgLine, err)
	}
	if len(got.Provider["ollama"]) == 0 || got.Model != "ollama/gpt-oss:120b" || got.MCP["existing"].URL != "http://localhost/existing" {
		t.Fatal("provider, model or existing MCP config lost on fallback")
	}
	c := got.MCP["cad"]
	if c.Type != "remote" || c.URL != ep.URL || !c.Enabled || c.Headers["Authorization"] != "Bearer "+ep.Token {
		t.Fatalf("config %#v", c)
	}
	if !strings.Contains(a, "OPENCODE_DISABLE_CLAUDE_CODE=1") {
		t.Fatal("preset env missing")
	}
	var news []map[string]any
	for _, f := range e.frames() {
		if f["method"] == "session/new" {
			news = append(news, f)
		}
	}
	if len(news) != 1 || len(news[0]["params"].(map[string]any)["mcpServers"].([]any)) != 0 {
		t.Fatalf("session/new frames %#v", news)
	}
}

func TestNoHTTPWithoutFallbackWarns(t *testing.T) {
	e := builtin(t, "echo-turn,nohttp")
	e.cfg.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: "http://127.0.0.1:1/mcp"}}
	s := e.open()
	evs := until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.SessionStarted); return ok })
	warned := false
	for _, ev := range evs {
		if l, ok := ev.(agentmanager.Log); ok && l.Level == "warn" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a warn Log, got %#v", evs)
	}
}

func TestHandshakeErrors(t *testing.T) {
	open := func(e *env) error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s, err := e.opener.Open(ctx, e.cfg)
		if err == nil {
			s.Close()
		}
		return err
	}
	if err := open(builtin(t, "echo-turn,badversion")); err == nil || !strings.Contains(err.Error(), "protocol version 2") {
		t.Fatalf("badversion: %v", err)
	}
	e := builtin(t, "echo-turn,authreq")
	e.opener = acp.OpenCode()
	if err := open(e); err == nil || !strings.Contains(err.Error(), "opencode auth login") || !strings.Contains(err.Error(), "default") {
		t.Fatalf("authreq: %v", err)
	}
	e = newEnv(t, `{"steps":[{"op":"stderr","frame":{"text":"cannot start"}},{"op":"exit","code":1}]}`)
	if err := open(e); err == nil || !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("early exit: %v", err)
	}
	defer acp.SetHandshakeTimeout(300 * time.Millisecond)()
	e = newEnv(t, `{"steps":[{"op":"hang"}]}`)
	if err := open(e); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestAuthenticateWhenDefaultMethodAdvertised(t *testing.T) {
	e := builtin(t, "echo-turn,authdefault")
	e.open()
	a := e.find("authenticate")
	if a == nil || a["params"].(map[string]any)["methodId"] != "default" {
		t.Fatalf("authenticate %#v", a)
	}
}

func TestResume(t *testing.T) {
	e := builtin(t, "echo-turn")
	e.cfg.ResumeID = "sess_old"
	s := e.open()
	if s.ID() != "sess_old" {
		t.Fatalf("ID %q", s.ID())
	}
	prompt(t, s, "hi")
	evs := untilTurnEnd(t, s)
	for _, ev := range evs {
		if d, ok := ev.(agentmanager.TextDelta); ok && strings.Contains(d.Text, "old") {
			t.Fatalf("replayed history leaked: %#v", evs)
		}
	}
	if l := e.find("session/load")["params"].(map[string]any); l["sessionId"] != "sess_old" || l["cwd"] != e.dir {
		t.Fatalf("load %#v", l)
	}
	if e.find("session/new") != nil {
		t.Fatal("must not call session/new when resuming")
	}

	e = builtin(t, "echo-turn,noload")
	e.cfg.ResumeID = "sess_old"
	if _, err := e.opener.Open(context.Background(), e.cfg); err == nil {
		t.Fatal("resume without loadSession must fail")
	}
}

const handshakeSteps = `{"op":"expect","match":{"method":"initialize"},"var":"i"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$i","result":{"protocolVersion":1,"agentCapabilities":{"mcpCapabilities":{"http":true}}}}},
 {"op":"expect","match":{"method":"session/new"},"var":"n"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$n","result":{"sessionId":"s1"}}},`

func TestUpdateMapping(t *testing.T) {
	e := newEnv(t, `{"steps":[`+handshakeSteps+`
 {"op":"expect","match":{"method":"session/prompt"},"var":"p"},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"echoed"}}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":"hi"}}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"AAAA"}}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"t1","title":"Read file","kind":"teleport","status":"in_progress","rawInput":{"path":"a"}}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call_update","toolCallId":"t1","status":"completed","content":[{"type":"content","content":{"type":"text","text":"line1"}},{"type":"diff","path":"a.go","oldText":"x","newText":"y"}],"rawOutput":{"ok":true}}}}},
 {"op":"send","frame":{"method":"session/update","params":{"sessionUpdate":"plan","entries":[{"content":"a","priority":"high","status":"completed"},{"content":"b","priority":"low","status":"weird"}]}}},
 {"op":"send","frame":{"method":"session/update","params":"garbage"}},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$p","result":{"stopReason":"max_tokens"}}},
 {"op":"hang"}]}`)
	t.Cleanup(acp.SetCloseGrace(300 * time.Millisecond))
	s := e.open()
	prompt(t, s, "go")
	evs := untilTurnEnd(t, s)
	var kinds []string
	for _, ev := range evs {
		switch v := ev.(type) {
		case agentmanager.SessionStarted:
			kinds = append(kinds, "started")
		case agentmanager.TextDelta:
			if v.MessageID != "m1" || v.Text != "hi" {
				t.Fatalf("delta %#v", v)
			}
			kinds = append(kinds, "text")
		case agentmanager.ToolCallStart:
			c := v.Call
			if c.ID != "t1" || c.Title != "Read file" || c.Kind != agentmanager.KindOther || c.Status != agentmanager.ToolInProgress || string(c.RawInput) != `{"path":"a"}` {
				t.Fatalf("start %#v", c)
			}
			kinds = append(kinds, "start")
		case agentmanager.ToolCallUpdate:
			u := v.Update
			if u.Title != nil || u.Kind != nil || *u.Status != agentmanager.ToolCompleted || *u.Output != "line1\ndiff a.go" || string(u.RawOutput) != `{"ok":true}` {
				t.Fatalf("update %#v", u)
			}
			kinds = append(kinds, "update")
		case agentmanager.PlanUpdate:
			if len(v.Entries) != 2 || v.Entries[0] != (agentmanager.PlanEntry{Content: "a", Status: "completed"}) || v.Entries[1].Status != "pending" {
				t.Fatalf("plan %#v", v)
			}
			kinds = append(kinds, "plan")
		case agentmanager.TurnEnd:
			if v.Reason != agentmanager.StopMaxTokens {
				t.Fatalf("end %#v", v)
			}
			kinds = append(kinds, "end")
		default:
			t.Fatalf("unexpected %#v", ev)
		}
	}
	if got := strings.Join(kinds, ","); got != "started,text,start,update,plan,end" {
		t.Fatalf("sequence %s", got)
	}
}

func TestPermissionIDKeysAndNumericEcho(t *testing.T) {
	e := newEnv(t, `{"steps":[`+handshakeSteps+`
 {"op":"expect","match":{"method":"session/prompt"},"var":"p"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"sessionId":"s1","toolCall":{"toolCallId":"c"},"options":[{"optionId":"o","name":"O","kind":"allow_once"}]}}},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"7","method":"session/request_permission","params":{"sessionId":"s1","toolCall":{"toolCallId":"c"},"options":[{"optionId":"o","name":"O","kind":"allow_once"}]}}},
 {"op":"expect","match":{"id":"7","result":{"outcome":"*"}}},
 {"op":"expect","match":{"id":"7","result":{"outcome":"*"}}},
 {"op":"hang"}]}`)
	t.Cleanup(acp.SetCloseGrace(300 * time.Millisecond))
	s := e.open()
	prompt(t, s, "go")
	var ids []string
	for len(ids) < 2 {
		ev, _ := next(t, s)
		if r, ok := ev.(agentmanager.PermissionRequest); ok {
			ids = append(ids, r.RequestID)
		}
	}
	if ids[0] != "n:7" || ids[1] != "s:7" {
		t.Fatalf("ids %v", ids)
	}
	s.RespondPermission("n:7", agentmanager.PermissionAnswer{OptionID: "o"})
	s.RespondPermission("s:7", agentmanager.PermissionAnswer{Cancelled: true})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var num, str bool
		for _, f := range e.frames() {
			if string(mustJSON(f["id"])) == "7" && f["result"] != nil {
				num = true
			}
			if f["id"] == "7" && f["result"] != nil {
				str = true
			}
		}
		if num && str {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("did not see both a numeric and a string id reply")
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestCloseKillsAgentIgnoringStdinEOF(t *testing.T) {
	e := newEnv(t, `{"steps":[`+handshakeSteps+`{"op":"hang"}]}`)
	t.Cleanup(acp.SetCloseGrace(300 * time.Millisecond))
	s := e.open()
	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatal("Close took too long")
	}
	if ex := drainToExit(t, s); ex.Err != "" {
		t.Fatalf("deliberate close must not report an error: %q", ex.Err)
	}
}

func TestCloseWithPendingPermissionAnswersCancelled(t *testing.T) {
	e := builtin(t, "tool-turn")
	s := e.open()
	prompt(t, s, "draw")
	until(t, s, func(ev agentmanager.Event) bool { _, ok := ev.(agentmanager.PermissionRequest); return ok })
	s.Close()
	found := false
	for _, f := range e.frames() {
		if f["id"] == "p1" {
			found = true
		}
	}
	if !found {
		t.Fatal("pending permission was not answered on Close")
	}
}

type capture struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *capture) Debugf(f string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.b.WriteString(strings.TrimSpace(fmt.Sprintf(f, a...)) + "\n")
}
func (c *capture) all() string { c.mu.Lock(); defer c.mu.Unlock(); return c.b.String() }
func (c *capture) len() int    { return len(c.all()) }
