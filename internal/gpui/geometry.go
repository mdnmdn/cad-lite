package gpui

import "math"

// Pixels is a length in logical (device-independent) pixels.
type Pixels float32

// Point is a 2D position, origin top-left, +y down.
type Point struct {
	X Pixels
	Y Pixels
}

// Size is a 2D extent.
type Size struct {
	W Pixels
	H Pixels
}

// Bounds is an axis-aligned rectangle.
type Bounds struct {
	Origin Point
	Size   Size
}

// Edges is a per-edge length (padding, margin, border widths).
type Edges struct {
	Top    Pixels
	Right  Pixels
	Bottom Pixels
	Left   Pixels
}

// Corners is a per-corner radius.
type Corners struct {
	TopLeft     Pixels
	TopRight    Pixels
	BottomLeft  Pixels
	BottomRight Pixels
}

// NewBounds returns Bounds with origin (x, y) and size w by h.
func NewBounds(x, y, w, h float32) Bounds {
	return Bounds{
		Origin: Point{X: Pixels(x), Y: Pixels(y)},
		Size:   Size{W: Pixels(w), H: Pixels(h)},
	}
}

// Min returns the top-left corner.
func (b Bounds) Min() Point { return b.Origin }

// Max returns the bottom-right corner (origin plus size).
func (b Bounds) Max() Point {
	return Point{X: b.Origin.X + b.Size.W, Y: b.Origin.Y + b.Size.H}
}

// Width returns the width.
func (b Bounds) Width() Pixels { return b.Size.W }

// Height returns the height.
func (b Bounds) Height() Pixels { return b.Size.H }

// IsEmpty reports whether the width or height is not positive.
func (b Bounds) IsEmpty() bool { return b.Size.W <= 0 || b.Size.H <= 0 }

// Contains reports whether p lies inside b; the right and bottom edges are exclusive.
func (b Bounds) Contains(p Point) bool {
	return p.X >= b.Origin.X && p.X < b.Origin.X+b.Size.W &&
		p.Y >= b.Origin.Y && p.Y < b.Origin.Y+b.Size.H
}

// ContainsBounds reports whether o lies entirely inside b.
func (b Bounds) ContainsBounds(o Bounds) bool {
	return o.Origin.X >= b.Origin.X && o.Origin.Y >= b.Origin.Y &&
		o.Max().X <= b.Max().X && o.Max().Y <= b.Max().Y
}

// Intersects reports whether b and o overlap with non-zero area.
func (b Bounds) Intersects(o Bounds) bool {
	return b.Origin.X < o.Max().X && o.Origin.X < b.Max().X &&
		b.Origin.Y < o.Max().Y && o.Origin.Y < b.Max().Y
}

// Intersection returns the overlap of b and o, or the zero Bounds when they do not intersect.
func (b Bounds) Intersection(o Bounds) Bounds {
	if !b.Intersects(o) {
		return Bounds{}
	}
	minX := maxf(b.Origin.X, o.Origin.X)
	minY := maxf(b.Origin.Y, o.Origin.Y)
	maxX := minf(b.Max().X, o.Max().X)
	maxY := minf(b.Max().Y, o.Max().Y)
	return Bounds{
		Origin: Point{X: minX, Y: minY},
		Size:   Size{W: maxX - minX, H: maxY - minY},
	}
}

// Translate returns b moved by d.
func (b Bounds) Translate(d Point) Bounds {
	return Bounds{
		Origin: Point{X: b.Origin.X + d.X, Y: b.Origin.Y + d.Y},
		Size:   b.Size,
	}
}

// Inset shrinks the rectangle by the given edge lengths.
func (b Bounds) Inset(e Edges) Bounds {
	return Bounds{
		Origin: Point{X: b.Origin.X + e.Left, Y: b.Origin.Y + e.Top},
		Size: Size{
			W: maxPixels(0, b.Size.W-e.Left-e.Right),
			H: maxPixels(0, b.Size.H-e.Top-e.Bottom),
		},
	}
}

// Union returns the smallest Bounds containing both b and o; an empty operand is ignored.
func (b Bounds) Union(o Bounds) Bounds {
	if b.IsEmpty() {
		return o
	}
	if o.IsEmpty() {
		return b
	}
	minX := minf(b.Origin.X, o.Origin.X)
	minY := minf(b.Origin.Y, o.Origin.Y)
	maxX := maxf(b.Max().X, o.Max().X)
	maxY := maxf(b.Max().Y, o.Max().Y)
	return Bounds{
		Origin: Point{X: minX, Y: minY},
		Size:   Size{W: maxX - minX, H: maxY - minY},
	}
}

// Center returns the middle point of b.
func (b Bounds) Center() Point {
	return Point{
		X: b.Origin.X + b.Size.W/2,
		Y: b.Origin.Y + b.Size.H/2,
	}
}

// Horizontal returns Left + Right.
func (e Edges) Horizontal() Pixels { return e.Left + e.Right }

// Vertical returns Top + Bottom.
func (e Edges) Vertical() Pixels { return e.Top + e.Bottom }

// Sum returns the total of all four edges.
func (e Edges) Sum() Pixels { return e.Horizontal() + e.Vertical() }

// All returns Edges with every side set to v; the receiver is ignored.
func (e Edges) All(v float32) Edges {
	return Edges{Top: Pixels(v), Right: Pixels(v), Bottom: Pixels(v), Left: Pixels(v)}
}

// Uniform returns Corners with every radius set to v; the receiver is ignored.
func (c Corners) Uniform(v float32) Corners {
	return Corners{
		TopLeft:     Pixels(v),
		TopRight:    Pixels(v),
		BottomLeft:  Pixels(v),
		BottomRight: Pixels(v),
	}
}

// Max returns the largest corner radius.
func (c Corners) Max() Pixels {
	return maxf(c.TopLeft, maxf(c.TopRight, maxf(c.BottomLeft, c.BottomRight)))
}

// IsEmpty reports whether every radius is zero or negative.
func (c Corners) IsEmpty() bool { return c.Max() <= 0 }

func maxPixels(a, b Pixels) Pixels {
	if a > b {
		return a
	}
	return b
}

func minPixels(a, b Pixels) Pixels {
	if a < b {
		return a
	}
	return b
}

func minf(a, b Pixels) Pixels {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b Pixels) Pixels {
	if a > b {
		return a
	}
	return b
}

func clampf(v, lo, hi Pixels) Pixels {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func isFinite(f float32) bool { return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0) }
