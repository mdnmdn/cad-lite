// Package gpuirender draws a render.DisplayList with the gpui library. It is
// the only rendering code that knows about gpui; everything above it is
// backend-neutral.
package gpuirender

import (
	"go-draw/geom"
	"go-draw/internal/gpui"
	"go-draw/render"
)

// Rgba converts a render colour to a gpui colour.
func Rgba(c render.Color) gpui.Rgba { return gpui.Rgba{R: c.R, G: c.G, B: c.B, A: c.A} }

// Painter converts display lists to gpui strokes and quads for one viewport.
type Painter struct {
	// Origin is the viewport's top-left corner in window coordinates.
	Origin gpui.Point
	// View maps model space to the viewport.
	View render.View
}

// screen maps a model point to window coordinates. The camera-relative
// subtraction happens in float64 inside View.ToScreen; only the small
// viewport-relative result is narrowed to float32.
func (p Painter) screen(q geom.Vec2) gpui.Point {
	x, y := p.View.ToScreen(q)
	return gpui.Point{X: p.Origin.X + gpui.Pixels(x), Y: p.Origin.Y + gpui.Pixels(y)}
}

// Paint emits every item of dl into the window's scene. Items are culled
// against the viewport; the caller pushes the clip.
func (p Painter) Paint(w *gpui.Window, dl *render.DisplayList) {
	vis := p.View.Visible()
	total := 0
	for i := range dl.Items {
		total += len(dl.Items[i].Points)
	}
	// One backing array per frame: the scene reads stroke points at raster
	// time, so they must not be reused before the frame is presented.
	buf := make([]gpui.Point, 0, total)
	for i := range dl.Items {
		it := &dl.Items[i]
		if len(it.Points) == 0 || !geom.BoxOf(it.Points...).Grow(p.View.PixelSize()*float64(max(it.Weight, 1))).Intersects(vis) {
			continue
		}
		switch it.Kind {
		case render.ItemRect:
			a, b := p.screen(it.Points[0]), p.screen(it.Points[1])
			box := gpui.Bounds{
				Origin: gpui.Point{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
				Size:   gpui.Size{W: abs(b.X - a.X), H: abs(b.Y - a.Y)},
			}
			q := gpui.Quad{Bounds: box, Background: gpui.BackgroundSolid, Fill: Rgba(it.Color)}
			if it.Outline.A > 0 {
				q.BorderColor = Rgba(it.Outline)
				q.BorderWidths = gpui.Edges{}.All(max(it.Weight, 1))
			}
			w.PaintQuad(q)
		default:
			start := len(buf)
			for _, q := range it.Points {
				buf = append(buf, p.screen(q))
			}
			w.PaintStroke(gpui.Stroke{
				Points: buf[start:len(buf):len(buf)],
				Closed: it.Closed,
				Width:  gpui.Pixels(it.Weight),
				Color:  Rgba(it.Color),
			})
		}
	}
}

func abs(v gpui.Pixels) gpui.Pixels {
	if v < 0 {
		return -v
	}
	return v
}
