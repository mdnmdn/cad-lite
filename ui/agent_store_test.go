package ui_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/ui"
)

type memFiles struct{}

func (memFiles) Open(string) (*doc.Drawing, []string, error) { return doc.New(), nil, nil }
func (memFiles) Save(string, *doc.Drawing) ([]string, error) { return nil, nil }

func (r *agentRig) sess(i int) *fakeSession {
	if i == 0 {
		return r.fs
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if i-1 < len(r.more) {
		return r.more[i-1]
	}
	return nil
}

func (r *agentRig) nextCfg() agentmanager.Config {
	r.t.Helper()
	select {
	case c := <-r.cfg:
		return c
	case <-time.After(3 * time.Second):
		r.t.Fatal("no session was started")
	}
	return agentmanager.Config{}
}

func (fs *fakeSession) promptList() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]string(nil), fs.prompts...)
}

func (r *agentRig) enter() {
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}})
}

// say sends text on the active tab and waits until its session got the prompt.
func (r *agentRig) say(text string, session int) {
	r.t.Helper()
	r.ws.AgentPanel().Send(text, r.hw.Window())
	r.until("prompt "+text, func() bool {
		s := r.sess(session)
		return s != nil && len(s.promptList()) > 0 && s.promptList()[len(s.promptList())-1] == text
	})
}

// finish plays a complete turn on session s and waits for the active transcript to settle.
func (r *agentRig) turn(s *fakeSession, id, text string) {
	s.events <- agentmanager.SessionStarted{SessionID: id}
	s.events <- agentmanager.TextDelta{Text: text}
	s.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
}

func (r *agentRig) hasText(i int, text string) bool { return r.tabHasText(i, text) }

func TestAgentIsLazyUntilOpened(t *testing.T) {
	dir := t.TempDir()
	ws := ui.NewWorkspace(doc.New(), nil)
	ws.SetProjectPath(filepath.Join(dir, "a.dxf"))
	p := gpui.NewHeadlessPlatform()
	if err := p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(ui.Bindings()...)
		_ = cx.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws })
	}); err != nil {
		t.Fatal(err)
	}
	p.Windows()[0].Pixels()
	for range 20 {
		p.Advance(300 * time.Millisecond)
	}
	if ws.HasAgentPanel() {
		t.Fatal("panel exists before it was opened")
	}
	if _, err := os.Stat(filepath.Join(dir, ".cad-lite")); err == nil {
		t.Fatal("store directory created at startup")
	}
	p.Windows()[0].Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "f2"}})
	if !ws.HasAgentPanel() || !ws.AgentPanel().Visible() {
		t.Fatal("F2 did not open the panel")
	}
	t.Cleanup(ws.CloseAgent)
}

func TestOpeningPanelStartsNothingAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	for range 10 {
		r.tick()
	}
	if len(r.cfg) != 0 || r.ws.AgentPanel().Running() {
		t.Fatal("opening the panel started a session")
	}
	if _, err := os.Stat(filepath.Join(dir, ".cad-lite")); err == nil {
		t.Fatal("history directory created before any prompt")
	}
}

func TestAICommandOpensPanelAndSends(t *testing.T) {
	r := newRig(t, t.TempDir(), nil, false)
	r.typ("AI")
	r.enter()
	if got := r.ws.Session().Prompt(); !strings.Contains(got, "Ask the agent") {
		t.Fatalf("prompt = %q", got)
	}
	r.typ("draw a box 10 by 20")
	r.enter()
	if !r.ws.AgentPanel().Visible() {
		t.Fatal("panel not opened")
	}
	r.until("prompt", func() bool { return len(r.fs.promptList()) == 1 })
	if got := r.fs.promptList()[0]; got != "draw a box 10 by 20" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestAICommandAliasAndEmptyToggles(t *testing.T) {
	r := newRig(t, t.TempDir(), nil, false)
	r.typ("AGENT")
	r.enter()
	if !strings.Contains(r.ws.Session().Prompt(), "Ask the agent") {
		t.Fatalf("prompt = %q", r.ws.Session().Prompt())
	}
	r.enter()
	if !r.ws.AgentPanel().Visible() {
		t.Fatal("empty answer did not open the panel")
	}
	r.typ("AI")
	r.enter()
	r.enter()
	if r.ws.AgentPanel().Visible() {
		t.Fatal("second empty answer did not hide the panel")
	}
	if len(r.cfg) != 0 {
		t.Fatal("a session started")
	}
}

func TestTranscriptClearsAfterRestart(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	r.chat(0, "hello", "claude-sid-1", "hi there")
	r.ws.CloseAgent()
	r2 := newRig(t, dir, nil, true)
	if len(r2.ws.AgentPanel().Tabs()) != 1 || r2.hasText(0, "hi there") || len(r2.ws.AgentPanel().Sessions()) != 0 {
		t.Fatal("restart kept history")
	}
	r2.say("again", 0)
	if cfg := r2.nextCfg(); cfg.ResumeID != "" {
		t.Fatal("restart resumed an old conversation")
	}
}

func TestFirstSessionHasNoResumeID(t *testing.T) {
	r := openAgent(t)
	r.say("hello", 0)
	if cfg := r.nextCfg(); cfg.ResumeID != "" {
		t.Fatalf("ResumeID = %q", cfg.ResumeID)
	}
}

func TestTwoConcurrentSessionsShareOneServer(t *testing.T) {
	r := openAgent(t)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	r.say("first", 0)
	c1 := r.nextCfg()
	p.NewTab(w)
	r.say("second", 1)
	c2 := r.nextCfg()
	if c1.MCPServers[0].URL != c2.MCPServers[0].URL || c1.MCPServers[0].Token != c2.MCPServers[0].Token {
		t.Fatal("sessions use different MCP servers")
	}
	s1, s2 := r.sess(0), r.sess(1)
	if len(p.Tabs()) != 2 || !p.Tabs()[1].Active {
		t.Fatalf("tabs = %+v", p.Tabs())
	}
	// Events of both sessions, interleaved; tab 0 is in the background.
	s1.events <- agentmanager.SessionStarted{SessionID: "A"}
	s2.events <- agentmanager.SessionStarted{SessionID: "B"}
	s1.events <- agentmanager.TextDelta{Text: "one"}
	s2.events <- agentmanager.TextDelta{Text: "two"}
	title := "run ls"
	s1.events <- agentmanager.PermissionRequest{RequestID: "p", Call: agentmanager.ToolCallPatch{ID: "t", Title: &title}}
	s2.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("both", func() bool { return r.hasText(0, "one") && r.hasText(1, "two") })
	tabs := p.Tabs()
	if !tabs[0].Attention || tabs[1].Attention {
		t.Fatalf("attention flags = %+v", tabs)
	}
	if !tabs[0].Live || !tabs[1].Live {
		t.Fatal("tabs not live")
	}
	if r.hasText(1, "one") || r.hasText(0, "two") {
		t.Fatal("events crossed between tabs")
	}
	p.SwitchTab(0, w)
	if p.Tabs()[0].Attention {
		t.Fatal("attention not cleared on switch")
	}
	// Closing one tab leaves the other session and the server running.
	p.CloseTab(1, w)
	r.until("s2 closed", s2.isClosed)
	if s1.isClosed() {
		t.Fatal("closing tab 2 closed session 1")
	}
	cl := http.Client{Timeout: time.Second}
	resp, err := cl.Post(c1.MCPServers[0].URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("server stopped with a session still live: %v", err)
	}
	resp.Body.Close()
	p.CloseTab(0, w)
	r.until("s1 closed", s1.isClosed)
	if len(p.Tabs()) != 1 || p.Tabs()[0].Live {
		t.Fatalf("tabs after closing all = %+v", p.Tabs())
	}
	r.until("server down", func() bool {
		resp, err := cl.Post(c1.MCPServers[0].URL, "application/json", strings.NewReader("{}"))
		if err == nil {
			resp.Body.Close()
		}
		return err != nil
	})
}

func TestOpenTabsClearAfterRestart(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	r.chat(0, "first", "A", "reply one")
	p.NewTab(w)
	r.say("second", 1)
	r.nextCfg()
	r.turn(r.sess(1), "B", "reply two")
	r.until("second reply", func() bool { return r.hasText(1, "reply two") && !p.Transcript().Running() })
	r.ws.CloseAgent()
	r2 := newRig(t, dir, nil, true)
	tabs := r2.ws.AgentPanel().Tabs()
	if len(tabs) != 1 || tabs[0].Live || tabs[0].SessionID != "" || len(r2.ws.AgentPanel().Sessions()) != 0 {
		t.Fatalf("restart tabs: %+v", tabs)
	}
	if len(r2.cfg) != 0 {
		t.Fatal("restart spawned a process")
	}
}

func TestOpeningDrawingInAnotherProjectSwitchesStore(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	r := newRig(t, a, memFiles{}, true)
	p := r.ws.AgentPanel()
	r.say("in project a", 0)
	r.nextCfg()
	r.turn(r.fs, "SA", "answer a")
	r.until("answer", func() bool { return r.hasText(0, "answer a") && !p.Transcript().Running() })

	r.open(filepath.Join(b, "b.dxf"))
	tabs := p.Tabs()
	if len(tabs) != 1 || tabs[0].Title != "New" || tabs[0].Live || len(p.Transcript().Blocks()) != 0 {
		t.Fatalf("after opening a drawing of project b: %+v", tabs)
	}
	r.say("in project b", 1)
	if cfg := r.nextCfg(); cfg.Cwd != b {
		t.Fatalf("project b's agent runs in %q", cfg.Cwd)
	}
	if _, err := os.Stat(filepath.Join(b, ".cad-lite", "agent")); err != nil {
		t.Fatal(err)
	}

	// Back to plate.dxf's tab: project a's conversation, still live.
	r.open(filepath.Join(a, "plate.dxf"))
	if !r.hasText(0, "answer a") || !p.Tabs()[0].Live {
		t.Fatalf("project a not shown: %+v", p.Tabs())
	}
	r.ws.CloseDocument(0, r.hw.Window())
	r.until("closed", r.fs.isClosed)
	r.open(filepath.Join(a, "plate.dxf"))
	if r.hasText(0, "answer a") || p.Tabs()[0].Live || p.Tabs()[0].SessionID != "" {
		t.Fatalf("reopened drawing did not start fresh: %+v", p.Tabs())
	}
	r.say("back in a", 2)
	if cfg := r.nextCfg(); cfg.ResumeID != "" {
		t.Fatalf("ResumeID = %q", cfg.ResumeID)
	}
}

func TestHistoryOpensEarlierSessionInATab(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	r.say("old question", 0)
	r.nextCfg()
	r.turn(r.fs, "OLD", "old answer")
	r.until("answer", func() bool { return r.hasText(0, "old answer") && !r.ws.AgentPanel().Transcript().Running() })
	p, w := r.ws.AgentPanel(), r.hw.Window()
	id := p.Tabs()[0].SessionID
	p.CloseTab(0, w)
	ms := p.Sessions()
	if len(ms) != 1 || ms[0].ID != id || ms[0].Title != "old question" || ms[0].AgentSessionID != "OLD" {
		t.Fatalf("sessions = %+v", ms)
	}
	p.OpenSession(id, w)
	if tabs := p.Tabs(); len(tabs) != 1 || tabs[0].SessionID != id || !r.hasText(0, "old answer") {
		t.Fatalf("tabs = %+v", tabs)
	}
	r.say("follow up", 1)
	if cfg := r.nextCfg(); cfg.ResumeID != "OLD" {
		t.Fatalf("ResumeID = %q", cfg.ResumeID)
	}
}

func (r *agentRig) tabHasText(i int, text string) bool {
	for _, b := range r.ws.AgentPanel().TabTranscript(i).Blocks() {
		switch m := b.(type) {
		case transcript.AgentMsg:
			if strings.Contains(m.Text, text) {
				return true
			}
		case transcript.UserMsg:
			if strings.Contains(m.Text, text) {
				return true
			}
		}
	}
	return false
}
