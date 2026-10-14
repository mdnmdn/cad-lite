package geom

import "math"

// tangentEps is the relative tolerance of the tangent solvers.
const tangentEps = 1e-9

// round returns the circle a Circle or an Arc lies on.
func round(c Curve) (Circle, bool) {
	switch v := c.(type) {
	case Circle:
		return v, true
	case Arc:
		return Circle{C: v.C, R: v.R}, true
	}
	return Circle{}, false
}

// tangentKind reports whether c is a line (as an infinite line) or a round curve (as its full
// circle); any other curve has no tangent circles.
func tangentKind(c Curve) (l Line, circ Circle, isLine, ok bool) {
	switch v := c.(type) {
	case Line:
		return v, Circle{}, true, v.A.Dist(v.B) > 0
	case Circle, Arc:
		circ, ok = round(c)
		return Line{}, circ, false, ok && circ.R > 0
	}
	return Line{}, Circle{}, false, false
}

// TangentCircles returns every circle of radius r tangent to curves a and b, each taken as its
// full extension: a Line is the infinite line, a Circle or an Arc is the whole circle. The
// result holds up to four circles (line and line, line and circle), up to eight (two circles)
// and none when the curves leave no room for one, when r is not positive, or when a curve is of
// another type. Two parallel lines 2r apart have infinitely many solutions; the result is empty
// and NearestTangentCircle picks one.
func TangentCircles(a, b Curve, r float64) []Circle {
	if !(r > 0) {
		return nil
	}
	la, ca, aLine, okA := tangentKind(a)
	lb, cb, bLine, okB := tangentKind(b)
	if !okA || !okB {
		return nil
	}
	var centers []Vec2
	switch {
	case aLine && bLine:
		centers = lineLineCenters(la, lb, r)
	case aLine:
		centers = lineRoundCenters(la, cb, r)
	case bLine:
		centers = lineRoundCenters(lb, ca, r)
	default:
		centers = roundRoundCenters(ca, cb, r)
	}
	var out []Circle
	for _, c := range dedupe(centers) {
		out = append(out, Circle{C: c, R: r})
	}
	return out
}

// dedupe removes points closer than the tangent tolerance to an earlier one.
func dedupe(ps []Vec2) []Vec2 {
	var out []Vec2
	for _, p := range ps {
		dup := false
		for _, q := range out {
			if p.Dist(q) <= tangentEps*(1+p.Len()) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// offsetLines returns the two lines parallel to l at distance r, as a point and a direction.
func offsetLines(l Line, r float64) (pts [2]Vec2, dir Vec2) {
	dir = l.B.Sub(l.A).Norm()
	n := dir.Perp().Scale(r)
	return [2]Vec2{l.A.Add(n), l.A.Sub(n)}, dir
}

func lineLineCenters(l1, l2 Line, r float64) []Vec2 {
	p1, d1 := offsetLines(l1, r)
	p2, d2 := offsetLines(l2, r)
	den := d1.Cross(d2)
	if math.Abs(den) <= 1e-12 {
		return nil
	}
	var out []Vec2
	for _, a := range p1 {
		for _, b := range p2 {
			t := b.Sub(a).Cross(d2) / den
			out = append(out, a.Add(d1.Scale(t)))
		}
	}
	return out
}

func lineRoundCenters(l Line, c Circle, r float64) []Vec2 {
	ps, d := offsetLines(l, r)
	var out []Vec2
	for _, p := range ps {
		for _, rad := range []float64{c.R + r, math.Abs(c.R - r)} {
			out = append(out, lineCircleHits(p, d, Circle{C: c.C, R: rad})...)
		}
	}
	return out
}

// lineCircleHits intersects the infinite line through p with unit direction d and circle c.
func lineCircleHits(p, d Vec2, c Circle) []Vec2 {
	if c.R <= 0 {
		return nil
	}
	f := p.Sub(c.C)
	b := f.Dot(d)
	disc := b*b - (f.Dot(f) - c.R*c.R)
	tol := tangentEps * c.R * c.R
	if disc < -tol {
		return nil
	}
	s := math.Sqrt(math.Max(0, disc))
	return []Vec2{p.Add(d.Scale(-b + s)), p.Add(d.Scale(-b - s))}
}

func roundRoundCenters(c1, c2 Circle, r float64) []Vec2 {
	var out []Vec2
	for _, d1 := range []float64{c1.R + r, math.Abs(c1.R - r)} {
		for _, d2 := range []float64{c2.R + r, math.Abs(c2.R - r)} {
			out = append(out, circleCircleHits(Circle{C: c1.C, R: d1}, Circle{C: c2.C, R: d2})...)
		}
	}
	return out
}

func circleCircleHits(a, b Circle) []Vec2 {
	d := a.C.Dist(b.C)
	if d == 0 || a.R <= 0 || b.R <= 0 {
		return nil
	}
	tol := tangentEps * (1 + a.R + b.R)
	if d > a.R+b.R+tol || d < math.Abs(a.R-b.R)-tol {
		return nil
	}
	x := (d*d + a.R*a.R - b.R*b.R) / (2 * d)
	h := math.Sqrt(math.Max(0, a.R*a.R-x*x))
	u := b.C.Sub(a.C).Scale(1 / d)
	m := a.C.Add(u.Scale(x))
	return []Vec2{m.Add(u.Perp().Scale(h)), m.Sub(u.Perp().Scale(h))}
}

// TangentPoint returns the point where circle t (a tangent circle from TangentCircles) touches
// curve c, which must be a Line, a Circle or an Arc taken at full extension.
func TangentPoint(c Curve, t Circle) Vec2 {
	if l, ok := c.(Line); ok {
		return projectOnto(l, t.C)
	}
	o, ok := round(c)
	if !ok {
		return t.C
	}
	u := t.C.Sub(o.C).Norm()
	if t.R > o.R && math.Abs(t.C.Dist(o.C)-(t.R-o.R)) <= tangentEps*(1+t.R) {
		// The tangent circle encloses the curve: they touch on the far side.
		u = u.Scale(-1)
	}
	return o.C.Add(u.Scale(o.R))
}

// projectOnto returns the foot of the perpendicular from p to the infinite line l.
func projectOnto(l Line, p Vec2) Vec2 {
	d := l.B.Sub(l.A).Norm()
	return l.A.Add(d.Scale(p.Sub(l.A).Dot(d)))
}

// NearestTangentCircle returns the circle of radius r tangent to a and b whose points of
// tangency lie nearest the pick points pa (on a) and pb (on b), which is the solution AutoCAD's
// CIRCLE Ttr chooses. It also resolves the infinite family of two parallel lines 2r apart by
// centring the circle opposite the mean of the picks. ok is false when no circle exists.
func NearestTangentCircle(a, b Curve, pa, pb Vec2, r float64) (Circle, bool) {
	sols := TangentCircles(a, b, r)
	if len(sols) == 0 {
		if c, ok := parallelTangent(a, b, pa, pb, r); ok {
			return c, true
		}
		return Circle{}, false
	}
	best, bestD := sols[0], math.Inf(1)
	for _, s := range sols {
		d := TangentPoint(a, s).Dist(pa) + TangentPoint(b, s).Dist(pb)
		if d < bestD {
			best, bestD = s, d
		}
	}
	return best, true
}

func parallelTangent(a, b Curve, pa, pb Vec2, r float64) (Circle, bool) {
	la, okA := a.(Line)
	lb, okB := b.(Line)
	if !okA || !okB || !(r > 0) {
		return Circle{}, false
	}
	d := la.B.Sub(la.A).Norm()
	if d == (Vec2{}) || math.Abs(d.Cross(lb.B.Sub(lb.A).Norm())) > 1e-9 {
		return Circle{}, false
	}
	n := d.Perp()
	gap := lb.A.Sub(la.A).Dot(n)
	if math.Abs(math.Abs(gap)-2*r) > tangentEps*(1+r) {
		return Circle{}, false
	}
	mid := la.A.Add(n.Scale(gap / 2))
	along := pa.Add(pb).Scale(0.5).Sub(la.A).Dot(d)
	return Circle{C: mid.Add(d.Scale(along - mid.Sub(la.A).Dot(d))), R: r}, true
}
