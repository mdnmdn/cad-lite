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

// Errors returned by ParseInput, shown on the command line.
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
// "d<angle", "@d<angle", a bare number (a distance, an angle in degrees, an
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
	rel := strings.HasPrefix(t, "@")
	body := strings.TrimPrefix(t, "@")
	origin := geom.V(0, 0)
	if rel {
		if base == nil {
			return geom.Vec2{}, true, ErrNoBase
		}
		origin = *base
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
