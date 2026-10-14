package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/geom"
	"go-draw/render"
)

// --- ARC ----------------------------------------------------------------

type arcStep int

const (
	arcStart       arcStep = iota // start point or [Center]; Enter continues
	arcSecond                     // second point or [Center/End]
	arcThird                      // end point of the three-point arc
	arcCenterAfter                // centre, after Start then Center
	arcCenterFirst                // centre, after the first Center
	arcStartFirst                 // start point, after Center first
	arcCEnd                       // Start, Center: end point or [Angle/chord Length]
	arcCAngle                     // Start, Center, Angle: included angle
	arcCLength                    // Start, Center, Length: chord length
	arcEnd2                       // Start, End: end point
	arcEC                         // Start, End: centre or [Angle/Direction/Radius]
	arcEAngle                     // Start, End, Angle: included angle
	arcEDirection                 // Start, End, Direction: tangent direction at the start
	arcERadius                    // Start, End, Radius: radius
	arcContinue                   // Enter at the first prompt: end point, tangent to the last
)

// arcCmd implements ARC with every sequence AutoCAD offers: three points, Start-Center-End/
// Angle/Length, Start-End-Angle/Direction/Radius, Center-Start-..., and Enter to continue
// tangent from the last line or arc.
type arcCmd struct {
	base
	step arcStep
	s    geom.Vec2 // start
	p2   geom.Vec2 // second point of the three-point arc
	c    geom.Vec2 // centre
	e    geom.Vec2 // end
	tan  float64   // tangent direction of the continued arc
}

func (*arcCmd) Name() string { return "ARC" }

func (c *arcCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.prompt(ctx) }

func (c *arcCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	pointP := func(key string, base *geom.Vec2, kws ...string) *cmdsys.Prompt {
		p := &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputPoint, Base: base}
		if len(kws) > 0 {
			p.Accepts |= cmdsys.InputKeyword
			p.Keywords = kws
		}
		return p
	}
	switch c.step {
	case arcSecond:
		return pointP("cmd.arc.prompt.second_point", pt(c.s), "Center", "End")
	case arcThird:
		return pointP("cmd.arc.prompt.end_point", pt(c.p2))
	case arcCenterAfter:
		return pointP("cmd.arc.prompt.center", pt(c.s))
	case arcCenterFirst:
		return pointP("cmd.arc.prompt.center", nil)
	case arcStartFirst:
		return pointP("cmd.arc.prompt.start_point", pt(c.c))
	case arcCEnd:
		return pointP("cmd.arc.prompt.end_point", pt(c.c), "Angle", "chord Length")
	case arcCAngle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.included_angle"), Accepts: cmdsys.InputAngle | cmdsys.InputPoint, Base: pt(c.c)}
	case arcCLength:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.chord_length"), Accepts: cmdsys.InputDistance, Base: pt(c.s)}
	case arcEnd2, arcContinue:
		return pointP("cmd.arc.prompt.end_point", pt(c.s))
	case arcEC:
		return pointP("cmd.arc.prompt.center", pt(c.e), "Angle", "Direction", "Radius")
	case arcEAngle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.included_angle"), Accepts: cmdsys.InputAngle}
	case arcEDirection:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.tangent_direction"), Accepts: cmdsys.InputAngle, Base: pt(c.s)}
	case arcERadius:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.radius"), Accepts: cmdsys.InputDistance, Base: pt(c.s)}
	}
	p := pointP("cmd.arc.prompt.start_point", nil, "Center")
	p.Accepts |= cmdsys.InputEnter
	return p
}

// atPoint returns the arc the point cur completes at a point-taking step.
func (c *arcCmd) atPoint(cur geom.Vec2) (arcSeg, bool) {
	switch c.step {
	case arcThird:
		return seg3(c.s, c.p2, cur)
	case arcCEnd:
		return segCenterEnd(c.s, c.c, cur)
	case arcEC:
		return segCenterEnd(c.s, cur, c.e)
	case arcContinue:
		return segTangent(c.s, c.tan, cur)
	}
	return arcSeg{}, false
}

// angleFromPoint turns a click at an included-angle prompt into the signed angle between the
// start and the point as seen from the centre.
func (c *arcCmd) angleFromPoint(p geom.Vec2) float64 {
	return signedAngle(c.s.Sub(c.c), p.Sub(c.c))
}

// atValue returns the arc a number completes at a value-taking step.
func (c *arcCmd) atValue(v float64) (arcSeg, bool) {
	switch c.step {
	case arcCAngle:
		return segCenterAngle(c.s, c.c, v)
	case arcCLength:
		return segCenterChord(c.s, c.c, v)
	case arcEAngle:
		return segChordAngle(c.s, c.e, v)
	case arcEDirection:
		return segTangent(c.s, v, c.e)
	case arcERadius:
		return segChordRadius(c.s, c.e, v)
	}
	return arcSeg{}, false
}

func (c *arcCmd) commit(ctx *cmdsys.Context, a arcSeg) *cmdsys.Prompt {
	ctx.Tx().Add(a.Entity())
	ctx.SetLastEnd(a.E, a.EndTangent())
	return nil
}

func (c *arcCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	isPt := in.Kind == cmdsys.InputPoint
	switch c.step {
	case arcStart:
		switch {
		case in.Kind == cmdsys.InputEnter:
			p, tan, ok := ctx.LastEnd()
			if !ok {
				return nil
			}
			c.s, c.tan, c.step = p, tan, arcContinue
		case in.Keyword == "Center":
			c.step = arcCenterFirst
		case isPt:
			c.s, c.step = in.Point, arcSecond
		}
	case arcSecond:
		switch {
		case in.Keyword == "Center":
			c.step = arcCenterAfter
		case in.Keyword == "End":
			c.step = arcEnd2
		case isPt:
			c.p2, c.step = in.Point, arcThird
		}
	case arcCenterAfter:
		if isPt {
			c.c, c.step = in.Point, arcCEnd
		}
	case arcCenterFirst:
		if isPt {
			c.c, c.step = in.Point, arcStartFirst
		}
	case arcStartFirst:
		if isPt {
			c.s, c.step = in.Point, arcCEnd
		}
	case arcCEnd:
		switch {
		case in.Keyword == "Angle":
			c.step = arcCAngle
		case in.Keyword == "chord Length":
			c.step = arcCLength
		case isPt:
			return c.finishPoint(ctx, in.Point)
		}
	case arcEnd2:
		if isPt {
			c.e, c.step = in.Point, arcEC
		}
	case arcEC:
		switch {
		case in.Keyword == "Angle":
			c.step = arcEAngle
		case in.Keyword == "Direction":
			c.step = arcEDirection
		case in.Keyword == "Radius":
			c.step = arcERadius
		case isPt:
			return c.finishPoint(ctx, in.Point)
		}
	case arcThird, arcContinue:
		if isPt {
			if c.step == arcThird {
				if _, ok := c.atPoint(in.Point); !ok {
					ctx.Say("cmd.arc.msg.collinear")
					return nil
				}
			}
			return c.finishPoint(ctx, in.Point)
		}
	case arcCAngle, arcCLength, arcEAngle, arcEDirection, arcERadius:
		v := in.Value
		switch {
		case isPt && c.step == arcCAngle:
			v = c.angleFromPoint(in.Point)
		case in.Kind != cmdsys.InputAngle && in.Kind != cmdsys.InputDistance:
			return c.prompt(ctx)
		}
		if a, ok := c.atValue(v); ok {
			return c.commit(ctx, a)
		}
		ctx.Say("cmd.arc.msg.no_solution")
	}
	return c.prompt(ctx)
}

// finishPoint completes the arc at a point-taking step, or reports that no arc fits.
func (c *arcCmd) finishPoint(ctx *cmdsys.Context, p geom.Vec2) *cmdsys.Prompt {
	if a, ok := c.atPoint(p); ok {
		return c.commit(ctx, a)
	}
	ctx.Say("cmd.arc.msg.no_solution")
	return c.prompt(ctx)
}

func (c *arcCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	tol := previewTol(ctx)
	guide := func(a, b geom.Vec2) { dl.Polyline([]geom.Vec2{a, b}, PreviewColor.WithAlpha(120), 0, false) }
	switch c.step {
	case arcSecond, arcEnd2:
		guide(c.s, cur)
	case arcCenterAfter:
		guide(c.s, cur)
	case arcStartFirst:
		guide(c.c, cur)
	case arcThird, arcCEnd, arcEC, arcContinue:
		if a, ok := c.atPoint(cur); ok {
			dl.Curve(a.Arc(), tol, PreviewColor, 1)
		}
		switch c.step {
		case arcCEnd:
			guide(c.c, c.s)
			guide(c.c, cur)
		case arcThird:
			guide(c.s, c.p2)
		}
	case arcCAngle:
		if a, ok := c.atValue(c.angleFromPoint(cur)); ok {
			dl.Curve(a.Arc(), tol, PreviewColor, 1)
		}
	case arcCLength:
		if a, ok := c.atValue(cur.Dist(c.s)); ok {
			dl.Curve(a.Arc(), tol, PreviewColor, 1)
		}
	case arcEDirection:
		if a, ok := c.atValue(cur.Sub(c.s).Angle()); ok {
			dl.Curve(a.Arc(), tol, PreviewColor, 1)
		}
	case arcERadius:
		if a, ok := c.atValue(math.Max(cur.Dist(c.s), 1e-9)); ok {
			dl.Curve(a.Arc(), tol, PreviewColor, 1)
		}
	}
}
