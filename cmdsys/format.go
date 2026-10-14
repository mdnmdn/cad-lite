package cmdsys

import (
	"math"
	"strconv"
	"strings"
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

// Fmt formats a length or other real for output with the drawing's LUPREC decimals (default 4,
// trailing zeros trimmed, exactly what FormatNumber prints). The units work (LUNITS, fixed
// decimals, architectural) extends this one function.
func (c *Context) Fmt(v float64) string {
	return FormatNumberPrec(v, clampPrec(c.vars.Int("LUPREC")))
}

// FmtAngle formats an angle given in radians as degrees with the AUPREC decimals (default 0
// means "as many as needed up to 4", the form the commands print today). Degrees only: the
// other AUNITS formats belong to the units work.
func (c *Context) FmtAngle(rad float64) string {
	prec := clampPrec(c.vars.Int("AUPREC"))
	if prec == 0 {
		prec = 4
	}
	return FormatNumberPrec(rad*180/math.Pi, prec)
}

func clampPrec(p int) int { return max(0, min(p, 8)) }
