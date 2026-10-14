package cmdsys

import (
	"math"

	"cad-lite/geom"
	"cad-lite/render"
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

// Hover updates the cursor from a raw model point. At a point prompt it is, in order: the typed
// object snap override pending (if any), else the running object snaps, else the snap grid; then
// ortho relative to the prompt's base constrains what is left.
func (s *Session) Hover(p geom.Vec2) {
	s.snapped = false
	if s.wantsPoint() {
		o := &s.aids.ov
		live := s.overrideLive()
		switch {
		case live && o.armed && !o.none:
			if hit, ok := FindTyped(s.ctx.Doc, s.ctx.Index, p, s.aperture(), o.modes, s.snapBase()); ok {
				s.snap, s.snapped = hit, true
				s.cursor = hit.Point
				return
			}
		case live && o.none:
		case s.SnapOn:
			if hit, ok := FindSnapFrom(s.ctx.Doc, s.ctx.Index, p, s.aperture(), s.Snaps, s.snapBase()); ok {
				s.snap, s.snapped = hit, true
				s.cursor = hit.Point
				return
			}
		}
		if s.aids.snapGrid && !(live && o.none) {
			p = s.snapToGrid(p)
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

// Click handles a primary click at raw model point p. At a point, distance or angle prompt it
// answers with the snapped cursor (a point answer carries Input.Shift when remove, the Shift key,
// is set); at a selection prompt or when idle it picks the entity under the cursor, toggling it
// off when remove is set. In Remove mode of "Select objects:" a plain click removes and Shift
// adds, the reverse of the other modes.
func (s *Session) Click(p geom.Vec2, remove bool) {
	s.Hover(p)
	if s.prompt != nil {
		pr := s.prompt
		switch {
		case pr.Accepts&InputPoint != 0:
			if s.overrideLive() {
				pt := s.cursor
				if o := &s.aids.ov; o.armed && !o.none {
					pt = p
				}
				s.overrideAnswer(pt, "", remove)
				return
			}
			s.respond(Input{Kind: InputPoint, Point: s.cursor, Shift: remove}, FormatPoint(s.cursor))
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
		if s.inRemoveMode() {
			if remove {
				remove = false
			} else {
				s.Submit(FormatPoint(p))
				return
			}
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

// inRemoveMode reports whether the selection prompt shown is the Remove-mode one of the typed
// selection options.
func (s *Session) inRemoveMode() bool {
	return s.active != nil && s.prompt != nil && s.prompt.Accepts&InputSelection != 0 &&
		s.prompt.Message == s.ctx.T("cmd.select.prompt.remove_objects")
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
