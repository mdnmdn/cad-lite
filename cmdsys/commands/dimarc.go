package commands

import (
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- DIMARC -------------------------------------------------------------

// dimArcKeywords are the options of DIMARC's location prompt, in AutoCAD's order. Leader is
// offered and answered with a note: arc length dimensions have no leader here.
var dimArcKeywords = []string{"Mtext", "Text", "Angle", "Partial", "Leader"}

type dimArcStep int

const (
	daSelect    dimArcStep = iota // arc or polyline arc segment
	daLocation                    // dimension location or options
	daPartial1                    // Partial: first point
	daPartial2                    // Partial: second point
	daText                        // Text, Mtext: the dimension text
	daTextAngle                   // Angle: the text angle
)

// dimArcCmd is DIMARC: an arc length dimension of an arc or a polyline arc segment, whole or,
// with Partial, between two points on it.
type dimArcCmd struct {
	base
	step    dimArcStep
	arc     geom.Arc
	partial bool
	q1, q2  geom.Vec2
	text    string
	tAngle  float64
}

func (*dimArcCmd) Name() string { return "DIMARC" }

func (c *dimArcCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.prompt(ctx) }

func (c *dimArcCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.step {
	case daLocation:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.arc_location"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: dimArcKeywords}
	case daPartial1:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.partial_first"), Accepts: cmdsys.InputPoint}
	case daPartial2:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.partial_second"), Accepts: cmdsys.InputPoint, Base: pt(c.q1)}
	case daText:
		return dimTextPrompt(ctx, measuredText(c.entity(ctx, c.arc.MidPoint())))
	case daTextAngle:
		return dimTextAnglePrompt(ctx, c.arc.MidPoint())
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.select_arc"), Accepts: cmdsys.InputPoint, Pick: true}
}

func (c *dimArcCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case daSelect:
		if c.pickArc(ctx, in.Point) {
			c.step = daLocation
		}
	case daLocation:
		switch in.Keyword {
		case "Mtext", "Text":
			c.step = daText
		case "Angle":
			c.step = daTextAngle
		case "Partial":
			c.step = daPartial1
		case "Leader":
			ctx.Say("cmd.dim.msg.no_leader")
		case "":
			addDim(ctx, c.entity(ctx, in.Point))
			return nil
		}
	case daPartial1:
		c.q1, c.step = c.onArc(in.Point), daPartial2
	case daPartial2:
		c.q2, c.partial, c.step = c.onArc(in.Point), true, daLocation
	case daText:
		c.text, c.step = textAnswer(in), daLocation
	case daTextAngle:
		c.tAngle, c.step = in.Value, daLocation
	}
	return c.prompt(ctx)
}

// pickArc takes the arc at p: an ARC or the picked arc segment of a polyline.
func (c *dimArcCmd) pickArc(ctx *cmdsys.Context, p geom.Vec2) bool {
	e, ok := pickAt(ctx, p)
	if !ok {
		ctx.Say("cmd.common.msg.no_object_found")
		return false
	}
	if a, ok := arcOf(e, p); ok {
		c.arc = a
		return true
	}
	ctx.Say("cmd.dim.msg.not_arc")
	return false
}

// arcOf returns the arc DIMARC measures on e picked at p.
func arcOf(e doc.Entity, p geom.Vec2) (geom.Arc, bool) {
	switch e := e.(type) {
	case *doc.Arc:
		a, ok := e.Geometry()[0].(geom.Arc)
		return a, ok
	case *doc.LWPolyline:
		if pl, ok := e.Geometry()[0].(geom.Polyline); ok {
			if s, ok := nearestSegment(pl, p); ok {
				a, ok := s.(geom.Arc)
				return a, ok
			}
		}
	}
	return geom.Arc{}, false
}

// onArc projects p radially onto the arc, clamped to its nearer end when it falls outside it.
func (c *dimArcCmd) onArc(p geom.Vec2) geom.Vec2 {
	a := p.Sub(c.arc.C).Angle()
	if p.Near(c.arc.C, 0) {
		a = c.arc.Start
	}
	if !c.arc.ContainsAngle(a) {
		past, before := geom.NormAngle(a-c.arc.End), geom.NormAngle(c.arc.Start-a)
		a = c.arc.End
		if before < past {
			a = c.arc.Start
		}
	}
	return c.arc.C.Add(geom.V(math.Cos(a), math.Sin(a)).Scale(c.arc.R))
}

// entity returns the dimension the command makes with its arc through loc.
func (c *dimArcCmd) entity(ctx *cmdsys.Context, loc geom.Vec2) *doc.Dimension {
	d := &doc.Dimension{Kind: doc.DimArcLength, Center: c.arc.C, Radius: c.arc.R, StartAngle: c.arc.Start, EndAngle: c.arc.End,
		DimLine: loc, Partial: c.partial, Ext1: c.q1, Ext2: c.q2, ArcSymbol: ctx.Vars().Int("DIMARCSYM"),
		Text: c.text, TextAngle: c.tAngle}
	d.SetStyle(ctx.Doc, dimStyle(ctx))
	d.Recompute()
	return d
}

func (c *dimArcCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch c.step {
	case daSelect:
		previewHover(ctx, cur, dl, func(e doc.Entity) bool { _, ok := arcOf(e, cur); return ok })
	case daLocation:
		previewDim(ctx, c.entity(ctx, cur), dl)
	case daPartial2:
		dl.Polyline([]geom.Vec2{c.q1, c.onArc(cur)}, PreviewColor.WithAlpha(120), 0, false)
	}
}
