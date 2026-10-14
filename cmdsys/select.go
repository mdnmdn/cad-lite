package cmdsys

import (
	"slices"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// This file is the typed side of "Select objects:". A text stage (TextOrderSelection) reads the
// options of the AutoCAD selection prompt (W C WP CP F BOX ALL L P A R M U AU SI), a typed point
// is a pick, and the objects accumulate in the session's Selection exactly as clicks do; the
// command still gets the set when Enter ends the prompt. Options that need further points
// (corners, polygon and fence vertices) swap the active command for a selSub wrapper until the
// shape is complete, so clicks and typed points reach it through the normal point prompt.

type selEngine struct {
	s *Session
	// owner is the command's own selection prompt; shown is the Remove-mode copy while Remove
	// is on. Both nil until an option is typed. State resets when the prompt changes.
	owner, shown *Prompt
	remove       bool
	single       bool
	// undo holds the selection before each operation, for the U option.
	undo [][]doc.Handle
}

func init() {
	RegisterVarGroup(func(s *Session, _ *Vars) {
		e := &selEngine{s: s}
		s.AddTextHandler(TextOrderSelection, e.handle)
	})
}

// selectable reports whether e may be selected: modelled, on a thawed and unlocked layer, and
// (unless off is allowed, for ALL) not on a layer that is switched off.
func selectable(d *doc.Drawing, e doc.Entity, allowOff bool) bool {
	if e == nil || isUnknown(e) {
		return false
	}
	l := d.Layer(e.Common().Layer)
	return l == nil || (!l.Locked && !l.Frozen && (allowOff || !l.Off))
}

func (e *selEngine) handle(s *Session, text string) bool {
	if s.active == nil || s.prompt == nil {
		return false
	}
	if _, ok := s.active.(*selSub); ok {
		return false
	}
	p := s.prompt
	if p.Accepts&InputSelection == 0 || p.Accepts&InputPoint != 0 {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	tr := s.Translator()
	if p.Accepts&InputKeyword != 0 {
		if _, ok := tr.MatchKeywords(p.Keywords, t); ok {
			return false
		}
	}
	if p != e.owner && p != e.shown {
		e.owner, e.shown, e.remove, e.single, e.undo = p, nil, false, false, nil
	}
	s.print(s.Prompt() + " " + text)
	if id, ok := tr.MatchKeywords(selOptions, t); ok {
		e.option(id)
		return true
	}
	if in, err := ParseInputIn(tr, &Prompt{Accepts: InputPoint}, t, s.base(), s.cursor); err == nil {
		s.mem.SetPoint(in.Point)
		e.pick(in.Point)
		return true
	}
	s.ctx.Say("cmd.select.msg.invalid")
	s.ctx.Say("cmd.select.msg.expects")
	return true
}

func (e *selEngine) option(id string) {
	d := e.s.ctx.Doc
	switch id {
	case selOptWindow:
		e.begin(subWindow)
	case selOptCrossing:
		e.begin(subCrossing)
	case selOptBox:
		e.begin(subBox)
	case selOptFence:
		e.begin(subFence)
	case selOptWPolygon:
		e.begin(subWPoly)
	case selOptCPolygon:
		e.begin(subCPoly)
	case selOptAll:
		var hs []doc.Handle
		for _, h := range d.Handles() {
			if selectable(d, d.Entity(h), true) {
				hs = append(hs, h)
			}
		}
		e.done(e.apply(hs))
	case selOptLast:
		var hs []doc.Handle
		all := d.Handles()
		for i := len(all) - 1; i >= 0; i-- {
			if selectable(d, d.Entity(all[i]), false) {
				hs = []doc.Handle{all[i]}
				break
			}
		}
		e.done(e.apply(hs))
	case selOptPrevious:
		var hs []doc.Handle
		for _, h := range e.s.ctx.PreviousSelection() {
			if selectable(d, d.Entity(h), false) {
				hs = append(hs, h)
			}
		}
		e.done(e.apply(hs))
	case selOptAdd:
		e.remove = false
		e.s.prompt, e.shown = e.owner, nil
	case selOptRemove:
		e.remove = true
		e.shown = &Prompt{Message: e.s.ctx.T("cmd.select.prompt.remove_objects"), Accepts: InputSelection}
		e.s.prompt = e.shown
	case selOptSingle:
		e.single = true
	case selOptUndo:
		if n := len(e.undo); n > 0 {
			sel := e.s.ctx.Selection
			sel.Clear()
			sel.Add(e.undo[n-1]...)
			e.undo = e.undo[:n-1]
		}
	}
}

// pick handles a typed point: the object under it, or the first corner of an automatic box.
func (e *selEngine) pick(p geom.Vec2) {
	s := e.s
	if h, ok := Pick(s.ctx.Doc, s.ctx.Index, p, s.aperture()); ok {
		e.done(e.apply([]doc.Handle{h}))
		return
	}
	e.begin(subBox)
	s.active.(*selSub).pts = []geom.Vec2{p}
	s.prompt = s.active.(*selSub).prompt()
}

// begin swaps the active command for a selSub collecting the points of kind.
func (e *selEngine) begin(kind subKind) {
	s := e.s
	sub := &selSub{e: e, orig: s.active, kind: kind}
	s.active = sub
	s.prompt = sub.prompt()
}

// done ends the selection prompt (a typed answer to SIngle) when apply asked to.
func (e *selEngine) done(finish bool) {
	if finish {
		e.s.respond(Input{Kind: InputSelection, Selection: slices.Clone(e.s.ctx.Selection.Handles())}, "")
	}
}

// selPrompt is the selection prompt to show after an operation.
func (e *selEngine) selPrompt() *Prompt {
	if e.remove && e.shown != nil {
		return e.shown
	}
	return e.owner
}

// apply adds hs to the selection (or removes them in Remove mode), reports the counts and
// returns whether SIngle ends the prompt now.
func (e *selEngine) apply(hs []doc.Handle) bool {
	ctx := &e.s.ctx
	sel := ctx.Selection
	before := sel.Len()
	e.undo = append(e.undo, slices.Clone(sel.Handles()))
	n := len(hs)
	if e.remove {
		removed := 0
		for _, h := range hs {
			if sel.Has(h) {
				removed++
			}
		}
		sel.Remove(hs...)
		ctx.Say("cmd.select.msg.removed", n, removed, sel.Len())
		if notIn := n - removed; notIn > 0 {
			ctx.SayPlural("cmd.select.msg.not_in_set", notIn)
		}
		return false
	}
	dup := 0
	for _, h := range hs {
		if sel.Has(h) {
			dup++
		}
	}
	sel.Add(hs...)
	switch {
	case dup > 0 && before > 0:
		ctx.Say("cmd.select.msg.found_dup_total", n, dup, sel.Len())
	case dup > 0:
		ctx.Say("cmd.select.msg.found_dup", n, dup)
	case before > 0:
		ctx.Say("cmd.select.msg.found_total", n, sel.Len())
	default:
		ctx.SayPlural("cmd.common.msg.found", n)
	}
	return e.single && n > dup
}

// --- shape sub-prompts ---------------------------------------------------

type subKind int

const (
	subWindow subKind = iota
	subCrossing
	subBox
	subWPoly
	subCPoly
	subFence
)

// selSub stands in for the active command while a corner, polygon or fence is being collected.
type selSub struct {
	e    *selEngine
	orig Command
	kind subKind
	pts  []geom.Vec2
}

func (c *selSub) Name() string           { return c.orig.Name() }
func (c *selSub) Start(*Context) *Prompt { return nil }
func (c *selSub) Cancel(ctx *Context)    { c.e.s.active = c.orig; c.orig.Cancel(ctx) }
func (c *selSub) poly() bool             { return c.kind == subWPoly || c.kind == subCPoly || c.kind == subFence }
func (c *selSub) last() *geom.Vec2       { return &c.pts[len(c.pts)-1] }

func (c *selSub) prompt() *Prompt {
	ctx := &c.e.s.ctx
	n := len(c.pts)
	p := &Prompt{Accepts: InputPoint | InputEnter}
	if n > 0 {
		b := *c.last()
		p.Base = &b
	}
	switch {
	case c.kind == subFence && n == 0:
		p.Message = ctx.T("cmd.select.prompt.first_fence")
	case c.kind == subFence:
		p.Message, p.Keywords, p.Accepts = ctx.T("cmd.select.prompt.next_fence"), []string{selOptUndo}, p.Accepts|InputKeyword
	case c.poly() && n == 0:
		p.Message = ctx.T("cmd.select.prompt.first_polygon")
	case c.poly():
		p.Message, p.Keywords, p.Accepts = ctx.T("cmd.select.prompt.endpoint"), []string{selOptUndo}, p.Accepts|InputKeyword
	case n == 0:
		p.Message = ctx.T("cmd.select.prompt.first_corner")
	default:
		p.Message = ctx.T("cmd.select.prompt.opposite_corner")
	}
	return p
}

func (c *selSub) Input(ctx *Context, in Input) *Prompt {
	switch in.Kind {
	case InputPoint:
		c.pts = append(c.pts, in.Point)
		if !c.poly() && len(c.pts) == 2 {
			return c.finish(ctx, c.collect())
		}
	case InputKeyword:
		if len(c.pts) > 0 {
			c.pts = c.pts[:len(c.pts)-1]
		}
	case InputEnter:
		need := 3
		if c.kind == subFence {
			need = 2
		}
		if len(c.pts) >= need {
			return c.finish(ctx, c.collect())
		}
		return c.finish(ctx, nil)
	}
	return c.prompt()
}

// finish returns to the command's selection prompt after applying hs (nil: nothing was
// selected and nothing is reported, the shape was abandoned).
func (c *selSub) finish(ctx *Context, hs []doc.Handle) *Prompt {
	e := c.e
	e.s.active = c.orig
	if hs == nil {
		return e.selPrompt()
	}
	if e.apply(hs) {
		return c.orig.Input(ctx, Input{Kind: InputSelection, Selection: slices.Clone(ctx.Selection.Handles())})
	}
	return e.selPrompt()
}

// collect runs the shape against the drawing.
func (c *selSub) collect() []doc.Handle {
	s := c.e.s
	d, ix := s.ctx.Doc, s.ctx.Index
	tol := s.ctx.View.PixelSize() / 2
	var hs []doc.Handle
	add := func(box geom.Box2, keep func(doc.Entity) bool) {
		for _, h := range ix.Query(box) {
			if en := d.Entity(h); selectable(d, en, false) && keep(en) {
				hs = append(hs, h)
			}
		}
		slices.Sort(hs)
	}
	switch c.kind {
	case subWindow, subCrossing, subBox:
		a, b := c.pts[0], c.pts[1]
		box := geom.BoxOf(a, b)
		crossing := c.kind == subCrossing || (c.kind == subBox && b.X < a.X)
		add(box, func(en doc.Entity) bool {
			return box.ContainsBox(en.Bounds()) || (crossing && touchesBox(en, box, tol))
		})
	case subWPoly, subCPoly:
		box := polyBox(c.pts)
		add(box, func(en doc.Entity) bool {
			if c.kind == subWPoly {
				return PathsInsidePolygon(entityPaths(en, tol), c.pts)
			}
			return PathsCrossPolygon(entityPaths(en, tol), c.pts)
		})
	case subFence:
		add(polyBox(c.pts).Grow(tol), func(en doc.Entity) bool { return PathsCrossFence(entityPaths(en, tol), c.pts) })
	}
	if hs == nil {
		hs = []doc.Handle{}
	}
	return hs
}

func polyBox(pts []geom.Vec2) geom.Box2 {
	b := geom.EmptyBox()
	for _, p := range pts {
		b = b.Extend(p)
	}
	return b
}

// Preview draws the rectangle, polygon or fence collected so far, ending at the cursor.
func (c *selSub) Preview(ctx *Context, cursor geom.Vec2, dl *render.DisplayList) {
	col := render.Color{R: 120, G: 200, B: 255, A: 255}
	tol := ctx.View.PixelSize() / 4
	line := func(a, b geom.Vec2) { dl.Curve(geom.Line{A: a, B: b}, tol, col, 1) }
	if len(c.pts) == 0 {
		return
	}
	if !c.poly() {
		a := c.pts[0]
		q := []geom.Vec2{a, {X: cursor.X, Y: a.Y}, cursor, {X: a.X, Y: cursor.Y}}
		for i := range q {
			line(q[i], q[(i+1)%4])
		}
		return
	}
	for i := 1; i < len(c.pts); i++ {
		line(c.pts[i-1], c.pts[i])
	}
	line(*c.last(), cursor)
	if c.kind != subFence {
		line(cursor, c.pts[0])
	}
}
