package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// pickAt returns the entity under p, using the standard 8 pixel aperture.
func pickAt(ctx *cmdsys.Context, p geom.Vec2) (doc.Entity, bool) {
	h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, p, 8*ctx.View.PixelSize())
	if !ok {
		return nil, false
	}
	e := ctx.Doc.Entity(h)
	return e, e != nil
}

func previewEntity(ctx *cmdsys.Context, e doc.Entity, dl *render.DisplayList) {
	for _, c := range e.Geometry() {
		dl.Curve(c, previewTol(ctx), PreviewColor, 2)
	}
}

// previewHover highlights the entity under the cursor accepted by want.
func previewHover(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList, want func(doc.Entity) bool) {
	if e, ok := pickAt(ctx, cur); ok && want(e) {
		previewEntity(ctx, e, dl)
	}
}

func deg(a float64) string { return cmdsys.FormatNumber(a * 180 / math.Pi) }

func polygonPerimeter(pts []geom.Vec2) float64 {
	var s float64
	for i, a := range pts {
		s += a.Dist(pts[(i+1)%len(pts)])
	}
	return s
}

// entityArea measures e: the area it encloses and its perimeter, with the
// perimeter's label. Open polylines and arcs are closed by their chord.
func entityArea(e doc.Entity) (area, perim float64, label string, ok bool) {
	switch e := e.(type) {
	case *doc.Circle:
		return math.Pi * e.Radius * e.Radius, 2 * math.Pi * e.Radius, "Circumference", true
	case *doc.Arc:
		a := e.Geometry()[0].(geom.Arc)
		return math.Abs(geom.ArcSegmentArea(a.R, a.Sweep())), a.Length(), "Perimeter", true
	case *doc.LWPolyline:
		p := e.Geometry()[0].(geom.Polyline)
		if len(p.Pts) < 2 {
			return 0, 0, "", false
		}
		return math.Abs(geom.PolylineArea(p)), p.Length(), "Perimeter", true
	}
	return 0, 0, "", false
}

// --- AREA ---------------------------------------------------------------

type areaCmd struct {
	pts    []geom.Vec2
	object bool
}

func (*areaCmd) Name() string { return "AREA" }

func (*areaCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify first corner point", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Object"}}
}

func (c *areaCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.object {
		return c.inputObject(ctx, in)
	}
	switch {
	case in.Keyword == "Object" && len(c.pts) == 0:
		c.object = true
		return &cmdsys.Prompt{Message: "Select object", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
	case in.Kind == cmdsys.InputEnter:
		c.report(ctx)
		return nil
	case in.Kind == cmdsys.InputPoint:
		c.pts = append(c.pts, in.Point)
	}
	return &cmdsys.Prompt{Message: "Specify next corner point or press ENTER for total", Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: pt(c.pts[len(c.pts)-1])}
}

func (c *areaCmd) report(ctx *cmdsys.Context) {
	area, perim := 0.0, 0.0
	if len(c.pts) >= 3 {
		area = math.Abs(geom.PolygonArea(c.pts))
	}
	if len(c.pts) >= 2 {
		perim = polygonPerimeter(c.pts)
	}
	ctx.Printf("Area = %s, Perimeter = %s", cmdsys.FormatNumber(area), cmdsys.FormatNumber(perim))
}

func (c *areaCmd) inputObject(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind != cmdsys.InputPoint {
		return nil
	}
	e, ok := pickAt(ctx, in.Point)
	if !ok {
		ctx.Printf("No object found.")
		return &cmdsys.Prompt{Message: "Select object", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
	}
	area, perim, label, ok := entityArea(e)
	if !ok {
		ctx.Printf("Object has no area.")
		return nil
	}
	ctx.Printf("Area = %s, %s = %s", cmdsys.FormatNumber(area), label, cmdsys.FormatNumber(perim))
	return nil
}

func (c *areaCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.object {
		previewHover(ctx, cur, dl, func(e doc.Entity) bool { _, _, _, ok := entityArea(e); return ok })
		return
	}
	if len(c.pts) > 0 {
		dl.Polyline(append(append([]geom.Vec2(nil), c.pts...), cur), PreviewColor, 1, true)
	}
}

func (*areaCmd) Cancel(*cmdsys.Context) {}

// --- radius -------------------------------------------------------------

type radiusCmd struct{}

func (*radiusCmd) Name() string { return "RADIUS" }

func radiusPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select arc or circle", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func (*radiusCmd) Start(*cmdsys.Context) *cmdsys.Prompt { return radiusPrompt() }

func radiusOf(e doc.Entity) (float64, bool) {
	switch e := e.(type) {
	case *doc.Circle:
		return e.Radius, true
	case *doc.Arc:
		return e.Radius, true
	}
	return 0, false
}

func (*radiusCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind != cmdsys.InputPoint {
		return nil
	}
	e, ok := pickAt(ctx, in.Point)
	if !ok {
		ctx.Printf("No object found.")
		return radiusPrompt()
	}
	r, ok := radiusOf(e)
	if !ok {
		ctx.Printf("Select an arc or a circle.")
		return radiusPrompt()
	}
	ctx.Printf("Radius = %s, Diameter = %s", cmdsys.FormatNumber(r), cmdsys.FormatNumber(2*r))
	return nil
}

func (*radiusCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	previewHover(ctx, cur, dl, func(e doc.Entity) bool { _, ok := radiusOf(e); return ok })
}

func (*radiusCmd) Cancel(*cmdsys.Context) {}

// --- angle --------------------------------------------------------------

type angleStage int

const (
	angleObject angleStage = iota
	angleLine2
	angleVertex
	angleFirst
	angleSecond
)

type angleCmd struct {
	stage  angleStage
	vertex geom.Vec2
	first  geom.Vec2
	line1  geom.Line
	pick1  geom.Vec2
}

func (*angleCmd) Name() string { return "ANGLE" }

func (*angleCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select arc, circle, line or <specify vertex>", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func endpointPrompt(msg string, vertex geom.Vec2) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: msg, Accepts: cmdsys.InputPoint, Base: pt(vertex)}
}

func lineOf(e doc.Entity) (geom.Line, bool) {
	if l, ok := e.(*doc.Line); ok {
		return geom.Line{A: l.Start, B: l.End}, true
	}
	return geom.Line{}, false
}

// angleBetween returns the angle in [0, π] at v between the rays to a and b.
func angleBetween(v, a, b geom.Vec2) float64 {
	da, db := a.Sub(v), b.Sub(v)
	la, lb := da.Len(), db.Len()
	if la == 0 || lb == 0 {
		return 0
	}
	return math.Acos(math.Max(-1, math.Min(1, da.Dot(db)/(la*lb))))
}

func (c *angleCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case angleObject:
		if in.Kind == cmdsys.InputEnter {
			c.stage = angleVertex
			return &cmdsys.Prompt{Message: "Specify angle vertex", Accepts: cmdsys.InputPoint}
		}
		e, ok := pickAt(ctx, in.Point)
		if !ok {
			c.vertex, c.stage = in.Point, angleFirst
			return endpointPrompt("Specify first angle endpoint", c.vertex)
		}
		switch e := e.(type) {
		case *doc.Arc:
			ctx.Printf("Angle = %s°", cmdsys.FormatNumber(e.Geometry()[0].(geom.Arc).Sweep()*180/math.Pi))
			return nil
		case *doc.Circle:
			c.vertex, c.first, c.stage = e.Center, in.Point, angleSecond
			return endpointPrompt("Specify second angle endpoint", c.vertex)
		case *doc.Line:
			c.line1, _ = lineOf(e)
			c.pick1, c.stage = in.Point, angleLine2
			return &cmdsys.Prompt{Message: "Select second line", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
		}
		ctx.Printf("Select an arc, a circle or a line.")
		return c.Start(ctx)
	case angleLine2:
		if in.Kind != cmdsys.InputPoint {
			return nil
		}
		e, ok := pickAt(ctx, in.Point)
		l2, isLine := geom.Line{}, false
		if ok {
			l2, isLine = lineOf(e)
		}
		if !isLine {
			ctx.Printf("Select a line.")
			return &cmdsys.Prompt{Message: "Select second line", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
		}
		ctx.Printf("Angle = %s°", deg(lineAngle(c.line1, c.pick1, l2, in.Point)))
		return nil
	case angleVertex:
		c.vertex, c.stage = in.Point, angleFirst
		return endpointPrompt("Specify first angle endpoint", c.vertex)
	case angleFirst:
		c.first, c.stage = in.Point, angleSecond
		return endpointPrompt("Specify second angle endpoint", c.vertex)
	}
	ctx.Printf("Angle = %s°", deg(angleBetween(c.vertex, c.first, in.Point)))
	return nil
}

// lineAngle returns the angle in [0, π] between two lines at their
// intersection, measured between the rays that run through the pick points.
func lineAngle(l1 geom.Line, p1 geom.Vec2, l2 geom.Line, p2 geom.Vec2) float64 {
	d1, d2 := l1.B.Sub(l1.A), l2.B.Sub(l2.A)
	cross := d1.Cross(d2)
	if math.Abs(cross) <= 1e-12*d1.Len()*d2.Len() {
		return 0
	}
	t := l2.A.Sub(l1.A).Cross(d2) / cross
	x := l1.A.Add(d1.Scale(t))
	ray := func(d, p geom.Vec2) geom.Vec2 {
		if d.Dot(p.Sub(x)) < 0 {
			return x.Add(d.Scale(-1))
		}
		return x.Add(d)
	}
	return angleBetween(x, ray(d1, p1), ray(d2, p2))
}

func (c *angleCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch c.stage {
	case angleObject:
		previewHover(ctx, cur, dl, func(e doc.Entity) bool {
			switch e.(type) {
			case *doc.Arc, *doc.Circle, *doc.Line:
				return true
			}
			return false
		})
	case angleLine2:
		dl.Curve(c.line1, previewTol(ctx), PreviewColor, 2)
		previewHover(ctx, cur, dl, func(e doc.Entity) bool { _, ok := lineOf(e); return ok })
	case angleFirst:
		dl.Polyline([]geom.Vec2{c.vertex, cur}, PreviewColor, 1, false)
	case angleSecond:
		dl.Polyline([]geom.Vec2{c.first, c.vertex, cur}, PreviewColor, 1, false)
	}
}

func (*angleCmd) Cancel(*cmdsys.Context) {}

// --- MEASUREGEOM --------------------------------------------------------

type measureGeomCmd struct {
	sub cmdsys.Command
}

func (*measureGeomCmd) Name() string { return "MEASUREGEOM" }

func (*measureGeomCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	def := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Distance"}
	return &cmdsys.Prompt{Message: "Enter an option", Accepts: cmdsys.InputKeyword, Keywords: []string{"Distance", "Radius", "Angle", "ARea"}, Default: &def}
}

func (c *measureGeomCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.sub != nil {
		return c.sub.Input(ctx, in)
	}
	switch in.Keyword {
	case "Distance":
		c.sub = &distCmd{}
	case "Radius":
		c.sub = &radiusCmd{}
	case "Angle":
		c.sub = &angleCmd{}
	case "ARea":
		c.sub = &areaCmd{}
	default:
		return c.Start(ctx)
	}
	return c.sub.Start(ctx)
}

func (c *measureGeomCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.sub != nil {
		c.sub.Preview(ctx, cur, dl)
	}
}

func (c *measureGeomCmd) Cancel(ctx *cmdsys.Context) {
	if c.sub != nil {
		c.sub.Cancel(ctx)
	}
}
