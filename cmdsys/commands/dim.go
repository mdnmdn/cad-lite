package commands

import (
	"math"
	"unicode/utf8"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// The dimension commands build each new DIMENSION's style from AutoCAD's dimension variables,
// with the ISO-25 defaults of acadiso.dwt (doc.DefaultDimStyle): DIMSCALE, DIMASZ, DIMTXT,
// DIMEXO, DIMEXE, DIMGAP, DIMDEC, DIMDSEP (its first character), DIMZIN, DIMTAD (only zero
// against non-zero matters: on the line or above it), DIMARCSYM, and DIMSTYLE, the read-only
// style name. DRAW_DIMLAST is command memory: the handle of the last dimension a dimension
// command made, which DIMCONTINUE continues.
func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		st := doc.DefaultDimStyle()
		v.Register(cmdsys.Var{Name: "DIMSCALE", Kind: cmdsys.VarReal, Default: st.Scale})
		v.Register(cmdsys.Var{Name: "DIMASZ", Kind: cmdsys.VarReal, Default: st.ArrowSize})
		v.Register(cmdsys.Var{Name: "DIMTXT", Kind: cmdsys.VarReal, Default: st.TextHeight})
		v.Register(cmdsys.Var{Name: "DIMEXO", Kind: cmdsys.VarReal, Default: st.ExtOffset})
		v.Register(cmdsys.Var{Name: "DIMEXE", Kind: cmdsys.VarReal, Default: st.ExtExtend})
		v.Register(cmdsys.Var{Name: "DIMGAP", Kind: cmdsys.VarReal, Default: st.Gap})
		v.Register(cmdsys.Var{Name: "DIMDEC", Kind: cmdsys.VarInt, Default: st.Decimals, Min: 0, Max: 8})
		v.Register(cmdsys.Var{Name: "DIMDSEP", Kind: cmdsys.VarText, Default: st.DecSep})
		v.Register(cmdsys.Var{Name: "DIMZIN", Kind: cmdsys.VarInt, Default: st.ZeroSupp, Min: 0, Max: 15})
		v.Register(cmdsys.Var{Name: "DIMTAD", Kind: cmdsys.VarInt, Default: 1, Min: 0, Max: 4})
		v.Register(cmdsys.Var{Name: "DIMARCSYM", Kind: cmdsys.VarInt, Default: doc.DimArcSymBefore, Min: 0, Max: 2})
		v.Register(cmdsys.Var{Name: "DIMSTYLE", Kind: cmdsys.VarText, Default: st.Name, ReadOnly: true})
		v.Register(cmdsys.Var{Name: dimLastVar, Kind: cmdsys.VarInt, Default: 0})
	})
}

const dimLastVar = "DRAW_DIMLAST"

func registerDim(r *cmdsys.Registry) {
	r.Register("DIMLINEAR", func() cmdsys.Command { return &linearDimCmd{} })
	r.Register("DIMALIGNED", func() cmdsys.Command { return &linearDimCmd{aligned: true} })
	r.Register("DIMCONTINUE", func() cmdsys.Command { return &dimContinueCmd{} })
	r.Register("DIMARC", func() cmdsys.Command { return &dimArcCmd{} })
	r.Alias("DLI", "DIMLINEAR")
	r.Alias("DAL", "DIMALIGNED")
	r.Alias("DCO", "DIMCONTINUE")
	r.Alias("DAR", "DIMARC")
}

// Keywords of the dimension line location prompts, in AutoCAD's order.
var (
	dimLinearKeywords  = []string{"Mtext", "Text", "Angle", "Horizontal", "Vertical", "Rotated"}
	dimAlignedKeywords = []string{"Mtext", "Text", "Angle"}
)

// dimStyle returns the style a new dimension gets from the DIM* variables.
func dimStyle(ctx *cmdsys.Context) doc.DimStyle {
	v := ctx.Vars()
	sep := v.Text("DIMDSEP")
	if r, n := utf8.DecodeRuneInString(sep); n > 0 && r != utf8.RuneError {
		sep = string(r)
	}
	return doc.DimStyle{
		Name: v.Text("DIMSTYLE"), Scale: v.Real("DIMSCALE"), ArrowSize: v.Real("DIMASZ"), TextHeight: v.Real("DIMTXT"),
		ExtOffset: v.Real("DIMEXO"), ExtExtend: v.Real("DIMEXE"), Gap: v.Real("DIMGAP"), Decimals: v.Int("DIMDEC"),
		DecSep: sep, ZeroSupp: v.Int("DIMZIN"), TextAbove: v.Int("DIMTAD") != 0,
	}
}

// measuredText is the measurement of d formatted by its style, without its text override: what
// "Dimension text =" echoes and the Text option offers.
func measuredText(d *doc.Dimension) string {
	m := *d
	m.Text = ""
	return m.MeasurementText()
}

// addDim adds d to the command's transaction, makes it the dimension DIMCONTINUE continues and
// echoes its measurement.
func addDim(ctx *cmdsys.Context, d *doc.Dimension) doc.Handle {
	h := ctx.Tx().Add(d)
	_ = ctx.Vars().Set(dimLastVar, int(h))
	ctx.Say("cmd.dim.msg.text", measuredText(d))
	return h
}

// previewDim draws d as the rubber band.
func previewDim(ctx *cmdsys.Context, d *doc.Dimension, dl *render.DisplayList) {
	tol := previewTol(ctx)
	for _, c := range d.Geometry() {
		dl.Curve(c, tol, PreviewColor, 1)
	}
}

// dimTextPrompt is the Text and Mtext option's prompt, offering the measurement; Enter keeps it.
func dimTextPrompt(ctx *cmdsys.Context, measured string) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.text"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Literal: true,
		Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: measured}}
}

// dimTextAnglePrompt is the Angle option's prompt.
func dimTextAnglePrompt(ctx *cmdsys.Context, base geom.Vec2) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.text_angle"), Accepts: cmdsys.InputAngle, Base: pt(base)}
}

// textAnswer returns the text override an answer to dimTextPrompt sets: Enter keeps the
// measurement.
func textAnswer(in cmdsys.Input) string {
	if in.Kind == cmdsys.InputText {
		return in.Text
	}
	return ""
}

// nearestSegment returns the segment of p nearest q.
func nearestSegment(p geom.Polyline, q geom.Vec2) (geom.Curve, bool) {
	var best geom.Curve
	bestD := math.Inf(1)
	for _, s := range p.Segments() {
		if d := geom.Closest(s, q).Dist(q); d < bestD {
			best, bestD = s, d
		}
	}
	return best, best != nil
}

func unitVec(a float64) geom.Vec2 { return geom.V(math.Cos(a), math.Sin(a)) }

// --- DIMLINEAR, DIMALIGNED ----------------------------------------------

type linearDimStep int

const (
	ldFirst     linearDimStep = iota // first extension line origin or <select object>
	ldSelect                         // select object to dimension
	ldSecond                         // second extension line origin
	ldLocation                       // dimension line location or options
	ldText                           // Text, Mtext: the dimension text
	ldTextAngle                      // Angle: the text angle
	ldRotation                       // Rotated: the dimension line angle
)

type linearDimMode int

const (
	ldAuto       linearDimMode = iota // horizontal or vertical by the cursor
	ldHorizontal                      // Horizontal
	ldVertical                        // Vertical
	ldRotated                         // Rotated
)

// linearDimCmd is DIMLINEAR (a rotated dimension: horizontal, vertical or at an angle) and, with
// aligned set, DIMALIGNED (parallel to its origins). It makes one dimension.
type linearDimCmd struct {
	base
	aligned bool
	step    linearDimStep
	p1, p2  geom.Vec2
	// circle is the circle DIMLINEAR picked: its diameter along the dimension line is measured.
	circle *doc.Circle
	mode   linearDimMode
	rot    float64
	text   string
	tAngle float64
	// cur is the last cursor position the preview saw, which decides the automatic
	// horizontal or vertical choice for the measurement the Text option offers.
	cur    geom.Vec2
	hasCur bool
}

func (c *linearDimCmd) Name() string {
	if c.aligned {
		return "DIMALIGNED"
	}
	return "DIMLINEAR"
}

func (c *linearDimCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.prompt(ctx) }

func (c *linearDimCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.step {
	case ldSelect:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.select_object"), Accepts: cmdsys.InputPoint, Pick: true}
	case ldSecond:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.second_origin"), Accepts: cmdsys.InputPoint, Base: pt(c.p1)}
	case ldLocation:
		kws := dimLinearKeywords
		if c.aligned {
			kws = dimAlignedKeywords
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.location"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: kws}
	case ldText:
		loc := c.p2
		if c.hasCur {
			loc = c.cur
		}
		return dimTextPrompt(ctx, measuredText(c.entity(ctx, loc)))
	case ldTextAngle:
		return dimTextAnglePrompt(ctx, c.p1)
	case ldRotation:
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.line_angle"), Accepts: cmdsys.InputAngle, Base: pt(c.p1),
			Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: c.rot}}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.first_origin"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func (c *linearDimCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case ldFirst:
		if in.Kind == cmdsys.InputEnter {
			c.step = ldSelect
		} else {
			c.p1, c.step = in.Point, ldSecond
		}
	case ldSelect:
		if c.pickObject(ctx, in.Point) {
			c.step = ldLocation
		}
	case ldSecond:
		c.p2, c.step = in.Point, ldLocation
	case ldLocation:
		switch in.Keyword {
		case "Mtext", "Text":
			c.step = ldText
		case "Angle":
			c.step = ldTextAngle
		case "Horizontal":
			c.mode = ldHorizontal
		case "Vertical":
			c.mode = ldVertical
		case "Rotated":
			c.step = ldRotation
		case "":
			addDim(ctx, c.entity(ctx, in.Point))
			return nil
		}
	case ldText:
		c.text, c.step = textAnswer(in), ldLocation
	case ldTextAngle:
		c.tAngle, c.step = in.Value, ldLocation
	case ldRotation:
		c.rot, c.mode, c.step = in.Value, ldRotated, ldLocation
	}
	return c.prompt(ctx)
}

// pickObject takes the origins from the object at p: the ends of a line, an arc or the picked
// polyline segment, or a circle's diameter. It says why and returns false otherwise.
func (c *linearDimCmd) pickObject(ctx *cmdsys.Context, p geom.Vec2) bool {
	e, ok := pickAt(ctx, p)
	if !ok {
		ctx.Say("cmd.common.msg.no_object_found")
		return false
	}
	switch e := e.(type) {
	case *doc.Line:
		c.p1, c.p2 = e.Start, e.End
		return true
	case *doc.Arc:
		a := e.Geometry()[0].(geom.Arc)
		c.p1, c.p2 = a.StartPoint(), a.EndPoint()
		return true
	case *doc.Circle:
		if !c.aligned {
			c.circle = e
			c.p1, c.p2 = e.Center.Sub(geom.V(e.Radius, 0)), e.Center.Add(geom.V(e.Radius, 0))
			return true
		}
		// DIMALIGNED measures the diameter through the pick.
		u := p.Sub(e.Center).Norm()
		if u == (geom.Vec2{}) {
			u = geom.V(1, 0)
		}
		c.p1, c.p2 = e.Center.Add(u.Scale(e.Radius)), e.Center.Sub(u.Scale(e.Radius))
		return true
	case *doc.LWPolyline:
		if pl, ok := e.Geometry()[0].(geom.Polyline); ok {
			switch s, _ := nearestSegment(pl, p); s := s.(type) {
			case geom.Line:
				c.p1, c.p2 = s.A, s.B
				return true
			case geom.Arc:
				c.p1, c.p2 = s.StartPoint(), s.EndPoint()
				return true
			}
		}
	}
	ctx.Say("cmd.dim.msg.not_measurable")
	return false
}

// rotation returns the dimension line angle of DIMLINEAR with the cursor at loc. Unlocked, it is
// horizontal when loc is above or below the box of the origins and vertical when it is left or
// right of it; diagonally off a corner the larger offset decides, and inside the box the box's
// longer side.
func (c *linearDimCmd) rotation(loc geom.Vec2) float64 {
	switch c.mode {
	case ldHorizontal:
		return 0
	case ldVertical:
		return math.Pi / 2
	case ldRotated:
		return c.rot
	}
	box := geom.BoxOf(c.p1, c.p2)
	if c.circle != nil {
		r := geom.V(c.circle.Radius, c.circle.Radius)
		box = geom.BoxOf(c.circle.Center.Sub(r), c.circle.Center.Add(r))
	}
	outside := func(v, lo, hi float64) float64 { return max(lo-v, v-hi, 0) }
	dx, dy := outside(loc.X, box.Min.X, box.Max.X), outside(loc.Y, box.Min.Y, box.Max.Y)
	switch {
	case dx > dy:
		return math.Pi / 2
	case dy > dx:
		return 0
	case box.Max.Y-box.Min.Y > box.Max.X-box.Min.X:
		return math.Pi / 2
	}
	return 0
}

// entity returns the dimension the command makes with its line through loc.
func (c *linearDimCmd) entity(ctx *cmdsys.Context, loc geom.Vec2) *doc.Dimension {
	d := &doc.Dimension{Kind: doc.DimAligned, Ext1: c.p1, Ext2: c.p2, DimLine: loc, Text: c.text, TextAngle: c.tAngle, Style: dimStyle(ctx)}
	if !c.aligned {
		d.Kind, d.Rotation = doc.DimRotated, geom.NormAngle(c.rotation(loc))
		if c.circle != nil {
			u := unitVec(d.Rotation).Scale(c.circle.Radius)
			d.Ext1, d.Ext2 = c.circle.Center.Sub(u), c.circle.Center.Add(u)
		}
	}
	d.Recompute()
	return d
}

func (c *linearDimCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch c.step {
	case ldSecond:
		dl.Polyline([]geom.Vec2{c.p1, cur}, PreviewColor.WithAlpha(120), 0, false)
	case ldSelect:
		previewHover(ctx, cur, dl, func(e doc.Entity) bool {
			switch e.(type) {
			case *doc.Line, *doc.Arc, *doc.Circle, *doc.LWPolyline:
				return true
			}
			return false
		})
	case ldLocation:
		c.cur, c.hasCur = cur, true
		previewDim(ctx, c.entity(ctx, cur), dl)
	}
}

// --- DIMCONTINUE --------------------------------------------------------

// dimContinueCmd is DIMCONTINUE: a chain of dimensions, each from the second extension line
// origin of the one before, on the same dimension line. A rotated base gives rotated
// dimensions at its angle; an aligned base gives rotated dimensions at its direction.
type dimContinueCmd struct {
	base
	selecting bool
	baseH     doc.Handle
	baseDim   *doc.Dimension
	from      geom.Vec2
	undo      []dimContinueStep
}

// dimContinueStep is one continued dimension and the base it continued, for Undo.
type dimContinueStep struct {
	h, prevH doc.Handle
	prev     *doc.Dimension
	prevFrom geom.Vec2
}

func (*dimContinueCmd) Name() string { return "DIMCONTINUE" }

// continuable reports whether e is a dimension DIMCONTINUE can continue.
func continuable(e doc.Entity) (*doc.Dimension, bool) {
	d, ok := e.(*doc.Dimension)
	if !ok || (d.Kind != doc.DimRotated && d.Kind != doc.DimAligned) {
		return nil, false
	}
	return d, true
}

func (c *dimContinueCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	h := doc.Handle(ctx.Vars().Int(dimLastVar))
	if d, ok := continuable(ctx.Doc.Entity(h)); ok && h != 0 {
		c.baseH, c.baseDim, c.from = h, d, d.Ext2
	} else {
		c.selecting = true
	}
	return c.prompt(ctx)
}

func (c *dimContinueCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.selecting {
		return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.continued"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Pick: true}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.dim.prompt.continue_origin"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword | cmdsys.InputEnter,
		Keywords: []string{"Select", "Undo"}, Default: keywordInput("Select"), Base: pt(c.from)}
}

func (c *dimContinueCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.selecting {
		if in.Kind == cmdsys.InputEnter {
			return nil
		}
		c.pickBase(ctx, in.Point)
		return c.prompt(ctx)
	}
	switch {
	case in.Kind == cmdsys.InputEnter || in.Keyword == "Select":
		c.selecting = true
	case in.Keyword == "Undo":
		if len(c.undo) == 0 {
			ctx.Say("cmd.dim.msg.nothing_to_undo")
			break
		}
		u := c.undo[len(c.undo)-1]
		c.undo = c.undo[:len(c.undo)-1]
		ctx.Tx().Erase(u.h)
		c.baseH, c.baseDim, c.from = u.prevH, u.prev, u.prevFrom
		_ = ctx.Vars().Set(dimLastVar, int(u.prevH))
	case in.Kind == cmdsys.InputPoint:
		d := c.next(ctx, in.Point)
		h := addDim(ctx, d)
		c.undo = append(c.undo, dimContinueStep{h: h, prevH: c.baseH, prev: c.baseDim, prevFrom: c.from})
		c.baseH, c.baseDim, c.from = h, d, d.Ext2
	}
	return c.prompt(ctx)
}

// pickBase makes the dimension at p the base, continuing from its extension line nearest p.
func (c *dimContinueCmd) pickBase(ctx *cmdsys.Context, p geom.Vec2) {
	h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, p, pickAperturePx*ctx.View.PixelSize())
	if !ok {
		ctx.Say("cmd.common.msg.no_object_found")
		return
	}
	d, ok := continuable(ctx.Doc.Entity(h))
	if !ok {
		ctx.Say("cmd.dim.msg.not_linear")
		return
	}
	ext := d.Layout().ExtLines
	c.baseH, c.baseDim, c.from, c.selecting = h, d, d.Ext2, false
	if geom.Closest(ext[0], p).Dist(p) < geom.Closest(ext[1], p).Dist(p) {
		c.from = d.Ext1
	}
}

// next returns the dimension from the current origin to p on the base's dimension line.
func (c *dimContinueCmd) next(ctx *cmdsys.Context, p geom.Vec2) *doc.Dimension {
	b := c.baseDim
	rot := b.Rotation
	if b.Kind != doc.DimRotated {
		rot = geom.NormAngle(b.Direction().Angle())
	}
	d := &doc.Dimension{Kind: doc.DimRotated, Ext1: c.from, Ext2: p, DimLine: b.DimLine, Rotation: rot, Style: b.Style}
	d.Recompute()
	return d
}

func (c *dimContinueCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.selecting {
		previewHover(ctx, cur, dl, func(e doc.Entity) bool { _, ok := continuable(e); return ok })
		return
	}
	previewDim(ctx, c.next(ctx, cur), dl)
}
