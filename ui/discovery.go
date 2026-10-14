package ui

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"
	"weak"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/internal/gpui"
)

// Timing of the agent discovery probe.
const (
	// discoveryPoll is how often the UI looks at the worker while it runs.
	discoveryPoll = 200 * time.Millisecond
	// discoveryTimeout bounds one whole probe: the login shell plus the
	// version calls.
	discoveryTimeout = 15 * time.Second
)

// AgentDiscovery is what one probe of the user's environment found.
type AgentDiscovery struct {
	// Source says where the extra PATH came from; shellenv.SourceApp when the
	// login shell or registry could not be read (Err says why).
	Source shellenv.Source
	Err    error
	// AppPATH is the PATH this process started with; LoginPATH the one the
	// login shell or registry reported; Added the folders only LoginPATH has.
	AppPATH, LoginPATH string
	Added              []string
	// Agents has one entry per agent, in the order OpenCode, Claude, Codex.
	Agents []harnesses.Found
	// At is when the probe finished.
	At time.Time
}

// AgentDiscoveryProbe finds the user's PATH and the agents on it. The
// default probe runs the login shell; tests install a fake with
// SetAgentDiscoveryProbe. It runs on a goroutine and must not touch the UI.
type AgentDiscoveryProbe func(ctx context.Context) AgentDiscovery

// defaultDiscoveryProbe asks the login shell (or registry) for the PATH,
// stores it for launching agents (shellenv.Set) and looks the agents up.
func defaultDiscoveryProbe(ctx context.Context) AgentDiscovery {
	app := os.Getenv("PATH")
	r := shellenv.Run(ctx)
	if r.Err == nil {
		shellenv.Set(r.Path)
	}
	d := AgentDiscovery{Source: r.Source, Err: r.Err, AppPATH: app, LoginPATH: r.Path, Added: shellenv.Added(app, r.Path)}
	d.Agents = harnesses.Discover(ctx, shellenv.Effective())
	d.At = time.Now()
	return d
}

// discoveryState is a workspace's agent discovery state. Everything but mu
// and res belongs to the UI goroutine; the worker goroutine writes only
// those, and the poll timer moves the result across.
type discoveryState struct {
	probe   AgentDiscoveryProbe
	enabled bool
	started bool
	running bool

	last     *AgentDiscovery
	showPath bool
	// term is the outcome of the last "Open terminal" click; termErr is set
	// when it failed.
	term    string
	termErr bool
	open    func(dir string) (string, error)

	poll *gpui.Timer

	mu  sync.Mutex
	res *AgentDiscovery
}

var discTable = struct {
	sync.Mutex
	m map[weak.Pointer[Workspace]]*discoveryState
}{m: map[weak.Pointer[Workspace]]*discoveryState{}}

// disc returns ws's discovery state, creating it on first use. Like prefs,
// it lives beside the Workspace so the Workspace stays collectable.
func (ws *Workspace) disc() *discoveryState {
	key := weak.Make(ws)
	discTable.Lock()
	defer discTable.Unlock()
	if d, ok := discTable.m[key]; ok {
		return d
	}
	d := &discoveryState{probe: defaultDiscoveryProbe, open: OpenTerminal}
	discTable.m[key] = d
	runtime.AddCleanup(ws, func(k weak.Pointer[Workspace]) {
		discTable.Lock()
		delete(discTable.m, k)
		discTable.Unlock()
	}, key)
	return d
}

// SetAgentDiscoveryProbe replaces how the user's environment is probed
// (tests install a fake).
func (ws *Workspace) SetAgentDiscoveryProbe(f AgentDiscoveryProbe) {
	if f == nil {
		f = defaultDiscoveryProbe
	}
	ws.disc().probe = f
}

// SetTerminalOpener replaces how the "Open terminal" button opens a terminal
// in a folder (tests install a fake). f returns the name of what it opened.
func (ws *Workspace) SetTerminalOpener(f func(dir string) (string, error)) {
	if f == nil {
		f = OpenTerminal
	}
	ws.disc().open = f
}

// EnableAgentDiscovery runs the probe once in the background so agents
// installed outside the GUI's own PATH are found without any click. Hosts
// that must not run programs (-png renders, tests) do not call it.
func (ws *Workspace) EnableAgentDiscovery() {
	d := ws.disc()
	if d.enabled {
		return
	}
	d.enabled = true
	ws.runDiscovery()
}

// AgentDiscovery returns the last probe's result, if one has finished.
func (ws *Workspace) AgentDiscovery() (AgentDiscovery, bool) {
	if d := ws.disc(); d.last != nil {
		return *d.last, true
	}
	return AgentDiscovery{}, false
}

// runDiscovery starts a probe on a goroutine unless one is running.
func (ws *Workspace) runDiscovery() {
	d := ws.disc()
	if d.running {
		return
	}
	d.running = true
	probe := d.probe
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
		defer cancel()
		r := probe(ctx)
		d.mu.Lock()
		d.res = &r
		d.mu.Unlock()
	}()
}

// startDiscovery arms the poll for a probe begun before the window existed;
// Render calls it every frame through mountSettings.
func (ws *Workspace) startDiscovery(w *gpui.Window) {
	d := ws.disc()
	if d.started || w == nil || !d.running {
		return
	}
	d.started = true
	ws.armDiscoveryPoll(w)
}

func (ws *Workspace) armDiscoveryPoll(w *gpui.Window) {
	d := ws.disc()
	d.poll.Cancel()
	d.poll = w.After(discoveryPoll, func(w *gpui.Window, _ *gpui.App) { ws.pollDiscovery(w) })
}

// checkAgents is the Check button: it re-runs the probe.
func (ws *Workspace) checkAgents(w *gpui.Window) {
	d := ws.disc()
	if d.running {
		return
	}
	d.term, d.termErr = "", false
	ws.runDiscovery()
	ws.armDiscoveryPoll(w)
	w.Refresh()
}

// pollDiscovery is the poll timer body, on the UI goroutine: it adopts the
// worker's result and keeps polling while the worker runs.
func (ws *Workspace) pollDiscovery(w *gpui.Window) {
	d := ws.disc()
	d.mu.Lock()
	res := d.res
	d.res = nil
	d.mu.Unlock()
	if res != nil {
		d.running = false
		d.last = res
		ws.agent.reprobe()
	}
	if d.running {
		ws.armDiscoveryPoll(w)
	}
	w.Refresh()
}

// openTerminal is the Open terminal button.
func (ws *Workspace) openTerminal(w *gpui.Window) {
	d := ws.disc()
	name, err := d.open(ws.projectDir())
	if err != nil {
		d.term, d.termErr = err.Error(), true
	} else {
		d.term, d.termErr = name, false
	}
	w.Refresh()
}

// found returns the entry for h in the last probe.
func (d *discoveryState) found(h agentmanager.Harness) (harnesses.Found, bool) {
	if d.last == nil {
		return harnesses.Found{Harness: h}, false
	}
	for _, f := range d.last.Agents {
		if f.Harness == h {
			return f, true
		}
	}
	return harnesses.Found{Harness: h}, false
}
