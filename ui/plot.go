package ui

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"cad-lite/filedialog"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/plot"
	"cad-lite/settings"
)

// Printer hands a PDF file to the system's print spooler as a job; plot.Print is the real one and
// tests install a fake. It runs on a goroutine and must not touch the UI.
type Printer func(ctx context.Context, path string, job plot.Job) error

// PrinterLister lists the system's printers; plot.Printers is the real one. It runs on a goroutine
// and must not touch the UI.
type PrinterLister func(ctx context.Context) ([]plot.Printer, error)

// Viewer opens a PDF in the system's viewer; plot.OpenViewer is the real one.
type Viewer func(path string) error

// plotResult is what a print, save or viewer goroutine reports back.
type plotResult struct {
	// done and arg are the command line message (a catalog key and its argument) when the job
	// succeeded; done is "" when nothing happened (a cancelled save dialog).
	done, arg string
	err       error
}

// printerList is what the printer lookup reports back.
type printerList struct {
	printers []plot.Printer
	err      error
}

// plotState is the plot dialog: the options the user chose (kept in the settings file), the
// preview of the sheet they make, the printers and the print or save job in flight.
type plotState struct {
	open bool
	// picking is set while PLOT asks for the corners of a window, with the dialog hidden.
	picking bool
	opt     plot.Options
	// printer is the chosen queue, "" for the system default; copies is 1 to plot.MaxCopies.
	printer string
	copies  int
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
	// printers is the system's queues once listed; listing is set while the lookup runs and
	// noPrinting when the system has no spooler.
	printers   []plot.Printer
	listing    bool
	noPrinting bool
	lists      chan printerList
	// scaleIn, offX and offY are the custom scale and the offset fields.
	scaleIn, offX, offY *gpui.TextInput
	// message is the last job's failure, shown above the buttons.
	message string
	busy    bool
	results chan plotResult
	poll    *gpui.Timer
	focus   *gpui.FocusHandle
	scroll  *gpui.ScrollHandle
	// focusWanted asks the next frame to focus the dialog.
	focusWanted bool
}

// plotState returns the dialog state, made on first use from the settings' plot options.
func (ws *Workspace) plotState() *plotState {
	if ws.plt == nil {
		p := &plotState{results: make(chan plotResult, 2), lists: make(chan printerList, 1), window: geom.EmptyBox(), scroll: gpui.NewScrollHandle()}
		p.scaleIn = gpui.NewTextInput("1:250")
		p.offX, p.offY = gpui.NewTextInput("0"), gpui.NewTextInput("0")
		for _, in := range []*gpui.TextInput{p.scaleIn, p.offX, p.offY} {
			themeField(in)
		}
		ws.plt = p
		ws.loadPlotPrefs(ws.Settings().Plot)
		p.scaleIn.OnChange(func(text string, w *gpui.Window, _ *gpui.App) {
			if s, ok := plot.ParseScale(text); ok {
				ws.setPlot(w, func(o *plot.Options) { o.Scale = s })
			}
		})
		offset := func(in *gpui.TextInput, set func(o *plot.Options, v float64)) {
			in.OnChange(func(text string, w *gpui.Window, _ *gpui.App) {
				if v, ok := parseMM(text); ok {
					ws.setPlot(w, func(o *plot.Options) { set(o, v) })
				}
			})
		}
		offset(p.offX, func(o *plot.Options, v float64) { o.Offset.X = v })
		offset(p.offY, func(o *plot.Options, v float64) { o.Offset.Y = v })
	}
	return ws.plt
}

// parseMM reads an offset in millimetres within the settings' bounds; "" is 0.
func parseMM(text string) (float64, bool) {
	t := strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	if t == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || v != v || v < -settings.MaxPlotShift || v > settings.MaxPlotShift {
		return 0, false
	}
	return v, true
}

// loadPlotPrefs sets the dialog's options from the settings file.
func (ws *Workspace) loadPlotPrefs(s settings.Plot) {
	p := ws.plt
	paper, ok := plot.PaperByName(s.Paper)
	if !ok {
		paper, _ = plot.PaperByName("A4")
	}
	scale, ok := plot.ParseScale(s.Scale)
	if !ok {
		scale = plot.Fit
	}
	p.opt = plot.Options{Paper: paper, Landscape: s.Landscape, Scale: scale, NoCenter: !s.Center,
		Offset: geom.V(s.OffsetX, s.OffsetY), NoLineweights: !s.Lineweights}
	switch s.Style {
	case settings.PlotGray:
		p.opt.Style = plot.StyleGray
	case settings.PlotMono:
		p.opt.Style = plot.StyleMono
	}
	if s.Area == settings.PlotDisplay {
		p.opt.Area = plot.AreaDisplay
	}
	p.printer, p.copies = s.Printer, min(max(s.Copies, 1), plot.MaxCopies)
	if scale.Fixed() {
		p.scaleIn.SetText(scale.String())
	}
	p.offX.SetText(mm(s.OffsetX))
	p.offY.SetText(mm(s.OffsetY))
}

// savePlotPrefs writes the dialog's options to the settings file.
func (ws *Workspace) savePlotPrefs(w *gpui.Window) {
	p := ws.plt
	o := p.opt
	ws.change(w, func(s *settings.Settings) {
		pl := &s.Plot
		pl.Paper, pl.Landscape, pl.Scale = o.Paper.Name, o.Landscape, o.Scale.String()
		pl.Style = [...]string{plot.StyleColor: settings.PlotColor, plot.StyleGray: settings.PlotGray, plot.StyleMono: settings.PlotMono}[o.Style]
		switch o.Area {
		case plot.AreaExtents:
			pl.Area = settings.PlotExtents
		case plot.AreaDisplay:
			pl.Area = settings.PlotDisplay
		}
		pl.Center, pl.OffsetX, pl.OffsetY = !o.NoCenter, o.Offset.X, o.Offset.Y
		pl.Lineweights, pl.Printer, pl.Copies = !o.NoLineweights, p.printer, p.copies
	})
}

// SetPrinter replaces how a plot is sent to the printer (tests install a fake); nil restores
// plot.Print.
func (ws *Workspace) SetPrinter(p Printer) { ws.printer = p }

// SetPrinterLister replaces how the printers are listed (tests install a fake); nil restores
// plot.Printers.
func (ws *Workspace) SetPrinterLister(l PrinterLister) { ws.printerList = l }

// SetViewer replaces how a plot is opened in the PDF viewer (tests install a fake); nil restores
// plot.OpenViewer.
func (ws *Workspace) SetViewer(v Viewer) { ws.viewer = v }

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
// this drawing's dialog before. It also lists the printers again.
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
	ws.listPrinters()
}

// listPrinters looks the printers up on a goroutine; pollPlot collects the answer.
func (ws *Workspace) listPrinters() {
	p := ws.plotState()
	if p.listing || !canPrint() {
		return
	}
	lister := ws.printerList
	if lister == nil {
		lister = plot.Printers
	}
	p.listing = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ps, err := lister(ctx)
		p.lists <- printerList{ps, err}
	}()
	if ws.win != nil {
		ws.pollPlot(ws.win)
	}
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

// setPlot changes an option, redraws the preview and saves the options.
func (ws *Workspace) setPlot(w *gpui.Window, f func(o *plot.Options)) {
	p := ws.plotState()
	if p.busy {
		return
	}
	f(&p.opt)
	p.stale, p.message = true, ""
	ws.savePlotPrefs(w)
	w.Refresh()
}

// setScale picks a scale from the buttons and shows it in the custom field.
func (ws *Workspace) setScale(w *gpui.Window, s plot.Scale) {
	p := ws.plotState()
	if p.busy {
		return
	}
	if s.Fixed() {
		p.scaleIn.SetText(s.String())
	}
	ws.setPlot(w, func(o *plot.Options) { o.Scale = s })
}

// setJob changes the printer or the number of copies and saves them.
func (ws *Workspace) setJob(w *gpui.Window, printer string, copies int) {
	p := ws.plotState()
	if p.busy {
		return
	}
	p.printer, p.copies, p.message = printer, min(max(copies, 1), plot.MaxCopies), ""
	ws.savePlotPrefs(w)
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

// canPrint reports whether this platform has a print spooler or a PDF viewer to try.
func canPrint() bool { return runtime.GOOS != "js" }

// printerLabel names the printer a job goes to, for the command line.
func (p *plotState) printerLabel() string {
	if p.printer != "" {
		return p.printer
	}
	for _, q := range p.printers {
		if q.Default {
			return q.Name
		}
	}
	return i18n.T("ui.plot.printer_default")
}

// writeTempPDF writes sheet to a new file in the plot temporary folder and returns its path.
func writeTempPDF(sheet *plot.Sheet, title string) (string, error) {
	f, err := plot.TempPDF()
	if err != nil {
		return "", err
	}
	err = sheet.WritePDF(f, title)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// printPlot writes the sheet to a temporary PDF and sends it to the chosen printer on a goroutine.
// The file stays in the plot temporary folder, which TempPDF clears of day-old files: a spooler
// or a Windows PDF program may read it after the job is handed over.
func (ws *Workspace) printPlot(w *gpui.Window) {
	p := ws.plotState()
	if p.busy || p.sheet == nil || !canPrint() || p.noPrinting {
		return
	}
	sheet, printer := p.sheet, ws.printer
	if printer == nil {
		printer = plot.Print
	}
	job := plot.Job{Title: ws.plotTitle(), Printer: p.printer, Copies: p.copies}
	label := i18n.T("ui.plot.sent_to", p.opt.Paper.Name, p.printerLabel(), p.copies)
	p.busy, p.message = true, i18n.T("ui.plot.printing")
	go func() {
		path, err := writeTempPDF(sheet, job.Title)
		if err == nil {
			err = printer(context.Background(), path, job)
		}
		if errors.Is(err, plot.ErrNoPrinting) {
			err = errors.New(i18n.T("ui.plot.no_printing"))
		} else if err != nil {
			err = errors.New(i18n.T("ui.plot.print_failed", err.Error()))
		}
		p.results <- plotResult{done: "ui.plot.sent", arg: label, err: err}
	}()
	ws.pollPlot(w)
	w.Refresh()
}

// viewPlot writes the sheet to a temporary PDF and opens it in the system's PDF viewer, whose
// print command shows the operating system's print dialog.
func (ws *Workspace) viewPlot(w *gpui.Window) {
	p := ws.plotState()
	if p.busy || p.sheet == nil || !canPrint() {
		return
	}
	sheet, title, viewer := p.sheet, ws.plotTitle(), ws.viewer
	if viewer == nil {
		viewer = plot.OpenViewer
	}
	p.busy, p.message = true, i18n.T("ui.plot.opening")
	go func() {
		path, err := writeTempPDF(sheet, title)
		if err == nil {
			err = viewer(path)
		}
		if err != nil {
			err = errors.New(i18n.T("ui.plot.view_failed", err.Error()))
		}
		p.results <- plotResult{done: "ui.plot.viewed", arg: path, err: err}
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

// pollPlot moves finished work to the UI. A printer list replaces the old one (a chosen printer
// the system no longer has falls back to the default). A job's success closes the dialog and
// reports on the command line, a failure stays in the dialog, a cancelled save dialog changes
// nothing.
func (ws *Workspace) pollPlot(w *gpui.Window) {
	p := ws.plotState()
	if p.poll != nil {
		p.poll.Cancel()
		p.poll = nil
	}
	select {
	case l := <-p.lists:
		p.listing = false
		p.printers = l.printers
		p.noPrinting = errors.Is(l.err, plot.ErrNoPrinting)
		if len(l.printers) > 0 && p.printer != "" && !slices.ContainsFunc(l.printers, func(q plot.Printer) bool { return q.Name == p.printer }) {
			p.printer = ""
		}
		w.Refresh()
	default:
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
	}
	if p.busy || p.listing {
		p.poll = w.After(100*time.Millisecond, func(w *gpui.Window, _ *gpui.App) { ws.pollPlot(w) })
	}
}
