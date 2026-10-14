package cmdsys

import (
	"math"
	"strconv"
	"strings"
)

// Linear unit formats (LUNITS) and angular unit formats (AUNITS) as AutoCAD numbers them.
const (
	LUnitsScientific    = 1
	LUnitsDecimal       = 2
	LUnitsEngineering   = 3
	LUnitsArchitectural = 4
	LUnitsFractional    = 5

	AUnitsDegrees   = 0
	AUnitsDMS       = 1
	AUnitsGrads     = 2
	AUnitsRadians   = 3
	AUnitsSurveyors = 4
)

// FormatNumberPrec formats v with up to prec decimals and no trailing zeros ("-0" prints "0").
func FormatNumberPrec(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// FormatFixed formats v with exactly prec decimals, trailing zeros kept ("5.0000"); "-0.0000"
// prints without the sign.
func FormatFixed(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.HasPrefix(s, "-") && strings.Trim(s, "-0.") == "" {
		s = s[1:]
	}
	return s
}

// FormatLength formats a length (drawing units) in the LUNITS format with LUPREC precision:
// scientific 1.5500E+01, decimal 15.5000, engineering 1'-3.5000" and architectural 1'-3 1/2"
// (both assume inches), fractional 15 1/2. For architectural and fractional the precision is the
// denominator exponent: 4 means sixteenths.
func FormatLength(v float64, lunits, prec int) string {
	prec = clampPrec(prec)
	switch lunits {
	case LUnitsScientific:
		return strconv.FormatFloat(v, 'E', prec, 64)
	case LUnitsEngineering:
		return formatFeetInches(v, prec, false)
	case LUnitsArchitectural:
		return formatFeetInches(v, prec, true)
	case LUnitsFractional:
		return formatFraction(v, prec)
	}
	return FormatFixed(v, prec)
}

func sign(neg bool) string {
	if neg {
		return "-"
	}
	return ""
}

// fraction returns whole, numerator and denominator of |v| rounded to 1/2^prec, the fraction in
// lowest terms (num 0 when it vanishes).
func fraction(v float64, prec int) (whole, num, den int64) {
	den = 1 << uint(prec)
	n := int64(math.Round(math.Abs(v) * float64(den)))
	whole, num = n/den, n%den
	for num != 0 && num%2 == 0 {
		num, den = num/2, den/2
	}
	if num == 0 {
		den = 1
	}
	return
}

func formatFraction(v float64, prec int) string {
	whole, num, den := fraction(v, prec)
	neg := v < 0 && (whole != 0 || num != 0)
	switch {
	case num == 0:
		return sign(neg) + strconv.FormatInt(whole, 10)
	case whole == 0:
		return sign(neg) + strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)
	}
	return sign(neg) + strconv.FormatInt(whole, 10) + " " + strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)
}

func formatFeetInches(v float64, prec int, arch bool) string {
	a := math.Abs(v)
	feet := math.Floor(a / 12)
	in := a - feet*12
	var body string
	neg := v < 0
	if arch {
		whole, num, den := fraction(in, prec)
		if whole >= 12 {
			feet, whole = feet+1, whole-12
		}
		body = strconv.FormatInt(whole, 10)
		if num != 0 {
			body += " " + strconv.FormatInt(num, 10) + "/" + strconv.FormatInt(den, 10)
		}
		neg = neg && (feet != 0 || whole != 0 || num != 0)
	} else {
		body = FormatFixed(in, prec)
		if f, _ := strconv.ParseFloat(body, 64); f >= 12 {
			feet++
			body = FormatFixed(0, prec)
		}
		neg = neg && (feet != 0 || strings.Trim(body, "0.") != "")
	}
	return sign(neg) + strconv.FormatFloat(feet, 'f', 0, 64) + "'-" + body + `"`
}

// FormatAngle formats an angle given in radians in the AUNITS format with AUPREC decimals,
// without applying ANGBASE or ANGDIR: decimal degrees 53.13, degrees-minutes-seconds 53d7'48",
// grads 59.03g, radians 0.9273r, surveyor's units N 37d E. A negative angle keeps its sign except
// in surveyor's units, which are always a bearing.
func FormatAngle(rad float64, aunits, prec int) string {
	prec = clampPrec(prec)
	deg := rad * 180 / math.Pi
	switch aunits {
	case AUnitsDMS:
		return formatDMS(deg, prec)
	case AUnitsGrads:
		return FormatFixed(deg*10/9, prec) + "g"
	case AUnitsRadians:
		return FormatFixed(rad, prec) + "r"
	case AUnitsSurveyors:
		return formatBearing(deg, prec)
	}
	s := FormatFixed(deg, prec)
	if s == FormatFixed(360, prec) {
		s = FormatFixed(0, prec)
	}
	return s
}

// formatDMS prints degrees as d, d mm', d mm' ss" or with decimal seconds as prec grows:
// 0 degrees only, 1-2 minutes, 3 seconds, 4-8 seconds with prec-3 decimals.
func formatDMS(deg float64, prec int) string {
	neg := deg < 0
	deg = math.Abs(deg)
	dec := max(prec-3, 0)
	scale := math.Pow10(dec)
	per := int64(60 * scale) // units per minute
	unit := int64(1)         // rounding step in those units
	switch {
	case prec == 0:
		unit = per * 60
	case prec <= 2:
		unit = per
	}
	tu := int64(math.Round(deg*3600*scale/float64(unit))) * unit
	tu %= per * 60 * 360
	d, rest := tu/(per*60), tu%(per*60)
	m, s := rest/per, rest%per
	out := strconv.FormatInt(d, 10) + "d"
	if prec >= 1 {
		out += strconv.FormatInt(m, 10) + "'"
	}
	if prec >= 3 {
		out += strconv.FormatFloat(float64(s)/scale, 'f', dec, 64) + `"`
	}
	if neg && tu != 0 {
		out = "-" + out
	}
	return out
}

// formatBearing prints a direction as a surveyor's bearing: N 37d E, S 10d15' W, or the bare
// cardinal letter on an axis.
func formatBearing(deg float64, prec int) string {
	deg = math.Mod(math.Mod(deg, 360)+360, 360)
	switch deg {
	case 0:
		return "E"
	case 90:
		return "N"
	case 180:
		return "W"
	case 270:
		return "S"
	}
	var ns, ew string
	var a float64
	switch {
	case deg < 90:
		ns, ew, a = "N", "E", 90-deg
	case deg < 180:
		ns, ew, a = "N", "W", deg-90
	case deg < 270:
		ns, ew, a = "S", "W", 270-deg
	default:
		ns, ew, a = "S", "E", deg-270
	}
	return ns + " " + formatDMS(a, prec) + " " + ew
}

// Fmt formats a length or other real for output in the drawing's LUNITS format with LUPREC
// decimals: decimal prints trailing zeros as AutoCAD does ("5.0000").
func (c *Context) Fmt(v float64) string {
	return FormatLength(v, c.vars.Int("LUNITS"), c.vars.Int("LUPREC"))
}

// AngleBase returns ANGBASE in radians: the direction of angle 0.
func (c *Context) AngleBase() float64 { return c.vars.Real("ANGBASE") * math.Pi / 180 }

// AngleClockwise reports whether ANGDIR makes positive angles clockwise.
func (c *Context) AngleClockwise() bool { return c.vars.Int("ANGDIR") == 1 }

// FmtAngle formats a direction given in radians from the X axis, as the user reads it: measured
// from ANGBASE in the ANGDIR sense, in the AUNITS format with AUPREC decimals (default degrees,
// no decimals). Use FmtAngleRel for an angle between two things (an arc's sweep).
func (c *Context) FmtAngle(rad float64) string {
	a := rad - c.AngleBase()
	if c.AngleClockwise() {
		a = -a
	}
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return FormatAngle(a, c.vars.Int("AUNITS"), c.vars.Int("AUPREC"))
}

// FmtAngleRel formats an angle that is not a direction, so ANGBASE and ANGDIR do not apply.
func (c *Context) FmtAngleRel(rad float64) string {
	return FormatAngle(rad, c.vars.Int("AUNITS"), c.vars.Int("AUPREC"))
}

func clampPrec(p int) int { return max(0, min(p, 8)) }
