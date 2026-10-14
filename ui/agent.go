package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/mcp"
	"agentmanager/transcript"
	"go-draw/cadmcp"
	"go-draw/internal/gpui"
)

// Timer periods of the agent pump. The fast period applies while a session
// is starting, a turn runs or tool calls wait for the UI goroutine; an idle
// open session only polls slowly for a crashed agent.
const (
	agentBusyPoll = 16 * time.Millisecond
	agentIdlePoll = 250 * time.Millisecond
	// agentEventBatch bounds the events folded per timer tick.
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

// AgentPanel is the docked chat panel that drives an AI agent over the
// drawing. All of its methods and every callback run on the UI goroutine;
// the only cross-goroutine traffic is the cadmcp.Dispatcher mailbox, the
// session's event channel (read without blocking) and the start result.
type AgentPanel struct {
	ws      *Workspace
	win     *gpui.Window
	visible bool

	choices  []agentmanager.Harness
	selected agentmanager.Harness
	binary   string
	cwd      string
	open     AgentOpener
	probed   bool

	input  *gpui.TextInput
	scroll *gpui.ScrollHandle
	tr     transcript.T
	// expanded holds the indexes of thought blocks the user opened.
	expanded map[int]bool
	// shownVersion is the transcript version the scroll position follows.
	shownVersion uint64
	queued       string

	state agentState
	gen   int
	sess  agentmanager.Session
	srv   *mcp.Server
	disp  *cadmcp.Dispatcher
	timer *gpui.Timer
	// cancelOpen aborts a start in flight.
	cancelOpen context.CancelFunc

	mu       sync.Mutex
	startRes *agentStart
}

func newAgentPanel(ws *Workspace) *AgentPanel {
	p := &AgentPanel{
		ws:       ws,
		open:     harnesses.Open,
		input:    gpui.NewTextInput("Ask the agent"),
		scroll:   gpui.NewScrollHandle(),
		expanded: map[int]bool{},
	}
	p.input.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { p.Send(text, w) })
	p.input.OnEscape(func(w *gpui.Window, _ *gpui.App) { p.returnFocus(w) })
	return p
}

// AgentPanel returns the workspace's agent chat panel.
func (ws *Workspace) AgentPanel() *AgentPanel { return ws.agent }

// CloseAgent stops the agent session and its MCP server and waits for the
// agent process to exit. Call it when the application quits.
func (ws *Workspace) CloseAgent() { ws.agent.stop(true) }

// Visible reports whether the panel is shown.
func (p *AgentPanel) Visible() bool { return p.visible }

// Toggle shows or hides the panel. The session keeps running while hidden.
func (p *AgentPanel) Toggle(w *gpui.Window) {
	p.visible = !p.visible
	if !p.visible {
		p.returnFocus(w)
	}
	w.Refresh()
}

// Show opens the panel.
func (p *AgentPanel) Show() { p.visible = true }

// SetOpener replaces how sessions are started.
func (p *AgentPanel) SetOpener(o AgentOpener) { p.open = o }

// Configure selects the harness and, when binary is not empty, the
// executable to run for it. A configured harness is always offered, even
// when it is not on PATH.
func (p *AgentPanel) Configure(h agentmanager.Harness, binary string) {
	p.probe()
	p.selected = h
	p.binary = binary
	for _, c := range p.choices {
		if c == h {
			return
		}
	}
	if binary != "" {
		p.choices = append(p.choices, h)
	}
}

// Seed lets f add blocks to the transcript, for screenshots and tests.
func (p *AgentPanel) Seed(f func(t *transcript.T)) { f(&p.tr) }

// Transcript returns the transcript, for tests.
func (p *AgentPanel) Transcript() *transcript.T { return &p.tr }

// Running reports whether a session is starting or open.
func (p *AgentPanel) Running() bool { return p.state != agentIdle }

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

// Start opens a session on the selected harness. The process launch runs
// on a goroutine; the pump timer picks up the result.
func (p *AgentPanel) Start(w *gpui.Window) {
	if p.state != agentIdle || p.selected == "" {
		return
	}
	p.win = w
	p.gen++
	gen := p.gen
	p.disp = cadmcp.NewDispatcher(nil)
	srv := mcp.NewServer(mcp.Options{Name: "go-draw", Version: "1", Instructions: cadmcp.Instructions()})
	cadmcp.Register(srv, cadmcp.Env{
		Dispatch: p.disp,
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
		p.disp.Close()
		p.tr.Apply(agentmanager.Log{Level: "error", Message: "cannot start the MCP server: " + err.Error()})
		w.Refresh()
		return
	}
	p.srv = srv
	cwd := p.cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cfg := agentmanager.Config{
		Harness:      p.selected,
		Binary:       p.binary,
		Cwd:          cwd,
		SystemPrompt: cadmcp.Instructions(),
		MCPServers:   []agentmanager.MCPServer{{Name: agentMCPName, URL: ep.URL, Token: ep.Token}},
		AutoApprove:  agentmanager.AllowMCP(agentMCPName),
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelOpen = cancel
	p.state = agentStarting
	open := p.open
	go func() {
		s, err := open(ctx, cfg)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.gen != gen {
			if s != nil {
				go s.Close()
			}
			return
		}
		p.startRes = &agentStart{gen: gen, sess: s, err: err}
	}()
	p.arm(w, agentBusyPoll)
	w.Refresh()
}

// Stop closes the session and the MCP server; the transcript stays.
func (p *AgentPanel) Stop(w *gpui.Window) {
	p.stop(false)
	w.Refresh()
}

// stop tears the session down. With wait it blocks until the agent process
// is gone; otherwise the close runs in the background so the UI stays live.
func (p *AgentPanel) stop(wait bool) {
	if p.state == agentIdle && p.srv == nil && p.sess == nil {
		return
	}
	p.mu.Lock()
	p.gen++
	if r := p.startRes; r != nil && r.sess != nil {
		go r.sess.Close()
	}
	p.startRes = nil
	p.mu.Unlock()
	if p.cancelOpen != nil {
		p.cancelOpen()
		p.cancelOpen = nil
	}
	p.timer.Cancel()
	p.timer = nil
	if p.disp != nil {
		p.disp.Close()
	}
	sess, srv := p.sess, p.srv
	p.sess, p.srv, p.disp = nil, nil, nil
	p.state = agentIdle
	p.queued = ""
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

// Send submits a prompt. Without a session it starts one first and sends
// the prompt once the agent is connected.
func (p *AgentPanel) Send(text string, w *gpui.Window) {
	text = strings.TrimSpace(text)
	if text == "" || p.tr.Running() {
		return
	}
	p.win = w
	switch p.state {
	case agentIdle:
		if p.selected == "" {
			p.tr.Apply(agentmanager.Log{Level: "warn", Message: "No agent is available: install opencode, claude or codex, or pass -agent-cmd."})
			w.Refresh()
			return
		}
		p.queued = text
		p.input.SetText("")
		p.tr.AddUser(text)
		p.Start(w)
	case agentStarting:
		p.queued = text
		p.input.SetText("")
		p.tr.AddUser(text)
	case agentReady:
		p.input.SetText("")
		p.tr.AddUser(text)
		p.prompt(text)
	}
	w.Refresh()
}

func (p *AgentPanel) prompt(text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.sess.Prompt(ctx, text); err != nil {
		p.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: err.Error()})
	}
}

// Cancel aborts the running turn.
func (p *AgentPanel) Cancel(w *gpui.Window) {
	if p.state == agentReady {
		if err := p.sess.Cancel(); err != nil {
			p.tr.Apply(agentmanager.Log{Level: "error", Message: err.Error()})
		}
	} else if p.state == agentStarting {
		p.stop(false)
		p.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopCancelled})
	}
	w.Refresh()
}

// Respond answers a permission ask with an option, or cancels it.
func (p *AgentPanel) Respond(requestID, optionID string, w *gpui.Window) {
	if p.state != agentReady {
		return
	}
	ans := agentmanager.PermissionAnswer{OptionID: optionID, Cancelled: optionID == ""}
	if err := p.sess.RespondPermission(requestID, ans); err != nil {
		p.tr.Apply(agentmanager.Log{Level: "error", Message: err.Error()})
	}
	p.tr.ResolvePermission(requestID)
	w.Refresh()
}

// arm (re)schedules the pump timer.
func (p *AgentPanel) arm(w *gpui.Window, d time.Duration) {
	p.timer.Cancel()
	gen := p.gen
	p.timer = w.After(d, func(w *gpui.Window, _ *gpui.App) {
		if p.gen != gen {
			return
		}
		p.pump(w)
	})
}

// pump is the timer body, on the UI goroutine: it runs queued tool calls,
// adopts a finished start, folds pending agent events into the transcript
// and re-arms itself while a session exists.
func (p *AgentPanel) pump(w *gpui.Window) {
	p.win = w
	if p.disp != nil {
		p.disp.Drain()
	}
	p.mu.Lock()
	res := p.startRes
	p.startRes = nil
	p.mu.Unlock()
	if res != nil && res.gen == p.gen {
		p.started(res)
	}
	p.drainEvents()
	if p.state == agentIdle {
		return
	}
	next := agentIdlePoll
	if p.state == agentStarting || p.tr.Running() || (p.disp != nil && p.disp.Pending() > 0) {
		next = agentBusyPoll
	}
	p.arm(w, next)
}

func (p *AgentPanel) started(res *agentStart) {
	p.cancelOpen = nil
	if res.err != nil {
		p.tr.Apply(agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: "cannot start " + string(p.selected) + ": " + res.err.Error()})
		p.queued = ""
		p.stop(false)
		return
	}
	p.sess = res.sess
	p.state = agentReady
	if q := p.queued; q != "" {
		p.queued = ""
		p.prompt(q)
	}
}

func (p *AgentPanel) drainEvents() {
	if p.sess == nil {
		return
	}
	ch := p.sess.Events()
	for i := 0; i < agentEventBatch; i++ {
		select {
		case ev, ok := <-ch:
			if !ok {
				p.stop(false)
				return
			}
			p.tr.Apply(ev)
			if _, gone := ev.(agentmanager.Exited); gone {
				p.stop(false)
				return
			}
		default:
			return
		}
	}
}

// statusLine describes the session for the panel header.
func (p *AgentPanel) statusLine() string {
	switch p.state {
	case agentStarting:
		return fmt.Sprintf("starting %s...", p.selected)
	case agentReady:
		if p.tr.Running() {
			return "working..."
		}
		return "ready"
	}
	return "stopped"
}
