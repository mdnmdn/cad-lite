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
// is read).
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
	// TextAbove is DIMTAD non-zero: the text sits above the dimension line instead of on it.
	TextAbove bool
}

// DefaultDimStyle returns AutoCAD's ISO-25 style (acadiso.dwt): DIMSCALE 1, DIMASZ 2.5, DIMTXT
// 2.5, DIMEXO 0.625, DIMEXE 1.25, DIMGAP 0.625, DIMDEC 2, DIMDSEP ",", DIMZIN 8, DIMTAD 1.
func DefaultDimStyle() DimStyle {
	return DimStyle{
		Name: "ISO-25", Scale: 1, ArrowSize: 2.5, TextHeight: 2.5, ExtOffset: 0.625, ExtExtend: 1.25,
		Gap: 0.625, Decimals: 2, DecSep: ",", ZeroSupp: 8, TextAbove: true,
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
// they came from. Its drawing (Layout) is derived from the fields and cached.
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
	// TextPos is the middle of the text (DXF 11).
	TextPos geom.Vec2
	// TextMoved marks user-positioned text (DXF 70 bit 128): Recompute leaves TextPos alone.
	TextMoved bool
	// Rotation is the dimension line angle in radians (DXF 50), DimRotated only.
	Rotation float64
	// TextAngle is the text rotation in radians (DXF 53); 0 aligns the text with the line.
	TextAngle float64
	// Text overrides the measurement (DXF 1): "" draws the measurement, "<>" inside stands for
	// it, and a single space suppresses the text.
	Text  string
	Style DimStyle

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
}

// DimLayout is the drawing of a dimension.
type DimLayout struct {
	// ExtLines are the two extension lines, from the Ext1 and Ext2 sides.
	ExtLines [2]geom.Line
	// DimLine is the dimension line of a linear dimension followed, when the arrows are outside,
	// by the two stubs beyond them. For DimArcLength it holds only the stubs.
	DimLine []geom.Line
	// DimArc is the dimension arc of a DimArcLength (zero otherwise).
	DimArc geom.Arc
	// Arrows are the closed arrowheads, tip first, on the Ext1 and Ext2 ends.
	Arrows [2][3]geom.Vec2
	// ArrowTips are where the dimension line meets the extension lines.
	ArrowTips [2]geom.Vec2
	// Flipped reports that the arrows are outside the extension lines, the space being too short.
	Flipped bool
	// Text is the laid-out text (middle-centre at its position, readable). It is shared with
	// the cache: do not modify it.
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

// Geometry implements Entity: Lines, then the strokes that fill the arrowheads (the renderer
// has no fills), the arc symbol and the text strokes.
func (e *Dimension) Geometry() []geom.Curve { return e.layout().curves }

// Lines returns the geometry without the text: the extension lines, the dimension line (or arc
// and stubs) and the arrowhead outlines. Snaps use it.
func (e *Dimension) Lines() []geom.Curve { return e.layout().lines }

// Layout returns the dimension's drawing.
func (e *Dimension) Layout() DimLayout { return e.layout().lay }

// DefPoints returns the definition points Ext1, Ext2, DimLine and TextPos, and Center for a
// DimArcLength. STRETCH moves them and node snaps find them.
func (e *Dimension) DefPoints() []geom.Vec2 {
	p := []geom.Vec2{e.Ext1, e.Ext2, e.DimLine, e.TextPos}
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
	m := formatDim(e.Measurement(), e.Style)
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
// Ext2 on the arc's ends; and, unless TextMoved, TextPos goes to the middle of the dimension
// line, offset by Gap plus half the text height on the side above the readable text when TextAbove,
// on the line otherwise. For DimArcLength the line is the dimension arc and its middle tangent.
func (e *Dimension) Recompute() {
	var mid, up geom.Vec2
	if e.Kind == DimArcLength {
		if !e.Partial {
			e.Ext1 = e.Center.Add(unit(e.StartAngle).Scale(e.Radius))
			e.Ext2 = e.Center.Add(unit(e.EndAngle).Scale(e.Radius))
		}
		g := e.arcGeom()
		am := g.dimArc.Start + geom.NormAngle(g.dimArc.End-g.dimArc.Start)/2
		mid, up = g.dimArc.PointAt(am), unit(readableAngle(am+math.Pi/2)).Perp()
	} else {
		d := e.Direction()
		p1, p2 := e.project(e.Ext1, d), e.project(e.Ext2, d)
		e.DimLine = p2
		mid, up = p1.Lerp(p2, 0.5), unit(readableAngle(d.Angle())).Perp()
	}
	if e.TextMoved {
		return
	}
	e.TextPos = mid
	if e.Style.TextAbove {
		e.TextPos = mid.Add(up.Scale(e.Style.size(e.Style.Gap) + e.Style.size(e.Style.TextHeight)/2))
	}
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
	return dimKey{e.Kind, e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Rotation, e.TextAngle, e.Text, e.Style,
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
	st := e.Style
	asz := st.size(st.ArrowSize)
	var tips, outward [2]geom.Vec2
	var span, lineAngle float64
	if e.Kind == DimArcLength {
		g := e.arcGeom()
		for i := range 2 {
			lay.ExtLines[i] = geom.Line{A: g.ends[i].Add(g.dirs[i].Scale(st.size(st.ExtOffset))), B: g.feet[i].Add(g.dirs[i].Scale(st.size(st.ExtExtend)))}
		}
		lay.DimArc = g.dimArc
		tips = g.feet
		outward = [2]geom.Vec2{unit(g.dimArc.Start).Perp().Scale(-1), unit(g.dimArc.End).Perp()}
		sweep := geom.NormAngle(g.dimArc.End - g.dimArc.Start)
		span = g.dimArc.R * sweep
		lineAngle = g.dimArc.Start + sweep/2 + math.Pi/2
	} else {
		d := e.Direction()
		n := d.Perp()
		p1, p2 := e.project(e.Ext1, d), e.project(e.Ext2, d)
		for i, pr := range [2][2]geom.Vec2{{e.Ext1, p1}, {e.Ext2, p2}} {
			u := pr[1].Sub(pr[0]).Norm()
			if u == (geom.Vec2{}) {
				u = n
			}
			lay.ExtLines[i] = geom.Line{A: pr[0].Add(u.Scale(st.size(st.ExtOffset))), B: pr[1].Add(u.Scale(st.size(st.ExtExtend)))}
		}
		d12 := p2.Sub(p1).Norm()
		if d12 == (geom.Vec2{}) {
			d12 = d
		}
		tips = [2]geom.Vec2{p1, p2}
		outward = [2]geom.Vec2{d12.Scale(-1), d12}
		span = p1.Dist(p2)
		lineAngle = d.Angle()
		lay.DimLine = []geom.Line{{A: p1, B: p2}}
	}
	lay.ArrowTips = tips
	lay.Flipped = span < 2.5*asz
	var fills []geom.Curve
	for i := range 2 {
		point := outward[i]
		if lay.Flipped {
			point = point.Scale(-1)
			lay.DimLine = append(lay.DimLine, geom.Line{A: tips[i], B: tips[i].Add(outward[i].Scale(2 * asz))})
		}
		base := tips[i].Sub(point.Scale(asz))
		w := point.Perp().Scale(asz / 6)
		lay.Arrows[i] = [3]geom.Vec2{tips[i], base.Add(w), base.Sub(w)}
		for j := 1; j < arrowFill; j++ {
			fills = append(fills, geom.Line{A: tips[i], B: base.Add(w).Lerp(base.Sub(w), float64(j)/arrowFill)})
		}
	}

	ta := lineAngle
	if e.TextAngle != 0 {
		ta = e.TextAngle
	}
	ta = readableAngle(ta)
	th := st.size(st.TextHeight)
	value := e.MeasurementText()
	pos := e.TextPos
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
	lay.Text = &Text{Insert: pos, Align: pos, Height: th, Rotation: ta, WidthFactor: 1, Style: DefaultTextStyle,
		HAlign: TextCenter, VAlign: TextVMiddle, Value: value}
	lay.TextBox = lay.Text.Layout().Corners

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
		c.lines = append(c.lines, geom.Polyline{Pts: a[:], Closed: true})
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
