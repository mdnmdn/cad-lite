package gpui

import (
	"cmp"
	"encoding/binary"
	"hash/maphash"
	"image"
	"math"
	"slices"
	"unsafe"
)

// tileSize is the side of a damage-tracking tile in device pixels.
const tileSize = 64

// primKind identifies the scene list a drawOp indexes.
type primKind uint8

const (
	kindShadow primKind = iota
	kindQuad
	kindUnderline
	kindSprite
	kindStroke
)

// drawOp is one primitive in paint order: its kind, its index in the scene
// list, and (for damage tracking) its device-pixel window, empty when the
// primitive draws nothing.
type drawOp struct {
	kind           primKind
	idx            int32
	x0, y0, x1, y1 int32
}

// tileState is the damage-tracking state: the per-tile hashes of the last
// RenderDamage and the scratch reused to compute the next ones.
type tileState struct {
	valid  bool
	clear  Rgba
	scale  float32
	nx, ny int

	prev, next []uint64
	dirty      []bool
	bins       [][]int32
	damage     []image.Rectangle

	seed maphash.Seed
	mh   maphash.Hash
	buf  []byte

	// refs keeps the masks and versioned images hashed by pointer in the
	// current frame alive, prevRefs those of the frame compared against, so
	// the allocator cannot hand a compared pointer to a different object.
	refs, prevRefs []unsafe.Pointer
}

// tileHashInit is the hash of a tile no primitive touches.
const tileHashInit uint64 = 0x9e3779b97f4a7c15

// foldHash mixes a primitive hash into a tile's running hash; the result
// depends on the order of the primitives.
func foldHash(t, h uint64) uint64 {
	t ^= h
	t *= 0xbf58476d1ce4e5b9
	t ^= t >> 31
	t *= 0x94d049bb133111eb
	return t ^ t>>29
}

// Invalidate makes the next RenderDamage repaint and report the whole buffer.
func (r *Rasterizer) Invalidate() { r.tiles.valid = false }

// RenderDamage brings the buffer to the pixels Reset(clear) followed by
// Render(s) would produce, repainting only the tiles whose primitives changed
// since the previous RenderDamage, and returns the changed rects in device
// pixels (empty when nothing changed). The slice is reused by the next call.
func (r *Rasterizer) RenderDamage(s *Scene, clear Rgba) []image.Rectangle {
	return r.renderDamage(s, 1, clear)
}

// renderDamage is RenderDamage for a scene in logical pixels drawn at scale:
// every primitive is scaled while it is binned and drawn.
func (r *Rasterizer) renderDamage(s *Scene, scale float32, clear Rgba) []image.Rectangle {
	if s == nil {
		s = &Scene{}
	}
	t := &r.tiles
	if t.seed == (maphash.Seed{}) {
		t.seed = maphash.MakeSeed()
		t.mh.SetSeed(t.seed)
	}
	t.damage = t.damage[:0]
	if r.w <= 0 || r.h <= 0 {
		return t.damage
	}
	k := Pixels(scale)
	r.setLimit(0, 0, r.w, r.h)
	r.buildOps(s)

	nx, ny := (r.w+tileSize-1)/tileSize, (r.h+tileSize-1)/tileSize
	n := nx * ny
	full := !t.valid || t.clear != clear || t.scale != scale || t.nx != nx || t.ny != ny
	if len(t.next) != n {
		t.prev = make([]uint64, n)
		t.next = make([]uint64, n)
		t.dirty = make([]bool, n)
		t.bins = make([][]int32, n)
		full = true
	}
	t.prevRefs, t.refs = t.refs, t.prevRefs
	clear2 := t.refs[:cap(t.refs)]
	for i := range clear2 {
		clear2[i] = nil
	}
	t.refs = t.refs[:0]

	next := t.next
	for i := range next {
		next[i] = tileHashInit
	}
	for i := range r.ops {
		op := &r.ops[i]
		x0, y0, x1, y1, ok := r.opWindow(s, k, op)
		if !ok {
			op.x0, op.y0, op.x1, op.y1 = 0, 0, 0, 0
			continue
		}
		op.x0, op.y0, op.x1, op.y1 = int32(x0), int32(y0), int32(x1), int32(y1)
		h := r.hashOp(s, op)
		for ty := y0 / tileSize; ty <= (y1-1)/tileSize; ty++ {
			row := next[ty*nx : (ty+1)*nx]
			for tx := x0 / tileSize; tx <= (x1-1)/tileSize; tx++ {
				row[tx] = foldHash(row[tx], h)
			}
		}
	}

	damaged := 0
	for i := range next {
		d := full || next[i] != t.prev[i]
		t.dirty[i] = d
		if d {
			damaged++
		}
	}
	t.prev, t.next = t.next, t.prev
	t.valid, t.clear, t.scale, t.nx, t.ny = true, clear, scale, nx, ny
	if damaged == 0 {
		return t.damage
	}

	if full || damaged*4 >= n*3 {
		r.fill(0, 0, r.w, r.h, clear)
		for i := range r.ops {
			if op := &r.ops[i]; op.x0 < op.x1 {
				r.drawOp(s, k, op)
			}
		}
		r.masked = false
		t.damage = append(t.damage, image.Rect(0, 0, r.w, r.h))
		return t.damage
	}

	for i, d := range t.dirty {
		if d {
			t.bins[i] = t.bins[i][:0]
		}
	}
	for i := range r.ops {
		op := &r.ops[i]
		if op.x0 >= op.x1 {
			continue
		}
		for ty := int(op.y0) / tileSize; ty <= int(op.y1-1)/tileSize; ty++ {
			for tx := int(op.x0) / tileSize; tx <= int(op.x1-1)/tileSize; tx++ {
				if ti := ty*nx + tx; t.dirty[ti] {
					t.bins[ti] = append(t.bins[ti], int32(i))
				}
			}
		}
	}
	for ty := range ny {
		for tx := range nx {
			ti := ty*nx + tx
			if !t.dirty[ti] {
				continue
			}
			rect := image.Rect(tx*tileSize, ty*tileSize, min((tx+1)*tileSize, r.w), min((ty+1)*tileSize, r.h))
			r.renderTile(s, k, rect, t.bins[ti], clear)
			if last := len(t.damage) - 1; last >= 0 && t.damage[last].Max.X == rect.Min.X && t.damage[last].Min.Y == rect.Min.Y {
				t.damage[last].Max.X = rect.Max.X
			} else {
				t.damage = append(t.damage, rect)
			}
		}
	}
	r.setLimit(0, 0, r.w, r.h)
	return t.damage
}

// renderTile repaints one tile: it clears rect and draws the tile's bin with
// every pixel loop limited to rect. It touches only rect's pixels and the
// rasterizer's per-draw scratch, so tiles are independent of each other.
func (r *Rasterizer) renderTile(s *Scene, k Pixels, rect image.Rectangle, bin []int32, clear Rgba) {
	r.setLimit(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)
	r.fill(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y, clear)
	for _, i := range bin {
		r.drawOp(s, k, &r.ops[i])
	}
	r.masked = false
}

// setLimit restricts every pixel loop to [x0,x1)×[y0,y1).
func (r *Rasterizer) setLimit(x0, y0, x1, y1 int) {
	r.lx0, r.ly0, r.lx1, r.ly1 = x0, y0, x1, y1
}

// fill overwrites [x0,x1)×[y0,y1) with the premultiplied form of c.
func (r *Rasterizer) fill(x0, y0, x1, y1 int, c Rgba) {
	if x0 >= x1 || y0 >= y1 {
		return
	}
	pm := c.Premultiply()
	if x0 == 0 && x1 == r.w {
		row := r.pix[4*y0*r.w : 4*y1*r.w]
		row[0], row[1], row[2], row[3] = pm.R, pm.G, pm.B, pm.A
		for n := 4; n < len(row); n *= 2 {
			copy(row[n:], row[:n])
		}
		return
	}
	for y := y0; y < y1; y++ {
		r.fillSpan(y, x0, x1, pm)
	}
}

// buildOps lists the scene's primitives in paint order: each kind stably
// sorted by Order, merged with shadow < quad < stroke < underline < sprite on
// equal Order. Kinds already in order (the common case) are not sorted.
func (r *Rasterizer) buildOps(s *Scene) {
	r.ops = r.ops[:0]
	sh := orderPerm(&r.perms[kindShadow], s.Shadows, func(v *Shadow) uint32 { return v.Order })
	qs := orderPerm(&r.perms[kindQuad], s.Quads, func(v *Quad) uint32 { return v.Order })
	us := orderPerm(&r.perms[kindUnderline], s.Underlines, func(v *Underline) uint32 { return v.Order })
	gs := orderPerm(&r.perms[kindSprite], s.Sprites, func(v *GlyphSprite) uint32 { return v.Order })
	ts := orderPerm(&r.perms[kindStroke], s.Strokes, func(v *Stroke) uint32 { return v.Order })
	at := func(perm []int32, j int) int {
		if perm == nil {
			return j
		}
		return int(perm[j])
	}
	var si, qi, ui, gi, ti int
	for {
		const none = math.MaxUint32 + 1
		next, kind, idx := uint64(none), primKind(255), 0
		if si < len(s.Shadows) {
			i := at(sh, si)
			next, kind, idx = uint64(s.Shadows[i].Order), kindShadow, i
		}
		if qi < len(s.Quads) {
			if i := at(qs, qi); uint64(s.Quads[i].Order) < next {
				next, kind, idx = uint64(s.Quads[i].Order), kindQuad, i
			}
		}
		if ti < len(s.Strokes) {
			if i := at(ts, ti); uint64(s.Strokes[i].Order) < next {
				next, kind, idx = uint64(s.Strokes[i].Order), kindStroke, i
			}
		}
		if ui < len(s.Underlines) {
			if i := at(us, ui); uint64(s.Underlines[i].Order) < next {
				next, kind, idx = uint64(s.Underlines[i].Order), kindUnderline, i
			}
		}
		if gi < len(s.Sprites) {
			if i := at(gs, gi); uint64(s.Sprites[i].Order) < next {
				kind, idx = kindSprite, i
			}
		}
		switch kind {
		case kindShadow:
			si++
		case kindQuad:
			qi++
		case kindStroke:
			ti++
		case kindUnderline:
			ui++
		case kindSprite:
			gi++
		default:
			return
		}
		r.ops = append(r.ops, drawOp{kind: kind, idx: int32(idx)})
	}
}

// orderPerm returns nil when v is already sorted by order, else the indices
// of v stably sorted by order (stored in *perm for reuse).
func orderPerm[T any](perm *[]int32, v []T, order func(*T) uint32) []int32 {
	sorted := true
	for i := 1; i < len(v); i++ {
		if order(&v[i]) < order(&v[i-1]) {
			sorted = false
			break
		}
	}
	if sorted {
		return nil
	}
	p := (*perm)[:0]
	for i := range v {
		p = append(p, int32(i))
	}
	slices.SortStableFunc(p, func(a, b int32) int { return cmp.Compare(order(&v[a]), order(&v[b])) })
	*perm = p
	return p
}

// drawOp draws one primitive scaled by k with the current limit.
func (r *Rasterizer) drawOp(s *Scene, k Pixels, op *drawOp) {
	switch op.kind {
	case kindShadow:
		sh := scaleShadow(s.Shadows[op.idx], k)
		r.setMask(sh.ContentMask)
		r.drawShadow(sh)
	case kindQuad:
		q := scaleQuad(s.Quads[op.idx], k)
		r.setMask(q.ContentMask)
		r.drawQuad(q)
	case kindUnderline:
		u := scaleUnderline(s.Underlines[op.idx], k)
		r.setMask(u.ContentMask)
		r.drawUnderline(u)
	case kindSprite:
		g := scaleSprite(s.Sprites[op.idx], k)
		r.setMask(g.ContentMask)
		r.drawSprite(g)
	case kindStroke:
		st := scaleStrokeShape(s.Strokes[op.idx], k)
		r.setMask(st.ContentMask)
		r.drawStroke(st, k)
	}
}

// opWindow is the device-pixel window the primitive can touch (within the
// current limit, the whole buffer while binning); ok is false when it draws
// nothing. Each window is the one the matching draw function clamps to.
func (r *Rasterizer) opWindow(s *Scene, k Pixels, op *drawOp) (x0, y0, x1, y1 int, ok bool) {
	switch op.kind {
	case kindShadow:
		sh := scaleShadow(s.Shadows[op.idx], k)
		if sh.Color.IsTransparent() || sh.ContentMask.Bounds.IsEmpty() {
			return
		}
		if sh.Inset {
			return r.coverWindow(sh.Clip, sh.ContentMask.Bounds)
		}
		if sh.Bounds.IsEmpty() {
			return
		}
		sigma := float32(sh.BlurRadius) / 2
		if sigma < 0.05 {
			return r.coverWindow(sh.Bounds, sh.ContentMask.Bounds)
		}
		ext := Pixels(3 * sigma)
		return r.coverWindow(sh.Bounds.Inset(Edges{Top: -ext, Right: -ext, Bottom: -ext, Left: -ext}), sh.ContentMask.Bounds)
	case kindQuad:
		q := scaleQuad(s.Quads[op.idx], k)
		if q.ContentMask.Bounds.IsEmpty() || q.Bounds.IsEmpty() {
			return
		}
		fill := (q.Background == BackgroundSolid && !q.Fill.IsTransparent()) ||
			(q.Background == BackgroundLinearGradient && (q.Gradient.Stops[0].Color.A != 0 || q.Gradient.Stops[1].Color.A != 0))
		bw := q.BorderWidths
		border := !q.BorderColor.IsTransparent() && (bw.Top > 0 || bw.Right > 0 || bw.Bottom > 0 || bw.Left > 0)
		if !fill && !border {
			return
		}
		return r.coverWindow(q.Bounds, q.ContentMask.Bounds)
	case kindUnderline:
		u := scaleUnderline(s.Underlines[op.idx], k)
		if u.Color.IsTransparent() {
			return
		}
		rect, nonEmpty := underlineRect(u)
		if !nonEmpty {
			return
		}
		return r.coverWindow(rect, u.ContentMask.Bounds)
	case kindSprite:
		g := scaleSprite(s.Sprites[op.idx], k)
		if g.Image != nil {
			if g.Image.Rect.Dx() <= 0 || g.Image.Rect.Dy() <= 0 || g.Color.A == 0 || g.Bounds.IsEmpty() {
				return
			}
		} else if m := g.Mask; m == nil || m.W <= 0 || m.H <= 0 || len(m.Pix) < m.W*m.H || g.Color.IsTransparent() || g.Bounds.IsEmpty() {
			return
		}
		// The 1:1 blit window lies inside this one (see drawSprite).
		return r.coverWindow(g.Bounds, g.ContentMask.Bounds)
	case kindStroke:
		st := scaleStrokeShape(s.Strokes[op.idx], k)
		if len(st.Points) == 0 || st.Color.IsTransparent() {
			return
		}
		return r.strokeWindow(st, k)
	}
	return
}

// hashOp hashes every field of the primitive the raster reads, in logical
// pixels (the scale is the same for every frame compared).
func (r *Rasterizer) hashOp(s *Scene, op *drawOp) uint64 {
	t := &r.tiles
	b := append(t.buf[:0], byte(op.kind))
	var extra []byte
	switch op.kind {
	case kindShadow:
		sh := &s.Shadows[op.idx]
		b = putBounds(b, sh.Bounds)
		b = putCorners(b, sh.CornerRadii)
		b = putMask(b, sh.ContentMask)
		b = putRgba(b, sh.Color)
		b = putF32(b, sh.BlurRadius)
		b = putBool(b, sh.Inset)
		b = putBounds(b, sh.Clip)
		b = putCorners(b, sh.ClipRadii)
	case kindQuad:
		q := &s.Quads[op.idx]
		b = putBounds(b, q.Bounds)
		b = putMask(b, q.ContentMask)
		b = append(b, byte(q.Background))
		b = putRgba(b, q.Fill)
		b = putRgba(b, q.BorderColor)
		b = putF32(b, q.BorderWidths.Top, q.BorderWidths.Right, q.BorderWidths.Bottom, q.BorderWidths.Left)
		b = putCorners(b, q.CornerRadii)
		g := &q.Gradient
		b = putF32(b, Pixels(g.Angle), Pixels(g.Stops[0].Percentage), Pixels(g.Stops[1].Percentage))
		b = putRgba(b, g.Stops[0].Color)
		b = putRgba(b, g.Stops[1].Color)
		b = append(b, byte(g.Space))
	case kindUnderline:
		u := &s.Underlines[op.idx]
		b = putBounds(b, u.Bounds)
		b = putMask(b, u.ContentMask)
		b = putRgba(b, u.Color)
		b = putF32(b, u.Thickness)
	case kindSprite:
		g := &s.Sprites[op.idx]
		b = putBounds(b, g.Bounds)
		b = putMask(b, g.ContentMask)
		b = putRgba(b, g.Color)
		if img := g.Image; img != nil {
			b = append(b, 1)
			b = putInt(b, img.Rect.Min.X, img.Rect.Min.Y, img.Rect.Max.X, img.Rect.Max.Y, img.Stride)
			if g.ImageVersion != 0 {
				b = putInt(b, int(uintptr(unsafe.Pointer(img))))
				b = binary.LittleEndian.AppendUint64(b, g.ImageVersion)
				t.refs = append(t.refs, unsafe.Pointer(img))
			} else {
				extra = imagePixels(img)
			}
		} else {
			b = append(b, 0)
			b = putInt(b, int(uintptr(unsafe.Pointer(g.Mask))))
			t.refs = append(t.refs, unsafe.Pointer(g.Mask))
		}
	case kindStroke:
		st := &s.Strokes[op.idx]
		b = putMask(b, st.ContentMask)
		b = putBool(b, st.Closed)
		b = putF32(b, st.Width)
		b = putRgba(b, st.Color)
		b = putInt(b, len(st.Points))
		if len(st.Points) > 0 {
			extra = unsafe.Slice((*byte)(unsafe.Pointer(&st.Points[0])), len(st.Points)*int(unsafe.Sizeof(Point{})))
		}
	}
	t.buf = b
	if extra == nil {
		return maphash.Bytes(t.seed, b)
	}
	t.mh.Reset()
	t.mh.Write(b)
	t.mh.Write(extra)
	return t.mh.Sum64()
}

// imagePixels returns the bytes of img.Pix that hold its Rect.
func imagePixels(img *image.NRGBA) []byte {
	r := img.Rect
	if r.Empty() {
		return nil
	}
	start := img.PixOffset(r.Min.X, r.Min.Y)
	end := img.PixOffset(r.Max.X-1, r.Max.Y-1) + 4
	if start < 0 || end > len(img.Pix) || start >= end {
		return img.Pix
	}
	return img.Pix[start:end]
}

func putF32(b []byte, vs ...Pixels) []byte {
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(float32(v)))
	}
	return b
}

func putInt(b []byte, vs ...int) []byte {
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint64(b, uint64(v))
	}
	return b
}

func putBool(b []byte, v bool) []byte {
	if v {
		return append(b, 1)
	}
	return append(b, 0)
}

func putRgba(b []byte, c Rgba) []byte { return append(b, c.R, c.G, c.B, c.A) }

func putBounds(b []byte, v Bounds) []byte {
	return putF32(b, v.Origin.X, v.Origin.Y, v.Size.W, v.Size.H)
}

func putCorners(b []byte, c Corners) []byte {
	return putF32(b, c.TopLeft, c.TopRight, c.BottomLeft, c.BottomRight)
}

func putMask(b []byte, m ContentMask) []byte {
	return putCorners(putBounds(b, m.Bounds), m.CornerRadii)
}

// The scale helpers map a primitive from logical to device pixels. Every
// product goes through an explicit float32 conversion, which rounds it, so
// the compiler cannot fuse it into a later operation: a primitive scaled here
// and one scaled into a stored copy produce the same pixels.

func scalePx(v, k Pixels) Pixels { return Pixels(float32(v * k)) }

func scaleBounds(b Bounds, k Pixels) Bounds {
	return Bounds{
		Origin: Point{X: scalePx(b.Origin.X, k), Y: scalePx(b.Origin.Y, k)},
		Size:   Size{W: scalePx(b.Size.W, k), H: scalePx(b.Size.H, k)},
	}
}

func scaleCorners(c Corners, k Pixels) Corners {
	return Corners{TopLeft: scalePx(c.TopLeft, k), TopRight: scalePx(c.TopRight, k), BottomLeft: scalePx(c.BottomLeft, k), BottomRight: scalePx(c.BottomRight, k)}
}

// scaleMask leaves an "infinite" mask as it is.
func scaleMask(m ContentMask, k Pixels) ContentMask {
	if m.Bounds.Size.W >= 1e6 {
		return m
	}
	return ContentMask{Bounds: scaleBounds(m.Bounds, k), CornerRadii: scaleCorners(m.CornerRadii, k)}
}

func scaleQuad(q Quad, k Pixels) Quad {
	if k == 1 {
		return q
	}
	q.Bounds = scaleBounds(q.Bounds, k)
	q.ContentMask = scaleMask(q.ContentMask, k)
	q.BorderWidths = Edges{Top: scalePx(q.BorderWidths.Top, k), Right: scalePx(q.BorderWidths.Right, k), Bottom: scalePx(q.BorderWidths.Bottom, k), Left: scalePx(q.BorderWidths.Left, k)}
	q.CornerRadii = scaleCorners(q.CornerRadii, k)
	return q
}

func scaleShadow(sh Shadow, k Pixels) Shadow {
	if k == 1 {
		return sh
	}
	sh.Bounds = scaleBounds(sh.Bounds, k)
	sh.Clip = scaleBounds(sh.Clip, k)
	sh.ContentMask = scaleMask(sh.ContentMask, k)
	sh.BlurRadius = scalePx(sh.BlurRadius, k)
	sh.CornerRadii = scaleCorners(sh.CornerRadii, k)
	sh.ClipRadii = scaleCorners(sh.ClipRadii, k)
	return sh
}

func scaleUnderline(u Underline, k Pixels) Underline {
	if k == 1 {
		return u
	}
	u.Bounds = scaleBounds(u.Bounds, k)
	u.ContentMask = scaleMask(u.ContentMask, k)
	u.Thickness = scalePx(u.Thickness, k)
	return u
}

func scaleSprite(g GlyphSprite, k Pixels) GlyphSprite {
	if k == 1 {
		return g
	}
	g.Bounds = scaleBounds(g.Bounds, k)
	g.ContentMask = scaleMask(g.ContentMask, k)
	return g
}

// scaleStrokeShape scales a stroke's mask and width; its points stay logical
// and are scaled by drawStroke and strokeWindow as they are read.
func scaleStrokeShape(st Stroke, k Pixels) Stroke {
	if k == 1 {
		return st
	}
	st.ContentMask = scaleMask(st.ContentMask, k)
	st.Width = scalePx(st.Width, k)
	return st
}

// scalePoint maps a logical stroke point to device pixels.
func scalePoint(p Point, k Pixels) (x, y float32) {
	if k == 1 {
		return float32(p.X), float32(p.Y)
	}
	return float32(p.X * k), float32(p.Y * k)
}
