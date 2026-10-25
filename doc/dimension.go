package doc

import (
	"math"
	"strconv"
	"strings"

	"cad-lite/geom"
)

// DimKind is the dimension type: DXF group 70's low bits for the linear kinds, and an internal
// value for the arc length dimension, which DXF stores as its own class (ARC_DIMENSION).
type DimKind int

// The dimension kinds.
const (
	// DimRotated is a linear dimension measured along Rotation: horizontal, vertical or rotated
	// (DIMLINEAR, DXF type 0).
	DimRotated DimKind = 0
	// DimAligned is a linear dimension parallel to its extension line origins (DIMALIGNED, DXF
	// type 1).
	DimAligned DimKind = 1
	// DimArcLength is an arc length dimension (DIMARC, DXF class ARC_DIMENSION).
	DimArcLength DimKind = 8
)

// DIMARCSYM values: where an arc length dimension draws its arc symbol.
const (
	// DimArcSymBefore draws the symbol before the text (the default).
	DimArcSymBefore = 0
	// DimArcSymAbove draws the symbol above the text.
	DimArcSymAbove = 1
	// DimArcSymNone draws no symbol.
	DimArcSymNone = 2
)

// DimStyle holds the dimension variables a dimension is drawn with, captured from the DIM*
// system variables when it is created (and from the DIMSTYLE table and ACAD DSTYLE xdata when it
// is read). It is also a DIMSTYLE table record (Drawing.DimStyles). It is comparable with ==.
type DimStyle struct {
	// Name is the DIMSTYLE name (DXF group 3).
	Name string
	// Scale is DIMSCALE, the factor applied to every size below; 0 or less means 1.
	Scale float64
	// ArrowSize is DIMASZ, the arrowhead length.
	ArrowSize float64
	// TextHeight is DIMTXT, the capital height of the text.
	TextHeight float64
	// ExtOffset is DIMEXO, the gap from an origin to the start of its extension line.
	ExtOffset float64
	// ExtExtend is DIMEXE, how far an extension line runs past the dimension line.
	ExtExtend float64
	// Gap is DIMGAP, the space between the text and the dimension line.
	Gap float64
	// Decimals is DIMDEC, the number of decimal places (0 to 8).
	Decimals int
	// DecSep is DIMDSEP, the decimal separator; "" means ".".
	DecSep string
	// ZeroSupp is DIMZIN: bit 4 suppresses leading zeros, bit 8 trailing zeros.
	ZeroSupp int
	// TextVertical is DIMTAD, where the text sits across the dimension line: DimTextCentered,
	// DimTextAbove, DimTextOutside, DimTextJIS or DimTextBelow. With horizontal text on a line that
	// is not horizontal, the text is centred whatever it says, as in AutoCAD.
	TextVertical int
	// TextJust is DIMJUST, where the text sits along the line: DimJustCenter... DimJustOverExt2.
	TextJust int
	// TextInsideHorizontal is DIMTIH: text between the extension lines is horizontal rather than
	// aligned with the dimension line.
	TextInsideHorizontal bool
	// TextOutsideHorizontal is DIMTOH: text outside the extension lines is horizontal.
	TextOutsideHorizontal bool
	// TextStyle is DIMTXSTY, the text style name; "" is Standard. Every style draws with the
	// built-in font (see Text).
	TextStyle string
	// ArrowBlock is DIMBLK, the arrowhead of both ends: "" closed filled, or an AutoCAD arrow block
	// name (ArrowKind.BlockName, "_ArchTick") or a block of the drawing's own.
	ArrowBlock string
	// SeparateArrows is DIMSAH: ArrowBlock1 and ArrowBlock2 replace ArrowBlock (ArrowBlocks).
	SeparateArrows bool
	// ArrowBlock1 and ArrowBlock2 are DIMBLK1 and DIMBLK2, the arrowheads of the Ext1 and Ext2 ends.
	ArrowBlock1, ArrowBlock2 string
	// LineColor, ExtColor and TextColor are DIMCLRD, DIMCLRE and DIMCLRT, the colours of the
	// dimension line and arrows, the extension lines and the text: ACIs where 0 is ByBlock (the
	// dimension's own colour, AutoCAD's default) and 256 ByLayer. Drawing.ResolveColorIn resolves
	// them.
	LineColor, ExtColor, TextColor int
	// Fit is DIMATFIT, what moves outside when the text and the arrows do not both fit:
	// DimFitBoth, DimFitArrowsFirst, DimFitTextFirst or DimFitBest.
	Fit int
	// TextInside is DIMTIX: the text stays between the extension lines whatever Fit says.
	TextInside bool
	// LineInside is DIMTOFL: the dimension line is drawn between the extension lines even when
	// the arrows are outside.
	LineInside bool
}

// DefaultDimStyle returns AutoCAD's ISO-25 style (acadiso.dwt): DIMSCALE 1, DIMASZ 2.5, DIMTXT
// 2.5, DIMEXO 0.625, DIMEXE 1.25, DIMGAP 0.625, DIMDEC 2, DIMDSEP ",", DIMZIN 8, DIMTAD 1,
// DIMJUST 0, DIMTIH and DIMTOH off, DIMTXSTY Standard, DIMBLK closed filled, DIMCLRD, DIMCLRE and
// DIMCLRT ByBlock, DIMATFIT 3, DIMTIX off and DIMTOFL on.
func DefaultDimStyle() DimStyle {
	return DimStyle{
		Name: "ISO-25", Scale: 1, ArrowSize: 2.5, TextHeight: 2.5, ExtOffset: 0.625, ExtExtend: 1.25,
		Gap: 0.625, Decimals: 2, DecSep: ",", ZeroSupp: 8, TextVertical: DimTextAbove, TextStyle: DefaultTextStyle,
		Fit: DimFitBest, LineInside: true,
	}
}

// size returns v scaled by DIMSCALE, 0 when v is not a finite non-negative number.
func (s DimStyle) size(v float64) float64 {
	k := s.Scale
	if !(k > 0) || math.IsInf(k, 0) {
		k = 1
	}
	r := v * k
	if !(r >= 0) || math.IsInf(r, 0) {
		return 0
	}
	return r
}

// Dimension is a DIMENSION entity: a linear dimension (DimRotated, DimAligned) or an arc length
// one (DimArcLength). It is not associative: it measures its definition points, not the objects
// they came from. Its drawing (Layout) is derived from the fields and its effective style (Style)
// and cached against both, so an edit of its DIMSTYLE record redraws it.
type Dimension struct {
	EntityCommon
	Kind DimKind
	// Ext1 and Ext2 are the extension line origins (DXF 13, 14). For DimArcLength they are the
	// ends of the measured part of the arc.
	Ext1, Ext2 geom.Vec2
	// DimLine is a point on the dimension line (DXF 10); Recompute moves it to the Ext2 end of a
	// linear dimension line. For DimArcLength it is a point on the dimension arc: its distance
	// from Center is the arc's radius.
	DimLine geom.Vec2
	// TextPos is the middle of the text (DXF 11). Unless TextMoved, the layout places the text
	// where the effective style puts it and TextPos is that place as of the last Recompute
	// (TextMiddle is always current).
	TextPos geom.Vec2
	// TextMoved marks user-positioned text (DXF 70 bit 128): Recompute leaves TextPos alone and
	// the layout draws the text there.
	TextMoved bool
	// Rotation is the dimension line angle in radians (DXF 50), DimRotated only.
	Rotation float64
	// TextAngle is the text rotation in radians (DXF 53); 0 aligns the text with the line.
	TextAngle float64
	// Text overrides the measurement (DXF 1): "" draws the measurement, "<>" inside stands for
	// it, and a single space suppresses the text.
	Text string
	// StyleName names the DIMSTYLE record the dimension inherits its variables from (DXF 3).
	StyleName string
	// Overrides are the variables the dimension sets itself (ACAD DSTYLE extended data); the
	// others follow the record. Style returns the result; SetStyle sets both fields.
	Overrides DimOverrides

	// Center, Radius, StartAngle and EndAngle are the measured arc of a DimArcLength,
	// counter-clockwise from StartAngle to EndAngle in radians.
	Center               geom.Vec2
	Radius               float64
	StartAngle, EndAngle float64
	// Partial marks an arc length dimension of part of the arc, from Ext1 to Ext2; otherwise
	// Recompute puts Ext1 and Ext2 on the arc's ends.
	Partial bool
	// ArcSymbol is DIMARCSYM: DimArcSymBefore, DimArcSymAbove or DimArcSymNone.
	ArcSymbol int

	cache *dimCache
	// d is the drawing holding the dimension, whose DIMSTYLE table Style reads.
	d *Drawing
}

// DimLayout is the drawing of a dimension.
type DimLayout struct {
	// ExtLines are the two extension lines, from the Ext1 and Ext2 sides.
	ExtLines [2]geom.Line
	// DimLine is the dimension line of a linear dimension, in two pieces when it is broken around
	// centred text and absent when the arrows are outside and DIMTOFL is off; then, when the
	// arrows are outside, the two stubs beyond them; then, when the text is outside, the
	// extension of the line to the text. For DimArcLength it holds only the stubs.
	DimLine []geom.Line
	// DimArc is the dimension arc of a DimArcLength (zero otherwise).
	DimArc geom.Arc
	// Arrows are the arrowheads on the Ext1 and Ext2 ends, of the style's DIMBLK kinds.
	Arrows [2]DimArrow
	// ArrowTips are where the dimension line meets the extension lines.
	ArrowTips [2]geom.Vec2
	// Flipped reports that the arrows are outside the extension lines (DIMATFIT, DIMTIX).
	Flipped bool
	// TextOutside reports that the text is outside the extension lines (DIMATFIT), or for moved
	// text that it lies beyond them.
	TextOutside bool
	// Text is the laid-out text (middle-centre at its position, readable, in DIMTXSTY). It is
	// shared with the cache: do not modify it.
	Text *Text
	// TextBox is the text's box (TextLayout.Corners).
	TextBox [4]geom.Vec2
	// Symbol is the arc length symbol (zero radius when there is none).
	Symbol geom.Arc
}

type dimKey struct {
	kind                       DimKind
	ext1, ext2, dimLine, text  geom.Vec2
	rot, textAngle             float64
	value                      string
	style                      DimStyle
	center                     geom.Vec2
	radius, startAngle, endAng float64
	partial                    bool
	arcSym                     int
}

type dimCache struct {
	key    dimKey
	lay    DimLayout
	lines  []geom.Curve
	curves []geom.Curve
	bounds geom.Box2
}

// Class implements Object.
func (*Dimension) Class() string { return "DIMENSION" }

// Clone implements Object.
func (e *Dimension) Clone() Object { c := *e; return &c }

// Bounds implements Entity: the drawn geometry, the text box and the definition points.
func (e *Dimension) Bounds() geom.Box2 { return e.layout().bounds }

// Geometry implements Entity: Lines, then the strokes that make filled arrowheads and dots read
// solid and give the architectural tick its width (for block parts and snaps, which carry curves
// only; the renderer fills top-level arrowheads from Layout), the arc symbol and the text strokes.
func (e *Dimension) Geometry() []geom.Curve { return e.layout().curves }

// Lines returns the geometry without the text: the extension lines, the dimension line (or arc
// and stubs) and the arrowhead outlines. Snaps use it.
func (e *Dimension) Lines() []geom.Curve { return e.layout().lines }

// Layout returns the dimension's drawing.
func (e *Dimension) Layout() DimLayout { return e.layout().lay }

// TextMiddle returns where the text's middle is: TextPos when TextMoved, else where the effective
// style places it now (which TextPos holds after Recompute). Writers store it as DXF group 11.
func (e *Dimension) TextMiddle() geom.Vec2 {
	if e.TextMoved {
		return e.TextPos
	}
	if e.Kind == DimArcLength {
		return e.arcPlace(e.arcGeom()).pos
	}
	return e.linearPlace().pos
}

// DefPoints returns the definition points Ext1, Ext2, DimLine and TextMiddle, and Center for a
// DimArcLength. STRETCH moves them and node snaps find them.
func (e *Dimension) DefPoints() []geom.Vec2 {
	p := []geom.Vec2{e.Ext1, e.Ext2, e.DimLine, e.TextMiddle()}
	if e.Kind == DimArcLength {
		p = append(p, e.Center)
	}
	return p
}

// Transform implements Entity. It maps the definition points (and the arc of a DimArcLength),
// so a scale changes the measurement, keeps the style sizes, and regenerates the text readable
// after a reflection; then it calls Recompute.
func (e *Dimension) Transform(m geom.Mat3) {
	if e.Kind == DimRotated {
		e.Rotation = geom.NormAngle(m.ApplyVec(unit(e.Rotation)).Angle())
	}
	if e.TextAngle != 0 {
		e.TextAngle = geom.NormAngle(m.ApplyVec(unit(e.TextAngle)).Angle())
	}
	e.Ext1, e.Ext2, e.DimLine, e.TextPos = m.Apply(e.Ext1), m.Apply(e.Ext2), m.Apply(e.DimLine), m.Apply(e.TextPos)
	if e.Kind == DimArcLength {
		a := geom.Arc{C: e.Center, R: e.Radius, Start: e.StartAngle, End: e.EndAngle}.Transform(m).(geom.Arc)
		e.Center, e.Radius, e.StartAngle, e.EndAngle = a.C, a.R, a.Start, a.End
	}
	e.Recompute()
}

// Direction returns the unit vector of the dimension line: along Rotation for DimRotated, from
// Ext1 to Ext2 for DimAligned ((1, 0) when they coincide), and the counter-clockwise tangent at
// the middle of the dimension arc for DimArcLength.
func (e *Dimension) Direction() geom.Vec2 {
	switch e.Kind {
	case DimAligned:
		if d := e.Ext2.Sub(e.Ext1).Norm(); d != (geom.Vec2{}) {
			return d
		}
		return geom.V(1, 0)
	case DimArcLength:
		a1, sw := e.arcSpan()
		return unit(a1 + sw/2).Perp()
	}
	return unit(e.Rotation)
}

// Measurement returns the measured value: the distance between Ext1 and Ext2 along Direction
// for a linear dimension, the length of the measured arc for DimArcLength.
func (e *Dimension) Measurement() float64 {
	if e.Kind == DimArcLength {
		_, sw := e.arcSpan()
		return math.Abs(e.Radius) * sw
	}
	return math.Abs(e.Ext2.Sub(e.Ext1).Dot(e.Direction()))
}

// MeasurementText returns the text the dimension draws: the measurement formatted by the style
// (Decimals, DecSep, ZeroSupp), with the Text override applied.
func (e *Dimension) MeasurementText() string {
	m := formatDim(e.Measurement(), e.Style())
	switch {
	case e.Text == "":
		return m
	case e.Text == " ":
		return ""
	}
	return strings.Replace(e.Text, "<>", m, 1)
}

// formatDim formats a measured value as DIMDEC, DIMDSEP and DIMZIN say.
func formatDim(v float64, s DimStyle) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	str := strconv.FormatFloat(math.Abs(v), 'f', min(max(s.Decimals, 0), 8), 64)
	if s.ZeroSupp&8 != 0 && strings.Contains(str, ".") {
		str = strings.TrimRight(strings.TrimRight(str, "0"), ".")
	}
	if s.ZeroSupp&4 != 0 && strings.HasPrefix(str, "0.") {
		str = str[1:]
	}
	sep := s.DecSep
	if sep == "" {
		sep = "."
	}
	return strings.Replace(str, ".", sep, 1)
}

// Recompute derives the dependent fields after the definition points change: a linear
// dimension's DimLine moves to the Ext2 end of its line; a whole-arc DimArcLength puts Ext1 and
// Ext2 on the arc's ends; and, unless TextMoved, TextPos goes where the style puts the text
// (linearPlace, arcPlace): along the line by DIMJUST, inside or outside the extension lines by
// DIMATFIT and DIMTIX, and across it by DIMTAD, Gap plus half the text height off the line.
func (e *Dimension) Recompute() {
	var pos geom.Vec2
	if e.Kind == DimArcLength {
		if !e.Partial {
			e.Ext1 = e.Center.Add(unit(e.StartAngle).Scale(e.Radius))
			e.Ext2 = e.Center.Add(unit(e.EndAngle).Scale(e.Radius))
		}
		pos = e.arcPlace(e.arcGeom()).pos
	} else {
		d := e.Direction()
		e.DimLine = e.project(e.Ext2, d)
		pos = e.linearPlace().pos
	}
	if !e.TextMoved {
		e.TextPos = pos
	}
}

// dimPlace is where a dimension's text and arrows go.
type dimPlace struct {
	arrowsOut, textOut bool
	// angle is the readable text rotation and pos the text middle the style asks for.
	angle float64
	pos   geom.Vec2
	// extent is the text's length along the line; onLine reports text centred on it.
	extent float64
	onLine bool
	// side is +1 when the text is beyond Ext2 (or centred), -1 beyond Ext1, along d12.
	side float64
}

// linearFrame is the construction of a linear dimension line.
type linearFrame struct {
	p1, p2, d12 geom.Vec2 // line ends and unit direction from p1 to p2
	out         [2]geom.Vec2
	span        float64
	lineAngle   float64
}

func (e *Dimension) linearFrame() linearFrame {
	d := e.Direction()
	f := linearFrame{p1: e.project(e.Ext1, d), p2: e.project(e.Ext2, d), lineAngle: d.Angle()}
	f.d12 = f.p2.Sub(f.p1).Norm()
	if f.d12 == (geom.Vec2{}) {
		f.d12 = d
	}
	f.span = f.p1.Dist(f.p2)
	for i, pr := range [2][2]geom.Vec2{{e.Ext1, f.p1}, {e.Ext2, f.p2}} {
		u := pr[1].Sub(pr[0]).Norm()
		if u == (geom.Vec2{}) {
			u = d.Perp()
		}
		f.out[i] = u
	}
	return f
}

// textWidth is the length of the dimension's text line at height th.
func (e *Dimension) textWidth(th float64) float64 {
	v := e.MeasurementText()
	if v == "" || !(th > 0) {
		return 0
	}
	return TextAdvance(v) * th
}

// textAngle is the readable text rotation, inside or outside the extension lines: TextAngle when
// set, else horizontal by DIMTIH or DIMTOH, else along lineAngle.
func (e *Dimension) textAngle(lineAngle float64, outside bool) float64 {
	st := e.Style()
	switch {
	case e.TextAngle != 0:
		return readableAngle(e.TextAngle)
	case !outside && st.TextInsideHorizontal, outside && st.TextOutsideHorizontal:
		return 0
	}
	return readableAngle(lineAngle)
}

// across returns the offset of the text middle from the line: none for centred text (DIMTAD 0,
// or text not aligned with the line), Gap plus half the height above the line in the reading
// sense (DIMTAD 1 and 3), below it (4), or on the side away from the origins (2, along out).
func (e *Dimension) across(lineAngle, angle float64, out geom.Vec2) (geom.Vec2, bool) {
	st := e.Style()
	la := readableAngle(lineAngle)
	if st.TextVertical == DimTextCentered || math.Abs(math.Sin(angle-la)) > 1e-9 {
		return geom.Vec2{}, true
	}
	k := st.size(st.Gap) + st.size(st.TextHeight)/2
	up := unit(la).Perp()
	switch st.TextVertical {
	case DimTextBelow:
		return up.Scale(-k), false
	case DimTextOutside:
		if out.Dot(up) < 0 {
			return up.Scale(-k), false
		}
	}
	return up.Scale(k), false
}

// linearPlace decides, AutoCAD's way, whether the arrows and the text fit between the extension
// lines and where the text goes. The arrows fit in 2.5 arrow lengths; the text fits in its length
// along the line plus a Gap each side; centred text needs room for both together. DIMTIX keeps
// the text inside; otherwise DIMATFIT says what moves out. DIMJUST 3 and 4 put the text over an
// extension line, aligned with it.
func (e *Dimension) linearPlace() dimPlace {
	st := e.Style()
	f := e.linearFrame()
	asz, gap, th := st.size(st.ArrowSize), st.size(st.Gap), st.size(st.TextHeight)
	w := e.textWidth(th)
	extent := func(a float64) float64 {
		r := a - f.lineAngle
		return math.Abs(w*math.Cos(r)) + math.Abs(th*math.Sin(r))
	}
	var pl dimPlace
	pl.side = 1
	arrowsFit := f.span >= 2.5*asz
	if st.TextJust == DimJustOverExt1 || st.TextJust == DimJustOverExt2 {
		i, sg := 0, -1.0
		if st.TextJust == DimJustOverExt2 {
			i, sg = 1, 1
		}
		pl.arrowsOut = !arrowsFit
		pl.angle = readableAngle(f.out[i].Angle())
		if e.TextAngle != 0 {
			pl.angle = readableAngle(e.TextAngle)
		}
		tip := [2]geom.Vec2{f.p1, f.p2}[i]
		up := unit(pl.angle).Perp()
		if up.Dot(f.d12.Scale(sg)) < 0 {
			up = up.Scale(-1)
		}
		pl.pos = tip.Add(f.out[i].Scale(gap + w/2)).Add(up.Scale(gap + th/2))
		pl.extent, pl.side = th, sg
		return pl
	}
	aIn := e.textAngle(f.lineAngle, false)
	need := extent(aIn) + 2*gap
	_, onIn := e.across(f.lineAngle, aIn, f.out[0])
	textFits := w == 0 || f.span >= need
	bothFit := textFits && arrowsFit && (!onIn || w == 0 || f.span >= need+2*asz)
	switch {
	case bothFit:
	case st.TextInside || w == 0:
		pl.arrowsOut = true
	case st.Fit == DimFitArrowsFirst:
		pl.arrowsOut, pl.textOut = true, !textFits
	case st.Fit == DimFitTextFirst:
		pl.textOut, pl.arrowsOut = true, !arrowsFit
	case st.Fit == DimFitBest:
		pl.arrowsOut = textFits || !arrowsFit
		pl.textOut = !textFits
	default:
		pl.arrowsOut, pl.textOut = true, true
	}
	pl.angle = e.textAngle(f.lineAngle, pl.textOut)
	pl.extent = extent(pl.angle)
	off, on := e.across(f.lineAngle, pl.angle, f.out[0])
	pl.onLine = on
	var along geom.Vec2
	inset := gap
	if !pl.arrowsOut {
		inset += asz
	}
	switch {
	case pl.textOut:
		end, dir := f.p2, f.d12
		if st.TextJust == DimJustExt1 {
			end, dir, pl.side = f.p1, f.d12.Scale(-1), -1
		}
		reach := asz
		if pl.arrowsOut {
			reach = 2 * asz
		}
		along = end.Add(dir.Scale(reach + gap + pl.extent/2))
	case st.TextJust == DimJustExt1:
		along = f.p1.Add(f.d12.Scale(inset + pl.extent/2))
	case st.TextJust == DimJustExt2:
		along = f.p2.Sub(f.d12.Scale(inset + pl.extent/2))
	default:
		along = f.p1.Lerp(f.p2, 0.5)
	}
	pl.pos = along.Add(off)
	return pl
}

// arcPlace is linearPlace for an arc length dimension: the text stays at the middle of the arc,
// across it by DIMTAD (outward for above), horizontal by DIMTIH; the arrows move outside when
// the arc is shorter than 2.5 arrows.
func (e *Dimension) arcPlace(g arcParts) dimPlace {
	st := e.Style()
	sweep := geom.NormAngle(g.dimArc.End - g.dimArc.Start)
	am := g.dimArc.Start + sweep/2
	lineAngle := am + math.Pi/2
	pl := dimPlace{side: 1}
	pl.arrowsOut = g.dimArc.R*sweep < 2.5*st.size(st.ArrowSize)
	pl.angle = e.textAngle(lineAngle, false)
	off, on := e.across(lineAngle, pl.angle, unit(am))
	pl.onLine = on
	pl.pos = g.dimArc.PointAt(am).Add(off)
	return pl
}

// project returns p projected along the normal of d onto the dimension line through DimLine.
func (e *Dimension) project(p, d geom.Vec2) geom.Vec2 {
	return e.DimLine.Add(d.Scale(p.Sub(e.DimLine).Dot(d)))
}

// arcSpan returns the start angle and sweep of the measured arc: the whole arc, or, for a
// Partial one, the part from Ext1 to Ext2 (in whichever order lies on the arc).
func (e *Dimension) arcSpan() (a1, sw float64) {
	whole := geom.Arc{Start: e.StartAngle, End: e.EndAngle}
	if !e.Partial {
		return whole.Start, whole.Sweep()
	}
	p, q := e.Ext1.Sub(e.Center).Angle(), e.Ext2.Sub(e.Center).Angle()
	sw = geom.NormAngle(q - p)
	if geom.NormAngle(p-whole.Start)+sw > whole.Sweep()+1e-9 {
		p, sw = q, geom.NormAngle(p-q)
	}
	return p, sw
}

// arcParts is the construction of an arc length dimension.
type arcParts struct {
	ends   [2]geom.Vec2 // measured arc ends
	feet   [2]geom.Vec2 // extension line ends on the dimension arc
	dirs   [2]geom.Vec2 // extension line directions, from the arc outward
	dimArc geom.Arc
}

// arcGeom builds the dimension arc and extension lines: parallel to the bisector of the
// measured arc when it spans less than 90°, radial from 90° up (AutoCAD's rule) or when the
// parallel lines miss the dimension arc.
func (e *Dimension) arcGeom() arcParts {
	a1, sw := e.arcSpan()
	r0 := math.Abs(e.Radius)
	R := e.DimLine.Dist(e.Center)
	if R == 0 {
		R = r0
	}
	out := 1.0
	if R < r0 {
		out = -1
	}
	var g arcParts
	g.ends = [2]geom.Vec2{e.Center.Add(unit(a1).Scale(r0)), e.Center.Add(unit(a1 + sw).Scale(r0))}
	b := unit(a1 + sw/2)
	parallel := sw < math.Pi/2
	for i, p := range g.ends {
		q := p.Sub(e.Center)
		qb := q.Dot(b)
		disc := qb*qb - q.Dot(q) + R*R
		if parallel && disc >= 0 {
			t := -qb + math.Sqrt(disc)
			g.feet[i] = p.Add(b.Scale(t))
			switch {
			case t > 0:
				g.dirs[i] = b
			case t < 0:
				g.dirs[i] = b.Scale(-1)
			default:
				g.dirs[i] = b.Scale(out)
			}
			continue
		}
		parallel = false
	}
	if !parallel {
		for i, p := range g.ends {
			r := p.Sub(e.Center).Norm()
			if r == (geom.Vec2{}) {
				r = b
			}
			g.feet[i] = e.Center.Add(r.Scale(R))
			g.dirs[i] = r.Scale(out)
		}
	}
	g.dimArc = geom.Arc{C: e.Center, R: R, Start: geom.NormAngle(g.feet[0].Sub(e.Center).Angle()), End: geom.NormAngle(g.feet[1].Sub(e.Center).Angle())}
	if sw == 0 || g.dimArc.Start == g.dimArc.End {
		g.dimArc.End = g.dimArc.Start // a zero sweep, not Arc's full circle
	}
	return g
}

// unit returns the unit vector at angle a.
func unit(a float64) geom.Vec2 { return geom.V(math.Cos(a), math.Sin(a)) }

// readableAngle reduces a text angle to (-90°, 90°], where text reads left to right or bottom to
// top.
func readableAngle(a float64) float64 {
	const eps = 1e-9
	a = geom.NormAngle(a)
	switch {
	case a > math.Pi/2+eps && a <= 3*math.Pi/2+eps:
		return a - math.Pi
	case a > 3*math.Pi/2+eps:
		return a - 2*math.Pi
	}
	return a
}

func (e *Dimension) key() dimKey {
	return dimKey{e.Kind, e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Rotation, e.TextAngle, e.Text, e.Style(),
		e.Center, e.Radius, e.StartAngle, e.EndAngle, e.Partial, e.ArcSymbol}
}

// arrowFill is the number of strokes from an arrow's tip to its base that make it read solid.
const arrowFill = 6

// layout returns the cached drawing, rebuilding it when a field changed since; see Text.layout.
func (e *Dimension) layout() *dimCache {
	k := e.key()
	if e.cache != nil && e.cache.key == k {
		return e.cache
	}
	c := &dimCache{key: k}
	lay := &c.lay
	st := e.Style()
	asz := st.size(st.ArrowSize)
	gap := st.size(st.Gap)
	var tips, outward [2]geom.Vec2
	var pl dimPlace
	var f linearFrame
	if e.Kind == DimArcLength {
		g := e.arcGeom()
		for i := range 2 {
			lay.ExtLines[i] = geom.Line{A: g.ends[i].Add(g.dirs[i].Scale(st.size(st.ExtOffset))), B: g.feet[i].Add(g.dirs[i].Scale(st.size(st.ExtExtend)))}
		}
		lay.DimArc = g.dimArc
		tips = g.feet
		outward = [2]geom.Vec2{unit(g.dimArc.Start).Perp().Scale(-1), unit(g.dimArc.End).Perp()}
		pl = e.arcPlace(g)
	} else {
		f = e.linearFrame()
		for i, pr := range [2][2]geom.Vec2{{e.Ext1, f.p1}, {e.Ext2, f.p2}} {
			lay.ExtLines[i] = geom.Line{A: pr[0].Add(f.out[i].Scale(st.size(st.ExtOffset))), B: pr[1].Add(f.out[i].Scale(st.size(st.ExtExtend)))}
		}
		tips = [2]geom.Vec2{f.p1, f.p2}
		outward = [2]geom.Vec2{f.d12.Scale(-1), f.d12}
		pl = e.linearPlace()
		if e.TextMoved {
			// The text is where the user put it: outside when it lies beyond the extension lines.
			t := e.TextPos.Sub(f.p1).Dot(f.d12)
			pl.textOut = t < 0 || t > f.span
			pl.angle = e.textAngle(f.lineAngle, pl.textOut)
			_, pl.onLine = e.across(f.lineAngle, pl.angle, f.out[0])
		}
	}
	lay.ArrowTips = tips
	lay.Flipped = pl.arrowsOut
	lay.TextOutside = pl.textOut
	blocks := [2]string{}
	blocks[0], blocks[1] = st.ArrowBlocks()
	var fills []geom.Curve
	for i := range 2 {
		point := outward[i]
		if lay.Flipped {
			point = point.Scale(-1)
		}
		lay.Arrows[i] = LayoutArrow(blocks[i], tips[i], point, asz)
	}
	if e.Kind != DimArcLength {
		a, b := lay.Arrows[0].LineEnd, lay.Arrows[1].LineEnd
		if lay.Flipped {
			a, b = f.p1, f.p2
		}
		if !lay.Flipped || st.LineInside {
			lay.DimLine = append(lay.DimLine, geom.Line{A: a, B: b})
		}
	}
	if lay.Flipped {
		for i := range 2 {
			lay.DimLine = append(lay.DimLine, geom.Line{A: lay.Arrows[i].LineEnd, B: tips[i].Add(outward[i].Scale(2 * asz))})
		}
	}

	th := st.size(st.TextHeight)
	value := e.MeasurementText()
	pos := e.TextPos
	if !e.TextMoved {
		pos = pl.pos
	}
	ta := pl.angle
	u := unit(ta)
	if e.Kind == DimArcLength && value != "" && th > 0 {
		w := TextAdvance(value) * th
		switch e.ArcSymbol {
		case DimArcSymBefore:
			// The symbol is part of the text line: the pair is centred on TextPos.
			sw, gap := 0.9*th, 0.2*th
			lay.Symbol = symbolArc(pos.Sub(u.Scale((w+gap)/2)), u, sw, 0, 0.4*th)
			pos = pos.Add(u.Scale((sw + gap) / 2))
		case DimArcSymAbove:
			lay.Symbol = symbolArc(pos, u, max(w, th), 0.7*th, 0.95*th)
		}
	}
	style := st.TextStyle
	if style == "" {
		style = DefaultTextStyle
	}
	lay.Text = &Text{Insert: pos, Align: pos, Height: th, Rotation: ta, WidthFactor: 1, Style: style,
		HAlign: TextCenter, VAlign: TextVMiddle, Value: value}
	lay.TextBox = lay.Text.Layout().Corners
	if e.Kind != DimArcLength && value != "" && th > 0 {
		e.fitLineToText(lay, f, pl, gap)
	}

	for _, l := range lay.ExtLines {
		c.lines = append(c.lines, l)
	}
	if e.Kind == DimArcLength {
		c.lines = append(c.lines, lay.DimArc)
	}
	for _, l := range lay.DimLine {
		c.lines = append(c.lines, l)
	}
	for _, a := range lay.Arrows {
		outline, fill := a.curves(true)
		c.lines = append(c.lines, outline...)
		fills = append(fills, fill...)
	}
	c.curves = append(append([]geom.Curve(nil), c.lines...), fills...)
	if lay.Symbol.R > 0 {
		c.curves = append(c.curves, lay.Symbol)
	}
	c.curves = append(c.curves, lay.Text.Geometry()...)
	c.bounds = geom.BoxOf(e.DefPoints()...).Union(geom.BoxOf(lay.TextBox[:]...))
	for _, cv := range c.curves {
		c.bounds = c.bounds.Union(cv.Bounds())
	}
	e.cache = c
	return c
}

// fitLineToText breaks a linear dimension line around text centred on it, and runs the line out
// to text placed outside the extension lines (AutoCAD's DIMTMOVE 0): under the text when it is
// off the line, up to a Gap short of it when it is on the line.
func (e *Dimension) fitLineToText(lay *DimLayout, f linearFrame, pl dimPlace, gap float64) {
	// The text's interval along the line, from p1.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range lay.TextBox {
		t := p.Sub(f.p1).Dot(f.d12)
		lo, hi = min(lo, t), max(hi, t)
	}
	// Only text centred on the line breaks it (moved text never does).
	crosses := pl.onLine && !e.TextMoved
	if pl.textOut && !e.TextMoved {
		if pl.side > 0 {
			end := hi
			if crosses {
				end = lo - gap
			}
			if end > f.span {
				lay.DimLine = append(lay.DimLine, geom.Line{A: f.p2, B: f.p1.Add(f.d12.Scale(end))})
			}
		} else {
			end := lo
			if crosses {
				end = hi + gap
			}
			if end < 0 {
				lay.DimLine = append(lay.DimLine, geom.Line{A: f.p1, B: f.p1.Add(f.d12.Scale(end))})
			}
		}
		return
	}
	if !crosses || len(lay.DimLine) == 0 || lay.Flipped && !e.Style().LineInside {
		return
	}
	line := lay.DimLine[0]
	a, b := line.A.Sub(f.p1).Dot(f.d12), line.B.Sub(f.p1).Dot(f.d12)
	cutLo, cutHi := lo-gap, hi+gap
	if cutHi <= a || cutLo >= b {
		return
	}
	var pieces []geom.Line
	if cutLo > a {
		pieces = append(pieces, geom.Line{A: line.A, B: f.p1.Add(f.d12.Scale(cutLo))})
	}
	if cutHi < b {
		pieces = append(pieces, geom.Line{A: f.p1.Add(f.d12.Scale(cutHi)), B: line.B})
	}
	lay.DimLine = append(pieces, lay.DimLine[1:]...)
}

// symbolArc returns the arc length symbol: an arc of the given chord whose chord ends lie y0 and
// apex y1 above the point at, in the text frame of direction u.
func symbolArc(at, u geom.Vec2, chord, y0, y1 float64) geom.Arc {
	v := u.Perp()
	s := y1 - y0
	r := (chord*chord/4 + s*s) / (2 * s)
	half := math.Asin(min(1, chord/2/r))
	up := v.Angle()
	return geom.Arc{C: at.Add(v.Scale(y1 - r)), R: r, Start: geom.NormAngle(up - half), End: geom.NormAngle(up + half)}
}
