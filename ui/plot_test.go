package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/plot"
	"cad-lite/settings"
)

// fakePrinter records the PDFs it is asked to print, reading each before the job removes it.
type fakePrinter struct {
	mu     sync.Mutex
	jobs   []string
	titles []string
	specs  []plot.Job
	err    error
}

func (f *fakePrinter) print(_ context.Context, path string, job plot.Job) error {
	data, err := os.ReadFile(path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		return err
	}
	f.jobs = append(f.jobs, string(data))
	f.titles = append(f.titles, job.Title)
	f.specs = append(f.specs, job)
	return f.err
}

func (f *fakePrinter) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.jobs) }

// plotRig is a workspace with the demo-like drawing, a fake printer and fd as its file dialog.
func plotRig(t *testing.T, fd *fakeDialog) (*sHarness, *fakePrinter) {
	t.Helper()
	h := dialogRig(t, fd)
	fp := &fakePrinter{}
	h.ws.SetPrinter(fp.print)
	h.ws.SetPrinterLister(func(context.Context) ([]plot.Printer, error) {
		return []plot.Printer{{Name: "Office", Default: true}, {Name: "Plotter"}}, nil
	})
	h.ws.SetViewer(func(string) error { return errors.New("no viewer in tests") })
	h.ws.Session().Script("RECTANG 0,0 220,130 CIRCLE 110,65 20 ")
	h.ws.Session().Escape()
	h.frame()
	return h, fp
}

// plotShown reports whether the dialog is open, checking the frame agrees.
func (h *sHarness) plotShown() bool {
	h.t.Helper()
	h.frame()
	open := h.ws.PlotOpen()
	if open != h.inFrame("plot-dialog") {
		h.t.Fatalf("dialog state %v but frame shows it: %v", open, !open)
	}
	return open
}

func (h *sHarness) historyHas(s string) bool {
	for _, l := range h.ws.Session().History() {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func TestPlotOpensFromTheTitleBarCtrlPAndTheCommandLine(t *testing.T) {
	h, _ := plotRig(t, nil)
	h.clickID("tool-plot")
	if !h.plotShown() {
		t.Fatal("the Plot button should open the dialog")
	}
	h.savePNG("plot", "dialog.png")
	h.key("escape")
	if h.plotShown() {
		t.Fatal("Escape should close the dialog")
	}
	h.ctrl("p")
	if !h.plotShown() {
		t.Fatal("Ctrl+P should open the dialog")
	}
	h.clickID("plot-cancel")
	if h.plotShown() {
		t.Fatal("Cancel should close the dialog")
	}
	h.typeCmd("PRINT")
	h.key("enter")
	if !h.plotShown() {
		t.Fatal("typed PRINT should open the dialog")
	}
}

func TestPlotOptionsChangeThePreview(t *testing.T) {
	h, _ := plotRig(t, nil)
	h.clickID("tool-plot")
	h.plotShown()
	p := h.ws.plotState()
	if p.sheet == nil || p.preview == nil || p.opt.What != plot.WhatAll {
		t.Fatalf("no preview or wrong default: sheet %v what %v", p.sheet != nil, p.opt.What)
	}
	if b := p.preview.Bounds(); b.Dx() <= b.Dy() {
		t.Fatalf("A4 landscape preview is %v", b)
	}
	v := p.version
	h.clickID("plot-paper-A3")
	h.clickID("plot-portrait")
	h.clickID("plot-mono")
	h.plotShown()
	if p.opt.Paper.Name != "A3" || p.opt.Landscape || p.opt.Style != plot.StyleMono || p.version == v {
		t.Fatalf("options %+v, preview version %d after %d", p.opt, p.version, v)
	}
	if p.sheet.Width != 297 || p.sheet.Height != 420 {
		t.Fatalf("sheet %vx%v, want A3 portrait", p.sheet.Width, p.sheet.Height)
	}
	if b := p.preview.Bounds(); b.Dx() >= b.Dy() {
		t.Fatalf("portrait preview is %v", b)
	}
	h.savePNG("plot", "a3-mono.png")
	// The selection button does nothing without a selection.
	h.clickID("plot-what-selection")
	if p.opt.What != plot.WhatAll {
		t.Fatal("the selection button should be disabled with nothing selected")
	}
	// The options outlive the dialog.
	h.key("escape")
	h.ctrl("p")
	h.plotShown()
	if p.opt.Paper.Name != "A3" || p.opt.Style != plot.StyleMono {
		t.Fatalf("options were not kept: %+v", p.opt)
	}
}

func TestPlotStartsOnTheSelection(t *testing.T) {
	h, _ := plotRig(t, nil)
	hs := h.ws.Session().Drawing().Handles()
	h.selectHandles(hs[1])
	h.ctrl("p")
	h.plotShown()
	p := h.ws.plotState()
	if p.opt.What != plot.WhatSelection || len(p.opt.Selection) != 1 || p.sheet == nil {
		t.Fatalf("with a selection the dialog should plot it: %+v", p.opt)
	}
	// The circle alone: 40 units across 190 mm of A4 landscape height.
	if want := 190.0 / 40; p.sheet.Scale < want-1e-9 || p.sheet.Scale > want+1e-9 {
		t.Fatalf("scale %v, want %v", p.sheet.Scale, want)
	}
	h.clickID("plot-what-all")
	h.plotShown()
	if want := 277.0 / 220; p.opt.What != plot.WhatAll || p.sheet.Scale < want-1e-9 || p.sheet.Scale > want+1e-9 {
		t.Fatalf("All should plot the whole drawing: what %v scale %v", p.opt.What, p.sheet.Scale)
	}
}

func TestPlotWindowIsPickedOnTheDrawing(t *testing.T) {
	h, _ := plotRig(t, nil)
	h.ctrl("p")
	h.plotShown()
	h.clickID("plot-area-window")
	if h.plotShown() || h.ws.Session().Active() != "PLOT" {
		t.Fatalf("Window with no window yet should hide the dialog and ask for corners: active %q", h.ws.Session().Active())
	}
	h.typeCmd("100,40")
	h.key("enter")
	h.typeCmd("120,90")
	h.key("enter")
	if !h.plotShown() {
		t.Fatal("the dialog should come back after the second corner")
	}
	p := h.ws.plotState()
	if p.opt.Area != plot.AreaWindow || p.window != geom.BoxOf(geom.V(100, 40), geom.V(120, 90)) || p.sheet == nil {
		t.Fatalf("area %v window %+v", p.opt.Area, p.window)
	}
	if want := 190.0 / 50; p.sheet.Scale < want-1e-9 || p.sheet.Scale > want+1e-9 {
		t.Fatalf("scale %v, want the 50-unit window height on 190 mm", p.sheet.Scale)
	}
	h.savePNG("plot", "window.png")
	// Escape during a pick brings the dialog back with the window it had.
	h.clickID("plot-pick")
	if h.plotShown() {
		t.Fatal("Pick should hide the dialog")
	}
	h.key("escape")
	if !h.plotShown() || p.opt.Area != plot.AreaWindow || p.window.Max.Y != 90 {
		t.Fatalf("cancelled pick: open %v area %v window %+v", h.ws.PlotOpen(), p.opt.Area, p.window)
	}
	// A window with nothing in it says so and cannot be printed.
	h.clickID("plot-pick")
	h.typeCmd("1000,1000")
	h.key("enter")
	h.typeCmd("1100,1100")
	h.key("enter")
	h.plotShown()
	if p.sheet != nil || !errors.Is(p.err, plot.ErrNothing) {
		t.Fatalf("empty window: sheet %v err %v", p.sheet != nil, p.err)
	}
	h.clickID("plot-print")
	if !h.ws.PlotOpen() {
		t.Fatal("Print should do nothing with nothing to plot")
	}
}

func TestPlotPrintSendsAPDFAndCloses(t *testing.T) {
	h, fp := plotRig(t, nil)
	h.ctrl("p")
	h.plotShown()
	h.clickID("plot-print")
	h.settle(func() bool { return !h.ws.PlotOpen() })
	if fp.count() != 1 || !strings.HasPrefix(fp.jobs[0], "%PDF-") || fp.titles[0] != "plan" && fp.titles[0] != h.ws.plotTitle() {
		t.Fatalf("printer got %d jobs, titles %v", fp.count(), fp.titles)
	}
	if !h.historyHas("A4") {
		t.Fatalf("the command line should report the job: %q", h.ws.Session().History())
	}
	// A failing printer keeps the dialog open and says why.
	fp.err = errors.New("no default destination")
	h.ctrl("p")
	h.plotShown()
	h.key("enter")
	h.settle(func() bool { return !h.ws.plotState().busy })
	if !h.plotShown() || !strings.Contains(h.ws.plotState().message, "no default destination") {
		t.Fatalf("failure: open %v message %q", h.ws.PlotOpen(), h.ws.plotState().message)
	}
}

func TestPlotSavesAPDF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sheet")
	fd := &fakeDialog{path: path}
	h, _ := plotRig(t, fd)
	h.ctrl("p")
	h.plotShown()
	h.clickID("plot-save")
	h.settle(func() bool { return !h.ws.PlotOpen() })
	data, err := os.ReadFile(path + ".pdf")
	if err != nil || !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatalf("saved PDF: %v", err)
	}
	if fd.defaultName != h.ws.plotTitle()+".pdf" || len(fd.filters) != 1 || fd.filters[0].Extensions[0] != "pdf" {
		t.Fatalf("dialog asked for %q with %+v", fd.defaultName, fd.filters)
	}
	if !h.historyHas(path + ".pdf") {
		t.Fatal("the command line should name the saved file")
	}
}

func TestPlotPrinterCopiesAndViewer(t *testing.T) {
	h, fp := plotRig(t, nil)
	var viewed []string
	h.ws.SetViewer(func(path string) error {
		data, err := os.ReadFile(path)
		if err == nil && strings.HasPrefix(string(data), "%PDF-") {
			viewed = append(viewed, path)
		}
		return err
	})
	h.ctrl("p")
	h.settle(func() bool { return !h.ws.plotState().listing })
	if !h.plotShown() || !h.inFrame("plot-printer-1") {
		t.Fatal("the printers should be listed in the dialog")
	}
	h.savePNG("plot", "printers.png")
	h.clickID("plot-printer-1")
	h.clickID("plot-copies-more")
	h.clickID("plot-copies-more")
	h.clickID("plot-copies-less")
	p := h.ws.plotState()
	if p.printer != "Plotter" || p.copies != 2 {
		t.Fatalf("printer %q copies %d", p.printer, p.copies)
	}
	h.clickID("plot-print")
	h.settle(func() bool { return !h.ws.PlotOpen() })
	if fp.count() != 1 || fp.specs[0].Printer != "Plotter" || fp.specs[0].Copies != 2 {
		t.Fatalf("job %+v", fp.specs)
	}
	if !h.historyHas("Plotter") {
		t.Fatalf("the command line should name the printer: %q", h.ws.Session().History())
	}
	// The viewer gets a PDF and the dialog closes.
	h.ctrl("p")
	h.plotShown()
	h.clickID("plot-view")
	h.settle(func() bool { return !h.ws.PlotOpen() })
	if len(viewed) != 1 || filepath.Dir(viewed[0]) != plot.TempDir() {
		t.Fatalf("viewer got %v", viewed)
	}
	// A printer the system no longer lists falls back to the default.
	h.ws.SetPrinterLister(func(context.Context) ([]plot.Printer, error) {
		return []plot.Printer{{Name: "Office", Default: true}}, nil
	})
	h.ctrl("p")
	h.settle(func() bool { return !h.ws.plotState().listing })
	if p.printer != "" {
		t.Fatalf("a vanished printer should fall back to the default, got %q", p.printer)
	}
	// No printing system: Print is disabled and says so.
	h.key("escape")
	h.ws.SetPrinterLister(func(context.Context) ([]plot.Printer, error) { return nil, plot.ErrNoPrinting })
	h.ctrl("p")
	h.settle(func() bool { return !h.ws.plotState().listing })
	h.clickID("plot-print")
	if !h.plotShown() || fp.count() != 1 {
		t.Fatal("Print should be disabled without a printing system")
	}
}

func TestPlotScaleOffsetStyleAndLineweights(t *testing.T) {
	h, _ := plotRig(t, nil)
	h.ctrl("p")
	h.plotShown()
	p := h.ws.plotState()
	h.clickID("plot-scale-1:2")
	h.plotShown()
	if p.opt.Scale != (plot.Scale{Paper: 1, Drawing: 2}) || p.sheet.Scale != 0.5 || p.scaleIn.Text() != "1:2" {
		t.Fatalf("scale %v sheet %v field %q", p.opt.Scale, p.sheet.Scale, p.scaleIn.Text())
	}
	// The custom field takes a typed ratio.
	h.hw.Window().RequestFocus(p.scaleIn.FocusHandle())
	p.scaleIn.SetText("")
	h.frame()
	h.typeCmd("1:4")
	h.key("enter")
	h.plotShown()
	if p.opt.Scale != (plot.Scale{Paper: 1, Drawing: 4}) || p.sheet.Scale != 0.25 || !h.ws.PlotOpen() {
		t.Fatalf("typed scale %v (field %q), sheet %v, open %v", p.opt.Scale, p.scaleIn.Text(), p.sheet.Scale, h.ws.PlotOpen())
	}
	// 2:1 makes the 220 mm plate 440 mm wide, more than landscape A4.
	h.clickID("plot-scale-2:1")
	h.plotShown()
	if !p.sheet.Overflow {
		t.Fatal("2:1 should overflow")
	}
	h.savePNG("plot", "overflow.png")
	// The body scrolls under the wheel in a short window; the buttons stay put.
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: h.find("plot-options"), DeltaY: 300})
	h.plotShown()
	if p.scroll.Offset().Y == 0 || !h.inFrame("plot-print") {
		t.Fatalf("wheel: offset %v, print shown %v", p.scroll.Offset(), h.inFrame("plot-print"))
	}
	p.scroll.ScrollToTop()
	h.frame()
	h.clickID("plot-offset")
	h.plotShown()
	h.hw.Window().RequestFocus(p.offX.FocusHandle())
	p.offX.SetText("")
	h.frame()
	h.typeCmd("5")
	h.plotShown()
	if !p.opt.NoCenter || p.opt.Offset.X != 5 {
		t.Fatalf("offset %+v nocenter %v", p.opt.Offset, p.opt.NoCenter)
	}
	h.clickID("plot-gray")
	h.clickID("plot-lineweights")
	h.plotShown()
	if p.opt.Style != plot.StyleGray || !p.opt.NoLineweights || p.sheet.Items[0].Width != plot.ThinWidth {
		t.Fatalf("style %v nolw %v", p.opt.Style, p.opt.NoLineweights)
	}
	h.clickID("plot-scale-fit")
	h.clickID("plot-center")
	h.plotShown()
	if p.opt.Scale.Fixed() || p.opt.NoCenter || p.sheet.Overflow {
		t.Fatalf("back to fit and centred: %+v", p.opt)
	}
}

func TestPlotOptionsAreSavedInTheSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	h, _ := plotRig(t, nil)
	if err := h.ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	h.ctrl("p")
	h.settle(func() bool { return !h.ws.plotState().listing })
	h.clickID("plot-paper-A3")
	h.clickID("plot-mono")
	h.clickID("plot-scale-1:50")
	h.clickID("plot-area-display")
	h.clickID("plot-printer-1")
	h.clickID("plot-copies-more")
	if err := h.ws.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	s, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := settings.Plot{Paper: "A3", Landscape: true, Style: settings.PlotMono, Scale: "1:50", Area: settings.PlotDisplay,
		Center: true, Lineweights: true, Printer: "Plotter", Copies: 2}
	if s.Plot != want {
		t.Fatalf("saved %+v, want %+v", s.Plot, want)
	}
	// A new workspace starts from them.
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	p := ws.plotState()
	if p.opt.Paper.Name != "A3" || p.opt.Style != plot.StyleMono || p.opt.Scale != (plot.Scale{Paper: 1, Drawing: 50}) ||
		p.opt.Area != plot.AreaDisplay || p.printer != "Plotter" || p.copies != 2 {
		t.Fatalf("loaded %+v printer %q copies %d", p.opt, p.printer, p.copies)
	}
}
