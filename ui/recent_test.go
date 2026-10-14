package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/filedialog"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// recentWS is a workspace with the DXF registry, an optional fake dialog and a settings file whose
// recent list holds files (newest first). The settings are loaded before the window opens, as
// cmd/cad does.
func recentWS(t *testing.T, fd *fakeDialog, files ...string) (*Workspace, string) {
	t.Helper()
	ws := NewWorkspace(doc.New(), dxfRegistry())
	if fd != nil {
		ws.SetFileDialog(fd)
	}
	path := filepath.Join(t.TempDir(), "settings.toml")
	s := settings.Default()
	for i := len(files) - 1; i >= 0; i-- {
		s.Recent.AddFile(files[i])
	}
	if err := settings.Save(path, s); err != nil {
		t.Fatal(err)
	}
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	return ws, path
}

func recentRig(t *testing.T, fd *fakeDialog, files ...string) (*sHarness, string) {
	t.Helper()
	ws, path := recentWS(t, fd, files...)
	return openSettingsWS(t, ws, 1000, 700, 1), path
}

// savedRecent reads the recent files back from the settings file.
func savedRecent(t *testing.T, path string) []string {
	t.Helper()
	s, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s.Recent.Files
}

func TestRecentRecordsOpenAndSaveAndPersists(t *testing.T) {
	ws, path := recentWS(t, nil)
	a := savedDrawing(t)
	s := ws.Session()
	s.Script("OPEN " + a + "\n")
	b := filepath.Join(t.TempDir(), "copy.dxf")
	s.Script("SAVEAS " + b + "\n")
	want := []string{b, a}
	if got := ws.RecentFiles(); !slices.Equal(got, want) {
		t.Fatalf("recent files %v, want %v", got, want)
	}
	if got := ws.RecentFolders(); !slices.Equal(got, []string{filepath.Dir(b), filepath.Dir(a)}) {
		t.Fatalf("recent folders %v", got)
	}
	if got := savedRecent(t, path); !slices.Equal(got, want) {
		t.Fatalf("settings file holds %v, want %v", got, want)
	}
	s.Script("NEW\n")
	if got := ws.RecentFiles(); !slices.Equal(got, want) {
		t.Fatalf("NEW changed the recent files: %v", got)
	}
}

func TestRecentRecordsFileOpenedBeforeSettings(t *testing.T) {
	a := savedDrawing(t)
	ws := NewWorkspace(doc.New(), dxfRegistry())
	ws.SetProjectPath(a) // what cmd/cad does for -open, before LoadSettings
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	if got := savedRecent(t, path); !slices.Equal(got, []string{a}) {
		t.Fatalf("settings file holds %v, want the -open file", got)
	}
	if ws.StartPanelShown() {
		t.Fatal("start panel shown over a drawing with a file")
	}
}

func TestOpenDialogRecordsRecent(t *testing.T) {
	fd := &fakeDialog{path: savedDrawing(t)}
	h, _ := recentRig(t, fd)
	h.clickID("tool-open")
	h.answered(fd)
	if got := h.ws.RecentFiles(); !slices.Equal(got, []string{fd.path}) {
		t.Fatalf("recent files %v", got)
	}
}

func TestOpenMenuOpensRecentFile(t *testing.T) {
	a, b := savedDrawing(t), savedDrawing(t)
	h, _ := recentRig(t, nil, a, b)
	h.clickID("tool-open-menu")
	h.frame()
	if !h.ws.OpenMenuShown() || !h.has("open-menu") || !h.has("recent-file-1") || !h.has("recent-folder-0") {
		t.Fatal("the Open menu does not list the recent files and folders")
	}
	h.clickID("recent-file-1")
	h.frame()
	if got := h.ws.Session().FilePath(); got != b {
		t.Fatalf("opened %q, want %q", got, b)
	}
	if h.ws.OpenMenuShown() || h.has("open-menu") {
		t.Fatal("menu still open after choosing a file")
	}
	if got := h.ws.RecentFiles(); !slices.Equal(got, []string{b, a}) {
		t.Fatalf("recent files %v, want the opened one first", got)
	}
}

func TestOpenMenuOpensBesideADirtyDrawing(t *testing.T) {
	a := savedDrawing(t)
	h, _ := recentRig(t, nil, a)
	h.ws.Session().Script("LINE 0,0 5,5  ")
	h.frame()
	h.clickID("tool-open-menu")
	h.frame()
	h.clickID("recent-file-0")
	if h.ws.Session().FilePath() != a || len(h.ws.Documents()) != 2 {
		t.Fatalf("the file should open in a tab of its own: %+v", h.ws.Documents())
	}
	if s := h.ws.DocumentSession(0); !s.Dirty() || s.Active() != "" {
		t.Fatalf("the dirty drawing was touched: dirty %v, active %q", s.Dirty(), s.Active())
	}
	// Opening it again shows its tab instead of a third one.
	h.ws.SwitchDocument(0)
	h.frame()
	h.clickID("tool-open-menu")
	h.frame()
	h.clickID("recent-file-0")
	if docs := h.ws.Documents(); len(docs) != 2 || !docs[1].Active {
		t.Fatalf("reopening an open file should switch to its tab: %+v", docs)
	}
}

func TestOpenMenuFolderStartsTheDialogThere(t *testing.T) {
	a := savedDrawing(t)
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h, _ := recentRig(t, fd, a)
	h.clickID("tool-open-menu")
	h.frame()
	h.clickID("recent-folder-0")
	h.answered(fd)
	if fd.opens != 1 || fd.startDir != filepath.Dir(a) {
		t.Fatalf("%d open dialogs starting in %q, want one in %q", fd.opens, fd.startDir, filepath.Dir(a))
	}
}

func TestOpenDialogStartsInTheLatestFolderForAnUntitledDrawing(t *testing.T) {
	a := savedDrawing(t)
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h, _ := recentRig(t, fd, a)
	h.clickID("tool-open")
	h.answered(fd)
	if fd.startDir != filepath.Dir(a) {
		t.Fatalf("dialog started in %q, want %q", fd.startDir, filepath.Dir(a))
	}
}

func TestOpenMenuMissingFileIsGreyedThenPruned(t *testing.T) {
	a := savedDrawing(t)
	gone := filepath.Join(t.TempDir(), "gone.dxf")
	h, path := recentRig(t, nil, gone, a)
	h.clickID("tool-open-menu")
	h.frame()
	if !h.ws.recent.missing[gone] || h.ws.recent.missing[a] {
		t.Fatalf("missing set %v, want only %q", h.ws.recent.missing, gone)
	}
	h.clickID("recent-file-0")
	h.frame()
	s := h.ws.Session()
	if s.FilePath() != "" || s.Active() != "" {
		t.Fatalf("a missing file opened or left OPEN running: %q %q", s.FilePath(), s.Active())
	}
	if hist := s.History(); len(hist) == 0 || !strings.Contains(hist[len(hist)-1], "Open failed") {
		t.Fatalf("no failure line in the history: %q", hist)
	}
	if got := savedRecent(t, path); !slices.Equal(got, []string{a}) {
		t.Fatalf("settings file holds %v, want the missing file dropped", got)
	}
}

func TestOpenMenuClearRecent(t *testing.T) {
	h, path := recentRig(t, nil, savedDrawing(t))
	h.clickID("tool-open-menu")
	h.frame()
	h.clickID("recent-clear")
	h.frame()
	if len(h.ws.RecentFiles()) != 0 || len(h.ws.RecentFolders()) != 0 || len(savedRecent(t, path)) != 0 {
		t.Fatal("Clear recent left entries")
	}
	h.clickID("tool-open-menu")
	h.frame()
	if h.has("recent-clear") {
		t.Fatal("Clear recent offered on empty lists")
	}
}

func TestOpenMenuClosesOnPressOutsideAndEscape(t *testing.T) {
	h, _ := recentRig(t, nil, savedDrawing(t))
	h.clickID("tool-open-menu")
	h.frame()
	n := h.ws.Session().RunCount()
	h.click(gpui.Point{X: 40, Y: 400}) // over the canvas, clear of the start panel
	h.frame()
	if h.ws.OpenMenuShown() {
		t.Fatal("press outside did not close the menu")
	}
	if h.ws.Session().RunCount() != n {
		t.Fatal("the press that closed the menu reached the canvas")
	}
	h.clickID("tool-open-menu")
	h.ws.escape(h.hw.Window())
	if h.ws.OpenMenuShown() {
		t.Fatal("Escape did not close the menu")
	}
}

func TestStartPanelListsRecentAndGoesOnFirstAction(t *testing.T) {
	a := savedDrawing(t)
	h, _ := recentRig(t, nil, a)
	if !h.ws.StartPanelShown() || !h.has("start-panel") || !h.has("start-file-0") {
		t.Fatal("start panel not shown over an empty untitled drawing with recent files")
	}
	h.click(gpui.Point{X: 100, Y: 450}) // the canvas, away from the panel
	h.frame()
	if h.ws.StartPanelShown() || h.has("start-panel") {
		t.Fatal("a click on the canvas did not dismiss the start panel")
	}
	h.ws.Session().Script("NEW\n")
	h.frame()
	if h.has("start-panel") {
		t.Fatal("the start panel came back")
	}
}

func TestStartPanelGoesWhenACommandRuns(t *testing.T) {
	h, _ := recentRig(t, nil, savedDrawing(t))
	h.ws.run("LINE", h.hw.Window())
	h.frame()
	if h.has("start-panel") {
		t.Fatal("start panel still shown after a command started")
	}
}

func TestStartPanelOpensARecentFile(t *testing.T) {
	a := savedDrawing(t)
	h, _ := recentRig(t, nil, a)
	h.clickID("start-file-0")
	h.frame()
	if h.ws.Session().FilePath() != a || h.has("start-panel") {
		t.Fatalf("start panel did not open %q (path %q)", a, h.ws.Session().FilePath())
	}
}

func TestStartPanelNeedsRecentFilesAndSettings(t *testing.T) {
	h, _ := recentRig(t, nil)
	if h.ws.StartPanelShown() {
		t.Fatal("start panel shown with no recent files")
	}
	// Without loaded settings (a -png render) it never shows.
	ws := NewWorkspace(doc.New(), dxfRegistry())
	s := settings.Default()
	s.Recent.AddFile(savedDrawing(t))
	ws.SetSettings(s, nil)
	if ws.StartPanelShown() {
		t.Fatal("start panel shown without LoadSettings")
	}
}

func TestSettingsResetKeepsRecent(t *testing.T) {
	a := savedDrawing(t)
	h, _ := recentRig(t, nil, a)
	h.openPanel()
	h.clickID("settings-reset")
	if got := h.ws.RecentFiles(); !slices.Equal(got, []string{a}) {
		t.Fatalf("reset changed the recent files: %v", got)
	}
}
