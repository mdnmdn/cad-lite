package cmdsys

import "cad-lite/geom"

// PlotHost is the host's plot dialog, which the PLOT command opens and feeds a picked window.
type PlotHost interface {
	// PlotPicking reports whether the dialog asked for a plot window to be picked: PLOT then asks
	// for two corners instead of opening the dialog.
	PlotPicking() bool
	// ShowPlot shows the dialog. window is the model-space box just picked, or nil when the dialog
	// is opened afresh or the pick was cancelled.
	ShowPlot(window *geom.Box2)
}

// SetPlotHost installs the host's plot dialog. Without one PLOT reports that plotting is not
// available.
func (s *Session) SetPlotHost(h PlotHost) { s.ctx.plot = h }

// HasPlot reports whether the host installed a plot dialog.
func (c *Context) HasPlot() bool { return c.plot != nil }

// PlotPicking reports whether the host's plot dialog is waiting for a window to be picked.
func (c *Context) PlotPicking() bool { return c.plot != nil && c.plot.PlotPicking() }

// ShowPlot asks the host to show its plot dialog with window, a picked box or nil.
func (c *Context) ShowPlot(window *geom.Box2) {
	if c.plot != nil {
		c.plot.ShowPlot(window)
	}
}
