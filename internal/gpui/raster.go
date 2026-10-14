package gpui

import (
	"cmp"
	"image"
	"math"
	"slices"
)

// Rasterizer paints a Scene into a premultiplied RGBA byte buffer.
type Rasterizer struct {
	pix  []byte // premultiplied RGBA, 4*w*h
	w, h int

	// scratch slices reused across Render calls for the sorted primitives.
	quads      []Quad
	shadows    []Shadow
	underlines []Underline
	sprites    []GlyphSprite
	strokes    []Stroke

	// stroke accumulates the coverage of the stroke being drawn.
	stroke strokeBuf

	// lut and rowBuf are scratch space for gradient and shadow fills.
	lut    gradientLUT
	rowBuf []float32

	// masked is set while drawing a primitive whose ContentMask is rounded;
	// mask is that rounded rect and band its fully-covered cross region.
	masked bool
	mask   rrect
	band   maskBand
}

// maskBand is the part of a rounded mask where coverage is exactly 1 for
// any pixel inside the mask's rect: the horizontal band between the corner
// rows and the vertical band between the corner columns.
type maskBand struct {
	top, bottom, left, right float32
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
	if len(r.pix) < 4 {
		return
	}
	c := clear.Premultiply()
	r.pix[0], r.pix[1], r.pix[2], r.pix[3] = c.R, c.G, c.B, c.A
	for n := 4; n < len(r.pix); n *= 2 {
		copy(r.pix[n:], r.pix[:n])
	}
}

// Render draws every primitive in scene order: each kind is stably sorted by
// Order and the five lists are merged, so a later background covers earlier
// text (GPUI paints by draw order, not by primitive kind). On equal Order a
// shadow paints before a quad, a quad before a stroke, a stroke before an
// underline, an underline before a sprite.
func (r *Rasterizer) Render(s *Scene) {
	if s == nil {
		return
	}
	r.quads = sortByOrder(append(r.quads[:0], s.Quads...), func(q Quad) uint32 { return q.Order })
	r.shadows = sortByOrder(append(r.shadows[:0], s.Shadows...), func(sh Shadow) uint32 { return sh.Order })
	r.underlines = sortByOrder(append(r.underlines[:0], s.Underlines...), func(u Underline) uint32 { return u.Order })
	r.sprites = sortByOrder(append(r.sprites[:0], s.Sprites...), func(g GlyphSprite) uint32 { return g.Order })
	r.strokes = sortByOrder(append(r.strokes[:0], s.Strokes...), func(st Stroke) uint32 { return st.Order })
	qs, shs, us, gs, sts := r.quads, r.shadows, r.underlines, r.sprites, r.strokes
	var qi, si, ui, gi, ti int
	for {
		const none = math.MaxUint32 + 1
		next, kind := uint64(none), -1
		if si < len(shs) {
			next, kind = uint64(shs[si].Order), 0
		}
		if qi < len(qs) && uint64(qs[qi].Order) < next {
			next, kind = uint64(qs[qi].Order), 1
		}
		if ti < len(sts) && uint64(sts[ti].Order) < next {
			next, kind = uint64(sts[ti].Order), 4
		}
		if ui < len(us) && uint64(us[ui].Order) < next {
			next, kind = uint64(us[ui].Order), 2
		}
		if gi < len(gs) && uint64(gs[gi].Order) < next {
			kind = 3
		}
		switch kind {
		case 0:
			r.setMask(shs[si].ContentMask)
			r.drawShadow(shs[si])
			si++
		case 1:
			r.setMask(qs[qi].ContentMask)
			r.drawQuad(qs[qi])
			qi++
		case 2:
			r.setMask(us[ui].ContentMask)
			r.drawUnderline(us[ui])
			ui++
		case 3:
			r.setMask(gs[gi].ContentMask)
			r.drawSprite(gs[gi])
			gi++
		case 4:
			r.setMask(sts[ti].ContentMask)
			r.drawStroke(sts[ti])
			ti++
		default:
			r.masked = false
			return
		}
	}
}

// setMask prepares the rounded content mask for the next primitive; a mask
// without radii leaves blending unmasked (the clip rect is applied by the
// pixel windows).
func (r *Rasterizer) setMask(m ContentMask) {
	r.masked = m.isRounded()
	if !r.masked {
		return
	}
	r.mask = newRRect(m.Bounds, m.CornerRadii)
	k := r.mask
	r.band = maskBand{
		top:    k.cy - k.hh + maxf32(k.rTL, k.rTR),
		bottom: k.cy + k.hh - maxf32(k.rBL, k.rBR),
		left:   k.cx - k.hw + maxf32(k.rTL, k.rBL),
		right:  k.cx + k.hw - maxf32(k.rTR, k.rBR),
	}
}

// maskCoverage is the rounded mask's coverage of pixel (x, y), which the
// caller guarantees lies inside the mask's rect; only the corner squares
// evaluate the SDF.
func (r *Rasterizer) maskCoverage(x, y int) float32 {
	px, py := float32(x)+0.5, float32(y)+0.5
	if (py >= r.band.top && py <= r.band.bottom) || (px >= r.band.left && px <= r.band.right) {
		return 1
	}
	return r.mask.coverage(px, py)
}

// sortByOrder stably sorts v by order, skipping the sort when v is already
// in order (the common case: primitives are inserted in paint order).
func sortByOrder[T any](v []T, order func(T) uint32) []T {
	byOrder := func(a, b T) int { return cmp.Compare(order(a), order(b)) }
	if !slices.IsSortedFunc(v, byOrder) {
		slices.SortStableFunc(v, byOrder)
	}
	return v
}

// Image returns a copy of the current frame as a non-premultiplied
// *image.NRGBA (suitable for png.Encode). The copy is un-premultiplied; the
// rasterizer's own buffer stays premultiplied end to end.
func (r *Rasterizer) Image() *image.NRGBA {
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
	return &image.NRGBA{Pix: pix, Stride: 4 * r.w, Rect: image.Rect(0, 0, r.w, r.h)}
}

func (r *Rasterizer) drawQuad(q Quad) {
	clip := q.ContentMask.Bounds
	if clip.IsEmpty() || q.Bounds.IsEmpty() {
		return
	}
	if q.Background == BackgroundSolid && !q.Fill.IsTransparent() {
		r.fillRounded(q.Bounds, q.CornerRadii, clip, q.Fill)
	}
	if q.Background == BackgroundLinearGradient {
		r.fillGradient(q.Bounds, q.CornerRadii, clip, q.Gradient)
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
	if g.Image != nil {
		r.drawImage(g)
		return
	}
	m := g.Mask
	if m == nil || m.W <= 0 || m.H <= 0 || len(m.Pix) < m.W*m.H {
		return
	}
	if g.Color.IsTransparent() || g.Bounds.IsEmpty() {
		return
	}
	pm := g.Color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	minX, minY := float32(g.Bounds.Origin.X), float32(g.Bounds.Origin.Y)
	maxX, maxY := float32(g.Bounds.Max().X), float32(g.Bounds.Max().Y)
	if abs32(maxX-minX-float32(m.W)) < 0.01 && abs32(maxY-minY-float32(m.H)) < 0.01 {
		// 1:1 blit: every destination pixel maps onto exactly one texel.
		ox := int(math.Round(float64(minX)))
		oy := int(math.Round(float64(minY)))
		x0, y0, x1, y1, ok := r.clipWindow(g.ContentMask.Bounds)
		if !ok {
			return
		}
		x0, y0 = max(x0, ox), max(y0, oy)
		x1, y1 = min(x1, ox+m.W), min(y1, oy+m.H)
		for y := y0; y < y1; y++ {
			row := m.Pix[(y-oy)*m.W : (y-oy+1)*m.W]
			for x := x0; x < x1; x++ {
				t := row[x-ox]
				if t == 0 {
					continue
				}
				cov := float32(t) / 255
				r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
			}
		}
		return
	}
	x0, y0, x1, y1, ok := r.coverWindow(g.Bounds, g.ContentMask.Bounds)
	if !ok {
		return
	}
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

// drawImage draws a sprite's colour image scaled into its bounds with
// bilinear sampling of premultiplied colours; Color's alpha scales it.
func (r *Rasterizer) drawImage(g GlyphSprite) {
	img := g.Image
	iw, ih := img.Rect.Dx(), img.Rect.Dy()
	if iw <= 0 || ih <= 0 || g.Color.A == 0 || g.Bounds.IsEmpty() {
		return
	}
	x0, y0, x1, y1, ok := r.coverWindow(g.Bounds, g.ContentMask.Bounds)
	if !ok {
		return
	}
	minX, minY := float32(g.Bounds.Origin.X), float32(g.Bounds.Origin.Y)
	maxX, maxY := float32(g.Bounds.Max().X), float32(g.Bounds.Max().Y)
	// Pixels whose centre falls outside the bounds are skipped, so the image
	// does not bleed into the anti-aliasing margin coverWindow adds.
	sx := float32(iw) / (maxX - minX)
	sy := float32(ih) / (maxY - minY)
	alpha := float32(g.Color.A) / 255
	texel := func(x, y int) [4]float32 {
		x = min(max(x, 0), iw-1)
		y = min(max(y, 0), ih-1)
		i := img.PixOffset(img.Rect.Min.X+x, img.Rect.Min.Y+y)
		p := img.Pix[i : i+4 : i+4]
		a := float32(p[3]) / 255
		return [4]float32{float32(p[0]) * a, float32(p[1]) * a, float32(p[2]) * a, float32(p[3])}
	}
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		if py < minY || py > maxY {
			continue
		}
		v := (py-minY)*sy - 0.5
		vy := int(math.Floor(float64(v)))
		ty := v - float32(vy)
		for x := x0; x < x1; x++ {
			px := float32(x) + 0.5
			if px < minX || px > maxX {
				continue
			}
			u := (px-minX)*sx - 0.5
			ux := int(math.Floor(float64(u)))
			tx := u - float32(ux)
			a, b, c, d := texel(ux, vy), texel(ux+1, vy), texel(ux, vy+1), texel(ux+1, vy+1)
			var out [4]float32
			for k := range out {
				top := a[k]*(1-tx) + b[k]*tx
				bot := c[k]*(1-tx) + d[k]*tx
				out[k] = (top*(1-ty) + bot*ty) * alpha
			}
			if out[3] <= 0 {
				continue
			}
			r.blend(x, y, out[0], out[1], out[2], out[3])
		}
	}
}

// fillRounded fills the rounded rect b with color, anti-aliased via signed
// distance coverage, restricting the pixel loop to b (plus a one-pixel AA
// margin) within clip. Pixels whose coverage is certainly 1 skip the SDF.
func (r *Rasterizer) fillRounded(b Bounds, radii Corners, clip Bounds, color Rgba) {
	x0, y0, x1, y1, ok := r.coverWindow(b, clip)
	if !ok {
		return
	}
	pm := color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	rr := newRRect(b, radii)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		a, z := rr.solidRow(py, x0, x1)
		for x := x0; x < a; x++ {
			if cov := rr.coverage(float32(x)+0.5, py); cov > 0 {
				r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
			}
		}
		if pm.A == 255 {
			sa, sz := a, z
			if r.masked {
				ma, mz := r.mask.solidRow(py, x0, x1)
				sa = min(max(a, ma), z)
				sz = min(max(min(z, mz), sa), z)
			}
			for x := a; x < sa; x++ {
				r.blend(x, y, pr, pg, pb, pa)
			}
			r.fillSpan(y, sa, sz, pm)
			for x := sz; x < z; x++ {
				r.blend(x, y, pr, pg, pb, pa)
			}
		} else {
			for x := a; x < z; x++ {
				r.blend(x, y, pr, pg, pb, pa)
			}
		}
		for x := z; x < x1; x++ {
			if cov := rr.coverage(float32(x)+0.5, py); cov > 0 {
				r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
			}
		}
	}
}

// fillSpan overwrites pixels [a, z) of row y with the opaque premultiplied
// color c (identical to blending c at full coverage).
func (r *Rasterizer) fillSpan(y, a, z int, c Rgba) {
	if a >= z {
		return
	}
	row := r.pix[4*(y*r.w+a) : 4*(y*r.w+z)]
	row[0], row[1], row[2], row[3] = c.R, c.G, c.B, c.A
	for n := 4; n < len(row); n *= 2 {
		copy(row[n:], row[:n])
	}
}

// strokeBorder draws the border as the ring between the outer rounded rect
// and the inner rounded rect (bounds inset by the border widths, corner radii
// reduced by the adjacent border widths). Pixels where the inner rect fully
// covers (ring coverage 0) are skipped without evaluating the SDF.
func (r *Rasterizer) strokeBorder(b Bounds, radii Corners, bw Edges, clip Bounds, color Rgba) {
	inner := b.Inset(bw)
	innerRadii := Corners{
		TopLeft:     maxPixels(0, radii.TopLeft-minPixels(bw.Left, bw.Top)),
		TopRight:    maxPixels(0, radii.TopRight-minPixels(bw.Right, bw.Top)),
		BottomLeft:  maxPixels(0, radii.BottomLeft-minPixels(bw.Left, bw.Bottom)),
		BottomRight: maxPixels(0, radii.BottomRight-minPixels(bw.Right, bw.Bottom)),
	}
	x0, y0, x1, y1, ok := r.coverWindow(b, clip)
	if !ok {
		return
	}
	hasInner := !inner.IsEmpty()
	outerRR, innerRR := newRRect(b, radii), newRRect(inner, innerRadii)
	pm := color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		a, z := x1, x1
		if hasInner {
			a, z = innerRR.solidRow(py, x0, x1) // ring coverage 0 there
		}
		for x := x0; x < x1; x++ {
			if x == a {
				x = z
				if x >= x1 {
					break
				}
			}
			px := float32(x) + 0.5
			cov := outerRR.coverage(px, py)
			if cov <= 0 {
				continue
			}
			if hasInner {
				cov -= innerRR.coverage(px, py)
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

// coverWindow is clipWindow further restricted to the pixels that can have
// non-zero coverage from b: its pixel span grown by one pixel on each side
// (a pixel center 1.5px outside b has coverage 0 under the SDF rule).
func (r *Rasterizer) coverWindow(b, clip Bounds) (x0, y0, x1, y1 int, ok bool) {
	x0, y0, x1, y1, ok = r.clipWindow(clip)
	if !ok {
		return
	}
	x0 = max(x0, floorInt(b.Origin.X)-1)
	y0 = max(y0, floorInt(b.Origin.Y)-1)
	x1 = min(x1, ceilInt(b.Max().X)+1)
	y1 = min(y1, ceilInt(b.Max().Y)+1)
	return x0, y0, x1, y1, x0 < x1 && y0 < y1
}

// solidRow returns the pixel span [a, z) of the row whose centers are at py
// where coverage is certainly exactly 1 (signed distance <= -1, a 0.5 margin
// over the clamp threshold that absorbs float error), clamped to [x0, x1). An
// empty span is returned as a == z == x1.
func (r rrect) solidRow(py float32, x0, x1 int) (a, z int) {
	rl, rr := r.rBL, r.rBR
	if py < r.cy {
		rl, rr = r.rTL, r.rTR
	}
	dy := abs32(py - r.cy)
	al, okl := solidReach(dy-(r.hh-rl), rl)
	ar, okr := solidReach(dy-(r.hh-rr), rr)
	if !okl || !okr {
		return x1, x1
	}
	lo := r.cx - r.hw + rl - al
	hi := r.cx + r.hw - rr + ar
	a = min(max(ceilInt(Pixels(lo-0.5)), x0), x1)
	z = min(max(floorInt(Pixels(hi-0.5))+1, a), x1)
	if a == z {
		return x1, x1
	}
	return a, z
}

// solidReach returns how far past the straight edge (qx in the SDF) a pixel
// center may lie and still have signed distance <= -1, for a row at vertical
// SDF offset qy against a corner of radius rad; ok is false when no pixel of
// the row qualifies.
func solidReach(qy, rad float32) (reach float32, ok bool) {
	t := rad - 1
	if qy > t {
		return 0, false
	}
	if qy <= 0 {
		return t, true
	}
	return float32(math.Sqrt(float64(t*t - qy*qy))), true
}

// floorInt and ceilInt convert to int, saturating far outside any buffer.
func floorInt(v Pixels) int { return satInt(math.Floor(float64(v))) }
func ceilInt(v Pixels) int  { return satInt(math.Ceil(float64(v))) }

func satInt(f float64) int {
	switch {
	case f != f:
		return 0
	case f < -1<<30:
		return -1 << 30
	case f > 1<<30:
		return 1 << 30
	}
	return int(f)
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
	if r.masked {
		m := r.maskCoverage(x, y)
		if m <= 0 {
			return
		}
		if m < 1 {
			sr, sg, sb, sa = sr*m, sg*m, sb*m, sa*m
		}
	}
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
	return newRRect(b, radii).coverage(px, py)
}

// rrect is a rounded rect with its SDF constants precomputed, so per-pixel
// coverage skips the per-call setup.
type rrect struct {
	hw, hh, cx, cy     float32
	rTL, rTR, rBL, rBR float32
}

func newRRect(b Bounds, radii Corners) rrect {
	minX, minY := float32(b.Origin.X), float32(b.Origin.Y)
	maxX, maxY := float32(b.Max().X), float32(b.Max().Y)
	hw := (maxX - minX) / 2
	hh := (maxY - minY) / 2
	maxR := minf32(hw, hh)
	return rrect{
		hw: hw, hh: hh, cx: (minX + maxX) / 2, cy: (minY + maxY) / 2,
		rTL: clamp32(float32(radii.TopLeft), 0, maxR),
		rTR: clamp32(float32(radii.TopRight), 0, maxR),
		rBL: clamp32(float32(radii.BottomLeft), 0, maxR),
		rBR: clamp32(float32(radii.BottomRight), 0, maxR),
	}
}

// coverage is roundedRectCoverage for the precomputed rect.
func (r rrect) coverage(px, py float32) float32 {
	var rad float32
	switch {
	case px < r.cx && py < r.cy:
		rad = r.rTL
	case px >= r.cx && py < r.cy:
		rad = r.rTR
	case px < r.cx && py >= r.cy:
		rad = r.rBL
	default:
		rad = r.rBR
	}
	qx := abs32(px-r.cx) - (r.hw - rad)
	qy := abs32(py-r.cy) - (r.hh - rad)
	ox, oy := maxf32(qx, 0), maxf32(qy, 0)
	var outside float64
	switch {
	case oy == 0:
		outside = float64(ox) // math.Hypot(x, 0) == x exactly
	case ox == 0:
		outside = float64(oy)
	default:
		outside = math.Hypot(float64(ox), float64(oy))
	}
	d := outside + float64(minf32(maxf32(qx, qy), 0)) - float64(rad)
	return clamp32(0.5-float32(d), 0, 1)
}

// sampleBilinear samples the mask at texel-space coords (u, v) — the pixel
// center of texel i is at u = i — with edge clamping.
func sampleBilinear(m *GlyphMask, u, v float32) float32 {
	fu := clamp32(u, 0, float32(m.W-1))
	fv := clamp32(v, 0, float32(m.H-1))
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

// scaleScene returns s with every primitive scaled from logical to device
// pixels, written into out (reusing its slices); at scale 1 it returns s.
func scaleScene(s *Scene, scale float32, out *Scene) *Scene {
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
		return ContentMask{Bounds: sb(m.Bounds), CornerRadii: scaleCorners(m.CornerRadii, k)}
	}
	out.Clear()
	for _, q := range s.Quads {
		q.Bounds = sb(q.Bounds)
		q.ContentMask = sm(q.ContentMask)
		q.BorderWidths = Edges{Top: q.BorderWidths.Top * k, Right: q.BorderWidths.Right * k, Bottom: q.BorderWidths.Bottom * k, Left: q.BorderWidths.Left * k}
		q.CornerRadii = Corners{TopLeft: q.CornerRadii.TopLeft * k, TopRight: q.CornerRadii.TopRight * k, BottomLeft: q.CornerRadii.BottomLeft * k, BottomRight: q.CornerRadii.BottomRight * k}
		out.Quads = append(out.Quads, q)
	}
	for _, sh := range s.Shadows {
		sh.Bounds = sb(sh.Bounds)
		sh.Clip = sb(sh.Clip)
		sh.ContentMask = sm(sh.ContentMask)
		sh.BlurRadius *= k
		sh.CornerRadii = scaleCorners(sh.CornerRadii, k)
		sh.ClipRadii = scaleCorners(sh.ClipRadii, k)
		out.Shadows = append(out.Shadows, sh)
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
	for _, st := range s.Strokes {
		start := len(out.points)
		for _, p := range st.Points {
			out.points = append(out.points, Point{X: p.X * k, Y: p.Y * k})
		}
		st.Points = out.points[start:len(out.points):len(out.points)]
		st.ContentMask = sm(st.ContentMask)
		st.Width *= k
		out.Strokes = append(out.Strokes, st)
	}
	return out
}

func scaleCorners(c Corners, k Pixels) Corners {
	return Corners{TopLeft: c.TopLeft * k, TopRight: c.TopRight * k, BottomLeft: c.BottomLeft * k, BottomRight: c.BottomRight * k}
}

// fillGradient fills the rounded rect b with the linear gradient g,
// anti-aliased like fillRounded; colors come from a per-call lookup table.
func (r *Rasterizer) fillGradient(b Bounds, radii Corners, clip Bounds, g Gradient) {
	if g.Stops[0].Color.A == 0 && g.Stops[1].Color.A == 0 {
		return
	}
	x0, y0, x1, y1, ok := r.coverWindow(b, clip)
	if !ok {
		return
	}
	g.buildLUT(&r.lut)
	m := newGradientMapper(b, g)
	rr := newRRect(b, radii)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		a, z := rr.solidRow(py, x0, x1)
		for x := x0; x < x1; x++ {
			px := float32(x) + 0.5
			cov := float32(1)
			if x < a || x >= z {
				if cov = rr.coverage(px, py); cov <= 0 {
					continue
				}
			}
			c := &r.lut[m.index(px, py)]
			if c[3] <= 0 {
				continue
			}
			r.blend(x, y, c[0]*cov, c[1]*cov, c[2]*cov, c[3]*cov)
		}
	}
}

// phiRange and phiN define the standard normal CDF lookup table: phiN
// samples over [-phiRange, phiRange].
const (
	phiRange = 4
	phiN     = 1024
)

var phiTable = func() (t [phiN + 1]float32) {
	for i := range t {
		z := float64(i)/phiN*2*phiRange - phiRange
		t[i] = float32(0.5 * math.Erfc(-z/math.Sqrt2))
	}
	return t
}()

// phi is the standard normal CDF, linearly interpolated from phiTable.
func phi(z float32) float32 {
	if z <= -phiRange {
		return 0
	}
	if z >= phiRange {
		return 1
	}
	f := (z + phiRange) * (phiN / (2 * phiRange))
	i := int(f)
	t := f - float32(i)
	return phiTable[i] + (phiTable[i+1]-phiTable[i])*t
}

// dist is the signed distance from the pixel center (px, py) to the
// rounded rect (negative inside).
func (r rrect) dist(px, py float32) float32 {
	var rad float32
	switch {
	case px < r.cx && py < r.cy:
		rad = r.rTL
	case px >= r.cx && py < r.cy:
		rad = r.rTR
	case px < r.cx && py >= r.cy:
		rad = r.rBL
	default:
		rad = r.rBR
	}
	qx := abs32(px-r.cx) - (r.hw - rad)
	qy := abs32(py-r.cy) - (r.hh - rad)
	ox, oy := maxf32(qx, 0), maxf32(qy, 0)
	var outside float32
	switch {
	case oy == 0:
		outside = ox
	case ox == 0:
		outside = oy
	default:
		outside = float32(math.Sqrt(float64(ox*ox + oy*oy)))
	}
	return outside + minf32(maxf32(qx, qy), 0) - rad
}

// blurredShape evaluates the coverage of a gaussian-blurred rounded rect:
// min(Φ(-sdf/σ), box blur), where the separable box blur of the rect is
// fx (per column, precomputed) times fy (per row). It is exact for square
// corners and a close approximation for rounded ones.
type blurredShape struct {
	rr       rrect
	invSigma float32
	minX     float32
	maxX     float32
	minY     float32
	maxY     float32
}

func newBlurredShape(b Bounds, radii Corners, sigma float32) blurredShape {
	return blurredShape{
		rr:       newRRect(b, radii),
		invSigma: 1 / sigma,
		minX:     float32(b.Origin.X), maxX: float32(b.Max().X),
		minY: float32(b.Origin.Y), maxY: float32(b.Max().Y),
	}
}

func (s *blurredShape) colFactors(buf []float32, x0 int) {
	for i := range buf {
		px := float32(x0+i) + 0.5
		buf[i] = phi((px-s.minX)*s.invSigma) - phi((px-s.maxX)*s.invSigma)
	}
}

func (s *blurredShape) rowFactor(py float32) float32 {
	return phi((py-s.minY)*s.invSigma) - phi((py-s.maxY)*s.invSigma)
}

func (s *blurredShape) coverage(px, py, box float32) float32 {
	if box <= 0 {
		return 0
	}
	if c := phi(-s.rr.dist(px, py) * s.invSigma); c < box {
		return c
	}
	return box
}

// drawShadow paints an outset or inset box shadow. The pixel loop is
// bounded by the shape grown by 3σ (outset) or by the element (inset).
func (r *Rasterizer) drawShadow(sh Shadow) {
	if sh.Color.IsTransparent() || sh.ContentMask.Bounds.IsEmpty() {
		return
	}
	sigma := float32(sh.BlurRadius) / 2
	if sh.Inset {
		r.drawInsetShadow(sh, sigma)
		return
	}
	if sh.Bounds.IsEmpty() {
		return
	}
	if sigma < 0.05 {
		r.fillRounded(sh.Bounds, sh.CornerRadii, sh.ContentMask.Bounds, sh.Color)
		return
	}
	ext := Pixels(3 * sigma)
	area := sh.Bounds.Inset(Edges{Top: -ext, Right: -ext, Bottom: -ext, Left: -ext})
	x0, y0, x1, y1, ok := r.coverWindow(area, sh.ContentMask.Bounds)
	if !ok {
		return
	}
	bs := newBlurredShape(sh.Bounds, sh.CornerRadii, sigma)
	fx := r.rowScratch(x1 - x0)
	bs.colFactors(fx, x0)
	pm := sh.Color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		fy := bs.rowFactor(py)
		if fy <= 0 {
			continue
		}
		for x := x0; x < x1; x++ {
			cov := bs.coverage(float32(x)+0.5, py, fx[x-x0]*fy)
			if cov <= 0.5/255 {
				continue
			}
			r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
		}
	}
}

func (r *Rasterizer) drawInsetShadow(sh Shadow, sigma float32) {
	x0, y0, x1, y1, ok := r.coverWindow(sh.Clip, sh.ContentMask.Bounds)
	if !ok {
		return
	}
	outer := newRRect(sh.Clip, sh.ClipRadii)
	hasHole := !sh.Bounds.IsEmpty()
	crisp := sigma < 0.05
	var bs blurredShape
	var hole rrect
	fx := r.rowScratch(x1 - x0)
	if hasHole {
		if crisp {
			hole = newRRect(sh.Bounds, sh.CornerRadii)
		} else {
			bs = newBlurredShape(sh.Bounds, sh.CornerRadii, sigma)
			bs.colFactors(fx, x0)
		}
	}
	pm := sh.Color.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		py := float32(y) + 0.5
		var fy float32
		if hasHole && !crisp {
			fy = bs.rowFactor(py)
		}
		for x := x0; x < x1; x++ {
			px := float32(x) + 0.5
			cov := outer.coverage(px, py)
			if cov <= 0 {
				continue
			}
			if hasHole {
				var in float32
				if crisp {
					in = hole.coverage(px, py)
				} else {
					in = bs.coverage(px, py, fx[x-x0]*fy)
				}
				cov *= 1 - in
			}
			if cov <= 0.5/255 {
				continue
			}
			r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
		}
	}
}

// rowScratch returns a reusable float slice of length n.
func (r *Rasterizer) rowScratch(n int) []float32 {
	if cap(r.rowBuf) < n {
		r.rowBuf = make([]float32, n)
	}
	return r.rowBuf[:n]
}
