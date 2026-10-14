package geom

import "math"

// Primitives decomposes c into Lines, Circles and Arcs: a Polyline becomes
// its segments, anything else is returned as is.
func Primitives(c Curve) []Curve {
	if p, ok := c.(Polyline); ok {
		return p.Segments()
	}
	return []Curve{c}
}

// Intersect returns the points where a and b cross or touch, within tol.
// Overlapping collinear or concentric pieces report no points.
func Intersect(a, b Curve, tol float64) []Vec2 {
	var out []Vec2
	for _, pa := range Primitives(a) {
		for _, pb := range Primitives(b) {
			if !pa.Bounds().Grow(tol).Intersects(pb.Bounds().Grow(tol)) {
				continue
			}
			out = append(out, intersectPrim(pa, pb, tol)...)
		}
	}
	return out
}

func intersectPrim(a, b Curve, tol float64) []Vec2 {
	switch a := a.(type) {
	case Line:
		switch b := b.(type) {
		case Line:
			return lineLine(a, b, tol)
		default:
			return lineRound(a, b, tol)
		}
	default:
		if l, ok := b.(Line); ok {
			return lineRound(l, a, tol)
		}
		return roundRound(a, b, tol)
	}
}

// circleOf returns the supporting circle of a Circle or an Arc, and whether
// the angle t lies on the curve.
func circleOf(c Curve) (Circle, func(t float64) bool) {
	switch c := c.(type) {
	case Circle:
		return c, func(float64) bool { return true }
	case Arc:
		return Circle{c.C, c.R}, c.ContainsAngle
	}
	return Circle{}, func(float64) bool { return false }
}

func lineLine(a, b Line, tol float64) []Vec2 {
	d1, d2 := a.B.Sub(a.A), b.B.Sub(b.A)
	den := d1.Cross(d2)
	if math.Abs(den) <= 1e-12*d1.Len()*d2.Len() {
		return nil
	}
	w := b.A.Sub(a.A)
	t := w.Cross(d2) / den
	u := w.Cross(d1) / den
	et, eu := tol/math.Max(d1.Len(), 1e-300), tol/math.Max(d2.Len(), 1e-300)
	if t < -et || t > 1+et || u < -eu || u > 1+eu {
		return nil
	}
	return []Vec2{a.PointAt(t)}
}

func lineRound(l Line, c Curve, tol float64) []Vec2 {
	circ, on := circleOf(c)
	d := l.B.Sub(l.A)
	f := l.A.Sub(circ.C)
	qa, qb, qc := d.Dot(d), 2*f.Dot(d), f.Dot(f)-circ.R*circ.R
	if qa == 0 {
		return nil
	}
	disc := qb*qb - 4*qa*qc
	et := tol / math.Sqrt(qa)
	var ts []float64
	switch {
	case disc < 0:
		// A miss by less than tol counts as a tangent touch.
		t := -qb / (2 * qa)
		if math.Abs(l.PointAt(t).Dist(circ.C)-circ.R) <= tol {
			ts = append(ts, t)
		}
	case disc == 0:
		ts = append(ts, -qb/(2*qa))
	default:
		s := math.Sqrt(disc)
		ts = append(ts, (-qb-s)/(2*qa), (-qb+s)/(2*qa))
	}
	var out []Vec2
	for _, t := range ts {
		if t < -et || t > 1+et {
			continue
		}
		p := l.PointAt(t)
		if on(p.Sub(circ.C).Angle()) {
			out = append(out, p)
		}
	}
	return out
}

func roundRound(a, b Curve, tol float64) []Vec2 {
	ca, onA := circleOf(a)
	cb, onB := circleOf(b)
	d := ca.C.Dist(cb.C)
	if d <= tol || d > ca.R+cb.R+tol || d < math.Abs(ca.R-cb.R)-tol {
		return nil
	}
	x := (d*d + ca.R*ca.R - cb.R*cb.R) / (2 * d)
	h := math.Sqrt(math.Max(0, ca.R*ca.R-x*x))
	u := cb.C.Sub(ca.C).Scale(1 / d)
	m := ca.C.Add(u.Scale(x))
	pts := []Vec2{m.Add(u.Perp().Scale(h))}
	if h > tol {
		pts = append(pts, m.Sub(u.Perp().Scale(h)))
	}
	var out []Vec2
	for _, p := range pts {
		if onA(p.Sub(ca.C).Angle()) && onB(p.Sub(cb.C).Angle()) {
			out = append(out, p)
		}
	}
	return out
}

// SegmentIntersectsBox reports whether the segment a-b touches box b.
func SegmentIntersectsBox(a, c Vec2, box Box2) bool {
	if box.Contains(a) || box.Contains(c) {
		return true
	}
	if !BoxOf(a, c).Intersects(box) {
		return false
	}
	s := Line{a, c}
	corners := [4]Vec2{box.Min, {box.Max.X, box.Min.Y}, box.Max, {box.Min.X, box.Max.Y}}
	for i := range corners {
		if len(lineLine(s, Line{corners[i], corners[(i+1)%4]}, 0)) > 0 {
			return true
		}
	}
	return false
}
