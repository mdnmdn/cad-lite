package ui

import (
	"os"
	"path/filepath"
	"time"

	"agentmanager"
	"agentmanager/store"
	"agentmanager/transcript"
	"go-draw/i18n"
	"go-draw/internal/gpui"
)

// agentStoreDir is the per-project directory of agent conversations, relative to the project
// directory (the directory of the open drawing file, else the working directory).
const agentStoreDir = ".go-draw/agent"

// agentHistoryShown caps the sessions listed in the history menu.
const agentHistoryShown = 8

// agentHistory is the panel's persistence state for the current project.
type agentHistory struct {
	// dir is the project directory the store belongs to; "" until the panel is first opened.
	dir    string
	loaded bool
	// off disables persistence (the canned demo).
	off      bool
	st       *store.Store
	sessions []store.Meta
	menu     bool
	// failed is set once a problem was reported, so it is shown once.
	failed bool
}

// projectDir returns the directory agent history is stored under: the directory of the last file
// opened, saved or passed to SetProjectPath, else the working directory.
func (ws *Workspace) projectDir() string {
	if ws.projectPath != "" {
		if abs, err := filepath.Abs(ws.projectPath); err == nil {
			return filepath.Dir(abs)
		}
		return filepath.Dir(ws.projectPath)
	}
	wd, _ := os.Getwd()
	return wd
}

// SetProjectPath tells the workspace which file the drawing was loaded from, for a file opened
// before the workspace was built. OPEN and SAVE do this themselves.
func (ws *Workspace) SetProjectPath(path string) { ws.session.SetFilePath(path) }

// pathChanged follows the session's file path: a file in another directory is another project.
func (ws *Workspace) pathChanged(path string) {
	ws.projectPath = path
	if ws.agent != nil {
		ws.agent.projectChanged()
	}
}

// ensureLoaded opens the project's store once and restores the conversations that were open when
// the project was last used, as tabs with their transcripts. No agent starts: a tab resumes its
// agent session when the user sends to it.
func (p *AgentPanel) ensureLoaded() {
	h := &p.hist
	if h.loaded {
		return
	}
	h.loaded = true
	if h.off {
		return
	}
	h.dir = p.ws.projectDir()
	st, err := store.Open(filepath.Join(h.dir, filepath.FromSlash(agentStoreDir)))
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.history_unavailable", err))
		return
	}
	h.st = st
	p.syncSessions()
	var ids []string
	active := ""
	if set, err := st.LoadOpen(); err == nil {
		ids, active = set.IDs, set.Active
	} else if len(h.sessions) > 0 {
		ids = []string{h.sessions[0].ID}
	}
	var tabs []*agentTab
	var cur *agentTab
	for _, id := range ids {
		rec, err := st.Load(id)
		if err != nil {
			continue
		}
		t := newAgentTab()
		t.rec = rec
		rec.Replay(&t.tr)
		tabs = append(tabs, t)
		if id == active {
			cur = t
		}
	}
	if len(tabs) == 0 {
		return
	}
	if cur == nil {
		cur = tabs[len(tabs)-1]
	}
	p.tabs, p.cur = tabs, cur
}

// syncSessions refreshes the history list from disk.
func (p *AgentPanel) syncSessions() {
	if p.hist.st == nil {
		p.hist.sessions = nil
		return
	}
	ms, err := p.hist.st.List()
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.cannot_list", err))
	}
	p.hist.sessions = ms
}

// persistOpen stores which sessions are open and which tab is active.
func (p *AgentPanel) persistOpen() {
	h := &p.hist
	if !h.loaded || h.off || h.st == nil {
		return
	}
	set := store.OpenSet{}
	for _, t := range p.tabs {
		if t.rec != nil {
			set.IDs = append(set.IDs, t.rec.Meta().ID)
			if t == p.cur {
				set.Active = t.rec.Meta().ID
			}
		}
	}
	p.check(h.st.SaveOpen(set))
}

// Tab describes one panel tab, for the view and tests.
type Tab struct {
	// Title is the first prompt, clipped; "New" for an empty tab.
	Title string
	// SessionID is the stored id, "" before the first prompt.
	SessionID string
	Harness   agentmanager.Harness
	// Live is true while an agent session is starting or open.
	Live      bool
	Working   bool
	Attention bool
	Active    bool
}

// Tabs returns the panel's tabs in order.
func (p *AgentPanel) Tabs() []Tab {
	out := make([]Tab, len(p.tabs))
	for i, t := range p.tabs {
		out[i] = Tab{Title: p.tabTitle(t), Harness: p.harnessOf(t), Live: t.state != agentIdle,
			Working: t.tr.Running(), Attention: t.attention, Active: t == p.cur}
		if t.rec != nil {
			out[i].SessionID = t.rec.Meta().ID
		}
	}
	return out
}

func (p *AgentPanel) tabTitle(t *agentTab) string {
	for _, b := range t.tr.Blocks() {
		if u, ok := b.(transcript.UserMsg); ok {
			return clip(u.Text, 18)
		}
	}
	if t.rec != nil {
		if m := t.rec.Meta(); m.Title != "" {
			return clip(m.Title, 18)
		}
	}
	return i18n.T("ui.agent.tab_new")
}

// NewTab adds an empty conversation and makes it active; an empty active tab is reused. The
// session starts on the first prompt.
func (p *AgentPanel) NewTab(w *gpui.Window) {
	p.ensureLoaded()
	p.hist.menu = false
	if t := p.cur; t.rec == nil && t.state == agentIdle && len(t.tr.Blocks()) == 0 {
		w.Refresh()
		return
	}
	t := newAgentTab()
	p.tabs = append(p.tabs, t)
	p.activate(t)
	w.Refresh()
}

func (p *AgentPanel) activate(t *agentTab) {
	p.cur = t
	t.attention = false
	p.persistOpen()
}

// SwitchTab makes tab i active; the other tabs' sessions keep running.
func (p *AgentPanel) SwitchTab(i int, w *gpui.Window) {
	if i >= 0 && i < len(p.tabs) {
		p.activate(p.tabs[i])
	}
	w.Refresh()
}

// CloseTab ends tab i's session and removes the tab; its record stays in the history. Closing the
// last tab leaves an empty one.
func (p *AgentPanel) CloseTab(i int, w *gpui.Window) {
	if i < 0 || i >= len(p.tabs) {
		return
	}
	t := p.tabs[i]
	p.stopTab(t, false)
	if t.rec != nil {
		t.rec.Close()
	}
	p.tabs = append(p.tabs[:i:i], p.tabs[i+1:]...)
	if len(p.tabs) == 0 {
		p.tabs = []*agentTab{newAgentTab()}
	}
	if p.cur == t {
		p.cur = p.tabs[min(i, len(p.tabs)-1)]
		p.cur.attention = false
	}
	p.syncSessions()
	p.persistOpen()
	w.Refresh()
}

// OpenSession shows the stored conversation id in a tab (switching to it when already open);
// sending to it resumes the agent session.
func (p *AgentPanel) OpenSession(id string, w *gpui.Window) {
	p.ensureLoaded()
	p.hist.menu = false
	for _, t := range p.tabs {
		if t.rec != nil && t.rec.Meta().ID == id {
			p.activate(t)
			w.Refresh()
			return
		}
	}
	if p.hist.st == nil {
		return
	}
	rec, err := p.hist.st.Load(id)
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.cannot_load", err))
		p.syncSessions()
		w.Refresh()
		return
	}
	t := newAgentTab()
	t.rec = rec
	rec.Replay(&t.tr)
	if c := p.cur; c.rec == nil && c.state == agentIdle && len(c.tr.Blocks()) == 0 {
		for i := range p.tabs {
			if p.tabs[i] == c {
				p.tabs[i] = t
			}
		}
	} else {
		p.tabs = append(p.tabs, t)
	}
	p.activate(t)
	w.Refresh()
}

// Sessions returns the stored conversations of the current project, newest first.
func (p *AgentPanel) Sessions() []store.Meta {
	p.ensureLoaded()
	p.syncSessions()
	return p.hist.sessions
}

// projectChanged switches to the store of the new project: every live session is closed and its
// record kept; a panel that was open restores the new project's conversations.
func (p *AgentPanel) projectChanged() {
	h := &p.hist
	if !h.loaded || h.off || p.ws.projectDir() == h.dir {
		return
	}
	p.stopAll(false)
	for _, t := range p.tabs {
		if t.rec != nil {
			t.rec.Close()
		}
	}
	p.cur = newAgentTab()
	p.tabs = []*agentTab{p.cur}
	*h = agentHistory{}
	p.ensureLoaded()
	if p.win != nil {
		p.win.Refresh()
	}
}

// prepareRecord makes sure tab t has a record before a user message is added.
func (p *AgentPanel) prepareRecord(t *agentTab) {
	h := &p.hist
	if h.st == nil || t.rec != nil {
		return
	}
	rec, err := h.st.Create(p.harnessOf(t))
	if err != nil {
		p.warnOnce(t, i18n.T("ui.agent.warn.not_saved", err))
		return
	}
	t.rec = rec
	p.syncSessions()
	p.persistOpen()
}

// resumeID is the agent session id tab t resumes, when its record has one for harness h.
func (p *AgentPanel) resumeID(t *agentTab, h agentmanager.Harness) string {
	if r := t.rec; r != nil && r.Meta().Harness == string(h) {
		return r.Meta().AgentSessionID
	}
	return ""
}

// apply folds ev into tab t's transcript and stores it. Debug log lines are neither shown nor
// stored. A background tab is flagged when it needs the user.
func (p *AgentPanel) apply(t *agentTab, ev agentmanager.Event) {
	t.tr.Apply(ev)
	if t != p.cur {
		switch ev.(type) {
		case agentmanager.PermissionRequest, agentmanager.TurnEnd:
			t.attention = true
		}
	}
	if l, ok := ev.(agentmanager.Log); ok && l.Level == "debug" {
		return
	}
	if t.rec != nil {
		p.check(t.rec.Append(ev))
	}
}

// userMessage adds and stores a message the user sent to tab t.
func (p *AgentPanel) userMessage(t *agentTab, text string) {
	p.prepareRecord(t)
	t.tr.AddUser(text)
	if t.rec != nil {
		p.check(t.rec.AddUser(text))
	}
}

func (p *AgentPanel) check(err error) {
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.write_failed", err))
	}
}

// warnOnce shows a warning in the transcript of t, once per project. It is not stored.
func (p *AgentPanel) warnOnce(t *agentTab, msg string) {
	if p.hist.failed {
		return
	}
	p.hist.failed = true
	t.tr.Apply(agentmanager.Log{Level: "warn", Message: msg})
}

// relTime renders t relative to now for the history list.
func relTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return i18n.T("ui.agent.time.just_now")
	case d < time.Hour:
		return i18n.T("ui.agent.time.minutes_ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return i18n.T("ui.agent.time.hours_ago", int(d/time.Hour))
	case d < 7*24*time.Hour:
		return i18n.T("ui.agent.time.days_ago", int(d/(24*time.Hour)))
	}
	return t.Format(i18n.T("ui.agent.time.date_layout"))
}

// HasAgentPanel reports whether the agent panel exists. It is created the first time the panel is
// opened or the AI command runs; until then nothing agent related is allocated.
func (ws *Workspace) HasAgentPanel() bool { return ws.agent != nil }

// TabTranscript returns the transcript of tab i, for tests.
func (p *AgentPanel) TabTranscript(i int) *transcript.T { return &p.tabs[i].tr }
