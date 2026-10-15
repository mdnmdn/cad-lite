package commands

import (
	"math"
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// ZoomMargin is the pixel margin ZOOM Extents leaves around the drawing.
const ZoomMargin = 24

// ZoomExtents fits the view to the extents of what is displayed: the entities on layers that are
// on and thawed. A drawing with nothing displayed leaves the view as it is.
func ZoomExtents(d *doc.Drawing, v *render.View) { v.Fit(visibleExtents(d), ZoomMargin) }

func visibleExtents(d *doc.Drawing) geom.Box2 {
	b := geom.EmptyBox()
	for _, h := range d.Handles() {
		if e := d.Entity(h); e != nil && d.EntityVisible(e) {
			if _, unknown := e.(*doc.Unknown); !unknown {
				if in, ok := e.(*doc.Insert); ok {
					b = b.Union(in.VisibleBounds())
				} else {
					b = b.Union(e.Bounds())
				}
			}
		}
	}
	return b
}

// changeView applies fn to the view and records the view before it as a previous view (ZOOM
// Previous), so that every view command shares one history.
func changeView(ctx *cmdsys.Context, fn func(v *render.View)) {
	st := ctx.View.History()
	st.Save(*ctx.View)
	fn(ctx.View)
	st.Settle(*ctx.View)
}

// --- ZOOM ---------------------------------------------------------------

// The ZOOM keyword ids; the catalog gives each its display text, shortcut and Italian name.
const (
	zmAll      = "zAll"
	zmCenter   = "Center"
	zmExtents  = "Extents"
	zmPrevious = "Previous"
	zmScale    = "Scale"
	zmWindow   = "Window"
	zmObject   = "Object"
)

type zoomStep int

const (
	zsCorner zoomStep = iota
	zsFirst           // first corner of Window
	zsOpposite
	zsCenter // the centre point of Center
	zsHeight // magnification or height after the centre
	zsScale  // the scale factor of Scale
	zsObject
)

type zoomCmd struct {
	base
	step   zoomStep
	corner *geom.Vec2
	center geom.Vec2
}

func (*zoomCmd) Name() string { return "ZOOM" }

func (c *zoomCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.first(ctx) }

func (c *zoomCmd) first(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step = zsCorner
	ext := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: zmExtents}
	return &cmdsys.Prompt{
		Message:  ctx.T("cmd.zoom.prompt.corner"),
		Accepts:  cmdsys.InputPoint | cmdsys.InputKeyword | cmdsys.InputDistance | cmdsys.InputText,
		Keywords: []string{zmAll, zmCenter, zmExtents, zmPrevious, zmScale, zmWindow, zmObject},
		Default:  &ext,
	}
}

func (c *zoomCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case zsCorner:
		return c.corner0(ctx, in)
	case zsFirst:
		c.corner = pt(in.Point)
		c.step = zsOpposite
		return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.opposite_corner"), Accepts: cmdsys.InputPoint, Base: c.corner}
	case zsOpposite:
		a, b := *c.corner, in.Point
		changeView(ctx, func(v *render.View) { v.Fit(geom.BoxOf(a, b), 0) })
		return nil
	case zsCenter:
		c.center, c.step = in.Point, zsHeight
		h := ctx.View.Height / ctx.View.Scale
		def := cmdsys.Input{Kind: cmdsys.InputDistance, Value: h}
		return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.height"), Accepts: cmdsys.InputDistance | cmdsys.InputText | cmdsys.InputEnter, Default: &def}
	case zsHeight:
		c.centerOn(ctx, in)
		return nil
	case zsScale:
		if !c.scaled(ctx, in) {
			return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.scale"), Accepts: cmdsys.InputDistance | cmdsys.InputText}
		}
		return nil
	case zsObject:
		c.object(ctx, in.Selection)
		return nil
	}
	return nil
}

func (c *zoomCmd) corner0(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch in.Kind {
	case cmdsys.InputKeyword:
		switch in.Keyword {
		case zmExtents, zmAll:
			changeView(ctx, func(v *render.View) { ZoomExtents(ctx.Doc, v) })
		case zmWindow:
			c.step = zsFirst
			return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.first_corner"), Accepts: cmdsys.InputPoint}
		case zmCenter:
			c.step = zsCenter
			return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.center"), Accepts: cmdsys.InputPoint}
		case zmScale:
			c.step = zsScale
			return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.scale"), Accepts: cmdsys.InputDistance | cmdsys.InputText}
		case zmPrevious:
			c.previous(ctx)
		case zmObject:
			c.step = zsObject
			return selectPrompt(ctx)
		}
		return nil
	case cmdsys.InputPoint:
		c.corner, c.step = pt(in.Point), zsOpposite
		return &cmdsys.Prompt{Message: ctx.T("cmd.zoom.prompt.opposite_corner"), Accepts: cmdsys.InputPoint, Base: c.corner}
	}
	if !c.scaled(ctx, in) {
		ctx.Say("cmd.zoom.msg.bad_scale")
		return c.first(ctx)
	}
	return nil
}

func (c *zoomCmd) previous(ctx *cmdsys.Context) {
	st := ctx.View.History()
	prev, ok := st.Pop(*ctx.View)
	if !ok {
		ctx.Say("cmd.zoom.msg.no_previous")
		return
	}
	ctx.View.Restore(prev)
	st.Settle(*ctx.View)
}

// parseZoomScale reads "2", "2X" or "2XP". relative is true for the X and XP forms, which scale
// the current view; a bare number scales the full view.
func parseZoomScale(text string) (factor float64, relative, ok bool) {
	t := strings.ToUpper(strings.TrimSpace(text))
	switch {
	case strings.HasSuffix(t, "XP"):
		t, relative = strings.TrimSuffix(t, "XP"), true
	case strings.HasSuffix(t, "X"):
		t, relative = strings.TrimSuffix(t, "X"), true
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false, false
	}
	return f, relative, true
}

// scaled applies a typed scale factor; it reports false when the answer is not one.
func (c *zoomCmd) scaled(ctx *cmdsys.Context, in cmdsys.Input) bool {
	var f float64
	relative := false
	switch in.Kind {
	case cmdsys.InputDistance:
		f = in.Value
		if f <= 0 {
			return false
		}
	case cmdsys.InputText:
		var ok bool
		if f, relative, ok = parseZoomScale(in.Text); !ok {
			return false
		}
	default:
		return false
	}
	changeView(ctx, func(v *render.View) {
		if !relative {
			// A bare number is relative to the full view: the drawing's extents fitted, or the
			// current view when there is nothing drawn.
			ZoomExtents(ctx.Doc, v)
		}
		v.ZoomAt(v.Width/2, v.Height/2, f)
	})
	return true
}

// centerOn finishes ZOOM Center: a number is the new view height in model units, nX a
// magnification of the current view, Enter keeps the height.
func (c *zoomCmd) centerOn(ctx *cmdsys.Context, in cmdsys.Input) {
	changeView(ctx, func(v *render.View) {
		switch in.Kind {
		case cmdsys.InputDistance:
			if in.Value > 0 {
				v.Scale = math.Max(render.MinScale, math.Min(render.MaxScale, v.Height/in.Value))
			}
		case cmdsys.InputText:
			if f, _, ok := parseZoomScale(in.Text); ok {
				v.Scale = math.Max(render.MinScale, math.Min(render.MaxScale, v.Scale*f))
			}
		}
		v.Center = c.center
	})
}

func (c *zoomCmd) object(ctx *cmdsys.Context, hs []doc.Handle) {
	box := geom.EmptyBox()
	for _, h := range hs {
		if e := ctx.Doc.Entity(h); e != nil {
			if in, ok := e.(*doc.Insert); ok {
				box = box.Union(in.VisibleBounds())
			} else {
				box = box.Union(e.Bounds())
			}
		}
	}
	ctx.Selection.Clear()
	if box.IsEmpty() {
		return
	}
	changeView(ctx, func(v *render.View) {
		if w, h := box.Size(); w < 1e-9 && h < 1e-9 {
			v.Center = box.Center()
			return
		}
		v.Fit(box, ZoomMargin)
	})
}

func (c *zoomCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.corner != nil && c.step == zsOpposite {
		dl.Polyline(rectPts(*c.corner, cur), PreviewColor, 0, true)
	}
}

// Transparent marks ZOOM as runnable inside another command (cmdsys.Transparent).
func (*zoomCmd) Transparent() bool { return true }

// --- PAN ----------------------------------------------------------------

// panCmd is PAN and -PAN: a base point and a second point (or a displacement). The real-time
// pan of AutoCAD has no scriptable form; the mouse drag and the wheel pan the view in the UI.
type panCmd struct {
	base
	name string
	from *geom.Vec2
}

func (c *panCmd) Name() string { return c.name }

func (*panCmd) Transparent() bool { return true }

func (c *panCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.pan.prompt.base"), Accepts: cmdsys.InputPoint}
}

func (c *panCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.from == nil {
		c.from = pt(in.Point)
		return &cmdsys.Prompt{Message: ctx.T("cmd.pan.prompt.second"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: c.from}
	}
	d := in.Point.Sub(*c.from)
	if in.Kind == cmdsys.InputEnter {
		d = *c.from
	}
	changeView(ctx, func(v *render.View) { v.Center = v.Center.Sub(d) })
	return nil
}

// --- REGEN, REDRAW ------------------------------------------------------

// registerView adds ZOOM, PAN, REGEN and REDRAW.
func registerView(r *cmdsys.Registry) {
	r.Register("ZOOM", func() cmdsys.Command { return &zoomCmd{} })
	r.Register("PAN", func() cmdsys.Command { return &panCmd{name: "PAN"} })
	r.Alias("P", "PAN")
	cmdsys.RegisterCLI(r, "PAN", func() cmdsys.Command { return &panCmd{name: "-PAN"} }, "P")
	// The display list is rebuilt from the document on every frame, so there is nothing to
	// regenerate: the four commands exist so scripts and habits keep working, and print nothing.
	for _, n := range []string{"REGEN", "REGENALL", "REDRAW", "REDRAWALL"} {
		name := n
		r.Register(name, func() cmdsys.Command { return &immediate{name: name, run: func(*cmdsys.Context) {}} })
	}
	r.Alias("RE", "REGEN")
	r.Alias("REA", "REGENALL")
	r.Alias("R", "REDRAW")
	r.Alias("RA", "REDRAWALL")
}
