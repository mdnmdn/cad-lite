// Package commands holds the built-in AutoCAD-style commands. Register adds
// them, with their default aliases, to a registry.
package commands

import (
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// The draw commands keep their remembered defaults as session variables: CIRCLERAD (the radius
// CIRCLE offers), PLINEWID (the width PLINE starts with) and POLYSIDES (the side count POLYGON
// offers) are AutoCAD's own; the DRAW_ ones are command memory AutoCAD keeps outside the
// variable list (RECTANG's chamfer, fillet, width, elevation, thickness, length and area, and
// POLYGON's last inscribed/circumscribed choice). Being session variables, two sessions never
// share them.
func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		v.Register(cmdsys.Var{Name: "CIRCLERAD", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "PLINEWID", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "POLYSIDES", Kind: cmdsys.VarInt, Default: 4, Min: 3, Max: 1024})
		v.Register(cmdsys.Var{Name: "DRAW_POLYCIRCUMSCRIBED", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 1})
		v.Register(cmdsys.Var{Name: "DRAW_RECTCHAMFER1", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTCHAMFER2", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTFILLET", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTWIDTH", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTELEVATION", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTTHICKNESS", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTLENGTH", Kind: cmdsys.VarReal, Default: 10.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTDEPTH", Kind: cmdsys.VarReal, Default: 10.0})
		v.Register(cmdsys.Var{Name: "DRAW_RECTAREA", Kind: cmdsys.VarReal, Default: 100.0})
	})
}

// kwShortcut returns the typed shortcut of keyword id in the session's language: the capitals of
// its label, or its first letter. Prompt defaults that name a keyword print it this way (<I>).
func kwShortcut(ctx *cmdsys.Context, id string) string {
	label := ctx.Translator().KeywordLabel(id)
	var caps []rune
	for _, r := range label {
		if r >= 'A' && r <= 'Z' || r >= 'À' && r <= 'Þ' {
			caps = append(caps, r)
		}
	}
	if len(caps) > 0 {
		return string(caps)
	}
	for _, r := range label {
		return string(r)
	}
	return id
}

// distDefault returns a distance default for a prompt, nil when v is not positive.
func distDefault(v float64) *cmdsys.Input {
	if v > 0 {
		return &cmdsys.Input{Kind: cmdsys.InputDistance, Value: v}
	}
	return nil
}

// --- LINE ---------------------------------------------------------------

type lineCmd struct {
	pts   []geom.Vec2
	added []doc.Handle
	// tangent is set after Enter at the first prompt continued an arc: the next input is the
	// length of a line tangent to it.
	tangent    float64
	needLength bool
}

func (*lineCmd) Name() string { return "LINE" }

func (c *lineCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.line.prompt.first_point"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
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

// lastEndIsArc reports whether the last line or arc a command made is an arc ending at p.
func lastEndIsArc(ctx *cmdsys.Context, p geom.Vec2) bool {
	h, ok := ctx.LastEntity()
	if !ok {
		return false
	}
	a, ok := ctx.Doc.Entity(h).(*doc.Arc)
	if !ok {
		return false
	}
	g := geom.Arc{C: a.Center, R: a.Radius, Start: a.StartAngle, End: a.EndAngle}
	return g.StartPoint().Near(p, 1e-6) || g.EndPoint().Near(p, 1e-6)
}

// addSegment draws the line a-b and records its end for Enter-continue.
func (c *lineCmd) addSegment(ctx *cmdsys.Context, a, b geom.Vec2) {
	c.added = append(c.added, ctx.Tx().Add(&doc.Line{Start: a, End: b}))
	ctx.SetLastEnd(b, b.Sub(a).Angle())
}

func (c *lineCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.needLength {
		if in.Kind != cmdsys.InputDistance || in.Value == 0 {
			return c.lengthPrompt(ctx)
		}
		start := c.pts[0]
		c.needLength = false
		end := start.Polar(in.Value, c.tangent)
		c.addSegment(ctx, start, end)
		c.pts = append(c.pts, end)
		return c.prompt(ctx)
	}
	switch {
	case in.Kind == cmdsys.InputEnter && len(c.pts) == 0:
		// Continue from the end of the last line or arc, else from the last point.
		p, tan, ok := ctx.LastEnd()
		if !ok {
			p = ctx.LastPoint()
		}
		c.pts = append(c.pts, p)
		if ok && lastEndIsArc(ctx, p) {
			c.tangent, c.needLength = tan, true
			return c.lengthPrompt(ctx)
		}
		return c.prompt(ctx)
	case in.Kind == cmdsys.InputEnter:
		return nil
	case in.Keyword == "Undo":
		if len(c.added) == 0 {
			c.pts = c.pts[:0]
			return c.Start(ctx)
		}
		ctx.Tx().Erase(c.added[len(c.added)-1])
		c.added = c.added[:len(c.added)-1]
		c.pts = c.pts[:len(c.pts)-1]
		if n := len(c.added); n > 0 {
			ctx.SetLastEnd(c.pts[len(c.pts)-1], c.pts[len(c.pts)-1].Sub(c.pts[len(c.pts)-2]).Angle())
		}
		return c.prompt(ctx)
	case in.Keyword == "Close":
		last, first := c.pts[len(c.pts)-1], c.pts[0]
		c.addSegment(ctx, last, first)
		return nil
	case in.Kind == cmdsys.InputPoint:
		if n := len(c.pts); n > 0 {
			c.addSegment(ctx, c.pts[n-1], in.Point)
		}
		c.pts = append(c.pts, in.Point)
		return c.prompt(ctx)
	}
	return c.prompt(ctx)
}

func (c *lineCmd) lengthPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.line.prompt.length"), Accepts: cmdsys.InputDistance, Base: pt(c.pts[0])}
}

func (c *lineCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	n := len(c.pts)
	if n == 0 {
		return
	}
	if c.needLength {
		dl.Polyline([]geom.Vec2{c.pts[0], c.pts[0].Polar(math.Max(cur.Dist(c.pts[0]), 0), c.tangent)}, PreviewColor, 1, false)
		return
	}
	dl.Polyline([]geom.Vec2{c.pts[n-1], cur}, PreviewColor, 1, false)
}

func (*lineCmd) Cancel(*cmdsys.Context) {}

// registerDraw adds the draw commands (draw.go, pline.go, circle.go, arc.go, rect.go) and, from
// polygon.go and point.go, POLYGON.
func registerDraw(r *cmdsys.Registry) {
	r.Register("LINE", func() cmdsys.Command { return &lineCmd{} })
	r.Register("PLINE", func() cmdsys.Command { return &plineCmd{} })
	r.Register("CIRCLE", func() cmdsys.Command { return &circleCmd{} })
	r.Register("ARC", func() cmdsys.Command { return &arcCmd{} })
	r.Register("RECTANG", func() cmdsys.Command { return &rectCmd{} })
	registerPolygon(r)
}
