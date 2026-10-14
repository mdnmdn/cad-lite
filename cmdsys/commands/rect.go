package commands

import (
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- RECTANG ------------------------------------------------------------

type rectStep int

const (
	rectFirst     rectStep = iota // first corner or [Chamfer/Elevation/Fillet/Thickness/Width]
	rectChamfer1                  // first chamfer distance
	rectChamfer2                  // second chamfer distance
	rectFillet                    // fillet radius
	rectWidth                     // polyline width
	rectElevation                 // elevation
	rectThickness                 // thickness
	rectOther                     // other corner or [Area/Dimensions/Rotation]
	rectDimLength                 // Dimensions: length
	rectDimWidth                  // Dimensions: width
	rectArea                      // Area: the area
	rectAreaBasis                 // Area: [Length/Width]
	rectAreaSide                  // Area: the length or the width
	rectRotation                  // Rotation: angle or [Pick points]
	rectRotFirst                  // Rotation, Pick points: first point
	rectRotSecond                 // Rotation, Pick points: second point
)

// rectCmd draws a closed LWPOLYLINE from two corners. Chamfer, Fillet and Width stay set in the
// session (the DRAW_RECT* variables) until changed; Dimensions and Area fix the size and let the
// pick choose the quadrant; Rotation turns the rectangle about its first corner.
// rectCornerKeywords are the ids of the options at the other-corner prompt. "Rect area" is the
// id of kw.rect_area, labelled "Area" (kw.area is MEASUREGEOM's, with the shortcut AR).
var rectCornerKeywords = []string{"Rect area", "Dimensions", "Rotation"}

type rectCmd struct {
	base
	step  rectStep
	first geom.Vec2
	rot   float64
	dims  *[2]float64 // length (along the rotated x) and width, set by Dimensions and Area
	// areaByWidth is set when Area is based on the width rather than the length
	area       float64
	areaByWide bool
	rotFirst   geom.Vec2
	tmp        float64 // first chamfer distance while the second is asked
}

func (*rectCmd) Name() string { return "RECTANG" }

func (c *rectCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.prompt(ctx) }

func (c *rectCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	v := ctx.Vars()
	dist := func(key string, def float64) *cmdsys.Prompt {
		return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputDistance, Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: def}}
	}
	switch c.step {
	case rectChamfer1:
		return dist("cmd.rectang.prompt.chamfer_first", v.Real("DRAW_RECTCHAMFER1"))
	case rectChamfer2:
		return dist("cmd.rectang.prompt.chamfer_second", c.tmp)
	case rectFillet:
		return dist("cmd.rectang.prompt.fillet", v.Real("DRAW_RECTFILLET"))
	case rectWidth:
		return dist("cmd.rectang.prompt.width", v.Real("DRAW_RECTWIDTH"))
	case rectElevation:
		return dist("cmd.rectang.prompt.elevation", v.Real("DRAW_RECTELEVATION"))
	case rectThickness:
		return dist("cmd.rectang.prompt.thickness", v.Real("DRAW_RECTTHICKNESS"))
	case rectOther:
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.other_corner"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: rectCornerKeywords, Base: pt(c.first)}
	case rectDimLength:
		return dist("cmd.rectang.prompt.length", v.Real("DRAW_RECTLENGTH"))
	case rectDimWidth:
		return dist("cmd.rectang.prompt.dim_width", v.Real("DRAW_RECTDEPTH"))
	case rectArea:
		return dist("cmd.rectang.prompt.area", v.Real("DRAW_RECTAREA"))
	case rectAreaBasis:
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.based_on"), Accepts: cmdsys.InputKeyword, Keywords: []string{"Length", "Width"}, Default: keywordInput("Length")}
	case rectAreaSide:
		if c.areaByWide {
			return dist("cmd.rectang.prompt.enter_width", v.Real("DRAW_RECTDEPTH"))
		}
		return dist("cmd.rectang.prompt.enter_length", v.Real("DRAW_RECTLENGTH"))
	case rectRotation:
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.rotation"), Accepts: cmdsys.InputAngle | cmdsys.InputKeyword, Keywords: []string{"Pick points"}, Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: c.rot}, Base: pt(c.first)}
	case rectRotFirst:
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.rot_first"), Accepts: cmdsys.InputPoint}
	case rectRotSecond:
		return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.rot_second"), Accepts: cmdsys.InputPoint, Base: pt(c.rotFirst)}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.rectang.prompt.first_corner"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Chamfer", "Elevation", "Fillet", "Thickness", "Width"}}
}

// set stores a remembered value.
func set(ctx *cmdsys.Context, name string, v float64) { _ = ctx.Vars().Set(name, v) }

func (c *rectCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	num := in.Kind == cmdsys.InputDistance
	switch c.step {
	case rectFirst:
		switch {
		case in.Keyword == "Chamfer":
			c.step = rectChamfer1
		case in.Keyword == "Elevation":
			c.step = rectElevation
		case in.Keyword == "Fillet":
			c.step = rectFillet
		case in.Keyword == "Thickness":
			c.step = rectThickness
		case in.Keyword == "Width":
			c.step = rectWidth
		case in.Kind == cmdsys.InputPoint:
			c.first, c.step = in.Point, rectOther
		}
	case rectChamfer1, rectChamfer2, rectFillet, rectWidth:
		if !num {
			break
		}
		if in.Value < 0 {
			ctx.Say("cmd.rectang.msg.negative")
			break
		}
		switch c.step {
		case rectChamfer1:
			c.tmp, c.step = in.Value, rectChamfer2
		case rectChamfer2:
			set(ctx, "DRAW_RECTCHAMFER1", c.tmp)
			set(ctx, "DRAW_RECTCHAMFER2", in.Value)
			set(ctx, "DRAW_RECTFILLET", 0) // Chamfer and Fillet cancel each other
			c.step = rectFirst
		case rectFillet:
			set(ctx, "DRAW_RECTFILLET", in.Value)
			set(ctx, "DRAW_RECTCHAMFER1", 0)
			set(ctx, "DRAW_RECTCHAMFER2", 0)
			c.step = rectFirst
		default:
			set(ctx, "DRAW_RECTWIDTH", in.Value)
			c.step = rectFirst
		}
	case rectElevation:
		if num {
			set(ctx, "DRAW_RECTELEVATION", in.Value)
			c.step = rectFirst
		}
	case rectThickness:
		if num {
			set(ctx, "DRAW_RECTTHICKNESS", in.Value)
			c.step = rectFirst
		}
	case rectOther:
		switch {
		case in.Keyword == "Rect area":
			c.step = rectArea
		case in.Keyword == "Dimensions":
			c.step = rectDimLength
		case in.Keyword == "Rotation":
			c.step = rectRotation
		case in.Kind == cmdsys.InputPoint:
			return c.finish(ctx, in.Point)
		}
	case rectDimLength, rectDimWidth, rectArea, rectAreaSide:
		if !num {
			break
		}
		if !(in.Value > 0) {
			ctx.Say("cmd.common.msg.positive")
			break
		}
		switch c.step {
		case rectDimLength:
			set(ctx, "DRAW_RECTLENGTH", in.Value)
			c.step = rectDimWidth
		case rectDimWidth:
			set(ctx, "DRAW_RECTDEPTH", in.Value)
			v := ctx.Vars()
			c.dims = &[2]float64{v.Real("DRAW_RECTLENGTH"), in.Value}
			c.step = rectOther
		case rectArea:
			c.area = in.Value
			set(ctx, "DRAW_RECTAREA", in.Value)
			c.step = rectAreaBasis
		default:
			c.dims = c.areaDims(ctx, in.Value)
			c.step = rectOther
		}
	case rectAreaBasis:
		switch in.Keyword {
		case "Length":
			c.areaByWide, c.step = false, rectAreaSide
		case "Width":
			c.areaByWide, c.step = true, rectAreaSide
		}
	case rectRotation:
		switch {
		case in.Keyword == "Pick points":
			c.step = rectRotFirst
		case in.Kind == cmdsys.InputAngle:
			c.rot, c.step = in.Value, rectOther
		}
	case rectRotFirst:
		if in.Kind == cmdsys.InputPoint {
			c.rotFirst, c.step = in.Point, rectRotSecond
		}
	case rectRotSecond:
		if in.Kind == cmdsys.InputPoint {
			if d := in.Point.Sub(c.rotFirst); d.Len() > 0 {
				c.rot = d.Angle()
			}
			c.step = rectOther
		}
	}
	return c.prompt(ctx)
}

// areaDims returns length and width of a rectangle of c.area given one side, counting the area
// the chamfers or fillets take away (AutoCAD's Area includes them).
func (c *rectCmd) areaDims(ctx *cmdsys.Context, side float64) *[2]float64 {
	v := ctx.Vars()
	loss := 0.0
	if r := v.Real("DRAW_RECTFILLET"); r > 0 {
		loss = (4 - math.Pi) * r * r
	} else {
		loss = 2 * v.Real("DRAW_RECTCHAMFER1") * v.Real("DRAW_RECTCHAMFER2")
	}
	other := (c.area + loss) / side
	if c.areaByWide {
		set(ctx, "DRAW_RECTDEPTH", side)
		return &[2]float64{other, side}
	}
	set(ctx, "DRAW_RECTLENGTH", side)
	return &[2]float64{side, other}
}

// outline returns the vertices, bulges and per-segment widths of the rectangle from the first
// corner to other, with the session's chamfer, fillet and width.
func (c *rectCmd) outline(ctx *cmdsys.Context, other geom.Vec2) (pts []geom.Vec2, bulges []float64, widths [][2]float64) {
	u := geom.V(math.Cos(c.rot), math.Sin(c.rot))
	vv := u.Perp()
	d := other.Sub(c.first)
	dx, dy := d.Dot(u), d.Dot(vv)
	if c.dims != nil {
		dx, dy = math.Copysign(c.dims[0], signOr1(dx)), math.Copysign(c.dims[1], signOr1(dy))
	}
	corner := [4]geom.Vec2{c.first, c.first.Add(u.Scale(dx)), c.first.Add(u.Scale(dx)).Add(vv.Scale(dy)), c.first.Add(vv.Scale(dy))}
	lenU, lenV := math.Abs(dx), math.Abs(dy)
	v := ctx.Vars()
	fillet, d1, d2 := v.Real("DRAW_RECTFILLET"), v.Real("DRAW_RECTCHAMFER1"), v.Real("DRAW_RECTCHAMFER2")
	w := v.Real("DRAW_RECTWIDTH")
	switch {
	case fillet > 0 && 2*fillet <= math.Min(lenU, lenV)+1e-12:
		d1, d2 = fillet, fillet
	case fillet <= 0 && (d1 > 0 || d2 > 0) && 2*d1 <= lenU+1e-12 && 2*d2 <= lenV+1e-12:
		fillet = 0
	default:
		fillet, d1, d2 = 0, 0, 0
	}
	if d1 == 0 && d2 == 0 {
		pts = corner[:]
	} else {
		// Edge k runs from corner k to corner k+1; even edges lie along u, odd along v.
		edgeDist := func(k int) float64 {
			if k%2 == 0 {
				return d1
			}
			return d2
		}
		unit := func(k int) geom.Vec2 { return corner[(k+1)%4].Sub(corner[k]).Norm() }
		out := func(k int) geom.Vec2 { return corner[k].Add(unit(k).Scale(edgeDist(k))) }
		in := func(k int) geom.Vec2 { return corner[k].Sub(unit((k + 3) % 4).Scale(edgeDist((k + 3) % 4))) }
		bulge := 0.0
		if fillet > 0 {
			bulge = math.Tan(math.Pi / 8)
			if dx*dy < 0 {
				bulge = -bulge
			}
		}
		pts = []geom.Vec2{out(0)}
		bulges = []float64{0}
		for k := 1; k <= 4; k++ {
			pts = append(pts, in(k%4))
			bulges = append(bulges, bulge)
			if k < 4 {
				pts = append(pts, out(k))
				bulges = append(bulges, 0)
			}
		}
	}
	if fillet == 0 {
		bulges = nil
	}
	if w > 0 {
		widths = make([][2]float64, len(pts))
		for i := range widths {
			widths[i] = [2]float64{w, w}
		}
	}
	return pts, bulges, widths
}

func signOr1(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func (c *rectCmd) finish(ctx *cmdsys.Context, other geom.Vec2) *cmdsys.Prompt {
	pts, bulges, widths := c.outline(ctx, other)
	ctx.Tx().Add(&doc.LWPolyline{Points: pts, Bulges: bulges, Widths: widths, Closed: true})
	return nil
}

func (c *rectCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.step == rectOther {
		pts, bulges, _ := c.outline(ctx, cur)
		dl.Curve(geom.Polyline{Pts: pts, Bulges: bulges, Closed: true}, previewTol(ctx), PreviewColor, 1)
	}
}
