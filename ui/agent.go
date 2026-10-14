package ui

import (
	"context"
	"errors"
	"image"
	"slices"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/mcp"
	"agentmanager/store"
	"agentmanager/transcript"
	"cad-lite/cadmcp"
	"cad-lite/cmdsys"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
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
	// images wait in the composer for the next prompt; queuedImages go with queued.
	images, queuedImages []*agentImage
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
	// mdl is the tab's model choice and what its agent reported.
	mdl tabModel

	mu       sync.Mutex
	startRes *agentStart
	// modelRes is the outcome of a model switch running on a goroutine.
	modelRes *modelResult
}

func newAgentTab() *agentTab {
	return &agentTab{expanded: map[int]bool{}, scroll: gpui.NewScrollHandle(), shownVersion: ^uint64(0)}
}

// agentSet is the agent state of one document tab: its conversation tabs, its place in the
// project store and the cad MCP server its sessions share. The server's tools act on that
// document tab's session, so an agent session stays bound to the drawing it was started from
// whichever tab is active (decision D32).
type agentSet struct {
	// doc is the document tab the set belongs to.
	doc  *docTab
	tabs []*agentTab
	cur  *agentTab
	hist agentHistory
	// srv and disp are the cad MCP server shared by the set's live sessions; nil while none is.
	srv  *mcp.Server
	disp *cadmcp.Dispatcher
	ep   mcp.Endpoint
}

func newAgentSet(doc *docTab) *agentSet {
	s := &agentSet{doc: doc, cur: newAgentTab()}
	s.tabs = []*agentTab{s.cur}
	return s
}

// live reports whether a tab of the set has a session starting or open.
func (s *agentSet) live() bool {
	for _, t := range s.tabs {
		if t.state != agentIdle {
			return true
		}
	}
	return false
}

// AgentPanel is the docked chat panel that drives AI agents over the
// drawing: one tab per conversation, each with its own agent session. It shows
// the conversations of the active document tab (the embedded agentSet); the
// sessions of the other document tabs keep running. All of
// its methods and every callback run on the UI goroutine; the only
// cross-goroutine traffic is the cadmcp.Dispatcher mailbox, the sessions'
// event channels (read without blocking) and the start results.
type AgentPanel struct {
	// agentSet is the active document tab's agent state.
	*agentSet
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
	// modelInput is the model menu's free-text field; modelMenu shows the menu.
	modelInput *gpui.TextInput
	modelMenu  bool
	// thumbs caches decoded transcript previews by their stored PNG bytes.
	thumbs map[string]*image.NRGBA
	// deferred is a prompt from the AI command that waits for the first frame.
	deferred string

	timer *gpui.Timer
	tgen  int
	// openers counts start goroutines in flight, so quitting can wait for them and close
	// the session a late start returns.
	openers sync.WaitGroup
}

// agentPromptLines is how many lines the prompt field grows to before it scrolls.
const agentPromptLines = 8

// openerJoinTimeout bounds how long quitting waits for a start in flight.
const openerJoinTimeout = 10 * time.Second

func newAgentPanel(ws *Workspace) *AgentPanel {
	p := &AgentPanel{
		ws:    ws,
		open:  harnesses.Open,
		input: themeBareField(gpui.NewTextInput(i18n.T("ui.agent.input_placeholder")), fontUI, false).Multiline(agentPromptLines),
	}
	p.agentSet = ws.docTab.agentSet()
	p.input.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { p.Send(text, w) })
	p.input.OnEscape(func(w *gpui.Window, _ *gpui.App) { p.returnFocus(w) })
	return p
}

// refreshPlaceholders re-reads the placeholders from the catalog after a
// language change.
func (ws *Workspace) refreshPlaceholders() {
	ws.input.SetPlaceholder(i18n.T("ui.cmdline.placeholder"))
	if ws.agent != nil {
		ws.agent.input.SetPlaceholder(i18n.T("ui.agent.input_placeholder"))
		if ws.agent.modelInput != nil {
			ws.agent.modelInput.SetPlaceholder(i18n.T("ui.agent.model.input_placeholder"))
		}
	}
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
	for _, s := range p.sets() {
		for _, t := range s.tabs {
			p.stopTab(t, true)
		}
	}
	p.joinOpeners()
	for _, s := range p.sets() {
		s.closeRecords()
	}
}

// closeRecords saves which of the set's conversations are open and closes their records.
func (s *agentSet) closeRecords() {
	s.persistOpen()
	for _, t := range s.tabs {
		if t.rec != nil {
			t.rec.Close()
		}
	}
}

// agentSet returns the document tab's agent state, creating it on first use.
func (t *docTab) agentSet() *agentSet {
	if t.agents == nil {
		t.agents = newAgentSet(t)
	}
	return t.agents
}

// sets returns the agent state of every document tab that has one, in tab order.
func (p *AgentPanel) sets() []*agentSet {
	var out []*agentSet
	for _, t := range p.ws.tabs {
		if t.agents != nil {
			out = append(out, t.agents)
		}
	}
	return out
}

// setOf returns the agent state tab t belongs to, or nil for a tab no longer in one.
func (p *AgentPanel) setOf(t *agentTab) *agentSet {
	for _, s := range p.sets() {
		if slices.Contains(s.tabs, t) {
			return s
		}
	}
	return nil
}

// follow shows the conversations of document tab t: the panel follows the active tab. Sessions
// started from the previous tab keep running there.
func (p *AgentPanel) follow(t *docTab) {
	p.agentSet = t.agentSet()
	p.modelMenu = false
	if p.visible {
		p.ensureLoaded()
	}
}

// closeDocument stops every session of document tab t and stores its open conversations, before
// the tab closes.
func (p *AgentPanel) closeDocument(t *docTab) {
	s := t.agents
	if s == nil {
		return
	}
	for _, at := range s.tabs {
		p.stopTab(at, false)
	}
	s.closeRecords()
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

// Live reports whether any tab, of any document tab, has a session starting
// or open, and whether a background tab wants attention.
func (p *AgentPanel) Live() (live, attention bool) {
	for _, s := range p.sets() {
		for _, t := range s.tabs {
			live = live || t.state != agentIdle
			attention = attention || t.attention
		}
	}
	return
}

// probe lists the harnesses installed on PATH, once.
func (p *AgentPanel) probe() {
	if p.probed {
		return
	}
	p.probed = true
	p.addFound()
}

// reprobe adds the harnesses that appeared on the effective PATH since the
// first probe (the login shell's PATH arrives in the background). It never
// removes a choice and does nothing for a panel that was never created.
func (p *AgentPanel) reprobe() {
	if p == nil || !p.probed {
		return
	}
	p.addFound()
}

func (p *AgentPanel) addFound() {
	for _, h := range harnesses.All {
		if _, ok := harnesses.Available(h); ok && !slices.Contains(p.choices, h) {
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

// agentChanged is the MCP refresh hook of document tab d, run on the UI goroutine after a mutating
// tool. Entity edits already reached the render cache through the drawing's change sets, so only a
// layer table edit (which no change set reports) drops the whole cache.
func (p *AgentPanel) agentChanged(d *docTab) {
	d.builder.SyncLayers()
	if p.win != nil {
		p.win.Refresh()
	}
}

// ensureServer starts set s's MCP server on first use. Its tools act on the session of the
// document tab s belongs to, whichever tab is active when a call arrives.
func (p *AgentPanel) ensureServer(s *agentSet) error {
	if s.srv != nil {
		return nil
	}
	d := s.doc
	disp := cadmcp.NewDispatcher(nil)
	srv := mcp.NewServer(mcp.Options{Name: "cad-lite", Version: "1", Instructions: cadmcp.Instructions()})
	cadmcp.Register(srv, cadmcp.Env{
		Dispatch: disp,
		Session:  func() *cmdsys.Session { return d.session },
		Snapshot: SnapshotImage,
		Changed:  func() { p.agentChanged(d) },
	})
	ep, err := srv.Start()
	if err != nil {
		disp.Close()
		return err
	}
	s.srv, s.disp, s.ep = srv, disp, ep
	return nil
}

// Start opens a session for the active tab on its harness, resuming the stored
// agent session when the tab has one. The process launch runs on a goroutine;
// the pump timer picks up the result.
func (p *AgentPanel) Start(w *gpui.Window) {
	p.ensureLoaded()
	if t := p.cur; t.state == agentIdle && p.harnessOf(t) == "" {
		t.tr.Apply(agentmanager.Log{Level: "warn", Message: i18n.T("ui.agent.warn.no_agent")})
		w.Refresh()
		return
	}
	p.startTab(p.cur, w)
}

func (p *AgentPanel) startTab(t *agentTab, w *gpui.Window) {
	h := p.harnessOf(t)
	if t.state != agentIdle || h == "" {
		return
	}
	p.win = w
	set := p.setOf(t)
	if set == nil {
		return
	}
	if err := p.ensureServer(set); err != nil {
		p.apply(t, agentmanager.Log{Level: "error", Message: i18n.T("ui.agent.err.mcp", err)})
		w.Refresh()
		return
	}
	// gen is read under t.mu by start and model-switch goroutines of earlier sessions.
	t.mu.Lock()
	t.gen++
	gen := t.gen
	t.mu.Unlock()
	p.commitSuggestion(t)
	cfg := agentmanager.Config{
		Harness:      h,
		Cwd:          set.doc.projectDir(),
		ResumeID:     p.resumeID(t, h),
		Model:        p.modelFor(t, h),
		SystemPrompt: cadmcp.Instructions(),
		MCPServers:   []agentmanager.MCPServer{{Name: agentMCPName, URL: set.ep.URL, Token: set.ep.Token}},
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

// stopAll stops the sessions of the active document tab's conversations.
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
	t.queued, t.queuedImages = "", nil
	t.mdl.setting, t.mdl.restart, t.mdl.pending = false, false, false
	var srv *mcp.Server
	if s := p.setOf(t); s != nil && !s.live() {
		srv = s.srv
		if s.disp != nil {
			s.disp.Close()
		}
		s.disp, s.srv = nil, nil
	}
	if !p.anyLive() {
		p.timer.Cancel()
		p.timer = nil
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

// anyLive reports whether any document tab has a live agent session.
func (p *AgentPanel) anyLive() bool {
	return slices.ContainsFunc(p.sets(), (*agentSet).live)
}

// Send submits a prompt on the active tab. Without a session it starts one
// first (resuming the tab's stored agent session) and sends the prompt once
// the agent is connected.
//
// The images waiting in the composer go with it; when the tab's model does not read images the
// prompt is refused with a note and text and images stay.
func (p *AgentPanel) Send(text string, w *gpui.Window) {
	p.ensureLoaded()
	text = strings.TrimSpace(text)
	t := p.cur
	imgs := t.images
	if (text == "" && len(imgs) == 0) || t.tr.Running() {
		return
	}
	p.win = w
	if len(imgs) > 0 && p.visionOf(t) == agentmanager.VisionNo {
		p.note(t, i18n.T("ui.agent.image.no_vision_send", p.modelLabel(t)))
		w.Refresh()
		return
	}
	switch t.state {
	case agentIdle:
		if p.harnessOf(t) == "" {
			t.tr.Apply(agentmanager.Log{Level: "warn", Message: i18n.T("ui.agent.warn.no_agent")})
			w.Refresh()
			return
		}
		t.queued, t.queuedImages = text, imgs
		p.sent(t, text, imgs)
		p.startTab(t, w)
	case agentStarting:
		t.queued, t.queuedImages = text, imgs
		p.sent(t, text, imgs)
	case agentReady:
		p.sent(t, text, imgs)
		p.prompt(t, text, imgs)
	}
	w.Refresh()
}

// sent clears the composer and records the user message with its images.
func (p *AgentPanel) sent(t *agentTab, text string, imgs []*agentImage) {
	p.input.SetText("")
	t.images = nil
	p.userMessage(t, text, imgs)
}

func (p *AgentPanel) prompt(t *agentTab, text string, imgs []*agentImage) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	if len(imgs) == 0 {
		err = t.sess.Prompt(ctx, text)
	} else if ip, ok := t.sess.(agentmanager.ImagePrompter); ok {
		send := make([]agentmanager.Image, len(imgs))
		for i, im := range imgs {
			send[i] = im.img
		}
		err = ip.PromptImages(ctx, text, send)
	} else {
		err = agentmanager.ErrNoImages
	}
	if errors.Is(err, agentmanager.ErrNoImages) {
		err = errors.New(i18n.T("ui.agent.image.not_accepted"))
	}
	if err != nil {
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
// adopts finished starts, folds pending agent events of every tab of every
// document tab into its transcript and record, and re-arms itself while a
// session exists.
func (p *AgentPanel) pump(w *gpui.Window) {
	p.win = w
	live, busy, pending := false, false, false
	for _, s := range p.sets() {
		if s.disp != nil {
			s.disp.Drain()
		}
		live, busy = p.pumpTabs(s.tabs, live, busy)
		pending = pending || (s.disp != nil && s.disp.Pending() > 0)
	}
	if !live {
		return
	}
	next := agentIdlePoll
	if busy || pending {
		next = agentBusyPoll
	}
	p.arm(w, next)
}

// pumpTabs adopts the finished starts and model switches of tabs and folds their events; it adds
// to live and busy whether one has a session and whether one needs the fast period.
func (p *AgentPanel) pumpTabs(tabs []*agentTab, live, busy bool) (bool, bool) {
	for _, t := range append([]*agentTab(nil), tabs...) {
		t.mu.Lock()
		res := t.startRes
		t.startRes = nil
		t.mu.Unlock()
		if res != nil && res.gen == t.gen {
			p.started(t, res)
		}
		p.adoptModel(t)
		p.drainEvents(t)
		if t.state != agentIdle {
			live = true
			busy = busy || t.state == agentStarting || t.tr.Running() || t.mdl.setting
		}
	}
	return live, busy
}

func (p *AgentPanel) started(t *agentTab, res *agentStart) {
	t.cancelOpen = nil
	if res.err != nil {
		p.apply(t, agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: i18n.T("ui.agent.err.start", string(t.harness), res.err)})
		t.queued, t.queuedImages = "", nil
		p.stopTab(t, false)
		return
	}
	t.sess = res.sess
	t.state = agentReady
	if t.mdl.pending {
		t.mdl.pending = false
		p.applyModel(t)
	}
	if q, qi := t.queued, t.queuedImages; q != "" || len(qi) > 0 {
		t.queued, t.queuedImages = "", nil
		p.prompt(t, q, qi)
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
			if _, end := ev.(agentmanager.TurnEnd); end && t.mdl.restart {
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
