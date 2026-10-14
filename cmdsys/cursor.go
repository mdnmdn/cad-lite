package cmdsys

import (
	"math"

	"go-draw/geom"
	"go-draw/render"
)

// This file is the pointer side of the session: the effective cursor (snap, then ortho), clicks,
// box selection and the command overlay. Drafting-aid work (typed snaps, polar, grid snap) lands
// here; the command line side is session.go.

// Cursor returns the effective cursor: snapped, or ortho-constrained.
func (s *Session) Cursor() geom.Vec2 { return s.cursor }

// Snap returns the snap the cursor is on, if any.
func (s *Session) Snap() (SnapHit, bool) { return s.snap, s.snapped }

// aperture returns the pick/snap radius in model units.
func (s *Session) aperture() float64 { return s.Aperture * s.ctx.View.PixelSize() }

// Hover updates the cursor from a raw model point: object snap first, then
// ortho relative to the prompt's base.
func (s *Session) Hover(p geom.Vec2) {
	s.snapped = false
	if s.SnapOn && s.wantsPoint() {
		if hit, ok := FindSnap(s.ctx.Doc, s.ctx.Index, p, s.aperture(), s.Snaps); ok {
			s.snap, s.snapped = hit, true
			s.cursor = hit.Point
			return
		}
	}
	s.cursor = s.constrain(p)
}

func (s *Session) wantsPoint() bool {
	return s.prompt != nil && s.prompt.Accepts&(InputPoint|InputDistance|InputAngle) != 0
}

func (s *Session) constrain(p geom.Vec2) geom.Vec2 {
	if !s.Ortho || s.prompt == nil || s.prompt.Base == nil {
		return p
	}
	b := *s.prompt.Base
	d := p.Sub(b)
	if math.Abs(d.X) >= math.Abs(d.Y) {
		return geom.V(p.X, b.Y)
	}
	return geom.V(b.X, p.Y)
}

// Click handles a primary click at raw model point p. At a point, distance
// or angle prompt it answers with the snapped cursor; at a selection prompt
// or when idle it picks the entity under the cursor (toggling it off when
// remove is set).
func (s *Session) Click(p geom.Vec2, remove bool) {
	s.Hover(p)
	if s.prompt != nil {
		pr := s.prompt
		switch {
		case pr.Accepts&InputPoint != 0:
			s.respond(Input{Kind: InputPoint, Point: s.cursor}, FormatPoint(s.cursor))
			return
		case pr.Accepts&InputDistance != 0 && pr.Base != nil:
			v := s.cursor.Dist(*pr.Base)
			s.respond(Input{Kind: InputDistance, Value: v, Point: s.cursor}, FormatNumber(v))
			return
		case pr.Accepts&InputAngle != 0 && pr.Base != nil:
			v := s.cursor.Sub(*pr.Base).Angle()
			s.respond(Input{Kind: InputAngle, Value: v, Point: s.cursor}, FormatNumber(v*180/math.Pi))
			return
		case pr.Accepts&InputSelection == 0:
			return
		}
	}
	h, ok := Pick(s.ctx.Doc, s.ctx.Index, p, s.aperture())
	switch {
	case !ok:
	case remove:
		s.ctx.Selection.Remove(h)
	default:
		s.ctx.Selection.Add(h)
	}
}

// SelectBox handles a box drag from a to b (raw model points): a window or
// crossing selection when idle or at a selection prompt.
func (s *Session) SelectBox(a, b geom.Vec2, remove bool) {
	if s.prompt != nil && s.prompt.Accepts&InputSelection == 0 {
		return
	}
	hs := SelectBox(s.ctx.Doc, s.ctx.Index, a, b, s.ctx.View.PixelSize()/2)
	if remove {
		s.ctx.Selection.Remove(hs...)
	} else {
		s.ctx.Selection.Add(hs...)
	}
}

// Overlay appends the active command's rubber-band preview for the current
// cursor.
func (s *Session) Overlay(dl *render.DisplayList) {
	if s.active != nil && s.prompt != nil {
		s.guard(func() { s.active.Preview(&s.ctx, s.cursor, dl) })
	}
}
