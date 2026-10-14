package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// drop drops paths on the middle of the window, as the platform reports a drop from the OS.
func (h *sHarness) drop(paths ...string) {
	h.t.Helper()
	b := h.hw.Window().Bounds()
	h.hw.Dispatch(gpui.FileDropEvent{Position: gpui.Point{X: b.Size.W / 2, Y: b.Size.H / 2}, Paths: paths})
}

// historyHas reports whether a history line contains every part.
func historyHas(ws *Workspace, parts ...string) bool {
	return slices.ContainsFunc(ws.Session().History(), func(l string) bool {
		for _, p := range parts {
			if !strings.Contains(l, p) {
				return false
			}
		}
		return true
	})
}

// drawingAt copies the one-line drawing savedDrawing writes to name in a fresh folder.
func drawingAt(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(savedDrawing(t))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDroppedFileOpens(t *testing.T) {
	h := dialogRig(t, nil)
	path := drawingAt(t, "floor plan.dxf")
	h.drop(path)
	s := h.ws.Session()
	if s.FilePath() != path {
		t.Fatalf("file path %q, want %q", s.FilePath(), path)
	}
	if n := s.Drawing().Len(); n != 1 {
		t.Fatalf("drawing has %d entities, want the saved line", n)
	}
	if s.Active() != "" {
		t.Fatalf("OPEN still running: %q", s.Active())
	}
}

func TestDroppedUnsupportedFileIsReported(t *testing.T) {
	h := dialogRig(t, nil)
	h.drop("/somewhere/notes.txt")
	s := h.ws.Session()
	if s.Active() != "" || s.FilePath() != "" || s.Drawing().Len() != 0 {
		t.Fatalf("an unsupported drop changed state: active %q path %q", s.Active(), s.FilePath())
	}
	if !historyHas(h.ws, "Cannot open notes.txt", "DXF") {
		t.Fatalf("no report in the history: %q", s.History())
	}
}

func TestDroppedFilesOpenOneTabEach(t *testing.T) {
	h := dialogRig(t, nil)
	a, b, c := drawingAt(t, "a.dxf"), drawingAt(t, "b.dxf"), drawingAt(t, "c.dxf")
	h.drop("/x/readme.txt", a, b, c)
	docs := h.ws.Documents()
	if len(docs) != 3 {
		t.Fatalf("want three tabs (the untitled one replaced by the first), got %+v", docs)
	}
	for i, want := range []string{a, b, c} {
		if docs[i].Path != want || h.ws.DocumentSession(i).Drawing().Len() != 1 {
			t.Fatalf("tab %d is %q, want %q with its line", i, docs[i].Path, want)
		}
	}
	if !docs[2].Active {
		t.Fatalf("the last drawing opened should be active: %+v", docs)
	}
	if !slices.ContainsFunc(h.ws.DocumentSession(0).History(), func(l string) bool { return strings.Contains(l, "Cannot open readme.txt") }) {
		t.Fatalf("the unsupported file is not reported: %q", h.ws.DocumentSession(0).History())
	}
	if !historyHas(h.ws, "Opened", "c.dxf") {
		t.Fatalf("the tab's history does not say what was opened: %q", h.ws.Session().History())
	}
	// Dropping an open file again shows its tab.
	h.drop(a)
	if docs := h.ws.Documents(); len(docs) != 3 || !docs[0].Active {
		t.Fatalf("an open file should switch to its tab: %+v", docs)
	}
}

func TestDroppedFileOpensBesideADirtyDrawing(t *testing.T) {
	h := dialogRig(t, nil)
	h.ws.Session().Script("LINE 0,0 5,5  ")
	path := drawingAt(t, "c.dxf")
	h.drop(path)
	if s := h.ws.Session(); s.FilePath() != path || s.Active() != "" {
		t.Fatalf("the file should open in its own tab, path %q active %q", s.FilePath(), s.Active())
	}
	if s := h.ws.DocumentSession(0); !s.Dirty() || s.Drawing().Len() != 1 || s.FilePath() != "" {
		t.Fatal("the dirty drawing was replaced")
	}
}

func TestOpenPathsBeforeTheWindowOpensTheFile(t *testing.T) {
	// cad plan.dxf, and a macOS launch by document: the request comes before the first frame.
	ws := NewWorkspace(doc.New(), dxfRegistry())
	path := drawingAt(t, "start.dxf")
	ws.OpenPaths([]string{path})
	if ws.Session().FilePath() != path || ws.Session().Drawing().Len() != 1 {
		t.Fatalf("not opened: path %q, %d entities", ws.Session().FilePath(), ws.Session().Drawing().Len())
	}
	if !ws.fitPending {
		t.Fatal("the opened drawing should be fitted once the canvas has its size")
	}
}

func TestOpenPathsFromTheOSReachTheWorkspace(t *testing.T) {
	h := dialogRig(t, nil)
	path := drawingAt(t, "dock.dxf")
	app := gpui.NewApp()
	app.OnOpenPaths(func(paths []string, _ *gpui.App) { h.ws.OpenPaths(paths) })
	app.OpenPaths([]string{path})
	if h.ws.Session().FilePath() != path {
		t.Fatalf("file path %q, want %q", h.ws.Session().FilePath(), path)
	}
}
