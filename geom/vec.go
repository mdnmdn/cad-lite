// Package geom is the geometry kernel: double-precision 2D vectors, affine
// transforms, boxes and curves. It is pure Go with no dependencies outside
// the standard library and knows nothing about documents or rendering.
package geom

import "math"

// Vec2 is a point or vector in model space.
type Vec2 struct{ X, Y float64 }

// V returns the vector (x, y).
func V(x, y float64) Vec2 { return Vec2{x, y} }

// Add returns v + o.
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{v.X + o.X, v.Y + o.Y} }

// Sub returns v - o.
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{v.X - o.X, v.Y - o.Y} }

// Scale returns v * k.
func (v Vec2) Scale(k float64) Vec2 { return Vec2{v.X * k, v.Y * k} }

// Dot returns the dot product of v and o.
func (v Vec2) Dot(o Vec2) float64 { return v.X*o.X + v.Y*o.Y }

// Cross returns the z component of the 3D cross product of v and o.
func (v Vec2) Cross(o Vec2) float64 { return v.X*o.Y - v.Y*o.X }

// Len returns the Euclidean length of v.
func (v Vec2) Len() float64 { return math.Hypot(v.X, v.Y) }

// Dist returns the distance between v and o.
func (v Vec2) Dist(o Vec2) float64 { return o.Sub(v).Len() }

// Norm returns v scaled to unit length, or the zero vector for a zero v.
func (v Vec2) Norm() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return v.Scale(1 / l)
}

// Perp returns v rotated by +90 degrees.
func (v Vec2) Perp() Vec2 { return Vec2{-v.Y, v.X} }

// Angle returns the direction of v in radians, in (-π, π].
func (v Vec2) Angle() float64 { return math.Atan2(v.Y, v.X) }

// Lerp returns the point a fraction t of the way from v to o.
func (v Vec2) Lerp(o Vec2, t float64) Vec2 { return v.Add(o.Sub(v).Scale(t)) }

// Polar returns the point at distance d and angle a (radians) from v.
func (v Vec2) Polar(d, a float64) Vec2 {
	return Vec2{v.X + d*math.Cos(a), v.Y + d*math.Sin(a)}
}

// Near reports whether v and o are within tol of each other.
func (v Vec2) Near(o Vec2, tol float64) bool { return v.Dist(o) <= tol }

// Box2 is an axis-aligned box. The zero value is not empty; use EmptyBox
// for an accumulator.
type Box2 struct{ Min, Max Vec2 }

// EmptyBox returns a box that contains nothing and grows with Extend.
func EmptyBox() Box2 {
	inf := math.Inf(1)
	return Box2{Min: Vec2{inf, inf}, Max: Vec2{-inf, -inf}}
}

// BoxOf returns the smallest box containing every point in ps.
func BoxOf(ps ...Vec2) Box2 {
	b := EmptyBox()
	for _, p := range ps {
		b = b.Extend(p)
	}
	return b
}

// IsEmpty reports whether b contains no point.
func (b Box2) IsEmpty() bool { return b.Min.X > b.Max.X || b.Min.Y > b.Max.Y }

// Extend returns b grown to contain p.
func (b Box2) Extend(p Vec2) Box2 {
	return Box2{
		Min: Vec2{math.Min(b.Min.X, p.X), math.Min(b.Min.Y, p.Y)},
		Max: Vec2{math.Max(b.Max.X, p.X), math.Max(b.Max.Y, p.Y)},
	}
}

// Union returns the smallest box containing b and o.
func (b Box2) Union(o Box2) Box2 {
	if o.IsEmpty() {
		return b
	}
	if b.IsEmpty() {
		return o
	}
	return b.Extend(o.Min).Extend(o.Max)
}

// Grow returns b expanded by d on every side.
func (b Box2) Grow(d float64) Box2 {
	return Box2{Min: Vec2{b.Min.X - d, b.Min.Y - d}, Max: Vec2{b.Max.X + d, b.Max.Y + d}}
}

// Intersects reports whether b and o overlap (touching counts).
func (b Box2) Intersects(o Box2) bool {
	return !b.IsEmpty() && !o.IsEmpty() &&
		b.Min.X <= o.Max.X && o.Min.X <= b.Max.X && b.Min.Y <= o.Max.Y && o.Min.Y <= b.Max.Y
}

// Contains reports whether p lies inside b or on its edge.
func (b Box2) Contains(p Vec2) bool {
	return p.X >= b.Min.X && p.X <= b.Max.X && p.Y >= b.Min.Y && p.Y <= b.Max.Y
}

// ContainsBox reports whether o lies entirely inside b.
func (b Box2) ContainsBox(o Box2) bool {
	return !o.IsEmpty() && b.Contains(o.Min) && b.Contains(o.Max)
}

// Center returns the midpoint of b.
func (b Box2) Center() Vec2 { return b.Min.Lerp(b.Max, 0.5) }

// Size returns the width and height of b.
func (b Box2) Size() (w, h float64) { return b.Max.X - b.Min.X, b.Max.Y - b.Min.Y }

// Mat3 is a 2D affine transform [a b c d e f] mapping (x, y) to
// (a·x + c·y + e, b·x + d·y + f), the SVG/PDF matrix convention.
type Mat3 [6]float64

// Identity returns the identity transform.
func Identity() Mat3 { return Mat3{1, 0, 0, 1, 0, 0} }

// Translate returns a translation by v.
func Translate(v Vec2) Mat3 { return Mat3{1, 0, 0, 1, v.X, v.Y} }

// Rotate returns a rotation by a radians about the point c.
func Rotate(c Vec2, a float64) Mat3 {
	s, k := math.Sincos(a)
	return Translate(c).Mul(Mat3{k, s, -s, k, 0, 0}).Mul(Translate(c.Scale(-1)))
}

// ScaleAbout returns a uniform scale by f about the point c.
func ScaleAbout(c Vec2, f float64) Mat3 {
	return Translate(c).Mul(Mat3{f, 0, 0, f, 0, 0}).Mul(Translate(c.Scale(-1)))
}

// Mul returns m·o: the transform applying o first, then m.
func (m Mat3) Mul(o Mat3) Mat3 {
	return Mat3{
		m[0]*o[0] + m[2]*o[1],
		m[1]*o[0] + m[3]*o[1],
		m[0]*o[2] + m[2]*o[3],
		m[1]*o[2] + m[3]*o[3],
		m[0]*o[4] + m[2]*o[5] + m[4],
		m[1]*o[4] + m[3]*o[5] + m[5],
	}
}

// Apply transforms the point p.
func (m Mat3) Apply(p Vec2) Vec2 {
	return Vec2{m[0]*p.X + m[2]*p.Y + m[4], m[1]*p.X + m[3]*p.Y + m[5]}
}

// ApplyVec transforms the direction v (no translation).
func (m Mat3) ApplyVec(v Vec2) Vec2 { return Vec2{m[0]*v.X + m[2]*v.Y, m[1]*v.X + m[3]*v.Y} }

// Det returns the determinant of the linear part.
func (m Mat3) Det() float64 { return m[0]*m[3] - m[1]*m[2] }

// ScaleFactor returns the uniform scale a similarity transform applies,
// sqrt(|det|).
func (m Mat3) ScaleFactor() float64 { return math.Sqrt(math.Abs(m.Det())) }

// RotationAngle returns the rotation of the x axis under m, in radians.
func (m Mat3) RotationAngle() float64 { return math.Atan2(m[1], m[0]) }
