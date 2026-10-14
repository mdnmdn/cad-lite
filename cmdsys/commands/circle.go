package commands

import (
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- CIRCLE -------------------------------------------------------------

type circleStep int

const (
	circleCenter    circleStep = iota // centre or [3P/2P/Ttr]
	circleRadius                      // radius or [Diameter] <CIRCLERAD>
	circleDiameter                    // diameter <2r>
	circle3P                          // three points on the circle (pts holds the ones given)
	circle2P                          // two ends of a diameter (pts holds the ones given)
	circleTanFirst                    // pick on the first tangent object
	circleTanSecond                   // pick on the second tangent object
	circleTanRadius                   // radius of the tangent circle
)

// circleKeywords are the ids of the options at the centre prompt; their labels (the English one
// is "Ttr (tan tan radius)") come from kw.* in the catalogs.
var circleKeywords = []string{"3P", "2P", "Ttr"}

type circleCmd struct {
	base
	step   circleStep
	center geom.Vec2
	pts    []geom.Vec2
	// tangent objects (as full-extension lines or circles) and the picks on them
	tan   [2]geom.Curve
	picks [2]geom.Vec2
}

func (*circleCmd) Name() string { return "CIRCLE" }

func (c *circleCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.prompt(ctx) }

func (c *circleCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	rad := ctx.Vars().Real("CIRCLERAD")
	switch c.step {
	case circleRadius:
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.radius"), Accepts: cmdsys.InputDistance | cmdsys.InputKeyword, Keywords: []string{"Diameter"}, Default: distDefault(rad), Base: pt(c.center)}
	case circleDiameter:
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.diameter"), Accepts: cmdsys.InputDistance, Default: distDefault(2 * rad), Base: pt(c.center)}
	case circle3P:
		keys := [...]string{"cmd.circle.prompt.first_point", "cmd.circle.prompt.second_point", "cmd.circle.prompt.third_point"}
		p := &cmdsys.Prompt{Message: ctx.T(keys[len(c.pts)]), Accepts: cmdsys.InputPoint}
		if n := len(c.pts); n > 0 {
			p.Base = pt(c.pts[n-1])
		}
		return p
	case circle2P:
		keys := [...]string{"cmd.circle.prompt.diameter_first", "cmd.circle.prompt.diameter_second"}
		p := &cmdsys.Prompt{Message: ctx.T(keys[len(c.pts)]), Accepts: cmdsys.InputPoint}
		if n := len(c.pts); n > 0 {
			p.Base = pt(c.pts[n-1])
		}
		return p
	case circleTanFirst:
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.tangent_first"), Accepts: cmdsys.InputPoint}
	case circleTanSecond:
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.tangent_second"), Accepts: cmdsys.InputPoint}
	case circleTanRadius:
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.radius"), Accepts: cmdsys.InputDistance, Default: distDefault(rad), Base: pt(c.picks[1])}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.center"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: circleKeywords}
}

// finish adds the circle, remembering its radius as CIRCLERAD when remember is set.
func (c *circleCmd) finish(ctx *cmdsys.Context, centre geom.Vec2, r float64, remember bool) *cmdsys.Prompt {
	if !(r > 0) || math.IsInf(r, 0) {
		ctx.Say("cmd.common.msg.positive")
		return c.prompt(ctx)
	}
	ctx.Tx().Add(&doc.Circle{Center: centre, Radius: r})
	if remember {
		_ = ctx.Vars().Set("CIRCLERAD", r)
	}
	return nil
}

func (c *circleCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case circleCenter:
		switch {
		case in.Keyword == "3P":
			c.step = circle3P
		case in.Keyword == "2P":
			c.step = circle2P
		case in.Keyword == "Ttr":
			c.step = circleTanFirst
		case in.Kind == cmdsys.InputPoint:
			c.center = in.Point
			c.step = circleRadius
		}
	case circleRadius:
		switch {
		case in.Keyword == "Diameter":
			c.step = circleDiameter
		case in.Kind == cmdsys.InputDistance:
			return c.finish(ctx, c.center, in.Value, true)
		}
	case circleDiameter:
		if in.Kind == cmdsys.InputDistance {
			return c.finish(ctx, c.center, in.Value/2, true)
		}
	case circle3P:
		if in.Kind != cmdsys.InputPoint {
			break
		}
		c.pts = append(c.pts, in.Point)
		if len(c.pts) < 3 {
			break
		}
		a, ok := doc.ArcFrom3Points(c.pts[0], c.pts[1], c.pts[2])
		if !ok {
			ctx.Say("cmd.circle.msg.collinear")
			return nil
		}
		return c.finish(ctx, a.C, a.R, false)
	case circle2P:
		if in.Kind != cmdsys.InputPoint {
			break
		}
		c.pts = append(c.pts, in.Point)
		if len(c.pts) < 2 {
			break
		}
		return c.finish(ctx, c.pts[0].Lerp(c.pts[1], 0.5), c.pts[0].Dist(c.pts[1])/2, false)
	case circleTanFirst, circleTanSecond:
		if in.Kind != cmdsys.InputPoint {
			break
		}
		i := 0
		if c.step == circleTanSecond {
			i = 1
		}
		curve, ok := tangentObject(ctx, in.Point)
		if !ok {
			ctx.Say("cmd.circle.msg.no_tangent_object")
			break
		}
		c.tan[i], c.picks[i] = curve, in.Point
		if i == 0 {
			c.step = circleTanSecond
		} else {
			c.step = circleTanRadius
		}
	case circleTanRadius:
		if in.Kind != cmdsys.InputDistance {
			break
		}
		if !(in.Value > 0) {
			ctx.Say("cmd.common.msg.positive")
			break
		}
		t, ok := geom.NearestTangentCircle(c.tan[0], c.tan[1], c.picks[0], c.picks[1], in.Value)
		if !ok {
			ctx.Say("cmd.circle.msg.no_ttr")
			return nil
		}
		return c.finish(ctx, t.C, t.R, true)
	}
	return c.prompt(ctx)
}

// tangentObject returns the line or circle the entity at p offers a tangent circle: a line, a
// circle, an arc (taken as its circle) or the segment of a polyline nearest p.
func tangentObject(ctx *cmdsys.Context, p geom.Vec2) (geom.Curve, bool) {
	h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, p, pickAperturePx*ctx.View.PixelSize())
	if !ok {
		return nil, false
	}
	e := ctx.Doc.Entity(h)
	if e == nil {
		return nil, false
	}
	var best geom.Curve
	bestD := math.Inf(1)
	consider := func(c geom.Curve) {
		switch c.(type) {
		case geom.Line, geom.Circle, geom.Arc:
		default:
			return
		}
		if d := geom.Closest(c, p).Dist(p); d < bestD {
			best, bestD = c, d
		}
	}
	for _, g := range e.Geometry() {
		if pl, ok := g.(geom.Polyline); ok {
			for _, s := range pl.Segments() {
				consider(s)
			}
			continue
		}
		consider(g)
	}
	return best, best != nil
}

func (c *circleCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	tol := previewTol(ctx)
	switch c.step {
	case circleRadius, circleDiameter:
		r := cur.Dist(c.center)
		if c.step == circleDiameter {
			r /= 2
		}
		dl.Curve(geom.Circle{C: c.center, R: r}, tol, PreviewColor, 1)
		dl.Polyline([]geom.Vec2{c.center, cur}, PreviewColor.WithAlpha(120), 0, false)
	case circle3P:
		pts := append(append([]geom.Vec2(nil), c.pts...), cur)
		switch len(pts) {
		case 2:
			dl.Polyline(pts, PreviewColor, 1, false)
		case 3:
			if a, ok := doc.ArcFrom3Points(pts[0], pts[1], pts[2]); ok {
				dl.Curve(geom.Circle{C: a.C, R: a.R}, tol, PreviewColor, 1)
			}
		}
	case circle2P:
		if len(c.pts) == 1 {
			dl.Curve(geom.Circle{C: c.pts[0].Lerp(cur, 0.5), R: c.pts[0].Dist(cur) / 2}, tol, PreviewColor, 1)
			dl.Polyline([]geom.Vec2{c.pts[0], cur}, PreviewColor.WithAlpha(120), 0, false)
		}
	case circleTanRadius:
		if t, ok := geom.NearestTangentCircle(c.tan[0], c.tan[1], c.picks[0], c.picks[1], cur.Dist(c.picks[1])); ok {
			dl.Curve(t, tol, PreviewColor, 1)
		}
	}
}
