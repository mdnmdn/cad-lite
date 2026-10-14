package cmdsys

import "cad-lite/doc"

// This file is the session's file and history state: the modified counter behind DBMOD and
// Session.Dirty, the host's quit hook, and what UNDO and REDO remember between commands.

// UndoControl is the state UNDO Control sets.
type UndoControl int

const (
	// UndoAll allows U and UNDO to any depth (the default).
	UndoAll UndoControl = iota
	// UndoNone turns U and UNDO off.
	UndoNone
	// UndoOne limits U and UNDO to a single step at a time.
	UndoOne
)

// HistoryState is what UNDO, U, REDO and MREDO share for one session. It lives with the session,
// not the drawing, and is reset when the drawing is replaced.
type HistoryState struct {
	// Control is the UNDO Control setting. Combine and Layer are accepted and change nothing, as
	// the journal always merges steps and never records layer edits.
	Control UndoControl
	// Auto is the UNDO Auto setting. It is kept for the prompt and has no other effect, because a
	// command is already one undo step.
	Auto bool
	// Batches are the sizes of the recent undo operations, newest last: one REDO redoes a whole
	// batch (an UNDO 2 is one REDO). They are meaningful only while their sum equals the journal's
	// redo depth; Redoable reconciles them.
	Batches []int
}

// fileState is the per-session state of this file.
type fileState struct {
	mods    int
	quit    func()
	host    DocumentHost
	path    func() string
	history HistoryState
}

func newFileState(path func() string) *fileState {
	return &fileState{path: path, history: HistoryState{Auto: true}}
}

// attach starts counting d's changes from zero and forgets the undo state of the previous drawing.
// Every published change (a committed transaction, an undo, a redo) counts once.
func (f *fileState) attach(d *doc.Drawing) {
	f.mods = 0
	f.history.Batches = nil
	d.OnChange(func(doc.ChangeSet) { f.mods++ })
}

func init() { RegisterVarGroup(dirtyVars) }

// dirtyVars registers DBMOD: 1 while the drawing has unsaved changes, else 0 (AutoCAD's bit 1,
// the object database). The other bits describe state Cad-lite does not keep.
func dirtyVars(s *Session, v *Vars) {
	v.Register(Var{Name: "DBMOD", Kind: VarInt, Default: 0, ReadOnly: true,
		Read: func() any { return boolInt(s.Dirty()) }})
}

// Modified returns the number of changes made to the drawing since it was created, opened or
// saved: every committed transaction, undo and redo adds one. It is zero for a clean drawing.
func (s *Session) Modified() int { return s.files.mods }

// Dirty reports whether the drawing has unsaved changes. The host shows it as "*" and asks before
// discarding the drawing.
func (s *Session) Dirty() bool { return s.files.mods != 0 }

// MarkSaved records that the drawing was just saved: Dirty is false until the next change. The
// SAVE family of commands calls it; a host that saves by itself must too.
func (s *Session) MarkSaved() { s.files.mods = 0 }

// SetQuitHandler installs the host's way of ending the application, which QUIT and EXIT call once
// any unsaved changes are dealt with. Without one they report that quitting is not available.
func (s *Session) SetQuitHandler(f func()) { s.files.quit = f }

// DocumentHost is the host's side of the file commands when it keeps several drawings open, one
// tab each, with a session per tab. The host installs one per session (Session.SetDocumentHost),
// bound to that session's tab. Without one, OPEN replaces the session's drawing and NEW and CLOSE
// empty it.
type DocumentHost interface {
	// ShowOpen switches to the tab that has the file path open and reports whether one has. OPEN
	// asks before it reads the file.
	ShowOpen(path string) bool
	// OpenNew puts d, just read from path, in a tab of its own, records path as its file, makes
	// it the active tab and returns its session. It returns nil when OPEN should replace this
	// session's drawing instead (the host's choice for an untouched untitled one).
	OpenNew(path string, d *doc.Drawing) *Session
	// New opens an empty drawing in a new active tab and reports whether it did.
	New() bool
	// Close closes this session's tab once its unsaved changes are dealt with and reports whether
	// it did; false (the last tab) has CLOSE empty the drawing.
	Close() bool
}

// SetDocumentHost installs the host that gives each drawing a tab of its own (see DocumentHost).
func (s *Session) SetDocumentHost(h DocumentHost) { s.files.host = h }

// Documents returns the session's document host, or nil when the host keeps one drawing.
func (c *Context) Documents() DocumentHost { return c.files.host }

// Modified returns the session's modified counter (see Session.Modified).
func (c *Context) Modified() int { return c.files.mods }

// Dirty reports whether the drawing has unsaved changes.
func (c *Context) Dirty() bool { return c.files.mods != 0 }

// MarkSaved records that the drawing was just saved.
func (c *Context) MarkSaved() { c.files.mods = 0 }

// FilePath returns the path of the drawing's file, or "" for a drawing that has none.
func (c *Context) FilePath() string { return c.files.path() }

// Quit asks the host to end the application and reports whether there is a host to ask.
func (c *Context) Quit() bool {
	if c.files.quit == nil {
		return false
	}
	c.files.quit()
	return true
}

// History returns the session's undo and redo state.
func (c *Context) History() *HistoryState { return &c.files.history }

// Redoable returns the number of steps one REDO redoes: the latest undo batch. It drops batches
// the journal no longer agrees with (a new change discards the redo steps).
func (h *HistoryState) Redoable(d *doc.Drawing) int {
	sum := 0
	for _, b := range h.Batches {
		sum += b
	}
	if sum != d.RedoDepth() {
		h.Batches = nil
		if n := d.RedoDepth(); n > 0 {
			h.Batches = []int{n}
		}
	}
	if len(h.Batches) == 0 {
		return 0
	}
	return h.Batches[len(h.Batches)-1]
}

// Undone records an undo that took n steps and has just happened on d. Batches left from before
// a new change discarded the redo steps no longer match the journal and are dropped first.
func (h *HistoryState) Undone(d *doc.Drawing, n int) {
	if n <= 0 {
		return
	}
	sum := 0
	for _, b := range h.Batches {
		sum += b
	}
	if sum != d.RedoDepth()-n {
		h.Batches = nil
	}
	h.Batches = append(h.Batches, n)
}

// Redone records that n steps were redone, newest batch first.
func (h *HistoryState) Redone(n int) {
	for n > 0 && len(h.Batches) > 0 {
		last := &h.Batches[len(h.Batches)-1]
		if *last > n {
			*last -= n
			return
		}
		n -= *last
		h.Batches = h.Batches[:len(h.Batches)-1]
	}
}
