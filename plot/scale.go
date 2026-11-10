package plot

import (
	"strconv"
	"strings"
)

// Scale is a plot scale: Paper millimetres on paper show Drawing millimetres of the drawing (1:50
// is Paper 1, Drawing 50). The zero value fits the area on the paper.
type Scale struct {
	Paper, Drawing float64
}

// Fit is the scale that fits the area on the paper.
var Fit = Scale{}

// Scales are the fixed scales the plot dialog offers.
var Scales = []Scale{{1, 1}, {1, 2}, {1, 5}, {1, 10}, {1, 20}, {1, 50}, {1, 100}, {1, 200}, {1, 500}, {1, 1000}, {2, 1}, {5, 1}, {10, 1}}

// Fixed reports whether s is a fixed scale rather than Fit.
func (s Scale) Fixed() bool { return s.Paper > 0 && s.Drawing > 0 }

// String is "1:50", or "fit".
func (s Scale) String() string {
	if !s.Fixed() {
		return "fit"
	}
	return num1(s.Paper) + ":" + num1(s.Drawing)
}

func num1(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// ParseScale reads "fit", "1:50", "1/50" or "1=50"; both numbers must be positive and finite.
func ParseScale(text string) (Scale, bool) {
	t := strings.TrimSpace(strings.ToLower(text))
	if t == "fit" || t == "" {
		return Fit, true
	}
	a, b, ok := strings.Cut(t, ":")
	if !ok {
		a, b, ok = strings.Cut(t, "/")
	}
	if !ok {
		a, b, ok = strings.Cut(t, "=")
	}
	if !ok {
		return Scale{}, false
	}
	p, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	d, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	s := Scale{p, d}
	if err1 != nil || err2 != nil || !s.Fixed() || p > 1e9 || d > 1e9 {
		return Scale{}, false
	}
	return s, true
}

// UnitMM is the length of one drawing unit of INSUNITS code in millimetres. Unitless (0) and the
// codes without a fixed length read as millimetres, the drawing default.
func UnitMM(code int) float64 {
	switch code {
	case 1: // inches
		return 25.4
	case 2: // feet
		return 304.8
	case 3: // miles
		return 1609344
	case 5: // centimetres
		return 10
	case 6: // metres
		return 1000
	case 7: // kilometres
		return 1e6
	case 8: // microinches
		return 25.4e-6
	case 9: // mils
		return 0.0254
	case 10: // yards
		return 914.4
	case 11: // ångströms
		return 1e-7
	case 12: // nanometres
		return 1e-6
	case 13: // microns
		return 1e-3
	case 14: // decimetres
		return 100
	case 15: // decametres
		return 1e4
	case 16: // hectometres
		return 1e5
	case 21: // US survey feet
		return 1200.0 / 3937 * 1000
	}
	return 1
}
