package ui

import (
	"path/filepath"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

// twoTabRig is propsRig with a second, empty tab added after the first, the first active, the
// rig's line selected and its Start X field open holding "5".
func twoTabRig(t *testing.T) (*sHarness, []doc.Handle) {
	t.Helper()
	h, hs := propsRig(t, 1)
	h.ws.run("NEW", h.hw.Window())
	h.ws.SwitchDocument(0)
	h.frame()
	h.selectHandles(hs[0])
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("5")
	if h.ws.pe.id != "start_x" {
		t.Fatal("the Start X field did not open")
	}
	return h, hs
}

// tabLine is the rig's line in the first tab's drawing.
func tabLine(h *sHarness, hs []doc.Handle) *doc.Line {
	return h.ws.DocumentSession(0).Drawing().Entity(hs[0]).(*doc.Line)
}

func TestPropsEditCommittedOnCtrlTab(t *testing.T) {
	h, hs := twoTabRig(t)
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Control}})
	h.frame()
	if h.ws.docTab == h.ws.tabs[0] {
		t.Fatal("Ctrl+Tab did not switch tabs")
	}
	if x := tabLine(h, hs).Start.X; x != 5 {
		t.Fatalf("Ctrl+Tab lost the open edit: start x %v, want 5", x)
	}
	if h.ws.pe.id != "" {
		t.Fatal("the field stays open after the switch")
	}
	h.ws.SwitchDocument(0)
	h.ws.submit("U", h.hw.Window())
	if x := tabLine(h, hs).Start.X; x != 0 {
		t.Fatalf("the committed edit is not one undo step in its own tab: start x %v", x)
	}
}

func TestPropsEditCommittedOnTabClick(t *testing.T) {
	h, hs := twoTabRig(t)
	h.clickID("doc-tab-chip-1")
	h.frame()
	if h.ws.docTab != h.ws.tabs[1] {
		t.Fatal("the click did not switch tabs")
	}
	if x := tabLine(h, hs).Start.X; x != 5 {
		t.Fatalf("a tab click lost the open edit: start x %v, want 5", x)
	}
}

func TestPropsEditCommittedOnTabClose(t *testing.T) {
	h, hs := twoTabRig(t)
	s0 := h.ws.DocumentSession(0)
	// Checked before the next frame, when a focus change could commit the field by blurring it.
	h.ws.CloseDocument(0, h.hw.Window())
	if x := s0.Drawing().Entity(hs[0]).(*doc.Line).Start.X; x != 5 {
		t.Fatalf("closing the tab lost the open edit: start x %v, want 5", x)
	}
}

func TestPropsEditCommittedBeforeSave(t *testing.T) {
	ws := NewWorkspace(doc.New(), dxfRegistry())
	h := openSettingsWS(t, ws, 1280, 800, 1)
	s := ws.Session()
	path := filepath.Join(t.TempDir(), "plan.dxf")
	s.Script("LINE 0,0 100,0  ZOOM E")
	s.SetFilePath(path)
	l := s.Drawing().Handles()[0]
	h.selectHandles(l)
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("5")
	h.ctrl("s")
	h.frame()
	d, _, err := dxfRegistry().Open(path)
	if err != nil {
		t.Fatalf("Ctrl+S did not save: %v", err)
	}
	var got geom.Vec2
	for _, hd := range d.Handles() {
		if ln, ok := d.Entity(hd).(*doc.Line); ok {
			got = ln.Start
		}
	}
	if got.X != 5 {
		t.Fatalf("the saved file misses the open edit: start %v", got)
	}
	if s.Dirty() {
		t.Fatal("the drawing is dirty after the save: the edit landed after it")
	}
}
