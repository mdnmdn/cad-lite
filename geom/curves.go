package geom

import "math"

// Curve is a bounded parametric 2D curve.
type Curve interface {
	// Bounds returns the tight axis-aligned bounding box.
	Bounds() Box2
	// PointAt returns the point at parameter t.
	PointAt(t float64) Vec2
	// Tangent returns the (unnormalized) derivative direction at t.
	Tangent(t float64) Vec2
	// ParamRange returns the parameter interval [t0, t1].
	ParamRange() (t0, t1 float64)
	// ClosestParam returns the parameter of the point on the curve nearest p.
	ClosestParam(p Vec2) float64
	// Length returns the arc length.
	Length() float64
	// Transform returns the curve under the similarity transform m.
	Transform(m Mat3) Curve
	// Tessellate appends to dst a polyline approximating the curve within
	// chordTol (the maximum distance from chord to curve) and returns it.
	Tessellate(chordTol float64, dst []Vec2) []Vec2
}

// Closest returns the point on c nearest p.
func Closest(c Curve, p Vec2) Vec2 { return c.PointAt(c.ClosestParam(p)) }

// Line is the segment from A to B; t runs from 0 at A to 1 at B.
type Line struct{ A, B Vec2 }

// Bounds implements Curve.
func (l Line) Bounds() Box2 { return BoxOf(l.A, l.B) }

// PointAt implements Curve.
func (l Line) PointAt(t float64) Vec2 { return l.A.Lerp(l.B, t) }

// Tangent implements Curve.
func (l Line) Tangent(float64) Vec2 { return l.B.Sub(l.A) }

// ParamRange implements Curve.
func (l Line) ParamRange() (float64, float64) { return 0, 1 }

// ClosestParam implements Curve.
func (l Line) ClosestParam(p Vec2) float64 {
	d := l.B.Sub(l.A)
	l2 := d.Dot(d)
	if l2 == 0 {
		return 0
	}
	return math.Max(0, math.Min(1, p.Sub(l.A).Dot(d)/l2))
}

// Length implements Curve.
func (l Line) Length() float64 { return l.A.Dist(l.B) }

// Transform implements Curve.
func (l Line) Transform(m Mat3) Curve { return Line{m.Apply(l.A), m.Apply(l.B)} }

// Tessellate implements Curve.
func (l Line) Tessellate(_ float64, dst []Vec2) []Vec2 { return append(dst, l.A, l.B) }

// Mid returns the midpoint.
func (l Line) Mid() Vec2 { return l.A.Lerp(l.B, 0.5) }

// Circle is a full circle; t is the angle in radians from the +x axis.
type Circle struct {
	C Vec2
	R float64
}

// Bounds implements Curve.
func (c Circle) Bounds() Box2 {
	return Box2{Min: V(c.C.X-c.R, c.C.Y-c.R), Max: V(c.C.X+c.R, c.C.Y+c.R)}
}

// PointAt implements Curve.
func (c Circle) PointAt(t float64) Vec2 { return c.C.Polar(c.R, t) }

// Tangent implements Curve.
func (c Circle) Tangent(t float64) Vec2 { return V(-math.Sin(t), math.Cos(t)).Scale(c.R) }

// ParamRange implements Curve.
func (c Circle) ParamRange() (float64, float64) { return 0, 2 * math.Pi }

// ClosestParam implements Curve.
func (c Circle) ClosestParam(p Vec2) float64 { return NormAngle(p.Sub(c.C).Angle()) }

// Length implements Curve.
func (c Circle) Length() float64 { return 2 * math.Pi * c.R }

// Transform implements Curve.
func (c Circle) Transform(m Mat3) Curve { return Circle{m.Apply(c.C), c.R * m.ScaleFactor()} }

// Tessellate implements Curve.
func (c Circle) Tessellate(tol float64, dst []Vec2) []Vec2 {
	return tessArc(c.C, c.R, 0, 2*math.Pi, tol, dst)
}

// Arc is a counter-clockwise circular arc from angle Start to angle End
// (radians). t is the angle, running from Start to Start+Sweep().
type Arc struct {
	C          Vec2
	R          float64
	Start, End float64
}

// Sweep returns the counter-clockwise angle from Start to End in (0, 2π];
// equal angles mean a full turn.
func (a Arc) Sweep() float64 {
	s := NormAngle(a.End - a.Start)
	if s == 0 {
		s = 2 * math.Pi
	}
	return s
}

// StartPoint returns the point at Start.
func (a Arc) StartPoint() Vec2 { return a.C.Polar(a.R, a.Start) }

// EndPoint returns the point at End.
func (a Arc) EndPoint() Vec2 { return a.C.Polar(a.R, a.End) }

// MidPoint returns the point halfway along the arc.
func (a Arc) MidPoint() Vec2 { return a.C.Polar(a.R, a.Start+a.Sweep()/2) }

// ContainsAngle reports whether the direction t lies on the arc.
func (a Arc) ContainsAngle(t float64) bool {
	return NormAngle(t-a.Start) <= a.Sweep()+1e-12
}

// Bounds implements Curve.
func (a Arc) Bounds() Box2 {
	b := BoxOf(a.StartPoint(), a.EndPoint())
	for q := 0; q < 4; q++ {
		t := float64(q) * math.Pi / 2
		if a.ContainsAngle(t) {
			b = b.Extend(a.C.Polar(a.R, t))
		}
	}
	return b
}

// PointAt implements Curve.
func (a Arc) PointAt(t float64) Vec2 { return a.C.Polar(a.R, t) }

// Tangent implements Curve.
func (a Arc) Tangent(t float64) Vec2 { return V(-math.Sin(t), math.Cos(t)).Scale(a.R) }

// ParamRange implements Curve.
func (a Arc) ParamRange() (float64, float64) { return a.Start, a.Start + a.Sweep() }

// ClosestParam implements Curve.
func (a Arc) ClosestParam(p Vec2) float64 {
	d := NormAngle(p.Sub(a.C).Angle() - a.Start)
	if s := a.Sweep(); d > s {
		// Outside the arc: snap to whichever end is angularly closer.
		if d-s < 2*math.Pi-d {
			return a.Start + s
		}
		return a.Start
	}
	return a.Start + d
}

// Length implements Curve.
func (a Arc) Length() float64 { return a.R * a.Sweep() }

// Transform implements Curve. A mirroring transform reverses the arc so it
// stays counter-clockwise.
func (a Arc) Transform(m Mat3) Curve {
	c := m.Apply(a.C)
	s, e := m.Apply(a.StartPoint()), m.Apply(a.EndPoint())
	if m.Det() < 0 {
		s, e = e, s
	}
	return Arc{C: c, R: a.R * m.ScaleFactor(), Start: NormAngle(s.Sub(c).Angle()), End: NormAngle(e.Sub(c).Angle())}
}

// Tessellate implements Curve.
func (a Arc) Tessellate(tol float64, dst []Vec2) []Vec2 {
	return tessArc(a.C, a.R, a.Start, a.Sweep(), tol, dst)
}

// arcSteps returns how many chords approximate a sweep of radius r within tol.
func arcSteps(r, sweep, tol float64) int {
	sweep = math.Abs(sweep)
	if r <= 0 || sweep == 0 {
		return 1
	}
	step := math.Pi / 2
	if tol < r {
		step = math.Min(step, 2*math.Acos(1-tol/r))
	}
	n := int(math.Ceil(sweep / step))
	return max(1, min(n, 4096))
}

// tessArc appends n+1 points from angle a0 sweeping by sweep (signed).
func tessArc(c Vec2, r, a0, sweep, tol float64, dst []Vec2) []Vec2 {
	n := arcSteps(r, sweep, tol)
	for i := 0; i <= n; i++ {
		dst = append(dst, c.Polar(r, a0+sweep*float64(i)/float64(n)))
	}
	return dst
}

// Polyline is a sequence of vertices joined by straight or arc segments.
// Bulges[i] describes the segment leaving vertex i: 0 is straight, otherwise
// tan(θ/4) of the included angle θ, positive counter-clockwise (the DWG
// LWPOLYLINE convention). A nil or short Bulges means straight segments.
// t runs from 0 to NumSegments(), one unit per segment.
type Polyline struct {
	Pts    []Vec2
	Bulges []float64
	Closed bool
}

// NumSegments returns the number of segments.
func (p Polyline) NumSegments() int {
	n := len(p.Pts)
	if n < 2 {
		return 0
	}
	if p.Closed {
		return n
	}
	return n - 1
}

func (p Polyline) bulge(i int) float64 {
	if i < len(p.Bulges) {
		return p.Bulges[i]
	}
	return 0
}

// Segment returns segment i as a Line or an Arc.
func (p Polyline) Segment(i int) Curve {
	a, b := p.Pts[i], p.Pts[(i+1)%len(p.Pts)]
	bu := p.bulge(i)
	if bu == 0 || a.Near(b, 0) {
		return Line{a, b}
	}
	c, r, a0, sweep := BulgeArc(a, b, bu)
	if sweep > 0 {
		return Arc{C: c, R: r, Start: NormAngle(a0), End: NormAngle(a0 + sweep)}
	}
	return Arc{C: c, R: r, Start: NormAngle(a0 + sweep), End: NormAngle(a0)}
}

// Segments returns every segment as a Line or an Arc.
func (p Polyline) Segments() []Curve {
	out := make([]Curve, 0, p.NumSegments())
	for i := 0; i < p.NumSegments(); i++ {
		out = append(out, p.Segment(i))
	}
	return out
}

// BulgeArc returns the circle through a and b with the given bulge: its
// center, radius, the angle of a, and the signed sweep from a to b.
func BulgeArc(a, b Vec2, bulge float64) (c Vec2, r, a0, sweep float64) {
	chord := b.Sub(a)
	l := chord.Len()
	sweep = 4 * math.Atan(bulge)
	off := l / 2 * (1 - bulge*bulge) / (2 * bulge)
	c = a.Lerp(b, 0.5).Add(chord.Norm().Perp().Scale(off))
	r = c.Dist(a)
	a0 = a.Sub(c).Angle()
	return c, r, a0, sweep
}

func (p Polyline) segParam(t float64) (int, float64) {
	n := p.NumSegments()
	if n == 0 {
		return 0, 0
	}
	i := int(math.Floor(t))
	i = max(0, min(i, n-1))
	return i, t - float64(i)
}

func segPoint(s Curve, f float64) Vec2 {
	t0, t1 := s.ParamRange()
	return s.PointAt(t0 + (t1-t0)*f)
}

// Bounds implements Curve.
func (p Polyline) Bounds() Box2 {
	b := BoxOf(p.Pts...)
	for i := 0; i < p.NumSegments(); i++ {
		if p.bulge(i) != 0 {
			b = b.Union(p.Segment(i).Bounds())
		}
	}
	return b
}

// PointAt implements Curve.
func (p Polyline) PointAt(t float64) Vec2 {
	if p.NumSegments() == 0 {
		if len(p.Pts) == 1 {
			return p.Pts[0]
		}
		return Vec2{}
	}
	i, f := p.segParam(t)
	s := p.Segment(i)
	if arc, ok := s.(Arc); ok && p.bulge(i) < 0 {
		return segPoint(arc, 1-f)
	}
	return segPoint(s, f)
}

// Tangent implements Curve.
func (p Polyline) Tangent(t float64) Vec2 {
	if p.NumSegments() == 0 {
		return Vec2{}
	}
	i, f := p.segParam(t)
	s := p.Segment(i)
	t0, t1 := s.ParamRange()
	if arc, ok := s.(Arc); ok && p.bulge(i) < 0 {
		return arc.Tangent(t0 + (t1-t0)*(1-f)).Scale(-1)
	}
	return s.Tangent(t0 + (t1-t0)*f)
}

// ParamRange implements Curve.
func (p Polyline) ParamRange() (float64, float64) { return 0, float64(p.NumSegments()) }

// ClosestParam implements Curve.
func (p Polyline) ClosestParam(q Vec2) float64 {
	best, bestD := 0.0, math.Inf(1)
	for i := 0; i < p.NumSegments(); i++ {
		s := p.Segment(i)
		t := s.ClosestParam(q)
		if d := s.PointAt(t).Dist(q); d < bestD {
			t0, t1 := s.ParamRange()
			f := (t - t0) / (t1 - t0)
			if _, ok := s.(Arc); ok && p.bulge(i) < 0 {
				f = 1 - f
			}
			best, bestD = float64(i)+f, d
		}
	}
	return best
}

// Length implements Curve.
func (p Polyline) Length() float64 {
	l := 0.0
	for i := 0; i < p.NumSegments(); i++ {
		l += p.Segment(i).Length()
	}
	return l
}

// Transform implements Curve. A mirroring transform negates the bulges.
func (p Polyline) Transform(m Mat3) Curve {
	out := Polyline{Pts: make([]Vec2, len(p.Pts)), Closed: p.Closed}
	for i, v := range p.Pts {
		out.Pts[i] = m.Apply(v)
	}
	if len(p.Bulges) > 0 {
		out.Bulges = make([]float64, len(p.Bulges))
		k := 1.0
		if m.Det() < 0 {
			k = -1
		}
		for i, b := range p.Bulges {
			out.Bulges[i] = b * k
		}
	}
	return out
}

// Tessellate implements Curve. A closed polyline repeats its first point at
// the end.
func (p Polyline) Tessellate(tol float64, dst []Vec2) []Vec2 {
	if len(p.Pts) == 0 {
		return dst
	}
	dst = append(dst, p.Pts[0])
	for i := 0; i < p.NumSegments(); i++ {
		a, b := p.Pts[i], p.Pts[(i+1)%len(p.Pts)]
		if bu := p.bulge(i); bu != 0 && !a.Near(b, 0) {
			c, r, a0, sweep := BulgeArc(a, b, bu)
			n := arcSteps(r, sweep, tol)
			for k := 1; k < n; k++ {
				dst = append(dst, c.Polar(r, a0+sweep*float64(k)/float64(n)))
			}
		}
		dst = append(dst, b)
	}
	return dst
}
