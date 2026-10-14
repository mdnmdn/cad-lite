package commands

import (
	"math"
	"slices"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

// This file is the engine TRIM and EXTEND share: the cutting or boundary edges, the TRIMEXTENDMODE
// prompts (Standard asks for the edges first, Quick takes every object), the pick loop with its
// Fence, Crossing, Project, Edge, eRase and Undo options, and the per-pick undo log. The model is
// planar, so Project is stored (PROJMODE) and has no other effect.

// Keyword ids of the TRIM and EXTEND prompts.
const (
	kwFence    = "Fence"
	kwCrossing = "Crossing"
	kwProject  = "Project"
	kwEdge     = "Edge"
	kwErase    = "eRase"
	kwUndo     = "Undo"
	kwMode     = "mOde"
	kwCutting  = "cuTting edges"
	kwBoundary = "Boundary edges"
	kwExtend   = "Extend"
	kwNoExtend = "No extend"
	kwNone     = "None"
	kwUcs      = "Ucs"
	kwView     = "View"
	kwQuick    = "Quick"
	kwStandard = "Standard"
)

// shiftPicked reports a point picked with Shift held (Input.Shift), which swaps TRIM and EXTEND
// for that pick.
func shiftPicked(in cmdsys.Input) bool { return in.Shift }

type edgeStage int

const (
	stEdges   edgeStage = iota // asking for the cutting or boundary edges
	stObject                   // the pick loop
	stFence1                   // first fence point
	stFenceN                   // next fence point
	stCross1                   // first crossing corner
	stCross2                   // opposite crossing corner
	stErase                    // objects to erase
	stEdgeOpt                  // Edge [Extend/No extend]
	stProjOpt                  // Project [None/Ucs/View]
	stModeOpt                  // mOde [Quick/Standard]
)

// edgeCmd is TRIM (extend false) or EXTEND (extend true).
type edgeCmd struct {
	base
	name   string
	extend bool
	stage  edgeStage
	edges  []doc.Handle // explicit edges; nil means every visible object
	fence  []geom.Vec2
	corner geom.Vec2
	ops    []*edgeOp
	alias  map[doc.Handle]doc.Handle // handle an undone erase had -> the handle it was re-added as
}

func (c *edgeCmd) Name() string { return c.name }

func (c *edgeCmd) quick(ctx *cmdsys.Context) bool {
	return ctx.Vars().Int("TRIMEXTENDMODE") == 1
}

// edgeOp is one undoable step of the command: a pick, a fence, a crossing or an erase.
type edgeOp struct {
	before map[doc.Handle]doc.Entity // state of each entity the step modified or erased
	added  []doc.Handle
}

func newEdgeOp() *edgeOp { return &edgeOp{before: map[doc.Handle]doc.Entity{}} }

func (o *edgeOp) snap(ctx *cmdsys.Context, h doc.Handle) {
	if _, ok := o.before[h]; ok {
		return
	}
	if e := ctx.Doc.Entity(h); e != nil {
		o.before[h] = e.Clone().(doc.Entity)
	}
}

// replace is replaceWith that records what it touches.
func (o *edgeOp) replace(ctx *cmdsys.Context, h doc.Handle, curves []geom.Curve) {
	e := ctx.Doc.Entity(h)
	if e == nil {
		return
	}
	o.snap(ctx, h)
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
			o.added = append(o.added, tx.Add(ne))
		}
	}
}

func (o *edgeOp) erase(ctx *cmdsys.Context, h doc.Handle) {
	o.snap(ctx, h)
	ctx.Tx().Erase(h)
}

// restoreEntity copies src's state into dst (same concrete type), keeping dst's handle and owner.
func restoreEntity(dst, src doc.Entity) {
	h, owner := dst.Common().H, dst.Common().Owner
	switch d := dst.(type) {
	case *doc.Line:
		if s, ok := src.Clone().(*doc.Line); ok {
			*d = *s
		}
	case *doc.Circle:
		if s, ok := src.Clone().(*doc.Circle); ok {
			*d = *s
		}
	case *doc.Arc:
		if s, ok := src.Clone().(*doc.Arc); ok {
			*d = *s
		}
	case *doc.LWPolyline:
		if s, ok := src.Clone().(*doc.LWPolyline); ok {
			*d = *s
		}
	}
	dst.Common().H, dst.Common().Owner = h, owner
}

func (c *edgeCmd) resolve(h doc.Handle) doc.Handle {
	for i := 0; i < 64; i++ {
		n, ok := c.alias[h]
		if !ok {
			break
		}
		h = n
	}
	return h
}

// undo reverts the last step.
func (c *edgeCmd) undo(ctx *cmdsys.Context) {
	n := len(c.ops)
	if n == 0 {
		ctx.Say("cmd.trimext.msg.nothing_to_undo")
		return
	}
	op := c.ops[n-1]
	c.ops = c.ops[:n-1]
	tx := ctx.Tx()
	for _, h := range op.added {
		tx.Erase(c.resolve(h))
	}
	for h, snap := range op.before {
		cur := c.resolve(h)
		if ctx.Doc.Entity(cur) != nil {
			tx.Modify(cur, func(x doc.Entity) { restoreEntity(x, snap) })
			continue
		}
		if c.alias == nil {
			c.alias = map[doc.Handle]doc.Handle{}
		}
		c.alias[h] = tx.Add(snap.Clone().(doc.Entity))
	}
}

// --- prompts ------------------------------------------------------------

func (c *edgeCmd) verb(trim, extend string) string {
	if c.extend {
		return extend
	}
	return trim
}

func (c *edgeCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.saySettings(ctx)
	if c.quick(ctx) {
		return c.objectPrompt(ctx)
	}
	return c.edgesPrompt(ctx)
}

func (c *edgeCmd) saySettings(ctx *cmdsys.Context) {
	proj := [...]string{"none", "ucs", "view"}[min(max(ctx.Vars().Int("PROJMODE"), 0), 2)]
	edge := "none"
	if ctx.Vars().Int("EDGEMODE") == 1 {
		edge = "extend"
	}
	pw, ew := ctx.T("cmd.trimext.word."+proj), ctx.T("cmd.trimext.word."+edge)
	if c.quick(ctx) {
		ctx.Say("cmd.trimext.msg.settings_quick", pw, ew)
		return
	}
	ctx.Say("cmd.trimext.msg.settings", pw, ew)
}

func (c *edgeCmd) edgesPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stEdges
	ctx.Say(c.verb("cmd.trim.msg.select_edges", "cmd.extend.msg.select_edges"))
	return &cmdsys.Prompt{Message: ctx.T("cmd.trimext.prompt.select_edges"), Accepts: cmdsys.InputSelection}
}

func (c *edgeCmd) objectPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stObject
	p := &cmdsys.Prompt{
		Message: ctx.T(c.verb("cmd.trim.prompt.select_object", "cmd.extend.prompt.select_object")),
		Accepts: cmdsys.InputPoint | cmdsys.InputEnter | cmdsys.InputKeyword,
	}
	if c.quick(ctx) {
		p.Keywords = []string{c.verb(kwCutting, kwBoundary), kwCrossing, kwMode, kwProject, kwErase, kwUndo}
	} else {
		p.Keywords = []string{kwFence, kwCrossing, kwProject, kwEdge}
		if !c.extend {
			p.Keywords = append(p.Keywords, kwErase)
		}
		p.Keywords = append(p.Keywords, kwUndo)
	}
	return p
}

func keywordPrompt(msg string, def string, kws ...string) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: msg, Accepts: cmdsys.InputKeyword, Keywords: kws,
		Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: def}, NoOr: true}
}

// --- input --------------------------------------------------------------

func (c *edgeCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case stEdges:
		ctx.Selection.Clear()
		c.edges = nil
		if len(in.Selection) > 0 {
			c.edges = slices.Clone(in.Selection)
			ctx.SayPlural("cmd.common.msg.found", len(c.edges))
		}
		return c.objectPrompt(ctx)
	case stFence1:
		if in.Kind != cmdsys.InputPoint {
			return c.objectPrompt(ctx)
		}
		c.fence = []geom.Vec2{in.Point}
		return c.fenceNext(ctx)
	case stFenceN:
		switch in.Kind {
		case cmdsys.InputPoint:
			c.fence = append(c.fence, in.Point)
		case cmdsys.InputKeyword:
			if len(c.fence) > 1 {
				c.fence = c.fence[:len(c.fence)-1]
			}
		default:
			c.run(ctx, selector{fence: c.fence})
			c.fence = nil
			return c.objectPrompt(ctx)
		}
		return c.fenceNext(ctx)
	case stCross1:
		if in.Kind != cmdsys.InputPoint {
			return c.objectPrompt(ctx)
		}
		c.corner, c.stage = in.Point, stCross2
		return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.opposite_corner"), Accepts: cmdsys.InputPoint, Base: pt(c.corner)}
	case stCross2:
		if in.Kind == cmdsys.InputPoint {
			c.run(ctx, selector{poly: rectPts(c.corner, in.Point), crossing: true, corner: c.corner})
		}
		return c.objectPrompt(ctx)
	case stErase:
		ctx.Selection.Clear()
		if len(in.Selection) > 0 {
			ctx.SayPlural("cmd.common.msg.found", len(in.Selection))
			op := newEdgeOp()
			for _, h := range in.Selection {
				if e := ctx.Doc.Entity(h); e != nil && editable(ctx, e) {
					op.erase(ctx, h)
				}
			}
			if len(op.before) > 0 {
				c.ops = append(c.ops, op)
			}
		}
		return c.objectPrompt(ctx)
	case stEdgeOpt:
		if in.Kind == cmdsys.InputKeyword {
			v := 0
			if in.Keyword == kwExtend {
				v = 1
			}
			_ = ctx.Vars().Set("EDGEMODE", v)
		}
		return c.objectPrompt(ctx)
	case stProjOpt:
		if in.Kind == cmdsys.InputKeyword {
			_ = ctx.Vars().Set("PROJMODE", map[string]int{kwNone: 0, kwUcs: 1, kwView: 2}[in.Keyword])
		}
		return c.objectPrompt(ctx)
	case stModeOpt:
		if in.Kind == cmdsys.InputKeyword {
			quick := 0
			if in.Keyword == kwQuick {
				quick = 1
			}
			_ = ctx.Vars().Set("TRIMEXTENDMODE", quick)
			if quick == 0 {
				return c.edgesPrompt(ctx)
			}
			c.edges = nil
		}
		return c.objectPrompt(ctx)
	}
	return c.objectInput(ctx, in)
}

func (c *edgeCmd) fenceNext(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = stFenceN
	return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.next_fence"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter | cmdsys.InputKeyword,
		Keywords: []string{kwUndo}, Base: pt(c.fence[len(c.fence)-1])}
}

func (c *edgeCmd) objectInput(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch in.Kind {
	case cmdsys.InputEnter:
		return nil
	case cmdsys.InputPoint:
		c.pick(ctx, in)
	case cmdsys.InputKeyword:
		switch in.Keyword {
		case kwFence:
			c.stage = stFence1
			return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.first_fence"), Accepts: cmdsys.InputPoint}
		case kwCrossing:
			c.stage = stCross1
			return &cmdsys.Prompt{Message: ctx.T("cmd.select.prompt.first_corner"), Accepts: cmdsys.InputPoint}
		case kwErase:
			c.stage = stErase
			return &cmdsys.Prompt{Message: ctx.T("cmd.trimext.prompt.erase"), Accepts: cmdsys.InputSelection}
		case kwUndo:
			c.undo(ctx)
		case kwCutting, kwBoundary:
			return c.edgesPrompt(ctx)
		case kwEdge:
			c.stage = stEdgeOpt
			def := kwNoExtend
			if ctx.Vars().Int("EDGEMODE") == 1 {
				def = kwExtend
			}
			return keywordPrompt(ctx.T("cmd.trimext.prompt.edge_mode"), def, kwExtend, kwNoExtend)
		case kwProject:
			c.stage = stProjOpt
			def := [...]string{kwNone, kwUcs, kwView}[min(max(ctx.Vars().Int("PROJMODE"), 0), 2)]
			return keywordPrompt(ctx.T("cmd.trimext.prompt.project"), def, kwNone, kwUcs, kwView)
		case kwMode:
			c.stage = stModeOpt
			def := kwStandard
			if c.quick(ctx) {
				def = kwQuick
			}
			return keywordPrompt(ctx.T(c.verb("cmd.trim.prompt.mode", "cmd.extend.prompt.mode")), def, kwQuick, kwStandard)
		}
	}
	return c.objectPrompt(ctx)
}

// --- geometry -----------------------------------------------------------

// visibleLayer reports whether e's layer is displayed (on and thawed).
func visibleLayer(d *doc.Drawing, e doc.Entity) bool {
	l := d.Layer(e.Common().Layer)
	return l == nil || (!l.Off && !l.Frozen)
}

// editable reports whether e may be changed: visible and on an unlocked layer.
func editable(ctx *cmdsys.Context, e doc.Entity) bool {
	if _, unknown := e.(*doc.Unknown); unknown || !visibleLayer(ctx.Doc, e) {
		return false
	}
	l := ctx.Doc.Layer(e.Common().Layer)
	return l == nil || !l.Locked
}

// livePick returns the editable entity nearest p within the pick radius. It reads the document
// itself, not the spatial index, which lags the command's own open transaction.
func livePick(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, doc.Entity, bool) {
	aperture := pickPixels * ctx.View.PixelSize()
	var best doc.Handle
	bestD := math.Inf(1)
	for _, h := range ctx.Doc.Handles() {
		e := ctx.Doc.Entity(h)
		if e == nil || !editable(ctx, e) {
			continue
		}
		for _, g := range e.Geometry() {
			if d := geom.Closest(g, p).Dist(p); d <= aperture && d <= bestD {
				best, bestD = h, d
			}
		}
	}
	if best == 0 {
		return 0, nil, false
	}
	return best, ctx.Doc.Entity(best), true
}

// edgeCurves returns the geometry of the cutting or boundary edges other than skip: the explicit
// edges, or every visible object. With EDGEMODE 1 each edge stands for its projection.
func (c *edgeCmd) edgeCurves(ctx *cmdsys.Context, skip doc.Handle, near geom.Box2) []geom.Curve {
	hs := c.edges
	if hs == nil {
		hs = slices.Clone(ctx.Doc.Handles())
	}
	implied := ctx.Vars().Int("EDGEMODE") == 1
	reach := 0.0
	if implied {
		ext := ctx.Doc.Extents().Union(near)
		w, h := ext.Size()
		reach = 2*math.Hypot(w, h) + 1
	}
	var out []geom.Curve
	for _, h := range hs {
		e := ctx.Doc.Entity(h)
		if h == skip || e == nil || !visibleLayer(ctx.Doc, e) {
			continue
		}
		for _, g := range e.Geometry() {
			if implied {
				out = append(out, geom.ImpliedEdge(g, reach)...)
			} else {
				out = append(out, g)
			}
		}
	}
	return out
}

// cuts returns the parameters of curve where the edges cross it.
func (c *edgeCmd) cuts(ctx *cmdsys.Context, h doc.Handle, curve geom.Curve) []float64 {
	tol := workTol(ctx)
	var ts []float64
	for _, cu := range c.edgeCurves(ctx, h, curve.Bounds()) {
		for _, q := range geom.Intersect(curve, cu, tol) {
			ts = append(ts, curve.ClosestParam(q))
		}
	}
	return ts
}

func curvesOf(curve geom.Curve, spans []geom.Span) []geom.Curve {
	var out []geom.Curve
	for _, s := range spans {
		if sc := geom.SubCurve(curve, s); sc != nil {
			out = append(out, sc)
		}
	}
	return out
}

// pick trims or extends the object at in.Point (the other verb when Shift was held).
func (c *edgeCmd) pick(ctx *cmdsys.Context, in cmdsys.Input) {
	h, e, ok := livePick(ctx, in.Point)
	if !ok {
		ctx.Say("cmd.common.msg.no_object_at_point")
		return
	}
	op := newEdgeOp()
	var done bool
	if c.extend != shiftPicked(in) {
		done = c.extendObject(ctx, op, h, e, in.Point)
	} else {
		done = c.trimObject(ctx, op, h, e, in.Point)
	}
	if done {
		c.ops = append(c.ops, op)
	}
}

func (c *edgeCmd) trimObject(ctx *cmdsys.Context, op *edgeOp, h doc.Handle, e doc.Entity, p geom.Vec2) bool {
	curve, ok := editCurve(e)
	if !ok {
		ctx.Say("cmd.trim.msg.cannot_trim")
		return false
	}
	spans, ok := geom.TrimSpan(curve, c.cuts(ctx, h, curve), curve.ClosestParam(p))
	if !ok {
		ctx.Say("cmd.trim.msg.no_intersection")
		return false
	}
	op.replace(ctx, h, curvesOf(curve, spans))
	return true
}

func (c *edgeCmd) extendObject(ctx *cmdsys.Context, op *edgeOp, h doc.Handle, e doc.Entity, p geom.Vec2) bool {
	tol := workTol(ctx)
	var nc geom.Curve
	ok := false
	switch e := e.(type) {
	case *doc.Line, *doc.Arc:
		b := c.edgeCurves(ctx, h, e.Bounds())
		nc, ok = extendCurve(e.Geometry()[0], p, b, tol)
	case *doc.LWPolyline:
		if e.Closed {
			ctx.Say("cmd.extend.msg.closed_polyline")
			return false
		}
		b := c.edgeCurves(ctx, h, e.Bounds())
		nc, ok = extendPolyline(e.Geometry()[0].(geom.Polyline), p, b, tol)
	default:
		ctx.Say("cmd.extend.msg.cannot_extend")
		return false
	}
	if !ok {
		ctx.Say("cmd.extend.msg.no_boundary")
		return false
	}
	op.replace(ctx, h, []geom.Curve{nc})
	return true
}

// --- Fence and Crossing -------------------------------------------------

// selector is the Fence or the Crossing window of a TRIM or EXTEND.
type selector struct {
	fence    []geom.Vec2
	poly     []geom.Vec2
	crossing bool
	corner   geom.Vec2
}

func (s selector) touches(path []geom.Vec2) bool {
	paths := [][]geom.Vec2{path}
	if s.crossing {
		return cmdsys.PathsCrossPolygon(paths, s.poly)
	}
	return cmdsys.PathsCrossFence(paths, s.fence)
}

func (s selector) start() geom.Vec2 {
	if s.crossing {
		return s.corner
	}
	return s.fence[0]
}

// run applies the verb to every object the selector touches, as one undo step: TRIM removes the
// pieces between cuts that the fence or window touches, EXTEND grows the end nearest the selector's
// start.
func (c *edgeCmd) run(ctx *cmdsys.Context, sel selector) {
	tol := previewTol(ctx)
	var hit []doc.Handle
	for _, h := range slices.Clone(ctx.Doc.Handles()) {
		e := ctx.Doc.Entity(h)
		if e == nil || !editable(ctx, e) {
			continue
		}
		for _, g := range e.Geometry() {
			if sel.touches(g.Tessellate(tol, nil)) {
				hit = append(hit, h)
				break
			}
		}
	}
	op := newEdgeOp()
	changed := false
	for _, h := range hit {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		if c.extend {
			if g, ok := editCurve(e); ok && c.extendObject(ctx, op, h, e, geom.Closest(g, sel.start())) {
				changed = true
			}
			continue
		}
		if c.trimTouched(ctx, op, h, e, sel, tol) {
			changed = true
		}
	}
	if !changed {
		ctx.Say(c.verb("cmd.trim.msg.no_intersection", "cmd.extend.msg.no_boundary"))
		return
	}
	c.ops = append(c.ops, op)
}

func (c *edgeCmd) trimTouched(ctx *cmdsys.Context, op *edgeOp, h doc.Handle, e doc.Entity, sel selector, tol float64) bool {
	curve, ok := editCurve(e)
	if !ok {
		return false
	}
	pieces := geom.CutPieces(curve, c.cuts(ctx, h, curve))
	if pieces == nil {
		return false
	}
	remove := make([]bool, len(pieces))
	for i, s := range pieces {
		if sc := geom.SubCurve(curve, s); sc != nil {
			remove[i] = sel.touches(sc.Tessellate(tol, nil))
		}
	}
	period := 0.0
	if geom.IsPeriodic(curve) {
		t0, t1 := curve.ParamRange()
		period = t1 - t0
	}
	keep, any := geom.KeepRuns(pieces, remove, period)
	if !any {
		return false
	}
	op.replace(ctx, h, curvesOf(curve, keep))
	return true
}
