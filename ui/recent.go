package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Recent drawings: the Open menu (the chevron beside the title bar's Open button) and the start
// panel over an empty canvas. The lists themselves are settings.Recent, kept in the workspace's
// settings and saved with them.
const (
	// recentMenuWidth is the Open menu's width.
	recentMenuWidth = 380
	// startPanelWidth is the start panel's width.
	startPanelWidth = 420
	// startShown is how many recent drawings the start panel lists.
	startShown = 6
	// recentDirRunes caps a folder shown beside a name; longer ones keep their tail.
	recentDirRunes = 28
)

// recentState is the UI state of the recent lists.
type recentState struct {
	// menu shows the Open menu.
	menu bool
	// missing holds the recent files and folders that did not exist when the menu or the start
	// panel was last built; they are drawn greyed.
	missing map[string]bool
	// anchor is the Open button's bounds in the last frame, which places the menu under it.
	anchor gpui.Bounds
	// start is the start panel's state.
	start startState
}

// startState is the start panel's lifecycle: armed once at startup when the drawing is empty and
// untitled, done for good after the first action.
type startState struct {
	armed, done bool
	// runs is the session's run count when the panel was armed; any command started since
	// dismisses it.
	runs int
}

// noteRecentFile records a drawing the session opened or saved; it is the session's OnFilePath
// listener, so OPEN, SAVE, SAVEAS, QSAVE, the dialogs and a file opened at startup all pass here.
// NEW and CLOSE report "" and are ignored.
func (ws *Workspace) noteRecentFile(path string) {
	if path == "" {
		return
	}
	ws.prefs().cur.Recent.AddFile(path)
	ws.saveRecent()
}

// saveRecent writes the settings file after a change to the recent lists, when there is one.
func (ws *Workspace) saveRecent() {
	if ws.prefs().path != "" {
		ws.SaveSettings()
	}
}

// RecentFiles returns the recent drawings, newest first.
func (ws *Workspace) RecentFiles() []string {
	return append([]string(nil), ws.prefs().cur.Recent.Files...)
}

// RecentFolders returns the recent folders, newest first.
func (ws *Workspace) RecentFolders() []string {
	return append([]string(nil), ws.prefs().cur.Recent.Folders...)
}

// ClearRecent forgets the recent drawings and folders and saves the settings.
func (ws *Workspace) ClearRecent() {
	ws.prefs().cur.Recent.Clear()
	ws.saveRecent()
}

// statMissing returns the paths in lists that do not exist (or cannot be looked at).
func statMissing(lists ...[]string) map[string]bool {
	m := map[string]bool{}
	for _, l := range lists {
		for _, p := range l {
			if _, err := os.Stat(p); err != nil {
				m[p] = true
			}
		}
	}
	return m
}

// refreshMissing looks at the disk again for the recent entries.
func (ws *Workspace) refreshMissing() {
	r := ws.prefs().cur.Recent
	ws.recent.missing = statMissing(r.Files, r.Folders)
}

// openRecentFile opens a recent drawing the way OPEN does. A drawing that is gone fails as a typed
// OPEN would, with the error in the history, and leaves the list.
func (ws *Workspace) openRecentFile(path string, w *gpui.Window) {
	ws.recent.menu = false
	ws.dismissStart()
	ws.openFilePath(path, w)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		ws.prefs().cur.Recent.RemoveFile(path)
		ws.saveRecent()
	}
	w.Refresh()
}

// openRecentFolder shows the Open dialog starting in dir and moves dir to the front of the list. A
// folder that is gone leaves the list, and the dialog starts where it would without it.
func (ws *Workspace) openRecentFolder(dir string, w *gpui.Window) {
	ws.recent.menu = false
	ws.dismissStart()
	r := &ws.prefs().cur.Recent
	start := dir
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		r.RemoveFolder(dir)
		start = ""
	} else {
		r.AddFolder(dir)
	}
	ws.saveRecent()
	ws.startDialogIn(w, dialogOpen, start)
	w.Refresh()
}

// toggleOpenMenu shows or hides the Open menu, looking at the disk for missing entries when it
// opens.
func (ws *Workspace) toggleOpenMenu(w *gpui.Window) {
	ws.recent.menu = !ws.recent.menu
	if ws.recent.menu {
		ws.refreshMissing()
	}
	w.Refresh()
}

// OpenMenuShown reports whether the Open menu is showing.
func (ws *Workspace) OpenMenuShown() bool { return ws.recent.menu }

// openButtons is the title bar's Open button with the chevron that opens the recent menu beside it.
// A marker in front of the Open button records its position while painting, so the menu can hang
// under it.
func (ws *Workspace) openButtons() *gpui.Div {
	open := ws.fileButton("open", ws.openTip(), icons.FileOpen, ws.openFile, nil)
	// A zero-width marker just before the button reports where it is; being zero wide it never
	// takes a press or the hover from the button.
	mark := gpui.NewDiv().SizeWH(0, iconButtonSize).FlexShrink(0).Child(gpui.NewCanvas(func(b gpui.Bounds, _ *gpui.Window, _ *gpui.App) {
		ws.recent.anchor = b
	}))
	col := uiMuted
	chev := gpui.NewDiv().ID("tool-open-menu").FlexRow().ItemsCenter().JustifyCenter().SizeWH(16, iconButtonSize).Rounded(5).FlexShrink(0).
		Tooltip(i18n.T("ui.recent.menu_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.toggleOpenMenu(w) })
	if ws.recent.menu {
		col = uiOn
		chev.Bg(activeFill())
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().FlexShrink(0).Child(mark, open, chev.Child(NewIcon(icons.AppChevronDown, 12, col)))
}

// openMenuOverlay is the Open menu over the whole window: a transparent backdrop that closes the
// menu on any press outside it, and the menu itself under the Open button. Closed, it is an empty
// absolute box. It is mounted after the chrome, so it paints above the ribbon and the canvas.
func (ws *Workspace) openMenuOverlay() *gpui.Div {
	if !ws.recent.menu {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	a := ws.recent.anchor
	stop := func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }
	menu := gpui.NewDiv().ID("open-menu").Absolute().Left(float32(a.Origin.X)).Top(float32(a.Origin.Y+a.Size.H)+6).
		SizeW(recentMenuWidth).FlexCol().Gap(1).P(6).Rounded(8).Bg(uiPanelAlt).Border(1, uiBorder).
		OnMouseDown(stop).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	r := ws.prefs().cur.Recent
	menu.Child(menuHeading(i18n.T("ui.recent.files")))
	if len(r.Files) == 0 {
		menu.Child(gpui.NewText(i18n.T("ui.recent.none")).TextSize(fontUI).TextColor(uiMuted).Px_(8).Py_(4))
	}
	for i, f := range r.Files {
		f := f
		menu.Child(ws.recentFileRow("recent-file-"+strconv.Itoa(i), f, func(w *gpui.Window) { ws.openRecentFile(f, w) }))
	}
	if len(r.Folders) > 0 {
		menu.Child(gpui.NewDiv().SizeH(1).Bg(uiBorder).My(4), menuHeading(i18n.T("ui.recent.folders")))
		for i, d := range r.Folders {
			d := d
			menu.Child(ws.recentFolderRow("recent-folder-"+strconv.Itoa(i), d, func(w *gpui.Window) { ws.openRecentFolder(d, w) }))
		}
	}
	if !r.Empty() {
		menu.Child(gpui.NewDiv().SizeH(1).Bg(uiBorder).My(4),
			menuRow("recent-clear", i18n.T("ui.recent.clear_tip"), func(w *gpui.Window) {
				ws.recent.menu = false
				ws.ClearRecent()
				w.Refresh()
			}).Child(gpui.NewText(i18n.T("ui.recent.clear")).TextSize(fontUI).TextColor(uiText)))
	}
	return gpui.NewDiv().ID("open-menu-backdrop").Absolute().Inset(0).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			w.StopPropagation()
			ws.recent.menu = false
			w.Refresh()
		}).
		Child(menu)
}

// menuHeading is a small muted caption over a group of menu rows.
func menuHeading(s string) *gpui.Text {
	return gpui.NewText(s).TextSize(fontSmall - 1).TextColor(uiMuted).Px_(8).Pt(4).Pb(2)
}

// menuRow is a clickable menu row with a hover fill and a tooltip.
func menuRow(id, tip string, act func(*gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).FlexRow().ItemsCenter().Gap(8).Px_(8).Py_(4).Rounded(5).MinW(0).OverflowHidden().
		Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { act(w) })
}

// recentFileRow is one recent drawing: its format, its name and its folder; greyed when the file was
// not found.
func (ws *Workspace) recentFileRow(id, path string, act func(*gpui.Window)) *gpui.Div {
	missing := ws.recent.missing[path]
	tip := i18n.T("ui.recent.file_tip", path)
	name, dirCol := uiText, uiMuted
	if missing {
		tip = i18n.T("ui.recent.missing_tip", path)
		name = uiMuted
	}
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
	row := menuRow(id, tip, act).Child(
		gpui.NewDiv().SizeW(30).FlexShrink(0).Child(monoText(ext, fontSmall-1).TextColor(uiMuted)),
		gpui.NewText(filepath.Base(path)).TextSize(fontUI).TextColor(name).FlexShrink(0),
		gpui.NewDiv().Flex1().MinW(0).OverflowHidden().FlexRow().JustifyEnd().
			Child(gpui.NewText(displayDir(filepath.Dir(path))).TextSize(fontSmall).TextColor(dirCol).FlexShrink(0)),
	)
	if missing {
		row.Opacity(0.55)
	}
	return row
}

// recentFolderRow is one recent folder: a folder icon, its name and its parent; greyed when it was
// not found.
func (ws *Workspace) recentFolderRow(id, dir string, act func(*gpui.Window)) *gpui.Div {
	missing := ws.recent.missing[dir]
	tip := i18n.T("ui.recent.folder_tip", dir)
	if missing {
		tip = i18n.T("ui.recent.missing_tip", dir)
	}
	row := menuRow(id, tip, act).Child(
		gpui.NewDiv().SizeW(30).FlexShrink(0).Child(NewIcon(icons.FileOpen, 16, uiMuted)),
		gpui.NewText(filepath.Base(dir)).TextSize(fontUI).TextColor(uiText).FlexShrink(0),
		gpui.NewDiv().Flex1().MinW(0).OverflowHidden().FlexRow().JustifyEnd().
			Child(gpui.NewText(displayDir(filepath.Dir(dir))).TextSize(fontSmall).TextColor(uiMuted).FlexShrink(0)),
	)
	if missing {
		row.Opacity(0.55)
	}
	return row
}

// displayDir shortens a folder for a menu row: the home folder becomes "~", and a long path keeps
// its last recentDirRunes characters after an ellipsis.
func displayDir(dir string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			dir = "~"
		} else if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
			dir = "~" + string(filepath.Separator) + rest
		}
	}
	if r := []rune(dir); len(r) > recentDirRunes {
		dir = "…" + string(r[len(r)-recentDirRunes+1:])
	}
	return dir
}

// armStart decides, once the settings are loaded, whether the start panel shows: only over an
// empty, untitled drawing, and only when there are recent drawings to list.
func (ws *Workspace) armStart() {
	s := &ws.recent.start
	if s.done {
		return
	}
	s.armed = ws.session.FilePath() == "" && ws.session.Drawing().Len() == 0 && len(ws.prefs().cur.Recent.Files) > 0
	s.runs = ws.session.RunCount()
	if s.armed {
		ws.refreshMissing()
	}
}

// dismissStart hides the start panel for the rest of the session.
func (ws *Workspace) dismissStart() {
	if ws.recent.start.armed {
		ws.recent.start.done = true
	}
}

// StartPanelShown reports whether the start panel is showing. It goes away for good on the first
// action: a command, a click on the canvas, typing, Escape, opening a file, or a drawing that is no
// longer empty.
func (ws *Workspace) StartPanelShown() bool {
	s := &ws.recent.start
	if !s.armed || s.done {
		return false
	}
	if ws.session.FilePath() != "" || ws.session.Drawing().Len() > 0 || ws.session.RunCount() != s.runs ||
		len(ws.prefs().cur.Recent.Files) == 0 {
		s.done = true
		return false
	}
	return true
}

// startPanel is the card over an empty canvas that lists the recent drawings, with an Open button
// for any other file. Presses on it do not reach the canvas. When it is not shown it is an empty box.
func (ws *Workspace) startPanel() *gpui.Div {
	if !ws.StartPanelShown() {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	b := ws.canvas.bounds
	left, top := float32(overlayInset+4), float32(overlayInset+32)
	if cw := float32(b.Size.W); cw > 0 {
		left = max(left, (cw-startPanelWidth)/2)
		top = max(top, float32(b.Size.H)*0.18)
	}
	card := gpui.NewDiv().ID("start-panel").Absolute().Left(left).Top(top).SizeW(startPanelWidth).FlexCol().Gap(1).P(10).
		Rounded(10).Bg(uiPanel).Border(1, uiBorder).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(
			gpui.NewText(i18n.T("ui.recent.start_title")).TextSize(fontTitle).TextColor(uiText).Px_(8).Pt(2),
			gpui.NewText(i18n.T("ui.recent.start_hint")).TextSize(fontSmall).TextColor(uiMuted).Px_(8).Pb(6),
		)
	for i, f := range ws.prefs().cur.Recent.Files {
		if i == startShown {
			break
		}
		f := f
		card.Child(ws.recentFileRow("start-file-"+strconv.Itoa(i), f, func(w *gpui.Window) { ws.openRecentFile(f, w) }))
	}
	card.Child(gpui.NewDiv().SizeH(1).Bg(uiBorder).My(4),
		menuRow("start-open", tb("open_tip"), func(w *gpui.Window) {
			ws.dismissStart()
			ws.openFile(w)
		}).Child(
			gpui.NewDiv().SizeW(30).FlexShrink(0).Child(NewIcon(icons.FileOpen, 16, uiMuted)),
			gpui.NewText(i18n.T("ui.recent.start_open")).TextSize(fontUI).TextColor(uiText),
		))
	return card
}
