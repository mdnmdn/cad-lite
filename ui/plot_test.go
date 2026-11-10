package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cad-lite/geom"
	"cad-lite/plot"
)

// fakePrinter records the PDFs it is asked to print, reading each before the job removes it.
type fakePrinter struct {
	mu     sync.Mutex
	jobs   []string
	titles []string
	err    error
}

func (f *fakePrinter) print(_ context.Context, path, title string) error {
	data, err := os.ReadFile(path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		return err
	}
	f.jobs = append(f.jobs, string(data))
	f.titles = append(f.titles, title)
	return f.err
}

func (f *fakePrinter) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.jobs) }

// plotRig is a workspace with the demo-like drawing, a fake printer and fd as its file dialog.
func plotRig(t *testing.T, fd *fakeDialog) (*sHarness, *fakePrinter) {
	t.Helper()
	h := dialogRig(t, fd)
	fp := &fakePrinter{}
	h.ws.SetPrinter(fp.print)
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
	if p.opt.Paper.Name != "A3" || p.opt.Landscape || !p.opt.Mono || p.version == v {
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
	if p.opt.Paper.Name != "A3" || !p.opt.Mono {
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
