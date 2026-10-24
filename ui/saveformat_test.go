package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cad-lite/filedialog"
	"cad-lite/internal/gpui"
)

func fileVersion(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, after, ok := strings.Cut(string(b), "$ACADVER\n  1\n"); ok {
		v, _, _ := strings.Cut(after, "\n")
		return v
	}
	t.Fatalf("%s has no $ACADVER", path)
	return ""
}

// Save as shows the format menu with the drawing's format ticked; choosing one shows the save
// dialog listing that format alone and runs SAVEAS with it.
func TestSaveAsFormatMenuChoosesTheFormat(t *testing.T) {
	target := filepath.Join(t.TempDir(), "old.dxf")
	fd := &fakeDialog{path: target}
	h := dialogRig(t, fd)
	h.ws.Session().Script("LINE 0,0 10,10  ")
	h.frame()
	h.rightClick("tool-save")
	h.frame()
	if !h.ws.SaveFormatMenuShown() || fd.calls() != 0 {
		t.Fatalf("menu %v, dialog calls %d", h.ws.SaveFormatMenuShown(), fd.calls())
	}
	ids := h.frameIDs()
	for _, id := range []string{"save-format-menu", "save-format-ac1015", "save-format-ac1009"} {
		if !ids[id] {
			t.Fatalf("%s not shown", id)
		}
	}
	if vs, cur := h.ws.saveFormats(); len(vs) != 2 || vs[0] != "AC1015" || cur != "AC1009" {
		t.Fatalf("formats %v current %q", vs, cur)
	}
	h.png("save-format-menu.png")
	h.clickID("save-format-ac1009")
	h.answered(fd)
	if h.ws.SaveFormatMenuShown() {
		t.Fatal("the menu stayed open")
	}
	if len(fd.filters) != 1 || fd.filters[0].Name != "AutoCAD R12/LT2 DXF (*.dxf)" || fd.filters[0].Extensions[0] != "dxf" {
		t.Fatalf("dialog filters %+v", fd.filters)
	}
	s := h.ws.Session()
	if s.Active() != "" || s.FilePath() != target || fileVersion(t, target) != "AC1009" || s.Drawing().Header.Version != "AC1009" {
		t.Fatalf("active %q path %q version %q", s.Active(), s.FilePath(), s.Drawing().Header.Version)
	}

	// Choosing 2000 the next time saves AutoCAD 2000, and the Save button keeps it.
	h.frame()
	h.rightClick("tool-save")
	h.frame()
	h.clickID("save-format-ac1015")
	h.answered(fd)
	if fileVersion(t, target) != "AC1015" {
		t.Fatalf("saved %s", fileVersion(t, target))
	}
	os.Remove(target)
	s.Script("LINE 1,1 2,2  ")
	h.frame()
	h.clickID("tool-save")
	if fileVersion(t, target) != "AC1015" {
		t.Fatal("QSAVE changed the format")
	}
}

func TestSaveAsFormatMenuClosesOnAPressOutside(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h := dialogRig(t, fd)
	h.ws.saveFileAs(h.hw.Window()) // Ctrl+Shift+S
	h.frame()
	if !h.ws.SaveFormatMenuShown() {
		t.Fatal("Ctrl+Shift+S showed no format menu")
	}
	h.click(gpui.Point{X: 600, Y: 400})
	h.frame()
	if h.ws.SaveFormatMenuShown() || fd.calls() != 0 || h.ws.Session().Active() != "" {
		t.Fatalf("menu %v, dialog calls %d, active %q", h.ws.SaveFormatMenuShown(), fd.calls(), h.ws.Session().Active())
	}
}

// Without a dialog, Save as runs SAVEAS on the command line, which asks for the format.
func TestSaveAsWithoutDialogAsksTheFormatOnTheCommandLine(t *testing.T) {
	h := dialogRig(t, nil)
	h.rightClick("tool-save")
	s := h.ws.Session()
	if s.Active() != "SAVEAS" || !strings.HasPrefix(s.Prompt(), "Enter file format [R12/2000]") {
		t.Fatalf("active %q prompt %q", s.Active(), s.Prompt())
	}
}
