package props

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// Parse reads text typed into row p as a Value for Apply. Numbers accept "." or "," as the
// decimal separator; a Length also reads the LUNITS forms (feet and inches 1'-3 1/2", fractions
// 15 1/2) and an Angle the AUNITS forms (45, 45d30', 50g, 0.8r, N 45d E), measured from the X
// axis. A colour takes a number, a name, BYLAYER or BYBLOCK; a lineweight BYLAYER, BYBLOCK,
// DEFAULT or millimetres; a choice its label or value, case-insensitive. The "vertex" selector
// reads the 1-based number it displays and returns it 0-based. Bounds and Positive are checked.
func Parse(h Host, p Property, text string) (Value, error) {
	c := newEnv(h.Drawing(), h.Vars(), 0)
	t := strings.TrimSpace(text)
	bad := func() (Value, error) { return Value{}, newError(Invalid, "ui.props.err.number") }
	switch p.Kind {
	case KindNumber:
		f, ok := parseNumber(t)
		if !ok {
			return bad()
		}
		return Value{Num: f}, checkBounds(p, f)
	case KindLength:
		f, ok := parseNumber(t)
		if !ok {
			f, ok = parseFeetInches(t)
		}
		if !ok {
			return bad()
		}
		return Value{Num: f}, checkBounds(p, f)
	case KindAngle:
		f, ok := cmdsys.ParseAngleText(decimalPoint(t), c.intVar("AUNITS", cmdsys.AUnitsDegrees))
		if !ok {
			return bad()
		}
		return Value{Num: f}, checkBounds(p, f)
	case KindInt:
		n, err := strconv.Atoi(t)
		if err != nil {
			return bad()
		}
		if p.Selector {
			n--
		}
		if p.Selector && n < 0 {
			return bad()
		}
		return Value{Int: n}, checkBounds(p, float64(n))
	case KindBool:
		switch strings.ToLower(t) {
		case "1", "yes", "y", "on", "true", "sì", "si", "s":
			return BoolValue(true), nil
		case "0", "no", "n", "off", "false":
			return BoolValue(false), nil
		}
		return Value{}, ErrInvalid
	case KindText:
		if !validContent(text, false) {
			return Value{}, newError(Invalid, "ui.props.err.text_line")
		}
		return StrValue(text), nil
	case KindMultiText:
		return StrValue(text), nil
	case KindColor:
		if n, ok := cmdsys.ParseColor(t); ok {
			return IntValue(n), nil
		}
	case KindLineweight:
		if w, ok := cmdsys.ParseLineweight(t); ok {
			return IntValue(int(w)), nil
		}
	case KindLinetype:
		if n, ok := cmdsys.CanonicalLinetype(c.d, t); ok {
			if n == "" {
				n = doc.LinetypeByLayer
			}
			return StrValue(n), nil
		}
	}
	for _, o := range p.Options {
		if strings.EqualFold(o.Label, t) || o.Value.Str != "" && strings.EqualFold(o.Value.Str, t) {
			return o.Value, nil
		}
	}
	switch p.Kind {
	case KindEnum, KindColor, KindLineweight:
		return Value{}, newError(Invalid, "ui.props.err.choice", t)
	}
	v, err := c.canon(p, StrValue(t))
	if err != nil {
		return Value{}, newError(Invalid, "ui.props.err.choice", t)
	}
	return v, nil
}

// decimalPoint turns a decimal comma into a point when the text has no point already.
func decimalPoint(t string) string {
	if strings.Contains(t, ",") && !strings.Contains(t, ".") {
		return strings.ReplaceAll(t, ",", ".")
	}
	return t
}

// parseNumber reads a finite number with "." or "," as the decimal separator.
func parseNumber(t string) (float64, bool) {
	f, err := strconv.ParseFloat(decimalPoint(t), 64)
	return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
}

// feetInchesRe matches 5', 5'-3", 5'3-1/2", 3 1/2", 3-1/2, 1/2, 15 1/2 (inches unless the feet
// mark is given).
var feetInchesRe = regexp.MustCompile(`^(-)?(?:(\d+(?:[.,]\d+)?)')?\s*-?\s*(\d+(?:[.,]\d+)?)?(?:[\s-]*(\d+)/(\d+))?\s*"?$`)

// parseFeetInches reads an architectural, engineering or fractional length in inches.
func parseFeetInches(t string) (float64, bool) {
	m := feetInchesRe.FindStringSubmatch(t)
	if m == nil || m[2] == "" && m[3] == "" && m[4] == "" {
		return 0, false
	}
	num := func(s string) float64 { f, _ := strconv.ParseFloat(decimalPoint(s), 64); return f }
	v := num(m[2])*12 + num(m[3])
	if m[4] != "" {
		den := num(m[5])
		if den == 0 {
			return 0, false
		}
		v += num(m[4]) / den
	}
	if m[1] != "" {
		v = -v
	}
	return v, true
}
