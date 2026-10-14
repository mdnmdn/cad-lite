package cmdsys

import (
	"math"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/spatial"
)

// SnapMode is one object snap, as a bit in a mode set. The bit values are AutoCAD's OSMODE
// values, so a mode set is the OSMODE variable without its 16384 "off" bit.
type SnapMode uint16

const (
	// SnapEndpoint snaps to the ends of lines, arcs and polyline vertices.
	SnapEndpoint SnapMode = 1 << iota
	// SnapMidpoint snaps to the middle of lines, arcs and polyline segments.
	SnapMidpoint
	// SnapCenter snaps to the centre of circles and arcs.
	SnapCenter
	// SnapNode snaps to point objects. No object Cad-lite draws is a node yet.
	SnapNode
	// SnapQuadrant snaps to the 0/90/180/270 degree points of circles and arcs.
	SnapQuadrant
	// SnapIntersection snaps to where two curves cross.
	SnapIntersection
	// SnapInsertion snaps to block and text insertion points (no such objects yet).
	SnapInsertion
	// SnapPerpendicular snaps to the foot of the perpendicular from the previous point.
	SnapPerpendicular
	// SnapTangent snaps to the tangent point of a line from the previous point to a circle or arc.
	SnapTangent
	// SnapNearest snaps to the nearest point on a curve.
	SnapNearest
	// SnapQuick is stored for OSMODE but has no effect.
	SnapQuick
	// SnapApparent is stored for OSMODE but has no effect.
	SnapApparent
	// SnapExtension is stored for OSMODE but has no effect.
	SnapExtension
	// SnapParallel is stored for OSMODE but has no effect.
	SnapParallel
)

// osmodeOff is the OSMODE bit that suppresses the running snaps (F3 off).
const osmodeOff = 16384

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
	case SnapNode:
		return t.T("snap.node")
	case SnapQuadrant:
		return t.T("snap.quadrant")
	case SnapIntersection:
		return t.T("snap.intersection")
	case SnapInsertion:
		return t.T("snap.insertion")
	case SnapPerpendicular:
		return t.T("snap.perpendicular")
	case SnapTangent:
		return t.T("snap.tangent")
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
	return FindSnapFrom(d, ix, p, aperture, modes, nil)
}

// FindSnapFrom is FindSnap with the previous point base, which Perpendicular and Tangent measure
// from (nil: those two find nothing).
func FindSnapFrom(d *doc.Drawing, ix *spatial.Index, p geom.Vec2, aperture float64, modes SnapMode, base *geom.Vec2) (SnapHit, bool) {
	return findSnap(snapQuery{d: d, ix: ix, p: p, aperture: aperture, modes: modes, base: base})
}

// FindTyped resolves a typed object snap override: p is the pick, the objects within aperture of it
// are the candidates, and the nearest snap point of the listed modes on them wins, wherever it
// lies (the pick only has to touch the object). Intersection needs both curves within the
// aperture of the pick (spec V31).
func FindTyped(d *doc.Drawing, ix *spatial.Index, p geom.Vec2, aperture float64, modes SnapMode, base *geom.Vec2) (SnapHit, bool) {
	return findSnap(snapQuery{d: d, ix: ix, p: p, aperture: aperture, modes: modes, base: base, typed: true})
}

type snapQuery struct {
	d        *doc.Drawing
	ix       *spatial.Index
	p        geom.Vec2
	aperture float64
	modes    SnapMode
	base     *geom.Vec2
	typed    bool
}

func findSnap(q snapQuery) (SnapHit, bool) {
	modes, p, aperture := q.modes, q.p, q.aperture
	if modes == 0 {
		return SnapHit{}, false
	}
	var curves []geom.Curve
	n := 0
	for _, h := range q.ix.Near(p, aperture) {
		e := q.d.Entity(h)
		if e == nil || isUnknown(e) || layerOff(q.d, e) {
			continue
		}
		if n >= snapCandidateLimit {
			break
		}
		n++
		for _, c := range e.Geometry() {
			for _, pc := range geom.Primitives(c) {
				if q.typed && geom.Closest(pc, p).Dist(p) > aperture {
					continue
				}
				curves = append(curves, pc)
			}
		}
	}
	best, bestD, found := SnapHit{}, math.Inf(1), false
	consider := func(pt geom.Vec2, m SnapMode) {
		if modes&m == 0 {
			return
		}
		dd := pt.Dist(p)
		if !q.typed && dd > aperture {
			return
		}
		if dd < bestD-1e-12 || (math.Abs(dd-bestD) <= 1e-12 && m < best.Mode) {
			best, bestD, found = SnapHit{Point: pt, Mode: m}, dd, true
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
			for k := 0; k < 4; k++ {
				consider(c.PointAt(float64(k)*math.Pi/2), SnapQuadrant)
			}
		case geom.Arc:
			consider(c.StartPoint(), SnapEndpoint)
			consider(c.EndPoint(), SnapEndpoint)
			consider(c.MidPoint(), SnapMidpoint)
			consider(c.C, SnapCenter)
			for k := 0; k < 4; k++ {
				if t := float64(k) * math.Pi / 2; c.ContainsAngle(t) {
					consider(c.PointAt(t), SnapQuadrant)
				}
			}
		}
		if q.base != nil {
			for _, f := range perpendicularFeet(c, *q.base) {
				consider(f, SnapPerpendicular)
			}
			for _, f := range tangentPoints(c, *q.base) {
				consider(f, SnapTangent)
			}
		}
	}
	if modes&SnapIntersection != 0 {
		tol := aperture * 1e-6
		for i := range curves {
			for j := i + 1; j < len(curves); j++ {
				for _, pt := range geom.Intersect(curves[i], curves[j], tol) {
					consider(pt, SnapIntersection)
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

// perpendicularFeet returns the points of c where the line from b meets c at a right angle: the
// foot on the (infinite) line, the two points of a circle on the ray through its centre, those
// of them on an arc.
func perpendicularFeet(c geom.Curve, b geom.Vec2) []geom.Vec2 {
	switch c := c.(type) {
	case geom.Line:
		d := c.B.Sub(c.A)
		l2 := d.Dot(d)
		if l2 == 0 {
			return nil
		}
		return []geom.Vec2{c.A.Add(d.Scale(b.Sub(c.A).Dot(d) / l2))}
	case geom.Circle:
		return radialPoints(c.C, c.R, b, nil)
	case geom.Arc:
		return radialPoints(c.C, c.R, b, &c)
	}
	return nil
}

// radialPoints returns the points of the circle (c, r) on the line through its centre and b,
// limited to arc when it is not nil. It returns nothing when b is the centre.
func radialPoints(c geom.Vec2, r float64, b geom.Vec2, arc *geom.Arc) []geom.Vec2 {
	d := b.Sub(c)
	if d.Len() < 1e-12 {
		return nil
	}
	a := d.Angle()
	var out []geom.Vec2
	for _, t := range []float64{a, a + math.Pi} {
		if arc == nil || arc.ContainsAngle(t) {
			out = append(out, c.Polar(r, t))
		}
	}
	return out
}

// tangentPoints returns the points where a line from b touches the circle or arc c (none when b is
// on or inside the circle, or c has no tangent).
func tangentPoints(c geom.Curve, b geom.Vec2) []geom.Vec2 {
	var ctr geom.Vec2
	var r float64
	var arc *geom.Arc
	switch c := c.(type) {
	case geom.Circle:
		ctr, r = c.C, c.R
	case geom.Arc:
		ctr, r, arc = c.C, c.R, &c
	default:
		return nil
	}
	d := b.Sub(ctr)
	dist := d.Len()
	if dist <= r*(1+1e-9) {
		return nil
	}
	a, off := d.Angle(), math.Acos(r/dist)
	var out []geom.Vec2
	for _, t := range []float64{a - off, a + off} {
		if arc == nil || arc.ContainsAngle(t) {
			out = append(out, ctr.Polar(r, t))
		}
	}
	return out
}

func layerOff(d *doc.Drawing, e doc.Entity) bool {
	return !d.EntityVisible(e)
}

// isUnknown reports whether e is an unmodelled entity kept only for the file
// round trip; it is neither selectable nor snappable.
func isUnknown(e doc.Entity) bool {
	_, ok := e.(*doc.Unknown)
	return ok
}

func layerLocked(d *doc.Drawing, e doc.Entity) bool {
	return !d.EntityEditable(e)
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
