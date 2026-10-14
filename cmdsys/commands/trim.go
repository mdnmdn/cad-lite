package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

// registerTrim adds TRIM, EXTEND, BREAK, JOIN and EXPLODE and their aliases.
func registerTrim(r *cmdsys.Registry) {
	r.Register("TRIM", func() cmdsys.Command { return &trimCmd{} })
	r.Register("EXTEND", func() cmdsys.Command { return &extendCmd{} })
	r.Register("BREAK", func() cmdsys.Command { return &breakCmd{} })
	r.Register("JOIN", func() cmdsys.Command { return &joinCmd{} })
	r.Register("EXPLODE", func() cmdsys.Command { return &explodeCmd{} })
	r.Alias("TR", "TRIM")
	r.Alias("EX", "EXTEND")
	r.Alias("BR", "BREAK")
	r.Alias("J", "JOIN")
	r.Alias("X", "EXPLODE")
}

// pickPixels is the pick radius, in pixels, of the point-picking prompts.
const pickPixels = 8

// workTol returns the distance below which two points of the drawing are the
// same point.
func workTol(ctx *cmdsys.Context) float64 {
	ext := ctx.Doc.Extents()
	if ext.IsEmpty() {
		return geom.DefaultTol(1).Linear
	}
	w, h := ext.Size()
	return geom.DefaultTol(math.Max(w, h)).Linear
}

// pickEntity returns the selectable entity under p.
func pickEntity(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, doc.Entity, bool) {
	h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, p, pickPixels*ctx.View.PixelSize())
	if !ok {
		return 0, nil, false
	}
	e := ctx.Doc.Entity(h)
	return h, e, e != nil
}

// shown reports whether e's layer is displayed.
func shown(d *doc.Drawing, e doc.Entity) bool {
	l := d.Layer(e.Common().Layer)
	return l == nil || !l.Off
}

// editCurve returns the single curve of a LINE, CIRCLE, ARC or LWPOLYLINE.
func editCurve(e doc.Entity) (geom.Curve, bool) {
	switch e.(type) {
	case *doc.Line, *doc.Circle, *doc.Arc, *doc.LWPolyline:
		if g := e.Geometry(); len(g) == 1 {
			return g[0], true
		}
	}
	return nil, false
}

// boundaryCurves returns the geometry of the visible entities among hs other than
// skip; with hs nil it takes every entity near box.
func boundaryCurves(ctx *cmdsys.Context, hs []doc.Handle, skip doc.Handle, box *geom.Box2) []geom.Curve {
	if hs == nil {
		if box != nil {
			hs = ctx.Index.Query(*box)
		} else {
			hs = ctx.Doc.Handles()
		}
	}
	var out []geom.Curve
	for _, h := range hs {
		e := ctx.Doc.Entity(h)
		if h == skip || e == nil || !shown(ctx.Doc, e) {
			continue
		}
		out = append(out, e.Geometry()...)
	}
	return out
}

// setGeometry stores c in e when their kinds match.
func setGeometry(e doc.Entity, c geom.Curve) bool {
	switch e := e.(type) {
	case *doc.Line:
		if l, ok := c.(geom.Line); ok {
			e.Start, e.End = l.A, l.B
			return true
		}
	case *doc.Circle:
		if k, ok := c.(geom.Circle); ok {
			e.Center, e.Radius = k.C, k.R
			return true
		}
	case *doc.Arc:
		if a, ok := c.(geom.Arc); ok {
			e.Center, e.Radius, e.StartAngle, e.EndAngle = a.C, a.R, a.Start, a.End
			return true
		}
	case *doc.LWPolyline:
		if p, ok := c.(geom.Polyline); ok {
			e.Points, e.Bulges, e.Closed = p.Pts, p.Bulges, p.Closed
			return true
		}
	}
	return false
}

// newEntity returns an entity of the kind of c carrying like's layer and
// colour, or nil for a curve with no entity kind.
func newEntity(c geom.Curve, like doc.Entity) doc.Entity {
	common := doc.EntityCommon{Layer: like.Common().Layer, Color: like.Common().Color}
	switch c := c.(type) {
	case geom.Line:
		return &doc.Line{EntityCommon: common, Start: c.A, End: c.B}
	case geom.Circle:
		return &doc.Circle{EntityCommon: common, Center: c.C, Radius: c.R}
	case geom.Arc:
		return &doc.Arc{EntityCommon: common, Center: c.C, Radius: c.R, StartAngle: c.Start, EndAngle: c.End}
	case geom.Polyline:
		return &doc.LWPolyline{EntityCommon: common, Points: c.Pts, Bulges: c.Bulges, Closed: c.Closed}
	}
	return nil
}

// replaceWith replaces entity h by curves, all on h's layer and colour. The
// first curve reuses h when it is of the same kind; no curves erase h.
func replaceWith(ctx *cmdsys.Context, h doc.Handle, curves []geom.Curve) {
	e := ctx.Doc.Entity(h)
	tx := ctx.Tx()
	if len(curves) == 0 {
		tx.Erase(h)
		return
	}
	rest := curves
	if probe := e.Clone().(doc.Entity); setGeometry(probe, curves[0]) {
		c0 := curves[0]
		tx.Modify(h, func(x doc.Entity) { setGeometry(x, c0) })
		rest = curves[1:]
	} else {
		tx.Erase(h)
	}
	for _, c := range rest {
		if ne := newEntity(c, e); ne != nil {
			tx.Add(ne)
		}
	}
}

// --- TRIM ---------------------------------------------------------------

type trimCmd struct {
	base
	edges    []doc.Handle // explicit cutting edges; nil means every visible entity
	choosing bool
}

func (*trimCmd) Name() string { return "TRIM" }

func (c *trimCmd) prompt() *cmdsys.Prompt {
	p := &cmdsys.Prompt{Message: "Select object to trim", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
	if c.edges == nil {
		p.Accepts |= cmdsys.InputKeyword
		p.Keywords = []string{"cuTting edges"}
	}
	return p
}

func (c *trimCmd) Start(*cmdsys.Context) *cmdsys.Prompt { return c.prompt() }

func (c *trimCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.choosing:
		c.choosing = false
		if len(in.Selection) > 0 {
			c.edges = append([]doc.Handle(nil), in.Selection...)
			ctx.Printf("%d found", len(c.edges))
		}
		ctx.Selection.Clear()
		return c.prompt()
	case in.Kind == cmdsys.InputEnter:
		return nil
	case in.Kind == cmdsys.InputKeyword:
		c.choosing = true
		return &cmdsys.Prompt{Message: "Select cutting edges <select all>", Accepts: cmdsys.InputSelection}
	case in.Kind == cmdsys.InputPoint:
		c.trimAt(ctx, in.Point)
	}
	return c.prompt()
}

func (c *trimCmd) trimAt(ctx *cmdsys.Context, p geom.Vec2) {
	h, e, ok := pickEntity(ctx, p)
	if !ok {
		ctx.Printf("No object found at that point.")
		return
	}
	curve, ok := editCurve(e)
	if !ok {
		ctx.Printf("Cannot trim this object.")
		return
	}
	tol := workTol(ctx)
	box := curve.Bounds().Grow(tol)
	var cutters []geom.Curve
	if c.edges == nil {
		cutters = boundaryCurves(ctx, nil, h, &box)
	} else {
		cutters = boundaryCurves(ctx, c.edges, h, nil)
	}
	var ts []float64
	for _, cu := range cutters {
		for _, q := range geom.Intersect(curve, cu, tol) {
			ts = append(ts, curve.ClosestParam(q))
		}
	}
	spans, ok := geom.TrimSpan(curve, ts, curve.ClosestParam(p))
	if !ok {
		ctx.Printf("Object does not intersect an edge.")
		return
	}
	var out []geom.Curve
	for _, s := range spans {
		if sc := geom.SubCurve(curve, s); sc != nil {
			out = append(out, sc)
		}
	}
	replaceWith(ctx, h, out)
}

// --- EXTEND -------------------------------------------------------------

type extendCmd struct{ base }

func (*extendCmd) Name() string { return "EXTEND" }

func (*extendCmd) prompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select object to extend", Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func (c *extendCmd) Start(*cmdsys.Context) *cmdsys.Prompt { return c.prompt() }

func (c *extendCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	if in.Kind == cmdsys.InputPoint {
		c.extendAt(ctx, in.Point)
	}
	return c.prompt()
}

func (c *extendCmd) extendAt(ctx *cmdsys.Context, p geom.Vec2) {
	h, e, ok := pickEntity(ctx, p)
	if !ok {
		ctx.Printf("No object found at that point.")
		return
	}
	boundaries := boundaryCurves(ctx, nil, h, nil)
	tol := workTol(ctx)
	var nc geom.Curve
	switch e := e.(type) {
	case *doc.Line:
		nc, ok = extendCurve(e.Geometry()[0], p, boundaries, tol)
	case *doc.Arc:
		nc, ok = extendCurve(e.Geometry()[0], p, boundaries, tol)
	case *doc.LWPolyline:
		if e.Closed {
			ctx.Printf("Cannot extend a closed polyline.")
			return
		}
		nc, ok = extendPolyline(e.Geometry()[0].(geom.Polyline), p, boundaries, tol)
	default:
		ctx.Printf("Cannot extend this object.")
		return
	}
	if !ok {
		ctx.Printf("No boundary edge in the extension of that end.")
		return
	}
	replaceWith(ctx, h, []geom.Curve{nc})
}

// extendCurve extends the end of the Line or Arc c nearest p.
func extendCurve(c geom.Curve, p geom.Vec2, boundaries []geom.Curve, tol float64) (geom.Curve, bool) {
	t0, t1 := c.ParamRange()
	atEnd := p.Dist(c.PointAt(t1)) < p.Dist(c.PointAt(t0))
	return geom.ExtendEnd(c, atEnd, boundaries, tol)
}

// extendPolyline extends the end vertex of the open polyline nearest p
// along its end segment.
func extendPolyline(pl geom.Polyline, p geom.Vec2, boundaries []geom.Curve, tol float64) (geom.Curve, bool) {
	n := len(pl.Pts)
	if n < 2 {
		return nil, false
	}
	last := p.Dist(pl.Pts[n-1]) < p.Dist(pl.Pts[0])
	seg, vertex := 0, 0
	if last {
		seg, vertex = n-2, n-1
	}
	curve := pl.Segment(seg)
	rev := seg < len(pl.Bulges) && pl.Bulges[seg] < 0
	if _, isArc := curve.(geom.Arc); !isArc {
		rev = false
	}
	nc, ok := geom.ExtendEnd(curve, last != rev, boundaries, tol)
	if !ok {
		return nil, false
	}
	piece := geom.Piece{Curve: nc, Rev: rev}
	out := geom.Polyline{Pts: append([]geom.Vec2(nil), pl.Pts...), Bulges: make([]float64, n)}
	copy(out.Bulges, pl.Bulges)
	if last {
		out.Pts[vertex] = piece.End()
	} else {
		out.Pts[vertex] = piece.Start()
	}
	out.Bulges[seg] = piece.Bulge()
	return out, true
}

// --- BREAK --------------------------------------------------------------

type breakCmd struct {
	base
	h        doc.Handle
	first    *geom.Vec2
	choosing bool // asking for the first point again after "First point"
}

func (*breakCmd) Name() string { return "BREAK" }

func (*breakCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Select object", Accepts: cmdsys.InputPoint}
}

func (c *breakCmd) secondPrompt() *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify second break point", Accepts: cmdsys.InputPoint, Base: c.first}
}

func (c *breakCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case c.h == 0:
		h, e, ok := pickEntity(ctx, in.Point)
		if !ok {
			ctx.Printf("No object found at that point.")
			return c.Start(ctx)
		}
		if _, ok := editCurve(e); !ok {
			ctx.Printf("Cannot break this object.")
			return c.Start(ctx)
		}
		c.h, c.first = h, pt(in.Point)
		return &cmdsys.Prompt{Message: "Specify second break point", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"First point"}, Base: c.first}
	case in.Kind == cmdsys.InputKeyword:
		c.choosing = true
		return &cmdsys.Prompt{Message: "Specify first break point", Accepts: cmdsys.InputPoint}
	case c.choosing:
		c.choosing = false
		c.first = pt(in.Point)
		return c.secondPrompt()
	}
	c.breakAt(ctx, *c.first, in.Point)
	return nil
}

func (c *breakCmd) breakAt(ctx *cmdsys.Context, p1, p2 geom.Vec2) {
	e := ctx.Doc.Entity(c.h)
	if e == nil {
		return
	}
	curve, ok := editCurve(e)
	if !ok {
		return
	}
	pa, pb := curve.ClosestParam(p1), curve.ClosestParam(p2)
	if curve.PointAt(pa).Near(curve.PointAt(pb), workTol(ctx)) {
		pb = pa
		if _, circle := curve.(geom.Circle); circle {
			ctx.Printf("Cannot break a circle at a single point.")
			return
		}
	}
	var out []geom.Curve
	for _, s := range geom.KeepOutside(curve, pa, pb) {
		if sc := geom.SubCurve(curve, s); sc != nil {
			out = append(out, sc)
		}
	}
	replaceWith(ctx, c.h, out)
}

// --- JOIN ---------------------------------------------------------------

type joinCmd struct {
	base
	hs     []doc.Handle
	source bool // the source object is chosen, more objects are being asked for
}

func (*joinCmd) Name() string { return "JOIN" }

func (c *joinCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if ctx.Selection.Len() > 0 {
		c.hs = append([]doc.Handle(nil), ctx.Selection.Handles()...)
		ctx.Selection.Clear()
		return c.run(ctx)
	}
	return &cmdsys.Prompt{Message: "Select source object or multiple objects to join at once", Accepts: cmdsys.InputSelection}
}

func (c *joinCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	ctx.Selection.Clear()
	if len(in.Selection) == 0 {
		if c.source {
			return c.run(ctx)
		}
		return nil
	}
	ctx.Printf("%d found", len(in.Selection))
	if !c.source {
		c.hs = append([]doc.Handle(nil), in.Selection...)
		if len(c.hs) == 1 {
			c.source = true
			return &cmdsys.Prompt{Message: "Select objects to join to source", Accepts: cmdsys.InputSelection}
		}
		return c.run(ctx)
	}
	for _, h := range in.Selection {
		if h != c.hs[0] {
			c.hs = append(c.hs, h)
		}
	}
	return c.run(ctx)
}

// joinNode is a source object or a group of objects already merged into one
// Line or Arc, ready to chain.
type joinNode struct {
	members []doc.Handle
	curve   geom.Curve // the merged Line, Arc or Circle; nil for a polyline
	pieces  []geom.Piece
}

func (c *joinCmd) run(ctx *cmdsys.Context) *cmdsys.Prompt {
	tol := workTol(ctx)
	var nodes []*joinNode
	skipped := 0
	for i, h := range c.hs {
		e := ctx.Doc.Entity(h)
		var n *joinNode
		switch e := e.(type) {
		case *doc.Line:
			if l := (geom.Line{A: e.Start, B: e.End}); l.Length() > tol {
				n = &joinNode{curve: l, pieces: []geom.Piece{{Curve: l}}}
			}
		case *doc.Arc:
			a := e.Geometry()[0].(geom.Arc)
			n = &joinNode{curve: a, pieces: []geom.Piece{{Curve: a}}}
		case *doc.LWPolyline:
			if pl := e.Geometry()[0].(geom.Polyline); !e.Closed {
				if ps := geom.PolylinePieces(pl); len(ps) > 0 {
					n = &joinNode{pieces: ps}
				}
			}
		}
		if n == nil {
			if i == 0 {
				ctx.Printf("Cannot join the source object.")
				return nil
			}
			skipped++
			continue
		}
		n.members = []doc.Handle{h}
		nodes = append(nodes, n)
	}
	nodes = mergeNodes(nodes, tol)
	groups := make([][]geom.Piece, len(nodes))
	for i, n := range nodes {
		groups[i] = n.pieces
	}
	chain, used := geom.ChainPieces(groups, 0, tol)
	var members []doc.Handle
	for _, i := range used {
		members = append(members, nodes[i].members...)
	}
	if skipped > 0 {
		ctx.Printf("%d %s cannot be joined.", skipped, plural(skipped))
	}
	if len(members) < 2 {
		ctx.Printf("0 objects joined.")
		return nil
	}
	var result geom.Curve
	if len(used) == 1 && nodes[0].curve != nil {
		result = nodes[0].curve
	} else {
		pl := geom.PolylineFromPieces(chain, tol)
		result = pl
	}
	for _, h := range members[1:] {
		ctx.Tx().Erase(h)
	}
	replaceWith(ctx, c.hs[0], []geom.Curve{result})
	n := len(members) - 1
	ctx.Printf("%d %s joined.", n, plural(n))
	return nil
}

// mergeNodes folds collinear touching Lines and co-circular touching Arcs
// into single nodes, keeping the source first. A pair of arcs that close a
// circle becomes a Circle node, which cannot chain further.
func mergeNodes(nodes []*joinNode, tol float64) []*joinNode {
	for changed := true; changed; {
		changed = false
	scan:
		for i := 0; i < len(nodes); i++ {
			for j := i + 1; j < len(nodes); j++ {
				if m, ok := mergeCurves(nodes[i].curve, nodes[j].curve, tol); ok {
					nodes[i].curve = m
					nodes[i].members = append(nodes[i].members, nodes[j].members...)
					nodes[i].pieces = nil
					if p, ok := m.(geom.Line); ok {
						nodes[i].pieces = []geom.Piece{{Curve: p}}
					} else if a, ok := m.(geom.Arc); ok {
						nodes[i].pieces = []geom.Piece{{Curve: a}}
					}
					nodes = append(nodes[:j], nodes[j+1:]...)
					changed = true
					break scan
				}
			}
		}
	}
	return nodes
}

func mergeCurves(a, b geom.Curve, tol float64) (geom.Curve, bool) {
	switch a := a.(type) {
	case geom.Line:
		if b, ok := b.(geom.Line); ok {
			l, ok := geom.JoinLines(a, b, tol)
			return l, ok
		}
	case geom.Arc:
		if b, ok := b.(geom.Arc); ok {
			return geom.JoinArcs(a, b, tol)
		}
	}
	return nil, false
}

// --- EXPLODE ------------------------------------------------------------

type explodeCmd struct {
	base
	selecting
}

func (*explodeCmd) Name() string { return "EXPLODE" }

func (c *explodeCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		c.run(ctx)
		return nil
	}
	return &cmdsys.Prompt{Message: "Select objects to explode", Accepts: cmdsys.InputSelection}
}

func (c *explodeCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.take(ctx, in) {
		c.run(ctx)
	}
	return nil
}

func (c *explodeCmd) run(ctx *cmdsys.Context) {
	exploded, refused := 0, 0
	for _, h := range c.hs {
		pl, ok := ctx.Doc.Entity(h).(*doc.LWPolyline)
		if !ok {
			refused++
			continue
		}
		var parts []geom.Curve
		for _, s := range pl.Geometry()[0].(geom.Polyline).Segments() {
			if a, b := ends(s); !a.Near(b, 0) || isArc(s) {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			refused++
			continue
		}
		tx := ctx.Tx()
		for _, s := range parts {
			tx.Add(newEntity(s, pl))
		}
		tx.Erase(h)
		exploded++
	}
	if refused > 0 {
		ctx.Printf("%d %s cannot be exploded.", refused, plural(refused))
	}
	ctx.Printf("%d %s exploded.", exploded, plural(exploded))
}

func isArc(c geom.Curve) bool { _, ok := c.(geom.Arc); return ok }

func ends(c geom.Curve) (geom.Vec2, geom.Vec2) {
	t0, t1 := c.ParamRange()
	return c.PointAt(t0), c.PointAt(t1)
}

func plural(n int) string {
	if n == 1 {
		return "object"
	}
	return "objects"
}
