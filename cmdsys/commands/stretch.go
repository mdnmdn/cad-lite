package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// --- STRETCH ------------------------------------------------------------

type stretchStage int

const (
	stSelect stretchStage = iota
	stCorner
	stPoly
	stBase
	stDisplacement
	stSecond
)

// stretchCmd is STRETCH: crossing windows or crossing polygons choose the objects, then the
// vertices and endpoints inside them move by the displacement.
type stretchCmd struct {
	base
	stage stretchStage
	// windows are the crossing shapes picked so far, each a closed polygon.
	windows [][]geom.Vec2
	// pts are the corners or polygon points of the shape being collected.
	pts []geom.Vec2
	// hs are the objects crossed by the windows.
	hs   []doc.Handle
	from *geom.Vec2
}

func (*stretchCmd) Name() string { return "STRETCH" }

func (c *stretchCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ctx.Say("cmd.stretch.msg.hint")
	return c.selectPrompt(ctx)
}

func (c *stretchCmd) selectPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stSelect
	return &cmdsys.Prompt{Message: ctx.T("cmd.common.prompt.select_objects"),
		Accepts:  cmdsys.InputPoint | cmdsys.InputKeyword | cmdsys.InputEnter,
		Keywords: []string{"Crossing", "CPolygon"}}
}

func (c *stretchCmd) cornerPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stCorner
	return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.opposite_corner"), Accepts: cmdsys.InputPoint,
		Base: pt(c.pts[0])}
}

func (c *stretchCmd) polyPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stPoly
	p := &cmdsys.Prompt{Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
	if len(c.pts) == 0 {
		p.Message = ctx.T("cmd.select.prompt.first_polygon")
		return p
	}
	p.Message = ctx.T("cmd.select.prompt.endpoint")
	p.Base = pt(c.pts[len(c.pts)-1])
	return p
}

func (c *stretchCmd) basePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stBase
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.base_point"),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Displacement"},
		Default: defaultKeyword("Displacement")}
}

func (c *stretchCmd) secondPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stSecond
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.second_point_or"),
		Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: c.from,
		Default: defaultText(ctx.T("cmd.modify.prompt.use_first"))}
}

// addWindow records the shape just collected and the objects it crosses, and reports the count
// as the selection prompt does.
func (c *stretchCmd) addWindow(ctx *cmdsys.Context, poly []geom.Vec2) *cmdsys.Prompt {
	c.windows = append(c.windows, poly)
	c.pts = nil
	tol := ctx.View.PixelSize() / 2
	box := geom.EmptyBox()
	for _, p := range poly {
		box = box.Extend(p)
	}
	n := 0
	for _, h := range ctx.Index.Query(box) {
		e := ctx.Doc.Entity(h)
		if !stretchable(ctx.Doc, e) || !crossesPolygon(e, poly, tol) {
			continue
		}
		n++
		dup := false
		for _, x := range c.hs {
			dup = dup || x == h
		}
		if !dup {
			c.hs = append(c.hs, h)
		}
	}
	ctx.SayPlural("cmd.common.msg.found", n, n)
	return c.selectPrompt(ctx)
}

// stretchable reports whether e may be edited: it exists and its layer is thawed, unlocked and on.
func stretchable(d *doc.Drawing, e doc.Entity) bool {
	if e == nil {
		return false
	}
	l := d.Layer(e.Common().Layer)
	return l == nil || (!l.Locked && !l.Frozen && !l.Off)
}

func crossesPolygon(e doc.Entity, poly []geom.Vec2, tol float64) bool {
	var paths [][]geom.Vec2
	for _, cv := range e.Geometry() {
		paths = append(paths, cv.Tessellate(tol, nil))
	}
	return cmdsys.PathsCrossPolygon(paths, poly)
}

func (c *stretchCmd) inside(p geom.Vec2) bool {
	for _, w := range c.windows {
		if cmdsys.PointInPolygon(p, w) {
			return true
		}
	}
	return false
}

func (c *stretchCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case stSelect:
		switch {
		case in.Kind == cmdsys.InputKeyword && in.Keyword == "CPolygon":
			c.pts = nil
			return c.polyPrompt(ctx)
		case in.Kind == cmdsys.InputKeyword:
			// Crossing: the first corner follows.
			c.pts = nil
			c.stage = stCorner
			return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.first_corner"), Accepts: cmdsys.InputPoint}
		case in.Kind == cmdsys.InputPoint:
			c.pts = []geom.Vec2{in.Point}
			return c.cornerPrompt(ctx)
		}
		if len(c.windows) == 0 {
			return nil
		}
		return c.basePrompt(ctx)
	case stCorner:
		if len(c.pts) == 0 {
			c.pts = []geom.Vec2{in.Point}
			return c.cornerPrompt(ctx)
		}
		return c.addWindow(ctx, rectPts(c.pts[0], in.Point))
	case stPoly:
		if in.Kind == cmdsys.InputPoint {
			c.pts = append(c.pts, in.Point)
			return c.polyPrompt(ctx)
		}
		if len(c.pts) >= 3 {
			return c.addWindow(ctx, c.pts)
		}
		c.pts = nil
		return c.selectPrompt(ctx)
	case stBase:
		if in.Kind == cmdsys.InputKeyword {
			c.stage = stDisplacement
			return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.displacement"),
				Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
		}
		c.from = pt(in.Point)
		return c.secondPrompt(ctx)
	case stDisplacement:
		v := geom.Vec2{}
		if in.Kind == cmdsys.InputPoint {
			v = in.Point
		}
		c.apply(ctx, v)
		return nil
	}
	v := *c.from
	if in.Kind == cmdsys.InputPoint {
		v = in.Point.Sub(*c.from)
	}
	c.apply(ctx, v)
	return nil
}

func (c *stretchCmd) apply(ctx *cmdsys.Context, v geom.Vec2) {
	for _, h := range c.hs {
		if e := ctx.Doc.Entity(h); e != nil {
			ctx.Tx().Modify(h, func(e doc.Entity) { c.stretch(e, v) })
		}
	}
}

// stretch moves the parts of e inside the windows by v: the vertices of a polyline, the
// endpoints of a line or arc (an arc keeps its radius and its side, so its centre follows), the
// whole of a circle whose centre is inside, and any other entity lying wholly inside.
func (c *stretchCmd) stretch(e doc.Entity, v geom.Vec2) {
	switch e := e.(type) {
	case *doc.Line:
		if c.inside(e.Start) {
			e.Start = e.Start.Add(v)
		}
		if c.inside(e.End) {
			e.End = e.End.Add(v)
		}
	case *doc.Circle:
		if c.inside(e.Center) {
			e.Center = e.Center.Add(v)
		}
	case *doc.LWPolyline:
		for i, p := range e.Points {
			if c.inside(p) {
				e.Points[i] = p.Add(v)
			}
		}
	case *doc.Arc:
		c.stretchArc(e, v)
	default:
		b := e.Bounds()
		if !b.IsEmpty() && c.inside(b.Min) && c.inside(b.Max) &&
			c.inside(geom.Vec2{X: b.Min.X, Y: b.Max.Y}) && c.inside(geom.Vec2{X: b.Max.X, Y: b.Min.Y}) {
			e.Transform(geom.Translate(v))
		}
	}
}

func (c *stretchCmd) stretchArc(a *doc.Arc, v geom.Vec2) {
	s := a.Center.Add(geom.Vec2{X: math.Cos(a.StartAngle), Y: math.Sin(a.StartAngle)}.Scale(a.Radius))
	en := a.Center.Add(geom.Vec2{X: math.Cos(a.EndAngle), Y: math.Sin(a.EndAngle)}.Scale(a.Radius))
	in0, in1 := c.inside(s), c.inside(en)
	switch {
	case in0 && in1:
		a.Center = a.Center.Add(v)
		return
	case !in0 && !in1:
		return
	}
	sweep := math.Mod(a.EndAngle-a.StartAngle, 2*math.Pi)
	if sweep <= 0 {
		sweep += 2 * math.Pi
	}
	if in0 {
		s = s.Add(v)
	} else {
		en = en.Add(v)
	}
	chord := en.Sub(s)
	d := chord.Len()
	if d < 1e-12 {
		return
	}
	half := d / 2
	h := 0.0
	if a.Radius > half {
		h = math.Sqrt(a.Radius*a.Radius - half*half)
	} else {
		// The chord outgrew the diameter: the arc becomes a semicircle.
		a.Radius = half
	}
	// A counterclockwise minor arc has its centre left of the chord, a major arc right of it.
	left := geom.Vec2{X: -chord.Y, Y: chord.X}.Scale(1 / d)
	if sweep > math.Pi {
		left = left.Scale(-1)
	}
	a.Center = s.Add(en).Scale(0.5).Add(left.Scale(h))
	a.StartAngle = math.Atan2(s.Y-a.Center.Y, s.X-a.Center.X)
	a.EndAngle = math.Atan2(en.Y-a.Center.Y, en.X-a.Center.X)
}

func (c *stretchCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch c.stage {
	case stCorner:
		if len(c.pts) == 1 {
			dl.Polyline(rectPts(c.pts[0], cur), PreviewColor.WithAlpha(120), 0, true)
		}
	case stPoly:
		if len(c.pts) > 0 {
			dl.Polyline(append(append([]geom.Vec2(nil), c.pts...), cur), PreviewColor.WithAlpha(120), 0, false)
		}
	case stSecond:
		dl.Polyline([]geom.Vec2{*c.from, cur}, PreviewColor.WithAlpha(120), 0, false)
		tol := previewTol(ctx)
		for _, h := range c.hs {
			if e := ctx.Doc.Entity(h); e != nil {
				cp := copyOf(e)
				c.stretch(cp, cur.Sub(*c.from))
				for _, cv := range cp.Geometry() {
					dl.Curve(cv, tol, PreviewColor, 1)
				}
			}
		}
	}
}

// registerStretch adds STRETCH (alias S).
func registerStretch(r *cmdsys.Registry) {
	r.Register("STRETCH", func() cmdsys.Command { return &stretchCmd{} })
	r.Alias("S", "STRETCH")
}
