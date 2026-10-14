package ui

import (
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// tabsRig is a window over a workspace with a DXF handler and the drawings a.dxf and b.dxf open,
// one tab each.
func tabsRig(t *testing.T) (*sHarness, string, string) {
	t.Helper()
	h := dialogRig(t, nil)
	a, b := drawingAt(t, "a.dxf"), drawingAt(t, "b.dxf")
	w := h.hw.Window()
	h.ws.openFilePath(a, w)
	h.ws.openFilePath(b, w)
	h.frame()
	return h, a, b
}

func TestTwoFilesOpenTwoTabs(t *testing.T) {
	h, a, b := tabsRig(t)
	docs := h.ws.Documents()
	if len(docs) != 2 || docs[0].Path != a || docs[1].Path != b || !docs[1].Active {
		t.Fatalf("tabs %+v, want a.dxf (replacing the untitled drawing) and b.dxf, b active", docs)
	}
	for _, id := range []string{"doc-tab-0", "doc-tab-1", "doc-tab-close-0", "doc-tab-new"} {
		if !h.has(id) {
			t.Fatalf("%s is not in the tab strip", id)
		}
	}
	h.clickID("doc-tab-0")
	if !h.ws.Documents()[0].Active || h.ws.Session().FilePath() != a {
		t.Fatal("clicking a tab did not show its drawing")
	}
	if h.ws.docName() != "a.dxf" {
		t.Fatalf("the document chip shows %q", h.ws.docName())
	}
}

func TestTabsKeepTheirOwnUndoAndView(t *testing.T) {
	h, _, _ := tabsRig(t)
	w := h.hw.Window()
	h.ws.SwitchDocument(0)
	h.ws.Session().Script("LINE 20,20 30,30  ")
	h.ws.Session().Escape()
	h.ws.SwitchDocument(1)
	h.ws.Session().Script("CIRCLE 5,5 2 ")
	h.ws.Session().Escape()
	h.ws.View().PanBy(120, -40)
	h.ws.View().ZoomAt(10, 10, 2)
	viewB := *h.ws.View()
	h.frame()

	h.ws.SwitchDocument(0)
	h.frame()
	if *h.ws.View() == viewB {
		t.Fatal("tab a shows tab b's view")
	}
	h.ws.submit("U", w)
	if n := h.ws.Session().Drawing().Len(); n != 1 {
		t.Fatalf("undo in a left %d entities, want the opened line", n)
	}
	if n := h.ws.DocumentSession(1).Drawing().Len(); n != 2 {
		t.Fatalf("undo in a changed b: %d entities", n)
	}
	h.ws.SwitchDocument(1)
	h.frame()
	if *h.ws.View() != viewB {
		t.Fatalf("tab b's view not kept: %+v, want %+v", *h.ws.View(), viewB)
	}
	h.ws.submit("U", w)
	if n := h.ws.Session().Drawing().Len(); n != 1 {
		t.Fatalf("undo in b left %d entities", n)
	}
}

func TestClosingADirtyTabAsks(t *testing.T) {
	h, a, _ := tabsRig(t)
	h.ws.DocumentSession(1).Script("LINE 0,0 1,1  ")
	h.ws.SwitchDocument(0)
	h.frame()
	h.clickID("doc-tab-close-1")
	s := h.ws.Session()
	p := s.CurrentPrompt()
	if !h.ws.Documents()[1].Active || s.Active() != "CLOSE" || p == nil || !slices.Contains(p.Keywords, "Yes") {
		t.Fatalf("closing a dirty tab should show it and ask Save changes, active %q prompt %+v", s.Active(), p)
	}
	h.ws.submit("Cancel", h.hw.Window())
	if len(h.ws.Documents()) != 2 {
		t.Fatal("Cancel closed the tab")
	}
	h.frame()
	h.clickID("doc-tab-close-1")
	h.ws.submit("No", h.hw.Window())
	docs := h.ws.Documents()
	if len(docs) != 1 || docs[0].Path != a || !docs[0].Active {
		t.Fatalf("after No: %+v", docs)
	}
}

func TestCleanTabClosesAtOnceAndByMiddleClick(t *testing.T) {
	h, _, b := tabsRig(t)
	h.ws.run("NEW", h.hw.Window())
	if docs := h.ws.Documents(); len(docs) != 3 || !docs[2].Active || docs[2].Path != "" {
		t.Fatalf("NEW should open an untitled tab: %+v", docs)
	}
	h.frame()
	h.clickID("doc-tab-close-0")
	if docs := h.ws.Documents(); len(docs) != 2 || docs[0].Path != b {
		t.Fatalf("a clean tab should close without asking: %+v", docs)
	}
	h.frame()
	p := h.find("doc-tab-0")
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 1}
	h.hw.Dispatch(up)
	h.hw.Dispatch(gpui.ClickEvent{Up: &up})
	if docs := h.ws.Documents(); len(docs) != 1 || docs[0].Path != "" {
		t.Fatalf("middle click should close the tab: %+v", docs)
	}
	// The last tab is emptied, not removed.
	h.ws.Session().Script("LINE 0,0 1,1  ")
	h.ws.Session().MarkSaved()
	h.ws.run("CLOSE", h.hw.Window())
	if docs := h.ws.Documents(); len(docs) != 1 || h.ws.Session().Drawing().Len() != 0 {
		t.Fatalf("CLOSE of the last tab: %+v", docs)
	}
}

func TestCtrlTabSwitchesTabs(t *testing.T) {
	h, _, _ := tabsRig(t)
	h.ws.run("NEW", h.hw.Window())
	h.frame()
	h.ctrl("tab")
	if !h.ws.Documents()[0].Active {
		t.Fatalf("Ctrl+Tab from the last tab should wrap to the first: %+v", h.ws.Documents())
	}
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Control | gpui.Shift}})
	if !h.ws.Documents()[2].Active {
		t.Fatalf("Ctrl+Shift+Tab should go back: %+v", h.ws.Documents())
	}
}

func TestQuitAsksEachDirtyTab(t *testing.T) {
	h, _, _ := tabsRig(t)
	quit := 0
	h.ws.SetQuitHandler(func() { quit++ })
	h.ws.run("NEW", h.hw.Window()) // a clean third tab is not asked about
	h.ws.DocumentSession(0).Script("LINE 0,0 1,1  ")
	h.ws.DocumentSession(1).Script("LINE 0,0 2,2  ")
	h.ws.SwitchDocument(1)
	w := h.hw.Window()
	h.ws.run("QUIT", w)
	if h.ws.Session().Active() != "QUIT" {
		t.Fatal("QUIT on a dirty tab should ask")
	}
	h.ws.submit("No", w)
	if quit != 0 || !h.ws.Documents()[0].Active || h.ws.Session().Active() != "QUIT" {
		t.Fatalf("the other dirty tab should be shown and asked: quit %d, %+v", quit, h.ws.Documents())
	}
	h.ws.submit("Cancel", w)
	if quit != 0 {
		t.Fatal("Cancel did not stop the quit")
	}
	// A new QUIT starts over and asks every dirty tab again.
	h.ws.run("QUIT", w)
	h.ws.submit("No", w)
	if !h.ws.Documents()[1].Active || h.ws.Session().Active() != "QUIT" {
		t.Fatalf("a new quit should ask the other tab again: %+v", h.ws.Documents())
	}
	h.ws.submit("No", w)
	if quit != 1 {
		t.Fatalf("quit called %d times, want once after the last answer", quit)
	}
}

func TestOpenPathsOpensEveryDrawingBeforeTheWindow(t *testing.T) {
	ws := NewWorkspace(doc.New(), dxfRegistry())
	a, b, c := drawingAt(t, "a.dxf"), drawingAt(t, "b.dxf"), drawingAt(t, "c.dxf")
	ws.OpenPaths([]string{a, b, c})
	docs := ws.Documents()
	if len(docs) != 3 || docs[0].Path != a || docs[1].Path != b || docs[2].Path != c {
		t.Fatalf("tabs %+v", docs)
	}
	for i := range docs {
		if !ws.tabs[i].fitPending {
			t.Fatalf("tab %d is not fitted on its first frame", i)
		}
	}
}
