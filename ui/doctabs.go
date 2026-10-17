package ui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
	"cad-lite/settings"
)

// docTab is one open drawing, shown as a document tab: its session (the drawing, the selection,
// the undo journal, the command history and the drafting aids), its view, display cache and canvas
// state, and the file it came from. The Workspace embeds the active one.
type docTab struct {
	session *cmdsys.Session
	view    *render.View
	builder *render.Builder
	canvas  canvasState
	// fitPending zooms to extents once the canvas size is known.
	fitPending bool
	// projectPath is the drawing's file, which decides where agent history lives.
	projectPath string
	// pathDoc is the drawing projectPath was last set for: the same drawing at the next path
	// change means it was saved, another one that it was opened or replaced.
	pathDoc *doc.Drawing
	// agents is the agent panel's state for this drawing; nil until the panel shows the tab.
	agents *agentSet
}

// docTabHeight is the height of the document tab strip.
const docTabHeight = 30

// newDocTab returns a tab editing d and adds it at the end of the strip; it does not make it
// active. Its view takes the canvas size of the active tab, so it is framed on the first frame.
func (ws *Workspace) newDocTab(d *doc.Drawing) *docTab {
	v := render.DefaultView(800, 600)
	t := &docTab{view: &v, fitPending: d.Len() > 0, pathDoc: d}
	if cur := ws.docTab; cur != nil {
		v.Width, v.Height = cur.view.Width, cur.view.Height
		t.canvas.bounds, t.canvas.mouse, t.canvas.hover = cur.canvas.bounds, cur.canvas.mouse, cur.canvas.hover
	}
	s := cmdsys.NewSession(d, t.view, commands.NewRegistry())
	t.session = s
	s.SetFiles(ws.files)
	s.SetAnalytics(ws.analytics)
	t.attach(d)
	s.OnDrawing(t.attach)
	s.OnFilePath(func(path string) { ws.pathChanged(t, path) })
	s.OnFilePath(ws.noteRecentFile)
	s.SetAgentHandler(ws.agentCommand)
	s.SetTextEditHandler(func(h doc.Handle) bool { return ws.openTextEditor(t, h) })
	s.SetSettingsHandler(func() {
		if ws.win != nil {
			ws.ToggleSettings(ws.win)
		}
	})
	s.SetDocumentHost(tabHost{ws: ws, t: t})
	if ws.quit.f != nil {
		s.SetQuitHandler(func() { ws.quitFrom(t) })
	}
	if ws.aidsSet {
		ws.applyDrawingDefaultsTo(s)
	}
	ws.tabs = append(ws.tabs, t)
	return t
}

// attach builds the display cache for a (new) drawing.
func (t *docTab) attach(d *doc.Drawing) {
	t.builder = render.NewBuilder(d, colCanvas)
	d.OnChange(t.builder.Invalidate)
}

// untouched reports whether the tab holds the empty untitled drawing nothing was done to, which a
// file opened next replaces instead of opening beside it.
func (t *docTab) untouched() bool {
	return t.session.FilePath() == "" && !t.session.Dirty() && t.session.Drawing().Len() == 0
}

// name is the tab's caption: the file name, or the translated "Untitled".
func (t *docTab) name() string {
	if p := t.session.FilePath(); p != "" {
		return filepath.Base(p)
	}
	return i18n.T("ui.title.untitled")
}

// tabHost is the cmdsys.DocumentHost of one tab's session: OPEN, NEW and CLOSE reach the tab strip
// through it.
type tabHost struct {
	ws *Workspace
	t  *docTab
}

// ShowOpen switches to the tab that has path open.
func (h tabHost) ShowOpen(path string) bool {
	o := h.ws.tabOf(path)
	if o == nil {
		return false
	}
	h.ws.activate(o)
	return true
}

// OpenNew opens d in a new tab unless this tab is an untouched untitled one, which OPEN replaces.
func (h tabHost) OpenNew(path string, d *doc.Drawing) *cmdsys.Session {
	if h.t.untouched() {
		return nil
	}
	ws := h.ws
	t := ws.newDocTab(d)
	// The file is known before the tab shows, so the agent panel shows that drawing's conversations.
	t.projectPath = path
	ws.activate(t)
	t.session.SetFilePath(path)
	return t.session
}

// New opens an empty drawing in a new tab.
func (h tabHost) New() bool {
	h.ws.activate(h.ws.newDocTab(doc.New()))
	return true
}

// Close closes this tab, unless it is the last one.
func (h tabHost) Close() bool {
	if len(h.ws.tabs) < 2 {
		return false
	}
	h.ws.removeTab(h.t)
	return true
}

// tabOf returns the tab that has the file path open, or nil.
func (ws *Workspace) tabOf(path string) *docTab {
	want := settings.CleanPath(path)
	if want == "" {
		return nil
	}
	for _, t := range ws.tabs {
		if p := settings.CleanPath(t.session.FilePath()); p != "" && sameFile(p, want) {
			return t
		}
	}
	return nil
}

// sameFile compares two cleaned paths, ignoring case where the file systems do by default.
func sameFile(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// activate makes t the active tab: the canvas, the command line, the status bar, the panels and
// the agent panel show its drawing. The command line's text is cleared; a command running in the
// tab left keeps its prompt there.
func (ws *Workspace) activate(t *docTab) {
	if t == ws.docTab {
		return
	}
	ws.closeTextEditor(ws.win)
	ws.status().hudDrag.active = false
	if cur := ws.docTab; cur != nil {
		c := &cur.canvas
		c.pressed, c.boxing, c.panning = false, false, false
		t.canvas.bounds, t.canvas.mouse, t.canvas.hover = c.bounds, c.mouse, c.hover
	}
	ws.docTab = t
	ws.input.SetText("")
	ws.cmd.open = false
	ws.dismissStart()
	if ws.agent != nil {
		ws.agent.follow(t)
	}
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// removeTab closes tab t without asking: its agent sessions stop and the tab to its right (else
// to its left) becomes active when t was.
func (ws *Workspace) removeTab(t *docTab) {
	i := ws.tabIndex(t)
	if i < 0 || len(ws.tabs) < 2 {
		return
	}
	if ws.agent != nil {
		ws.agent.closeDocument(t)
	}
	if t == ws.docTab {
		next := i + 1
		if next == len(ws.tabs) {
			next = i - 1
		}
		ws.activate(ws.tabs[next])
	}
	ws.tabs = append(ws.tabs[:i:i], ws.tabs[i+1:]...)
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// anyDirty reports whether a tab has unsaved changes.
func (ws *Workspace) anyDirty() bool {
	return slices.ContainsFunc(ws.tabs, func(t *docTab) bool { return t.session.Dirty() })
}

// tabIndex returns t's place in the strip, or -1.
func (ws *Workspace) tabIndex(t *docTab) int {
	for i, o := range ws.tabs {
		if o == t {
			return i
		}
	}
	return -1
}

// closeTab is the tab strip's close: a tab with unsaved changes, and the last tab, close through
// CLOSE in their own session, so the "Save changes" question is asked on the command line; any
// other tab closes at once.
func (ws *Workspace) closeTab(t *docTab, w *gpui.Window) {
	if t.session.Dirty() || len(ws.tabs) < 2 {
		ws.activate(t)
		ws.run("CLOSE", w)
		return
	}
	ws.removeTab(t)
	w.Refresh()
}

// cycleTab activates the next tab (step 1) or the previous one (step -1), wrapping.
func (ws *Workspace) cycleTab(step int) {
	n := len(ws.tabs)
	if n < 2 {
		return
	}
	ws.activate(ws.tabs[(ws.tabIndex(ws.docTab)+step+n)%n])
}

// DocumentTab describes one document tab, for the tab strip and tests.
type DocumentTab struct {
	// Name is the file name, or "Untitled".
	Name string
	// Path is the drawing's file, "" for an untitled drawing.
	Path   string
	Dirty  bool
	Active bool
}

// Documents returns the open drawings in tab order.
func (ws *Workspace) Documents() []DocumentTab {
	out := make([]DocumentTab, len(ws.tabs))
	for i, t := range ws.tabs {
		out[i] = DocumentTab{Name: t.name(), Path: t.session.FilePath(), Dirty: t.session.Dirty(), Active: t == ws.docTab}
	}
	return out
}

// DocumentSession returns the session of tab i, for scripting and tests.
func (ws *Workspace) DocumentSession(i int) *cmdsys.Session { return ws.tabs[i].session }

// SwitchDocument makes tab i active.
func (ws *Workspace) SwitchDocument(i int) {
	if i >= 0 && i < len(ws.tabs) {
		ws.activate(ws.tabs[i])
	}
}

// CloseDocument closes tab i as its close button does: one with unsaved changes asks first.
func (ws *Workspace) CloseDocument(i int, w *gpui.Window) {
	if i >= 0 && i < len(ws.tabs) {
		ws.closeTab(ws.tabs[i], w)
	}
}

// quitState is the quit hook and the progress of a quit that asks about several dirty tabs.
type quitState struct {
	f func()
	// next is the tab whose QUIT the workspace started to ask its question, runs that session's
	// run count right after: the same tab and count mean the question was answered, not a new QUIT.
	next  *docTab
	runs  int
	asked map[*docTab]bool
}

// SetQuitHandler installs the host's way of ending the application. QUIT and EXIT, in any tab,
// ask "Save changes" for that tab's drawing and then for every other dirty tab in turn, each
// shown while it asks; Cancel at any of them stops the quit.
func (ws *Workspace) SetQuitHandler(f func()) {
	ws.quit.f = f
	for _, t := range ws.tabs {
		t.session.SetQuitHandler(func() { ws.quitFrom(t) })
	}
}

// quitFrom continues a quit whose question tab t has just dealt with: the next dirty tab not yet
// asked runs QUIT, and when none is left the application ends.
func (ws *Workspace) quitFrom(t *docTab) {
	q := &ws.quit
	if q.next != t || t.session.RunCount() != q.runs || q.asked == nil {
		q.asked = map[*docTab]bool{}
	}
	q.asked[t] = true
	for _, o := range ws.tabs {
		if !q.asked[o] && o.session.Dirty() {
			ws.activate(o)
			o.session.Run("QUIT")
			q.next, q.runs = o, o.session.RunCount()
			return
		}
	}
	q.next, q.asked = nil, nil
	q.f()
}

// tabStrip is the row of document tabs between the ribbon and the canvas: one tab per drawing
// with its name, a dot while it has unsaved changes and a close button, then "+" for a new drawing.
// A middle click closes a tab; the tooltip has the file's path.
func (ws *Workspace) tabStrip() *gpui.Div {
	row := gpui.NewDiv().ID("doc-tabs").FlexRow().ItemsEnd().Gap(2).SizeH(docTabHeight).FlexShrink(0).Px_(8).Bg(uiBg).OverflowHidden()
	for i, t := range ws.tabs {
		row.Child(ws.docTabElement(i, t))
	}
	add := iconButton("doc-tab-new", ws.commandTip("NEW"), icons.AppAdd, false, func(w *gpui.Window) { ws.run("NEW", w) })
	add.SizeWH(26, 26).Mb(2)
	return row.Child(add)
}

// docTabElement is one tab of the strip.
func (ws *Workspace) docTabElement(i int, t *docTab) *gpui.Div {
	active := t == ws.docTab
	dirty := t.session.Dirty()
	tip := i18n.T("ui.tabs.untitled_tip")
	if p := t.session.FilePath(); p != "" {
		tip = p
	}
	if dirty {
		tip += "\n" + i18n.T("ui.status.modified_tip")
	}
	tip += "\n" + i18n.T("ui.tabs.switch_tip")
	label := gpui.NewDiv().ID(fmt.Sprintf("doc-tab-%d", i)).FlexRow().ItemsCenter().Gap(6).MinW(0).Tooltip(tip)
	if dirty {
		label.Child(dot(6, uiOn))
	}
	text := gpui.NewText(t.name()).TextSize(fontUI)
	if !active {
		text.TextColor(uiMuted)
	}
	label.Child(text)
	closeBtn := gpui.NewDiv().ID(fmt.Sprintf("doc-tab-close-%d", i)).FlexRow().ItemsCenter().JustifyCenter().SizeWH(18, 18).Rounded(4).FlexShrink(0).
		Tooltip(i18n.T("ui.tabs.close_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(e *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			if e.Up == nil || e.Up.Button != gpui.MouseRight {
				ws.closeTab(t, w)
			}
		}).
		Child(gpui.NewText("x").TextSize(fontSmall).TextColor(uiMuted))
	chip := gpui.NewDiv().ID(fmt.Sprintf("doc-tab-chip-%d", i)).FlexRow().ItemsCenter().Gap(6).Pl(12).Pr(4).SizeH(docTabHeight-4).MinW(0).
		RoundedAll(7, 7, 0, 0).Border(1, uiBorder).
		OnClick(func(e *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			switch {
			case e.Up != nil && e.Up.Button == gpui.MouseMiddle:
				ws.closeTab(t, w)
			case e.Up == nil || e.Up.Button == gpui.MouseLeft:
				ws.activate(t)
				w.Refresh()
			}
		}).
		Child(label, closeBtn)
	if active {
		chip.Bg(uiPanel)
	} else {
		chip.Bg(uiBg).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) })
	}
	return chip
}
