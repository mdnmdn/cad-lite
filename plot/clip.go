package plot

import (
	"math"

	"cad-lite/geom"
)

// add appends it to the sheet clipped to the box around Clip that its stroke can reach: an item
// wholly outside is dropped and one that crosses the box is cut at it. Items therefore stay within
// a few millimetres of the paper whatever the scale, which keeps the PDF small and the preview
// rasteriser inside its fixed-point range.
func (s *Sheet) add(it Item) {
	if len(it.Points) == 0 {
		return
	}
	box := s.Clip.Grow(it.Width + 1)
	b := geom.BoxOf(it.Points...)
	if !b.Intersects(box) {
		return
	}
	if box.ContainsBox(b) {
		s.Items = append(s.Items, it)
		return
	}
	if it.Fill {
		if ps := clipPolygon(it.Points, box); len(ps) >= 3 {
			it.Points = ps
			s.Items = append(s.Items, it)
		}
		return
	}
	var pts []geom.Vec2
	var starts []int32
	it.runs(func(run []geom.Vec2) {
		if it.Closed && len(run) > 1 {
			run = append(run[:len(run):len(run)], run[0])
		}
		pts, starts = clipPolyline(run, box, pts, starts)
	})
	if len(pts) == 0 {
		return
	}
	it.Points, it.Starts, it.Closed = pts, starts, false
	s.Items = append(s.Items, it)
}

// clipPolyline appends the parts of the open polyline run inside box to pts, one run each, and
// records where each starts.
func clipPolyline(run []geom.Vec2, box geom.Box2, pts []geom.Vec2, starts []int32) ([]geom.Vec2, []int32) {
	if len(run) == 1 {
		if box.Contains(run[0]) {
			starts = append(starts, int32(len(pts)))
			pts = append(pts, run[0])
		}
		return pts, starts
	}
	open := false // the last point appended ends the run in progress
	for i := 1; i < len(run); i++ {
		a, b, ok := clipSegment(run[i-1], run[i], box)
		if !ok {
			open = false
			continue
		}
		if !open || pts[len(pts)-1] != a {
			starts = append(starts, int32(len(pts)))
			pts = append(pts, a)
		}
		pts = append(pts, b)
		// The run goes on only when the segment's end was not cut.
		open = b == run[i]
	}
	return pts, starts
}

// clipSegment cuts the segment a-b to box (Liang–Barsky); ok is false when none of it is inside.
func clipSegment(a, b geom.Vec2, box geom.Box2) (geom.Vec2, geom.Vec2, bool) {
	t0, t1 := 0.0, 1.0
	d := b.Sub(a)
	for _, e := range [4][2]float64{{-d.X, a.X - box.Min.X}, {d.X, box.Max.X - a.X}, {-d.Y, a.Y - box.Min.Y}, {d.Y, box.Max.Y - a.Y}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return a, b, false
			}
			continue
		}
		r := q / p
		if p < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
		if t0 > t1 {
			return a, b, false
		}
	}
	ca, cb := a, b
	if t0 > 0 {
		ca = a.Add(d.Scale(t0))
	}
	if t1 < 1 {
		cb = a.Add(d.Scale(t1))
	}
	return ca, cb, true
}

// clipPolygon cuts the polygon ps to box (Sutherland–Hodgman). Even-odd fills with holes keep
// their holes, since the box is convex and every edge is clipped alike.
func clipPolygon(ps []geom.Vec2, box geom.Box2) []geom.Vec2 {
	type edge struct {
		in  func(geom.Vec2) bool
		cut func(a, b geom.Vec2) geom.Vec2
	}
	atX := func(x float64) func(a, b geom.Vec2) geom.Vec2 {
		return func(a, b geom.Vec2) geom.Vec2 { return geom.V(x, a.Y+(b.Y-a.Y)*(x-a.X)/(b.X-a.X)) }
	}
	atY := func(y float64) func(a, b geom.Vec2) geom.Vec2 {
		return func(a, b geom.Vec2) geom.Vec2 { return geom.V(a.X+(b.X-a.X)*(y-a.Y)/(b.Y-a.Y), y) }
	}
	edges := []edge{
		{func(p geom.Vec2) bool { return p.X >= box.Min.X }, atX(box.Min.X)},
		{func(p geom.Vec2) bool { return p.X <= box.Max.X }, atX(box.Max.X)},
		{func(p geom.Vec2) bool { return p.Y >= box.Min.Y }, atY(box.Min.Y)},
		{func(p geom.Vec2) bool { return p.Y <= box.Max.Y }, atY(box.Max.Y)},
	}
	out := ps
	for _, e := range edges {
		in := out
		out = nil
		for i, p := range in {
			prev := in[(i+len(in)-1)%len(in)]
			switch {
			case e.in(p) && e.in(prev):
				out = append(out, p)
			case e.in(p):
				out = append(out, e.cut(prev, p), p)
			case e.in(prev):
				out = append(out, e.cut(prev, p))
			}
		}
		if len(out) == 0 {
			return nil
		}
	}
	return out
}
