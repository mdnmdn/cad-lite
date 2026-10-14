package ui

import (
	"path/filepath"
	"strings"

	"cad-lite/internal/gpui"
)

// OpenPaths opens files the operating system hands to the application: dropped on the window,
// opened from Finder or the Dock (macOS), or named on the command line. It is their one entry
// point. Each path a reader supports goes through OPEN, as a path typed at its prompt does, so it
// opens in a tab of its own (the untouched untitled drawing is replaced by the first), or shows the
// tab that already has it; each unsupported path is reported in the command history.
func (ws *Workspace) OpenPaths(paths []string) {
	for _, p := range paths {
		if !ws.canRead(p) {
			ws.session.Say("ui.file.unsupported", filepath.Base(p), ws.readFormats())
			continue
		}
		ws.openPath(p)
		if ws.win == nil {
			// Before the first frame (a launch request) the canvas size is not known yet.
			ws.fitPending = true
		}
	}
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// openPath runs OPEN with path in the active tab, which opens it in the right tab.
func (ws *Workspace) openPath(path string) {
	ws.input.SetText("")
	t := ws.docTab
	n := t.session.RunCount()
	t.session.Run("OPEN")
	ws.noteIfRan(t, n)
	t.session.Submit(path)
	t.builder.InvalidateAll()
}

// onFileDrop is the root view's drop listener: files dropped anywhere on the window are opened.
func (ws *Workspace) onFileDrop(e *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) {
	ws.win = w
	ws.OpenPaths(e.Paths)
}

// canRead reports whether the file handler reads path's format. Without a handler, or one that
// cannot tell, the path goes to OPEN, which reports what is wrong.
func (ws *Workspace) canRead(path string) bool {
	if ws.files == nil {
		return true
	}
	if c, ok := ws.files.(interface{ CanRead(string) bool }); ok {
		return c.CanRead(path)
	}
	return true
}

// readFormats lists the formats the file handler reads for a message, as "DWG, DXF".
func (ws *Workspace) readFormats() string {
	l, ok := ws.files.(extensionLister)
	if !ok {
		return ""
	}
	exts := l.ReadExtensions()
	names := make([]string, len(exts))
	for i, e := range exts {
		names[i] = strings.ToUpper(strings.TrimPrefix(e, "."))
	}
	return strings.Join(names, ", ")
}
