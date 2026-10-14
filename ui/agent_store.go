package ui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"agentmanager"
	"agentmanager/store"
	"agentmanager/transcript"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// agentStoreDir is the per-project directory of agent conversations, relative to the project
// directory (the directory of the open drawing file, else the working directory).
const agentStoreDir = ".cad-lite/agent"

// legacyAgentStoreDir is where the store lived while the project was called go-draw.
const legacyAgentStoreDir = ".go-draw/agent"

// agentHistoryShown caps the sessions listed in the history menu.
const agentHistoryShown = 8

// agentHistory is the panel's persistence state for the current project and drawing.
type agentHistory struct {
	// dir is the project directory the store belongs to; "" until the panel is first opened.
	dir string
	// drawing is the drawing whose open tabs the panel shows: the file name of the drawing in dir,
	// "" for an untitled drawing.
	drawing string
	loaded  bool
	// off disables persistence (the canned demo).
	off      bool
	st       *store.Store
	sessions []store.Meta
	menu     bool
	// failed is set once a problem was reported, so it is shown once.
	failed bool
}

// projectDir returns the directory the tab's agent history is stored under: the directory of the
// last file opened, saved or passed to SetProjectPath, else the working directory.
func (t *docTab) projectDir() string {
	if t.projectPath != "" {
		if abs, err := filepath.Abs(t.projectPath); err == nil {
			return filepath.Dir(abs)
		}
		return filepath.Dir(t.projectPath)
	}
	wd, _ := os.Getwd()
	return wd
}

// drawingKey names the drawing's open tabs in the project store: the drawing file's name, or ""
// for an untitled drawing.
func (t *docTab) drawingKey() string {
	if t.projectPath == "" {
		return ""
	}
	return filepath.Base(t.projectPath)
}

// SetProjectPath tells the workspace which file the drawing was loaded from, for a file opened
// before the workspace was built. OPEN and SAVE do this themselves.
func (ws *Workspace) SetProjectPath(path string) { ws.session.SetFilePath(path) }

// pathChanged follows the file path of tab t's session. The drawing is the same one when it was
// saved (SAVE, SAVEAS) and another one when it was opened or replaced (OPEN, NEW): a saved drawing
// takes its agent tabs along, another drawing brings its own. Paths change only through the
// commands of the active tab, whose conversations the panel shows.
func (ws *Workspace) pathChanged(t *docTab, path string) {
	d := t.session.Drawing()
	saved := t.pathDoc == d
	t.pathDoc = d
	t.projectPath = path
	if ws.agent != nil && t == ws.docTab {
		ws.agent.drawingChanged(saved)
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
	h.dir, h.drawing = p.doc.projectDir(), p.doc.drawingKey()
	migrateAgentStore(h.dir)
	st, err := agentStore(h.dir)
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.history_unavailable", err))
		return
	}
	h.st = st
	p.syncSessions()
	var ids []string
	active := ""
	set, err := st.LoadOpenFor(h.drawing)
	adopt := false
	if errors.Is(err, store.ErrNotFound) && h.drawing != "" {
		// A store from before per-drawing tabs: the first drawing opened takes its open tabs.
		if legacy, lerr := st.LoadOpen(); lerr == nil && legacy.Legacy {
			set, err, adopt = legacy, nil, true
		}
	}
	if err == nil {
		ids, active = set.IDs, set.Active
	} else if h.drawing == "" {
		// No open set yet: an untitled drawing shows the latest conversation of no drawing.
		for _, m := range h.sessions {
			if m.Drawing == "" {
				ids = []string{m.ID}
				break
			}
		}
	}
	var tabs []*agentTab
	var cur *agentTab
	for _, id := range ids {
		rec, err := st.Load(id)
		if err != nil {
			continue
		}
		t := storedTab(rec)
		tabs = append(tabs, t)
		if id == active {
			cur = t
		}
	}
	if len(tabs) > 0 {
		if cur == nil {
			cur = tabs[len(tabs)-1]
		}
		p.tabs, p.cur = tabs, cur
	}
	if adopt {
		p.persistOpen()
		p.check(st.DeleteOpenFor(""))
	}
}

// agentStore opens the conversation store of the project directory dir.
func agentStore(dir string) (*store.Store, error) {
	return store.Open(filepath.Join(dir, filepath.FromSlash(agentStoreDir)))
}

// storedTab is a tab showing the stored conversation rec, with the model it was on.
func storedTab(rec *store.Record) *agentTab {
	t := newAgentTab()
	t.rec = rec
	rec.Replay(&t.tr)
	t.mdl.chosen = rec.Meta().Model
	for _, it := range rec.Items() {
		if ev, ok := it.(agentmanager.Event); ok {
			t.mdl.observe(ev)
		}
	}
	return t
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

// persistOpen stores which sessions are open for the drawing and which tab is active.
func (p *agentSet) persistOpen() {
	h := &p.hist
	if !h.loaded || h.off || h.st == nil {
		return
	}
	p.check(h.st.SaveOpenFor(h.drawing, p.openSet()))
}

// openSet lists the stored sessions of the open tabs and the active one.
func (p *agentSet) openSet() store.OpenSet {
	set := store.OpenSet{}
	for _, t := range p.tabs {
		if t.rec != nil {
			set.IDs = append(set.IDs, t.rec.Meta().ID)
			if t == p.cur {
				set.Active = t.rec.Meta().ID
			}
		}
	}
	return set
}

// Tab describes one panel tab, for the view and tests.
type Tab struct {
	// Title is the first prompt, clipped; "New" for an empty tab.
	Title string
	// SessionID is the stored id, "" before the first prompt.
	SessionID string
	Harness   agentmanager.Harness
	// Model is the model the tab uses, as the panel shows it: the chosen model's name, else the
	// default from the settings, else what the agent reported, else "default".
	Model string
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
		out[i] = Tab{Title: p.tabTitle(t), Harness: p.harnessOf(t), Model: p.modelLabel(t), Live: t.state != agentIdle,
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
	t := storedTab(rec)
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

// drawingChanged follows the drawing file. A drawing saved under a new name takes its tabs along:
// in the same project the open set moves to the new name and the sessions keep running; in another
// project the open conversations are copied into that project's store and their sessions closed.
// Another drawing (OPEN, NEW) closes every live session, keeps the records and restores the tabs
// that drawing had open.
func (p *AgentPanel) drawingChanged(saved bool) {
	h := &p.hist
	dir, key := p.doc.projectDir(), p.doc.drawingKey()
	if !h.loaded || h.off || (dir == h.dir && key == h.drawing) {
		return
	}
	if saved && dir == h.dir {
		old := h.drawing
		h.drawing = key
		p.tagDrawing()
		p.persistOpen()
		if h.st != nil {
			p.check(h.st.DeleteOpenFor(old))
		}
		p.refresh()
		return
	}
	carry, from, oldKey := p.openSet(), h.st, h.drawing
	p.stopAll(false)
	for _, t := range p.tabs {
		if t.rec != nil {
			t.rec.Close()
		}
	}
	if saved && from != nil && len(carry.IDs) > 0 {
		migrateAgentStore(dir)
		if to, err := agentStore(dir); err == nil {
			var ids []string
			for _, id := range carry.IDs {
				if err := from.CopyTo(to, id); err == nil {
					ids = append(ids, id)
				} else {
					p.check(err)
				}
			}
			p.check(to.SaveOpenFor(key, store.OpenSet{IDs: ids, Active: carry.Active}))
			p.check(from.DeleteOpenFor(oldKey))
		}
	}
	p.cur = newAgentTab()
	p.tabs = []*agentTab{p.cur}
	*h = agentHistory{}
	p.ensureLoaded()
	if saved {
		p.tagDrawing()
	}
	p.refresh()
}

// tagDrawing records the current drawing in the record of every open tab.
func (p *AgentPanel) tagDrawing() {
	for _, t := range p.tabs {
		if t.rec != nil {
			p.check(t.rec.SetDrawing(p.hist.drawing))
		}
	}
}

func (p *AgentPanel) refresh() {
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
	if h.drawing != "" {
		p.check(rec.SetDrawing(h.drawing))
	}
	if t.mdl.chosen != "" {
		p.check(rec.SetModel(t.mdl.chosen))
	}
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
	t.mdl.observe(ev)
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

func (p *agentSet) check(err error) {
	if err != nil {
		p.warnOnce(p.cur, i18n.T("ui.agent.warn.write_failed", err))
	}
}

// warnOnce shows a warning in the transcript of t, once per project. It is not stored.
func (p *agentSet) warnOnce(t *agentTab, msg string) {
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

// migrateAgentStore copies the legacy store of the project directory dir into the current one
// when only the legacy one exists. The legacy folder is left in place; failures are ignored and
// the project then starts with an empty history.
func migrateAgentStore(dir string) {
	dst := filepath.Join(dir, filepath.FromSlash(agentStoreDir))
	src := filepath.Join(dir, filepath.FromSlash(legacyAgentStoreDir))
	if _, err := os.Stat(dst); err == nil {
		return
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return
	}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o644)
	})
	if err != nil {
		os.RemoveAll(dst)
	}
}
