package ui

import (
	"context"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/mcp"
	"agentmanager/store"
	"agentmanager/transcript"
	"go-draw/cadmcp"
	"go-draw/i18n"
	"go-draw/internal/gpui"
)

// Timer periods of the agent pump. The fast period applies while a session
// is starting, a turn runs or tool calls wait for the UI goroutine; idle
// open sessions only poll slowly for a crashed agent.
const (
	agentBusyPoll = 16 * time.Millisecond
	agentIdlePoll = 250 * time.Millisecond
	// agentEventBatch bounds the events folded per tab per timer tick.
	agentEventBatch = 256
	// agentMCPName is the MCP server name the agent sees; the permission
	// policy auto-allows exactly this server.
	agentMCPName = "cad"
)

// AgentOpener starts an agent session. The default is harnesses.Open; tests
// and embedders replace it with SetOpener.
type AgentOpener func(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error)

type agentState int

const (
	agentIdle agentState = iota
	agentStarting
	agentReady
)

type agentStart struct {
	gen  int
	sess agentmanager.Session
	err  error
}

// agentTab is one conversation: a transcript, its stored record and, while
// live, an agent session. Only the start result is touched off the UI goroutine.
type agentTab struct {
	tr transcript.T
	// expanded holds the indexes of thought blocks the user opened.
	expanded map[int]bool
	// shownVersion is the transcript version the scroll position follows.
	shownVersion uint64
	scroll       *gpui.ScrollHandle
	queued       string
	// attention is set when a background tab got a permission ask or finished a turn.
	attention bool

	state agentState
	gen   int
	sess  agentmanager.Session
	// harness is the harness the live session was started with.
	harness agentmanager.Harness
	// cancelOpen aborts a start in flight.
	cancelOpen context.CancelFunc
	rec        *store.Record

	mu       sync.Mutex
	startRes *agentStart
}

func newAgentTab() *agentTab {
	return &agentTab{expanded: map[int]bool{}, scroll: gpui.NewScrollHandle(), shownVersion: ^uint64(0)}
}

// AgentPanel is the docked chat panel that drives AI agents over the
// drawing: one tab per conversation, each with its own agent session. All of
// its methods and every callback run on the UI goroutine; the only
// cross-goroutine traffic is the cadmcp.Dispatcher mailbox, the sessions'
// event channels (read without blocking) and the start results.
type AgentPanel struct {
	ws      *Workspace
	win     *gpui.Window
	visible bool

	choices []agentmanager.Harness
	// selected is the harness a tab without a record starts with.
	selected agentmanager.Harness
	// explicit is set by Configure.
	explicit bool
	binary   string
	binFor   agentmanager.Harness
	open     AgentOpener
	probed   bool

	input *gpui.TextInput
	tabs  []*agentTab
	cur   *agentTab
	hist  agentHistory
	// deferred is a prompt from the AI command that waits for the first frame.
	deferred string

	// srv and disp are the one cad MCP server shared by every live session.
	srv   *mcp.Server
	disp  *cadmcp.Dispatcher
	ep    mcp.Endpoint
	timer *gpui.Timer
	tgen  int
	// openers counts start goroutines in flight, so quitting can wait for them and close
	// the session a late start returns.
	openers sync.WaitGroup
}

// openerJoinTimeout bounds how long quitting waits for a start in flight.
const openerJoinTimeout = 10 * time.Second

func newAgentPanel(ws *Workspace) *AgentPanel {
	p := &AgentPanel{
		ws:    ws,
		open:  harnesses.Open,
		input: themeField(gpui.NewTextInput(i18n.T("ui.agent.input_placeholder"))),
	}
	p.cur = newAgentTab()
	p.tabs = []*agentTab{p.cur}
	p.input.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { p.Send(text, w) })
	p.input.OnEscape(func(w *gpui.Window, _ *gpui.App) { p.returnFocus(w) })
	return p
}

// AgentPanel returns the workspace's agent chat panel, creating it on first
// use; nothing agent related exists before that.
func (ws *Workspace) AgentPanel() *AgentPanel {
	if ws.agent == nil {
		ws.agent = newAgentPanel(ws)
	}
	return ws.agent
}

// CloseAgent stops every agent session and the MCP server, waits for the
// agent processes to exit and saves which sessions were open. Call it when
// the application quits.
func (ws *Workspace) CloseAgent() {
	p := ws.agent
	if p == nil {
		return
	}
	p.stopAll(true)
	p.persistOpen()
	for _, t := range p.tabs {
		if t.rec != nil {
			t.rec.Close()
		}
	}
}

// Visible reports whether the panel is shown.
func (p *AgentPanel) Visible() bool { return p.visible }

// Toggle shows or hides the panel. Sessions keep running while hidden.
func (p *AgentPanel) Toggle(w *gpui.Window) {
	p.visible = !p.visible
	if p.visible {
		p.ensureLoaded()
	} else {
		p.returnFocus(w)
	}
	w.Refresh()
}

// Show opens the panel and restores the project's open conversations; no
// agent starts.
func (p *AgentPanel) Show() {
	p.visible = true
	p.ensureLoaded()
}

// SetOpener replaces how sessions are started.
func (p *AgentPanel) SetOpener(o AgentOpener) { p.open = o }

// Configure selects the harness and, when binary is not empty, the
// executable to run for it. A configured harness is always offered, even
// when it is not on PATH.
func (p *AgentPanel) Configure(h agentmanager.Harness, binary string) {
	p.probe()
	p.selected = h
	p.explicit = true
	if binary != "" {
		p.binary, p.binFor = binary, h
	}
	for _, c := range p.choices {
		if c == h {
			return
		}
	}
	if binary != "" {
		p.choices = append(p.choices, h)
	}
}

// Seed lets f add blocks to the active tab's transcript, for screenshots and tests.
func (p *AgentPanel) Seed(f func(t *transcript.T)) { f(&p.cur.tr) }

// Transcript returns the active tab's transcript, for tests.
func (p *AgentPanel) Transcript() *transcript.T { return &p.cur.tr }

// Running reports whether the active tab's session is starting or open.
func (p *AgentPanel) Running() bool { return p.cur.state != agentIdle }

// Live reports whether any tab has a session starting or open, and whether
// a background tab wants attention.
func (p *AgentPanel) Live() (live, attention bool) {
	for _, t := range p.tabs {
		live = live || t.state != agentIdle
		attention = attention || t.attention
	}
	return
}

// probe lists the harnesses installed on PATH, once.
func (p *AgentPanel) probe() {
	if p.probed {
		return
	}
	p.probed = true
	for _, h := range []agentmanager.Harness{agentmanager.OpenCode, agentmanager.Claude, agentmanager.Codex} {
		if _, ok := harnesses.Available(h); ok {
			p.choices = append(p.choices, h)
		}
	}
	if p.selected == "" && len(p.choices) > 0 {
		p.selected = p.choices[0]
	}
}

// returnFocus hands the keyboard back to the command line.
func (p *AgentPanel) returnFocus(w *gpui.Window) {
	if fh := p.ws.input.FocusHandle(); fh != nil {
		w.RequestFocus(fh)
	}
}

// harnessOf is the harness tab t talks to: its record's, else the selected one.
func (p *AgentPanel) harnessOf(t *agentTab) agentmanager.Harness {
	if t.rec != nil {
		if h := t.rec.Meta().Harness; h != "" {
			return agentmanager.Harness(h)
		}
	}
	return p.selected
}

// ensureServer starts the shared MCP server on first use.
func (p *AgentPanel) ensureServer(w *gpui.Window) error {
	if p.srv != nil {
		return nil
	}
	disp := cadmcp.NewDispatcher(nil)
	srv := mcp.NewServer(mcp.Options{Name: "go-draw", Version: "1", Instructions: cadmcp.Instructions()})
	cadmcp.Register(srv, cadmcp.Env{
		Dispatch: disp,
		Session:  p.ws.Session,
		Snapshot: Snapshot,
		Changed: func() {
			p.ws.builder.InvalidateAll()
			if p.win != nil {
				p.win.Refresh()
			}
		},
	})
	ep, err := srv.Start()
	if err != nil {
		disp.Close()
		return err
	}
	p.srv, p.disp, p.ep = srv, disp, ep
	return nil
}

// Start opens a session for the active tab on its harness, resuming the stored
// agent session when the tab has one. The process launch runs on a goroutine;
// the pump timer picks up the result.
func (p *AgentPanel) Start(w *gpui.Window) {
	p.ensureLoaded()
	p.startTab(p.cur, w)
}

func (p *AgentPanel) startTab(t *agentTab, w *gpui.Window) {
	h := p.harnessOf(t)
	if t.state != agentIdle || h == "" {
		return
	}
	p.win = w
	if err := p.ensureServer(w); err != nil {
		p.apply(t, agentmanager.Log{Level: "error", Message: i18n.T("ui.agent.err.mcp", err)})
		w.Refresh()
		return
	}
	t.gen++
	gen := t.gen
	cfg := agentmanager.Config{
		Harness:      h,
		Cwd:          p.ws.projectDir(),
		ResumeID:     p.resumeID(t, h),
		SystemPrompt: cadmcp.Instructions(),
		MCPServers:   []agentmanager.MCPServer{{Name: agentMCPName, URL: p.ep.URL, Token: p.ep.Token}},
		AutoApprove:  agentmanager.AllowMCP(agentMCPName),
	}
	if p.binary != "" && p.binFor == h {
		cfg.Binary = p.binary
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelOpen = cancel
	t.state = agentStarting
	t.harness = h
	open := p.open
	p.openers.Add(1)
	go func() {
		defer p.openers.Done()
		s, err := open(ctx, cfg)
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.gen != gen {
			if s != nil {
				s.Close()
			}
			return
		}
		t.startRes = &agentStart{gen: gen, sess: s, err: err}
	}()
	p.arm(w, agentBusyPoll)
	w.Refresh()
}

// Stop closes the active tab's session; the transcript stays. The MCP server
// closes with the last live session.
func (p *AgentPanel) Stop(w *gpui.Window) {
	p.stopTab(p.cur, false)
	w.Refresh()
}

func (p *AgentPanel) stopAll(wait bool) {
	for _, t := range p.tabs {
		p.stopTab(t, wait)
	}
	if wait {
		p.joinOpeners()
	}
}

// joinOpeners waits (bounded) for starts in flight; each closes the session it returns once its
// tab has moved on, so none outlives the application.
func (p *AgentPanel) joinOpeners() {
	done := make(chan struct{})
	go func() { p.openers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(openerJoinTimeout):
	}
}

// stopTab tears t's session down. With wait it blocks until the agent process
// is gone; otherwise the close runs in the background so the UI stays live.
func (p *AgentPanel) stopTab(t *agentTab, wait bool) {
	if t.state == agentIdle && t.sess == nil {
		return
	}
	t.mu.Lock()
	t.gen++
	if r := t.startRes; r != nil && r.sess != nil {
		go r.sess.Close()
	}
	t.startRes = nil
	t.mu.Unlock()
	if t.cancelOpen != nil {
		t.cancelOpen()
		t.cancelOpen = nil
	}
	sess := t.sess
	t.sess = nil
	t.state = agentIdle
	t.queued = ""
	var disp *cadmcp.Dispatcher
	var srv *mcp.Server
	if !p.anyLive() {
		p.timer.Cancel()
		p.timer = nil
		disp, srv = p.disp, p.srv
		p.disp, p.srv = nil, nil
		if disp != nil {
			disp.Close()
		}
	}
	closeAll := func() {
		if sess != nil {
			sess.Close()
		}
		if srv != nil {
			srv.Close()
		}
	}
	if wait {
		closeAll()
	} else {
		go closeAll()
	}
}

func (p *AgentPanel) anyLive() bool {
	for _, t := range p.tabs {
		if t.state != agentIdle {
			return true
		}
	}
	return false
}

// Send submits a prompt on the active tab. Without a session it starts one
// first (resuming the tab's stored agent session) and sends the prompt once
// the agent is connected.
func (p *AgentPanel) Send(text string, w *gpui.Window) {
	p.ensureLoaded()
	text = strings.TrimSpace(text)
	t := p.cur
	if text == "" || t.tr.Running() {
		return
	}
	p.win = w
	switch t.state {
	case agentIdle:
		if p.harnessOf(t) == "" {
			t.tr.Apply(agentmanager.Log{Level: "warn", Message: i18n.T("ui.agent.warn.no_agent")})
			w.Refresh()
			return
		}
		t.queued = text
		p.input.SetText("")
		p.userMessage(t, text)
		p.startTab(t, w)
	case agentStarting:
		t.queued = text
		p.input.SetText("")
		p.userMessage(t, text)
	case agentReady:
		p.input.SetText("")
		p.userMessage(t, text)
		p.prompt(t, text)
	}
	w.Refresh()
}

func (p *AgentPanel) prompt(t *agentTab, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := t.sess.Prompt(ctx, text); err != nil {
		p.apply(t, agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: err.Error()})
	}
}

// Cancel aborts the active tab's running turn.
func (p *AgentPanel) Cancel(w *gpui.Window) {
	t := p.cur
	if t.state == agentReady {
		if err := t.sess.Cancel(); err != nil {
			p.apply(t, agentmanager.Log{Level: "error", Message: err.Error()})
		}
	} else if t.state == agentStarting {
		p.stopTab(t, false)
		p.apply(t, agentmanager.TurnEnd{Reason: agentmanager.StopCancelled})
	}
	w.Refresh()
}

// Respond answers a permission ask of the active tab with an option, or cancels it.
func (p *AgentPanel) Respond(requestID, optionID string, w *gpui.Window) {
	t := p.cur
	if t.state != agentReady {
		return
	}
	ans := agentmanager.PermissionAnswer{OptionID: optionID, Cancelled: optionID == ""}
	if err := t.sess.RespondPermission(requestID, ans); err != nil {
		p.apply(t, agentmanager.Log{Level: "error", Message: err.Error()})
	}
	t.tr.ResolvePermission(requestID)
	if t.rec != nil {
		p.check(t.rec.ResolvePermission(requestID))
	}
	w.Refresh()
}

// arm (re)schedules the pump timer.
func (p *AgentPanel) arm(w *gpui.Window, d time.Duration) {
	p.timer.Cancel()
	p.tgen++
	gen := p.tgen
	p.timer = w.After(d, func(w *gpui.Window, _ *gpui.App) {
		if p.tgen != gen {
			return
		}
		p.pump(w)
	})
}

// pump is the timer body, on the UI goroutine: it runs queued tool calls,
// adopts finished starts, folds pending agent events of every tab into its
// transcript and record, and re-arms itself while a session exists.
func (p *AgentPanel) pump(w *gpui.Window) {
	p.win = w
	if p.disp != nil {
		p.disp.Drain()
	}
	live, busy := false, false
	for _, t := range append([]*agentTab(nil), p.tabs...) {
		t.mu.Lock()
		res := t.startRes
		t.startRes = nil
		t.mu.Unlock()
		if res != nil && res.gen == t.gen {
			p.started(t, res)
		}
		p.drainEvents(t)
		if t.state != agentIdle {
			live = true
			busy = busy || t.state == agentStarting || t.tr.Running()
		}
	}
	if !live {
		return
	}
	next := agentIdlePoll
	if busy || (p.disp != nil && p.disp.Pending() > 0) {
		next = agentBusyPoll
	}
	p.arm(w, next)
}

func (p *AgentPanel) started(t *agentTab, res *agentStart) {
	t.cancelOpen = nil
	if res.err != nil {
		p.apply(t, agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: i18n.T("ui.agent.err.start", string(t.harness), res.err)})
		t.queued = ""
		p.stopTab(t, false)
		return
	}
	t.sess = res.sess
	t.state = agentReady
	if q := t.queued; q != "" {
		t.queued = ""
		p.prompt(t, q)
	}
}

func (p *AgentPanel) drainEvents(t *agentTab) {
	if t.sess == nil {
		return
	}
	ch := t.sess.Events()
	for i := 0; i < agentEventBatch; i++ {
		select {
		case ev, ok := <-ch:
			if !ok {
				p.stopTab(t, false)
				return
			}
			p.apply(t, ev)
			if _, gone := ev.(agentmanager.Exited); gone {
				p.stopTab(t, false)
				return
			}
		default:
			return
		}
	}
}

// statusLine describes the active tab's session for the panel header.
func (p *AgentPanel) statusLine() string {
	t := p.cur
	switch t.state {
	case agentStarting:
		return i18n.T("ui.agent.status.starting", t.harness)
	case agentReady:
		if t.tr.Running() {
			return i18n.T("ui.agent.status.working")
		}
		return i18n.T("ui.agent.status.ready")
	}
	return i18n.T("ui.agent.status.stopped")
}

// sendFromCommand sends a prompt typed after the AI command to the active tab.
func (p *AgentPanel) sendFromCommand(prompt string, w *gpui.Window) {
	if p.cur.tr.Running() {
		p.cur.tr.Apply(agentmanager.Log{Level: "warn", Message: i18n.T("ui.agent.warn.busy")})
		w.Refresh()
		return
	}
	p.Send(prompt, w)
}
