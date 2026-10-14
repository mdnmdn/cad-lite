package geom

import "math"

// OffsetLine returns l shifted sideways by d: positive d moves it to the
// left of the direction A→B, negative d to the right. A zero-length line is
// returned unchanged.
func OffsetLine(l Line, d float64) Line {
	n := l.B.Sub(l.A).Norm().Perp().Scale(d)
	return Line{l.A.Add(n), l.B.Add(n)}
}

// OffsetCircle returns the circle concentric with c whose radius is c.R+d:
// positive d grows it, negative d shrinks it. It reports false when the
// resulting radius would be zero or negative.
func OffsetCircle(c Circle, d float64) (Circle, bool) {
	r := c.R + d
	if r <= c.R*1e-9 {
		return Circle{}, false
	}
	return Circle{C: c.C, R: r}, true
}

// OffsetArc returns the arc concentric with a, with the same angles and
// radius a.R+d: positive d moves it away from the center. It reports false
// when the resulting radius would be zero or negative.
func OffsetArc(a Arc, d float64) (Arc, bool) {
	r := a.R + d
	if r <= a.R*1e-9 {
		return Arc{}, false
	}
	a.R = r
	return a, true
}

// offSeg is one polyline segment after offsetting, before its ends are
// joined to its neighbours.
type offSeg struct {
	arc  bool
	a, b Vec2 // offset ends, in travel order
	dir  Vec2 // line: the unit direction of travel
	c    Vec2 // arc: center
	r    float64
	ccw  bool
	swp  float64 // arc: sweep magnitude of the original segment
}

// supports returns the points where the infinite supports (the line or the
// full circle) of p and q meet; parallel lines and concentric circles meet
// nowhere.
func supports(p, q offSeg, eps float64) []Vec2 {
	switch {
	case !p.arc && !q.arc:
		d1, d2 := p.b.Sub(p.a), q.b.Sub(q.a)
		den := d1.Cross(d2)
		if math.Abs(den) <= 1e-12*d1.Len()*d2.Len() {
			return nil
		}
		t := q.a.Sub(p.a).Cross(d2) / den
		return []Vec2{p.a.Add(d1.Scale(t))}
	case p.arc && q.arc:
		return roundRound(Circle{p.c, p.r}, Circle{q.c, q.r}, eps)
	}
	l, a := p, q
	if p.arc {
		l, a = q, p
	}
	f := l.a.Sub(a.c)
	b := f.Dot(l.dir)
	disc := b*b - (f.Dot(f) - a.r*a.r)
	if disc < 0 {
		if disc < -1e-12*a.r*a.r {
			return nil
		}
		disc = 0
	}
	s := math.Sqrt(disc)
	return []Vec2{l.a.Add(l.dir.Scale(-b - s)), l.a.Add(l.dir.Scale(-b + s))}
}

// join returns where the offset of the segment before a vertex ends and
// where the offset of the segment after it starts: the same point when the
// extended supports meet, otherwise the two naive ends (the caller then
// bridges them with a straight segment). ok is false for anti-parallel
// neighbours, which have no sensible join.
func join(prev, next offSeg, eps float64) (e, s Vec2, ok bool) {
	if prev.b.Near(next.a, eps) {
		m := prev.b.Lerp(next.a, 0.5)
		return m, m, true
	}
	if !prev.arc && !next.arc && prev.dir.Dot(next.dir) < 0 &&
		math.Abs(prev.dir.Cross(next.dir)) <= 1e-12 {
		return prev.b, next.a, false
	}
	mid := prev.b.Lerp(next.a, 0.5)
	best, bestD, found := Vec2{}, math.Inf(1), false
	for _, x := range supports(prev, next, eps) {
		if d := x.Dist(mid); d < bestD {
			best, bestD, found = x, d, true
		}
	}
	if !found {
		return prev.b, next.a, true
	}
	return best, best, true
}

// sweepOf returns the sweep magnitude of an arc segment between its
// (joined) ends s and e, in [0, 2π).
func (g offSeg) sweepOf(s, e Vec2) float64 {
	as, ae := s.Sub(g.c).Angle(), e.Sub(g.c).Angle()
	if g.ccw {
		return NormAngle(ae - as)
	}
	return NormAngle(as - ae)
}

// OffsetPolyline returns p offset sideways by d: positive d to the left of
// the direction of travel, negative to the right. Straight neighbours are
// joined at the intersection of their offset lines (a miter); arc segments
// are offset concentrically, which changes their radius and so their bulge
// is recomputed. Segments that collapse or invert are dropped, as in
// AutoCAD. It reports false when nothing sensible remains (a radius
// reaching zero, a closed shape collapsing or flipping).
func OffsetPolyline(p Polyline, d float64) (Polyline, bool) {
	if len(p.Pts) < 2 {
		return Polyline{}, false
	}
	if d == 0 {
		out := Polyline{Pts: append([]Vec2(nil), p.Pts...), Closed: p.Closed}
		out.Bulges = append([]float64(nil), p.Bulges...)
		return out, true
	}
	bb := p.Bounds()
	w, h := bb.Size()
	eps := 1e-9 * math.Max(1, math.Max(w, h)+math.Abs(d))

	var segs []offSeg
	n := len(p.Pts)
	for i := 0; i < p.NumSegments(); i++ {
		a, b := p.Pts[i], p.Pts[(i+1)%n]
		if a.Dist(b) == 0 {
			continue
		}
		bu := p.bulge(i)
		if bu == 0 {
			dir := b.Sub(a).Norm()
			nv := dir.Perp().Scale(d)
			segs = append(segs, offSeg{a: a.Add(nv), b: b.Add(nv), dir: dir})
			continue
		}
		c, r, _, sweep := BulgeArc(a, b, bu)
		ccw := sweep > 0
		nr := r + d
		if ccw {
			nr = r - d
		}
		if nr <= r*1e-9 {
			continue
		}
		k := nr / r
		segs = append(segs, offSeg{arc: true, a: c.Add(a.Sub(c).Scale(k)), b: c.Add(b.Sub(c).Scale(k)),
			c: c, r: nr, ccw: ccw, swp: math.Abs(sweep)})
	}

	var S, E []Vec2
	for iter := 0; ; iter++ {
		m := len(segs)
		if m == 0 || (p.Closed && m < 2) {
			return Polyline{}, false
		}
		S, E = make([]Vec2, m), make([]Vec2, m)
		if !p.Closed {
			S[0], E[m-1] = segs[0].a, segs[m-1].b
		}
		for k := 0; k < m; k++ {
			if !p.Closed && k == 0 {
				continue
			}
			prev := segs[(k-1+m)%m]
			e, s, ok := join(prev, segs[k], eps)
			if !ok {
				return Polyline{}, false
			}
			E[(k-1+m)%m], S[k] = e, s
		}
		bad := -1
		for i, g := range segs {
			if g.arc {
				if sw := g.sweepOf(S[i], E[i]); sw <= 1e-12 || sw > g.swp+math.Pi {
					bad = i
				}
			} else if E[i].Sub(S[i]).Dot(g.dir) <= eps {
				bad = i
			}
			if bad >= 0 {
				break
			}
		}
		if bad < 0 {
			break
		}
		segs = append(segs[:bad], segs[bad+1:]...)
	}

	m := len(segs)
	out := Polyline{Closed: p.Closed}
	for i, g := range segs {
		out.Pts = append(out.Pts, S[i])
		bu := 0.0
		if g.arc {
			bu = math.Tan(g.sweepOf(S[i], E[i]) / 4)
			if !g.ccw {
				bu = -bu
			}
		}
		out.Bulges = append(out.Bulges, bu)
		if i < m-1 || p.Closed {
			if !E[i].Near(S[(i+1)%m], eps) {
				out.Pts = append(out.Pts, E[i])
				out.Bulges = append(out.Bulges, 0)
			}
		} else {
			out.Pts = append(out.Pts, E[i])
			out.Bulges = append(out.Bulges, 0)
		}
	}
	if p.Closed {
		a0, a1 := polyArea(p), polyArea(out)
		if a1 == 0 || (a0 < 0) != (a1 < 0) {
			return Polyline{}, false
		}
	}
	allStraight := true
	for _, b := range out.Bulges {
		if b != 0 {
			allStraight = false
		}
	}
	if allStraight {
		out.Bulges = nil
	}
	return out, true
}

// polyArea returns the signed area enclosed by a closed polyline,
// counter-clockwise positive, arcs included.
func polyArea(p Polyline) float64 {
	pts := p.Tessellate(1e-6, nil)
	a := 0.0
	for i := 0; i+1 < len(pts); i++ {
		a += pts[i].Cross(pts[i+1])
	}
	return a / 2
}
