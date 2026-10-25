// Package gpuirender draws a render.DisplayList with the gpui library. It is
// the only rendering code that knows about gpui; everything above it is
// backend-neutral.
package gpuirender

import (
	"slices"

	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Rgba converts a render colour to a gpui colour.
func Rgba(c render.Color) gpui.Rgba { return gpui.Rgba{R: c.R, G: c.G, B: c.B, A: c.A} }

// Painter converts display lists to gpui strokes and quads for one viewport.
type Painter struct {
	// Origin is the viewport's top-left corner in window coordinates.
	Origin gpui.Point
	// View maps model space to the viewport.
	View render.View
	// Scratch, when set, is the fill scratch kept from one frame to the next; nil makes one per
	// Paint call, shared by its fills.
	Scratch *Scratch
}

// Scratch is a Painter's reusable working storage for filled items: the polygon in window
// coordinates and a scan row's crossings. The scene never keeps it, so it can serve every frame.
type Scratch struct {
	pts []gpui.Point
	xs  []gpui.Pixels
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
	sc := p.Scratch
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
		case render.ItemFill:
			if sc == nil {
				sc = &Scratch{}
			}
			buf = p.fill(w, it, buf, sc)
		case render.ItemPolyline:
			if it.Starts == nil {
				buf = p.stroke(w, it, it.Points, it.Closed, buf)
				break
			}
			// One stroke per dash, each culled on its own: a dashed entity crossing the viewport
			// emits only the dashes the viewport shows.
			grow := p.View.PixelSize() * float64(max(it.Weight, 1))
			for k, s := range it.Starts {
				e := len(it.Points)
				if k+1 < len(it.Starts) {
					e = int(it.Starts[k+1])
				}
				run := it.Points[s:e]
				if len(run) > 0 && geom.BoxOf(run...).Grow(grow).Intersects(vis) {
					buf = p.stroke(w, it, run, false, buf)
				}
			}
		}
	}
}

// stroke paints pts as one polyline in the item's colour and width.
func (p Painter) stroke(w *gpui.Window, it *render.DisplayItem, pts []geom.Vec2, closed bool, buf []gpui.Point) []gpui.Point {
	start := len(buf)
	for _, q := range pts {
		buf = append(buf, p.screen(q))
	}
	w.PaintStroke(gpui.Stroke{
		Points: buf[start:len(buf):len(buf)],
		Closed: closed,
		Width:  gpui.Pixels(it.Weight),
		Color:  Rgba(it.Color),
	})
	return buf
}

// fill paints a filled polygon as horizontal strokes one logical pixel apart, scanning it in
// window space (even-odd) over the rows the viewport shows; gpui has no polygon fill. The strokes
// are a little wider than their spacing so no seam shows between rows. sc holds the polygon and
// the crossings.
func (p Painter) fill(w *gpui.Window, it *render.DisplayItem, buf []gpui.Point, sc *Scratch) []gpui.Point {
	n := len(it.Points)
	if n < 3 {
		return buf
	}
	pts := slices.Grow(sc.pts[:0], n)[:n]
	sc.pts = pts
	top, bottom := gpui.Pixels(1e30), gpui.Pixels(-1e30)
	for i, q := range it.Points {
		pts[i] = p.screen(q)
		top, bottom = min(top, pts[i].Y), max(bottom, pts[i].Y)
	}
	top = max(top, p.Origin.Y)
	bottom = min(bottom, p.Origin.Y+gpui.Pixels(p.View.Height))
	xs := sc.xs[:0]
	defer func() { sc.xs = xs }()
	for y := floorPx(top) + 0.5; y < bottom; y++ {
		xs = xs[:0]
		for i := range n {
			a, b := pts[i], pts[(i+1)%n]
			if (a.Y <= y) != (b.Y <= y) {
				xs = append(xs, a.X+(y-a.Y)*(b.X-a.X)/(b.Y-a.Y))
			}
		}
		slices.Sort(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			start := len(buf)
			buf = append(buf, gpui.Point{X: xs[k], Y: y}, gpui.Point{X: xs[k+1], Y: y})
			w.PaintStroke(gpui.Stroke{Points: buf[start:len(buf):len(buf)], Width: fillRow, Color: Rgba(it.Color)})
		}
	}
	return buf
}

// fillRow is the width of one row of a fill's strokes, in logical pixels.
const fillRow = 1.25

func floorPx(v gpui.Pixels) gpui.Pixels {
	f := gpui.Pixels(int64(v))
	if f > v {
		f--
	}
	return f
}

func abs(v gpui.Pixels) gpui.Pixels {
	if v < 0 {
		return -v
	}
	return v
}
