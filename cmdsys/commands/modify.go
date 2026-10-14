package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// copyOf returns a clone of e for a new entity: every property is kept (layer, colour and
// whatever the document gains later) except the handle and owner, which Txn.Add assigns.
func copyOf(e doc.Entity) doc.Entity {
	cp := e.Clone().(doc.Entity)
	c := cp.Common()
	c.H, c.Owner = 0, 0
	return cp
}

// addCopy adds a copy of entity h moved by m to the open transaction and returns its handle, or 0
// when h no longer exists.
func addCopy(ctx *cmdsys.Context, h doc.Handle, m geom.Mat3) doc.Handle {
	e := ctx.Doc.Entity(h)
	if e == nil {
		return 0
	}
	cp := copyOf(e)
	cp.Transform(m)
	return ctx.Tx().Add(cp)
}

// defaultKeyword is a prompt default that is a keyword (<Displacement>, <Exit>).
func defaultKeyword(id string) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: id}
}

// defaultText is a prompt default shown as plain text: "<use first point as displacement>", "<Y>".
// Pressing Enter answers with an Input of kind InputText.
func defaultText(s string) *cmdsys.Input { return &cmdsys.Input{Kind: cmdsys.InputText, Text: s} }

// noteClick records the point a click answered an angle or distance prompt with as LASTPOINT. A
// typed number carries no point, so a zero Point is left alone.
func noteClick(ctx *cmdsys.Context, in cmdsys.Input) {
	if in.Kind != cmdsys.InputPoint && in.Point != (geom.Vec2{}) {
		ctx.SetLastPoint(in.Point)
	}
}

// --- ERASE --------------------------------------------------------------

type eraseCmd struct {
	base
	selecting
}

func (*eraseCmd) Name() string { return "ERASE" }

func (c *eraseCmd) erase(ctx *cmdsys.Context) *cmdsys.Prompt {
	rememberErased(ctx, c.hs)
	for _, h := range c.hs {
		ctx.Tx().Erase(h)
	}
	return nil
}

func (c *eraseCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.erase(ctx)
	}
	return selectPrompt(ctx)
}

func (c *eraseCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if !c.take(ctx, in) {
		return nil
	}
	return c.erase(ctx)
}

// --- MOVE and COPY ------------------------------------------------------

type moveStage int

const (
	mvSelect moveStage = iota
	mvBase
	mvDisplacement
	mvMode
	mvSecond
	mvArrayCount
	mvArrayEnd
)

type moveCmd struct {
	base
	selecting
	copy  bool
	stage moveStage
	from  *geom.Vec2
	// placed lists the copies each COPY placement made, so Undo can take the last one back.
	placed [][]doc.Handle
	// items and fit are the Array option's count and its Fit toggle.
	items int
	fit   bool
}

func (c *moveCmd) Name() string {
	if c.copy {
		return "COPY"
	}
	return "MOVE"
}

func (c *moveCmd) single(ctx *cmdsys.Context) bool {
	return c.copy && ctx.Vars().Int("COPYMODE") == 1
}

func (c *moveCmd) basePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvBase
	p := &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.base_point"),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Displacement"},
		Default: defaultKeyword("Displacement")}
	if c.copy {
		p.Keywords = append(p.Keywords, "mOde")
	}
	return p
}

func (c *moveCmd) displacementPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvDisplacement
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.displacement"),
		Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func (c *moveCmd) modePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvMode
	def := "Multiple"
	if c.single(ctx) {
		def = kwSingleMode
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.copy_mode"), Accepts: cmdsys.InputKeyword,
		Keywords: []string{kwSingleMode, "Multiple"}, Default: defaultKeyword(def)}
}

func (c *moveCmd) secondPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvSecond
	p := &cmdsys.Prompt{Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: c.from}
	switch {
	case !c.copy:
		p.Message = ctx.T("cmd.modify.prompt.second_point_or")
		p.Default = defaultText(ctx.T("cmd.modify.prompt.use_first"))
	case len(c.placed) == 0:
		p.Message = ctx.T("cmd.modify.prompt.second_point")
		p.Accepts |= cmdsys.InputKeyword
		p.Keywords = []string{"Array"}
		p.Default = defaultText(ctx.T("cmd.modify.prompt.use_first"))
	default:
		p.Message = ctx.T("cmd.modify.prompt.second_point")
		p.Accepts |= cmdsys.InputKeyword
		p.Keywords = []string{"Array", "Exit", "Undo"}
		p.Default = defaultKeyword("Exit")
	}
	return p
}

func (c *moveCmd) arrayCountPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvArrayCount
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.array_items"), Accepts: cmdsys.InputInteger,
		Default: &cmdsys.Input{Kind: cmdsys.InputInteger, Value: 2}}
}

func (c *moveCmd) arrayEndPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = mvArrayEnd
	p := &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.second_point"),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Fit"}, Base: c.from}
	if c.fit {
		p.Keywords = []string{"Array"}
	}
	return p
}

func (c *moveCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.basePrompt(ctx)
	}
	return selectPrompt(ctx)
}

// place moves the selection by v, or adds copies of it moved by v, and returns the new handles.
func (c *moveCmd) place(ctx *cmdsys.Context, v geom.Vec2) []doc.Handle {
	m := geom.Translate(v)
	var added []doc.Handle
	for _, h := range c.hs {
		if c.copy {
			if n := addCopy(ctx, h, m); n != 0 {
				added = append(added, n)
			}
			continue
		}
		ctx.Tx().Modify(h, func(e doc.Entity) { e.Transform(m) })
	}
	return added
}

// next is the prompt after a COPY placement: COPYMODE Single ends the command.
func (c *moveCmd) next(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !c.copy || c.single(ctx) {
		return nil
	}
	return c.secondPrompt(ctx)
}

func (c *moveCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case mvSelect:
		if !c.take(ctx, in) {
			return nil
		}
		return c.basePrompt(ctx)
	case mvBase:
		switch {
		case in.Kind == cmdsys.InputKeyword && in.Keyword == "Displacement":
			return c.displacementPrompt(ctx)
		case in.Kind == cmdsys.InputKeyword:
			return c.modePrompt(ctx)
		}
		c.from = pt(in.Point)
		return c.secondPrompt(ctx)
	case mvMode:
		v := 0
		if in.Keyword == kwSingleMode {
			v = 1
		}
		_ = ctx.Vars().Set("COPYMODE", v)
		return c.basePrompt(ctx)
	case mvDisplacement:
		v := geom.Vec2{}
		if in.Kind == cmdsys.InputPoint {
			v = in.Point
		}
		c.place(ctx, v)
		return nil
	case mvSecond:
		return c.second(ctx, in)
	case mvArrayCount:
		if in.Value < 2 || in.Value > 32767 {
			ctx.Say("cmd.array.msg.count")
			return c.arrayCountPrompt(ctx)
		}
		c.items = int(in.Value)
		return c.arrayEndPrompt(ctx)
	}
	if in.Kind == cmdsys.InputKeyword {
		c.fit = !c.fit
		return c.arrayEndPrompt(ctx)
	}
	step := in.Point.Sub(*c.from)
	if c.fit {
		step = step.Scale(1 / float64(c.items-1))
	}
	for i := 1; i < c.items; i++ {
		c.place(ctx, step.Scale(float64(i)))
	}
	return nil
}

func (c *moveCmd) second(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputKeyword && in.Keyword == "Array":
		return c.arrayCountPrompt(ctx)
	case in.Kind == cmdsys.InputKeyword && in.Keyword == "Undo":
		if n := len(c.placed); n > 0 {
			for _, h := range c.placed[n-1] {
				ctx.Tx().Erase(h)
			}
			c.placed = c.placed[:n-1]
		}
		return c.secondPrompt(ctx)
	case in.Kind == cmdsys.InputKeyword, in.Kind == cmdsys.InputEnter && len(c.placed) > 0:
		return nil // Exit
	case in.Kind == cmdsys.InputEnter:
		// The base point is the displacement.
		c.placed = append(c.placed, c.place(ctx, *c.from))
		return nil
	}
	c.placed = append(c.placed, c.place(ctx, in.Point.Sub(*c.from)))
	return c.next(ctx)
}

func (c *moveCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.from == nil || (c.stage != mvSecond && c.stage != mvArrayEnd) {
		return
	}
	dl.Polyline([]geom.Vec2{*c.from, cur}, PreviewColor.WithAlpha(120), 0, false)
	if c.stage == mvSecond {
		c.previewTransformed(ctx, geom.Translate(cur.Sub(*c.from)), dl)
		return
	}
	step := cur.Sub(*c.from)
	if c.fit {
		step = step.Scale(1 / float64(c.items-1))
	}
	for i := 1; i < c.items; i++ {
		c.previewTransformed(ctx, geom.Translate(step.Scale(float64(i))), dl)
	}
}

// --- ROTATE and SCALE ---------------------------------------------------

type xformStage int

const (
	xfSelect xformStage = iota
	xfBase
	xfValue
	xfRef
	xfRefEnd
	xfNew
	xfNewFirst
	xfNewEnd
)

type transformCmd struct {
	base
	selecting
	scale  bool
	stage  xformStage
	center *geom.Vec2
	// copy keeps the originals; ref is the reference angle or length once given.
	copy   bool
	ref    float64
	hasRef bool
	// first is the first point of a two-point reference or new value.
	first *geom.Vec2
}

func (c *transformCmd) Name() string {
	if c.scale {
		return "SCALE"
	}
	return "ROTATE"
}

func (c *transformCmd) basePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = xfBase
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.base_point"), Accepts: cmdsys.InputPoint}
}

func (c *transformCmd) valuePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = xfValue
	kws := []string{"Copy", "Reference"}
	if c.scale {
		return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.scale_factor"),
			Accepts: cmdsys.InputDistance | cmdsys.InputKeyword, Keywords: kws, Base: c.center,
			Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: 1}}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.rotation_angle"),
		Accepts: cmdsys.InputAngle | cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: kws, Base: c.center,
		Default: &cmdsys.Input{Kind: cmdsys.InputAngle}}
}

func (c *transformCmd) refPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = xfRef
	if c.scale {
		return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.scale_ref"),
			Accepts: cmdsys.InputDistance | cmdsys.InputPoint, Base: c.center,
			Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: 1}}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.rotate_ref"),
		Accepts: cmdsys.InputAngle | cmdsys.InputPoint, Base: c.center,
		Default: &cmdsys.Input{Kind: cmdsys.InputAngle}}
}

func (c *transformCmd) secondPointPrompt(ctx *cmdsys.Context, stage xformStage) *cmdsys.Prompt {
	c.stage = stage
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.second_point"), Accepts: cmdsys.InputPoint, Base: c.first}
}

func (c *transformCmd) newPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = xfNew
	p := &cmdsys.Prompt{Accepts: cmdsys.InputKeyword, Keywords: []string{"Points"}, Base: c.center}
	if c.scale {
		p.Message = ctx.T("cmd.modify.prompt.scale_new")
		p.Accepts |= cmdsys.InputDistance
		p.Default = &cmdsys.Input{Kind: cmdsys.InputDistance, Value: 1}
	} else {
		p.Message = ctx.T("cmd.modify.prompt.rotate_new")
		p.Accepts |= cmdsys.InputAngle
		p.Default = &cmdsys.Input{Kind: cmdsys.InputAngle}
	}
	return p
}

func (c *transformCmd) firstPointPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = xfNewFirst
	return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.first_point"), Accepts: cmdsys.InputPoint}
}

// matrix is the transform for the angle (rotate) or factor (scale) v.
func (c *transformCmd) matrix(v float64) geom.Mat3 {
	if c.scale {
		return geom.ScaleAbout(*c.center, v)
	}
	return geom.Rotate(*c.center, v)
}

// measure is the angle or length a point sets: the direction or distance from the base point.
func (c *transformCmd) measure(p geom.Vec2) float64 {
	if c.scale {
		return p.Dist(*c.center)
	}
	return p.Sub(*c.center).Angle()
}

// between is the angle or length of the line from a to b.
func (c *transformCmd) between(a, b geom.Vec2) float64 {
	if c.scale {
		return a.Dist(b)
	}
	return b.Sub(a).Angle()
}

// apply transforms (or copies) the selection by the angle or the new/reference factor v.
func (c *transformCmd) apply(ctx *cmdsys.Context, v float64) *cmdsys.Prompt {
	if c.scale && !(v > 0) {
		ctx.Say("cmd.common.msg.positive")
		if c.hasRef {
			return c.newPrompt(ctx)
		}
		return c.valuePrompt(ctx)
	}
	m := c.matrix(v)
	for _, h := range c.hs {
		if c.copy {
			addCopy(ctx, h, m)
			continue
		}
		ctx.Tx().Modify(h, func(e doc.Entity) { e.Transform(m) })
	}
	return nil
}

func (c *transformCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !c.scale {
		ctx.Say("cmd.modify.msg.rotate_header")
	}
	if c.begin(ctx) {
		return c.basePrompt(ctx)
	}
	return selectPrompt(ctx)
}

// value reads the angle or distance a value prompt was answered with; a point answers by its
// direction or distance from the base point.
func (c *transformCmd) value(ctx *cmdsys.Context, in cmdsys.Input) float64 {
	noteClick(ctx, in)
	if in.Kind == cmdsys.InputPoint {
		return c.measure(in.Point)
	}
	return in.Value
}

func (c *transformCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case xfSelect:
		if !c.take(ctx, in) {
			return nil
		}
		return c.basePrompt(ctx)
	case xfBase:
		c.center = pt(in.Point)
		return c.valuePrompt(ctx)
	case xfValue:
		switch {
		case in.Kind == cmdsys.InputKeyword && in.Keyword == "Copy":
			c.copy = true
			if c.scale {
				ctx.Say("cmd.modify.msg.scale_copy")
			} else {
				ctx.Say("cmd.modify.msg.rotate_copy")
			}
			return c.valuePrompt(ctx)
		case in.Kind == cmdsys.InputKeyword:
			return c.refPrompt(ctx)
		}
		return c.apply(ctx, c.value(ctx, in))
	case xfRef:
		if in.Kind == cmdsys.InputPoint {
			c.first = pt(in.Point)
			return c.secondPointPrompt(ctx, xfRefEnd)
		}
		return c.setRef(ctx, in.Value)
	case xfRefEnd:
		return c.setRef(ctx, c.between(*c.first, in.Point))
	case xfNew:
		switch {
		case in.Kind == cmdsys.InputKeyword:
			return c.firstPointPrompt(ctx)
		}
		return c.finish(ctx, c.value(ctx, in))
	case xfNewFirst:
		c.first = pt(in.Point)
		return c.secondPointPrompt(ctx, xfNewEnd)
	}
	return c.finish(ctx, c.between(*c.first, in.Point))
}

// setRef stores the reference angle or length and asks for the new one.
func (c *transformCmd) setRef(ctx *cmdsys.Context, v float64) *cmdsys.Prompt {
	if c.scale && !(v > 0) {
		ctx.Say("cmd.common.msg.positive")
		return c.refPrompt(ctx)
	}
	c.ref, c.hasRef = v, true
	return c.newPrompt(ctx)
}

// finish applies the new value against the reference: the rotation is new - reference, the scale
// factor new / reference.
func (c *transformCmd) finish(ctx *cmdsys.Context, nv float64) *cmdsys.Prompt {
	if c.scale {
		return c.apply(ctx, nv/c.ref)
	}
	return c.apply(ctx, nv-c.ref)
}

func (c *transformCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.center == nil || (c.stage != xfValue && c.stage != xfNew) {
		return
	}
	v := c.measure(cur)
	if c.stage == xfNew {
		if c.scale {
			v /= c.ref
		} else {
			v -= c.ref
		}
	}
	if c.scale {
		v = math.Max(v, 1e-9)
	}
	dl.Polyline([]geom.Vec2{*c.center, cur}, PreviewColor.WithAlpha(120), 0, false)
	c.previewTransformed(ctx, c.matrix(v), dl)
}

// registerModify adds the transform commands, and the array and OOPS commands of array.go and
// oops.go.
func registerModify(r *cmdsys.Registry) {
	r.Register("ERASE", func() cmdsys.Command { return &eraseCmd{} })
	r.Register("MOVE", func() cmdsys.Command { return &moveCmd{} })
	r.Register("COPY", func() cmdsys.Command { return &moveCmd{copy: true} })
	r.Register("ROTATE", func() cmdsys.Command { return &transformCmd{} })
	r.Register("SCALE", func() cmdsys.Command { return &transformCmd{scale: true} })
	registerArray(r)
	registerOops(r)
}
