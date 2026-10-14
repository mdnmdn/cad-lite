package commands

import (
	"math"

	"go-draw/doc"
	"go-draw/geom"
)

// arcSeg is a circular arc as the user draws it: from S to E around C, through Sweep radians,
// positive counter-clockwise. A clockwise arc is stored in a doc.Arc from E to S; a polyline
// carries it as a bulge. ARC and PLINE's arc mode build every method on the constructors below,
// so the two commands agree on what each option means.
type arcSeg struct {
	C    geom.Vec2
	R    float64
	S, E geom.Vec2
	// Sweep is the signed included angle, in (-2π, 2π).
	Sweep float64
}

const arcEps = 1e-9

// newArcSeg builds the arc from s around c through sweep.
func newArcSeg(s, c geom.Vec2, sweep float64) (arcSeg, bool) {
	r := s.Dist(c)
	if r <= arcEps || math.Abs(sweep) <= arcEps || math.Abs(sweep) >= 2*math.Pi-arcEps {
		return arcSeg{}, false
	}
	a0 := s.Sub(c).Angle()
	return arcSeg{C: c, R: r, S: s, E: c.Polar(r, a0+sweep), Sweep: sweep}, true
}

// segCenterEnd is the arc from s counter-clockwise around c to the ray through e.
func segCenterEnd(s, c, e geom.Vec2) (arcSeg, bool) {
	if e.Dist(c) <= arcEps {
		return arcSeg{}, false
	}
	return newArcSeg(s, c, geom.NormAngle(e.Sub(c).Angle()-s.Sub(c).Angle()))
}

// segCenterAngle is the arc from s around c through the included angle theta (positive is
// counter-clockwise, negative clockwise).
func segCenterAngle(s, c geom.Vec2, theta float64) (arcSeg, bool) { return newArcSeg(s, c, theta) }

// segCenterChord is the arc from s around c whose chord is length l: a positive length is the
// minor arc counter-clockwise, a negative one the major arc.
func segCenterChord(s, c geom.Vec2, l float64) (arcSeg, bool) {
	r := s.Dist(c)
	if r <= arcEps || math.Abs(l) > 2*r*(1+arcEps) || math.Abs(l) <= arcEps {
		return arcSeg{}, false
	}
	theta := 2 * math.Asin(math.Min(1, math.Abs(l)/(2*r)))
	if l < 0 {
		theta = 2*math.Pi - theta
	}
	return newArcSeg(s, c, theta)
}

// segTangent is the arc from s whose tangent at s points along dir and that ends at e. The
// centre is left of the direction (counter-clockwise) or right of it (clockwise). It fails when
// e lies on the tangent line, where the arc degenerates to a straight segment.
func segTangent(s geom.Vec2, dir float64, e geom.Vec2) (arcSeg, bool) {
	d := e.Sub(s)
	n := geom.V(-math.Sin(dir), math.Cos(dir))
	nd := n.Dot(d)
	if d.Len() <= arcEps || math.Abs(nd) <= 1e-12*d.Len() {
		return arcSeg{}, false
	}
	k := d.Dot(d) / (2 * nd)
	c := s.Add(n.Scale(k))
	sweep := geom.NormAngle(e.Sub(c).Angle() - s.Sub(c).Angle())
	if k < 0 {
		sweep = -geom.NormAngle(s.Sub(c).Angle() - e.Sub(c).Angle())
	}
	return newArcSeg(s, c, sweep)
}

// segChordAngle is the arc from s to e with included angle theta (positive counter-clockwise).
func segChordAngle(s, e geom.Vec2, theta float64) (arcSeg, bool) {
	chord := e.Sub(s)
	l := chord.Len()
	if l <= arcEps || math.Abs(theta) <= arcEps || math.Abs(theta) >= 2*math.Pi-arcEps {
		return arcSeg{}, false
	}
	h := l / 2 / math.Tan(theta/2)
	c := s.Lerp(e, 0.5).Add(chord.Norm().Perp().Scale(h))
	a, ok := newArcSeg(s, c, theta)
	a.E = e
	return a, ok
}

// segChordRadius is the arc of radius r from s to e: a positive radius is the minor arc
// counter-clockwise, a negative one the major arc.
func segChordRadius(s, e geom.Vec2, r float64) (arcSeg, bool) {
	chord := e.Sub(s)
	l := chord.Len()
	if l <= arcEps || math.Abs(r) <= arcEps || l/2 > math.Abs(r)*(1+arcEps) {
		return arcSeg{}, false
	}
	h := math.Sqrt(math.Max(0, r*r-l*l/4))
	if r < 0 {
		h = -h
	}
	c := s.Lerp(e, 0.5).Add(chord.Norm().Perp().Scale(h))
	a, ok := newArcSeg(s, c, geom.NormAngle(e.Sub(c).Angle()-s.Sub(c).Angle()))
	a.E = e
	return a, ok
}

// seg3 is the arc from p1 through p2 to p3, drawn in whichever direction passes p2.
func seg3(p1, p2, p3 geom.Vec2) (arcSeg, bool) {
	a, ok := doc.ArcFrom3Points(p1, p2, p3)
	if !ok {
		return arcSeg{}, false
	}
	sweep := a.Sweep()
	if p2.Sub(p1).Cross(p3.Sub(p1)) < 0 {
		sweep = -sweep
	}
	s, ok := newArcSeg(p1, a.C, sweep)
	s.E = p3
	return s, ok
}

// Arc returns the arc as the counter-clockwise curve the document stores.
func (s arcSeg) Arc() geom.Arc {
	a0 := s.S.Sub(s.C).Angle()
	if s.Sweep > 0 {
		return geom.Arc{C: s.C, R: s.R, Start: geom.NormAngle(a0), End: geom.NormAngle(a0 + s.Sweep)}
	}
	return geom.Arc{C: s.C, R: s.R, Start: geom.NormAngle(a0 + s.Sweep), End: geom.NormAngle(a0)}
}

// Entity returns the ARC entity.
func (s arcSeg) Entity() *doc.Arc {
	a := s.Arc()
	return &doc.Arc{Center: a.C, Radius: a.R, StartAngle: a.Start, EndAngle: a.End}
}

// Bulge returns the polyline bulge of the segment from S to E: tan(sweep/4), negative clockwise.
func (s arcSeg) Bulge() float64 { return math.Tan(s.Sweep / 4) }

// EndTangent returns the direction (radians) in which the arc leaves E, the way it was drawn.
func (s arcSeg) EndTangent() float64 {
	a := s.E.Sub(s.C).Angle()
	if s.Sweep > 0 {
		return a + math.Pi/2
	}
	return a - math.Pi/2
}

// leftNormal is the unit vector 90° counter-clockwise from the direction dir.
func leftNormal(dir float64) geom.Vec2 { return geom.V(-math.Sin(dir), math.Cos(dir)) }

// signedAngle returns the angle from direction a to direction b in (-π, π].
func signedAngle(a, b geom.Vec2) float64 {
	return math.Atan2(a.Cross(b), a.Dot(b))
}
