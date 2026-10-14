package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// The settings the construct commands remember between runs are system variables of the session
// (AutoCAD keeps them the same way, and two sessions never share them): OFFSETDIST (negative
// means Through), FILLETRAD, CHAMFERA and CHAMFERB, plus OFFSETERASE and OFFSETLAYER (the Erase and
// Layer options of OFFSET) and CHAMFERD (the angle of CHAMFER's Angle method, in degrees). TRIMMODE,
// CHAMFERC and CHAMMODE are stored placeholders registered by the session.
func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		v.Register(cmdsys.Var{Name: "OFFSETDIST", Kind: cmdsys.VarReal, Default: -1.0})
		v.Register(cmdsys.Var{Name: "FILLETRAD", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "CHAMFERA", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "CHAMFERB", Kind: cmdsys.VarReal, Default: 0.0})
		v.Register(cmdsys.Var{Name: "OFFSETERASE", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 1})
		v.Register(cmdsys.Var{Name: "OFFSETLAYER", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 1})
		if !v.Has("CHAMFERD") {
			v.Register(cmdsys.Var{Name: "CHAMFERD", Kind: cmdsys.VarReal, Default: 0.0})
		}
	})
}

// Keyword ids whose catalog label differs from the id (kw.erase_offset shows "Erase", kw.chamfer_angle
// shows "Angle"): the ids stay distinct from the selection and TRIM keywords of the same slug.
const (
	kwEraseOffset  = "Erase offset"
	kwChamferAngle = "Chamfer angle"
)

// pickAperturePx is the pick aperture of the construct commands, in pixels.
const pickAperturePx = 8

// registerConstruct adds MIRROR, OFFSET, FILLET and CHAMFER and their aliases.
func registerConstruct(r *cmdsys.Registry) {
	r.Register("MIRROR", func() cmdsys.Command { return &mirrorCmd{} })
	r.Register("OFFSET", func() cmdsys.Command { return &offsetCmd{} })
	r.Register("FILLET", func() cmdsys.Command { return &cornerCmd{} })
	r.Register("CHAMFER", func() cmdsys.Command { return &cornerCmd{chamfer: true} })
	r.Alias("MI", "MIRROR")
	r.Alias("O", "OFFSET")
	r.Alias("F", "FILLET")
	r.Alias("CHA", "CHAMFER")
}

func keywordInput(kw string) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: kw}
}

func distanceInput(v float64) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputDistance, Value: v}
}

// pickConstruct returns the entity under p within the pick aperture.
func pickConstruct(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, bool) {
	return cmdsys.Pick(ctx.Doc, ctx.Index, p, pickAperturePx*ctx.View.PixelSize())
}

// --- MIRROR -------------------------------------------------------------

type mirrorCmd struct {
	base
	selecting
	p1, p2 *geom.Vec2
}

func (*mirrorCmd) Name() string { return "MIRROR" }

func (c *mirrorCmd) firstPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.mirror.prompt.first_point"), Accepts: cmdsys.InputPoint}
}

func (c *mirrorCmd) secondPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.mirror.prompt.second_point"), Accepts: cmdsys.InputPoint, Base: c.p1}
}

func (c *mirrorCmd) erasePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.mirror.prompt.erase_source"), Accepts: cmdsys.InputKeyword,
		Keywords: []string{"Yes", "No"}, Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: "N"}, NoOr: true}
}

func (c *mirrorCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.firstPrompt(ctx)
	}
	return selectPrompt(ctx)
}

func (c *mirrorCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.hs == nil:
		if !c.take(ctx, in) {
			return nil
		}
		return c.firstPrompt(ctx)
	case c.p1 == nil:
		c.p1 = pt(in.Point)
		return c.secondPrompt(ctx)
	case c.p2 == nil:
		if in.Point.Near(*c.p1, 1e-12) {
			ctx.Say("cmd.mirror.msg.points_differ")
			return c.secondPrompt(ctx)
		}
		c.p2 = pt(in.Point)
		return c.erasePrompt(ctx)
	}
	m := mirrorMatrix(*c.p1, *c.p2)
	tx := ctx.Tx()
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		cp := e.Clone().(doc.Entity)
		cp.Transform(m)
		layer, color := cp.Common().Layer, cp.Common().Color
		*cp.Common() = doc.EntityCommon{Layer: layer, Color: color}
		tx.Add(cp)
		if in.Keyword == "Yes" {
			tx.Erase(h)
		}
	}
	return nil
}

// mirrorMatrix returns the reflection across the line through a and b.
func mirrorMatrix(a, b geom.Vec2) geom.Mat3 {
	s, k := math.Sincos(2 * b.Sub(a).Angle())
	return geom.Translate(a).Mul(geom.Mat3{k, s, s, -k, 0, 0}).Mul(geom.Translate(a.Scale(-1)))
}

func (c *mirrorCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.p1 == nil || c.p2 != nil {
		return
	}
	dl.Polyline([]geom.Vec2{*c.p1, cur}, PreviewColor.WithAlpha(120), 0, false)
	if cur.Near(*c.p1, 1e-12) {
		return
	}
	c.previewTransformed(ctx, mirrorMatrix(*c.p1, cur), dl)
}

// --- OFFSET -------------------------------------------------------------

type offsetStage int

const (
	offDist offsetStage = iota
	offDist2
	offErase
	offLayer
	offSelect
	offSide
)

// offsetRec is one offset made in this command, so Undo can take it back.
type offsetRec struct {
	added  doc.Handle
	from   doc.Handle // the object it was offset from
	erased doc.Entity // a clone of the source this offset erased, to put back
}

type offsetCmd struct {
	base
	stage    offsetStage
	through  bool
	dist     float64
	first    *geom.Vec2 // first point of a two-point distance
	src      doc.Handle // the object being offset (the newest offset in Multiple mode)
	picked   doc.Handle // the object the user selected
	multiple bool
	done     []offsetRec
}

func (*offsetCmd) Name() string { return "OFFSET" }

func (c *offsetCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	layer := ctx.T("cmd.offset.msg.layer_source")
	if ctx.Vars().Int("OFFSETLAYER") == 1 {
		layer = ctx.T("cmd.offset.msg.layer_current")
	}
	ctx.Say("cmd.offset.msg.settings", yesNo(ctx, ctx.Vars().Int("OFFSETERASE") == 1), layer, ctx.Vars().Int("OFFSETGAPTYPE"))
	return c.distPrompt(ctx)
}

func (c *offsetCmd) distPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage, c.first = offDist, nil
	def := keywordInput("Through")
	if d := ctx.Vars().Real("OFFSETDIST"); d > 0 {
		def = distanceInput(d)
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.offset.prompt.distance"), Accepts: cmdsys.InputDistance | cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Through", kwEraseOffset, "Layer"}, Default: def}
}

func (c *offsetCmd) selectPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage, c.src, c.picked, c.multiple = offSelect, 0, 0, false
	return &cmdsys.Prompt{Message: ctx.T("cmd.offset.prompt.select"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Exit", "Undo"}, Default: keywordInput("Exit")}
}

func (c *offsetCmd) sidePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = offSide
	msg := ctx.T("cmd.offset.prompt.side")
	if c.through {
		msg = ctx.T("cmd.offset.prompt.through")
	}
	p := &cmdsys.Prompt{Message: msg, Accepts: cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Exit", "Multiple", "Undo"}, Default: keywordInput("Exit")}
	if c.multiple {
		p.Keywords = []string{"Exit", "Undo"}
		p.Accepts |= cmdsys.InputEnter
		p.Default = &cmdsys.Input{Kind: cmdsys.InputText, Text: ctx.T("cmd.offset.msg.next_object")}
	}
	return p
}

func (c *offsetCmd) setDistance(ctx *cmdsys.Context, d float64) *cmdsys.Prompt {
	if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		ctx.Say("cmd.common.msg.positive")
		return c.distPrompt(ctx)
	}
	c.dist, c.through = d, false
	_ = ctx.Vars().Set("OFFSETDIST", d)
	return c.selectPrompt(ctx)
}

// undo takes the last offset back and returns false when there is none. An erased source comes
// back as a new object (the transaction cannot un-erase), so a later offset from it follows.
func (c *offsetCmd) undo(ctx *cmdsys.Context) bool {
	n := len(c.done)
	if n == 0 {
		return false
	}
	r := c.done[n-1]
	c.done = c.done[:n-1]
	tx := ctx.Tx()
	tx.Erase(r.added)
	back := r.from
	if r.erased != nil {
		back = tx.Add(r.erased)
		for i := range c.done {
			if c.done[i].from == r.from {
				c.done[i].from = back
			}
		}
		if c.picked == r.from {
			c.picked = back
		}
	}
	c.src = back
	return true
}

func (c *offsetCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case offDist:
		return c.inDist(ctx, in)
	case offDist2:
		if in.Kind != cmdsys.InputPoint {
			return c.distPrompt(ctx)
		}
		return c.setDistance(ctx, c.first.Dist(in.Point))
	case offErase:
		_ = ctx.Vars().Set("OFFSETERASE", boolToInt(in.Keyword == "Yes"))
		return c.distPrompt(ctx)
	case offLayer:
		_ = ctx.Vars().Set("OFFSETLAYER", boolToInt(in.Keyword == "Current"))
		return c.distPrompt(ctx)
	case offSelect:
		return c.inSelect(ctx, in)
	}
	return c.inSide(ctx, in)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (c *offsetCmd) inDist(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputKeyword:
		switch in.Keyword {
		case "Through":
			c.through = true
			_ = ctx.Vars().Set("OFFSETDIST", -1.0)
			return c.selectPrompt(ctx)
		case kwEraseOffset:
			c.stage = offErase
			def := "No"
			if ctx.Vars().Int("OFFSETERASE") == 1 {
				def = "Yes"
			}
			return &cmdsys.Prompt{Message: ctx.T("cmd.offset.prompt.erase"), Accepts: cmdsys.InputKeyword,
				Keywords: []string{"Yes", "No"}, Default: keywordInput(def), NoOr: true}
		case "Layer":
			c.stage = offLayer
			def := "Source"
			if ctx.Vars().Int("OFFSETLAYER") == 1 {
				def = "Current"
			}
			return &cmdsys.Prompt{Message: ctx.T("cmd.offset.prompt.layer"), Accepts: cmdsys.InputKeyword,
				Keywords: []string{"Current", "Source"}, Default: keywordInput(def), NoOr: true}
		}
	case in.Kind == cmdsys.InputPoint:
		c.first, c.stage = pt(in.Point), offDist2
		return &cmdsys.Prompt{Message: ctx.T("cmd.offset.prompt.second_point"), Accepts: cmdsys.InputPoint, Base: c.first}
	}
	return c.setDistance(ctx, in.Value)
}

func (c *offsetCmd) inSelect(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputKeyword && in.Keyword == "Undo" {
		c.undo(ctx)
		return c.selectPrompt(ctx)
	}
	if in.Kind != cmdsys.InputPoint {
		return nil
	}
	h, ok := pickConstruct(ctx, in.Point)
	if !ok {
		ctx.Say("cmd.offset.msg.nothing_found")
		return c.selectPrompt(ctx)
	}
	if !offsettable(ctx.Doc.Entity(h)) {
		ctx.Say("cmd.offset.msg.cannot_offset")
		return c.selectPrompt(ctx)
	}
	c.src, c.picked = h, h
	return c.sidePrompt(ctx)
}

func (c *offsetCmd) inSide(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputEnter && c.multiple:
		return c.selectPrompt(ctx)
	case in.Kind == cmdsys.InputKeyword && in.Keyword == "Multiple":
		c.multiple = true
		return c.sidePrompt(ctx)
	case in.Kind == cmdsys.InputKeyword && in.Keyword == "Undo":
		if c.undo(ctx) && c.multiple {
			return c.sidePrompt(ctx)
		}
		return c.selectPrompt(ctx)
	case in.Kind != cmdsys.InputPoint:
		return nil
	}
	src := ctx.Doc.Entity(c.src)
	if src == nil {
		return c.selectPrompt(ctx)
	}
	ne, msg := offsetEntity(src, in.Point, c.dist, c.through)
	if ne == nil {
		ctx.Say(msg)
		if c.multiple {
			return c.sidePrompt(ctx)
		}
		return c.selectPrompt(ctx)
	}
	if ctx.Vars().Int("OFFSETLAYER") == 1 {
		ne.Common().Layer, ne.Common().Color = "", 0
	}
	tx := ctx.Tx()
	rec := offsetRec{from: c.src}
	if ctx.Vars().Int("OFFSETERASE") == 1 && c.src == c.picked {
		rec.erased = src.Clone().(doc.Entity)
		tx.Erase(c.src)
	}
	rec.added = tx.Add(ne)
	c.done = append(c.done, rec)
	if c.multiple {
		c.src = rec.added
		return c.sidePrompt(ctx)
	}
	return c.selectPrompt(ctx)
}

func (c *offsetCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.stage != offSide || c.src == 0 {
		return
	}
	src := ctx.Doc.Entity(c.src)
	if src == nil {
		return
	}
	ne, _ := offsetEntity(src, cur, c.dist, c.through)
	if ne == nil {
		return
	}
	tol := previewTol(ctx)
	for _, g := range ne.Geometry() {
		dl.Curve(g, tol, PreviewColor, 1)
	}
}

func offsettable(e doc.Entity) bool {
	switch e.(type) {
	case *doc.Line, *doc.Circle, *doc.Arc, *doc.LWPolyline:
		return true
	}
	return false
}

const degenerateOffset = "cmd.offset.msg.degenerate"

// offsetEntity returns the offset of e towards the point side, dist away or,
// with through, passing through side. It returns nil and a message key when the
// result is degenerate or e cannot be offset. The new entity inherits the
// layer and colour of e.
func offsetEntity(e doc.Entity, side geom.Vec2, dist float64, through bool) (doc.Entity, string) {
	var out doc.Entity
	switch e := e.(type) {
	case *doc.Line:
		l := geom.Line{A: e.Start, B: e.End}
		cross := l.B.Sub(l.A).Cross(side.Sub(l.A))
		if through {
			dist = math.Abs(cross) / math.Max(l.Length(), 1e-300)
		}
		if dist <= 0 {
			return nil, "cmd.offset.msg.through_on_object"
		}
		if cross < 0 {
			dist = -dist
		}
		o := geom.OffsetLine(l, dist)
		out = &doc.Line{Start: o.A, End: o.B}
	case *doc.Circle:
		d := side.Dist(e.Center) - e.Radius
		if through {
			dist = d
		} else if d < 0 {
			dist = -dist
		}
		if dist == 0 {
			return nil, "cmd.offset.msg.through_on_object"
		}
		o, ok := geom.OffsetCircle(geom.Circle{C: e.Center, R: e.Radius}, dist)
		if !ok {
			return nil, degenerateOffset
		}
		out = &doc.Circle{Center: o.C, Radius: o.R}
	case *doc.Arc:
		d := side.Dist(e.Center) - e.Radius
		if through {
			dist = d
		} else if d < 0 {
			dist = -dist
		}
		if dist == 0 {
			return nil, "cmd.offset.msg.through_on_object"
		}
		o, ok := geom.OffsetArc(geom.Arc{C: e.Center, R: e.Radius, Start: e.StartAngle, End: e.EndAngle}, dist)
		if !ok {
			return nil, degenerateOffset
		}
		out = &doc.Arc{Center: o.C, Radius: o.R, StartAngle: o.Start, EndAngle: o.End}
	case *doc.LWPolyline:
		p := e.Geometry()[0].(geom.Polyline)
		t := p.ClosestParam(side)
		q := p.PointAt(t)
		if through {
			dist = q.Dist(side)
		}
		if dist <= 0 {
			return nil, "cmd.offset.msg.through_on_object"
		}
		if p.Tangent(t).Cross(side.Sub(q)) < 0 {
			dist = -dist
		}
		o, ok := geom.OffsetPolyline(p, dist)
		if !ok {
			return nil, degenerateOffset
		}
		np := &doc.LWPolyline{Points: o.Pts, Bulges: o.Bulges, Closed: o.Closed}
		if len(o.Pts) == len(e.Points) {
			np.Widths = append([][2]float64(nil), e.Widths...)
		}
		out = np
	default:
		return nil, "cmd.offset.msg.cannot_offset"
	}
	c := out.Common()
	c.Layer, c.Color = e.Common().Layer, e.Common().Color
	return out, ""
}
