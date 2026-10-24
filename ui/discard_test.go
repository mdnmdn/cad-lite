package ui

import (
	"slices"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// dirtyTab draws a line to p in s and leaves no command running.
func dirtyTab(s *cmdsys.Session, p string) {
	s.Script("LINE 0,0 " + p + "  ")
	s.Escape()
}

// discardOpen returns whether the unsaved-changes dialog is open, whether it is the quit one,
// and what it lists, after checking that the frame shows it exactly then.
func (h *sHarness) discardOpen() (bool, bool, []string) {
	h.t.Helper()
	h.frame()
	open, quit, names := h.ws.DiscardDialog()
	if open != h.inFrame("discard-dialog") {
		h.t.Fatalf("dialog state %v but frame shows it: %v", open, !open)
	}
	return open, quit, names
}

func TestClosingADirtyTabShowsTheDialog(t *testing.T) {
	h, a, _ := tabsRig(t)
	dirtyTab(h.ws.DocumentSession(1), "1,1")
	h.ws.SwitchDocument(0)
	h.frame()
	h.clickID("doc-tab-close-1")
	open, quit, names := h.discardOpen()
	if !open || quit || !slices.Equal(names, []string{"b.dxf"}) || !h.ws.Documents()[1].Active {
		t.Fatalf("closing a dirty tab should show it and ask in the dialog: open %v quit %v names %v", open, quit, names)
	}
	if h.ws.Session().Active() != "" {
		t.Fatalf("the command line should not ask too: %q", h.ws.Session().Active())
	}
	h.savePNG("discard", "close-en.png")
	h.key("escape")
	if open, _, _ := h.discardOpen(); open || len(h.ws.Documents()) != 2 || !h.ws.Documents()[1].Dirty {
		t.Fatalf("Escape should cancel and keep the tab: %+v", h.ws.Documents())
	}
	h.clickID("doc-tab-close-1")
	h.clickID("discard-cancel")
	if open, _, _ := h.discardOpen(); open || len(h.ws.Documents()) != 2 {
		t.Fatal("Cancel should keep the tab")
	}
	h.clickID("doc-tab-close-1")
	h.clickID("discard-close")
	docs := h.ws.Documents()
	if open, _, _ := h.discardOpen(); open || len(docs) != 1 || docs[0].Path != a || !docs[0].Active {
		t.Fatalf("Close anyway should close the tab: %+v", docs)
	}
}

func TestTypedCloseAsksInTheDialogAndTheKeysWork(t *testing.T) {
	h, _, _ := tabsRig(t)
	dirtyTab(h.ws.Session(), "1,1")
	h.frame()
	h.typeCmd("CLOSE")
	h.key("enter")
	if open, _, names := h.discardOpen(); !open || !slices.Equal(names, []string{"b.dxf"}) {
		t.Fatalf("typed CLOSE: open %v names %v", open, names)
	}
	// Cancel has the focus: Enter keeps the drawing.
	h.key("enter")
	if open, _, _ := h.discardOpen(); open || len(h.ws.Documents()) != 2 {
		t.Fatal("Enter on the focused Cancel should keep the tab")
	}
	h.typeCmd("CLOSE")
	h.key("enter")
	h.key("tab")
	h.key("enter")
	if open, _, _ := h.discardOpen(); open || len(h.ws.Documents()) != 1 {
		t.Fatalf("Tab then Enter should press Close anyway: %+v", h.ws.Documents())
	}
	if h.ws.input.Text() != "" || !h.ws.input.FocusHandle().IsFocused(h.hw.Window()) {
		t.Fatal("the command line should have the focus back")
	}
}

func TestCloseAnywayOnTheLastTabEmptiesIt(t *testing.T) {
	h := dialogRig(t, nil)
	dirtyTab(h.ws.Session(), "1,1")
	h.frame()
	h.clickID("doc-tab-close-0")
	if open, _, names := h.discardOpen(); !open || !slices.Equal(names, []string{"Untitled"}) {
		t.Fatalf("open %v names %v", open, names)
	}
	h.clickID("discard-close")
	if docs := h.ws.Documents(); len(docs) != 1 || h.ws.Session().Drawing().Len() != 0 || docs[0].Dirty {
		t.Fatalf("the last tab should be emptied: %+v", docs)
	}
}

func TestQuitListsEveryDirtyDrawing(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	h, _, _ := tabsRig(t)
	quit := 0
	h.ws.SetQuitHandler(func() { quit++ })
	w := h.hw.Window()
	h.ws.run("NEW", w) // a clean third tab is not listed
	dirtyTab(h.ws.DocumentSession(0), "1,1")
	dirtyTab(h.ws.DocumentSession(1), "2,2")
	h.ws.run("QUIT", w)
	open, isQuit, names := h.discardOpen()
	if !open || !isQuit || !slices.Equal(names, []string{"a.dxf", "b.dxf"}) || quit != 0 {
		t.Fatalf("QUIT: open %v quit %v names %v quits %d", open, isQuit, names, quit)
	}
	for _, id := range []string{"discard-item-0", "discard-item-1", "discard-cancel", "discard-close"} {
		if !h.inFrame(id) {
			t.Fatalf("%s is not in the dialog", id)
		}
	}
	h.savePNG("discard", "quit-en.png")
	h.clickID("discard-cancel")
	if open, _, _ := h.discardOpen(); open || quit != 0 || len(h.ws.Documents()) != 3 || !h.ws.Documents()[0].Dirty {
		t.Fatal("Cancel should keep everything")
	}
	i18n.Set(i18n.Italian)
	h.ws.run("EXIT", w)
	h.frame()
	h.savePNG("discard", "quit-it.png")
	h.clickID("discard-close")
	if quit != 1 {
		t.Fatalf("Close anyway should quit once, got %d", quit)
	}
}

func TestItalianCloseDialog(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	i18n.Set(i18n.Italian)
	h, _, _ := tabsRig(t)
	dirtyTab(h.ws.Session(), "1,1")
	h.ws.run("CLOSE", h.hw.Window())
	if open, _, _ := h.discardOpen(); !open {
		t.Fatal("no dialog")
	}
	h.savePNG("discard", "close-it.png")
	if i18n.T("ui.discard.close_anyway") != "Chiudi comunque" || i18n.T("ui.discard.cancel") != "Annulla" {
		t.Fatalf("Italian buttons %q / %q", i18n.T("ui.discard.close_anyway"), i18n.T("ui.discard.cancel"))
	}
}

func TestQuitWithNothingUnsavedQuitsAtOnce(t *testing.T) {
	h, _, _ := tabsRig(t)
	quit := 0
	h.ws.SetQuitHandler(func() { quit++ })
	h.ws.run("QUIT", h.hw.Window())
	if open, _, _ := h.discardOpen(); open || quit != 1 {
		t.Fatalf("clean QUIT: dialog %v quits %d", open, quit)
	}
}

// closeRig is a window over a workspace whose App the test keeps, to press the close button.
func closeRig(t *testing.T) (*sHarness, *gpui.App) {
	t.Helper()
	ws := NewWorkspace(doc.New(), dxfRegistry())
	ws.docTab.projectPath = t.TempDir() + "/drawing.dxf"
	p := gpui.NewHeadlessPlatform()
	var app *gpui.App
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		app = cx
		cx.Keymap().AddBindings(Bindings()...)
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 1000, Height: 700}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	return &sHarness{t: t, ws: ws, hp: p, hw: hw}, app
}

func TestWindowCloseButtonAsksAboutUnsavedDrawings(t *testing.T) {
	h, app := closeRig(t)
	w := h.hw.Window()
	quit := 0
	h.ws.SetQuitHandler(func() { quit++; app.Quit() })
	dirtyTab(h.ws.Session(), "1,1")
	h.ws.run("NEW", w)
	if w.RequestClose(app) || app.IsQuitting() {
		t.Fatal("the close button must not close the window over unsaved changes")
	}
	open, isQuit, names := h.discardOpen()
	if !open || !isQuit || !slices.Equal(names, []string{"Untitled"}) {
		t.Fatalf("close button: open %v quit %v names %v", open, isQuit, names)
	}
	if w.RequestClose(app) {
		t.Fatal("a second press while the dialog shows must keep the window")
	}
	h.key("escape")
	if open, _, _ := h.discardOpen(); open || app.IsQuitting() || quit != 0 {
		t.Fatal("Cancel should keep the window open")
	}
	w.RequestClose(app)
	h.frame()
	h.clickID("discard-close")
	if quit != 1 || !app.IsQuitting() {
		t.Fatalf("Close anyway should quit: quits %d quitting %v", quit, app.IsQuitting())
	}
}

func TestWindowCloseButtonQuitsAtOnceWhenAllIsSaved(t *testing.T) {
	h, app := closeRig(t)
	if !h.hw.Window().RequestClose(app) || !app.IsQuitting() {
		t.Fatal("with nothing unsaved the close button should quit")
	}
}
