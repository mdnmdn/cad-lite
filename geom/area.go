package geom

import "math"

// PolygonArea returns the signed area of the polygon through pts, closed by
// an implicit last edge: positive when the vertices run counter-clockwise.
func PolygonArea(pts []Vec2) float64 {
	var s float64
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		s += a.X*b.Y - b.X*a.Y
	}
	return s / 2
}

// ArcSegmentArea returns the signed area between a chord and its arc of
// radius r and signed sweep (counter-clockwise positive): r²/2 (sweep -
// sin sweep).
func ArcSegmentArea(r, sweep float64) float64 {
	return r * r / 2 * (sweep - math.Sin(sweep))
}

// PolylineArea returns the signed area enclosed by p treated as closed: the
// last vertex is joined to the first by a straight chord even when p.Closed
// is false. It is the shoelace area of the vertices plus ArcSegmentArea of
// every bulged segment; a counter-clockwise polygon is positive, and a
// positive bulge adds area when the polygon is counter-clockwise.
func PolylineArea(p Polyline) float64 {
	if len(p.Pts) < 2 {
		return 0
	}
	a := PolygonArea(p.Pts)
	for i := 0; i < p.NumSegments(); i++ {
		b := p.bulge(i)
		if b == 0 {
			continue
		}
		from, to := p.Pts[i], p.Pts[(i+1)%len(p.Pts)]
		l := from.Dist(to)
		if l == 0 {
			continue
		}
		sweep := 4 * math.Atan(b)
		r := math.Abs(l / (2 * math.Sin(sweep/2)))
		a += ArcSegmentArea(r, sweep)
	}
	return a
}
