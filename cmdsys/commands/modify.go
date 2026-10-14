package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// selecting is the "Select objects:" phase shared by the modify commands:
// a non-empty pre-selection is used as is (noun-verb editing), otherwise
// the command asks for one.
type selecting struct {
	hs []doc.Handle
}

func selectPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select objects", Accepts: cmdsys.InputSelection}
}

// begin takes the pre-selection; it returns false when the command must
// prompt for objects.
func (s *selecting) begin(ctx *cmdsys.Context) bool {
	if ctx.Selection.Len() == 0 {
		return false
	}
	s.hs = append([]doc.Handle(nil), ctx.Selection.Handles()...)
	ctx.Selection.Clear()
	return true
}

// take accepts a selection response; it returns false when it was empty.
func (s *selecting) take(ctx *cmdsys.Context, in cmdsys.Input) bool {
	s.hs = in.Selection
	ctx.Selection.Clear()
	if len(s.hs) == 0 {
		return false
	}
	ctx.Printf("%d found", len(s.hs))
	return true
}

// previewTransformed draws the selected entities under m.
func (s *selecting) previewTransformed(ctx *cmdsys.Context, m geom.Mat3, dl *render.DisplayList) {
	tol := previewTol(ctx)
	for _, h := range s.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		for _, c := range e.Geometry() {
			dl.Curve(c.Transform(m), tol, PreviewColor, 1)
		}
	}
}

// --- ERASE --------------------------------------------------------------

type eraseCmd struct {
	base
	selecting
}

func (*eraseCmd) Name() string { return "ERASE" }

func (c *eraseCmd) erase(ctx *cmdsys.Context) *cmdsys.Prompt {
	for _, h := range c.hs {
		ctx.Tx().Erase(h)
	}
	return nil
}

func (c *eraseCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.erase(ctx)
	}
	return selectPrompt()
}

func (c *eraseCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if !c.take(ctx, in) {
		return nil
	}
	return c.erase(ctx)
}

// --- MOVE and COPY ------------------------------------------------------

type moveCmd struct {
	base
	selecting
	copy bool
	from *geom.Vec2
}

func (c *moveCmd) Name() string {
	if c.copy {
		return "COPY"
	}
	return "MOVE"
}

func (c *moveCmd) basePrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify base point", Accepts: cmdsys.InputPoint}
}

func (c *moveCmd) secondPrompt() *cmdsys.Prompt {
	p := &cmdsys.Prompt{Message: "Specify second point", Accepts: cmdsys.InputPoint, Base: c.from}
	if c.copy {
		p.Accepts |= cmdsys.InputEnter
	}
	return p
}

func (c *moveCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.basePrompt()
	}
	return selectPrompt()
}

func (c *moveCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.hs == nil:
		if !c.take(ctx, in) {
			return nil
		}
		return c.basePrompt()
	case c.from == nil:
		c.from = pt(in.Point)
		return c.secondPrompt()
	case in.Kind == cmdsys.InputEnter:
		return nil
	}
	m := geom.Translate(in.Point.Sub(*c.from))
	tx := ctx.Tx()
	for _, h := range c.hs {
		if c.copy {
			if e := ctx.Doc.Entity(h); e != nil {
				cp := e.Clone().(doc.Entity)
				cp.Transform(m)
				layer, color := cp.Common().Layer, cp.Common().Color
				*cp.Common() = doc.EntityCommon{Layer: layer, Color: color}
				tx.Add(cp)
			}
			continue
		}
		tx.Modify(h, func(e doc.Entity) { e.Transform(m) })
	}
	if c.copy {
		return c.secondPrompt()
	}
	return nil
}

func (c *moveCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.from == nil {
		return
	}
	dl.Polyline([]geom.Vec2{*c.from, cur}, PreviewColor.WithAlpha(120), 0, false)
	c.previewTransformed(ctx, geom.Translate(cur.Sub(*c.from)), dl)
}

// --- ROTATE and SCALE ---------------------------------------------------

type transformCmd struct {
	base
	selecting
	scale  bool
	center *geom.Vec2
}

func (c *transformCmd) Name() string {
	if c.scale {
		return "SCALE"
	}
	return "ROTATE"
}

func (c *transformCmd) basePrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify base point", Accepts: cmdsys.InputPoint}
}

func (c *transformCmd) valuePrompt() *cmdsys.Prompt {
	if c.scale {
		return &cmdsys.Prompt{Message: "Specify scale factor", Accepts: cmdsys.InputDistance, Base: c.center}
	}
	return &cmdsys.Prompt{Message: "Specify rotation angle", Accepts: cmdsys.InputAngle, Base: c.center}
}

func (c *transformCmd) matrix(v float64) geom.Mat3 {
	if c.scale {
		return geom.ScaleAbout(*c.center, v)
	}
	return geom.Rotate(*c.center, v)
}

func (c *transformCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.basePrompt()
	}
	return selectPrompt()
}

func (c *transformCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.hs == nil:
		if !c.take(ctx, in) {
			return nil
		}
		return c.basePrompt()
	case c.center == nil:
		c.center = pt(in.Point)
		return c.valuePrompt()
	}
	if c.scale && in.Value <= 0 {
		ctx.Printf("Value must be positive and nonzero.")
		return c.valuePrompt()
	}
	m := c.matrix(in.Value)
	for _, h := range c.hs {
		ctx.Tx().Modify(h, func(e doc.Entity) { e.Transform(m) })
	}
	return nil
}

func (c *transformCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.center == nil {
		return
	}
	v := cur.Sub(*c.center).Angle()
	if c.scale {
		v = math.Max(cur.Dist(*c.center), 1e-9)
	}
	dl.Polyline([]geom.Vec2{*c.center, cur}, PreviewColor.WithAlpha(120), 0, false)
	c.previewTransformed(ctx, c.matrix(v), dl)
}
