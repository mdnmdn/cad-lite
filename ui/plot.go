package ui

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cad-lite/filedialog"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/plot"
)

// Printer hands a PDF file to the system's print spooler under a job title; plot.Print is the real
// one and tests install a fake. It runs on a goroutine and must not touch the UI.
type Printer func(ctx context.Context, path, title string) error

// plotResult is what a print or save goroutine reports back.
type plotResult struct {
	// done and arg are the command line message (a catalog key and its argument) when the job
	// succeeded; done is "" when nothing happened (a cancelled save dialog).
	done, arg string
	err       error
}

// plotState is the plot dialog: the options the user chose (kept between openings), the preview
// of the sheet they make and the print or save job in flight.
type plotState struct {
	open bool
	// picking is set while PLOT asks for the corners of a window, with the dialog hidden.
	picking bool
	opt     plot.Options
	// display is the box the canvas showed when the dialog opened (Area Display).
	display geom.Box2
	// window is the last picked window, empty until one is picked.
	window geom.Box2
	// sheet and preview follow opt; err is why there is no sheet (nothing to plot). stale asks
	// the next frame to compose them again.
	sheet   *plot.Sheet
	preview *image.NRGBA
	version uint64
	err     error
	stale   bool
	// message is the last job's failure, shown above the buttons.
	message string
	busy    bool
	results chan plotResult
	poll    *gpui.Timer
	focus   *gpui.FocusHandle
	// focusWanted asks the next frame to focus the dialog.
	focusWanted bool
}

// plotState returns the dialog state, made on first use with A4 landscape in colour.
func (ws *Workspace) plotState() *plotState {
	if ws.plt == nil {
		a4, _ := plot.PaperByName("A4")
		ws.plt = &plotState{opt: plot.Options{Paper: a4, Landscape: true}, results: make(chan plotResult, 2), window: geom.EmptyBox()}
	}
	return ws.plt
}

// SetPrinter replaces how a plot is sent to the printer (tests install a fake); nil restores
// plot.Print.
func (ws *Workspace) SetPrinter(p Printer) { ws.printer = p }

// PlotOpen reports whether the plot dialog is showing.
func (ws *Workspace) PlotOpen() bool { return ws.plt != nil && ws.plt.open }

// plotHost is the cmdsys.PlotHost of one tab's session: PLOT reaches the dialog through it.
type plotHost struct{ ws *Workspace }

// PlotPicking reports whether the dialog asked for a window to be picked.
func (h plotHost) PlotPicking() bool { return h.ws.plt != nil && h.ws.plt.picking }

// ShowPlot opens the dialog, afresh when window is nil and no pick was in progress, or back from a
// pick with the picked window (nil when it was cancelled).
func (h plotHost) ShowPlot(window *geom.Box2) {
	ws := h.ws
	p := ws.plotState()
	switch {
	case p.picking:
		p.picking = false
		if window != nil {
			p.window = *window
			p.opt.Area = plot.AreaWindow
		}
	default:
		ws.startPlot()
	}
	p.open, p.stale, p.focusWanted, p.message = true, true, true, ""
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// startPlot takes what a fresh dialog plots from the active drawing: the selection (plotted on
// its own when there is one), the view the canvas shows, and a window only if it was picked in
// this drawing's dialog before.
func (ws *Workspace) startPlot() {
	p := ws.plotState()
	sel := ws.session.Selection().Handles()
	p.opt.Selection = append(p.opt.Selection[:0], sel...)
	p.opt.What = plot.WhatAll
	if len(sel) > 0 {
		p.opt.What = plot.WhatSelection
	}
	p.display = ws.view.Visible()
	if p.opt.Area == plot.AreaWindow {
		p.opt.Area = plot.AreaExtents
	}
	p.window = geom.EmptyBox()
}

// plotOptions is the dialog's options with the window its area needs.
func (p *plotState) plotOptions() plot.Options {
	o := p.opt
	switch o.Area {
	case plot.AreaDisplay:
		o.Window = p.display
	case plot.AreaWindow:
		o.Window = p.window
	}
	return o
}

// refreshPlot composes the sheet and its preview again when an option changed. The preview is
// rasterised so its long side is px device pixels.
func (ws *Workspace) refreshPlot(px float64) {
	p := ws.plotState()
	if !p.stale {
		return
	}
	p.stale = false
	p.sheet, p.err = plot.Compose(ws.session.Drawing(), p.plotOptions())
	p.version++
	if p.err != nil {
		w, h := p.opt.Paper.Size(p.opt.Landscape)
		blank := &plot.Sheet{Width: w, Height: h, Clip: geom.EmptyBox()}
		p.preview = blank.Raster(px / max(w, h))
		return
	}
	p.preview = p.sheet.Raster(px / max(p.sheet.Width, p.sheet.Height))
}

// setPlot changes an option and redraws the preview.
func (ws *Workspace) setPlot(w *gpui.Window, f func(o *plot.Options)) {
	p := ws.plotState()
	if p.busy {
		return
	}
	f(&p.opt)
	p.stale, p.message = true, ""
	w.Refresh()
}

// pickPlotWindow hides the dialog and runs PLOT, which asks for the window's corners and brings
// the dialog back.
func (ws *Workspace) pickPlotWindow(w *gpui.Window) {
	p := ws.plotState()
	if p.busy {
		return
	}
	p.open, p.picking = false, true
	ws.focusCommandLine(w)
	ws.run("PLOT", w)
}

// closePlot hides the dialog, keeping its options for the next time.
func (ws *Workspace) closePlot(w *gpui.Window) {
	p := ws.plotState()
	p.open, p.picking = false, false
	p.sheet, p.preview = nil, nil
	ws.focusCommandLine(w)
	w.Refresh()
}

// plotTitle is the job and PDF title: the drawing's name without its extension.
func (ws *Workspace) plotTitle() string {
	n := ws.docName()
	return strings.TrimSuffix(n, filepath.Ext(n))
}

// canPrint reports whether this platform has a print spooler to try.
func canPrint() bool { return runtime.GOOS != "js" }

// printPlot writes the sheet to a temporary PDF and sends it to the printer on a goroutine.
func (ws *Workspace) printPlot(w *gpui.Window) {
	p := ws.plotState()
	if p.busy || p.sheet == nil || !canPrint() {
		return
	}
	sheet, title, printer := p.sheet, ws.plotTitle(), ws.printer
	if printer == nil {
		printer = plot.Print
	}
	paper := p.opt.Paper.Name
	p.busy, p.message = true, i18n.T("ui.plot.printing")
	go func() {
		err := func() error {
			f, err := os.CreateTemp("", "cad-lite-plot-*.pdf")
			if err != nil {
				return err
			}
			path := f.Name()
			err = sheet.WritePDF(f, title)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err == nil {
				err = printer(context.Background(), path, title)
			}
			// lp and lpr copy the file into the spool before they return; Windows hands it to the
			// PDF program, which opens it later, so the file stays in the temporary folder there.
			if runtime.GOOS != "windows" || err != nil {
				os.Remove(path)
			}
			return err
		}()
		if errors.Is(err, plot.ErrNoPrinting) {
			err = errors.New(i18n.T("ui.plot.no_printing"))
		} else if err != nil {
			err = errors.New(i18n.T("ui.plot.print_failed", err.Error()))
		}
		p.results <- plotResult{done: "ui.plot.sent", arg: paper, err: err}
	}()
	ws.pollPlot(w)
	w.Refresh()
}

// savePlot asks where to save the sheet as a PDF and writes it there, on a goroutine.
func (ws *Workspace) savePlot(w *gpui.Window) {
	p := ws.plotState()
	dialog := ws.dlg.dialog
	if p.busy || p.sheet == nil || dialog == nil {
		return
	}
	sheet, title, dir := p.sheet, ws.plotTitle(), ws.dialogDir()
	p.busy, p.message = true, i18n.T("ui.plot.saving")
	go func() {
		path, err := dialog.Save(context.Background(), i18n.T("ui.plot.save_title"), dir, title+".pdf",
			[]filedialog.Filter{{Name: i18n.T("ui.plot.pdf_files"), Extensions: []string{"pdf"}}})
		if errors.Is(err, filedialog.ErrCancelled) || (err == nil && path == "") {
			p.results <- plotResult{}
			return
		}
		if err == nil {
			if !strings.EqualFold(filepath.Ext(path), ".pdf") {
				path += ".pdf"
			}
			err = writePlotPDF(sheet, path, title)
		}
		if err != nil {
			err = errors.New(i18n.T("ui.plot.save_failed", err.Error()))
		}
		p.results <- plotResult{done: "ui.plot.saved", arg: path, err: err}
	}()
	ws.pollPlot(w)
	w.Refresh()
}

func writePlotPDF(sheet *plot.Sheet, path, title string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := sheet.WritePDF(f, title); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// pollPlot moves a finished job's result to the UI: success closes the dialog and reports on the
// command line, failure stays in the dialog, a cancelled save dialog changes nothing.
func (ws *Workspace) pollPlot(w *gpui.Window) {
	p := ws.plotState()
	if p.poll != nil {
		p.poll.Cancel()
		p.poll = nil
	}
	select {
	case r := <-p.results:
		p.busy, p.message = false, ""
		switch {
		case r.err != nil:
			p.message = r.err.Error()
		case r.done != "":
			ws.session.Say(r.done, r.arg)
			ws.closePlot(w)
		}
		w.Refresh()
	default:
		if p.busy {
			p.poll = w.After(100*time.Millisecond, func(w *gpui.Window, _ *gpui.App) { ws.pollPlot(w) })
		}
	}
}
