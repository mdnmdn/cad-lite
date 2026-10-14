package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/filedialog"
	"cad-lite/fileio"
	"cad-lite/fileio/dwg"
	"cad-lite/fileio/dxf"
	"cad-lite/internal/gpui"
)

// fakeDialog answers every dialog with one path and error, and records what it was asked.
type fakeDialog struct {
	mu          sync.Mutex
	path        string
	err         error
	opens       int
	saves       int
	title       string
	startDir    string
	defaultName string
	filters     []filedialog.Filter
}

func (f *fakeDialog) Open(_ context.Context, title, startDir string, filters []filedialog.Filter) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	f.title, f.startDir, f.filters = title, startDir, filters
	return f.path, f.err
}

func (f *fakeDialog) Save(_ context.Context, title, startDir, defaultName string, filters []filedialog.Filter) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	f.title, f.startDir, f.defaultName, f.filters = title, startDir, defaultName, filters
	return f.path, f.err
}

func (f *fakeDialog) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens + f.saves
}

func dxfRegistry() *fileio.Registry {
	r := &fileio.Registry{}
	r.AddReader(dxf.Codec{})
	r.AddWriter(dxf.Codec{})
	return r
}

// dialogRig is a workspace with the real DXF registry and a fake dialog.
func dialogRig(t *testing.T, fd *fakeDialog) *sHarness {
	t.Helper()
	ws := NewWorkspace(doc.New(), dxfRegistry())
	if fd != nil {
		ws.SetFileDialog(fd)
	}
	return openSettingsWS(t, ws, 1000, 700, 1)
}

// savedDrawing writes a one-line DXF into a temporary folder and returns its path.
func savedDrawing(t *testing.T) string {
	t.Helper()
	d := doc.New()
	ws := NewWorkspace(d, dxfRegistry())
	ws.Session().Script("LINE 0,0 10,10  ")
	path := filepath.Join(t.TempDir(), "plan.dxf")
	if _, err := dxfRegistry().Save(path, ws.Session().Drawing()); err != nil {
		t.Fatal(err)
	}
	return path
}

func (h *sHarness) rightClick(id string) {
	h.t.Helper()
	p := h.find(id)
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseRight, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseRight, ClickCount: 1})
}

// answered waits for the dialog goroutine's answer to reach the UI.
func (h *sHarness) answered(fd *fakeDialog) {
	h.t.Helper()
	h.settle(func() bool { return fd.calls() > 0 && !h.ws.dlg.busy })
}

func TestOpenButtonOpensTheChosenFile(t *testing.T) {
	fd := &fakeDialog{path: savedDrawing(t)}
	h := dialogRig(t, fd)
	h.clickID("tool-open")
	h.answered(fd)
	if got := h.ws.Session().FilePath(); got != fd.path {
		t.Fatalf("file path %q, want %q", got, fd.path)
	}
	if n := h.ws.Session().Drawing().Len(); n != 1 {
		t.Fatalf("drawing has %d entities, want the saved line", n)
	}
	if h.ws.Session().Active() != "" {
		t.Fatalf("OPEN still running: %q", h.ws.Session().Active())
	}
	var exts []string
	var names []string
	for _, f := range fd.filters {
		names = append(names, f.Name)
		exts = append(exts, f.Extensions...)
	}
	if !slices.Contains(exts, "dxf") || names[0] != "All supported files" || names[len(names)-1] != "All files" {
		t.Fatalf("open filters %q (extensions %q)", names, exts)
	}
}

func TestOpenCancelledDoesNothing(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h := dialogRig(t, fd)
	before := len(h.ws.Session().History())
	h.clickID("tool-open")
	h.answered(fd)
	s := h.ws.Session()
	if s.Active() != "" || s.FilePath() != "" || len(s.History()) != before {
		t.Fatalf("cancel changed state: active %q path %q history %q", s.Active(), s.FilePath(), s.History())
	}
}

func TestOpenWithoutDialogBackendFallsBackToCommandLine(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrUnavailable}
	h := dialogRig(t, fd)
	h.clickID("tool-open")
	h.answered(fd)
	s := h.ws.Session()
	p := s.CurrentPrompt()
	if s.Active() != "OPEN" || p == nil || p.Accepts&cmdsys.InputText == 0 {
		t.Fatalf("want the OPEN path prompt, active %q prompt %+v", s.Active(), p)
	}
	// And a typed path works as it always did.
	path := savedDrawing(t)
	h.ws.submit(path, h.hw.Window())
	if s.FilePath() != path {
		t.Fatalf("typed path not opened: %q", s.FilePath())
	}
}

func TestToolbarWithoutDialogRunsTheCommands(t *testing.T) {
	h := dialogRig(t, nil)
	h.clickID("tool-open")
	if h.ws.Session().Active() != "OPEN" {
		t.Fatalf("Open without a dialog should prompt on the command line, active %q", h.ws.Session().Active())
	}
	h.ws.Session().Escape()
	h.clickID("tool-save")
	if h.ws.Session().Active() != "QSAVE" {
		t.Fatalf("Save of an unsaved drawing without a dialog should prompt for a path, active %q", h.ws.Session().Active())
	}
}

func TestOpenBesideADirtyDrawingKeepsIt(t *testing.T) {
	fd := &fakeDialog{path: savedDrawing(t)}
	h := dialogRig(t, fd)
	h.ws.Session().Script("LINE 0,0 5,5  ")
	h.frame() // the unsaved-changes dot moves the buttons; find them where they are now
	h.clickID("tool-open")
	h.answered(fd)
	docs := h.ws.Documents()
	if len(docs) != 2 || !docs[1].Active || docs[1].Path != fd.path {
		t.Fatalf("the file should open in a second tab: %+v", docs)
	}
	first := h.ws.DocumentSession(0)
	if !first.Dirty() || first.Drawing().Len() != 1 || first.Active() != "" {
		t.Fatalf("the dirty drawing changed: dirty %v, %d entities, active %q", first.Dirty(), first.Drawing().Len(), first.Active())
	}
}

func TestCtrlOOpensTheDialog(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h := dialogRig(t, fd)
	h.ctrl("o")
	h.answered(fd)
	if fd.opens != 1 {
		t.Fatalf("Ctrl+O showed %d open dialogs", fd.opens)
	}
}

func TestSaveButton(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out.dxf")
	fd := &fakeDialog{path: target}
	h := dialogRig(t, fd)
	h.ws.Session().Script("LINE 0,0 10,10  ")
	h.frame() // the unsaved-changes dot moves the buttons; find them where they are now
	h.clickID("tool-save")
	h.answered(fd)
	if fd.saves != 1 || fd.defaultName != "drawing" {
		t.Fatalf("saves %d default name %q", fd.saves, fd.defaultName)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("the drawing was not saved: %v", err)
	}
	if h.ws.Session().FilePath() != target || h.ws.Session().Dirty() {
		t.Fatalf("path %q dirty %v", h.ws.Session().FilePath(), h.ws.Session().Dirty())
	}
	var got []string
	for _, f := range fd.filters {
		got = append(got, f.Extensions...)
	}
	if !slices.Equal(got, []string{"dxf"}) {
		t.Fatalf("save filters list %q, want only what can be written", got)
	}

	// With a file the button saves to it and shows no dialog.
	h.ws.Session().Script("LINE 1,1 2,2  ")
	h.frame()
	h.clickID("tool-save")
	if fd.calls() != 1 || h.ws.Session().Dirty() {
		t.Fatalf("Save with a file: dialog calls %d, dirty %v", fd.calls(), h.ws.Session().Dirty())
	}
}

func TestSaveRightClickAsksForAName(t *testing.T) {
	target := filepath.Join(t.TempDir(), "copy.dxf")
	fd := &fakeDialog{path: target}
	h := dialogRig(t, fd)
	path := savedDrawing(t)
	h.ws.Session().SetFilePath(path)
	h.rightClick("tool-save")
	h.answered(fd)
	if fd.saves != 1 || fd.defaultName != "plan.dxf" {
		t.Fatalf("saves %d default name %q", fd.saves, fd.defaultName)
	}
	if h.ws.Session().FilePath() != target {
		t.Fatalf("file path %q, want %q", h.ws.Session().FilePath(), target)
	}
}

func TestSaveWithoutBackendFallsBack(t *testing.T) {
	fd := &fakeDialog{err: errors.New("zenity exploded")}
	h := dialogRig(t, fd)
	h.clickID("tool-save")
	h.answered(fd)
	if h.ws.Session().Active() != "QSAVE" {
		t.Fatalf("a failing dialog should fall back to the path prompt, active %q", h.ws.Session().Active())
	}
}

func TestDialogFilters(t *testing.T) {
	r := dxfRegistry()
	open := openFilters(r.ReadExtensions())
	if open[0].Name != "All supported files" || !slices.Equal(open[0].Extensions, r.ReadExtensions()) {
		t.Fatalf("first open filter %+v", open[0])
	}
	if last := open[len(open)-1]; last.Name != "All files" || len(last.Extensions) != 0 {
		t.Fatalf("last open filter %+v", last)
	}
	if open[1].Name != "DXF drawing" {
		t.Fatalf("per-format filter %+v", open[1])
	}
	save := saveFilters(r.WriteExtensions())
	if len(save) != 1 || save[0].Name != "DXF drawing" || !slices.Equal(save[0].Extensions, []string{"dxf"}) {
		t.Fatalf("save filters %+v", save)
	}
	// A format registered later (the DWG reader) shows up without any change here.
	more := openFilters([]string{".dwg", ".dxf"})
	if !slices.Equal(more[0].Extensions, []string{".dwg", ".dxf"}) || more[1].Name != "DWG drawing" {
		t.Fatalf("more filters %+v", more)
	}
	if got := saveFilters(nil); len(got) != 1 || got[0].Name != "All files" {
		t.Fatalf("no writers: %+v", got)
	}
}

// A drawing read from a DWG cannot be saved back to it: Save asks where, offering the name as .dxf.
func TestSaveAfterDWGShowsTheDialogWithADXFName(t *testing.T) {
	r := dxfRegistry()
	r.AddReader(dwg.Codec{})
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	ws := NewWorkspace(doc.New(), r)
	ws.SetFileDialog(fd)
	h := openSettingsWS(t, ws, 1000, 700, 1)
	src, err := filepath.Abs(filepath.Join("..", "fileio", "dwg", "testdata", "sample_AC1015.dwg"))
	if err != nil {
		t.Fatal(err)
	}
	h.ws.Session().Script("OPEN " + src)
	if h.ws.Session().FilePath() != src {
		t.Fatalf("OPEN did not load the DWG: path %q", h.ws.Session().FilePath())
	}
	h.hw.Window().Refresh() // the document chip took the file's name, which moves the buttons after it
	h.hw.Pixels()
	h.clickID("tool-save")
	h.answered(fd)
	if fd.saves != 1 || fd.defaultName != "sample_AC1015.dxf" {
		t.Fatalf("saves %d default name %q, want the save dialog offering sample_AC1015.dxf", fd.saves, fd.defaultName)
	}
}
