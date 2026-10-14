package commands

import (
	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// This file holds the helpers every command package shares. It is frozen after the seam split:
// a package that needs a new shared helper adds it to its own file and says so in the hand-off.

// PreviewColor is the rubber-band colour of command previews.
var PreviewColor = render.Color{R: 120, G: 200, B: 255, A: 255}

func pt(p geom.Vec2) *geom.Vec2 { return &p }

func previewTol(ctx *cmdsys.Context) float64 { return ctx.View.PixelSize() / 4 }

// base embeds the no-op parts of cmdsys.Command.
type base struct{}

func (base) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (base) Cancel(*cmdsys.Context)                                  {}

func rectPts(a, b geom.Vec2) []geom.Vec2 {
	return []geom.Vec2{a, {X: b.X, Y: a.Y}, b, {X: a.X, Y: b.Y}}
}

// selecting is the "Select objects:" phase shared by the modify commands:
// a non-empty pre-selection is used as is (noun-verb editing), otherwise
// the command asks for one.
type selecting struct {
	hs []doc.Handle
}

func selectPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.common.prompt.select_objects"), Accepts: cmdsys.InputSelection}
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

// take accepts a selection response; it returns false when it was empty. The "N found" count is
// printed by the selection prompt as each pick is added, so it is not repeated here.
func (s *selecting) take(ctx *cmdsys.Context, in cmdsys.Input) bool {
	s.hs = in.Selection
	ctx.Selection.Clear()
	if len(s.hs) == 0 {
		return false
	}
	ctx.SetPrevious(s.hs)
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

// yesNo renders a boolean as the session's Yes or No.
func yesNo(ctx *cmdsys.Context, b bool) string {
	if b {
		return ctx.T("common.yes")
	}
	return ctx.T("common.no")
}

// immediate is a command that runs entirely in Start.
type immediate struct {
	base
	name string
	run  func(ctx *cmdsys.Context)
	// humanOnly refuses the command to AI agents (cmdsys.HumanOnly).
	humanOnly bool
}

func (c *immediate) HumanOnly() bool { return c.humanOnly }

func (c *immediate) Name() string { return c.name }

func (c *immediate) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.run(ctx)
	return nil
}

func (c *immediate) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt { return nil }
