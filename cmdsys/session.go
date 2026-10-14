package cmdsys

import (
	"errors"
	"math"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/i18n"
	"go-draw/render"
	"go-draw/spatial"
)

// Session runs commands against one drawing: it owns the active command and
// prompt, the selection, snap and ortho settings, and the command history.
// The UI feeds it raw input (typed text, clicks, cursor moves, box drags)
// and reads back the prompt, the history and the overlay.
type Session struct {
	ctx      Context
	registry *Registry

	// Snaps are the enabled object snap modes; SnapOn toggles them all (F3).
	Snaps  SnapMode
	SnapOn bool
	// Ortho constrains rubber-band points to horizontal or vertical (F8).
	Ortho bool
	// Aperture is the pick and snap radius in logical pixels.
	Aperture float64

	active  Command
	prompt  *Prompt
	last    string
	history []string
	printed int

	// agentCaller is set while an AI agent drives the session; HumanOnly commands are refused.
	agentCaller bool
	// translator, when set, overrides the global language for everything the session prints.
	translator *i18n.Translator

	// dispatching is set while a command's Start or Input runs.
	dispatching bool

	cursor    geom.Vec2
	snap      SnapHit
	snapped   bool
	lastPoint *geom.Vec2

	onDrawing []func(*doc.Drawing)
	onPath    []func(string)
	filePath  string
}

// HistoryLimit caps the number of history lines kept.
const HistoryLimit = 200

// NewSession returns a session over d showing view, with commands from reg.
func NewSession(d *doc.Drawing, view *render.View, reg *Registry) *Session {
	s := &Session{registry: reg, Snaps: DefaultSnaps, SnapOn: true, Aperture: 8}
	s.ctx = Context{View: view, Selection: &Selection{}, out: s.print, tr: s.Translator, replace: s.SetDrawing, setPath: s.SetFilePath}
	s.SetDrawing(d)
	return s
}

// SetAgentCaller marks the session as driven by an AI agent (true) or a human (false). While it
// is on, commands implementing HumanOnly are refused with an error line, and everything the
// session prints, prompts and matches is English whatever the UI language, so agents always see
// the language the tool documentation is written in.
func (s *Session) SetAgentCaller(on bool) { s.agentCaller = on }

// SetTranslator pins the language of the session's output and keyword matching to t, whatever the
// global language; SetTranslator(nil) goes back to following i18n.Current(). An agent caller is
// English regardless.
func (s *Session) SetTranslator(t *i18n.Translator) { s.translator = t }

// Translator returns the translator the session uses now: English for an agent caller, the
// pinned translator when there is one, else the current global language (so a language change in
// the settings panel applies to the next line printed).
func (s *Session) Translator() i18n.Translator {
	switch {
	case s.agentCaller:
		return i18n.For(i18n.English)
	case s.translator != nil:
		return *s.translator
	}
	return i18n.For(i18n.Current())
}

func humanOnly(c Command) bool {
	h, ok := c.(HumanOnly)
	return ok && h.HumanOnly()
}

// guard runs f and recovers a panic: the open transaction is aborted, the active command ends
// and an error line is printed, so the session stays usable. It reports whether f returned.
func (s *Session) guard(f func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.dispatching = false
			name := s.Translator().T("session.fallback_command_name")
			if s.active != nil {
				name = s.active.Name()
			}
			if s.ctx.txn != nil {
				s.ctx.txn.Abort()
				s.ctx.txn = nil
			}
			if s.ctx.Doc != nil {
				s.ctx.Doc.AbortOpen()
			}
			s.active, s.prompt = nil, nil
			s.print(s.Translator().T("session.internal_error", name, r))
			ok = false
		}
	}()
	f()
	return true
}

// SetFiles installs the file handler OPEN and SAVE use.
func (s *Session) SetFiles(f FileHandler) { s.ctx.Files = f }

// OnDrawing registers f to be called whenever the session's drawing is
// replaced (OPEN, NEW), including the replacement's first change listener
// wiring.
func (s *Session) OnDrawing(f func(*doc.Drawing)) { s.onDrawing = append(s.onDrawing, f) }

// FilePath returns the path of the last file OPENed or SAVEd, or "".
func (s *Session) FilePath() string { return s.filePath }

// SetFilePath records path as the drawing's file (OPEN and SAVE call it) and notifies the
// OnFilePath listeners. The host calls it for a file opened at startup.
func (s *Session) SetFilePath(path string) {
	s.filePath = path
	for _, f := range s.onPath {
		f(path)
	}
}

// OnFilePath registers f to be called whenever the drawing's file path is set.
func (s *Session) OnFilePath(f func(string)) { s.onPath = append(s.onPath, f) }

// SetAgentHandler installs the host's AI agent entry point, which the AI command calls with the
// prompt text, or with "" to toggle the agent panel. Without a handler the command reports that
// no agent is available.
func (s *Session) SetAgentHandler(f func(prompt string)) { s.ctx.agent = f }

// SetDrawing replaces the document, cancelling any command and rebuilding
// the spatial index.
func (s *Session) SetDrawing(d *doc.Drawing) {
	if s.active != nil && !s.dispatching {
		s.Escape()
	}
	s.ctx.Doc = d
	s.ctx.Index = spatial.New()
	s.ctx.Index.Build(d)
	s.ctx.Selection.Clear()
	ix := s.ctx.Index
	sel := s.ctx.Selection
	d.OnChange(func(cs doc.ChangeSet) {
		ix.Apply(d, cs)
		sel.Prune(d)
	})
	for _, f := range s.onDrawing {
		f(d)
	}
}

// Drawing returns the open document.
func (s *Session) Drawing() *doc.Drawing { return s.ctx.Doc }

// Index returns the spatial index of the open document.
func (s *Session) Index() *spatial.Index { return s.ctx.Index }

// Selection returns the selection set.
func (s *Session) Selection() *Selection { return s.ctx.Selection }

// View returns the view the session measures apertures in.
func (s *Session) View() *render.View { return s.ctx.View }

// Registry returns the command registry.
func (s *Session) Registry() *Registry { return s.registry }

// Active returns the running command's name, or "".
func (s *Session) Active() string {
	if s.active == nil {
		return ""
	}
	return s.active.Name()
}

// Prompt returns the current prompt text: the active command's prompt, or
// "Command:" when idle.
func (s *Session) Prompt() string {
	t := s.Translator()
	if s.prompt == nil {
		return t.T("session.command_prompt")
	}
	return s.prompt.StringIn(t)
}

// CurrentPrompt returns the active prompt, or nil when idle.
func (s *Session) CurrentPrompt() *Prompt { return s.prompt }

// History returns the command history, oldest first.
func (s *Session) History() []string { return s.history }

// HistoryTotal returns how many lines have ever been printed to the history.
// It only grows, unlike History, which is capped at HistoryLimit lines, so
// the difference of two readings is the number of lines printed between them.
func (s *Session) HistoryTotal() int { return s.printed }

func (s *Session) print(line string) {
	s.printed++
	s.history = append(s.history, line)
	if n := len(s.history) - HistoryLimit; n > 0 {
		s.history = s.history[n:]
	}
}

// Cursor returns the effective cursor: snapped, or ortho-constrained.
func (s *Session) Cursor() geom.Vec2 { return s.cursor }

// Snap returns the snap the cursor is on, if any.
func (s *Session) Snap() (SnapHit, bool) { return s.snap, s.snapped }

// Run starts the named command, cancelling any running one.
func (s *Session) Run(name string) bool {
	cmd, ok := s.registry.Lookup(name)
	if !ok {
		s.print(s.Translator().T("session.unknown_command", strings.ToUpper(strings.TrimSpace(name))))
		return false
	}
	if s.agentCaller && humanOnly(cmd) {
		s.print(s.Translator().T("session.agent_forbidden", cmd.Name()))
		return false
	}
	if s.active != nil {
		s.Escape()
	}
	t := s.Translator()
	s.print(t.T("session.command_echo", strings.ToUpper(t.CommandName(cmd.Name()))))
	s.last = cmd.Name()
	s.active = cmd
	var p *Prompt
	s.dispatching = true
	ok = s.guard(func() { p = cmd.Start(&s.ctx) })
	s.dispatching = false
	if ok {
		s.next(p)
	}
	return true
}

// next installs p as the active prompt, finishing the command when p is nil.
func (s *Session) next(p *Prompt) {
	if p == nil {
		s.finish()
		return
	}
	s.prompt = p
}

func (s *Session) finish() {
	if s.ctx.txn != nil {
		s.ctx.txn.Commit()
		s.ctx.txn = nil
	}
	s.active, s.prompt = nil, nil
}

// Escape cancels the running command, keeping what it already committed to
// its transaction (as AutoCAD keeps the segments of a cancelled LINE).
func (s *Session) Escape() {
	if s.active == nil {
		if s.ctx.Selection.Len() > 0 {
			s.ctx.Selection.Clear()
		}
		return
	}
	s.guard(func() { s.active.Cancel(&s.ctx) })
	s.print(s.Translator().T("session.cancel"))
	s.finish()
}

// Submit handles a line typed on the command line. When idle, text is a
// command name (Enter alone repeats the last command); otherwise it is a
// response to the prompt.
func (s *Session) Submit(text string) {
	if s.active == nil {
		t := strings.TrimSpace(text)
		if t == "" {
			if s.last != "" {
				s.Run(s.last)
			}
			return
		}
		s.Run(t)
		return
	}
	if s.agentCaller && humanOnly(s.active) {
		s.print(s.Translator().T("session.agent_forbidden", s.active.Name()))
		return
	}
	p := s.prompt
	if p.Accepts&InputSelection != 0 && strings.TrimSpace(text) == "" {
		s.respond(Input{Kind: InputSelection, Selection: append([]doc.Handle(nil), s.ctx.Selection.Handles()...)}, text)
		return
	}
	in, err := ParseInputIn(s.Translator(), p, text, s.base(), s.cursor)
	if err != nil {
		s.print(s.Prompt() + " " + text)
		s.print(s.inputError(err))
		return
	}
	s.respond(in, text)
}

// Script runs AutoCAD script text: tokens separated by single spaces or
// newlines, each submitted in turn; an empty token (two separators in a
// row) is Enter.
func (s *Session) Script(text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", " ")
	for _, tok := range strings.Split(text, " ") {
		s.Submit(tok)
	}
}

func (s *Session) respond(in Input, echo string) {
	s.print(s.Prompt() + " " + echo)
	if in.Kind == InputPoint {
		p := in.Point
		s.lastPoint = &p
	}
	var p *Prompt
	s.dispatching = true
	ok := s.guard(func() { p = s.active.Input(&s.ctx, in) })
	s.dispatching = false
	if ok {
		s.next(p)
	}
}

// base returns the point relative input measures from: the prompt's base,
// else the last point entered.
func (s *Session) base() *geom.Vec2 {
	if s.prompt != nil && s.prompt.Base != nil {
		return s.prompt.Base
	}
	return s.lastPoint
}

// aperture returns the pick/snap radius in model units.
func (s *Session) aperture() float64 { return s.Aperture * s.ctx.View.PixelSize() }

// Hover updates the cursor from a raw model point: object snap first, then
// ortho relative to the prompt's base.
func (s *Session) Hover(p geom.Vec2) {
	s.snapped = false
	if s.SnapOn && s.wantsPoint() {
		if hit, ok := FindSnap(s.ctx.Doc, s.ctx.Index, p, s.aperture(), s.Snaps); ok {
			s.snap, s.snapped = hit, true
			s.cursor = hit.Point
			return
		}
	}
	s.cursor = s.constrain(p)
}

func (s *Session) wantsPoint() bool {
	return s.prompt != nil && s.prompt.Accepts&(InputPoint|InputDistance|InputAngle) != 0
}

func (s *Session) constrain(p geom.Vec2) geom.Vec2 {
	if !s.Ortho || s.prompt == nil || s.prompt.Base == nil {
		return p
	}
	b := *s.prompt.Base
	d := p.Sub(b)
	if math.Abs(d.X) >= math.Abs(d.Y) {
		return geom.V(p.X, b.Y)
	}
	return geom.V(b.X, p.Y)
}

// Click handles a primary click at raw model point p. At a point, distance
// or angle prompt it answers with the snapped cursor; at a selection prompt
// or when idle it picks the entity under the cursor (toggling it off when
// remove is set).
func (s *Session) Click(p geom.Vec2, remove bool) {
	s.Hover(p)
	if s.prompt != nil {
		pr := s.prompt
		switch {
		case pr.Accepts&InputPoint != 0:
			s.respond(Input{Kind: InputPoint, Point: s.cursor}, FormatPoint(s.cursor))
			return
		case pr.Accepts&InputDistance != 0 && pr.Base != nil:
			v := s.cursor.Dist(*pr.Base)
			s.respond(Input{Kind: InputDistance, Value: v, Point: s.cursor}, FormatNumber(v))
			return
		case pr.Accepts&InputAngle != 0 && pr.Base != nil:
			v := s.cursor.Sub(*pr.Base).Angle()
			s.respond(Input{Kind: InputAngle, Value: v, Point: s.cursor}, FormatNumber(v*180/math.Pi))
			return
		case pr.Accepts&InputSelection == 0:
			return
		}
	}
	h, ok := Pick(s.ctx.Doc, s.ctx.Index, p, s.aperture())
	switch {
	case !ok:
	case remove:
		s.ctx.Selection.Remove(h)
	default:
		s.ctx.Selection.Add(h)
	}
}

// SelectBox handles a box drag from a to b (raw model points): a window or
// crossing selection when idle or at a selection prompt.
func (s *Session) SelectBox(a, b geom.Vec2, remove bool) {
	if s.prompt != nil && s.prompt.Accepts&InputSelection == 0 {
		return
	}
	hs := SelectBox(s.ctx.Doc, s.ctx.Index, a, b, s.ctx.View.PixelSize()/2)
	if remove {
		s.ctx.Selection.Remove(hs...)
	} else {
		s.ctx.Selection.Add(hs...)
	}
}

// Overlay appends the active command's rubber-band preview for the current
// cursor.
func (s *Session) Overlay(dl *render.DisplayList) {
	if s.active != nil && s.prompt != nil {
		s.guard(func() { s.active.Preview(&s.ctx, s.cursor, dl) })
	}
}

// inputError maps a ParseInput error to its localized command-line sentence.
func (s *Session) inputError(err error) string {
	t := s.Translator()
	switch {
	case errors.Is(err, ErrInvalidPoint):
		return t.T("input.err.invalid_point")
	case errors.Is(err, ErrInvalidValue):
		return t.T("input.err.invalid_value")
	case errors.Is(err, ErrNoBase):
		return t.T("input.err.no_base")
	}
	return t.T("input.err.invalid_input")
}
