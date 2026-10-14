package doc

import (
	"math"

	"cad-lite/geom"
)

// Line is a LINE entity.
type Line struct {
	EntityCommon
	Start, End geom.Vec2
}

// Class implements Object.
func (*Line) Class() string { return "LINE" }

// Clone implements Object.
func (e *Line) Clone() Object { c := *e; return &c }

// Bounds implements Entity.
func (e *Line) Bounds() geom.Box2 { return geom.BoxOf(e.Start, e.End) }

// Geometry implements Entity.
func (e *Line) Geometry() []geom.Curve { return []geom.Curve{geom.Line{A: e.Start, B: e.End}} }

// Transform implements Entity.
func (e *Line) Transform(m geom.Mat3) { e.Start, e.End = m.Apply(e.Start), m.Apply(e.End) }

// Circle is a CIRCLE entity.
type Circle struct {
	EntityCommon
	Center geom.Vec2
	Radius float64
}

// Class implements Object.
func (*Circle) Class() string { return "CIRCLE" }

// Clone implements Object.
func (e *Circle) Clone() Object { c := *e; return &c }

// Bounds implements Entity.
func (e *Circle) Bounds() geom.Box2 { return e.curve().Bounds() }

// Geometry implements Entity.
func (e *Circle) Geometry() []geom.Curve { return []geom.Curve{e.curve()} }

// Transform implements Entity.
func (e *Circle) Transform(m geom.Mat3) {
	c := e.curve().Transform(m).(geom.Circle)
	e.Center, e.Radius = c.C, c.R
}

func (e *Circle) curve() geom.Circle { return geom.Circle{C: e.Center, R: e.Radius} }

// Arc is an ARC entity: counter-clockwise from StartAngle to EndAngle, in
// radians (DXF stores degrees; the codec converts).
type Arc struct {
	EntityCommon
	Center               geom.Vec2
	Radius               float64
	StartAngle, EndAngle float64
}

// Class implements Object.
func (*Arc) Class() string { return "ARC" }

// Clone implements Object.
func (e *Arc) Clone() Object { c := *e; return &c }

// Bounds implements Entity.
func (e *Arc) Bounds() geom.Box2 { return e.curve().Bounds() }

// Geometry implements Entity.
func (e *Arc) Geometry() []geom.Curve { return []geom.Curve{e.curve()} }

// Transform implements Entity.
func (e *Arc) Transform(m geom.Mat3) {
	a := e.curve().Transform(m).(geom.Arc)
	e.Center, e.Radius, e.StartAngle, e.EndAngle = a.C, a.R, a.Start, a.End
}

func (e *Arc) curve() geom.Arc {
	return geom.Arc{C: e.Center, R: e.Radius, Start: e.StartAngle, End: e.EndAngle}
}

// LWPolyline is an LWPOLYLINE entity: vertices with bulges.
type LWPolyline struct {
	EntityCommon
	Points []geom.Vec2
	// Bulges[i] is the bulge of the segment leaving vertex i.
	Bulges []float64
	Closed bool
	// Widths[i] is the {start, end} width of the segment leaving vertex i (DXF codes 40 and 41);
	// nil means zero width everywhere and a shorter slice means zero for the missing segments.
	// The field is carried through Clone only: the DXF codec, the renderer and Transform (which
	// does not scale it) ignore it until the polyline-width work lands.
	Widths [][2]float64
}

// Class implements Object.
func (*LWPolyline) Class() string { return "LWPOLYLINE" }

// Clone implements Object.
func (e *LWPolyline) Clone() Object {
	c := *e
	c.Points = append([]geom.Vec2(nil), e.Points...)
	c.Bulges = append([]float64(nil), e.Bulges...)
	c.Widths = append([][2]float64(nil), e.Widths...)
	return &c
}

// Bounds implements Entity.
func (e *LWPolyline) Bounds() geom.Box2 { return e.curve().Bounds() }

// Geometry implements Entity.
func (e *LWPolyline) Geometry() []geom.Curve { return []geom.Curve{e.curve()} }

// Transform implements Entity.
func (e *LWPolyline) Transform(m geom.Mat3) {
	p := e.curve().Transform(m).(geom.Polyline)
	e.Points, e.Bulges = p.Pts, p.Bulges
}

func (e *LWPolyline) curve() geom.Polyline {
	return geom.Polyline{Pts: e.Points, Bulges: e.Bulges, Closed: e.Closed}
}

// ArcFrom3Points returns the arc through p1, p2 and p3 in that order, or
// false when they are collinear.
func ArcFrom3Points(p1, p2, p3 geom.Vec2) (geom.Arc, bool) {
	a, b := p2.Sub(p1), p3.Sub(p1)
	d := 2 * a.Cross(b)
	if math.Abs(d) < 1e-12*a.Len()*b.Len() || d == 0 {
		return geom.Arc{}, false
	}
	al, bl := a.Dot(a), b.Dot(b)
	c := p1.Add(geom.V((b.Y*al-a.Y*bl)/d, (a.X*bl-b.X*al)/d))
	s, e := p1.Sub(c).Angle(), p3.Sub(c).Angle()
	if a.Cross(b) < 0 {
		// Clockwise through p2: store it counter-clockwise from p3 to p1.
		s, e = e, s
	}
	return geom.Arc{C: c, R: c.Dist(p1), Start: geom.NormAngle(s), End: geom.NormAngle(e)}, true
}
