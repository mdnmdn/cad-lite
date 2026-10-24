package gpui

import "math"

// Stroke is an anti-aliased polyline of constant width with round joins and
// caps. Coverage is the union of the segments' capsules, so a joint or a
// self-crossing is blended once, not once per segment.
type Stroke struct {
	// Order is the paint order; 0 means "assign the scene's next auto order".
	Order uint32
	// ContentMask clips the stroke; the zero value clips it away entirely.
	ContentMask ContentMask
	// Points are the polyline vertices in window space. The slice is read at
	// raster time, so it must not be modified until the frame is presented.
	Points []Point
	// Closed adds a segment from the last point back to the first.
	Closed bool
	// Width is the full stroke width in logical pixels. A width below one
	// device pixel is drawn one device pixel wide with proportionally
	// reduced alpha, which keeps hairlines visible at any zoom.
	Width Pixels
	// Color is the stroke color.
	Color Rgba
}

// InsertStroke appends a stroke, assigning its auto order when Order is 0.
func (s *Scene) InsertStroke(st Stroke) {
	st.Order = s.takeOrder(st.Order)
	s.Strokes = append(s.Strokes, st)
}

// PaintStroke inserts a stroke into the frame being built, filling an unset
// content mask with the current clip and applying the element opacity.
func (w *Window) PaintStroke(s Stroke) {
	if s.ContentMask.Bounds.Size.W <= 0 || s.ContentMask.Bounds.Size.H <= 0 {
		s.ContentMask = w.CurrentContentMask()
	}
	if w.opacity < 1 {
		s.Color = s.Color.Opacity(w.opacity)
	}
	w.nextFrame.scene.InsertStroke(s)
}

// strokeBuf accumulates one stroke's coverage before it is composited: per
// pixel the maximum over segments, with the touched x-range of each row. It
// covers only the stroke's pixel window: cov is indexed relative to (ox, oy)
// with the window's width as stride, so a tile-sized window needs only a
// tile-sized buffer. Between strokes every cov entry is 0 and every row range
// is empty, so a window of any size reuses the storage without clearing it.
type strokeBuf struct {
	cov            []float32
	rowMin, rowMax []int
	ox, oy, stride int
	yMin, yMax     int
}

// emptyRow is rowMin's value for a row no segment touched.
const emptyRow = math.MaxInt32

func (b *strokeBuf) reset(x0, y0, x1, y1 int) {
	w, h := x1-x0, y1-y0
	if cap(b.cov) < w*h {
		b.cov = make([]float32, w*h)
	}
	b.cov = b.cov[:w*h]
	if cap(b.rowMin) < h {
		b.rowMin = make([]int, h)
		b.rowMax = make([]int, h)
		for y := range b.rowMin {
			b.rowMin[y] = emptyRow
			b.rowMax[y] = -1
		}
	}
	b.rowMin, b.rowMax = b.rowMin[:h], b.rowMax[:h]
	b.ox, b.oy, b.stride = x0, y0, w
	b.yMin, b.yMax = y1, y0-1
}

// strokeHalf returns the half width a stroke is covered with and the alpha
// that scales it: below one device pixel the stroke is drawn one pixel wide
// with proportionally reduced alpha.
func strokeHalf(width Pixels) (half, alpha float32) {
	half, alpha = float32(width)/2, 1
	if half < 0.5 {
		alpha = max(float32(width), 0.05)
		half = 0.5
		if width <= 0 {
			alpha = 1
		}
	}
	return half, alpha
}

// strokeWindow is the pixel window a stroke (mask and width in device
// pixels, points logical, scaled by k) can touch: its clip window cut to the
// points' bounding box grown by the coverage reach, with a margin for
// rounding. A NaN coordinate keeps the whole clip window.
func (r *Rasterizer) strokeWindow(st Stroke, k Pixels) (x0, y0, x1, y1 int, ok bool) {
	x0, y0, x1, y1, ok = r.clipWindow(st.ContentMask.Bounds)
	if !ok || len(st.Points) == 0 {
		return x0, y0, x1, y1, false
	}
	minX, minY, maxX, maxY := float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, p := range st.Points {
		x, y := scalePoint(p, k)
		if x != x || y != y {
			return x0, y0, x1, y1, true
		}
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	half, _ := strokeHalf(st.Width)
	reach := half + 0.5
	mx := reach + 2 + max(abs32(minX), abs32(maxX))*1e-6
	my := reach + 2 + max(abs32(minY), abs32(maxY))*1e-6
	x0 = max(x0, floorInt(Pixels(minX-mx)))
	y0 = max(y0, floorInt(Pixels(minY-my)))
	x1 = min(x1, ceilInt(Pixels(maxX+mx)))
	y1 = min(y1, ceilInt(Pixels(maxY+my)))
	return x0, y0, x1, y1, x0 < x1 && y0 < y1
}

// drawStroke draws st, whose mask and width are in device pixels and whose
// points are logical pixels scaled by k as they are read.
func (r *Rasterizer) drawStroke(st Stroke, k Pixels) {
	n := len(st.Points)
	if n == 0 || st.Color.IsTransparent() {
		return
	}
	x0, y0, x1, y1, ok := r.strokeWindow(st, k)
	if !ok {
		return
	}
	half, alpha := strokeHalf(st.Width)
	b := &r.stroke
	b.reset(x0, y0, x1, y1)
	seg := func(a, c Point) {
		ax, ay := scalePoint(a, k)
		cx, cy := scalePoint(c, k)
		r.coverSegment(b, ax, ay, cx, cy, half, x0, y0, x1, y1)
	}
	if n == 1 {
		seg(st.Points[0], st.Points[0])
	}
	for i := 1; i < n; i++ {
		seg(st.Points[i-1], st.Points[i])
	}
	if st.Closed && n > 2 {
		seg(st.Points[n-1], st.Points[0])
	}
	pm := st.Color.Premultiply()
	pr, pg, pb, pa := float32(pm.R)*alpha, float32(pm.G)*alpha, float32(pm.B)*alpha, float32(pm.A)*alpha
	for y := b.yMin; y <= b.yMax; y++ {
		ry := y - b.oy
		lo, hi := b.rowMin[ry], b.rowMax[ry]
		row := b.cov[ry*b.stride : (ry+1)*b.stride]
		for x := lo; x <= hi; x++ {
			c := row[x-b.ox]
			if c <= 0 {
				continue
			}
			row[x-b.ox] = 0
			r.blend(x, y, pr*c, pg*c, pb*c, pa*c)
		}
		b.rowMin[ry], b.rowMax[ry] = emptyRow, -1
	}
}

// coverSegment max-accumulates the coverage of the capsule around segment
// (ax,ay)-(bx,by) with radius half, restricted to [x0,x1)×[y0,y1).
func (r *Rasterizer) coverSegment(b *strokeBuf, ax, ay, bx, by, half float32, x0, y0, x1, y1 int) {
	reach := half + 0.5
	ry0 := max(y0, int(math.Floor(float64(min(ay, by)-reach))))
	ry1 := min(y1, int(math.Ceil(float64(max(ay, by)+reach))))
	if ry0 >= ry1 {
		return
	}
	// A segment wholly left or right of the window touches none of its
	// pixels; the margin absorbs rounding in the per-row span below.
	if m := reach + 2 + max(abs32(ax), abs32(bx))*1e-6; max(ax, bx)+m <= float32(x0) || min(ax, bx)-m >= float32(x1) {
		return
	}
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	for y := ry0; y < ry1; y++ {
		py := float32(y) + 0.5
		// The x-range of the segment inside the row band, grown by reach.
		lo, hi := min(ax, bx), max(ax, bx)
		if abs32(dy) > 1e-6 {
			t0 := (py - reach - 0.5 - ay) / dy
			t1 := (py + reach + 0.5 - ay) / dy
			if t0 > t1 {
				t0, t1 = t1, t0
			}
			t0, t1 = clamp32(t0, 0, 1), clamp32(t1, 0, 1)
			xa, xb := ax+dx*t0, ax+dx*t1
			lo, hi = min(xa, xb), max(xa, xb)
		}
		sx0 := max(x0, int(math.Floor(float64(lo-reach))))
		sx1 := min(x1, int(math.Ceil(float64(hi+reach))))
		if sx0 >= sx1 {
			continue
		}
		ry := y - b.oy
		row := b.cov[ry*b.stride : (ry+1)*b.stride]
		touched := false
		for x := sx0; x < sx1; x++ {
			px := float32(x) + 0.5
			t := float32(0)
			if l2 > 0 {
				t = clamp32(((px-ax)*dx+(py-ay)*dy)/l2, 0, 1)
			}
			ex, ey := px-(ax+dx*t), py-(ay+dy*t)
			c := reach - float32(math.Sqrt(float64(ex*ex+ey*ey)))
			if c <= 0 {
				continue
			}
			if c > 1 {
				c = 1
			}
			if i := x - b.ox; c > row[i] {
				row[i] = c
			}
			touched = true
		}
		if touched {
			b.rowMin[ry] = min(b.rowMin[ry], sx0)
			b.rowMax[ry] = max(b.rowMax[ry], sx1-1)
			b.yMin = min(b.yMin, y)
			b.yMax = max(b.yMax, y)
		}
	}
}
