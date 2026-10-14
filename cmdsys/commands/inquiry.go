package commands

import (
	"fmt"
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
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
			storeResult(ctx, "DISTANCE", c.total)
			ctx.Say("cmd.dist.msg.total", ctx.Fmt(c.total))
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
	storeResult(ctx, "DISTANCE", d.Len())
	ctx.Say("cmd.dist.msg.result", ctx.Fmt(d.Len()), ctx.FmtAngle(d.Angle()), ctx.FmtAngleRel(0))
	ctx.Say("cmd.dist.msg.delta", ctx.Fmt(d.X), ctx.Fmt(d.Y), ctx.Fmt(0))
	return nil
}

// storeResult publishes an inquiry result in its read-only variable (DISTANCE, AREA, PERIMETER).
func storeResult(ctx *cmdsys.Context, name string, v float64) { _ = ctx.Vars().Store(name, v) }

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
	ctx.SetLastPoint(in.Point)
	ctx.Say("cmd.id.msg.result", ctx.Fmt(in.Point.X), ctx.Fmt(in.Point.Y), ctx.Fmt(0))
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
		ctx.Say("cmd.list.msg.header", e.Class(), e.Common().Layer)
		ctx.Say("cmd.list.msg.space")
		ctx.Say("cmd.list.msg.handle", h)
		switch e := e.(type) {
		case *doc.Line:
			d := e.End.Sub(e.Start)
			listPoint(ctx, "cmd.list.msg.from_point", e.Start)
			listPoint(ctx, "cmd.list.msg.to_point", e.End)
			ctx.Say("cmd.list.msg.line_length", ctx.Fmt(d.Len()), ctx.FmtAngle(d.Angle()))
			ctx.Say("cmd.list.msg.delta", ctx.Fmt(d.X), ctx.Fmt(d.Y), ctx.Fmt(0))
		case *doc.Circle:
			listPoint(ctx, "cmd.list.msg.center", e.Center)
			ctx.Say("cmd.list.msg.radius", ctx.Fmt(e.Radius))
			ctx.Say("cmd.list.msg.circumference", ctx.Fmt(2*math.Pi*e.Radius))
			ctx.Say("cmd.list.msg.area", ctx.Fmt(math.Pi*e.Radius*e.Radius))
		case *doc.Arc:
			listPoint(ctx, "cmd.list.msg.center", e.Center)
			ctx.Say("cmd.list.msg.radius", ctx.Fmt(e.Radius))
			ctx.Say("cmd.list.msg.start_angle", ctx.FmtAngle(e.StartAngle))
			ctx.Say("cmd.list.msg.end_angle", ctx.FmtAngle(e.EndAngle))
			ctx.Say("cmd.list.msg.arc_length", ctx.Fmt(e.Geometry()[0].Length()))
		case *doc.LWPolyline:
			closed := "cmd.list.msg.open"
			if e.Closed {
				closed = "cmd.list.msg.closed"
			}
			ctx.Say(closed)
			ctx.Say("cmd.list.msg.constant_width", ctx.Fmt(0))
			if area, perim, _, ok := entityArea(e); ok && e.Closed {
				ctx.Say("cmd.list.msg.area", ctx.Fmt(area))
				ctx.Say("cmd.list.msg.perimeter", ctx.Fmt(perim))
			} else if ok {
				ctx.Say("cmd.list.msg.length", ctx.Fmt(perim))
			}
			for _, p := range e.Points {
				listPoint(ctx, "cmd.list.msg.at_point", p)
			}
		case *doc.Text:
			listText(ctx, e)
		case *doc.MText:
			listMText(ctx, e)
		}
	}
}

// listText prints a TEXT the way LIST does: style, points, height, value and the layout values.
func listText(ctx *cmdsys.Context, e *doc.Text) {
	lay := e.Layout()
	style := e.Style
	if style == "" {
		style = doc.DefaultTextStyle
	}
	ctx.Say("cmd.list.msg.style", style)
	listPoint(ctx, "cmd.list.msg.start_point", lay.Origin)
	if e.HAlign != doc.TextLeft || e.VAlign != doc.TextBaseline {
		listPoint(ctx, "cmd.list.msg.align_point", e.Align)
	}
	ctx.Say("cmd.list.msg.justify", e.Justify())
	ctx.Say("cmd.list.msg.height", ctx.Fmt(lay.Height))
	ctx.Say("cmd.list.msg.text", e.Value)
	ctx.Say("cmd.list.msg.rotation", ctx.FmtAngle(lay.Rotation))
	ctx.Say("cmd.list.msg.width_factor", ctx.Fmt(lay.WidthFactor))
	ctx.Say("cmd.list.msg.oblique", ctx.FmtAngle(e.Oblique))
	gen := "cmd.list.msg.gen_normal"
	switch e.Generation & (doc.TextBackward | doc.TextUpsideDown) {
	case doc.TextBackward:
		gen = "cmd.list.msg.gen_backward"
	case doc.TextUpsideDown:
		gen = "cmd.list.msg.gen_upside"
	case doc.TextBackward | doc.TextUpsideDown:
		gen = "cmd.list.msg.gen_both"
	}
	ctx.Say("cmd.list.msg.generation", ctx.T(gen))
}

// listMText prints an MTEXT: style, insertion point, attachment, height, box width, rotation and
// the text without its format codes.
func listMText(ctx *cmdsys.Context, e *doc.MText) {
	style := e.Style
	if style == "" {
		style = doc.DefaultTextStyle
	}
	ctx.Say("cmd.list.msg.style", style)
	listPoint(ctx, "cmd.list.msg.insertion_point", e.Insert)
	a := e.Attachment
	if a < 1 || a > 9 {
		a = doc.MTextTopLeft
	}
	ctx.Say("cmd.list.msg.attachment", mtextJustifyKeywords[a-1])
	ctx.Say("cmd.list.msg.height", ctx.Fmt(e.CharHeight()))
	ctx.Say("cmd.list.msg.width", ctx.Fmt(e.Width))
	ctx.Say("cmd.list.msg.rotation", ctx.FmtAngle(e.Rotation))
	ctx.Say("cmd.list.msg.contents", strings.ReplaceAll(doc.MTextPlain(e.Value), "\n", " / "))
}

// listPoint prints a labelled coordinate triple the way LIST does: "from point, X=  0.0000  Y=  0.0000  Z=  0.0000".
func listPoint(ctx *cmdsys.Context, key string, p geom.Vec2) {
	ctx.Say(key, pad(ctx.Fmt(p.X)), pad(ctx.Fmt(p.Y)), pad(ctx.Fmt(0)))
}

func pad(s string) string { return fmt.Sprintf("%8s", s) }

// Transparent marks DIST as runnable inside another command (cmdsys.Transparent).
func (*distCmd) Transparent() bool { return true }

// Transparent marks ID as runnable inside another command (cmdsys.Transparent).
func (*idCmd) Transparent() bool { return true }

// registerInquiry adds DIST, ID and LIST.
func registerInquiry(r *cmdsys.Registry) {
	r.Register("DIST", func() cmdsys.Command { return &distCmd{} })
	r.Register("ID", func() cmdsys.Command { return &idCmd{} })
	r.Register("LIST", func() cmdsys.Command { return &listCmd{} })
	r.Alias("LS", "LIST")
}
