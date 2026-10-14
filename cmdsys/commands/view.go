package commands

import (
	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// ZoomMargin is the pixel margin ZOOM Extents leaves around the drawing.
const ZoomMargin = 24

// ZoomExtents fits the view to the drawing's extents.
func ZoomExtents(d *doc.Drawing, v *render.View) { v.Fit(d.Extents(), ZoomMargin) }

// --- ZOOM ---------------------------------------------------------------

type zoomCmd struct {
	base
	corner *geom.Vec2
}

func (*zoomCmd) Name() string { return "ZOOM" }

func (c *zoomCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ext := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Extents"}
	return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.corner"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Extents", "Window"}, Default: &ext}
}

func (c *zoomCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Keyword == "Extents":
		ZoomExtents(ctx.Doc, ctx.View)
		return nil
	case in.Keyword == "Window":
		return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.first_corner"), Accepts: cmdsys.InputPoint}
	case c.corner == nil:
		c.corner = pt(in.Point)
		return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.opposite_corner"), Accepts: cmdsys.InputPoint, Base: c.corner}
	}
	ctx.View.Fit(geom.BoxOf(*c.corner, in.Point), 0)
	return nil
}

func (c *zoomCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.corner != nil {
		dl.Polyline(rectPts(*c.corner, cur), PreviewColor, 0, true)
	}
}

// Transparent marks ZOOM as runnable inside another command (cmdsys.Transparent).
func (*zoomCmd) Transparent() bool { return true }

// registerView adds ZOOM.
func registerView(r *cmdsys.Registry) {
	r.Register("ZOOM", func() cmdsys.Command { return &zoomCmd{} })
}
