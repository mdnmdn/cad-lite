package ui

import (
	"cad-lite/doc"
	"cad-lite/render"
)

// PointerOnCanvas exposes the crosshair and coordinate gate to tests.
func (ws *Workspace) PointerOnCanvas() bool { return ws.pointerOnCanvas() }

// RenderItems returns the canvas builder's display items for h at the current view, served from
// its cache when present.
func (ws *Workspace) RenderItems(h doc.Handle) []render.DisplayItem {
	return ws.builder.Items(h, *ws.view)
}
