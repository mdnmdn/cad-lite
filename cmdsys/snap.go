package cmdsys

import (
	"math"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/i18n"
	"go-draw/spatial"
)

// SnapMode is one object snap, as a bit in a mode set.
type SnapMode uint16

const (
	// SnapEndpoint snaps to the ends of lines, arcs and polyline vertices.
	SnapEndpoint SnapMode = 1 << iota
	// SnapMidpoint snaps to the middle of lines, arcs and polyline segments.
	SnapMidpoint
	// SnapCenter snaps to the centre of circles and arcs.
	SnapCenter
	// SnapQuadrant snaps to the 0/90/180/270 degree points of circles and arcs.
	SnapQuadrant
	// SnapIntersection snaps to where two curves cross.
	SnapIntersection
	// SnapNearest snaps to the nearest point on a curve.
	SnapNearest
)

// DefaultSnaps is the mode set a new session starts with.
const DefaultSnaps = SnapEndpoint | SnapMidpoint | SnapCenter | SnapQuadrant | SnapIntersection

// String returns the AutoCAD tooltip name of a single mode in the current language.
func (m SnapMode) String() string { return m.In(i18n.For(i18n.Current())) }

// In returns the name of a single mode in the language of t.
func (m SnapMode) In(t i18n.Translator) string {
	switch m {
	case SnapEndpoint:
		return t.T("snap.endpoint")
	case SnapMidpoint:
		return t.T("snap.midpoint")
	case SnapCenter:
		return t.T("snap.center")
	case SnapQuadrant:
		return t.T("snap.quadrant")
	case SnapIntersection:
		return t.T("snap.intersection")
	case SnapNearest:
		return t.T("snap.nearest")
	}
	return t.T("snap.generic")
}

// SnapHit is a found snap point.
type SnapHit struct {
	Point geom.Vec2
	Mode  SnapMode
}

// snapCandidateLimit caps how many nearby entities take part in the
// pairwise intersection search per cursor move.
const snapCandidateLimit = 24

// FindSnap returns the snap point of the enabled modes closest to p within
// aperture (model units). Ties go to the mode declared first (Endpoint beats
// Intersection beats Nearest). Entities on layers that are off are ignored.
func FindSnap(d *doc.Drawing, ix *spatial.Index, p geom.Vec2, aperture float64, modes SnapMode) (SnapHit, bool) {
	if modes == 0 {
		return SnapHit{}, false
	}
	var curves []geom.Curve
	for _, h := range ix.Near(p, aperture) {
		e := d.Entity(h)
		if e == nil || isUnknown(e) || layerOff(d, e) {
			continue
		}
		if len(curves) >= snapCandidateLimit {
			break
		}
		for _, c := range e.Geometry() {
			curves = append(curves, geom.Primitives(c)...)
		}
	}
	best, bestD, found := SnapHit{}, math.Inf(1), false
	consider := func(q geom.Vec2, m SnapMode) {
		if modes&m == 0 {
			return
		}
		dd := q.Dist(p)
		if dd > aperture {
			return
		}
		if dd < bestD-1e-12 || (math.Abs(dd-bestD) <= 1e-12 && m < best.Mode) {
			best, bestD, found = SnapHit{Point: q, Mode: m}, dd, true
		}
	}
	for _, c := range curves {
		switch c := c.(type) {
		case geom.Line:
			consider(c.A, SnapEndpoint)
			consider(c.B, SnapEndpoint)
			consider(c.Mid(), SnapMidpoint)
		case geom.Circle:
			consider(c.C, SnapCenter)
			for q := 0; q < 4; q++ {
				consider(c.PointAt(float64(q)*math.Pi/2), SnapQuadrant)
			}
		case geom.Arc:
			consider(c.StartPoint(), SnapEndpoint)
			consider(c.EndPoint(), SnapEndpoint)
			consider(c.MidPoint(), SnapMidpoint)
			consider(c.C, SnapCenter)
			for q := 0; q < 4; q++ {
				if t := float64(q) * math.Pi / 2; c.ContainsAngle(t) {
					consider(c.PointAt(t), SnapQuadrant)
				}
			}
		}
	}
	if modes&SnapIntersection != 0 {
		tol := aperture * 1e-6
		for i := range curves {
			for j := i + 1; j < len(curves); j++ {
				for _, q := range geom.Intersect(curves[i], curves[j], tol) {
					consider(q, SnapIntersection)
				}
			}
		}
	}
	if !found && modes&SnapNearest != 0 {
		for _, c := range curves {
			consider(geom.Closest(c, p), SnapNearest)
		}
	}
	return best, found
}

func layerOff(d *doc.Drawing, e doc.Entity) bool {
	l := d.Layer(e.Common().Layer)
	return l != nil && l.Off
}

// isUnknown reports whether e is an unmodelled entity kept only for the file
// round trip; it is neither selectable nor snappable.
func isUnknown(e doc.Entity) bool {
	_, ok := e.(*doc.Unknown)
	return ok
}

func layerLocked(d *doc.Drawing, e doc.Entity) bool {
	l := d.Layer(e.Common().Layer)
	return l != nil && (l.Locked || l.Off)
}

// Pick returns the selectable entity closest to p within aperture, the
// newest one on a tie.
func Pick(d *doc.Drawing, ix *spatial.Index, p geom.Vec2, aperture float64) (doc.Handle, bool) {
	var best doc.Handle
	bestD := math.Inf(1)
	for _, h := range ix.Near(p, aperture) {
		e := d.Entity(h)
		if e == nil || isUnknown(e) || layerLocked(d, e) {
			continue
		}
		for _, c := range e.Geometry() {
			if dd := geom.Closest(c, p).Dist(p); dd <= aperture && dd <= bestD {
				best, bestD = h, dd
			}
		}
	}
	return best, best != 0
}

// SelectBox returns the selectable entities in the box from a to b. A box
// dragged left to right is a window (entities entirely inside); right to
// left is a crossing (entities inside or touching). tol is the
// tessellation tolerance for the crossing test.
func SelectBox(d *doc.Drawing, ix *spatial.Index, a, b geom.Vec2, tol float64) []doc.Handle {
	box := geom.BoxOf(a, b)
	crossing := b.X < a.X
	var out []doc.Handle
	for _, h := range ix.Query(box) {
		e := d.Entity(h)
		if e == nil || isUnknown(e) || layerLocked(d, e) {
			continue
		}
		eb := e.Bounds()
		if box.ContainsBox(eb) {
			out = append(out, h)
			continue
		}
		if crossing && touchesBox(e, box, tol) {
			out = append(out, h)
		}
	}
	return out
}

func touchesBox(e doc.Entity, box geom.Box2, tol float64) bool {
	for _, c := range e.Geometry() {
		pts := c.Tessellate(tol, nil)
		for i := 1; i < len(pts); i++ {
			if geom.SegmentIntersectsBox(pts[i-1], pts[i], box) {
				return true
			}
		}
	}
	return false
}
