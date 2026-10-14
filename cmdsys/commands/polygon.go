package commands

import (
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- POLYGON ------------------------------------------------------------

type polyStep int

const (
	polySides      polyStep = iota // number of sides <POLYSIDES>
	polyCenter                     // centre or [Edge]
	polyMode                       // [Inscribed in circle/Circumscribed about circle] <I>
	polyRadius                     // radius of the circle
	polyEdgeFirst                  // Edge: first endpoint
	polyEdgeSecond                 // Edge: second endpoint
)

// polygonCmd draws a regular polygon as a closed LWPOLYLINE: from the centre and a radius of the
// inscribed or circumscribed circle, or from one edge.
// polygonModeKeywords are the ids of the inscribed and circumscribed options.
var polygonModeKeywords = []string{"Inscribed in circle", "Circumscribed about circle"}

type polygonCmd struct {
	base
	step          polyStep
	n             int
	center, edgeA geom.Vec2
	circumscribed bool
}

func (*polygonCmd) Name() string { return "POLYGON" }

func (c *polygonCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.circumscribed = ctx.Vars().Int("DRAW_POLYCIRCUMSCRIBED") == 1
	return c.prompt(ctx)
}

func (c *polygonCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.step {
	case polyCenter:
		return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.center"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Edge"}}
	case polyMode:
		id := "Inscribed in circle"
		if c.circumscribed {
			id = "Circumscribed about circle"
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.option"), Accepts: cmdsys.InputKeyword, Keywords: polygonModeKeywords, Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: kwShortcut(ctx, id)}}
	case polyRadius:
		return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.radius"), Accepts: cmdsys.InputDistance | cmdsys.InputPoint, Base: pt(c.center)}
	case polyEdgeFirst:
		return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.edge_first"), Accepts: cmdsys.InputPoint}
	case polyEdgeSecond:
		return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.edge_second"), Accepts: cmdsys.InputPoint, Base: pt(c.edgeA)}
	}
	def := ctx.Vars().Int("POLYSIDES")
	return &cmdsys.Prompt{Message: ctx.T("cmd.polygon.prompt.sides"), Accepts: cmdsys.InputInteger, Default: &cmdsys.Input{Kind: cmdsys.InputInteger, Value: float64(def)}}
}

func (c *polygonCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case polySides:
		if in.Kind != cmdsys.InputInteger {
			break
		}
		n := int(in.Value)
		if n < 3 || n > 1024 {
			ctx.Say("cmd.polygon.msg.sides")
			break
		}
		c.n, c.step = n, polyCenter
		_ = ctx.Vars().Set("POLYSIDES", n)
	case polyCenter:
		switch {
		case in.Keyword == "Edge":
			c.step = polyEdgeFirst
		case in.Kind == cmdsys.InputPoint:
			c.center, c.step = in.Point, polyMode
		}
	case polyMode:
		switch {
		case in.Keyword == "Inscribed in circle", in.Kind == cmdsys.InputText && in.Text == kwShortcut(ctx, "Inscribed in circle"):
			c.circumscribed, c.step = false, polyRadius
		case in.Keyword == "Circumscribed about circle", in.Kind == cmdsys.InputText && in.Text == kwShortcut(ctx, "Circumscribed about circle"):
			c.circumscribed, c.step = true, polyRadius
		}
		if c.step == polyRadius {
			m := 0
			if c.circumscribed {
				m = 1
			}
			_ = ctx.Vars().Set("DRAW_POLYCIRCUMSCRIBED", m)
		}
	case polyRadius:
		if pts := c.byRadius(in); pts != nil {
			ctx.Tx().Add(&doc.LWPolyline{Points: pts, Closed: true})
			return nil
		}
		ctx.Say("cmd.common.msg.positive")
	case polyEdgeFirst:
		if in.Kind == cmdsys.InputPoint {
			c.edgeA, c.step = in.Point, polyEdgeSecond
		}
	case polyEdgeSecond:
		if in.Kind == cmdsys.InputPoint {
			pts := PolygonFromEdge(c.edgeA, in.Point, c.n)
			if pts == nil {
				ctx.Say("cmd.common.msg.positive")
				break
			}
			ctx.Tx().Add(&doc.LWPolyline{Points: pts, Closed: true})
			return nil
		}
	}
	return c.prompt(ctx)
}

// byRadius returns the polygon for a radius answer at polyRadius, nil when it is not positive.
// A picked point puts a vertex (inscribed) or an edge midpoint (circumscribed) at the point; a
// typed radius gives the polygon a horizontal bottom edge.
func (c *polygonCmd) byRadius(in cmdsys.Input) []geom.Vec2 {
	switch in.Kind {
	case cmdsys.InputPoint:
		d := in.Point.Sub(c.center)
		return polygonFromCenter(c.n, c.center, d.Len(), d.Angle(), c.circumscribed)
	case cmdsys.InputDistance:
		return PolygonFromCenter(c.center, in.Value, c.n, !c.circumscribed, 0)
	}
	return nil
}

// PolygonFromCenter returns the vertices, counter-clockwise, of the regular polygon POLYGON draws
// for a typed radius: sides vertices around center; radius is the circumradius when inscribed
// is set (Inscribed in circle) and the apothem otherwise (Circumscribed about circle). At
// rotation 0 the bottom edge is horizontal, as AutoCAD draws a polygon whose radius is typed;
// rotation (radians) turns it counter-clockwise about center. It returns nil when sides is under
// 3 or the radius is not a positive finite number.
func PolygonFromCenter(center geom.Vec2, radius float64, sides int, inscribed bool, rotation float64) []geom.Vec2 {
	if sides < 3 {
		return nil
	}
	// A horizontal bottom edge: an edge midpoint at -90°, or the vertices π/n past it.
	if inscribed {
		return polygonFromCenter(sides, center, radius, -math.Pi/2+math.Pi/float64(sides)+rotation, false)
	}
	return polygonFromCenter(sides, center, radius, -math.Pi/2+rotation, true)
}

// PolygonFromEdge returns the vertices of the regular polygon with sides sides whose first edge
// runs from a to b, counter-clockwise with the polygon to the left of the edge (POLYGON's Edge
// option). It returns nil when sides is under 3 or the edge has no length.
func PolygonFromEdge(a, b geom.Vec2, sides int) []geom.Vec2 { return polygonFromEdge(sides, a, b) }

// polygonFromCenter returns the n vertices of the regular polygon around c. With circumscribed
// unset, r is the circumradius and a vertex lies at angle; set, r is the apothem and an edge
// midpoint lies at angle. Vertices run counter-clockwise. It returns nil when r is not positive.
func polygonFromCenter(n int, c geom.Vec2, r, angle float64, circumscribed bool) []geom.Vec2 {
	if n < 3 || !(r > 0) || math.IsInf(r, 0) {
		return nil
	}
	first, rr := angle, r
	if circumscribed {
		rr = r / math.Cos(math.Pi/float64(n))
		first = angle + math.Pi/float64(n)
	}
	pts := make([]geom.Vec2, n)
	for k := range pts {
		pts[k] = c.Polar(rr, first+2*math.Pi*float64(k)/float64(n))
	}
	return pts
}

// polygonFromEdge returns the polygon whose first edge runs from a to b, built counter-clockwise
// so that the polygon lies to the left of the edge. It returns nil for a zero-length edge.
func polygonFromEdge(n int, a, b geom.Vec2) []geom.Vec2 {
	edge := b.Sub(a)
	s := edge.Len()
	if n < 3 || !(s > 0) || math.IsInf(s, 0) {
		return nil
	}
	apothem := s / 2 / math.Tan(math.Pi/float64(n))
	c := a.Lerp(b, 0.5).Add(edge.Norm().Perp().Scale(apothem))
	start := a.Sub(c).Angle()
	r := a.Dist(c)
	pts := make([]geom.Vec2, n)
	for k := range pts {
		pts[k] = c.Polar(r, start+2*math.Pi*float64(k)/float64(n))
	}
	return pts
}

func (c *polygonCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	var pts []geom.Vec2
	switch c.step {
	case polyRadius:
		pts = c.byRadius(cmdsys.Input{Kind: cmdsys.InputPoint, Point: cur})
		dl.Polyline([]geom.Vec2{c.center, cur}, PreviewColor.WithAlpha(120), 0, false)
	case polyEdgeSecond:
		pts = polygonFromEdge(c.n, c.edgeA, cur)
	}
	if pts != nil {
		dl.Polyline(pts, PreviewColor, 1, true)
	}
}

// registerPolygon adds POLYGON and its alias.
func registerPolygon(r *cmdsys.Registry) {
	r.Register("POLYGON", func() cmdsys.Command { return &polygonCmd{} })
	r.Alias("POL", "POLYGON")
}
