package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// --- DIST, ID, LIST -----------------------------------------------------

type distCmd struct {
	base
	first *geom.Vec2
	multi bool
	pts   []geom.Vec2
	total float64
}

func (*distCmd) Name() string { return "DIST" }

func (*distCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.dist.prompt.first_point"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Multiple points"}}
}

func (c *distCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Keyword == "Multiple points" {
		c.multi = true
		return &cmdsys.Prompt{Message: ctx.T("cmd.dist.prompt.first_point"), Accepts: cmdsys.InputPoint}
	}
	if c.multi {
		if in.Kind == cmdsys.InputEnter {
			ctx.Say("cmd.dist.msg.total", cmdsys.FormatNumber(c.total))
			return nil
		}
		if n := len(c.pts); n > 0 {
			c.total += c.pts[n-1].Dist(in.Point)
		}
		c.pts = append(c.pts, in.Point)
		return &cmdsys.Prompt{Message: ctx.T("cmd.dist.prompt.next_point"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: pt(in.Point)}
	}
	if c.first == nil {
		c.first = pt(in.Point)
		return &cmdsys.Prompt{Message: ctx.T("cmd.dist.prompt.second_point"), Accepts: cmdsys.InputPoint, Base: c.first}
	}
	d := in.Point.Sub(*c.first)
	ctx.Say("cmd.dist.msg.result",
		cmdsys.FormatNumber(d.Len()), cmdsys.FormatNumber(geom.NormAngle(d.Angle())*180/math.Pi),
		cmdsys.FormatNumber(d.X), cmdsys.FormatNumber(d.Y))
	return nil
}

func (c *distCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.multi && len(c.pts) > 0 {
		dl.Polyline(append(append([]geom.Vec2(nil), c.pts...), cur), PreviewColor, 0, false)
		return
	}
	if c.first != nil {
		dl.Polyline([]geom.Vec2{*c.first, cur}, PreviewColor, 0, false)
	}
}

type idCmd struct{ base }

func (*idCmd) Name() string { return "ID" }

func (*idCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.id.prompt.point"), Accepts: cmdsys.InputPoint}
}

func (*idCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	ctx.Say("cmd.id.msg.result", cmdsys.FormatNumber(in.Point.X), cmdsys.FormatNumber(in.Point.Y))
	return nil
}

type listCmd struct {
	base
	selecting
}

func (*listCmd) Name() string { return "LIST" }

func (c *listCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		c.list(ctx)
		return nil
	}
	return selectPrompt(ctx)
}

func (c *listCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.take(ctx, in) {
		c.list(ctx)
	}
	return nil
}

func (c *listCmd) list(ctx *cmdsys.Context) {
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		ctx.Say("cmd.list.msg.header", e.Class(), e.Common().Layer, h)
		switch e := e.(type) {
		case *doc.Line:
			ctx.Say("cmd.list.msg.line", cmdsys.FormatPoint(e.Start), cmdsys.FormatPoint(e.End), cmdsys.FormatNumber(e.Start.Dist(e.End)))
		case *doc.Circle:
			ctx.Say("cmd.list.msg.circle", cmdsys.FormatPoint(e.Center), cmdsys.FormatNumber(e.Radius))
		case *doc.Arc:
			ctx.Say("cmd.list.msg.arc", cmdsys.FormatPoint(e.Center), cmdsys.FormatNumber(e.Radius),
				cmdsys.FormatNumber(e.StartAngle*180/math.Pi), cmdsys.FormatNumber(e.EndAngle*180/math.Pi))
		case *doc.LWPolyline:
			ctx.Say("cmd.list.msg.polyline", len(e.Points), yesNo(ctx, e.Closed), cmdsys.FormatNumber(e.Geometry()[0].Length()))
		}
	}
}

// Transparent marks DIST as runnable inside another command (cmdsys.Transparent).
func (*distCmd) Transparent() bool { return true }

// Transparent marks ID as runnable inside another command (cmdsys.Transparent).
func (*idCmd) Transparent() bool { return true }

// registerInquiry adds DIST, ID and LIST.
func registerInquiry(r *cmdsys.Registry) {
	r.Register("DIST", func() cmdsys.Command { return &distCmd{} })
	r.Register("ID", func() cmdsys.Command { return &idCmd{} })
	r.Register("LIST", func() cmdsys.Command { return &listCmd{} })
}
