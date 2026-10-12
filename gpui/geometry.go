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

func NewBounds(x, y, w, h float32) Bounds {
	return Bounds{
		Origin: Point{X: Pixels(x), Y: Pixels(y)},
		Size:   Size{W: Pixels(w), H: Pixels(h)},
	}
}

func (b Bounds) Min() Point { return b.Origin }

func (b Bounds) Max() Point {
	return Point{X: b.Origin.X + b.Size.W, Y: b.Origin.Y + b.Size.H}
}

func (b Bounds) Width() Pixels { return b.Size.W }

func (b Bounds) Height() Pixels { return b.Size.H }

func (b Bounds) IsEmpty() bool { return b.Size.W <= 0 || b.Size.H <= 0 }

func (b Bounds) Contains(p Point) bool {
	return p.X >= b.Origin.X && p.X < b.Origin.X+b.Size.W &&
		p.Y >= b.Origin.Y && p.Y < b.Origin.Y+b.Size.H
}

func (b Bounds) ContainsBounds(o Bounds) bool {
	return o.Origin.X >= b.Origin.X && o.Origin.Y >= b.Origin.Y &&
		o.Max().X <= b.Max().X && o.Max().Y <= b.Max().Y
}

func (b Bounds) Intersects(o Bounds) bool {
	return b.Origin.X < o.Max().X && o.Origin.X < b.Max().X &&
		b.Origin.Y < o.Max().Y && o.Origin.Y < b.Max().Y
}

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

func (b Bounds) Center() Point {
	return Point{
		X: b.Origin.X + b.Size.W/2,
		Y: b.Origin.Y + b.Size.H/2,
	}
}

func (e Edges) Horizontal() Pixels { return e.Left + e.Right }

func (e Edges) Vertical() Pixels { return e.Top + e.Bottom }

func (e Edges) Sum() Pixels { return e.Horizontal() + e.Vertical() }

func (e Edges) All(v float32) Edges {
	return Edges{Top: Pixels(v), Right: Pixels(v), Bottom: Pixels(v), Left: Pixels(v)}
}

func (c Corners) Uniform(v float32) Corners {
	return Corners{
		TopLeft:     Pixels(v),
		TopRight:    Pixels(v),
		BottomLeft:  Pixels(v),
		BottomRight: Pixels(v),
	}
}

func (c Corners) Max() Pixels {
	return maxf(c.TopLeft, maxf(c.TopRight, maxf(c.BottomLeft, c.BottomRight)))
}

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
