package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cad-lite/assets/icons"
	"cad-lite/cmdsys"
	"cad-lite/filedialog"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// dialogPoll is how often the UI looks at the dialog goroutine while a file dialog is open.
const dialogPoll = 100 * time.Millisecond

// FileDialog shows the native open and save dialogs. filedialog.System is the real one; tests
// install a fake. Both calls block until the user answers, so the workspace runs them on a
// goroutine, and they must not touch the UI.
type FileDialog interface {
	Open(ctx context.Context, title, startDir string, filters []filedialog.Filter) (string, error)
	Save(ctx context.Context, title, startDir, defaultName string, filters []filedialog.Filter) (string, error)
}

// extensionLister is what a file handler offers when it knows which formats it reads and writes
// (fileio.Registry does); it decides the dialogs' file-type lists.
type extensionLister interface {
	ReadExtensions() []string
	WriteExtensions() []string
}

// dialogKind is which toolbar or keyboard action asked for a dialog.
type dialogKind int

const (
	dialogOpen   dialogKind = iota // Open button, Ctrl+O: then OPEN with the chosen path
	dialogSave                     // Save with no file yet: then SAVEAS with the chosen path
	dialogSaveAs                   // Save as, Ctrl+Shift+S: then SAVEAS with the chosen path
)

// command is the file command a dialog answer feeds, and the one that runs on the command line
// when no dialog can be shown.
func (k dialogKind) command(fallback bool) string {
	switch {
	case k == dialogOpen:
		return "OPEN"
	case fallback && k == dialogSave:
		return "QSAVE"
	}
	return "SAVEAS"
}

type dialogResult struct {
	kind dialogKind
	path string
	err  error
}

// dialogState is the workspace's file dialog state. dialog, busy and poll belong to the UI
// goroutine; the dialog goroutine writes only res, under mu, and the poll timer moves it across.
type dialogState struct {
	dialog FileDialog
	busy   bool
	poll   *gpui.Timer

	mu  sync.Mutex
	res *dialogResult
}

// SetFileDialog installs the dialog the Open and Save buttons use. Without one (the default,
// which -png renders and tests keep) they run OPEN and QSAVE on the command line instead.
func (ws *Workspace) SetFileDialog(d FileDialog) { ws.dlg.dialog = d }

// openFile is the Open button and Ctrl+O.
func (ws *Workspace) openFile(w *gpui.Window) { ws.startDialog(w, dialogOpen) }

// saveFile is the Save button and Ctrl+S: a drawing with a writable file is saved to it; one
// without a file, or read from a format Cad-lite only reads (DWG), asks where.
func (ws *Workspace) saveFile(w *gpui.Window) {
	if p := ws.session.FilePath(); p != "" && ws.canWrite(p) {
		ws.run("QSAVE", w)
		return
	}
	ws.startDialog(w, dialogSave)
}

// saveFileAs is Save as (right-click on Save) and Ctrl+Shift+S.
func (ws *Workspace) saveFileAs(w *gpui.Window) { ws.startDialog(w, dialogSaveAs) }

// startDialog shows the dialog for kind on a goroutine, or runs the command line fallback at once
// when there is no dialog or no file handler.
func (ws *Workspace) startDialog(w *gpui.Window, kind dialogKind) { ws.startDialogIn(w, kind, "") }

// dialogDir is the folder a dialog starts in: the drawing's, else the most recent folder, else the
// working directory.
func (ws *Workspace) dialogDir() string {
	if ws.session.FilePath() == "" {
		if f := ws.prefs().cur.Recent.Folders; len(f) > 0 {
			return f[0]
		}
	}
	return ws.projectDir()
}

// startDialogIn is startDialog with the folder the dialog starts in; "" means dialogDir.
func (ws *Workspace) startDialogIn(w *gpui.Window, kind dialogKind, dir string) {
	d := &ws.dlg
	if d.dialog == nil || ws.files == nil {
		ws.run(kind.command(true), w)
		return
	}
	if d.busy {
		return
	}
	d.busy = true
	dialog := d.dialog
	if dir == "" {
		dir = ws.dialogDir()
	}
	var read, write []string
	if l, ok := ws.files.(extensionLister); ok {
		read, write = l.ReadExtensions(), l.WriteExtensions()
	}
	name := i18n.T("ui.file.default_name")
	if p := ws.session.FilePath(); p != "" {
		name = filepath.Base(ws.writablePath(p))
	}
	go func() {
		var res dialogResult
		res.kind = kind
		if kind == dialogOpen {
			res.path, res.err = dialog.Open(context.Background(), i18n.T("ui.file.open_title"), dir, openFilters(read))
		} else {
			res.path, res.err = dialog.Save(context.Background(), i18n.T("ui.file.save_title"), dir, name, saveFilters(write))
		}
		d.mu.Lock()
		d.res = &res
		d.mu.Unlock()
	}()
	ws.armDialogPoll(w)
}

func (ws *Workspace) armDialogPoll(w *gpui.Window) {
	d := &ws.dlg
	d.poll.Cancel()
	d.poll = w.After(dialogPoll, func(w *gpui.Window, _ *gpui.App) { ws.pollDialog(w) })
}

// pollDialog is the poll timer body, on the UI goroutine: it adopts the dialog's answer, or keeps
// waiting.
func (ws *Workspace) pollDialog(w *gpui.Window) {
	d := &ws.dlg
	d.mu.Lock()
	res := d.res
	d.res = nil
	d.mu.Unlock()
	if res == nil {
		ws.armDialogPoll(w)
		return
	}
	d.busy = false
	switch {
	case res.err == nil && res.kind == dialogOpen:
		ws.openFilePath(res.path, w)
	case res.err == nil:
		// The same command a typed SAVEAS is: the unsaved-changes question and the project switch
		// happen in it.
		ws.run(res.kind.command(false), w)
		ws.submit(res.path, w)
	case errors.Is(res.err, filedialog.ErrCancelled):
	default:
		// No backend or a failing one: the user types the path as without a dialog.
		ws.run(res.kind.command(true), w)
	}
	w.Refresh()
}

// openFilePath opens the drawing at path as a typed OPEN does: the unsaved-changes question, the
// undo reset, the project switch and the recent list all happen in it. Every "open this file" in
// the UI (the Open dialog, the recent menu, the start panel) comes through here.
func (ws *Workspace) openFilePath(path string, w *gpui.Window) {
	ws.run("OPEN", w)
	if p := ws.session.CurrentPrompt(); ws.session.Active() == "OPEN" && p != nil && p.Accepts&cmdsys.InputText != 0 {
		ws.submit(path, w)
	}
}

// openFilters is the Open dialog's file-type list: every extension a reader reads together, one
// entry per extension, then every file.
func openFilters(read []string) []filedialog.Filter {
	var fs []filedialog.Filter
	if len(read) > 0 {
		fs = append(fs, filedialog.Filter{Name: i18n.T("ui.file.filter_all_supported"), Extensions: read})
		fs = append(fs, extFilters(read)...)
	}
	return append(fs, filedialog.Filter{Name: i18n.T("ui.file.filter_all")})
}

// saveFilters is the Save dialog's list: one entry per extension a writer writes. The first is the
// one a name typed without an extension gets.
func saveFilters(write []string) []filedialog.Filter {
	fs := extFilters(write)
	if len(fs) == 0 {
		fs = []filedialog.Filter{{Name: i18n.T("ui.file.filter_all")}}
	}
	return fs
}

func extFilters(exts []string) []filedialog.Filter {
	fs := make([]filedialog.Filter, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimPrefix(e, ".")
		fs = append(fs, filedialog.Filter{Name: i18n.T("ui.file.filter_ext", strings.ToUpper(e)), Extensions: []string{e}})
	}
	return fs
}

// fileButton is an icon-only title bar button for a file action, with its own tooltip. alt, when
// set, runs on a right click.
func (ws *Workspace) fileButton(id, tip string, icon icons.Name, act, alt func(*gpui.Window)) *gpui.Div {
	b := iconButton("tool-"+id, tip, icon, false, act)
	if alt != nil {
		b.OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if e.IsRight() {
				alt(w)
			}
		})
	}
	return b
}

// canWrite reports whether the file handler can save to path; a handler that cannot tell is
// trusted.
func (ws *Workspace) canWrite(path string) bool {
	if c, ok := ws.files.(interface{ CanWrite(string) bool }); ok {
		return c.CanWrite(path)
	}
	return true
}

// writablePath is path with an extension the file handler can save to (a.dwg gives a.dxf).
func (ws *Workspace) writablePath(path string) string {
	if c, ok := ws.files.(interface{ WritablePath(string) string }); ok {
		return c.WritablePath(path)
	}
	return path
}
