package gpui

import (
	"image"
	"math"
	"sort"
)

// Rasterizer paints a Scene into a premultiplied RGBA byte buffer.
type Rasterizer struct {
	pix  []byte // premultiplied RGBA, 4*w*h
	w, h int
}

// NewRasterizer allocates a w×h rasterizer whose buffer starts fully
// transparent.
func NewRasterizer(w, h int) *Rasterizer {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Rasterizer{pix: make([]byte, 4*w*h), w: w, h: h}
}

// Size returns the buffer dimensions in pixels.
func (r *Rasterizer) Size() (int, int) { return r.w, r.h }

// Pixels returns the premultiplied RGBA buffer, tightly packed (4*w*h bytes).
func (r *Rasterizer) Pixels() []byte { return r.pix }

// Reset fills the buffer with the premultiplied form of clear.
func (r *Rasterizer) Reset(clear Rgba) {
	c := clear.Premultiply()
	for i := 0; i < len(r.pix); i += 4 {
		r.pix[i] = c.R
		r.pix[i+1] = c.G
		r.pix[i+2] = c.B
		r.pix[i+3] = c.A
	}
}

// Render draws every quad, then every underline, then every sprite; each
// group in ascending Order with ties broken by insertion order. This keeps
// text on top of backgrounds.
func (r *Rasterizer) Render(s *Scene) {
	if s == nil {
		return
	}
	quads := append([]Quad(nil), s.Quads...)
	sort.SliceStable(quads, func(i, j int) bool { return quads[i].Order < quads[j].Order })
	for _, q := range quads {
		r.drawQuad(q)
	}

	underlines := append([]Underline(nil), s.Underlines...)
	sort.SliceStable(underlines, func(i, j int) bool { return underlines[i].Order < underlines[j].Order })
	for _, u := range underlines {
		r.drawUnderline(u)
	}

	sprites := append([]GlyphSprite(nil), s.Sprites...)
	sort.SliceStable(sprites, func(i, j int) bool { return sprites[i].Order < sprites[j].Order })
	for _, g := range sprites {
		r.drawSprite(g)
	}
}

// Image returns a copy of the current frame as a non-premultiplied
// *image.RGBA (suitable for png.Encode). The copy is un-premultiplied; the
// rasterizer's own buffer stays premultiplied end to end.
func (r *Rasterizer) Image() *image.RGBA {
	pix := make([]byte, len(r.pix))
	for i := 0; i < len(r.pix); i += 4 {
		a := uint32(r.pix[i+3])
		if a == 0 {
			continue
		}
		if a == 255 {
			pix[i] = r.pix[i]
			pix[i+1] = r.pix[i+1]
			pix[i+2] = r.pix[i+2]
		} else {
			pix[i] = uint8((uint32(r.pix[i])*255 + a/2) / a)
			pix[i+1] = uint8((uint32(r.pix[i+1])*255 + a/2) / a)
			pix[i+2] = uint8((uint32(r.pix[i+2])*255 + a/2) / a)
		}
		pix[i+3] = r.pix[i+3]
	}
	return &image.RGBA{Pix: pix, Stride: 4 * r.w, Rect: image.Rect(0, 0, r.w, r.h)}
}

func (r *Rasterizer) drawQuad(q Quad) {
	clip := q.ContentMask.Bounds
	if clip.IsEmpty() || q.Bounds.IsEmpty() {
		return
	}
	if q.Background == BackgroundSolid && !q.Fill.IsTransparent() {
		r.fillRounded(q.Bounds, q.CornerRadii, clip, q.Fill)
	}
	bw := q.BorderWidths
	if !q.BorderColor.IsTransparent() && (bw.Top > 0 || bw.Right > 0 || bw.Bottom > 0 || bw.Left > 0) {
		r.strokeBorder(q.Bounds, q.CornerRadii, bw, clip, q.BorderColor)
	}
}

func (r *Rasterizer) drawUnderline(u Underline) {
	if u.Color.IsTransparent() {
		return
	}
	th := u.Thickness
	if th > u.Bounds.Size.H {
		th = u.Bounds.Size.H
	}
	if th <= 0 {
		return
	}
	rect := Bounds{
		Origin: Point{X: u.Bounds.Origin.X, Y: u.Bounds.Max().Y - th},
		Size:   Size{W: u.Bounds.Size.W, H: th},
	}
	r.fillRounded(rect, Corners{}, u.ContentMask.Bounds, u.Color)
}

func (r *Rasterizer) drawSprite(g GlyphSprite) {
	m := g.Mask
	if m == nil || m.W <= 0 || m.H <= 0 || len(m.Pix) == 0 {
		return
	}
	if g.Color.IsTransparent() || g.Bounds.IsEmpty() {
		return
	}
	x0, y0, x1, y1, ok := r.clipWindow(g.ContentMask.Bounds)
	if !ok {
		return
	}
	pm := g.Color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	minX, minY := float32(g.Bounds.Origin.X), float32(g.Bounds.Origin.Y)
	maxX, maxY := float32(g.Bounds.Max().X), float32(g.Bounds.Max().Y)
	sx := float32(m.W) / (maxX - minX)
	sy := float32(m.H) / (maxY - minY)
	for y := y0; y < y1; y++ {
		v := (float32(y)+0.5-minY)*sy - 0.5
		for x := x0; x < x1; x++ {
			u := (float32(x)+0.5-minX)*sx - 0.5
			cov := sampleBilinear(m, u, v) / 255
			if cov <= 0 {
				continue
			}
			r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
		}
	}
}

// fillRounded fills the rounded rect b with color, anti-aliased via signed
// distance coverage, restricting the pixel loop to clip.
func (r *Rasterizer) fillRounded(b Bounds, radii Corners, clip Bounds, color Rgba) {
	x0, y0, x1, y1, ok := r.clipWindow(clip)
	if !ok {
		return
	}
	pm := color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			cov := roundedRectCoverage(b, radii, float32(x)+0.5, py)
			if cov <= 0 {
				continue
			}
			r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
		}
	}
}

// strokeBorder draws the border as the ring between the outer rounded rect
// and the inner rounded rect (bounds inset by the border widths, corner radii
// reduced by the adjacent border widths).
func (r *Rasterizer) strokeBorder(b Bounds, radii Corners, bw Edges, clip Bounds, color Rgba) {
	inner := b.Inset(bw)
	innerRadii := Corners{
		TopLeft:     maxPixels(0, radii.TopLeft-minPixels(bw.Left, bw.Top)),
		TopRight:    maxPixels(0, radii.TopRight-minPixels(bw.Right, bw.Top)),
		BottomLeft:  maxPixels(0, radii.BottomLeft-minPixels(bw.Left, bw.Bottom)),
		BottomRight: maxPixels(0, radii.BottomRight-minPixels(bw.Right, bw.Bottom)),
	}
	x0, y0, x1, y1, ok := r.clipWindow(clip)
	if !ok {
		return
	}
	pm := color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			px := float32(x) + 0.5
			cov := roundedRectCoverage(b, radii, px, py)
			if cov <= 0 {
				continue
			}
			if !inner.IsEmpty() {
				cov -= roundedRectCoverage(inner, innerRadii, px, py)
			}
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
		}
	}
}

// clipWindow maps a clip rect to the inclusive-exclusive integer pixel range
// [x0,x1)×[y0,y1) clamped to the buffer, keeping only pixels fully inside.
func (r *Rasterizer) clipWindow(clip Bounds) (x0, y0, x1, y1 int, ok bool) {
	x0 = int(math.Ceil(float64(clip.Origin.X)))
	y0 = int(math.Ceil(float64(clip.Origin.Y)))
	x1 = int(math.Ceil(float64(clip.Max().X)))
	y1 = int(math.Ceil(float64(clip.Max().Y)))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > r.w {
		x1 = r.w
	}
	if y1 > r.h {
		y1 = r.h
	}
	return x0, y0, x1, y1, x0 < x1 && y0 < y1
}

// blend composites a premultiplied source (channels as float 0..255, sa the
// source alpha) onto the destination pixel with source-over.
func (r *Rasterizer) blend(x, y int, sr, sg, sb, sa float32) {
	i := 4*(y*r.w) + 4*x
	inv := 1 - sa/255
	r.pix[i] = uint8(clamp32(sr+float32(r.pix[i])*inv, 0, 255) + 0.5)
	r.pix[i+1] = uint8(clamp32(sg+float32(r.pix[i+1])*inv, 0, 255) + 0.5)
	r.pix[i+2] = uint8(clamp32(sb+float32(r.pix[i+2])*inv, 0, 255) + 0.5)
	r.pix[i+3] = uint8(clamp32(sa+float32(r.pix[i+3])*inv, 0, 255) + 0.5)
}

// roundedRectCoverage returns the anti-aliased coverage (0..1) of the pixel
// center (px, py) against the rounded rect b with the given per-corner radii,
// via the signed-distance rule alpha = clamp(0.5 - d, 0, 1).
func roundedRectCoverage(b Bounds, radii Corners, px, py float32) float32 {
	minX, minY := float32(b.Origin.X), float32(b.Origin.Y)
	maxX, maxY := float32(b.Max().X), float32(b.Max().Y)
	hw := (maxX - minX) / 2
	hh := (maxY - minY) / 2
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	maxR := minf32(hw, hh)
	rTL := clamp32(float32(radii.TopLeft), 0, maxR)
	rTR := clamp32(float32(radii.TopRight), 0, maxR)
	rBL := clamp32(float32(radii.BottomLeft), 0, maxR)
	rBR := clamp32(float32(radii.BottomRight), 0, maxR)
	var rad float32
	switch {
	case px < cx && py < cy:
		rad = rTL
	case px >= cx && py < cy:
		rad = rTR
	case px < cx && py >= cy:
		rad = rBL
	default:
		rad = rBR
	}
	qx := abs32(px-cx) - (hw - rad)
	qy := abs32(py-cy) - (hh - rad)
	d := math.Hypot(float64(maxf32(qx, 0)), float64(maxf32(qy, 0))) +
		float64(minf32(maxf32(qx, qy), 0)) - float64(rad)
	return clamp32(0.5-float32(d), 0, 1)
}

// sampleBilinear samples the mask at texel-space coords (u, v) — the pixel
// center of texel i is at u = i — with edge clamping.
func sampleBilinear(m *GlyphMask, u, v float32) float32 {
	fu := clamp32(u+0.5, 0, float32(m.W))
	fv := clamp32(v+0.5, 0, float32(m.H))
	x0, y0 := int(fu), int(fv)
	if x0 >= m.W {
		x0 = m.W - 1
	}
	if y0 >= m.H {
		y0 = m.H - 1
	}
	x1, y1 := x0+1, y0+1
	if x1 >= m.W {
		x1 = m.W - 1
	}
	if y1 >= m.H {
		y1 = m.H - 1
	}
	tx, ty := fu-float32(x0), fv-float32(y0)
	top := float32(m.At(x0, y0))*(1-tx) + float32(m.At(x1, y0))*tx
	bot := float32(m.At(x0, y1))*(1-tx) + float32(m.At(x1, y1))*tx
	return top*(1-ty) + bot*ty
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// scaleScene returns a copy of s with every primitive scaled from logical to
// device pixels.
func scaleScene(s *Scene, scale float32) *Scene {
	k := Pixels(scale)
	if scale == 1 {
		return s
	}
	sb := func(b Bounds) Bounds {
		return Bounds{
			Origin: Point{X: b.Origin.X * k, Y: b.Origin.Y * k},
			Size:   Size{W: b.Size.W * k, H: b.Size.H * k},
		}
	}
	sm := func(m ContentMask) ContentMask {
		if m.Bounds.Size.W >= 1e6 {
			return m
		}
		return ContentMask{Bounds: sb(m.Bounds)}
	}
	out := &Scene{}
	for _, q := range s.Quads {
		q.Bounds = sb(q.Bounds)
		q.ContentMask = sm(q.ContentMask)
		q.BorderWidths = Edges{Top: q.BorderWidths.Top * k, Right: q.BorderWidths.Right * k, Bottom: q.BorderWidths.Bottom * k, Left: q.BorderWidths.Left * k}
		q.CornerRadii = Corners{TopLeft: q.CornerRadii.TopLeft * k, TopRight: q.CornerRadii.TopRight * k, BottomLeft: q.CornerRadii.BottomLeft * k, BottomRight: q.CornerRadii.BottomRight * k}
		out.Quads = append(out.Quads, q)
	}
	for _, u := range s.Underlines {
		u.Bounds = sb(u.Bounds)
		u.ContentMask = sm(u.ContentMask)
		u.Thickness *= k
		out.Underlines = append(out.Underlines, u)
	}
	for _, g := range s.Sprites {
		g.Bounds = sb(g.Bounds)
		g.ContentMask = sm(g.ContentMask)
		out.Sprites = append(out.Sprites, g)
	}
	return out
}
