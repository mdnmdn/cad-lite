package cmdsys

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/i18n"
)

// InputKind is a bit set of the input types a prompt accepts.
type InputKind uint16

const (
	// InputPoint is a model-space point: a click or typed coordinates.
	InputPoint InputKind = 1 << iota
	// InputDistance is a length: a number, or a click measured from Base.
	InputDistance
	// InputAngle is an angle in radians: a number of degrees, or a click
	// measured from Base.
	InputAngle
	// InputKeyword is one of the prompt's keywords.
	InputKeyword
	// InputSelection is a set of entities.
	InputSelection
	// InputText is free text.
	InputText
	// InputInteger is a whole number.
	InputInteger
	// InputEnter is an empty response (Enter, Space or right click).
	InputEnter
)

// Input is one response to a prompt.
type Input struct {
	Kind      InputKind
	Point     geom.Vec2
	Value     float64
	Keyword   string
	Text      string
	Selection []doc.Handle
}

// Prompt is what a command asks for next, as shown on the command line.
type Prompt struct {
	// Message is the text before the input, e.g. "Specify next point".
	Message string
	// Accepts is the set of input kinds the prompt takes.
	Accepts InputKind
	// Keywords are the options, written with their shortcut in capitals
	// ("Close", "Undo", "eXit"); a lower-case keyword's shortcut is its
	// first letter.
	Keywords []string
	// Default is used when the user presses Enter and InputEnter is not
	// accepted.
	Default *Input
	// Base is the rubber-band origin: relative coordinates, ortho, direct
	// distance entry and click distances measure from it.
	Base *geom.Vec2
}

// String renders the prompt as AutoCAD does, in the current language:
// "Specify next point or [Close/Undo] <default>:". Message is expected to be translated already
// by the command; the keywords and the "or" are localized here.
func (p *Prompt) String() string { return p.StringIn(i18n.For(i18n.Current())) }

// StringIn renders the prompt in the language of t (the session passes English for agents).
func (p *Prompt) StringIn(t i18n.Translator) string {
	var b strings.Builder
	b.WriteString(p.Message)
	if len(p.Keywords) > 0 && p.Accepts&InputKeyword != 0 {
		b.WriteString(" " + t.T("input.or") + " [")
		b.WriteString(t.PromptKeywords(p.Keywords))
		b.WriteString("]")
	}
	if p.Default != nil {
		b.WriteString(" <" + formatInput(t, *p.Default) + ">")
	}
	b.WriteString(":")
	return b.String()
}

func formatInput(t i18n.Translator, in Input) string {
	switch in.Kind {
	case InputKeyword:
		return t.KeywordLabel(in.Keyword)
	case InputDistance, InputInteger:
		return strconv.FormatFloat(in.Value, 'f', -1, 64)
	case InputAngle:
		return strconv.FormatFloat(in.Value*180/math.Pi, 'f', -1, 64)
	case InputPoint:
		return FormatPoint(in.Point)
	}
	return in.Text
}

// FormatPoint formats p as "x,y" with up to four decimals.
func FormatPoint(p geom.Vec2) string { return FormatNumber(p.X) + "," + FormatNumber(p.Y) }

// FormatNumber formats v with up to four decimals and no trailing zeros.
func FormatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		s = "0"
	}
	return s
}

// Errors returned by ParseInput, shown on the command line. ErrNoBase is no longer returned by
// ParseInput (relative input before any point measures from the origin, as LASTPOINT starts
// there) and is kept for callers that test for it.
var (
	ErrInvalidPoint = errors.New("invalid point")
	ErrInvalidValue = errors.New("invalid value")
	ErrNoBase       = errors.New("no base point for relative input")
	ErrInvalidInput = errors.New("invalid input")
)

// ParseInput interprets typed text for prompt p. base is the point relative
// input measures from (nil when there is none) and cursor the current
// cursor position, which gives direct distance entry its direction.
//
// Accepted forms: "" (Enter), a keyword or its shortcut, "x,y", "@dx,dy",
// "d<angle", "@d<angle", "#x,y" (absolute), "@" alone (the base), a bare number (a distance, an angle in degrees, an
// integer, or — at a point prompt with a base — a distance along the
// cursor direction), and free text. Keywords match in the current language and in English; a
// leading "_" forces English.
func ParseInput(p *Prompt, text string, base *geom.Vec2, cursor geom.Vec2) (Input, error) {
	return ParseInputIn(i18n.For(i18n.Current()), p, text, base, cursor)
}

// ParseInputIn is ParseInput with keywords matched in the language of tr (English for agents).
func ParseInputIn(tr i18n.Translator, p *Prompt, text string, base *geom.Vec2, cursor geom.Vec2) (Input, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		if p.Accepts&InputEnter != 0 {
			return Input{Kind: InputEnter}, nil
		}
		if p.Default != nil {
			return *p.Default, nil
		}
		return Input{}, ErrInvalidInput
	}
	if p.Accepts&InputKeyword != 0 {
		if kw, ok := tr.MatchKeywords(p.Keywords, t); ok {
			return Input{Kind: InputKeyword, Keyword: kw}, nil
		}
	}
	_, numErr := parseFloat(t)
	numberWanted := numErr == nil && p.Accepts&(InputDistance|InputAngle|InputInteger) != 0
	if p.Accepts&InputPoint != 0 && !numberWanted {
		if pt, ok, err := parsePoint(t, base, cursor); ok || err != nil {
			return Input{Kind: InputPoint, Point: pt}, err
		}
	}
	if v, err := parseFloat(t); err == nil {
		switch {
		case p.Accepts&InputDistance != 0:
			return Input{Kind: InputDistance, Value: v}, nil
		case p.Accepts&InputAngle != 0:
			return Input{Kind: InputAngle, Value: v * math.Pi / 180}, nil
		case p.Accepts&InputInteger != 0 && v == math.Trunc(v):
			return Input{Kind: InputInteger, Value: v}, nil
		}
	}
	if p.Accepts&InputText != 0 {
		return Input{Kind: InputText, Text: t}, nil
	}
	if p.Accepts&InputPoint != 0 {
		return Input{}, ErrInvalidPoint
	}
	return Input{}, ErrInvalidValue
}

// parseFloat is strconv.ParseFloat that rejects NaN and the infinities (nan, inf, 1e999).
func parseFloat(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
		err = ErrInvalidValue
	}
	return v, err
}

func finitePoint(p geom.Vec2) bool {
	return !math.IsNaN(p.X) && !math.IsInf(p.X, 0) && !math.IsNaN(p.Y) && !math.IsInf(p.Y, 0)
}

// parsePoint parses the point forms; ok is false when t is not shaped like
// a point at all, so other interpretations may be tried.
func parsePoint(t string, base *geom.Vec2, cursor geom.Vec2) (geom.Vec2, bool, error) {
	// "#x,y" and "*x,y" force an absolute point. A plain x,y is already absolute here (there is no
	// dynamic input to make it relative), so the prefix is stripped (spec V4); "*" is the WCS form
	// of a UCS, identical in 2D.
	if len(t) > 1 && (t[0] == '#' || t[0] == '*') {
		t = strings.TrimSpace(t[1:])
		base = nil
		if strings.HasPrefix(t, "@") {
			return geom.Vec2{}, true, ErrInvalidPoint
		}
	}
	rel := strings.HasPrefix(t, "@")
	body := strings.TrimPrefix(t, "@")
	origin := geom.V(0, 0)
	if rel {
		// Relative input measures from the base, else from the last point, which starts at the
		// origin: "@1,1" before any point is valid in AutoCAD.
		if base != nil {
			origin = *base
		}
		if body == "" {
			return origin, true, nil
		}
	}
	if d, a, found := strings.Cut(body, "<"); found {
		dist, err1 := parseFloat(strings.TrimSpace(d))
		ang, err2 := parseFloat(strings.TrimSpace(a))
		if err1 != nil || err2 != nil {
			return geom.Vec2{}, true, ErrInvalidPoint
		}
		pt := origin.Polar(dist, ang*math.Pi/180)
		if !finitePoint(pt) {
			return geom.Vec2{}, true, ErrInvalidPoint
		}
		return pt, true, nil
	}
	if xs, ys, found := strings.Cut(body, ","); found {
		x, err1 := parseFloat(strings.TrimSpace(xs))
		y, err2 := parseFloat(strings.TrimSpace(ys))
		if err1 != nil || err2 != nil {
			return geom.Vec2{}, true, ErrInvalidPoint
		}
		pt := origin.Add(geom.V(x, y))
		if !finitePoint(pt) {
			return geom.Vec2{}, true, ErrInvalidPoint
		}
		return pt, true, nil
	}
	if rel {
		return geom.Vec2{}, true, ErrInvalidPoint
	}
	// Direct distance entry: a bare number along the base→cursor direction.
	if base != nil {
		if d, err := parseFloat(body); err == nil {
			dir := cursor.Sub(*base).Norm()
			if dir == (geom.Vec2{}) {
				dir = geom.V(1, 0)
			}
			pt := base.Add(dir.Scale(d))
			if !finitePoint(pt) {
				return geom.Vec2{}, true, ErrInvalidPoint
			}
			return pt, true, nil
		}
	}
	return geom.Vec2{}, false, nil
}

// Text stage orders of the point-input forms (commandline stages of this package).
const (
	// TextOrderTransparent is a ' command typed at a prompt (transparent.go).
	TextOrderTransparent = 15
	// TextOrderPointForms is "<angle" (the angle override) and "@" alone (the last point).
	TextOrderPointForms = 35
)

// cmdLine is the command-line state a Session keeps besides the prompt: the interrupted
// commands under a transparent one, the angle override, the queued scripts.
type cmdLine struct {
	frames []frame
	// angleDeg is the direction in degrees the last "<angle" locked, valid while lockOn.
	angleDeg float64
	lockOn   bool

	pending  []string
	pumping  bool
	cur      string
	restarts int
}

// installCommandLine registers the text stages of the command line on a new session.
func installCommandLine(s *Session) {
	s.AddTextFilter(TextOrderNames, stripNamePrefixes)
	s.AddTextHandler(TextOrderTransparent, transparentStage)
	s.AddTextHandler(TextOrderPointForms, angleOverrideStage)
	s.AddTextFilter(TextOrderPointForms+1, pointFormsFilter)
}

// pointPrompt reports whether the session waits at a prompt that takes a point.
func (s *Session) pointPrompt() bool {
	return s.active != nil && s.prompt != nil && s.prompt.Accepts&InputPoint != 0
}

// angleOverrideStage consumes "<angle" at a point prompt: the next point is then a bare number,
// a distance along that direction from the base (the AutoCAD angle override, spec V5). The
// override lasts for one answer.
func angleOverrideStage(s *Session, text string) bool {
	t := strings.TrimSpace(text)
	if !s.pointPrompt() || len(t) < 2 || t[0] != '<' {
		return false
	}
	a, err := parseFloat(strings.TrimSpace(t[1:]))
	if err != nil {
		return false
	}
	s.print(s.Prompt() + " " + text)
	s.cl.angleDeg, s.cl.lockOn = a, true
	return true
}

// pointFormsFilter rewrites "@" alone to the last point and, while an angle override is on, a
// bare distance to the relative polar form along the locked direction. Any other answer ends
// the override.
func pointFormsFilter(s *Session, text string) string {
	lock := s.cl.lockOn
	s.cl.lockOn = false
	if !s.pointPrompt() {
		return text
	}
	t := strings.TrimSpace(text)
	switch {
	case t == "@":
		p := s.mem.Point()
		return strconv.FormatFloat(p.X, 'g', -1, 64) + "," + strconv.FormatFloat(p.Y, 'g', -1, 64)
	case lock:
		if d, err := parseFloat(t); err == nil {
			return "@" + strconv.FormatFloat(d, 'g', -1, 64) + "<" + strconv.FormatFloat(s.cl.angleDeg, 'g', -1, 64)
		}
	}
	return text
}

// enterEnds handles Enter at a prompt that takes no Enter and has no default: the command ends
// quietly (AutoCAD: a required point or distance answered with Enter just stops, spec G5-1, V3),
// keeping what it committed. It reports whether it handled err.
func (s *Session) enterEnds(err error, text string) bool {
	if !errors.Is(err, ErrInvalidInput) || strings.TrimSpace(text) != "" {
		return false
	}
	s.print(s.Prompt())
	s.guard(func() { s.active.Cancel(&s.ctx) })
	s.finish()
	return true
}
