package geom

import "math"

// CornerCut says how one vertex of a polyline is cut back: A is the distance removed from the
// segment arriving at the vertex, B the distance removed from the segment leaving it, and Bulge
// the bulge of the new segment that joins the two cut points (zero for a chamfer).
type CornerCut struct {
	A, B, Bulge float64
}

// CutFunc decides the cut of one vertex. u1 is the unit vector from the vertex back along the
// arriving segment, u2 the unit vector along the leaving one, la and lb the segment lengths; turn
// is positive when the polyline turns left at the vertex. It returns false to leave the vertex.
type CutFunc func(u1, u2 Vec2, la, lb, turn float64) (CornerCut, bool)

// FilletCut returns the CutFunc that rounds a vertex with radius r.
func FilletCut(r float64) CutFunc {
	return func(u1, u2 Vec2, _, _, turn float64) (CornerCut, bool) {
		phi := math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2))))
		if r <= 0 || phi < 1e-9 || phi > math.Pi-1e-9 {
			return CornerCut{}, false
		}
		t := r / math.Tan(phi/2)
		b := math.Tan((math.Pi - phi) / 4)
		if turn < 0 {
			b = -b
		}
		return CornerCut{A: t, B: t, Bulge: b}, true
	}
}

// ChamferCut returns the CutFunc that bevels a vertex, cutting d1 off the arriving segment and
// d2 off the leaving one.
func ChamferCut(d1, d2 float64) CutFunc {
	return func(u1, u2 Vec2, _, _, _ float64) (CornerCut, bool) {
		phi := math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2))))
		if (d1 <= 0 && d2 <= 0) || phi < 1e-9 || phi > math.Pi-1e-9 {
			return CornerCut{}, false
		}
		return CornerCut{A: d1, B: d2}, true
	}
}

// ChamferAngleDistance returns the distance along the second line for a chamfer that starts length
// along the first line from the corner and makes angle alpha with it, in a wedge of angle phi
// between the lines. It returns false when the chamfer never meets the second line.
func ChamferAngleDistance(phi, length, alpha float64) (float64, bool) {
	s := math.Sin(phi + alpha)
	if s <= 1e-12 || alpha <= 0 || length <= 0 {
		return 0, false
	}
	return length * math.Sin(alpha) / s, true
}

// CornerWedge returns the angle between the picked sides of the infinite lines l1 and l2.
func CornerWedge(l1, l2 Line, p1, p2 Vec2) (float64, error) {
	_, u1, u2, _, _, err := cornerRays(l1, l2, p1, p2)
	if err != nil {
		return 0, err
	}
	return math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2)))), nil
}

// CutPolyline cuts the corners of p chosen by cut. Only a vertex between two straight segments is
// cut, and only when the cut fits in what the neighbouring cuts leave of both segments. widths, the
// optional {start, end} width of the segment leaving each vertex, is interpolated at every cut
// point and returned for the new polyline (nil when widths is nil). cut counts the vertices
// changed and skipped those that were asked for but did not fit.
func CutPolyline(p Polyline, widths [][2]float64, cut CutFunc) (out Polyline, outW [][2]float64, cutN, skipped int) {
	n := len(p.Pts)
	nseg := n - 1
	if p.Closed {
		nseg = n
	}
	if n < 3 || nseg < 2 {
		return p, widths, 0, 0
	}
	seg := func(i int) (a, b Vec2) { return p.Pts[i], p.Pts[(i+1)%n] }
	straight := func(i int) bool { return i < len(p.Bulges) && p.Bulges[i] == 0 || i >= len(p.Bulges) }
	s := make([]float64, nseg) // fraction kept from the start of segment i
	e := make([]float64, nseg) // fraction kept to the end of segment i
	for i := range e {
		e[i] = 1
	}
	corner := make([]*CornerCut, n)
	first, last := 1, n-2
	if p.Closed {
		first, last = 0, n-1
	}
	for j := first; j <= last; j++ {
		in, outSeg := (j-1+nseg)%nseg, j%nseg
		if !straight(in) || !straight(outSeg) {
			continue
		}
		a0, a1 := seg(in)
		b0, b1 := seg(outSeg)
		la, lb := a0.Dist(a1), b0.Dist(b1)
		if la < 1e-12 || lb < 1e-12 {
			continue
		}
		u1, u2 := a0.Sub(a1).Scale(1/la), b1.Sub(b0).Scale(1/lb)
		turn := a1.Sub(a0).Cross(b1.Sub(b0))
		c, ok := cut(u1, u2, la, lb, turn)
		if !ok {
			continue
		}
		ea, sb := e[in]-c.A/la, s[outSeg]+c.B/lb
		if ea < s[in]-1e-9 || sb > e[outSeg]+1e-9 || c.A < 0 || c.B < 0 || math.IsNaN(c.A+c.B) {
			skipped++
			continue
		}
		if in == outSeg { // a two-segment closed polyline cannot lose both ends of one segment
			skipped++
			continue
		}
		e[in], s[outSeg] = ea, sb
		cc := c
		corner[j] = &cc
		cutN++
	}
	if cutN == 0 {
		return p, widths, 0, skipped
	}
	wAt := func(i int, f float64) float64 {
		if i >= len(widths) {
			return 0
		}
		return widths[i][0] + (widths[i][1]-widths[i][0])*f
	}
	emit := func(pt Vec2, bulge float64, w [2]float64) {
		out.Pts = append(out.Pts, pt)
		out.Bulges = append(out.Bulges, bulge)
		if widths != nil {
			outW = append(outW, w)
		}
	}
	for i := 0; i < nseg; i++ {
		a, b := seg(i)
		bulge := 0.0
		if i < len(p.Bulges) {
			bulge = p.Bulges[i]
		}
		w1 := wAt(i, e[i])
		emit(a.Lerp(b, s[i]), bulge, [2]float64{wAt(i, s[i]), w1})
		j := (i + 1) % n
		if c := corner[j]; c != nil {
			next := j % nseg
			emit(a.Lerp(b, e[i]), c.Bulge, [2]float64{w1, wAt(next, s[next])})
		}
	}
	out.Closed = p.Closed
	if !p.Closed {
		out.Pts = append(out.Pts, p.Pts[n-1])
		out.Bulges = append(out.Bulges, 0)
		if widths != nil {
			outW = append(outW, [2]float64{})
		}
	}
	return out, outW, cutN, skipped
}
