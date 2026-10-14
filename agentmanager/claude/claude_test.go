package claude_test

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
	"agentmanager/claude"
	"agentmanager/internal/testutil"
)

type harness struct {
	t    *testing.T
	s    agentmanager.Session
	log  string
	argv string
}

type opt func(*agentmanager.Config)

func open(t *testing.T, scenario any, opts ...opt) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, log: filepath.Join(dir, "log"), argv: filepath.Join(dir, "argv")}
	sc := filepath.Join(dir, "scenario.json")
	b, _ := json.Marshal(scenario)
	if err := os.WriteFile(sc, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := agentmanager.Config{
		Harness: agentmanager.Claude,
		Binary:  testutil.BuildFake(t),
		Args:    []string{"claude"},
		Cwd:     dir,
		Env:     []string{"FAKEAGENT_SCENARIO=" + sc, "FAKEAGENT_LOG=" + h.log, "FAKEAGENT_ARGV=" + h.argv},
	}
	for _, o := range opts {
		o(&cfg)
	}
	s, err := claude.New().Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.s = s
	t.Cleanup(func() { s.Close() })
	return h
}

func builtin(name string) map[string]any { return map[string]any{"builtin": name} }

func (h *harness) next() agentmanager.Event {
	h.t.Helper()
	select {
	case ev, ok := <-h.s.Events():
		if !ok {
			h.t.Fatal("events closed")
		}
		return ev
	case <-time.After(10 * time.Second):
		h.t.Fatal("timeout waiting for event")
	}
	return nil
}

func expect[T agentmanager.Event](h *harness) T {
	h.t.Helper()
	ev := h.next()
	v, ok := ev.(T)
	if !ok {
		h.t.Fatalf("got %T %+v, want %T", ev, ev, v)
	}
	return v
}

func (h *harness) prompt(text string) {
	h.t.Helper()
	if err := h.s.Prompt(context.Background(), text); err != nil {
		h.t.Fatal(err)
	}
}

// logLines returns the frames the client wrote, without the initialize control request every
// session opens with (see TestModelsListedAndSwitched).
func (h *harness) logLines() []string {
	var out []string
	for _, l := range h.allLines() {
		if !strings.Contains(l, `"subtype":"initialize"`) {
			out = append(out, l)
		}
	}
	return out
}

func (h *harness) allLines() []string {
	b, _ := os.ReadFile(h.log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestModelsListedAndSwitched(t *testing.T) {
	h := open(t, builtin("echo-turn"), func(c *agentmanager.Config) {
		c.Model = "opus"
		c.Env = append(c.Env, "FAKEAGENT_MODELS=1")
	})
	mc := expect[agentmanager.ModelsChanged](h)
	if mc.Current != "opus" || len(mc.Available) != 2 || mc.Available[1] != (agentmanager.ModelInfo{ID: "fake-fast", Name: "Fake Fast", Vision: agentmanager.VisionYes}) {
		t.Fatalf("%+v", mc)
	}
	if first := h.allLines()[0]; !strings.Contains(first, `"type":"control_request"`) || !strings.Contains(first, `"subtype":"initialize"`) {
		t.Errorf("first frame %s", first)
	}
	ms, ok := h.s.(agentmanager.ModelSetter)
	if !ok {
		t.Fatal("claude session is not a ModelSetter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ms.SetModel(ctx, "fake-fast"); err != nil {
		t.Fatal(err)
	}
	if mc := expect[agentmanager.ModelsChanged](h); mc.Current != "fake-fast" || mc.Available != nil {
		t.Errorf("%+v", mc)
	}
	if err := ms.SetModel(ctx, "bad-model"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("bad model: %v", err)
	}
	if err := ms.SetModel(ctx, ""); !errors.Is(err, agentmanager.ErrNoModel) {
		t.Errorf("empty model: %v", err)
	}
	h.prompt("hi")
	if ss := expect[agentmanager.SessionStarted](h); ss.Model != "fake-fast" {
		t.Errorf("model after switch %q", ss.Model)
	}
}

func TestModelsUnansweredInitialize(t *testing.T) {
	h := open(t, builtin("echo-turn"))
	h.prompt("hi")
	expect[agentmanager.SessionStarted](h)
	expect[agentmanager.TextDelta](h)
	expect[agentmanager.TurnEnd](h)
}

func waitFile(t *testing.T, path string) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "CWD") {
			return string(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never written", path)
	return ""
}

func TestLaunchArgvAndMCPConfig(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_SSE_PORT", "9")
	h := open(t, builtin("echo-turn"), func(c *agentmanager.Config) {
		c.Model = "opus"
		c.ResumeID = "sess-9"
		c.SystemPrompt = "be brief"
		c.MCPServers = []agentmanager.MCPServer{{Name: "cad", URL: "http://127.0.0.1:1234/mcp", Token: "s3cret"}}
		c.Args = append(c.Args, "--extra")
		c.Env = append(c.Env, "KEEP_ME=1")
	})
	dump := waitFile(t, h.argv)
	argvPart, envPart, _ := strings.Cut(dump, "ENV\n")
	args := strings.Split(strings.TrimSpace(strings.TrimPrefix(argvPart, "ARGV\n")), "\n")
	idx := map[string]int{}
	for i, a := range args {
		idx[a] = i
	}
	for _, f := range []string{"-p", "--input-format", "--output-format", "--verbose", "--include-partial-messages",
		"--permission-prompt-tool", "--disallowedTools", "--mcp-config", "--strict-mcp-config", "--extra"} {
		if _, ok := idx[f]; !ok {
			t.Errorf("missing %s in %v", f, args)
		}
	}
	for flag, val := range map[string]string{"--input-format": "stream-json", "--output-format": "stream-json",
		"--permission-prompt-tool": "stdio", "--disallowedTools": "AskUserQuestion", "--model": "opus",
		"--resume": "sess-9", "--append-system-prompt": "be brief"} {
		if i, ok := idx[flag]; !ok || args[i+1] != val {
			t.Errorf("%s: want %q in %v", flag, val, args)
		}
	}
	for _, a := range args {
		if strings.Contains(a, "dangerously") || strings.Contains(a, "s3cret") {
			t.Errorf("forbidden argv %q", a)
		}
	}
	if args[len(args)-1] != "--extra" {
		t.Errorf("Config.Args not last: %v", args)
	}
	if strings.Contains(envPart, "CLAUDECODE=") || strings.Contains(envPart, "CLAUDE_CODE_SSE_PORT=") {
		t.Error("parent Claude env leaked")
	}
	if !strings.Contains(envPart, "KEEP_ME=1") {
		t.Error("Config.Env missing")
	}
	path := args[idx["--mcp-config"]+1]
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mcp config mode %v", st.Mode().Perm())
	}
	b, _ := os.ReadFile(path)
	var cfg map[string]map[string]map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	cad := cfg["mcpServers"]["cad"]
	if cad["type"] != "http" || cad["url"] != "http://127.0.0.1:1234/mcp" ||
		cad["headers"].(map[string]any)["Authorization"] != "Bearer s3cret" {
		t.Errorf("mcp entry %v", cad)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("mcp config not removed on Close")
	}
	if err := h.s.Close(); err != nil {
		t.Error("second Close", err)
	}
	if err := h.s.Prompt(context.Background(), "x"); err != agentmanager.ErrClosed {
		t.Errorf("Prompt after Close = %v", err)
	}
	last := h.next()
	for {
		if _, ok := last.(agentmanager.Exited); ok {
			break
		}
		last = h.next()
	}
	if last.(agentmanager.Exited).Err != "" {
		t.Errorf("clean close Exited.Err = %q", last.(agentmanager.Exited).Err)
	}
}

func TestStreamingTurn(t *testing.T) {
	h := open(t, builtin("echo-turn"), func(c *agentmanager.Config) { c.ResumeID = "" })
	if h.s.ID() != "" {
		t.Error("ID before init")
	}
	h.prompt("hi")
	ss := expect[agentmanager.SessionStarted](h)
	if ss.SessionID != "fake-session-1" || ss.Model != "fake-model" || h.s.ID() != "fake-session-1" {
		t.Errorf("%+v id=%q", ss, h.s.ID())
	}
	if td := expect[agentmanager.TextDelta](h); td.Text != "echo: hi" || td.MessageID == "" {
		t.Errorf("%+v", td)
	}
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopEndTurn {
		t.Errorf("%+v", te)
	}
	h.prompt("again")
	if td := expect[agentmanager.TextDelta](h); td.Text != "echo: again" {
		t.Errorf("%+v", td)
	}
	expect[agentmanager.TurnEnd](h)
	var user map[string]any
	if err := json.Unmarshal([]byte(h.logLines()[0]), &user); err != nil {
		t.Fatal(err)
	}
	want := `{"message":{"content":[{"text":"hi","type":"text"}],"role":"user"},"type":"user"}`
	got, _ := json.Marshal(user)
	if string(got) != want {
		t.Errorf("user line %s", got)
	}
}

func TestResumeUsesSessionID(t *testing.T) {
	h := open(t, builtin("echo-turn"), func(c *agentmanager.Config) { c.ResumeID = "old-1" })
	h.prompt("x")
	if ss := expect[agentmanager.SessionStarted](h); ss.SessionID != "old-1" {
		t.Errorf("%+v", ss)
	}
}

func TestPartialDedupe(t *testing.T) {
	h := open(t, builtin("partial-turn"))
	h.prompt("w")
	expect[agentmanager.SessionStarted](h)
	var text, thought string
	for {
		switch ev := h.next().(type) {
		case agentmanager.TextDelta:
			if ev.MessageID != "msg_1" {
				t.Errorf("message id %q", ev.MessageID)
			}
			text += ev.Text
		case agentmanager.ThoughtDelta:
			thought += ev.Text
		case agentmanager.TurnEnd:
			if text != "Hello w" || thought != "hmm " {
				t.Fatalf("text %q thought %q (duplicated or missing)", text, thought)
			}
			return
		default:
			t.Fatalf("unexpected %T", ev)
		}
	}
}

func TestFinalOnlyAssistantSkipsEmptyThinking(t *testing.T) {
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "system", "subtype": "init", "session_id": "s"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "system", "subtype": "init", "session_id": "s"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "content": []any{
			map[string]any{"type": "thinking", "thinking": "", "signature": "x"},
			map[string]any{"type": "thinking", "thinking": "deep"},
			map[string]any{"type": "text", "text": "answer"},
		}}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "result", "subtype": "error_max_turns", "is_error": true}},
	}}
	h := open(t, sc)
	h.prompt("q")
	expect[agentmanager.SessionStarted](h)
	if th := expect[agentmanager.ThoughtDelta](h); th.Text != "deep" {
		t.Errorf("%+v", th)
	}
	expect[agentmanager.TextDelta](h)
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopMaxTurns {
		t.Errorf("%+v", te)
	}
}

func TestResultFailed(t *testing.T) {
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "result", "subtype": "success", "is_error": true, "result": "API Error: boom"}},
	}}
	h := open(t, sc)
	h.prompt("q")
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopFailed || te.Err != "API Error: boom" {
		t.Errorf("%+v", te)
	}
}

func toolOpts(tool string, policy agentmanager.PermissionPolicy) opt {
	return func(c *agentmanager.Config) {
		c.Env = append(c.Env, "FAKEAGENT_CLAUDE_TOOL="+tool)
		c.AutoApprove = policy
	}
}

func controlResponse(t *testing.T, h *harness) map[string]any {
	t.Helper()
	for _, l := range h.logLines() {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] == "control_response" {
			return m
		}
	}
	t.Fatalf("no control_response in %v", h.logLines())
	return nil
}

func TestToolCallAndPermissionAllow(t *testing.T) {
	h := open(t, builtin("tool-turn"), toolOpts("mcp__cad__add_line", nil))
	h.prompt("draw")
	expect[agentmanager.SessionStarted](h)
	expect[agentmanager.TextDelta](h)
	st := expect[agentmanager.ToolCallStart](h)
	c := st.Call
	if c.ID != "toolu_1" || c.MCPServer != "cad" || c.MCPTool != "add_line" || c.Kind != agentmanager.KindOther ||
		c.Status != agentmanager.ToolInProgress || c.Title != "mcp__cad__add_line" {
		t.Errorf("%+v", c)
	}
	pr := expect[agentmanager.PermissionRequest](h)
	if pr.RequestID != "req-1" || pr.Call.ID != "toolu_1" || pr.Call.MCPServer != "cad" || len(pr.Call.RawInput) == 0 {
		t.Errorf("%+v", pr)
	}
	var ids []string
	for _, o := range pr.Options {
		ids = append(ids, o.ID)
	}
	if strings.Join(ids, ",") != "allow,allow_always:0,deny" {
		t.Errorf("options %v", ids)
	}
	if err := h.s.RespondPermission("nope", agentmanager.PermissionAnswer{OptionID: "allow"}); err == nil {
		t.Error("unknown request accepted")
	}
	if err := h.s.RespondPermission("req-1", agentmanager.PermissionAnswer{OptionID: "bogus"}); err == nil {
		t.Error("bogus option accepted")
	}
	if err := h.s.RespondPermission("req-1", agentmanager.PermissionAnswer{OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	up := expect[agentmanager.ToolCallUpdate](h).Update
	if up.ID != "toolu_1" || *up.Status != agentmanager.ToolCompleted || *up.Output != "tool said allow" || up.MCPServer != "cad" {
		t.Errorf("%+v", up)
	}
	expect[agentmanager.TextDelta](h)
	expect[agentmanager.TurnEnd](h)
	b, _ := json.Marshal(controlResponse(t, h))
	want := `{"response":{"request_id":"req-1","response":{"behavior":"allow","updatedInput":{"command":"ls","x1":0,"x2":10,"y1":0,"y2":10}},"subtype":"success"},"type":"control_response"}`
	if string(b) != want {
		t.Errorf("reply\n got %s\nwant %s", b, want)
	}
}

func TestPermissionDenyAndAlways(t *testing.T) {
	h := open(t, builtin("tool-turn"), toolOpts("Bash", nil))
	h.prompt("run")
	expect[agentmanager.SessionStarted](h)
	expect[agentmanager.TextDelta](h)
	if st := expect[agentmanager.ToolCallStart](h); st.Call.Kind != agentmanager.KindExecute || st.Call.Title != "Bash ls" || st.Call.MCPServer != "" {
		t.Errorf("%+v", st.Call)
	}
	pr := expect[agentmanager.PermissionRequest](h)
	if *pr.Call.Kind != agentmanager.KindExecute {
		t.Errorf("%+v", pr.Call)
	}
	if err := h.s.RespondPermission(pr.RequestID, agentmanager.PermissionAnswer{OptionID: "deny"}); err != nil {
		t.Fatal(err)
	}
	if up := expect[agentmanager.ToolCallUpdate](h).Update; *up.Status != agentmanager.ToolFailed {
		t.Errorf("%+v", up)
	}
	expect[agentmanager.TextDelta](h)
	expect[agentmanager.TurnEnd](h)
	m := controlResponse(t, h)["response"].(map[string]any)["response"].(map[string]any)
	if m["behavior"] != "deny" || m["message"] != "Denied by user" {
		t.Errorf("%v", m)
	}

	h2 := open(t, builtin("tool-turn"), toolOpts("Bash", nil))
	h2.prompt("run")
	pr = func() agentmanager.PermissionRequest {
		for {
			if p, ok := h2.next().(agentmanager.PermissionRequest); ok {
				return p
			}
		}
	}()
	if err := h2.s.RespondPermission(pr.RequestID, agentmanager.PermissionAnswer{OptionID: "allow_always:0"}); err != nil {
		t.Fatal(err)
	}
	expect[agentmanager.ToolCallUpdate](h2)
	m = controlResponse(t, h2)["response"].(map[string]any)["response"].(map[string]any)
	ups, _ := json.Marshal(m["updatedPermissions"])
	if m["behavior"] != "allow" || string(ups) != `[{"destination":"session","mode":"acceptEdits","type":"setMode"}]` {
		t.Errorf("%v %s", m, ups)
	}
}

func TestAutoApproveMCP(t *testing.T) {
	h := open(t, builtin("tool-turn"), toolOpts("mcp__cad__add_line", agentmanager.AllowMCP("cad")))
	h.prompt("draw")
	for {
		switch ev := h.next().(type) {
		case agentmanager.PermissionRequest:
			t.Fatalf("auto-approved request reached the UI: %+v", ev)
		case agentmanager.TurnEnd:
			m := controlResponse(t, h)["response"].(map[string]any)["response"].(map[string]any)
			if m["behavior"] != "allow" {
				t.Errorf("%v", m)
			}
			return
		}
	}
}

func TestAutoApproveSkipsNonMCP(t *testing.T) {
	h := open(t, builtin("tool-turn"), toolOpts("Bash", agentmanager.AllowMCP("cad")))
	h.prompt("run")
	for {
		if _, ok := h.next().(agentmanager.PermissionRequest); ok {
			return
		}
	}
}

func TestInterruptMidTurn(t *testing.T) {
	h := open(t, builtin("interrupt-turn"))
	h.prompt("long")
	expect[agentmanager.SessionStarted](h)
	if td := expect[agentmanager.TextDelta](h); td.Text != "working" {
		t.Errorf("%+v", td)
	}
	if err := h.s.Prompt(context.Background(), "second"); err == nil || err == agentmanager.ErrClosed {
		t.Errorf("prompt while running = %v", err)
	}
	if err := h.s.Cancel(); err != nil {
		t.Fatal(err)
	}
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopCancelled {
		t.Errorf("%+v", te)
	}
	if err := h.s.Cancel(); err != nil {
		t.Errorf("idle Cancel = %v", err)
	}
	h.prompt("next")
	if td := expect[agentmanager.TextDelta](h); td.Text != "echo: next" {
		t.Errorf("%+v", td)
	}
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopEndTurn {
		t.Errorf("%+v", te)
	}
	var got map[string]any
	for _, l := range h.logLines() {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] == "control_request" {
			got = m
		}
	}
	req := got["request"].(map[string]any)
	if req["subtype"] != "interrupt" || !strings.HasPrefix(got["request_id"].(string), "gd-int-") {
		t.Errorf("%v", got)
	}
}

func TestCancelDeniesPendingPermissionFirst(t *testing.T) {
	h := open(t, builtin("permission-interrupt"), toolOpts("Bash", nil))
	h.prompt("run")
	for {
		if _, ok := h.next().(agentmanager.PermissionRequest); ok {
			break
		}
	}
	if err := h.s.Cancel(); err != nil {
		t.Fatal(err)
	}
	if te := expect[agentmanager.TurnEnd](h); te.Reason != agentmanager.StopCancelled {
		t.Errorf("%+v", te)
	}
	var kinds []string
	for _, l := range h.logLines() {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] != "user" {
			kinds = append(kinds, m["type"].(string))
		}
	}
	if strings.Join(kinds, ",") != "control_response,control_request" {
		t.Errorf("order %v", kinds)
	}
	m := controlResponse(t, h)["response"].(map[string]any)["response"].(map[string]any)
	if m["behavior"] != "deny" {
		t.Errorf("%v", m)
	}
	if err := h.s.RespondPermission("req-1", agentmanager.PermissionAnswer{OptionID: "allow"}); err == nil {
		t.Error("answered a request that was already denied")
	}
	h.prompt("after")
	if td := expect[agentmanager.TextDelta](h); td.Text != "echo: after" {
		t.Errorf("%+v", td)
	}
}

func TestUnsupportedControlRequestGetsError(t *testing.T) {
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "control_request", "request_id": "u1",
			"request": map[string]any{"subtype": "hook_callback"}}},
		map[string]any{"op": "expect", "match": map[string]any{"type": "control_response",
			"response": map[string]any{"subtype": "error", "request_id": "u1", "error": "unsupported"}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "log", "log": map[string]any{"level": "warn", "message": "m"}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "result", "subtype": "success"}},
	}}
	h := open(t, sc)
	h.prompt("q")
	if l := expect[agentmanager.Log](h); l.Level != "warn" || l.Message != "m" {
		t.Errorf("%+v", l)
	}
	expect[agentmanager.TurnEnd](h)
}

func TestAsyncLaunchedStaysInProgress(t *testing.T) {
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "content": []any{
			map[string]any{"type": "tool_use", "id": "t1", "name": "TodoWrite", "input": map[string]any{"todos": []any{
				map[string]any{"content": "a", "status": "pending"}}}}}}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "user", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "status": "async_launched", "content": "bg"}}}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "result", "subtype": "success"}},
	}}
	h := open(t, sc)
	h.prompt("q")
	if st := expect[agentmanager.ToolCallStart](h); st.Call.Kind != agentmanager.KindThink {
		t.Errorf("%+v", st)
	}
	if pu := expect[agentmanager.PlanUpdate](h); len(pu.Entries) != 1 || pu.Entries[0].Content != "a" {
		t.Errorf("%+v", pu)
	}
	if up := expect[agentmanager.ToolCallUpdate](h).Update; *up.Status != agentmanager.ToolInProgress || *up.Output != "bg" {
		t.Errorf("%+v", up)
	}
}

func TestCrashMidTurn(t *testing.T) {
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "system", "subtype": "init", "session_id": "s"}},
		map[string]any{"op": "stderr", "frame": map[string]any{"text": "kaboom"}},
		map[string]any{"op": "exit", "code": 3},
	}}
	h := open(t, sc)
	h.prompt("q")
	expect[agentmanager.SessionStarted](h)
	ex := expect[agentmanager.Exited](h)
	if !strings.Contains(ex.Err, "kaboom") || !strings.Contains(ex.Err, "exit status 3") {
		t.Errorf("Exited.Err = %q", ex.Err)
	}
	if _, ok := <-h.s.Events(); ok {
		t.Error("events not closed after Exited")
	}
	if err := h.s.Prompt(context.Background(), "x"); err != agentmanager.ErrClosed {
		t.Errorf("Prompt = %v", err)
	}
	if err := h.s.Cancel(); err != agentmanager.ErrClosed {
		t.Errorf("Cancel = %v", err)
	}
	if err := h.s.RespondPermission("r", agentmanager.PermissionAnswer{}); err != agentmanager.ErrClosed {
		t.Errorf("RespondPermission = %v", err)
	}
	if err := h.s.Close(); err != nil {
		t.Error(err)
	}
}

func TestLargeLine(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	sc := map[string]any{"steps": []any{
		map[string]any{"op": "expect", "match": map[string]any{"type": "user"}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "content": []any{
			map[string]any{"type": "text", "text": big}}}}},
		map[string]any{"op": "send", "frame": map[string]any{"type": "result", "subtype": "success"}},
	}}
	h := open(t, sc)
	h.prompt("q")
	if td := expect[agentmanager.TextDelta](h); len(td.Text) != len(big) {
		t.Errorf("len %d", len(td.Text))
	}
	expect[agentmanager.TurnEnd](h)
}

func TestOpenMissingBinary(t *testing.T) {
	_, err := claude.New().Open(context.Background(), agentmanager.Config{Binary: "/nonexistent/claude"})
	if err == nil {
		t.Fatal("want error")
	}
	if m, _ := filepath.Glob(filepath.Join(os.TempDir(), "agentmanager-claude-mcp-*")); len(m) > 0 {
		for _, f := range m {
			if st, _ := os.Stat(f); st != nil && time.Since(st.ModTime()) < 5*time.Second {
				t.Errorf("leaked temp config %s", f)
			}
		}
	}
}
