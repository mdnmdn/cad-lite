package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// --- PLINE --------------------------------------------------------------

type plStep int

const (
	plStart        plStep = iota // start point
	plLine                       // next point in line mode
	plArcMode                    // end point in arc mode
	plWidthStart                 // starting (half-)width
	plWidthEnd                   // ending (half-)width
	plLength                     // length of a segment along the previous direction
	plAngle                      // Arc, Angle: included angle
	plAngleEnd                   // Arc, Angle: end point or [CEnter/Radius]
	plAngleCenter                // Arc, Angle, CEnter: centre
	plRadius                     // Arc, Radius: radius
	plRadiusEnd                  // Arc, Radius: end point or [Angle]
	plChordDir                   // Arc with an angle and a radius: direction of the chord
	plCenter                     // Arc, CEnter: centre
	plCenterEnd                  // Arc, CEnter: end point or [Angle/Length]
	plCenterAngle                // Arc, CEnter, Angle: included angle
	plCenterLen                  // Arc, CEnter, Length: chord length
	plDirection                  // Arc, Direction: tangent direction at the start
	plDirectionEnd               // Arc, Direction: end point
	plSecond                     // Arc, Second pt: second point
	plSecondEnd                  // Arc, Second pt: end point
	plRadiusAngle                // Arc, Radius, Angle: included angle
)

// plineCmd draws one LWPOLYLINE: line and arc segments (bulges), each with its own start and end
// width. Segment data is kept per segment (bulges, widths, tans: the direction the segment
// leaves its end), points per vertex, so Undo drops one of each.
type plineCmd struct {
	step plStep
	pts  []geom.Vec2
	// per segment, parallel to each other; the closing segment of a closed polyline is the last
	bulges []float64
	widths [][2]float64
	tans   []float64

	w0, w1  float64 // width of the segments to come
	half    bool    // the width prompts ask for half-widths
	arcMode bool
	// angKnown is set when Radius was chosen after Angle, so the direction of the chord follows
	angKnown bool

	// answers of the arc sub-prompts
	ang, rad, dir float64
	ctr, p2       geom.Vec2
}

func (*plineCmd) Name() string { return "PLINE" }

func (c *plineCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.w0 = ctx.Vars().Real("PLINEWID")
	c.w1 = c.w0
	return &cmdsys.Prompt{Message: ctx.T("cmd.pline.prompt.start_point"), Accepts: cmdsys.InputPoint}
}

func (c *plineCmd) last() geom.Vec2 { return c.pts[len(c.pts)-1] }

// tan returns the direction the polyline leaves its last vertex, 0 before any segment.
func (c *plineCmd) tan() float64 {
	if n := len(c.tans); n > 0 {
		return c.tans[n-1]
	}
	return 0
}

// modePrompt is the prompt of the current mode.
func (c *plineCmd) modePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.angKnown = false
	if c.arcMode {
		c.step = plArcMode
	} else {
		c.step = plLine
	}
	return c.prompt(ctx)
}

func (c *plineCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	pointKW := func(key string, base geom.Vec2, kws ...string) *cmdsys.Prompt {
		return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: kws, Base: pt(base)}
	}
	value := func(key string, kind cmdsys.InputKind, base *geom.Vec2, def *cmdsys.Input) *cmdsys.Prompt {
		return &cmdsys.Prompt{Message: ctx.T(key), Accepts: kind, Base: base, Default: def}
	}
	s := c.last
	switch c.step {
	case plLine:
		kws := []string{"Arc"}
		if len(c.pts) >= 3 {
			kws = append(kws, "Close")
		}
		kws = append(kws, "Halfwidth", "Length", "Undo", "Width")
		p := pointKW("cmd.pline.prompt.next_point", s(), kws...)
		p.Accepts |= cmdsys.InputEnter
		return p
	case plArcMode:
		kws := []string{"Arc angle", "Pline center"}
		if len(c.pts) >= 2 {
			kws = append(kws, "Close arc")
		}
		kws = append(kws, "Direction", "Halfwidth", "Line", "Radius", "Second pt", "Undo", "Width")
		p := pointKW("cmd.pline.prompt.arc_endpoint", s(), kws...)
		p.Accepts |= cmdsys.InputEnter
		return p
	case plWidthStart, plWidthEnd:
		key, w := "cmd.pline.prompt.start_width", c.w0
		switch {
		case c.step == plWidthStart && c.half:
			key, w = "cmd.pline.prompt.start_halfwidth", c.w0/2
		case c.step == plWidthEnd && !c.half:
			key, w = "cmd.pline.prompt.end_width", c.w1
		case c.step == plWidthEnd:
			key, w = "cmd.pline.prompt.end_halfwidth", c.w1/2
		}
		return value(key, cmdsys.InputDistance, pt(s()), &cmdsys.Input{Kind: cmdsys.InputDistance, Value: w})
	case plLength:
		return value("cmd.pline.prompt.length", cmdsys.InputDistance, pt(s()), nil)
	case plAngle, plCenterAngle, plRadiusAngle:
		return value("cmd.pline.prompt.included_angle", cmdsys.InputAngle, nil, nil)
	case plAngleEnd:
		return pointKW("cmd.pline.prompt.arc_endpoint", s(), "Pline center", "Radius")
	case plAngleCenter, plCenter:
		return value("cmd.pline.prompt.center", cmdsys.InputPoint, pt(s()), nil)
	case plRadius:
		return value("cmd.pline.prompt.radius", cmdsys.InputDistance, pt(s()), nil)
	case plRadiusEnd:
		return pointKW("cmd.pline.prompt.arc_endpoint", s(), "Arc angle")
	case plChordDir:
		return value("cmd.pline.prompt.chord_direction", cmdsys.InputAngle, pt(s()), &cmdsys.Input{Kind: cmdsys.InputAngle, Value: c.tan()})
	case plCenterEnd:
		return pointKW("cmd.pline.prompt.arc_endpoint", s(), "Arc angle", "Length")
	case plCenterLen:
		return value("cmd.pline.prompt.chord_length", cmdsys.InputDistance, pt(s()), nil)
	case plDirection:
		return value("cmd.pline.prompt.direction", cmdsys.InputAngle, pt(s()), nil)
	case plDirectionEnd, plSecondEnd:
		return value("cmd.pline.prompt.arc_endpoint", cmdsys.InputPoint, pt(s()), nil)
	case plSecond:
		return value("cmd.pline.prompt.second_point", cmdsys.InputPoint, pt(s()), nil)
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.pline.prompt.start_point"), Accepts: cmdsys.InputPoint}
}

// push adds a segment ending at end.
func (c *plineCmd) push(end geom.Vec2, bulge, tan float64) {
	c.pts = append(c.pts, end)
	c.bulges = append(c.bulges, bulge)
	c.widths = append(c.widths, [2]float64{c.w0, c.w1})
	c.tans = append(c.tans, tan)
}

func (c *plineCmd) pushLine(end geom.Vec2) { c.push(end, 0, end.Sub(c.last()).Angle()) }

// pushArc adds the arc a, whose start is the last vertex.
func (c *plineCmd) pushArc(a arcSeg) { c.push(a.E, a.Bulge(), a.EndTangent()) }

// pushClosing adds the closing segment back to the first vertex without a new vertex: a tangent
// arc when arc is set (a line when the tangent arc degenerates), else a line.
func (c *plineCmd) pushClosing(arc bool) {
	first := c.pts[0]
	bulge, tan := 0.0, first.Sub(c.last()).Angle()
	if arc {
		if a, ok := segTangent(c.last(), c.tan(), first); ok {
			bulge, tan = a.Bulge(), a.EndTangent()
		}
	}
	c.bulges = append(c.bulges, bulge)
	c.widths = append(c.widths, [2]float64{c.w0, c.w1})
	c.tans = append(c.tans, tan)
}

// undo drops the last segment; at the start point it asks for the start again.
func (c *plineCmd) undo(ctx *cmdsys.Context) *cmdsys.Prompt {
	if len(c.bulges) == 0 {
		c.pts, c.step = nil, plStart
		return &cmdsys.Prompt{Message: ctx.T("cmd.pline.prompt.start_point"), Accepts: cmdsys.InputPoint}
	}
	n := len(c.bulges) - 1
	c.pts = c.pts[:n+1]
	c.bulges, c.widths, c.tans = c.bulges[:n], c.widths[:n], c.tans[:n]
	return c.modePrompt(ctx)
}

// entity returns the polyline drawn so far, nil with fewer than two vertices.
func (c *plineCmd) entity(closed bool) *doc.LWPolyline {
	if len(c.pts) < 2 {
		return nil
	}
	e := &doc.LWPolyline{Points: append([]geom.Vec2(nil), c.pts...), Closed: closed}
	var curved, wide bool
	e.Bulges = make([]float64, len(c.pts))
	e.Widths = make([][2]float64, len(c.pts))
	for i := range c.bulges {
		e.Bulges[i], e.Widths[i] = c.bulges[i], c.widths[i]
		curved = curved || c.bulges[i] != 0
		wide = wide || c.widths[i] != [2]float64{}
	}
	if !curved {
		e.Bulges = nil
	}
	if !wide {
		e.Widths = nil
	}
	return e
}

func (c *plineCmd) finish(ctx *cmdsys.Context, closed bool) *cmdsys.Prompt {
	if e := c.entity(closed); e != nil {
		ctx.Tx().Add(e)
		end := c.last()
		if closed {
			end = c.pts[0]
		}
		ctx.SetLastEnd(end, c.tan())
	}
	return nil
}

// atPoint returns the arc segment the point p completes at a point-taking arc step.
func (c *plineCmd) atPoint(p geom.Vec2) (arcSeg, bool) {
	s := c.last()
	switch c.step {
	case plArcMode:
		return segTangent(s, c.tan(), p)
	case plSecondEnd:
		return seg3(s, c.p2, p)
	case plAngleEnd:
		return segChordAngle(s, p, c.ang)
	case plAngleCenter:
		return segCenterAngle(s, p, c.ang)
	case plCenterEnd:
		return segCenterEnd(s, c.ctr, p)
	case plDirectionEnd:
		return segTangent(s, c.dir, p)
	case plRadiusEnd:
		return segChordRadius(s, p, c.rad)
	}
	return arcSeg{}, false
}

// atValue returns the arc segment the number v completes at a value-taking arc step.
func (c *plineCmd) atValue(v float64) (arcSeg, bool) {
	s := c.last()
	switch c.step {
	case plCenterAngle:
		return segCenterAngle(s, c.ctr, v)
	case plCenterLen:
		return segCenterChord(s, c.ctr, v)
	case plChordDir:
		chord := 2 * math.Abs(c.rad) * math.Sin(math.Abs(c.ang)/2)
		return segChordAngle(s, s.Polar(chord, v), c.ang)
	}
	return arcSeg{}, false
}

func (c *plineCmd) setWidth(ctx *cmdsys.Context, v float64) {
	if c.step == plWidthStart {
		c.w0, c.w1 = v, v
	} else {
		c.w1 = v
	}
}

func (c *plineCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	isPt := in.Kind == cmdsys.InputPoint
	switch c.step {
	case plStart:
		if !isPt {
			return c.prompt(ctx)
		}
		c.pts, c.step = []geom.Vec2{in.Point}, plLine
		ctx.Say("cmd.pline.msg.current_width", ctx.Fmt(c.w0))
		return c.prompt(ctx)
	case plLine, plArcMode:
		return c.modeInput(ctx, in)
	case plWidthStart, plWidthEnd:
		if in.Kind != cmdsys.InputDistance {
			return c.prompt(ctx)
		}
		v := in.Value
		if c.half {
			v *= 2
		}
		if v < 0 {
			ctx.Say("cmd.pline.msg.width_negative")
			return c.prompt(ctx)
		}
		c.setWidth(ctx, v)
		if c.step == plWidthStart {
			c.step = plWidthEnd
			return c.prompt(ctx)
		}
		return c.modePrompt(ctx)
	case plLength:
		if in.Kind != cmdsys.InputDistance || in.Value == 0 {
			return c.prompt(ctx)
		}
		c.pushLine(c.last().Polar(in.Value, c.tan()))
		return c.modePrompt(ctx)
	case plAngle:
		if in.Kind == cmdsys.InputAngle {
			c.ang, c.step = in.Value, plAngleEnd
		}
	case plRadius:
		if in.Kind == cmdsys.InputDistance && in.Value != 0 {
			c.rad, c.step = in.Value, plRadiusEnd
			if c.angKnown {
				c.step = plChordDir
			}
		}
	case plCenter:
		if isPt {
			c.ctr, c.step = in.Point, plCenterEnd
		}
	case plDirection:
		if in.Kind == cmdsys.InputAngle {
			c.dir, c.step = in.Value, plDirectionEnd
		}
	case plSecond:
		if isPt {
			c.p2, c.step = in.Point, plSecondEnd
		}
	case plAngleEnd:
		switch {
		case in.Keyword == "Pline center":
			c.step = plAngleCenter
		case in.Keyword == "Radius":
			c.step, c.angKnown = plRadius, true
		case isPt:
			return c.finishArcPoint(ctx, in.Point)
		}
	case plAngleCenter, plDirectionEnd, plSecondEnd:
		if isPt {
			return c.finishArcPoint(ctx, in.Point)
		}
	case plRadiusEnd:
		switch {
		case in.Keyword == "Arc angle":
			c.step = plRadiusAngle
		case isPt:
			return c.finishArcPoint(ctx, in.Point)
		}
	case plRadiusAngle:
		if in.Kind == cmdsys.InputAngle {
			c.ang, c.step = in.Value, plChordDir
		}
	case plCenterEnd:
		switch {
		case in.Keyword == "Arc angle":
			c.step = plCenterAngle
		case in.Keyword == "Length":
			c.step = plCenterLen
		case isPt:
			return c.finishArcPoint(ctx, in.Point)
		}
	case plCenterAngle, plCenterLen, plChordDir:
		v := in.Value
		if in.Kind != cmdsys.InputAngle && in.Kind != cmdsys.InputDistance {
			return c.prompt(ctx)
		}
		if a, ok := c.atValue(v); ok {
			c.pushArc(a)
			return c.modePrompt(ctx)
		}
		ctx.Say("cmd.pline.msg.no_arc")
		return c.modePrompt(ctx)
	}
	return c.prompt(ctx)
}

// finishArcPoint adds the arc a point completes, or reports that none fits.
func (c *plineCmd) finishArcPoint(ctx *cmdsys.Context, p geom.Vec2) *cmdsys.Prompt {
	if a, ok := c.atPoint(p); ok {
		c.pushArc(a)
		return c.modePrompt(ctx)
	}
	if c.step == plArcMode {
		// The point lies on the tangent line: the arc is a straight segment.
		c.pushLine(p)
		return c.modePrompt(ctx)
	}
	ctx.Say("cmd.pline.msg.no_arc")
	return c.prompt(ctx)
}

// modeInput handles the line-mode and arc-mode prompts.
func (c *plineCmd) modeInput(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputEnter:
		return c.finish(ctx, false)
	case in.Keyword == "Close":
		c.pushClosing(false)
		return c.finish(ctx, true)
	case in.Keyword == "Close arc":
		c.pushClosing(true)
		return c.finish(ctx, true)
	case in.Keyword == "Undo":
		return c.undo(ctx)
	case in.Keyword == "Arc":
		c.arcMode = true
	case in.Keyword == "Line":
		c.arcMode = false
	case in.Keyword == "Width" || in.Keyword == "Halfwidth":
		c.half = in.Keyword == "Halfwidth"
		c.step = plWidthStart
		return c.prompt(ctx)
	case in.Keyword == "Length":
		c.step = plLength
		return c.prompt(ctx)
	case in.Keyword == "Arc angle":
		c.step = plAngle
		return c.prompt(ctx)
	case in.Keyword == "Pline center":
		c.step = plCenter
		return c.prompt(ctx)
	case in.Keyword == "Direction":
		c.step = plDirection
		return c.prompt(ctx)
	case in.Keyword == "Radius":
		c.step = plRadius
		return c.prompt(ctx)
	case in.Keyword == "Second pt":
		c.step = plSecond
		return c.prompt(ctx)
	case in.Kind == cmdsys.InputPoint:
		if c.arcMode {
			return c.finishArcPoint(ctx, in.Point)
		}
		c.pushLine(in.Point)
	}
	return c.modePrompt(ctx)
}

// Preview draws the polyline so far and the segment the cursor would add.
func (c *plineCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if len(c.pts) == 0 {
		return
	}
	pl := geom.Polyline{Pts: append([]geom.Vec2(nil), c.pts...), Bulges: append([]float64(nil), c.bulges...)}
	pl.Bulges = append(pl.Bulges, 0)
	extend := func(end geom.Vec2, bulge float64) {
		pl.Bulges[len(pl.Bulges)-1] = bulge
		pl.Pts = append(pl.Pts, end)
		pl.Bulges = append(pl.Bulges, 0)
	}
	switch c.step {
	case plLine:
		extend(cur, 0)
	case plLength:
		extend(c.last().Polar(cur.Dist(c.last()), c.tan()), 0)
	case plArcMode, plAngleEnd, plAngleCenter, plCenterEnd, plDirectionEnd, plSecondEnd, plRadiusEnd:
		if a, ok := c.atPoint(cur); ok {
			extend(a.E, a.Bulge())
		} else if c.step == plArcMode {
			extend(cur, 0)
		}
	}
	dl.Curve(pl, previewTol(ctx), PreviewColor, 1)
}

// Cancel keeps the polyline drawn so far, as AutoCAD does.
func (c *plineCmd) Cancel(ctx *cmdsys.Context) { c.finish(ctx, false) }
