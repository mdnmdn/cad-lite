package gpui

// Canvas is a leaf element that paints through a callback, the port of
// GPUI's canvas(). It is sized by its style like any element (SizeFull by
// default) and calls Paint with its laid-out bounds; the callback emits
// primitives with Window.PaintQuad, PaintStroke and friends. A canvas does
// not clip: push a clip in the callback if the drawing may overflow.
type Canvas struct {
	StyledMixin
	paint func(bounds Bounds, w *Window, cx *App)
}

// NewCanvas returns a canvas that fills its parent and paints with paint.
func NewCanvas(paint func(bounds Bounds, w *Window, cx *App)) *Canvas {
	c := &Canvas{StyledMixin: NewStyledMixin(), paint: paint}
	c.SizeFull()
	return c
}

// IntoElement returns the canvas's Element adapter.
func (c *Canvas) IntoElement() Element { return canvasElement{c} }

type canvasElement struct{ c *Canvas }

func (e canvasElement) ID() (ElementID, bool) { return ElementID{}, false }

func (e canvasElement) RequestLayout(w *Window, _ *App) (LayoutID, any) {
	st := e.c.ResolvedStyle(DefaultStyle())
	return w.RegisterLayoutNode(&LayoutNode{Style: st}), nil
}

func (e canvasElement) Prepaint(*Window, *App, ElementID, Bounds, any) any { return nil }

func (e canvasElement) Paint(w *Window, cx *App, _ ElementID, b Bounds, _, _ any) {
	if e.c.paint != nil {
		e.c.paint(b, w, cx)
	}
}
