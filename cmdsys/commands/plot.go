package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/geom"
	"cad-lite/render"
)

// plotCmd opens the host's plot dialog. When the dialog asked for a plot window, it instead asks
// for two corners, as AutoCAD's Window< button does, and hands the box back to the dialog.
type plotCmd struct {
	base
	picking bool
	first   *geom.Vec2
}

func (*plotCmd) Name() string { return "PLOT" }

// HumanOnly keeps agents from printing or writing files.
func (*plotCmd) HumanOnly() bool { return true }

func (c *plotCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !ctx.HasPlot() {
		ctx.Say("cmd.plot.msg.unavailable")
		return nil
	}
	if !ctx.PlotPicking() {
		ctx.ShowPlot(nil)
		return nil
	}
	c.picking = true
	return &cmdsys.Prompt{Message: ctx.T("cmd.plot.prompt.first_corner"), Accepts: cmdsys.InputPoint}
}

func (c *plotCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind != cmdsys.InputPoint {
		return c.prompt(ctx)
	}
	if c.first == nil {
		c.first = pt(in.Point)
		return c.prompt(ctx)
	}
	box := geom.BoxOf(*c.first, in.Point)
	c.picking = false
	ctx.ShowPlot(&box)
	return nil
}

func (c *plotCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.first == nil {
		return &cmdsys.Prompt{Message: ctx.T("cmd.plot.prompt.first_corner"), Accepts: cmdsys.InputPoint}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.plot.prompt.other_corner"), Accepts: cmdsys.InputPoint, Base: c.first}
}

// Preview draws the window from the first corner to the cursor.
func (c *plotCmd) Preview(ctx *cmdsys.Context, cursor geom.Vec2, dl *render.DisplayList) {
	if c.first != nil {
		dl.Polyline(rectPts(*c.first, cursor), PreviewColor, 1, true)
	}
}

// Cancel brings the dialog back unchanged when a window pick is abandoned.
func (c *plotCmd) Cancel(ctx *cmdsys.Context) {
	if c.picking {
		c.picking = false
		ctx.ShowPlot(nil)
	}
}

func registerPlot(r *cmdsys.Registry) {
	r.Register("PLOT", func() cmdsys.Command { return &plotCmd{} })
	r.Alias("PRINT", "PLOT")
}
