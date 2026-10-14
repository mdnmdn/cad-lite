// Package commands holds the built-in AutoCAD-style commands. Register adds
// them, with their default aliases, to a registry.
package commands

import (
	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// PreviewColor is the rubber-band colour of command previews.
var PreviewColor = render.Color{R: 120, G: 200, B: 255, A: 255}

func pt(p geom.Vec2) *geom.Vec2 { return &p }

func previewTol(ctx *cmdsys.Context) float64 { return ctx.View.PixelSize() / 4 }

// base embeds the no-op parts of cmdsys.Command.
type base struct{}

func (base) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (base) Cancel(*cmdsys.Context)                                  {}

// --- LINE ---------------------------------------------------------------

type lineCmd struct {
	pts   []geom.Vec2
	added []doc.Handle
}

func (*lineCmd) Name() string { return "LINE" }

func (c *lineCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.line.prompt.first_point"), Accepts: cmdsys.InputPoint}
}

func (c *lineCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	p := &cmdsys.Prompt{Message: ctx.T("cmd.line.prompt.next_point"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter | cmdsys.InputKeyword, Base: pt(c.pts[len(c.pts)-1])}
	if len(c.pts) >= 3 {
		p.Keywords = []string{"Close", "Undo"}
	} else {
		p.Keywords = []string{"Undo"}
	}
	return p
}

func (c *lineCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputEnter:
		return nil
	case in.Keyword == "Undo":
		if len(c.added) == 0 {
			if len(c.pts) > 0 {
				c.pts = c.pts[:0]
			}
			return c.Start(ctx)
		}
		ctx.Tx().Erase(c.added[len(c.added)-1])
		c.added = c.added[:len(c.added)-1]
		c.pts = c.pts[:len(c.pts)-1]
		return c.prompt(ctx)
	case in.Keyword == "Close":
		ctx.Tx().Add(&doc.Line{Start: c.pts[len(c.pts)-1], End: c.pts[0]})
		return nil
	case in.Kind == cmdsys.InputPoint:
		if n := len(c.pts); n > 0 {
			c.added = append(c.added, ctx.Tx().Add(&doc.Line{Start: c.pts[n-1], End: in.Point}))
		}
		c.pts = append(c.pts, in.Point)
		return c.prompt(ctx)
	}
	return c.prompt(ctx)
}

func (c *lineCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if n := len(c.pts); n > 0 {
		dl.Polyline([]geom.Vec2{c.pts[n-1], cur}, PreviewColor, 1, false)
	}
}

func (*lineCmd) Cancel(*cmdsys.Context) {}

// --- PLINE --------------------------------------------------------------

type plineCmd struct {
	pts []geom.Vec2
}

func (*plineCmd) Name() string { return "PLINE" }

func (c *plineCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.pline.prompt.start_point"), Accepts: cmdsys.InputPoint}
}

func (c *plineCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	p := &cmdsys.Prompt{Message: ctx.T("cmd.pline.prompt.next_point"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter | cmdsys.InputKeyword, Base: pt(c.pts[len(c.pts)-1]), Keywords: []string{"Undo"}}
	if len(c.pts) >= 3 {
		p.Keywords = []string{"Close", "Undo"}
	}
	return p
}

func (c *plineCmd) finish(ctx *cmdsys.Context, closed bool) *cmdsys.Prompt {
	if len(c.pts) >= 2 {
		ctx.Tx().Add(&doc.LWPolyline{Points: c.pts, Closed: closed})
	}
	return nil
}

func (c *plineCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputEnter:
		return c.finish(ctx, false)
	case in.Keyword == "Close":
		return c.finish(ctx, true)
	case in.Keyword == "Undo":
		c.pts = c.pts[:len(c.pts)-1]
		if len(c.pts) == 0 {
			return c.Start(ctx)
		}
	case in.Kind == cmdsys.InputPoint:
		c.pts = append(c.pts, in.Point)
	}
	return c.prompt(ctx)
}

func (c *plineCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if len(c.pts) > 0 {
		dl.Polyline(append(append([]geom.Vec2(nil), c.pts...), cur), PreviewColor, 1, false)
	}
}

// Cancel keeps the polyline drawn so far, as AutoCAD does.
func (c *plineCmd) Cancel(ctx *cmdsys.Context) { c.finish(ctx, false) }

// --- CIRCLE -------------------------------------------------------------

type circleCmd struct {
	base
	center   *geom.Vec2
	diameter bool
}

func (*circleCmd) Name() string { return "CIRCLE" }

func (c *circleCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.center"), Accepts: cmdsys.InputPoint}
}

func (c *circleCmd) radiusPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.diameter {
		return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.diameter"), Accepts: cmdsys.InputDistance, Base: c.center}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.circle.prompt.radius"), Accepts: cmdsys.InputDistance | cmdsys.InputKeyword, Keywords: []string{"Diameter"}, Base: c.center}
}

func (c *circleCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.center == nil:
		c.center = pt(in.Point)
		return c.radiusPrompt(ctx)
	case in.Keyword == "Diameter":
		c.diameter = true
		return c.radiusPrompt(ctx)
	case in.Kind == cmdsys.InputDistance:
		r := in.Value
		if c.diameter {
			r /= 2
		}
		if r <= 0 {
			ctx.Say("cmd.common.msg.positive")
			return c.radiusPrompt(ctx)
		}
		ctx.Tx().Add(&doc.Circle{Center: *c.center, Radius: r})
		return nil
	}
	return c.radiusPrompt(ctx)
}

func (c *circleCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.center == nil {
		return
	}
	r := cur.Dist(*c.center)
	if c.diameter {
		r /= 2
	}
	dl.Curve(geom.Circle{C: *c.center, R: r}, previewTol(ctx), PreviewColor, 1)
	dl.Polyline([]geom.Vec2{*c.center, cur}, PreviewColor.WithAlpha(120), 0, false)
}

// --- ARC (three points) -------------------------------------------------

type arcCmd struct {
	base
	pts []geom.Vec2
}

func (*arcCmd) Name() string { return "ARC" }

func (c *arcCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.start_point"), Accepts: cmdsys.InputPoint}
}

func (c *arcCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	c.pts = append(c.pts, in.Point)
	switch len(c.pts) {
	case 1:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.second_point"), Accepts: cmdsys.InputPoint, Base: pt(c.pts[0])}
	case 2:
		return &cmdsys.Prompt{Message: ctx.T("cmd.arc.prompt.end_point"), Accepts: cmdsys.InputPoint, Base: pt(c.pts[1])}
	}
	a, ok := doc.ArcFrom3Points(c.pts[0], c.pts[1], c.pts[2])
	if !ok {
		ctx.Say("cmd.arc.msg.collinear")
		return nil
	}
	ctx.Tx().Add(&doc.Arc{Center: a.C, Radius: a.R, StartAngle: a.Start, EndAngle: a.End})
	return nil
}

func (c *arcCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch len(c.pts) {
	case 1:
		dl.Polyline([]geom.Vec2{c.pts[0], cur}, PreviewColor, 1, false)
	case 2:
		if a, ok := doc.ArcFrom3Points(c.pts[0], c.pts[1], cur); ok {
			dl.Curve(a, previewTol(ctx), PreviewColor, 1)
		}
	}
}

// --- RECTANG ------------------------------------------------------------

type rectCmd struct {
	base
	first *geom.Vec2
}

func (*rectCmd) Name() string { return "RECTANG" }

func (c *rectCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.first_corner"), Accepts: cmdsys.InputPoint}
}

func rectPts(a, b geom.Vec2) []geom.Vec2 {
	return []geom.Vec2{a, {X: b.X, Y: a.Y}, b, {X: a.X, Y: b.Y}}
}

func (c *rectCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.first == nil {
		c.first = pt(in.Point)
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.other_corner"), Accepts: cmdsys.InputPoint, Base: c.first}
	}
	ctx.Tx().Add(&doc.LWPolyline{Points: rectPts(*c.first, in.Point), Closed: true})
	return nil
}

func (c *rectCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.first != nil {
		dl.Polyline(rectPts(*c.first, cur), PreviewColor, 1, true)
	}
}
