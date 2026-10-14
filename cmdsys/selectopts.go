package cmdsys

import (
	"go-draw/doc"
	"go-draw/geom"
)

// This file is the pure geometry behind the typed selection options (W C WP CP F ...): which
// tessellated paths lie inside a polygon, cross it, or cross a fence. Nothing here touches a
// session; select.go drives it.

// Selection option ids, in the order the "Invalid selection" help lists them. The id of a
// keyword is its English spelling with the shortcut in capitals, as for every prompt keyword.
const (
	selOptWindow   = "Window"
	selOptCrossing = "Crossing"
	selOptBox      = "BOX"
	selOptAll      = "ALL"
	selOptFence    = "Fence"
	selOptWPolygon = "WPolygon"
	selOptCPolygon = "CPolygon"
	selOptLast     = "Last"
	selOptPrevious = "Previous"
	selOptAdd      = "Add"
	selOptRemove   = "Remove"
	selOptMultiple = "Multiple"
	selOptUndo     = "Undo"
	selOptAuto     = "AUto"
	selOptSingle   = "SIngle"
)

var selOptions = []string{selOptWindow, selOptLast, selOptCrossing, selOptBox, selOptAll, selOptFence,
	selOptWPolygon, selOptCPolygon, selOptAdd, selOptRemove, selOptMultiple, selOptPrevious,
	selOptUndo, selOptAuto, selOptSingle}

// PointInPolygon reports whether p is inside poly (even-odd rule; the polygon is implicitly
// closed). A point exactly on the boundary may fall either way.
func PointInPolygon(p geom.Vec2, poly []geom.Vec2) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

func orient(a, b, c geom.Vec2) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

func onSeg(a, b, p geom.Vec2) bool {
	return min(a.X, b.X) <= p.X && p.X <= max(a.X, b.X) && min(a.Y, b.Y) <= p.Y && p.Y <= max(a.Y, b.Y)
}

// SegmentsIntersect reports whether the closed segments ab and cd share a point (touching and
// collinear overlap count).
func SegmentsIntersect(a, b, c, d geom.Vec2) bool {
	o1, o2, o3, o4 := orient(a, b, c), orient(a, b, d), orient(c, d, a), orient(c, d, b)
	if ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) && ((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0)) {
		return true
	}
	return (o1 == 0 && onSeg(a, b, c)) || (o2 == 0 && onSeg(a, b, d)) ||
		(o3 == 0 && onSeg(c, d, a)) || (o4 == 0 && onSeg(c, d, b))
}

// pathHitsEdges reports whether any segment of path meets any segment of line; closed adds
// the edge from the last point of line back to the first.
func pathHitsEdges(path, line []geom.Vec2, closed bool) bool {
	n := len(line)
	for i := 1; i <= n; i++ {
		var c, d geom.Vec2
		switch {
		case i < n:
			c, d = line[i-1], line[i]
		case closed && n > 2:
			c, d = line[n-1], line[0]
		default:
			continue
		}
		if len(path) == 1 {
			if orient(c, d, path[0]) == 0 && onSeg(c, d, path[0]) {
				return true
			}
			continue
		}
		for k := 1; k < len(path); k++ {
			if SegmentsIntersect(path[k-1], path[k], c, d) {
				return true
			}
		}
	}
	return false
}

// PathsInsidePolygon reports whether every path lies entirely inside poly: all vertices inside
// and no segment crossing the boundary (so a concave polygon is respected). An empty path
// list is not inside.
func PathsInsidePolygon(paths [][]geom.Vec2, poly []geom.Vec2) bool {
	if len(paths) == 0 || len(poly) < 3 {
		return false
	}
	for _, p := range paths {
		for _, v := range p {
			if !PointInPolygon(v, poly) {
				return false
			}
		}
		if len(p) > 1 && pathHitsEdges(p, poly, true) {
			return false
		}
	}
	return true
}

// PathsCrossPolygon reports whether any path has a vertex inside poly or a segment meeting its
// boundary: the crossing-polygon test.
func PathsCrossPolygon(paths [][]geom.Vec2, poly []geom.Vec2) bool {
	if len(poly) < 3 {
		return false
	}
	for _, p := range paths {
		for _, v := range p {
			if PointInPolygon(v, poly) {
				return true
			}
		}
		if pathHitsEdges(p, poly, true) {
			return true
		}
	}
	return false
}

// PathsCrossFence reports whether any path meets the open polyline fence.
func PathsCrossFence(paths [][]geom.Vec2, fence []geom.Vec2) bool {
	for _, p := range paths {
		if pathHitsEdges(p, fence, false) {
			return true
		}
	}
	return false
}

// entityPaths tessellates e's geometry into polylines within tol; an entity without curves is
// its bounds corner as a single-point path.
func entityPaths(e doc.Entity, tol float64) [][]geom.Vec2 {
	var out [][]geom.Vec2
	for _, c := range e.Geometry() {
		if pts := c.Tessellate(tol, nil); len(pts) > 0 {
			out = append(out, pts)
		}
	}
	if len(out) == 0 {
		if b := e.Bounds(); !b.IsEmpty() {
			out = append(out, []geom.Vec2{b.Min})
		}
	}
	return out
}
