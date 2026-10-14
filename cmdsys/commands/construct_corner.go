package commands

import (
	"errors"
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- FILLET and CHAMFER -------------------------------------------------

type cornerStage int

const (
	cnFirst  cornerStage = iota // select the first line (or a keyword)
	cnSecond                    // select the second line
	cnPoly                      // select a polyline
	cnAsk                       // a number is being asked (radius, distances, length, angle)
	cnTrim                      // the Trim / No trim option
	cnMethod                    // the Distance / Angle method option
)

// cornerRec is one corner made in this command, so Undo can take it back.
type cornerRec struct {
	before map[doc.Handle]doc.Entity
	added  []doc.Handle
}

// cornerCmd is FILLET or, with chamfer set, CHAMFER: both cut two lines back to a corner and join
// them with an arc or a bevel, or do the same at every vertex of one polyline.
type cornerCmd struct {
	base
	chamfer  bool
	stage    cornerStage
	back     cornerStage // the stage to return to after an option's value
	ask      string      // "radius", "d1", "d2", "len", "ang" while a number is being asked
	multiple bool
	h1       doc.Handle
	p1       geom.Vec2
	d1       float64
	done     []cornerRec
}

func (c *cornerCmd) Name() string {
	if c.chamfer {
		return "CHAMFER"
	}
	return "FILLET"
}

func (c *cornerCmd) noun() string {
	if c.chamfer {
		return "line"
	}
	return "object"
}

func (c *cornerCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	mode := ctx.T("cmd.corner.msg.mode_trim")
	if ctx.Vars().Int("TRIMMODE") == 0 {
		mode = ctx.T("cmd.corner.msg.mode_notrim")
	}
	v := ctx.Vars()
	switch {
	case !c.chamfer:
		ctx.Say("cmd.corner.msg.fillet_settings", mode, cmdsys.FormatNumber(v.Real("FILLETRAD")))
	case v.Int("CHAMMODE") == 1:
		ctx.Say("cmd.corner.msg.chamfer_settings_angle", mode, cmdsys.FormatNumber(v.Real("CHAMFERC")), cmdsys.FormatNumber(v.Real("CHAMFERD")))
	default:
		ctx.Say("cmd.corner.msg.chamfer_settings", mode, cmdsys.FormatNumber(v.Real("CHAMFERA")), cmdsys.FormatNumber(v.Real("CHAMFERB")))
	}
	return c.firstPrompt(ctx)
}

// optionKeywords are the keywords the second-object and polyline prompts offer.
func (c *cornerCmd) optionKeywords() []string {
	if c.chamfer {
		return []string{"Distance", kwChamferAngle, "Method"}
	}
	return []string{"Radius"}
}

func (c *cornerCmd) firstPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.h1, c.ask, c.stage = 0, "", cnFirst
	kws := []string{"Undo", "Polyline", "Radius", "Trim", "Multiple"}
	if c.chamfer {
		kws = []string{"Undo", "Polyline", "Distance", kwChamferAngle, "Trim", "Method", "Multiple"}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.select_first_" + c.noun()),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: kws}
}

func (c *cornerCmd) secondPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = cnSecond
	return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.select_second_" + c.noun()),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: c.optionKeywords()}
}

func (c *cornerCmd) polyPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = cnPoly
	return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.select_polyline"),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: c.optionKeywords()}
}

// backPrompt returns to the prompt an option was chosen from.
func (c *cornerCmd) backPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.back {
	case cnSecond:
		return c.secondPrompt(ctx)
	case cnPoly:
		return c.polyPrompt(ctx)
	}
	return c.firstPrompt(ctx)
}

func (c *cornerCmd) askPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = cnAsk
	v := ctx.Vars()
	switch c.ask {
	case "radius":
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.fillet_radius"), Accepts: cmdsys.InputDistance, Default: distanceInput(v.Real("FILLETRAD"))}
	case "d1":
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.chamfer_d1"), Accepts: cmdsys.InputDistance, Default: distanceInput(v.Real("CHAMFERA"))}
	case "len":
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.chamfer_len"), Accepts: cmdsys.InputDistance, Default: distanceInput(v.Real("CHAMFERC"))}
	case "ang":
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.chamfer_ang"), Accepts: cmdsys.InputAngle,
			Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: v.Real("CHAMFERD") * math.Pi / 180}}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.chamfer_d2"), Accepts: cmdsys.InputDistance, Default: distanceInput(c.d1)}
}

func (c *cornerCmd) inAsk(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Value < 0 || math.IsNaN(in.Value) || math.IsInf(in.Value, 0) {
		ctx.Say("cmd.corner.msg.non_negative")
		return c.askPrompt(ctx)
	}
	v := ctx.Vars()
	switch c.ask {
	case "radius":
		_ = v.Set("FILLETRAD", in.Value)
	case "d1":
		c.d1 = in.Value
		_ = v.Set("CHAMFERA", in.Value)
		c.ask = "d2"
		return c.askPrompt(ctx)
	case "d2":
		_ = v.Set("CHAMFERB", in.Value)
		_ = v.Set("CHAMMODE", 0)
	case "len":
		_ = v.Set("CHAMFERC", in.Value)
		c.ask = "ang"
		return c.askPrompt(ctx)
	case "ang":
		_ = v.Set("CHAMFERD", in.Value*180/math.Pi)
		_ = v.Set("CHAMMODE", 1)
	}
	c.ask = ""
	return c.backPrompt(ctx)
}

// keyword handles an option typed at a pick prompt.
func (c *cornerCmd) keyword(ctx *cmdsys.Context, kw string) *cmdsys.Prompt {
	switch kw {
	case "Undo":
		c.undo(ctx)
		return c.firstPrompt(ctx)
	case "Polyline":
		return c.polyPrompt(ctx)
	case "Multiple":
		c.multiple = true
		return c.firstPrompt(ctx)
	case "Trim":
		c.back, c.stage = c.stage, cnTrim
		def := "Trim"
		if ctx.Vars().Int("TRIMMODE") == 0 {
			def = "No trim"
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.trim_mode"), Accepts: cmdsys.InputKeyword,
			Keywords: []string{"Trim", "No trim"}, Default: keywordInput(def), NoOr: true}
	case "Method":
		c.back, c.stage = c.stage, cnMethod
		def := "Distance"
		if ctx.Vars().Int("CHAMMODE") == 1 {
			def = kwChamferAngle
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.corner.prompt.method"), Accepts: cmdsys.InputKeyword,
			Keywords: []string{"Distance", kwChamferAngle}, Default: keywordInput(def), NoOr: true}
	}
	c.back = c.stage
	switch kw {
	case "Radius":
		c.ask = "radius"
	case "Distance":
		c.ask = "d1"
	default:
		c.ask = "len"
	}
	return c.askPrompt(ctx)
}

func (c *cornerCmd) lineAt(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, *doc.Line) {
	h, ok := pickConstruct(ctx, p)
	if !ok {
		return 0, nil
	}
	l, _ := ctx.Doc.Entity(h).(*doc.Line)
	return h, l
}

func (c *cornerCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case cnAsk:
		return c.inAsk(ctx, in)
	case cnTrim:
		_ = ctx.Vars().Set("TRIMMODE", boolToInt(in.Keyword != "No trim"))
		return c.backPrompt(ctx)
	case cnMethod:
		_ = ctx.Vars().Set("CHAMMODE", boolToInt(in.Keyword == kwChamferAngle))
		return c.backPrompt(ctx)
	}
	switch in.Kind {
	case cmdsys.InputKeyword:
		return c.keyword(ctx, in.Keyword)
	case cmdsys.InputPoint:
	default:
		return nil
	}
	switch c.stage {
	case cnPoly:
		return c.polyline(ctx, in)
	case cnFirst:
		h, ok := pickConstruct(ctx, in.Point)
		if !ok {
			ctx.Say("cmd.corner.msg.nothing_found")
			return c.firstPrompt(ctx)
		}
		if _, isLine := ctx.Doc.Entity(h).(*doc.Line); !isLine {
			ctx.Say("cmd.corner.msg.supports_two_lines", c.title(ctx))
			return nil
		}
		c.h1, c.p1 = h, in.Point
		return c.secondPrompt(ctx)
	}
	return c.second(ctx, in)
}

func (c *cornerCmd) second(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	h2, l2 := c.lineAt(ctx, in.Point)
	switch {
	case h2 == 0:
		ctx.Say("cmd.corner.msg.nothing_found")
		return c.secondPrompt(ctx)
	case l2 == nil:
		ctx.Say("cmd.corner.msg.supports_two_lines", c.title(ctx))
		return nil
	case h2 == c.h1:
		ctx.Say("cmd.corner.msg.select_different")
		return c.secondPrompt(ctx)
	}
	l1 := ctx.Doc.Entity(c.h1).(*doc.Line)
	corner, err := c.compute(ctx, l1, l2, c.p1, in.Point, in.Shift)
	if err != nil {
		switch {
		case errors.Is(err, geom.ErrParallel):
			ctx.Say("cmd.corner.msg.parallel")
		case c.chamfer:
			ctx.Say("cmd.corner.msg.chamfer_too_large")
		default:
			ctx.Say("cmd.corner.msg.radius_too_large")
		}
		return nil
	}
	tx := ctx.Tx()
	rec := cornerRec{before: map[doc.Handle]doc.Entity{}}
	if ctx.Vars().Int("TRIMMODE") != 0 {
		rec.before[c.h1], rec.before[h2] = l1.Clone().(doc.Entity), l2.Clone().(doc.Entity)
		tx.Modify(c.h1, func(e doc.Entity) { cutLine(e.(*doc.Line), corner.Far1, corner.Join1) })
		tx.Modify(h2, func(e doc.Entity) { cutLine(e.(*doc.Line), corner.Far2, corner.Join2) })
	}
	var ne doc.Entity
	switch {
	case corner.Arc != nil:
		ne = &doc.Arc{Center: corner.Arc.C, Radius: corner.Arc.R, StartAngle: corner.Arc.Start, EndAngle: corner.Arc.End}
	case corner.Bevel != nil:
		ne = &doc.Line{Start: corner.Bevel.A, End: corner.Bevel.B}
	}
	if ne != nil {
		ne.Common().Layer, ne.Common().Color = l1.Layer, l1.Color
		rec.added = append(rec.added, tx.Add(ne))
	}
	c.done = append(c.done, rec)
	return c.finish(ctx)
}

// finish ends the command, or with Multiple asks for the next pair.
func (c *cornerCmd) finish(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.multiple {
		return c.firstPrompt(ctx)
	}
	return nil
}

// polyline fillets or chamfers every vertex of the picked polyline.
func (c *cornerCmd) polyline(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	h, ok := pickConstruct(ctx, in.Point)
	if !ok {
		ctx.Say("cmd.corner.msg.nothing_found")
		return c.polyPrompt(ctx)
	}
	pl, ok := ctx.Doc.Entity(h).(*doc.LWPolyline)
	if !ok {
		ctx.Say("cmd.corner.msg.not_polyline")
		return c.polyPrompt(ctx)
	}
	cut := c.polyCut(ctx)
	out, widths, n, skipped := geom.CutPolyline(pl.Geometry()[0].(geom.Polyline), pl.Widths, cut)
	if n > 0 {
		tx := ctx.Tx()
		rec := cornerRec{before: map[doc.Handle]doc.Entity{h: pl.Clone().(doc.Entity)}}
		tx.Modify(h, func(e doc.Entity) {
			p := e.(*doc.LWPolyline)
			p.Points, p.Bulges, p.Closed, p.Widths = out.Pts, out.Bulges, out.Closed, widths
		})
		c.done = append(c.done, rec)
	}
	done := "cmd.corner.msg.filleted"
	if c.chamfer {
		done = "cmd.corner.msg.chamfered"
	}
	ctx.SayPlural(done, n)
	if skipped > 0 {
		ctx.SayPlural("cmd.corner.msg.too_short", skipped)
	}
	return c.finish(ctx)
}

// polyCut is the vertex cut the current settings describe.
func (c *cornerCmd) polyCut(ctx *cmdsys.Context) geom.CutFunc {
	v := ctx.Vars()
	if !c.chamfer {
		return geom.FilletCut(v.Real("FILLETRAD"))
	}
	if v.Int("CHAMMODE") != 1 {
		return geom.ChamferCut(v.Real("CHAMFERA"), v.Real("CHAMFERB"))
	}
	length, ang := v.Real("CHAMFERC"), v.Real("CHAMFERD")*math.Pi/180
	return func(u1, u2 geom.Vec2, la, lb, turn float64) (geom.CornerCut, bool) {
		phi := math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2))))
		d2, ok := geom.ChamferAngleDistance(phi, length, ang)
		if !ok {
			return geom.CornerCut{}, false
		}
		return geom.ChamferCut(length, d2)(u1, u2, la, lb, turn)
	}
}

// undo takes the last corner back and reports whether there was one.
func (c *cornerCmd) undo(ctx *cmdsys.Context) bool {
	n := len(c.done)
	if n == 0 {
		return false
	}
	r := c.done[n-1]
	c.done = c.done[:n-1]
	tx := ctx.Tx()
	for h, e := range r.before {
		tx.Modify(h, func(cur doc.Entity) { restoreEntity(cur, e) })
	}
	for _, h := range r.added {
		tx.Erase(h)
	}
	return true
}

// title is the command's display name in the session's language ("Fillet", "Raccordo").
func (c *cornerCmd) title(ctx *cmdsys.Context) string {
	return ctx.Translator().CommandName(c.Name())
}

// compute returns the corner of two lines under the current settings; sharp (a shift-pick) forces
// radius or distances of zero.
func (c *cornerCmd) compute(ctx *cmdsys.Context, l1, l2 *doc.Line, p1, p2 geom.Vec2, sharp bool) (geom.Corner, error) {
	a, b := geom.Line{A: l1.Start, B: l1.End}, geom.Line{A: l2.Start, B: l2.End}
	v := ctx.Vars()
	if !c.chamfer {
		r := v.Real("FILLETRAD")
		if sharp {
			r = 0
		}
		return geom.FilletLines(a, b, p1, p2, r)
	}
	d1, d2 := v.Real("CHAMFERA"), v.Real("CHAMFERB")
	if v.Int("CHAMMODE") == 1 {
		phi, err := geom.CornerWedge(a, b, p1, p2)
		if err != nil {
			return geom.Corner{}, err
		}
		var ok bool
		d1 = v.Real("CHAMFERC")
		if d2, ok = geom.ChamferAngleDistance(phi, d1, v.Real("CHAMFERD")*math.Pi/180); !ok {
			return geom.Corner{}, geom.ErrTooLarge
		}
	}
	if sharp {
		d1, d2 = 0, 0
	}
	return geom.ChamferLines(a, b, p1, p2, d1, d2)
}

// cutLine moves the end of l that is farther from far to join, keeping far.
func cutLine(l *doc.Line, far, join geom.Vec2) {
	if l.Start.Dist(far) <= l.End.Dist(far) {
		l.End = join
	} else {
		l.Start = join
	}
}

func (c *cornerCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.stage != cnSecond || c.h1 == 0 {
		return
	}
	l1, _ := ctx.Doc.Entity(c.h1).(*doc.Line)
	h2, l2 := c.lineAt(ctx, cur)
	if l1 == nil || l2 == nil || h2 == c.h1 {
		return
	}
	corner, err := c.compute(ctx, l1, l2, c.p1, cur, false)
	if err != nil {
		return
	}
	tol := previewTol(ctx)
	if ctx.Vars().Int("TRIMMODE") != 0 {
		dl.Curve(geom.Line{A: corner.Far1, B: corner.Join1}, tol, PreviewColor, 1)
		dl.Curve(geom.Line{A: corner.Far2, B: corner.Join2}, tol, PreviewColor, 1)
	}
	switch {
	case corner.Arc != nil:
		dl.Curve(*corner.Arc, tol, PreviewColor, 1)
	case corner.Bevel != nil:
		dl.Curve(*corner.Bevel, tol, PreviewColor, 1)
	}
}
