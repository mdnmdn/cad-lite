package ui

import (
	"path/filepath"
	"strings"

	"cad-lite/internal/gpui"
)

// OpenPaths opens files the operating system hands to the application: dropped on the window,
// opened from Finder or the Dock (macOS), or named on the command line. It is their one entry
// point. The first path a reader supports goes through OPEN, as a path typed at its prompt does,
// so a drawing with unsaved changes asks first; each unsupported path, and each further drawing,
// is reported in the command history. With document tabs (backlog N1) this is the function that
// opens every supported path in a tab of its own.
func (ws *Workspace) OpenPaths(paths []string) {
	var first string
	for _, p := range paths {
		switch {
		case !ws.canRead(p):
			ws.session.Say("ui.file.unsupported", filepath.Base(p), ws.readFormats())
		case first == "":
			first = p
		default:
			ws.session.Say("ui.file.not_opened", filepath.Base(p))
		}
	}
	if first != "" {
		ws.input.SetText("")
		n := ws.session.RunCount()
		ws.session.Run("OPEN")
		ws.noteIfRan(n)
		ws.session.Submit(first)
		ws.builder.InvalidateAll()
		if ws.win == nil {
			// Before the first frame (a launch request) the canvas size is not known yet.
			ws.fitPending = true
		}
	}
	if ws.win != nil {
		ws.win.Refresh()
	}
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
