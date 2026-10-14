package ui

import (
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// The canvas's rulers (ruler.go draws them): along the top and left edges, over the drawing, on
// by default. The view keeps the whole canvas, so the rulers change no coordinate.

// Rulers reports whether the canvas shows its rulers.
func (ws *Workspace) Rulers() bool { return !ws.canvas.noRulers }

// SetRulers shows or hides the canvas rulers.
func (ws *Workspace) SetRulers(on bool) { ws.canvas.noRulers = !on }

// toggleRulers switches the canvas rulers on or off and redraws (the status bar's RULER chip).
func (ws *Workspace) toggleRulers(w *gpui.Window) {
	ws.SetRulers(!ws.Rulers())
	if w != nil {
		w.Refresh()
	}
}

// rulerInsets returns how far the left and top rulers reach into the canvas (0, 0 without
// rulers): the overlays at the canvas's edges (viewport label, zoom buttons, UCS icon) sit
// inside them.
func (ws *Workspace) rulerInsets() (left, top float32) {
	if !ws.Rulers() || ws.view.Width <= 0 {
		return 0, 0
	}
	return float32(ws.rulerFrame().lb), rulerBand
}

// rulerFrame is the canvas rulers' layout for the current view, worked out once per view: the
// overlays, the ticks and the labels of a frame all ask for it.
func (ws *Workspace) rulerFrame() rulerFrame {
	c, v := &ws.canvas, *ws.view
	v.Saved = nil
	if !c.rulerOK || c.rulerView != v {
		c.rulerView, c.rulerFrm, c.rulerOK = v, canvasRulers().frame(v), true
	}
	return c.rulerFrm
}

// rulerItems appends the rulers' bands, ticks and cursor markers to the frame.
func (ws *Workspace) rulerItems(dl *render.DisplayList) {
	if !ws.Rulers() {
		return
	}
	var cur *geom.Vec2
	if ws.pointerOnCanvas() {
		p := ws.cursorPoint()
		cur = &p
	}
	canvasRulers().linesIn(dl, *ws.view, ws.rulerFrame(), cur)
}

// rulerLabels holds the labels of the rulers' major ticks, placed from the view as of this
// render; a frame that changes the view asks for another render.
func (ws *Workspace) rulerLabels() *gpui.Div {
	d := gpui.NewDiv().ID("canvas-rulers").Absolute().Left(0).Top(0)
	if ws.Rulers() && ws.view.Width > 0 {
		canvasRulers().labelsIn(d, ws.rulerFrame())
	}
	return d
}
