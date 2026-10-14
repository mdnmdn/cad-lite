package gpui

import "image"

// ContentMask is the clip applied to a primitive before it is drawn: a rect,
// optionally with rounded corners. The zero value is an empty rect, which
// clips the primitive away entirely; use InfiniteContentMask to express "no
// clipping".
type ContentMask struct {
	Bounds Bounds
	// CornerRadii round the clip rect; zero radii clip to the plain rect.
	CornerRadii Corners
}

// isRounded reports whether the mask has any corner radius.
func (m ContentMask) isRounded() bool {
	c := m.CornerRadii
	return c.TopLeft > 0 || c.TopRight > 0 || c.BottomLeft > 0 || c.BottomRight > 0
}

// intersect returns the mask covering both m and o. Each corner of the
// intersection rect keeps the radius of the mask(s) contributing it, reduced
// by how far the corner lies inside that mask's own corner (exact when the
// offsets are equal on both axes, conservative otherwise).
func (m ContentMask) intersect(o ContentMask) ContentMask {
	ib := m.Bounds.Intersection(o.Bounds)
	out := ContentMask{Bounds: ib}
	if !m.isRounded() && !o.isRounded() {
		return out
	}
	l, t := ib.Origin.X, ib.Origin.Y
	r, b := ib.Max().X, ib.Max().Y
	corner := func(rad, dx, dy Pixels) Pixels { return maxPixels(0, rad-maxPixels(dx, dy)) }
	pick := func(f func(c ContentMask) Pixels) Pixels { return maxPixels(f(m), f(o)) }
	out.CornerRadii = Corners{
		TopLeft: pick(func(c ContentMask) Pixels {
			return corner(c.CornerRadii.TopLeft, l-c.Bounds.Origin.X, t-c.Bounds.Origin.Y)
		}),
		TopRight: pick(func(c ContentMask) Pixels {
			return corner(c.CornerRadii.TopRight, c.Bounds.Max().X-r, t-c.Bounds.Origin.Y)
		}),
		BottomLeft: pick(func(c ContentMask) Pixels {
			return corner(c.CornerRadii.BottomLeft, l-c.Bounds.Origin.X, c.Bounds.Max().Y-b)
		}),
		BottomRight: pick(func(c ContentMask) Pixels {
			return corner(c.CornerRadii.BottomRight, c.Bounds.Max().X-r, c.Bounds.Max().Y-b)
		}),
	}
	return out
}

// InfiniteContentMask returns a clip rect large enough to contain any
// practical primitive, which the rasterizer treats as "unclipped".
func InfiniteContentMask() ContentMask {
	return ContentMask{Bounds: NewBounds(-1e6, -1e6, 2e6, 2e6)}
}

// BackgroundKind selects how a Quad's interior is painted.
type BackgroundKind uint8

const (
	// BackgroundNone paints no interior fill.
	BackgroundNone BackgroundKind = iota
	// BackgroundSolid fills the quad's Bounds with Fill as an
	// anti-aliased rounded rectangle.
	BackgroundSolid
	// BackgroundLinearGradient fills the quad's Bounds with Gradient.
	BackgroundLinearGradient
)

// Quad is a rounded-rect draw command: an optional solid fill plus an
// optional border stroke.
type Quad struct {
	// Order is the paint order; 0 means "assign the scene's next auto order".
	Order uint32
	// Bounds is the outer rect of the quad.
	Bounds Bounds
	// ContentMask clips the quad; the zero value clips it away entirely.
	ContentMask ContentMask
	// Background selects the fill mode.
	Background BackgroundKind
	// Fill is the interior color when Background is BackgroundSolid.
	Fill Rgba
	// BorderColor is the border stroke color; transparent draws no border.
	BorderColor Rgba
	// BorderWidths are the per-edge border widths.
	BorderWidths Edges
	// CornerRadii are the rounded-corner radii.
	CornerRadii Corners
	// Gradient is the fill when Background is BackgroundLinearGradient.
	Gradient Gradient
}

// Shadow is a gaussian-blurred rounded rect. An outset shadow paints the
// blurred Bounds/CornerRadii shape; an inset shadow paints inside the
// Clip/ClipRadii shape everywhere the blurred Bounds shape does not reach.
type Shadow struct {
	// Order is the paint order; 0 means "assign the scene's next auto order".
	Order uint32
	// Bounds is the shadow shape before blurring.
	Bounds Bounds
	// CornerRadii are the shadow shape's corner radii.
	CornerRadii Corners
	// ContentMask clips the shadow; the zero value clips it away entirely.
	ContentMask ContentMask
	// Color is the shadow color.
	Color Rgba
	// BlurRadius is the CSS blur radius (gaussian sigma = BlurRadius/2).
	BlurRadius Pixels
	// Inset selects an inner shadow.
	Inset bool
	// Clip and ClipRadii are the element shape an inset shadow is drawn in.
	Clip      Bounds
	ClipRadii Corners
}

// GlyphSprite draws a grayscale alpha mask (typically a shaped glyph)
// scaled into Bounds and tinted with Color.
type GlyphSprite struct {
	// Order is the paint order; 0 means "assign the scene's next auto order".
	Order uint32
	// Bounds is the destination rect in window space.
	Bounds Bounds
	// ContentMask clips the sprite; the zero value clips it away entirely.
	ContentMask ContentMask
	// Mask is the grayscale alpha mask; nil draws nothing.
	Mask *GlyphMask
	// Color tints the mask; the mask supplies coverage, the color the tint.
	Color Rgba
	// Image, when set, is a colour picture drawn instead of Mask: its pixels
	// are scaled into Bounds (bilinear, 1:1 when the sizes match) and Color's
	// alpha multiplies them, so opaque white draws the image as is.
	Image *image.NRGBA
}

// GlyphMask is a grayscale alpha mask: 1 byte of coverage per pixel,
// row-major, stride W.
type GlyphMask struct {
	// Pix is the coverage data, row-major with stride W.
	Pix []uint8
	// W and H are the mask dimensions in pixels.
	W, H int
	// Ink bounds in font pixels (y-up) relative to the pen position. The
	// mask covers exactly this rect expanded by one pixel on every side,
	// screen-oriented: row 0 is the top of the ink. GlyphSprite.Bounds is
	// derived from these at paint time.
	MinX, MinY, MaxX, MaxY float32
	// Scale is the device scale the mask was rasterized at: W, H and Pix are
	// in device pixels. 0 means 1.
	Scale float32
	// X0 and Y0 are the mask's top-left in device pixels relative to the pen
	// position and baseline (y-down); valid when Scale > 0.
	X0, Y0 int
}

// At returns the coverage at (x, y); out-of-bounds lookups return 0.
func (m *GlyphMask) At(x, y int) uint8 {
	if m == nil || x < 0 || y < 0 || x >= m.W || y >= m.H {
		return 0
	}
	i := y*m.W + x
	if i >= len(m.Pix) {
		return 0
	}
	return m.Pix[i]
}

// Underline draws a horizontal rule along the bottom edge of Bounds.
type Underline struct {
	// Order is the paint order; 0 means "assign the scene's next auto order".
	Order uint32
	// Bounds is the full rect; the rule is drawn Thickness tall at its bottom.
	Bounds Bounds
	// ContentMask clips the underline; the zero value clips it away entirely.
	ContentMask ContentMask
	// Color is the underline color.
	Color Rgba
	// Thickness is the rule height in logical pixels.
	Thickness Pixels
}

// Scene accumulates draw primitives for one frame. The zero value is ready
// to use. Auto-ordering: primitives with Order == 0 get Order = the scene's
// next auto order, which starts at 0 and increments by 1 per primitive;
// PushLayer advances it by layerStride so everything inserted afterwards
// paints above everything inserted before.
type Scene struct {
	// Quads are the accumulated quad commands.
	Quads []Quad
	// Shadows are the accumulated shadow commands; they interleave with
	// Quads by Order.
	Shadows []Shadow
	// Sprites are the accumulated glyph sprite commands.
	Sprites []GlyphSprite
	// Underlines are the accumulated underline commands.
	Underlines []Underline
	// Strokes are the accumulated polyline stroke commands.
	Strokes []Stroke

	// points backs the scaled copies of stroke points made by scaleScene.
	points []Point

	nextOrder uint32
}

// layerStride is the auto-order gap PushLayer reserves between layers
// (1<<20 primitives may be inserted per layer before orders collide).
const layerStride uint32 = 1 << 20

// Clear removes every primitive and resets auto-ordering.
func (s *Scene) Clear() {
	s.Quads = s.Quads[:0]
	s.Shadows = s.Shadows[:0]
	s.Sprites = s.Sprites[:0]
	s.Underlines = s.Underlines[:0]
	s.Strokes = s.Strokes[:0]
	s.points = s.points[:0]
	s.nextOrder = 0
}

// InsertQuad appends a quad, assigning its auto order when Order is 0.
func (s *Scene) InsertQuad(q Quad) {
	q.Order = s.takeOrder(q.Order)
	s.Quads = append(s.Quads, q)
}

// InsertShadow appends a shadow, assigning its auto order when Order is 0.
func (s *Scene) InsertShadow(sh Shadow) {
	sh.Order = s.takeOrder(sh.Order)
	s.Shadows = append(s.Shadows, sh)
}

// InsertSprite appends a glyph sprite, assigning its auto order when Order is 0.
func (s *Scene) InsertSprite(g GlyphSprite) {
	g.Order = s.takeOrder(g.Order)
	s.Sprites = append(s.Sprites, g)
}

// InsertUnderline appends an underline, assigning its auto order when Order is 0.
func (s *Scene) InsertUnderline(u Underline) {
	u.Order = s.takeOrder(u.Order)
	s.Underlines = append(s.Underlines, u)
}

// PushLayer records a z-order boundary: every primitive inserted afterwards
// receives an auto order at least layerStride above the current counter, so
// it paints above everything inserted before the push.
func (s *Scene) PushLayer() {
	s.nextOrder += layerStride
}

// PrimitiveCount returns the total number of quads, shadows, sprites,
// underlines and strokes.
func (s *Scene) PrimitiveCount() int {
	return len(s.Quads) + len(s.Shadows) + len(s.Sprites) + len(s.Underlines) + len(s.Strokes)
}

// takeOrder resolves a primitive's order: 0 becomes the next auto order, and
// the auto counter always stays above any explicitly assigned order.
func (s *Scene) takeOrder(order uint32) uint32 {
	if order == 0 {
		order = s.nextOrder
	}
	if order >= s.nextOrder {
		s.nextOrder = order + 1
	}
	return order
}
