package commands

import (
	"errors"
	"math"
	"strings"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// Settings the construct commands remember between runs, as AutoCAD keeps
// them in system variables: the last OFFSET distance (0 means Through), the
// FILLET radius and the two CHAMFER distances.
var (
	lastOffset   float64
	filletRadius float64
	chamferD1    float64
	chamferD2    float64
)

// pickAperturePx is the pick aperture of the construct commands, in pixels.
const pickAperturePx = 8

// registerConstruct adds MIRROR, OFFSET, FILLET and CHAMFER and their aliases.
func registerConstruct(r *cmdsys.Registry) {
	r.Register("MIRROR", func() cmdsys.Command { return &mirrorCmd{} })
	r.Register("OFFSET", func() cmdsys.Command { return &offsetCmd{} })
	r.Register("FILLET", func() cmdsys.Command { return &cornerCmd{} })
	r.Register("CHAMFER", func() cmdsys.Command { return &cornerCmd{chamfer: true} })
	r.Alias("MI", "MIRROR")
	r.Alias("O", "OFFSET")
	r.Alias("F", "FILLET")
	r.Alias("CHA", "CHAMFER")
}

func keywordInput(kw string) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: kw}
}

func distanceInput(v float64) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputDistance, Value: v}
}

// pickConstruct returns the entity under p within the pick aperture.
func pickConstruct(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, bool) {
	return cmdsys.Pick(ctx.Doc, ctx.Index, p, pickAperturePx*ctx.View.PixelSize())
}

// --- MIRROR -------------------------------------------------------------

type mirrorCmd struct {
	base
	selecting
	p1, p2 *geom.Vec2
}

func (*mirrorCmd) Name() string { return "MIRROR" }

func (c *mirrorCmd) firstPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify first point of mirror line", Accepts: cmdsys.InputPoint}
}

func (c *mirrorCmd) secondPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify second point of mirror line", Accepts: cmdsys.InputPoint, Base: c.p1}
}

func (c *mirrorCmd) erasePrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Erase source objects", Accepts: cmdsys.InputKeyword,
		Keywords: []string{"Yes", "No"}, Default: keywordInput("No")}
}

func (c *mirrorCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.firstPrompt()
	}
	return selectPrompt()
}

func (c *mirrorCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.hs == nil:
		if !c.take(ctx, in) {
			return nil
		}
		return c.firstPrompt()
	case c.p1 == nil:
		c.p1 = pt(in.Point)
		return c.secondPrompt()
	case c.p2 == nil:
		if in.Point.Near(*c.p1, 1e-12) {
			ctx.Printf("The two points of the mirror line must differ.")
			return c.secondPrompt()
		}
		c.p2 = pt(in.Point)
		return c.erasePrompt()
	}
	m := mirrorMatrix(*c.p1, *c.p2)
	tx := ctx.Tx()
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		cp := e.Clone().(doc.Entity)
		cp.Transform(m)
		layer, color := cp.Common().Layer, cp.Common().Color
		*cp.Common() = doc.EntityCommon{Layer: layer, Color: color}
		tx.Add(cp)
		if in.Keyword == "Yes" {
			tx.Erase(h)
		}
	}
	return nil
}

// mirrorMatrix returns the reflection across the line through a and b.
func mirrorMatrix(a, b geom.Vec2) geom.Mat3 {
	s, k := math.Sincos(2 * b.Sub(a).Angle())
	return geom.Translate(a).Mul(geom.Mat3{k, s, s, -k, 0, 0}).Mul(geom.Translate(a.Scale(-1)))
}

func (c *mirrorCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.p1 == nil || c.p2 != nil {
		return
	}
	dl.Polyline([]geom.Vec2{*c.p1, cur}, PreviewColor.WithAlpha(120), 0, false)
	if cur.Near(*c.p1, 1e-12) {
		return
	}
	c.previewTransformed(ctx, mirrorMatrix(*c.p1, cur), dl)
}

// --- OFFSET -------------------------------------------------------------

type offsetCmd struct {
	base
	through bool
	dist    float64
	first   *geom.Vec2 // first point of a two-point distance
	haveDst bool
	src     doc.Handle
}

func (*offsetCmd) Name() string { return "OFFSET" }

func (c *offsetCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	def := keywordInput("Through")
	if lastOffset > 0 {
		def = distanceInput(lastOffset)
	}
	return &cmdsys.Prompt{Message: "Specify offset distance", Accepts: cmdsys.InputDistance | cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Through"}, Default: def}
}

func (c *offsetCmd) selectPrompt() *cmdsys.Prompt {
	c.src = 0
	return &cmdsys.Prompt{Message: "Select object to offset", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Exit"}, Default: keywordInput("Exit")}
}

func (c *offsetCmd) sidePrompt() *cmdsys.Prompt {
	msg := "Specify point on side to offset"
	if c.through {
		msg = "Specify through point"
	}
	return &cmdsys.Prompt{Message: msg, Accepts: cmdsys.InputPoint | cmdsys.InputKeyword,
		Keywords: []string{"Exit"}, Default: keywordInput("Exit")}
}

func (c *offsetCmd) setDistance(ctx *cmdsys.Context, d float64) *cmdsys.Prompt {
	if d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		ctx.Printf("Requires a positive, nonzero distance.")
		return c.Start(ctx)
	}
	c.dist, c.haveDst = d, true
	lastOffset = d
	return c.selectPrompt()
}

func (c *offsetCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case !c.haveDst && !c.through:
		switch {
		case in.Kind == cmdsys.InputKeyword:
			c.through = true
			return c.selectPrompt()
		case in.Kind == cmdsys.InputPoint && c.first == nil:
			c.first = pt(in.Point)
			return &cmdsys.Prompt{Message: "Specify second point", Accepts: cmdsys.InputPoint, Base: c.first}
		case in.Kind == cmdsys.InputPoint:
			d := c.first.Dist(in.Point)
			c.first = nil
			return c.setDistance(ctx, d)
		}
		return c.setDistance(ctx, in.Value)
	case c.src == 0:
		if in.Kind != cmdsys.InputPoint {
			return nil
		}
		h, ok := pickConstruct(ctx, in.Point)
		if !ok {
			ctx.Printf("Nothing found at that point.")
			return c.selectPrompt()
		}
		if !offsettable(ctx.Doc.Entity(h)) {
			ctx.Printf("Cannot offset this object.")
			return c.selectPrompt()
		}
		c.src = h
		return c.sidePrompt()
	}
	if in.Kind != cmdsys.InputPoint {
		return nil
	}
	src := ctx.Doc.Entity(c.src)
	ne, msg := offsetEntity(src, in.Point, c.dist, c.through)
	if ne == nil {
		ctx.Printf("%s", msg)
		return c.selectPrompt()
	}
	ctx.Tx().Add(ne)
	return c.selectPrompt()
}

func (c *offsetCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.src == 0 {
		return
	}
	ne, _ := offsetEntity(ctx.Doc.Entity(c.src), cur, c.dist, c.through)
	if ne == nil {
		return
	}
	tol := previewTol(ctx)
	for _, g := range ne.Geometry() {
		dl.Curve(g, tol, PreviewColor, 1)
	}
}

func offsettable(e doc.Entity) bool {
	switch e.(type) {
	case *doc.Line, *doc.Circle, *doc.Arc, *doc.LWPolyline:
		return true
	}
	return false
}

const degenerateOffset = "The offset would have a zero or negative radius; nothing was created."

// offsetEntity returns the offset of e towards the point side, dist away or,
// with through, passing through side. It returns nil and a message when the
// result is degenerate or e cannot be offset. The new entity inherits the
// layer and colour of e.
func offsetEntity(e doc.Entity, side geom.Vec2, dist float64, through bool) (doc.Entity, string) {
	var out doc.Entity
	switch e := e.(type) {
	case *doc.Line:
		l := geom.Line{A: e.Start, B: e.End}
		cross := l.B.Sub(l.A).Cross(side.Sub(l.A))
		if through {
			dist = math.Abs(cross) / math.Max(l.Length(), 1e-300)
		}
		if dist <= 0 {
			return nil, "The through point lies on the object; nothing was created."
		}
		if cross < 0 {
			dist = -dist
		}
		o := geom.OffsetLine(l, dist)
		out = &doc.Line{Start: o.A, End: o.B}
	case *doc.Circle:
		d := side.Dist(e.Center) - e.Radius
		if through {
			dist = d
		} else if d < 0 {
			dist = -dist
		}
		if dist == 0 {
			return nil, "The through point lies on the object; nothing was created."
		}
		o, ok := geom.OffsetCircle(geom.Circle{C: e.Center, R: e.Radius}, dist)
		if !ok {
			return nil, degenerateOffset
		}
		out = &doc.Circle{Center: o.C, Radius: o.R}
	case *doc.Arc:
		d := side.Dist(e.Center) - e.Radius
		if through {
			dist = d
		} else if d < 0 {
			dist = -dist
		}
		if dist == 0 {
			return nil, "The through point lies on the object; nothing was created."
		}
		o, ok := geom.OffsetArc(geom.Arc{C: e.Center, R: e.Radius, Start: e.StartAngle, End: e.EndAngle}, dist)
		if !ok {
			return nil, degenerateOffset
		}
		out = &doc.Arc{Center: o.C, Radius: o.R, StartAngle: o.Start, EndAngle: o.End}
	case *doc.LWPolyline:
		p := e.Geometry()[0].(geom.Polyline)
		t := p.ClosestParam(side)
		q := p.PointAt(t)
		if through {
			dist = q.Dist(side)
		}
		if dist <= 0 {
			return nil, "The through point lies on the object; nothing was created."
		}
		if p.Tangent(t).Cross(side.Sub(q)) < 0 {
			dist = -dist
		}
		o, ok := geom.OffsetPolyline(p, dist)
		if !ok {
			return nil, degenerateOffset
		}
		out = &doc.LWPolyline{Points: o.Pts, Bulges: o.Bulges, Closed: o.Closed}
	default:
		return nil, "Cannot offset this object."
	}
	c := out.Common()
	c.Layer, c.Color = e.Common().Layer, e.Common().Color
	return out, ""
}

// --- FILLET and CHAMFER -------------------------------------------------

// cornerCmd is FILLET or, with chamfer set, CHAMFER: both cut two lines
// back to a corner and join them with an arc or a bevel.
type cornerCmd struct {
	base
	chamfer bool
	ask     string // "radius", "d1", "d2" while a setting is being asked
	h1      doc.Handle
	p1      geom.Vec2
	d1      float64
}

func (c *cornerCmd) Name() string {
	if c.chamfer {
		return "CHAMFER"
	}
	return "FILLET"
}

func (c *cornerCmd) noun() string {
	if c.chamfer {
		return "line"
	}
	return "object"
}

func (c *cornerCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.chamfer {
		ctx.Printf("(TRIM mode) Current chamfer Dist1 = %s, Dist2 = %s", cmdsys.FormatNumber(chamferD1), cmdsys.FormatNumber(chamferD2))
	} else {
		ctx.Printf("Current settings: Mode = TRIM, Radius = %s", cmdsys.FormatNumber(filletRadius))
	}
	return c.firstPrompt()
}

func (c *cornerCmd) firstPrompt() *cmdsys.Prompt {
	c.h1, c.ask = 0, ""
	kw := "Radius"
	if c.chamfer {
		kw = "Distance"
	}
	return &cmdsys.Prompt{Message: "Select first " + c.noun(), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{kw}}
}

func (c *cornerCmd) secondPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select second " + c.noun(), Accepts: cmdsys.InputPoint}
}

func (c *cornerCmd) askPrompt() *cmdsys.Prompt {
	switch c.ask {
	case "radius":
		return &cmdsys.Prompt{Message: "Specify fillet radius", Accepts: cmdsys.InputDistance, Default: distanceInput(filletRadius)}
	case "d1":
		return &cmdsys.Prompt{Message: "Specify first chamfer distance", Accepts: cmdsys.InputDistance, Default: distanceInput(chamferD1)}
	}
	return &cmdsys.Prompt{Message: "Specify second chamfer distance", Accepts: cmdsys.InputDistance, Default: distanceInput(c.d1)}
}

func (c *cornerCmd) lineAt(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, *doc.Line) {
	h, ok := pickConstruct(ctx, p)
	if !ok {
		return 0, nil
	}
	l, _ := ctx.Doc.Entity(h).(*doc.Line)
	return h, l
}

func (c *cornerCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.ask != "" {
		if in.Value < 0 || math.IsNaN(in.Value) {
			ctx.Printf("Requires a non-negative distance.")
			return c.askPrompt()
		}
		switch c.ask {
		case "radius":
			filletRadius = in.Value
			return c.firstPrompt()
		case "d1":
			c.d1, chamferD1 = in.Value, in.Value
			c.ask = "d2"
			return c.askPrompt()
		}
		chamferD2 = in.Value
		return c.firstPrompt()
	}
	if in.Kind == cmdsys.InputKeyword {
		c.ask = "radius"
		if c.chamfer {
			c.ask = "d1"
		}
		return c.askPrompt()
	}
	if c.h1 == 0 {
		h, ok := pickConstruct(ctx, in.Point)
		if !ok {
			ctx.Printf("Nothing found at that point.")
			return c.firstPrompt()
		}
		if _, isLine := ctx.Doc.Entity(h).(*doc.Line); !isLine {
			ctx.Printf("%s supports two lines.", c.title())
			return nil
		}
		c.h1, c.p1 = h, in.Point
		return c.secondPrompt()
	}
	h2, l2 := c.lineAt(ctx, in.Point)
	switch {
	case h2 == 0:
		ctx.Printf("Nothing found at that point.")
		return c.secondPrompt()
	case l2 == nil:
		ctx.Printf("%s supports two lines.", c.title())
		return nil
	case h2 == c.h1:
		ctx.Printf("Select a different line.")
		return c.secondPrompt()
	}
	l1 := ctx.Doc.Entity(c.h1).(*doc.Line)
	corner, err := c.compute(l1, l2, c.p1, in.Point)
	if err != nil {
		switch {
		case errors.Is(err, geom.ErrParallel):
			ctx.Printf("Lines are parallel.")
		case c.chamfer:
			ctx.Printf("Chamfer distances are too large for the lines.")
		default:
			ctx.Printf("Radius is too large for the lines.")
		}
		return nil
	}
	tx := ctx.Tx()
	tx.Modify(c.h1, func(e doc.Entity) { cutLine(e.(*doc.Line), corner.Far1, corner.Join1) })
	tx.Modify(h2, func(e doc.Entity) { cutLine(e.(*doc.Line), corner.Far2, corner.Join2) })
	var ne doc.Entity
	switch {
	case corner.Arc != nil:
		ne = &doc.Arc{Center: corner.Arc.C, Radius: corner.Arc.R, StartAngle: corner.Arc.Start, EndAngle: corner.Arc.End}
	case corner.Bevel != nil:
		ne = &doc.Line{Start: corner.Bevel.A, End: corner.Bevel.B}
	}
	if ne != nil {
		ne.Common().Layer, ne.Common().Color = l1.Layer, l1.Color
		tx.Add(ne)
	}
	return nil
}

func (c *cornerCmd) title() string {
	n := c.Name()
	return n[:1] + strings.ToLower(n[1:])
}

func (c *cornerCmd) compute(l1, l2 *doc.Line, p1, p2 geom.Vec2) (geom.Corner, error) {
	a, b := geom.Line{A: l1.Start, B: l1.End}, geom.Line{A: l2.Start, B: l2.End}
	if c.chamfer {
		return geom.ChamferLines(a, b, p1, p2, chamferD1, chamferD2)
	}
	return geom.FilletLines(a, b, p1, p2, filletRadius)
}

// cutLine moves the end of l that is farther from far to join, keeping far.
func cutLine(l *doc.Line, far, join geom.Vec2) {
	if l.Start.Dist(far) <= l.End.Dist(far) {
		l.End = join
	} else {
		l.Start = join
	}
}

func (c *cornerCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.h1 == 0 || c.ask != "" {
		return
	}
	l1, _ := ctx.Doc.Entity(c.h1).(*doc.Line)
	h2, l2 := c.lineAt(ctx, cur)
	if l1 == nil || l2 == nil || h2 == c.h1 {
		return
	}
	corner, err := c.compute(l1, l2, c.p1, cur)
	if err != nil {
		return
	}
	tol := previewTol(ctx)
	dl.Curve(geom.Line{A: corner.Far1, B: corner.Join1}, tol, PreviewColor, 1)
	dl.Curve(geom.Line{A: corner.Far2, B: corner.Join2}, tol, PreviewColor, 1)
	switch {
	case corner.Arc != nil:
		dl.Curve(*corner.Arc, tol, PreviewColor, 1)
	case corner.Bevel != nil:
		dl.Curve(*corner.Bevel, tol, PreviewColor, 1)
	}
}
