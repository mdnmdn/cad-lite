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
// pixel the maximum over segments, with the touched x-range of each row.
type strokeBuf struct {
	cov        []float32
	rowMin     []int
	rowMax     []int
	yMin, yMax int
	w, h       int
}

func (b *strokeBuf) reset(w, h int) {
	if b.w != w || b.h != h || len(b.cov) != w*h {
		b.w, b.h = w, h
		b.cov = make([]float32, w*h)
		b.rowMin = make([]int, h)
		b.rowMax = make([]int, h)
		for y := range b.rowMin {
			b.rowMin[y] = w
			b.rowMax[y] = -1
		}
	}
	b.yMin, b.yMax = h, -1
}

func (r *Rasterizer) drawStroke(st Stroke) {
	n := len(st.Points)
	if n == 0 || st.Color.IsTransparent() {
		return
	}
	x0, y0, x1, y1, ok := r.clipWindow(st.ContentMask.Bounds)
	if !ok {
		return
	}
	half := float32(st.Width) / 2
	alpha := float32(1)
	if half < 0.5 {
		alpha = max(float32(st.Width), 0.05)
		half = 0.5
		if st.Width <= 0 {
			alpha = 1
		}
	}
	b := &r.stroke
	b.reset(r.w, r.h)
	seg := func(a, c Point) {
		r.coverSegment(b, float32(a.X), float32(a.Y), float32(c.X), float32(c.Y), half, x0, y0, x1, y1)
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
		lo, hi := b.rowMin[y], b.rowMax[y]
		row := b.cov[y*r.w : (y+1)*r.w]
		for x := lo; x <= hi; x++ {
			c := row[x]
			if c <= 0 {
				continue
			}
			row[x] = 0
			r.blend(x, y, pr*c, pg*c, pb*c, pa*c)
		}
		b.rowMin[y], b.rowMax[y] = r.w, -1
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
		row := b.cov[y*r.w : (y+1)*r.w]
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
			if c > row[x] {
				row[x] = c
			}
			touched = true
		}
		if touched {
			b.rowMin[y] = min(b.rowMin[y], sx0)
			b.rowMax[y] = max(b.rowMax[y], sx1-1)
			b.yMin = min(b.yMin, y)
			b.yMax = max(b.yMax, y)
		}
	}
}
