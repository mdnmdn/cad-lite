package dxf

import (
	"math"
	"strconv"
	"strings"

	"cad-lite/doc"
	"cad-lite/geom"
)

// Dimensions in DXF.
//
// Reading: a DIMENSION of type 0 (rotated, horizontal, vertical) or 1 (aligned) becomes a
// doc.Dimension, and so does an ARC_DIMENSION (AutoCAD 2006 and later) and the INSERT the writer
// emits for an arc length dimension (CADLITE_DIM extended data, below). The style is the DIMSTYLE
// record named by group 3, then the ACAD DSTYLE overrides of the entity. The anonymous block a
// converted dimension references is regenerated on writing, so the reader does not load it
// (dimensionBlocks). Every other dimension stays a doc.Unknown with its block.
//
// Writing (symbolwrite.go): a linear dimension is a DIMENSION record with a fresh *D block holding
// its drawing; an arc length dimension, which neither R12 nor AutoCAD 2000 has, is an INSERT of
// such a block carrying its definition in CADLITE_DIM extended data.

// dimApp is the application of the arc length dimension's extended data.
const dimApp = "CADLITE_DIM"

// acadApp is AutoCAD's application, whose DSTYLE extended data holds dimension style overrides.
const acadApp = "ACAD"

// defpointsLayer is the layer AutoCAD puts dimension definition points on; it does not plot.
const defpointsLayer = "DEFPOINTS"

// acadDimDefaults are the values AutoCAD assumes for the dimension variables a DIMSTYLE record
// leaves out (its built-in defaults, the imperial Standard style).
func acadDimDefaults(name string) doc.DimStyle {
	return doc.DimStyle{Name: name, Scale: 1, ArrowSize: 0.18, TextHeight: 0.18, ExtOffset: 0.0625,
		ExtExtend: 0.18, Gap: 0.09, Decimals: 4, DecSep: ".", ZeroSupp: 0, TextVertical: doc.DimTextCentered,
		TextInsideHorizontal: true, TextOutsideHorizontal: true, TextStyle: doc.DefaultTextStyle, Fit: doc.DimFitBest}
}

// names maps the handles of block records and text styles to their names, for the dimension
// variables R2000 stores as handles (DIMBLK 342 to 344, DIMTXSTY 340).
type names struct {
	blocks, styles map[doc.Handle]string
}

func (n *names) add(h doc.Handle, name string, block bool) {
	if h == 0 || name == "" {
		return
	}
	m := &n.styles
	if block {
		m = &n.blocks
	}
	if *m == nil {
		*m = map[doc.Handle]string{}
	}
	(*m)[h] = name
}

func (n *names) lookup(v string, block bool) (string, bool) {
	h, err := doc.ParseHandle(strings.TrimSpace(v))
	if err != nil || n == nil {
		return "", false
	}
	if h == 0 {
		return "", true // a null handle: closed filled, or the default style
	}
	m := n.styles
	if block {
		m = n.blocks
	}
	name, ok := m[h]
	return name, ok
}

// dimVarCodes are the DIMSTYLE group codes dimVar models; every other variable of a record is kept
// as unmodelled groups (doc.Drawing.DimStyleExtra).
var dimVarCodes = map[int]bool{40: true, 41: true, 42: true, 44: true, 140: true, 147: true, 271: true, 278: true,
	78: true, 77: true, 90: true, 5: true, 6: true, 7: true, 342: true, 343: true, 344: true, 173: true, 176: true,
	177: true, 178: true, 340: true, 280: true, 73: true, 74: true, 289: true, 174: true, 172: true}

// dimVar applies dimension variable code (its DIMSTYLE group code, also the code DSTYLE
// overrides use) with value v to s and *arcSym. R12 names the arrow blocks (5, 6, 7); R2000 points
// at their block records (342, 343, 344) and at the text style (340), which nm resolves. Codes the
// document does not model are ignored.
func dimVar(s *doc.DimStyle, arcSym *int, code int, v string, nm *names) {
	v = strings.TrimSpace(v)
	f, ferr := strconv.ParseFloat(v, 64)
	if ferr != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		f, ferr = 0, strconv.ErrSyntax
	}
	i, ierr := strconv.Atoi(v)
	if ierr != nil && ferr == nil {
		i, ierr = int(f), nil
	}
	flt := func(dst *float64) {
		if ferr == nil {
			*dst = f
		}
	}
	flag := func(dst *bool) {
		if ierr == nil {
			*dst = i != 0
		}
	}
	color := func(dst *int) {
		if ierr == nil && i >= 0 && i <= doc.ByLayer {
			*dst = i
		}
	}
	enum := func(dst *int, hi int) {
		if ierr == nil && i >= 0 && i <= hi {
			*dst = i
		}
	}
	block := func(dst *string) {
		if name, ok := nm.lookup(v, true); ok {
			*dst = name
		}
	}
	switch code {
	case 40:
		flt(&s.Scale)
	case 41:
		flt(&s.ArrowSize)
	case 42:
		flt(&s.ExtOffset)
	case 44:
		flt(&s.ExtExtend)
	case 140:
		flt(&s.TextHeight)
	case 147:
		flt(&s.Gap)
	case 271:
		if ierr == nil {
			s.Decimals = min(max(i, 0), 8)
		}
	case 278:
		if ierr == nil {
			s.DecSep = "."
			if i > 32 && i < 127 {
				s.DecSep = string(rune(i))
			}
		}
	case 78:
		if ierr == nil {
			s.ZeroSupp = i
		}
	case 77:
		enum(&s.TextVertical, 4)
	case 280:
		enum(&s.TextJust, 4)
	case 289:
		enum(&s.Fit, 3)
	case 73:
		flag(&s.TextInsideHorizontal)
	case 74:
		flag(&s.TextOutsideHorizontal)
	case 174:
		flag(&s.TextInside)
	case 172:
		flag(&s.LineInside)
	case 173:
		flag(&s.SeparateArrows)
	case 176:
		color(&s.LineColor)
	case 177:
		color(&s.ExtColor)
	case 178:
		color(&s.TextColor)
	case 5:
		s.ArrowBlock = v
	case 6:
		s.ArrowBlock1 = v
	case 7:
		s.ArrowBlock2 = v
	case 342:
		block(&s.ArrowBlock)
	case 343:
		block(&s.ArrowBlock1)
	case 344:
		block(&s.ArrowBlock2)
	case 340:
		if name, ok := nm.lookup(v, false); ok {
			s.TextStyle = name
		}
	case 90:
		if ierr == nil && arcSym != nil && i >= 0 && i <= 2 {
			*arcSym = i
		}
	}
}

// parseDimStyle reads a DIMSTYLE record (R12 or R2000 and later: both keep the variables in the
// groups dimVar reads; R12 has no DIMDEC, DIMDSEP, DIMJUST, DIMATFIT or DIMTXSTY); a variable it
// lacks takes AutoCAD's built-in default. It returns the groups of the variables the document
// does not model too, without the record's own handle, owner and subclass groups and without
// handles to other records (341, 345 to 347), which would not survive the rewrite.
func parseDimStyle(o object, nm *names) (doc.DimStyle, int, []doc.Pair) {
	s, arcSym := acadDimDefaults(o.str(2)), 0
	var extra []doc.Pair
	var own []pair
	r2000 := o.has(105) || o.has(100)
	xdata, mine := false, false
	for _, p := range o.pairs {
		if p.code == 1001 {
			xdata, mine = true, strings.EqualFold(strings.TrimSpace(p.val), dimStyleApp)
			if mine {
				continue
			}
		}
		switch {
		case mine:
			own = append(own, p)
		case xdata:
			extra = append(extra, doc.Pair{Code: p.code, Value: p.val})
		case p.code == 2 || p.code == 70 || p.code == 100 || p.code == 102 || p.code == 105 || p.code == 330 || p.code == 360 || p.code >= 341 && p.code <= 347 && !dimVarCodes[p.code]:
		case p.code == 5 && r2000:
		case dimVarCodes[p.code]:
			dimVar(&s, &arcSym, p.code, p.val, nm)
		default:
			extra = append(extra, doc.Pair{Code: p.code, Value: p.val})
		}
	}
	if own != nil {
		readDimStyleXData(&s, own)
	}
	if s.TextStyle == "" {
		s.TextStyle = doc.DefaultTextStyle
	}
	return s, arcSym, extra
}

// readDimStyles replaces d's dimension style table with the file's DIMSTYLE records.
func readDimStyles(d *doc.Drawing, objs []object, st *state) {
	st.dimArcSym = map[string]int{}
	var styles []doc.DimStyle
	var extras [][]doc.Pair
	for _, o := range objs {
		if o.str(2) == "" {
			continue
		}
		s, arcSym, extra := parseDimStyle(o, &st.names)
		styles, extras = append(styles, s), append(extras, extra)
		st.dimArcSym[strings.ToUpper(s.Name)] = arcSym
	}
	d.LoadDimStyles(styles, extras)
}

// dstyleOverrides returns the code and value pairs of o's ACAD DSTYLE extended data:
// {1000 DSTYLE, 1002 {, 1070 code, value, ..., 1002 }}.
func dstyleOverrides(o object) [][2]string {
	var out [][2]string
	app, in, inList := "", false, false
	code := -1
	for _, p := range o.pairs {
		if p.code == 1001 {
			app, in, inList = strings.TrimSpace(p.val), false, false
			continue
		}
		if !strings.EqualFold(app, acadApp) {
			continue
		}
		v := strings.TrimSpace(p.val)
		switch {
		case !in:
			in = p.code == 1000 && strings.EqualFold(v, "DSTYLE")
		case p.code == 1002 && v == "{":
			inList, code = true, -1
		case p.code == 1002 && v == "}":
			in, inList = false, false
		case !inList:
		case code < 0 && p.code == 1070:
			code, _ = strconv.Atoi(v)
		case code >= 0:
			out = append(out, [2]string{strconv.Itoa(code), v})
			code = -1
		}
	}
	return out
}

// dimStyleOf returns the style a dimension record o draws with: the DIMSTYLE name (group 3; the
// ISO-25 defaults when the table lacks it) with o's DSTYLE overrides applied, the variables those
// overrides set, and the DIMARCSYM value.
func dimStyleOf(name string, o object, d *doc.Drawing, st *state) (doc.DimStyle, doc.DimVar, int) {
	s := d.DimStyleBase(name)
	arcSym := st.dimArcSym[strings.ToUpper(name)]
	var set doc.DimVar
	for _, kv := range dstyleOverrides(o) {
		code, _ := strconv.Atoi(kv[0])
		dimVar(&s, &arcSym, code, kv[1], &st.names)
		set |= dimVarOfCode[code]
	}
	return s, set, arcSym
}

// dimVarOfCode maps a DIMSTYLE group code (and DSTYLE override code) to the variable it sets.
var dimVarOfCode = map[int]doc.DimVar{40: doc.DimVarScale, 41: doc.DimVarArrowSize, 42: doc.DimVarExtOffset,
	44: doc.DimVarExtExtend, 140: doc.DimVarTextHeight, 147: doc.DimVarGap, 271: doc.DimVarDecimals,
	278: doc.DimVarDecSep, 78: doc.DimVarZeroSupp, 77: doc.DimVarTextVertical, 280: doc.DimVarTextJust,
	289: doc.DimVarFit, 73: doc.DimVarTextInsideHorizontal, 74: doc.DimVarTextOutsideHorizontal,
	174: doc.DimVarTextInside, 172: doc.DimVarLineInside, 173: doc.DimVarSeparateArrows,
	176: doc.DimVarLineColor, 177: doc.DimVarExtColor, 178: doc.DimVarTextColor, 5: doc.DimVarArrowBlock,
	342: doc.DimVarArrowBlock, 6: doc.DimVarArrowBlock1, 343: doc.DimVarArrowBlock1, 7: doc.DimVarArrowBlock2,
	344: doc.DimVarArrowBlock2, 340: doc.DimVarTextStyle}

// subclass returns the groups of o's subclass marker name, up to the next marker or the
// extended data; o itself when it has no marker (R12).
func subclass(o object, name string) object {
	out := object{typ: o.typ}
	in, marked := false, false
	for _, p := range o.pairs {
		if p.code == 100 {
			marked = true
			in = strings.EqualFold(strings.TrimSpace(p.val), name)
			continue
		}
		if p.code >= 1000 {
			in = false
		}
		if in {
			out.pairs = append(out.pairs, p)
		}
	}
	if !marked {
		return o
	}
	return out
}

// flatZ reports whether o's extrusion (210, 220, 230) is the default +Z.
func flatZ(o object) bool {
	return o.float(210) == 0 && o.float(220) == 0 && (!o.has(230) || o.float(230) > 0)
}

// dimensionConvertible reports whether the reader turns o into a doc.Dimension: a DIMENSION of
// type 0 or 1 (not ordinate) in the XY plane without obliqued extension lines, an
// ARC_DIMENSION without a leader, or an INSERT carrying a valid CADLITE_DIM definition.
func dimensionConvertible(o object) bool {
	switch o.typ {
	case "DIMENSION":
		base := subclass(o, "AcDbDimension")
		t := base.int(70)
		return t&7 <= 1 && t&64 == 0 && flatZ(o) && o.float(52) == 0
	case "ARC_DIMENSION":
		arc := subclass(o, "AcDbArcDimension")
		return arc.has(15) && arc.int(71) == 0 && flatZ(o)
	case "INSERT":
		e, _ := parseDimXData(o)
		return o.int(66) != 1 && e != nil
	}
	return false
}

// dimensionBlocks returns the upper-cased names of the blocks referenced by the dimensions the
// reader converts and by nothing else: they are regenerated on writing, so the reader drops them.
func dimensionBlocks(ps []pair) map[string]bool {
	drop, keep := map[string]bool{}, map[string]bool{}
	var o object
	flush := func() {
		switch o.typ {
		case "DIMENSION", "ARC_DIMENSION", "INSERT":
			name := strings.ToUpper(o.str(2))
			if name == "" {
				return
			}
			if dimensionConvertible(o) {
				drop[name] = true
			} else {
				keep[name] = true
			}
		}
	}
	for _, p := range ps {
		if p.code == 0 {
			flush()
			o = object{typ: strings.TrimSpace(p.val)}
			continue
		}
		o.pairs = append(o.pairs, p)
	}
	flush()
	for n := range keep {
		delete(drop, n)
	}
	return drop
}

// readDimension converts a DIMENSION or ARC_DIMENSION record, or returns nil when it is not one
// the document models (dimensionConvertible).
func readDimension(o object, c doc.EntityCommon, st *state) doc.Entity {
	if !dimensionConvertible(o) {
		return nil
	}
	base := subclass(o, "AcDbDimension")
	e := &doc.Dimension{EntityCommon: c, DimLine: vec(base, 10), TextPos: vec(base, 11), TextMoved: base.int(70)&128 != 0}
	if v, ok := base.raw(1); ok {
		e.Text = decodeUnicode(v)
	}
	// Group 53 is the absolute text angle (measured from the X axis, as AutoCAD and ezdxf draw
	// it); 0 or absent is the default, aligned with the dimension line. The document's 0 means
	// the same, so an explicit angle of 0 (horizontal text) is kept as 180°, which draws the
	// same once made readable.
	if a := base.float(53); a != 0 {
		e.TextAngle = deg(a)
		if e.TextAngle == 0 {
			e.TextAngle = math.Pi
		}
	}
	main := st.main
	if main == nil {
		main = doc.New()
	}
	var style doc.DimStyle
	var set doc.DimVar
	style, set, e.ArcSymbol = dimStyleOf(base.str(3), o, main, st)
	rec := main.DimStyleBase(base.str(3))
	// An override equal to the record's value is dropped, so the dimension follows later edits of
	// the record (the R12 writer repeats values R12 records cannot hold as overrides).
	set &= doc.DimStyleDiff(rec, style)
	e.StyleName = rec.Name
	e.Overrides = doc.DimOverrides{}.With(set, style)
	if o.typ == "ARC_DIMENSION" {
		readArcDimension(e, o)
	} else {
		lin := subclass(o, "AcDbAlignedDimension")
		e.Kind = doc.DimKind(base.int(70) & 7)
		e.Ext1, e.Ext2 = vec(lin, 13), vec(lin, 14)
		if e.Kind == doc.DimRotated {
			// AutoCAD writes 50 inside AcDbAlignedDimension, before the AcDbRotatedDimension marker.
			a := lin.float(50)
			if !lin.has(50) {
				a = subclass(o, "AcDbRotatedDimension").float(50)
			}
			e.Rotation = deg(a)
		}
	}
	e.Recompute()
	return e
}

// readArcDimension fills an arc length dimension from an ARC_DIMENSION: 15 is the arc's centre,
// 13 and 14 the ends of the measured arc, 70 (in AcDbArcDimension) the partial flag, and 40 and
// 41 the arc's start and end parameters (angles, radians) of a partial dimension. The arc runs
// counter-clockwise between 13 and 14 through the side the text is on.
func readArcDimension(e *doc.Dimension, o object) {
	arc := subclass(o, "AcDbArcDimension")
	e.Kind = doc.DimArcLength
	e.Center = vec(arc, 15)
	p, q := vec(arc, 13), vec(arc, 14)
	e.Radius = p.Dist(e.Center)
	if e.Radius == 0 {
		e.Radius = q.Dist(e.Center)
	}
	a, b := geom.NormAngle(p.Sub(e.Center).Angle()), geom.NormAngle(q.Sub(e.Center).Angle())
	if t := e.TextPos.Sub(e.Center); t != (geom.Vec2{}) && geom.NormAngle(t.Angle()-a) > geom.NormAngle(b-a) {
		a, b = b, a
	}
	e.StartAngle, e.EndAngle = a, b
	if arc.int(70) != 0 {
		s, f := geom.NormAngle(arc.float(40)), geom.NormAngle(arc.float(41))
		whole := geom.Arc{Start: s, End: f}
		on := func(v geom.Vec2) bool {
			return geom.NormAngle(v.Sub(e.Center).Angle()-s) <= whole.Sweep()+1e-9
		}
		if s != f && on(p) && on(q) {
			e.Partial, e.Ext1, e.Ext2 = true, p, q
			e.StartAngle, e.EndAngle = s, f
		}
	}
	if e.DimLine.Dist(e.Center) == 0 {
		e.DimLine = e.TextPos
	}
}

// The CADLITE_DIM extended data of an arc length dimension written as an INSERT, in order:
// 1000 "ARC_LENGTH", 1070 version (1), 1010 Ext1, Ext2, DimLine, TextPos and Center, 1040
// Radius, StartAngle, EndAngle and TextAngle (radians), 1070 flags (1 TextMoved, 2 Partial),
// 1070 ArcSymbol, 1000 style name, 1040 DIMSCALE, DIMASZ, DIMTXT, DIMEXO, DIMEXE and DIMGAP,
// 1070 DIMDEC, DIMZIN and DIMTAD, 1000 DIMDSEP, 1070 the number of text chunks and a 1000 for
// each (the Text override, split to fit the 255-character limit). Version 2 (what the writer
// writes) goes on with 1000 DIMBLK, DIMBLK1, DIMBLK2 and DIMTXSTY, 1070 DIMCLRD, DIMCLRE, DIMCLRT,
// DIMJUST and DIMATFIT, and 1070 flags (1 DIMSAH, 2 DIMTIH, 4 DIMTOH, 8 DIMTIX, 16 DIMTOFL);
// version 1 takes the ISO-25 values for those.
const dimXDataKind = "ARC_LENGTH"

// xcursor walks one application's extended data.
type xcursor struct {
	ps  []pair
	bad bool
}

func (c *xcursor) take(code int) string {
	if c.bad || len(c.ps) == 0 || c.ps[0].code != code {
		c.bad = true
		return ""
	}
	v := c.ps[0].val
	c.ps = c.ps[1:]
	return v
}

func (c *xcursor) num(code int) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(c.take(code)), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		c.bad = true
	}
	return v
}

func (c *xcursor) int() int {
	v, err := strconv.Atoi(strings.TrimSpace(c.take(1070)))
	if err != nil {
		c.bad = true
	}
	return v
}

func (c *xcursor) point() geom.Vec2 {
	x := c.num(1010)
	y := c.num(1020)
	c.num(1030)
	return geom.V(x, y)
}

// parseDimXData returns the dimension o's CADLITE_DIM extended data defines and its full style, or
// nil when it has none or it is malformed. The result has no EntityCommon and no style yet.
func parseDimXData(o object) (*doc.Dimension, doc.DimStyle) {
	var ps []pair
	in := false
	for _, p := range o.pairs {
		if p.code == 1001 {
			if in {
				break
			}
			in = strings.EqualFold(strings.TrimSpace(p.val), dimApp)
			continue
		}
		if in {
			ps = append(ps, p)
		}
	}
	if !in {
		return nil, doc.DimStyle{}
	}
	c := &xcursor{ps: ps}
	if strings.TrimSpace(c.take(1000)) != dimXDataKind {
		return nil, doc.DimStyle{}
	}
	version := c.int()
	if version != 1 && version != 2 {
		return nil, doc.DimStyle{}
	}
	e := &doc.Dimension{Kind: doc.DimArcLength}
	style := doc.DefaultDimStyle()
	e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Center = c.point(), c.point(), c.point(), c.point(), c.point()
	e.Radius, e.StartAngle, e.EndAngle, e.TextAngle = c.num(1040), c.num(1040), c.num(1040), c.num(1040)
	flags := c.int()
	e.TextMoved, e.Partial = flags&1 != 0, flags&2 != 0
	e.ArcSymbol = c.int()
	s := &style
	s.Name = decodeUnicode(c.take(1000))
	s.Scale, s.ArrowSize, s.TextHeight = c.num(1040), c.num(1040), c.num(1040)
	s.ExtOffset, s.ExtExtend, s.Gap = c.num(1040), c.num(1040), c.num(1040)
	s.Decimals, s.ZeroSupp = c.int(), c.int()
	s.TextVertical = c.int()
	s.DecSep = decodeUnicode(c.take(1000))
	var text strings.Builder
	for n := c.int(); n > 0 && !c.bad; n-- {
		text.WriteString(c.take(1000))
	}
	e.Text = decodeUnicode(text.String())
	if version == 2 {
		s.ArrowBlock, s.ArrowBlock1, s.ArrowBlock2 = decodeUnicode(c.take(1000)), decodeUnicode(c.take(1000)), decodeUnicode(c.take(1000))
		s.TextStyle = decodeUnicode(c.take(1000))
		ints := []*int{&s.LineColor, &s.ExtColor, &s.TextColor, &s.TextJust, &s.Fit}
		for _, p := range ints {
			*p = c.int()
		}
		flags := c.int()
		s.SeparateArrows, s.TextInsideHorizontal, s.TextOutsideHorizontal = flags&1 != 0, flags&2 != 0, flags&4 != 0
		s.TextInside, s.LineInside = flags&8 != 0, flags&16 != 0
	}
	if s.TextVertical < 0 || s.TextVertical > 4 || s.TextJust < 0 || s.TextJust > 4 || s.Fit < 0 || s.Fit > 3 {
		return nil, doc.DimStyle{}
	}
	if c.bad || e.ArcSymbol < 0 || e.ArcSymbol > 2 {
		return nil, doc.DimStyle{}
	}
	return e, style
}

// readDimInsert converts an INSERT carrying CADLITE_DIM extended data, or returns nil. Its full
// style becomes a style name and the variables that differ from main's record of that name.
func readDimInsert(o object, c doc.EntityCommon, main *doc.Drawing) doc.Entity {
	if !dimensionConvertible(o) {
		return nil
	}
	e, style := parseDimXData(o)
	e.SetStyle(main, style)
	e.EntityCommon = c
	e.Recompute()
	return e
}

// ---- writing ----

// dimOut is a dimension being written: the anonymous block holding its drawing and that block's
// BLOCK_RECORD handle.
type dimOut struct {
	e     *doc.Dimension
	block string
	rec   string
}

// hasDimensions reports whether d holds a doc.Dimension, in model space or in a block.
func hasDimensions(d *doc.Drawing) bool { return len(drawingDimensions(d)) > 0 }

// drawingDimensions returns the dimensions of model space and then of the blocks, in order.
func drawingDimensions(d *doc.Drawing) []*doc.Dimension {
	var out []*doc.Dimension
	for _, h := range d.Handles() {
		if e, ok := d.Entity(h).(*doc.Dimension); ok {
			out = append(out, e)
		}
	}
	for _, b := range d.Blocks() {
		for _, e := range b.Entities {
			if dm, ok := e.(*doc.Dimension); ok {
				out = append(out, dm)
			}
		}
	}
	return out
}

// planDimensions gives every dimension of d an unused anonymous block name *D<n> and a block
// record handle, and records them in w.
func (w *writer) planDimensions(d *doc.Drawing, next func() string) {
	used := map[string]bool{}
	for _, b := range d.Blocks() {
		used[strings.ToUpper(b.Name)] = true
	}
	for _, b := range d.Preserved.Blocks {
		used[strings.ToUpper(b.Name())] = true
	}
	w.dimNames = map[doc.Handle]string{}
	n := 0
	for _, e := range drawingDimensions(d) {
		name := ""
		for name == "" || used[name] {
			n++
			name = "*D" + strconv.Itoa(n)
		}
		used[name] = true
		w.dims = append(w.dims, dimOut{e: e, block: name, rec: next()})
		w.dimNames[e.Handle()] = name
	}
}

// dimStyleName is the DIMSTYLE a dimension is written with.
func dimStyleName(e *doc.Dimension) string {
	if e.StyleName == "" {
		return doc.DefaultDimStyle().Name
	}
	return e.StyleName
}

// dimStyleApp is the application of the extended data an R12 DIMSTYLE record carries for the
// variables R12 lacks: 1070 DIMDEC, 1070 DIMDSEP (a character code), 1070 DIMJUST, 1070
// DIMATFIT and 1000 DIMTXSTY. The reader restores them; other programs ignore them.
const dimStyleApp = "CADLITE_DIMSTYLE"

// r12DimCodes are the DIMSTYLE group codes R12 defines; an R12 record keeps only these of the
// unmodelled groups.
func r12DimCode(c int) bool {
	return c >= 3 && c <= 7 || c >= 40 && c <= 48 || c >= 140 && c <= 147 || c >= 71 && c <= 78 || c >= 170 && c <= 178
}

// dimStyleRecord returns the DIMSTYLE record of s with its unmodelled groups extra: every
// modelled variable, the arrow blocks as names (R12, groups 5 to 7) or block record handles
// (AC1015, 342 to 344, from w.arrowHandles), DIMTXSTY as a STYLE handle (AC1015, 340). With xdata,
// an R12 record also carries dimStyleApp.
func (w *writer) dimStyleRecord(s doc.DimStyle, extra []doc.Pair, xdata bool) []doc.Pair {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	i := strconv.Itoa
	b := func(v bool) string { return i(boolInt(v)) }
	ps := []doc.Pair{{Code: 0, Value: "DIMSTYLE"}, {Code: 2, Value: s.Name}, {Code: 70, Value: "0"}}
	for _, p := range extra {
		if p.Code < 1000 && (w.modern || r12DimCode(p.Code)) {
			ps = append(ps, p)
		}
	}
	ps = append(ps, doc.Pair{Code: 40, Value: f(s.Scale)}, doc.Pair{Code: 41, Value: f(s.ArrowSize)},
		doc.Pair{Code: 42, Value: f(s.ExtOffset)}, doc.Pair{Code: 44, Value: f(s.ExtExtend)},
		doc.Pair{Code: 73, Value: b(s.TextInsideHorizontal)}, doc.Pair{Code: 74, Value: b(s.TextOutsideHorizontal)},
		doc.Pair{Code: 77, Value: i(s.TextVertical)}, doc.Pair{Code: 78, Value: i(s.ZeroSupp)},
		doc.Pair{Code: 140, Value: f(s.TextHeight)}, doc.Pair{Code: 147, Value: f(s.Gap)},
		doc.Pair{Code: 172, Value: b(s.LineInside)}, doc.Pair{Code: 173, Value: b(s.SeparateArrows)},
		doc.Pair{Code: 174, Value: b(s.TextInside)}, doc.Pair{Code: 176, Value: i(s.LineColor)},
		doc.Pair{Code: 177, Value: i(s.ExtColor)}, doc.Pair{Code: 178, Value: i(s.TextColor)})
	arrows := [3]string{s.ArrowBlock, s.ArrowBlock1, s.ArrowBlock2}
	if w.modern {
		ps = append(ps, doc.Pair{Code: 271, Value: i(s.Decimals)}, doc.Pair{Code: 278, Value: i(decSepCode(s.DecSep))},
			doc.Pair{Code: 280, Value: i(s.TextJust)}, doc.Pair{Code: 289, Value: i(s.Fit)})
		if h := w.styleHandles[strings.ToUpper(dimTextStyle(s))]; h != "" {
			ps = append(ps, doc.Pair{Code: 340, Value: h})
		}
		for k, a := range arrows {
			if h := w.arrowHandles[strings.ToUpper(a)]; a != "" && h != "" {
				ps = append(ps, doc.Pair{Code: 342 + k, Value: h})
			}
		}
	} else {
		for k, a := range arrows {
			if a != "" {
				ps = append(ps, doc.Pair{Code: 5 + k, Value: a})
			}
		}
	}
	for _, p := range extra {
		if p.Code >= 1000 {
			ps = append(ps, p)
		}
	}
	if xdata && !w.modern {
		ps = append(ps, doc.Pair{Code: 1001, Value: dimStyleApp}, doc.Pair{Code: 1070, Value: i(s.Decimals)},
			doc.Pair{Code: 1070, Value: i(decSepCode(s.DecSep))}, doc.Pair{Code: 1070, Value: i(s.TextJust)},
			doc.Pair{Code: 1070, Value: i(s.Fit)}, doc.Pair{Code: 1000, Value: encodeText(dimTextStyle(s))})
	}
	return ps
}

// dimTextStyle is DIMTXSTY as written: "" is Standard.
func dimTextStyle(s doc.DimStyle) string {
	if s.TextStyle == "" {
		return doc.DefaultTextStyle
	}
	return s.TextStyle
}

// readDimStyleXData applies a record's dimStyleApp extended data (from xd, its pairs after the
// 1001 group) to s.
func readDimStyleXData(s *doc.DimStyle, xd []pair) {
	c := &xcursor{ps: xd}
	dec, sep, just, fit := c.int(), c.int(), c.int(), c.int()
	style := decodeUnicode(c.take(1000))
	if c.bad {
		return
	}
	s.Decimals = min(max(dec, 0), 8)
	dimVar(s, nil, 278, strconv.Itoa(sep), nil)
	dimVar(s, nil, 280, strconv.Itoa(just), nil)
	dimVar(s, nil, 289, strconv.Itoa(fit), nil)
	if style != "" {
		s.TextStyle = style
	}
}

// dimStyleTable returns the DIMSTYLE records to write: the drawing's table, then one for each
// style a linear dimension names that the table lacks (the ISO-25 values under that name). It
// records in w how a reader that ignores dimStyleApp sees every record, so the dimensions write
// their differences from it as overrides, which other programs read too.
func (w *writer) dimStyleTable(d *doc.Drawing) [][]doc.Pair {
	w.dimStyles, w.dimStylesOwn = map[string]doc.DimStyle{}, map[string]doc.DimStyle{}
	var out [][]doc.Pair
	add := func(s doc.DimStyle, extra []doc.Pair) {
		s.TextStyle = dimTextStyle(s)
		view := s
		if !w.modern {
			view, _, _ = parseDimStyle(objectsFromRaw(w.dimStyleRecord(s, nil, false))[0], nil)
		}
		w.dimStyles[strings.ToUpper(s.Name)] = view
		w.dimStylesOwn[strings.ToUpper(s.Name)] = s
		out = append(out, w.dimStyleRecord(s, extra, true))
	}
	for _, s := range d.DimStyles() {
		add(s, d.DimStyleExtra(s.Name))
	}
	for _, o := range w.dims {
		if o.e.Kind == doc.DimArcLength {
			continue
		}
		name := dimStyleName(o.e)
		if _, ok := w.dimStyles[strings.ToUpper(name)]; ok {
			continue
		}
		s := doc.DefaultDimStyle()
		s.Name = name
		add(s, nil)
	}
	return out
}

// dimension writes a doc.Dimension: a DIMENSION record for a linear one, an INSERT with
// CADLITE_DIM extended data for an arc length one.
func (w *writer) dimension(e *doc.Dimension) {
	name := w.dimNames[e.Handle()]
	if e.Kind == doc.DimArcLength {
		w.head("INSERT", e)
		w.pair(2, name)
		w.point(10, geom.Vec2{})
		w.dimXData(e)
		return
	}
	w.head("DIMENSION", e)
	w.pair(2, name)
	w.point(10, e.DimLine)
	w.point(11, e.TextMiddle())
	flags := int(e.Kind) | 32
	if e.TextMoved {
		flags |= 128
	}
	w.int(70, flags)
	if w.modern {
		w.int(71, 5)
		w.num(42, e.Measurement())
	}
	if e.Text != "" {
		w.pair(1, encodeText(e.Text))
	}
	if e.TextAngle != 0 {
		w.num(53, e.TextAngle*180/math.Pi)
	}
	w.pair(3, dimStyleName(e))
	if w.modern {
		w.pair(100, "AcDbAlignedDimension")
	}
	w.point(13, e.Ext1)
	w.point(14, e.Ext2)
	if e.Kind == doc.DimRotated {
		w.num(50, e.Rotation*180/math.Pi)
		if w.modern {
			w.pair(100, "AcDbRotatedDimension")
		}
	}
	w.dstyle(e)
}

// dstyle writes the style values of e that differ from its DIMSTYLE record as ACAD DSTYLE
// extended data: 1040 for sizes, 1070 for integers and flags, and the arrow blocks and text
// style as 1005 handles (AC1015) or the arrow blocks as 1000 names under groups 5 to 7 (R12,
// which cannot override DIMTXSTY). A value is an override when it differs from the record as
// this reader reads it or as a reader that ignores CADLITE_DIMSTYLE reads it (R12).
func (w *writer) dstyle(e *doc.Dimension) {
	key := strings.ToUpper(dimStyleName(e))
	r, ok := w.dimStyles[key]
	if !ok {
		r = doc.DefaultDimStyle()
	}
	ovs := w.styleDiffs(e.Style(), r)
	if own, ok := w.dimStylesOwn[key]; ok {
		seen := map[int]bool{}
		for _, o := range ovs {
			seen[o.code] = true
		}
		for _, o := range w.styleDiffs(e.Style(), own) {
			if !seen[o.code] {
				ovs = append(ovs, o)
			}
		}
	}
	if len(ovs) == 0 {
		return
	}
	w.pair(1001, acadApp)
	w.pair(1000, "DSTYLE")
	w.pair(1002, "{")
	for _, o := range ovs {
		w.int(1070, o.code)
		w.pair(o.xcode, o.val)
	}
	w.pair(1002, "}")
}

// dimOverride is one DSTYLE override: the variable's group code, the extended data code of its
// value and the value.
type dimOverride struct {
	code, xcode int
	val         string
}

// styleDiffs returns the overrides that make record r into s.
func (w *writer) styleDiffs(s, r doc.DimStyle) []dimOverride {
	var ovs []dimOverride
	f := func(code int, a, b float64) {
		if a != b {
			ovs = append(ovs, dimOverride{code, 1040, strconv.FormatFloat(a, 'f', -1, 64)})
		}
	}
	i := func(code, a, b int) {
		if a != b {
			ovs = append(ovs, dimOverride{code, 1070, strconv.Itoa(a)})
		}
	}
	bl := func(code int, a, b bool) { i(code, boolInt(a), boolInt(b)) }
	f(40, s.Scale, r.Scale)
	f(41, s.ArrowSize, r.ArrowSize)
	f(42, s.ExtOffset, r.ExtOffset)
	f(44, s.ExtExtend, r.ExtExtend)
	f(140, s.TextHeight, r.TextHeight)
	f(147, s.Gap, r.Gap)
	i(271, s.Decimals, r.Decimals)
	i(278, decSepCode(s.DecSep), decSepCode(r.DecSep))
	i(78, s.ZeroSupp, r.ZeroSupp)
	i(77, s.TextVertical, r.TextVertical)
	i(280, s.TextJust, r.TextJust)
	i(289, s.Fit, r.Fit)
	bl(73, s.TextInsideHorizontal, r.TextInsideHorizontal)
	bl(74, s.TextOutsideHorizontal, r.TextOutsideHorizontal)
	bl(172, s.LineInside, r.LineInside)
	bl(173, s.SeparateArrows, r.SeparateArrows)
	bl(174, s.TextInside, r.TextInside)
	i(176, s.LineColor, r.LineColor)
	i(177, s.ExtColor, r.ExtColor)
	i(178, s.TextColor, r.TextColor)
	for k, a := range [3][2]string{{s.ArrowBlock, r.ArrowBlock}, {s.ArrowBlock1, r.ArrowBlock1}, {s.ArrowBlock2, r.ArrowBlock2}} {
		if strings.EqualFold(a[0], a[1]) {
			continue
		}
		if w.modern {
			h := "0"
			if a[0] != "" {
				h = w.arrowHandles[strings.ToUpper(a[0])]
			}
			if h != "" {
				ovs = append(ovs, dimOverride{342 + k, 1005, h})
			}
		} else {
			ovs = append(ovs, dimOverride{5 + k, 1000, a[0]})
		}
	}
	if w.modern && !strings.EqualFold(dimTextStyle(s), dimTextStyle(r)) {
		if h := w.styleHandles[strings.ToUpper(dimTextStyle(s))]; h != "" {
			ovs = append(ovs, dimOverride{340, 1005, h})
		}
	}
	return ovs
}

// decSepCode is DIMDSEP as DXF stores it, a character code ("" is ".").
func decSepCode(s string) int {
	if s == "" {
		return '.'
	}
	return int(s[0])
}

// dimXData writes an arc length dimension's CADLITE_DIM extended data, version 2 (see
// dimXDataKind).
func (w *writer) dimXData(e *doc.Dimension) {
	w.pair(1001, dimApp)
	w.pair(1000, dimXDataKind)
	w.int(1070, 2)
	for _, p := range []geom.Vec2{e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Center} {
		w.point(1010, p)
	}
	for _, v := range []float64{e.Radius, e.StartAngle, e.EndAngle, e.TextAngle} {
		w.num(1040, v)
	}
	w.int(1070, boolInt(e.TextMoved)|boolInt(e.Partial)<<1)
	w.int(1070, e.ArcSymbol)
	s := e.Style()
	w.pair(1000, encodeText(s.Name))
	for _, v := range []float64{s.Scale, s.ArrowSize, s.TextHeight, s.ExtOffset, s.ExtExtend, s.Gap} {
		w.num(1040, v)
	}
	w.int(1070, s.Decimals)
	w.int(1070, s.ZeroSupp)
	w.int(1070, s.TextVertical)
	w.pair(1000, encodeText(s.DecSep))
	text := encodeText(e.Text)
	var chunks []string
	for len(text) > 250 {
		chunks, text = append(chunks, text[:250]), text[250:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	w.int(1070, len(chunks))
	for _, c := range chunks {
		w.pair(1000, c)
	}
	for _, v := range []string{s.ArrowBlock, s.ArrowBlock1, s.ArrowBlock2, s.TextStyle} {
		w.pair(1000, encodeText(v))
	}
	for _, v := range []int{s.LineColor, s.ExtColor, s.TextColor, s.TextJust, s.Fit} {
		w.int(1070, v)
	}
	w.int(1070, boolInt(s.SeparateArrows)|boolInt(s.TextInsideHorizontal)<<1|boolInt(s.TextOutsideHorizontal)<<2|
		boolInt(s.TextInside)<<3|boolInt(s.LineInside)<<4)
}

// blockParts builds block entities: com makes the common fields of a new one with a colour.
type blockParts struct {
	out []doc.Entity
	com func(color int) doc.EntityCommon
	rec string
}

func (b *blockParts) line(l geom.Line, color int) {
	if l.A != l.B {
		b.out = append(b.out, &doc.Line{EntityCommon: b.com(color), Start: l.A, End: l.B})
	}
}

func (b *blockParts) arc(a geom.Arc, color int) {
	if a.R > 0 && a.Start != a.End {
		b.out = append(b.out, &doc.Arc{EntityCommon: b.com(color), Center: a.C, Radius: a.R, StartAngle: a.Start, EndAngle: a.End})
	}
}

// raw adds an entity the document does not model, as its groups.
func (b *blockParts) raw(typ, sub string, c doc.EntityCommon, ps ...doc.Pair) {
	r := []doc.Pair{{Code: 5, Value: c.H.String()}, {Code: 330, Value: b.rec}, {Code: 100, Value: "AcDbEntity"},
		{Code: 8, Value: c.Layer}}
	if c.Linetype != "" {
		r = append(r, doc.Pair{Code: 6, Value: c.Linetype})
	}
	if c.Color != doc.ByLayer {
		r = append(r, doc.Pair{Code: 62, Value: strconv.Itoa(c.Color)})
	}
	r = append(r, doc.Pair{Code: 100, Value: sub})
	b.out = append(b.out, &doc.Unknown{EntityCommon: c, Type: typ, Raw: append(r, ps...)})
}

func ptPairs(code int, p geom.Vec2) []doc.Pair {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return []doc.Pair{{Code: code, Value: f(p.X)}, {Code: code + 10, Value: f(p.Y)}, {Code: code + 20, Value: "0"}}
}

// arrow adds an arrowhead: a SOLID for a filled outline (a triangle, or a quadrilateral in
// SOLID's zigzag order), LINEs for an open outline and strokes, CIRCLEs for dots and origin
// indicators (a filled dot is a circle here, AutoCAD's a donut), ARCs for the integral sign.
func (b *blockParts) arrow(a doc.DimArrow, color int) {
	if n := len(a.Outline); n > 0 {
		if a.Filled && (n == 3 || n == 4) {
			pts := [4]geom.Vec2{a.Outline[0], a.Outline[1], a.Outline[2], a.Outline[2]}
			if n == 4 {
				pts = [4]geom.Vec2{a.Outline[0], a.Outline[1], a.Outline[3], a.Outline[2]}
			}
			var ps []doc.Pair
			for i, p := range pts {
				ps = append(ps, ptPairs(10+i, p)...)
			}
			b.raw("SOLID", "AcDbTrace", b.com(color), ps...)
		} else {
			for i := range a.Outline {
				b.line(geom.Line{A: a.Outline[i], B: a.Outline[(i+1)%n]}, color)
			}
		}
	}
	for _, l := range a.Lines {
		b.line(l, color)
	}
	for _, c := range a.Circles {
		if c.R > 0 {
			b.out = append(b.out, &doc.Circle{EntityCommon: b.com(color), Center: c.C, Radius: c.R})
		}
	}
	for _, c := range a.Arcs {
		b.arc(c, color)
	}
}

// dimBlockEntities returns the drawing of a dimension as block entities owned by the block
// record rec: LINE and ARC for the lines, the arrowheads (blockParts.arrow), a middle-centred
// TEXT and POINT definition points on DEFPOINTS. The lines, arrows and text are on layer 0 with
// linetype and lineweight BYBLOCK and the colours DIMCLRE (extension lines), DIMCLRD (dimension
// line and arrows) and DIMCLRT (text and arc symbol), BYBLOCK by default, so they take the
// dimension's properties, as AutoCAD's do.
func dimBlockEntities(e *doc.Dimension, rec string, next func() string) []doc.Entity {
	owner, _ := doc.ParseHandle(rec)
	b := &blockParts{rec: rec, com: func(color int) doc.EntityCommon {
		h, _ := doc.ParseHandle(next())
		return doc.EntityCommon{H: h, Layer: "0", Color: color, Owner: owner, Linetype: doc.LinetypeByBlock, Lineweight: doc.LineweightByBlock}
	}}
	st := e.Style()
	lay := e.Layout()
	for _, l := range lay.ExtLines {
		b.line(l, st.ExtColor)
	}
	if e.Kind == doc.DimArcLength {
		b.arc(lay.DimArc, st.LineColor)
	}
	for _, l := range lay.DimLine {
		b.line(l, st.LineColor)
	}
	for _, a := range lay.Arrows {
		b.arrow(a, st.LineColor)
	}
	b.arc(lay.Symbol, st.TextColor)
	if lay.Text != nil && lay.Text.Value != "" {
		t := lay.Text.Clone().(*doc.Text)
		t.EntityCommon = b.com(st.TextColor)
		b.out = append(b.out, t)
	}
	for _, p := range []geom.Vec2{e.Ext1, e.Ext2, e.DimLine} {
		c := b.com(doc.ByLayer)
		c.Layer, c.Linetype, c.Lineweight = defpointsLayer, "", doc.LineweightByLayer
		b.raw("POINT", "AcDbPoint", c, ptPairs(10, p)...)
	}
	return b.out
}

// arrowBlockEntities returns AutoCAD's unit arrow block of the given name (doc.LayoutArrow at the
// origin, pointing along +X) as block entities owned by rec, on layer 0 and BYBLOCK.
func arrowBlockEntities(name, rec string, next func() string) []doc.Entity {
	owner, _ := doc.ParseHandle(rec)
	b := &blockParts{rec: rec, com: func(color int) doc.EntityCommon {
		h, _ := doc.ParseHandle(next())
		return doc.EntityCommon{H: h, Layer: "0", Color: color, Owner: owner, Linetype: doc.LinetypeByBlock, Lineweight: doc.LineweightByBlock}
	}}
	b.arrow(doc.LayoutArrow(name, geom.V(0, 0), geom.V(1, 0), 1), doc.ByBlock)
	return b.out
}

// arrowOut is an arrow block the writer generates because the drawing lacks it.
type arrowOut struct{ name, rec string }

// arrowNames returns the arrow block names (DIMBLK, DIMBLK1, DIMBLK2) the dimension style table
// and the dimensions use, other than closed filled, first spelling of each kept.
func arrowNames(d *doc.Drawing) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s doc.DimStyle) {
		for _, a := range []string{s.ArrowBlock, s.ArrowBlock1, s.ArrowBlock2} {
			if u := strings.ToUpper(a); a != "" && !seen[u] {
				seen[u] = true
				out = append(out, a)
			}
		}
	}
	for _, s := range d.DimStyles() {
		add(s)
	}
	for _, e := range drawingDimensions(d) {
		add(e.Style())
	}
	return out
}

// planArrows gives every arrow block name the styles use a block record handle: the drawing's
// block or preserved block of that name, else a generated block (arrowBlockEntities).
func (w *writer) planArrows(d *doc.Drawing, preserved func(name string) string, next func() string) {
	w.arrowHandles = map[string]string{}
	for _, name := range arrowNames(d) {
		u := strings.ToUpper(name)
		if b := d.Block(name); b != nil {
			w.arrowHandles[u] = b.H.String()
			continue
		}
		if h := preserved(name); h != "" {
			w.arrowHandles[u] = h
			continue
		}
		rec := next()
		w.arrowHandles[u] = rec
		w.arrowGen = append(w.arrowGen, arrowOut{name: name, rec: rec})
	}
}

// dimApps returns the applications the dimensions' extended data needs: ACAD for the DSTYLE
// overrides of linear dimensions, CADLITE_DIM for arc length ones.
func (w *writer) dimApps() []string {
	var lin, arc bool
	for _, o := range w.dims {
		if o.e.Kind == doc.DimArcLength {
			arc = true
		} else {
			lin = true
		}
	}
	var out []string
	if lin {
		out = append(out, acadApp)
	}
	if arc {
		out = append(out, dimApp)
	}
	return out
}

// hasRecord reports whether d's preserved table holds a record named name (any case).
func hasRecord(d *doc.Drawing, table, name string) bool {
	for _, r := range d.Preserved.Tables[table] {
		if strings.EqualFold(r.Name(), name) {
			return true
		}
	}
	return false
}

// hasLayer reports whether d has a layer named name (any case, as DXF compares names).
func hasLayer(d *doc.Drawing, name string) bool {
	for _, l := range d.Layers() {
		if strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}
