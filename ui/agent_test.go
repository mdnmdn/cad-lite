package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"agentmanager"
	"go-draw/doc"
	"go-draw/internal/gpui"
	"go-draw/ui"
)

type fakeSession struct {
	mu        sync.Mutex
	events    chan agentmanager.Event
	prompts   []string
	answers   []agentmanager.PermissionAnswer
	answerIDs []string
	cancels   int
	closed    bool
	promptErr error
}

func newFakeSession() *fakeSession { return &fakeSession{events: make(chan agentmanager.Event, 64)} }

func (f *fakeSession) ID() string                        { return "fake" }
func (f *fakeSession) Events() <-chan agentmanager.Event { return f.events }
func (f *fakeSession) Prompt(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, text)
	return f.promptErr
}
func (f *fakeSession) Cancel() error { f.mu.Lock(); f.cancels++; f.mu.Unlock(); return nil }
func (f *fakeSession) RespondPermission(id string, a agentmanager.PermissionAnswer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answerIDs = append(f.answerIDs, id)
	f.answers = append(f.answers, a)
	return nil
}
func (f *fakeSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.events)
	}
	return nil
}
func (f *fakeSession) isClosed() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.closed }

type agentRig struct {
	t   *testing.T
	ws  *ui.Workspace
	hw  *gpui.HeadlessWindow
	p   *gpui.HeadlessPlatform
	fs  *fakeSession
	cfg chan agentmanager.Config
}

// Positions of the panel controls in the 1280x800 window.
var (
	posStart  = gpui.Point{X: 1245, Y: 88}
	posInput  = gpui.Point{X: 1040, Y: 639}
	posSend   = gpui.Point{X: 1246, Y: 639}
	posCancel = gpui.Point{X: 1241, Y: 639}
)

func openAgent(t *testing.T) *agentRig {
	t.Helper()
	ws := ui.NewWorkspace(doc.New(), nil)
	r := &agentRig{t: t, ws: ws, fs: newFakeSession(), cfg: make(chan agentmanager.Config, 1)}
	ws.AgentPanel().Configure(agentmanager.Claude, "/nonexistent/claude")
	ws.AgentPanel().SetOpener(func(_ context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
		r.cfg <- cfg
		return r.fs, nil
	})
	ws.AgentPanel().Show()
	r.p = gpui.NewHeadlessPlatform()
	if err := r.p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(ui.Bindings()...)
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		r.hw = r.p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	r.hw.Pixels()
	t.Cleanup(ws.CloseAgent)
	return r
}

func (r *agentRig) click(p gpui.Point) {
	r.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	r.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1}
	r.hw.Dispatch(up)
	r.hw.Dispatch(gpui.ClickEvent{Up: &up})
}

func (r *agentRig) tick() {
	r.t.Helper()
	if err := r.p.Advance(16 * time.Millisecond); err != nil {
		r.t.Fatal(err)
	}
}

func (r *agentRig) until(what string, cond func() bool) {
	r.t.Helper()
	for i := 0; i < 500; i++ {
		r.tick()
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatalf("timed out waiting for %s", what)
}

func (r *agentRig) typ(s string) {
	for _, ch := range s {
		c := string(ch)
		r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: c, KeyChar: &c}, PreferCharacterInput: true})
	}
}

func (r *agentRig) start() {
	r.t.Helper()
	r.click(posStart)
	r.until("session", func() bool { return r.ws.AgentPanel().Running() && len(r.cfg) == 1 })
	r.tick()
}

func TestAgentPanelRendersSeededTranscript(t *testing.T) {
	r := openAgent(t)
	before := append([]byte(nil), r.hw.Pixels()...)
	ui.SeedAgentDemo(r.ws)
	r.ws.AgentPanel().Show()
	r.hw.Window().Refresh()
	after := r.hw.Pixels()
	if bytes.Equal(before, after) {
		t.Fatal("seeding a transcript did not change the frame")
	}
	if n := len(r.ws.AgentPanel().Transcript().Blocks()); n < 6 {
		t.Fatalf("blocks = %d", n)
	}
}

func TestAgentStartWiresCadMCPAndPolicy(t *testing.T) {
	r := openAgent(t)
	r.start()
	cfg := <-r.cfg
	if cfg.Harness != agentmanager.Claude || cfg.Binary != "/nonexistent/claude" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if len(cfg.MCPServers) != 1 || cfg.MCPServers[0].Name != "cad" || cfg.MCPServers[0].URL == "" || cfg.MCPServers[0].Token == "" {
		t.Fatalf("mcp servers = %+v", cfg.MCPServers)
	}
	if cfg.AutoApprove == nil {
		t.Fatal("no permission policy")
	}
	cadCall := agentmanager.PermissionRequest{Call: agentmanager.ToolCallPatch{MCPServer: "cad"},
		Options: []agentmanager.PermissionOption{{ID: "y", Kind: agentmanager.AllowOnce}}}
	if id, ok := cfg.AutoApprove(cadCall); !ok || id != "y" {
		t.Fatalf("cad call not auto-allowed: %q %v", id, ok)
	}
	other := cadCall
	other.Call.MCPServer = "other"
	if _, ok := cfg.AutoApprove(other); ok {
		t.Fatal("foreign tool auto-allowed")
	}
}

func TestAgentPromptSubmitCallsPrompt(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.click(posInput)
	r.typ("draw a circle")
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}})
	r.fs.mu.Lock()
	got := append([]string(nil), r.fs.prompts...)
	r.fs.mu.Unlock()
	if len(got) != 1 || got[0] != "draw a circle" {
		t.Fatalf("prompts = %q", got)
	}
	if !r.ws.AgentPanel().Transcript().Running() {
		t.Fatal("turn not marked running")
	}
	if r.ws.Session().Active() != "" {
		t.Fatal("text leaked into the command line")
	}
	r.fs.events <- agentmanager.TextDelta{Text: "done"}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("turn end", func() bool { return !r.ws.AgentPanel().Transcript().Running() })
}

func TestAgentSendWithoutSessionStartsAndQueues(t *testing.T) {
	r := openAgent(t)
	r.click(posInput)
	r.typ("hello")
	r.click(posSend)
	r.until("queued prompt", func() bool {
		r.fs.mu.Lock()
		defer r.fs.mu.Unlock()
		return len(r.fs.prompts) == 1
	})
}

func TestAgentPermissionButtonAnswers(t *testing.T) {
	r := openAgent(t)
	r.start()
	title := "Run shell command: ls"
	r.fs.events <- agentmanager.PermissionRequest{RequestID: "p1", Call: agentmanager.ToolCallPatch{ID: "t", Title: &title},
		Options: []agentmanager.PermissionOption{
			{ID: "a", Name: "Allow once", Kind: agentmanager.AllowOnce},
			{ID: "r", Name: "Reject", Kind: agentmanager.RejectOnce}}}
	r.until("permission block", func() bool { return len(r.ws.AgentPanel().Transcript().Blocks()) == 1 })
	r.hw.Pixels()
	r.click(gpui.Point{X: 998, Y: 173})
	r.fs.mu.Lock()
	ids, ans := r.fs.answerIDs, r.fs.answers
	r.fs.mu.Unlock()
	if len(ids) != 1 || ids[0] != "p1" || ans[0].OptionID != "a" || ans[0].Cancelled {
		t.Fatalf("answers = %v %+v", ids, ans)
	}
}

func TestAgentCancelButtonWhileRunning(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.ws.AgentPanel().Send("go", r.hw.Window())
	r.tick()
	r.click(posCancel)
	r.fs.mu.Lock()
	n := r.fs.cancels
	r.fs.mu.Unlock()
	if n != 1 {
		t.Fatalf("cancels = %d", n)
	}
}

func TestAgentPromptErrorEndsTurn(t *testing.T) {
	r := openAgent(t)
	r.fs.promptErr = errors.New("boom")
	r.start()
	r.ws.AgentPanel().Send("go", r.hw.Window())
	if r.ws.AgentPanel().Transcript().Running() {
		t.Fatal("turn still running after prompt error")
	}
}

func TestAgentEscapeReturnsFocusToCommandLine(t *testing.T) {
	r := openAgent(t)
	r.click(posInput)
	r.typ("x")
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "escape"}})
	r.typ("line")
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}})
	if got := r.ws.Session().Active(); got != "LINE" {
		t.Fatalf("active command = %q, want LINE (typing after Escape must reach the command line)", got)
	}
}

func TestAgentToolCallFromAnotherGoroutineIsDrained(t *testing.T) {
	r := openAgent(t)
	r.start()
	cfg := <-r.cfg
	srv := cfg.MCPServers[0]
	type result struct {
		code int
		body []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add_line","arguments":{"x1":0,"y1":0,"x2":50,"y2":0}}}`
		req, _ := http.NewRequest("POST", srv.URL, bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+srv.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		done <- result{code: resp.StatusCode, body: buf.Bytes()}
	}()
	var res result
	r.until("tool call", func() bool {
		select {
		case res = <-done:
			return true
		default:
			return false
		}
	})
	if res.err != nil || res.code != 200 {
		t.Fatalf("http: %v %d %s", res.err, res.code, res.body)
	}
	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.body, &rpc); err != nil || rpc.Result.IsError {
		t.Fatalf("result: %v %s", err, res.body)
	}
	if n := r.ws.Session().Drawing().Len(); n != 1 {
		t.Fatalf("entities = %d, want 1", n)
	}
}

func TestAgentStopClosesSessionAndServer(t *testing.T) {
	r := openAgent(t)
	r.start()
	cfg := <-r.cfg
	r.ws.CloseAgent()
	if !r.fs.isClosed() {
		t.Fatal("session not closed")
	}
	if r.ws.AgentPanel().Running() {
		t.Fatal("panel still running")
	}
	req, _ := http.NewRequest("POST", cfg.MCPServers[0].URL, bytes.NewReader([]byte("{}")))
	c := http.Client{Timeout: time.Second}
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("MCP server still listening")
	}
}

func TestAgentExitedEventStopsSession(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.fs.events <- agentmanager.Exited{Err: "crash"}
	r.until("stop", func() bool { return !r.ws.AgentPanel().Running() })
}

func TestAgentF2TogglesPanel(t *testing.T) {
	r := openAgent(t)
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "f2"}})
	if r.ws.AgentPanel().Visible() {
		t.Fatal("F2 did not hide the panel")
	}
}
