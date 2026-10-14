package doc

import "math"

// Limits on the size fields of TEXT and MTEXT. A file may hold any finite number; past these the
// layout's products overflow (a height of 1e300 times a width factor of 1e300 is infinite) or the
// slant is a near-vertical smear. The width factor and obliquing angle are AutoCAD's own limits
// (STYLE takes 0.01 to 100 and -85° to 85°), as is the MTEXT line spacing factor (0.25 to 4).
const (
	// TextMaxSize is the largest text height, and MTEXT box width, kept: far beyond any drawing.
	TextMaxSize = 1e15
	// TextMinWidthFactor and TextMaxWidthFactor bound a TEXT width factor.
	TextMinWidthFactor, TextMaxWidthFactor = 0.01, 100
	// MTextMinSpacing and MTextMaxSpacing bound an MTEXT line spacing factor.
	MTextMinSpacing, MTextMaxSpacing = 0.25, 4
)

// TextMaxOblique is the largest obliquing angle magnitude kept, 85 degrees in radians.
const TextMaxOblique = 85 * math.Pi / 180

// clampHeight maps a text height into [0, TextMaxSize]; NaN and negatives are 0.
func clampHeight(h float64) float64 {
	if !(h > 0) {
		return 0
	}
	return math.Min(h, TextMaxSize)
}

// clampWidthFactor maps a width factor into AutoCAD's range; an unset or invalid one is 1.
func clampWidthFactor(wf float64) float64 {
	if !(wf > 0) || math.IsInf(wf, 0) {
		return 1
	}
	return math.Max(TextMinWidthFactor, math.Min(wf, TextMaxWidthFactor))
}

// clampOblique maps an obliquing angle into ±TextMaxOblique; NaN is 0.
func clampOblique(a float64) float64 {
	if math.IsNaN(a) {
		return 0
	}
	return math.Max(-TextMaxOblique, math.Min(a, TextMaxOblique))
}

// finiteAngle returns a, or 0 when it is NaN or infinite.
func finiteAngle(a float64) float64 {
	if math.IsNaN(a) || math.IsInf(a, 0) {
		return 0
	}
	return a
}

// Sanitize brings the height, width factor, obliquing angle and rotation into the ranges the
// layout supports (TextMaxSize, the width factor and oblique limits). File readers call it, so
// a damaged or hostile file cannot give the text an infinite box; the layout applies the same
// limits to whatever a caller sets.
func (e *Text) Sanitize() {
	e.Height = clampHeight(e.Height)
	if e.WidthFactor != 0 {
		e.WidthFactor = clampWidthFactor(e.WidthFactor)
	}
	e.Oblique = clampOblique(e.Oblique)
	e.Rotation = finiteAngle(e.Rotation)
}

// Sanitize brings the height, box width, line spacing factor and rotation into the ranges the
// layout supports; see Text.Sanitize.
func (e *MText) Sanitize() {
	e.Height = clampHeight(e.Height)
	if !(e.Width > 0) {
		e.Width = 0
	}
	e.Width = math.Min(e.Width, TextMaxSize)
	if e.LineSpacing != 0 {
		e.LineSpacing = clampSpacing(e.LineSpacing)
	}
	e.Rotation = finiteAngle(e.Rotation)
}

// clampSpacing maps a line spacing factor into AutoCAD's range; an unset or invalid one is 1.
func clampSpacing(ls float64) float64 {
	if !(ls > 0) || math.IsInf(ls, 0) {
		return 1
	}
	return math.Max(MTextMinSpacing, math.Min(ls, MTextMaxSpacing))
}
