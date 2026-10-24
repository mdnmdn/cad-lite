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
		ExtExtend: 0.18, Gap: 0.09, Decimals: 4, DecSep: ".", ZeroSupp: 0, TextAbove: false}
}

// dimVar applies dimension variable code (its DIMSTYLE group code, also the code DSTYLE
// overrides use) with value v to s and *arcSym. Codes the document does not model are ignored.
func dimVar(s *doc.DimStyle, arcSym *int, code int, v string) {
	v = strings.TrimSpace(v)
	f, ferr := strconv.ParseFloat(v, 64)
	if ferr != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		f, ferr = 0, strconv.ErrSyntax
	}
	i, ierr := strconv.Atoi(v)
	if ierr != nil && ferr == nil {
		i, ierr = int(f), nil
	}
	switch code {
	case 40:
		if ferr == nil {
			s.Scale = f
		}
	case 41:
		if ferr == nil {
			s.ArrowSize = f
		}
	case 42:
		if ferr == nil {
			s.ExtOffset = f
		}
	case 44:
		if ferr == nil {
			s.ExtExtend = f
		}
	case 140:
		if ferr == nil {
			s.TextHeight = f
		}
	case 147:
		if ferr == nil {
			s.Gap = f
		}
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
		if ierr == nil {
			s.TextAbove = i != 0
		}
	case 90:
		if ierr == nil && arcSym != nil && i >= 0 && i <= 2 {
			*arcSym = i
		}
	}
}

// dimStyleRec is a DIMSTYLE record as the document sees it.
type dimStyleRec struct {
	style  doc.DimStyle
	arcSym int
}

// parseDimStyle reads a DIMSTYLE record (R12 or R2000 and later: both keep the variables in
// the groups dimVar reads; R12 has no DIMDEC or DIMDSEP); a variable it lacks takes AutoCAD's
// built-in default.
func parseDimStyle(o object) dimStyleRec {
	r := dimStyleRec{style: acadDimDefaults(o.str(2))}
	for _, p := range o.pairs {
		if p.code >= 1000 {
			break
		}
		dimVar(&r.style, &r.arcSym, p.code, p.val)
	}
	return r
}

// dimStyles returns the DIMSTYLE records of d by upper-cased name.
func dimStyles(d *doc.Drawing) map[string]dimStyleRec {
	m := map[string]dimStyleRec{}
	for _, r := range d.Preserved.Tables["DIMSTYLE"] {
		if os := objectsFromRaw(r.Pairs); len(os) > 0 {
			m[strings.ToUpper(r.Name())] = parseDimStyle(os[0])
		}
	}
	return m
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
// ISO-25 defaults when the table lacks it) and o's DSTYLE overrides, and the DIMARCSYM value.
func dimStyleOf(name string, o object, styles map[string]dimStyleRec) (doc.DimStyle, int) {
	r, ok := styles[strings.ToUpper(name)]
	if !ok {
		r = dimStyleRec{style: doc.DefaultDimStyle()}
		if name != "" {
			r.style.Name = name
		}
	}
	for _, kv := range dstyleOverrides(o) {
		code, _ := strconv.Atoi(kv[0])
		dimVar(&r.style, &r.arcSym, code, kv[1])
	}
	return r.style, r.arcSym
}

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
		return o.int(66) != 1 && parseDimXData(o) != nil
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
	if st.dimStyles == nil {
		st.dimStyles = map[string]dimStyleRec{}
		if st.main != nil {
			st.dimStyles = dimStyles(st.main)
		}
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
	e.Style, e.ArcSymbol = dimStyleOf(base.str(3), o, st.dimStyles)
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
// each (the Text override, split to fit the 255-character limit).
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

// parseDimXData returns the dimension o's CADLITE_DIM extended data defines, or nil when it has
// none or it is malformed. The result has no EntityCommon.
func parseDimXData(o object) *doc.Dimension {
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
		return nil
	}
	c := &xcursor{ps: ps}
	if strings.TrimSpace(c.take(1000)) != dimXDataKind || c.int() != 1 {
		return nil
	}
	e := &doc.Dimension{Kind: doc.DimArcLength}
	e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Center = c.point(), c.point(), c.point(), c.point(), c.point()
	e.Radius, e.StartAngle, e.EndAngle, e.TextAngle = c.num(1040), c.num(1040), c.num(1040), c.num(1040)
	flags := c.int()
	e.TextMoved, e.Partial = flags&1 != 0, flags&2 != 0
	e.ArcSymbol = c.int()
	s := &e.Style
	s.Name = decodeUnicode(c.take(1000))
	s.Scale, s.ArrowSize, s.TextHeight = c.num(1040), c.num(1040), c.num(1040)
	s.ExtOffset, s.ExtExtend, s.Gap = c.num(1040), c.num(1040), c.num(1040)
	s.Decimals, s.ZeroSupp = c.int(), c.int()
	s.TextAbove = c.int() != 0
	s.DecSep = decodeUnicode(c.take(1000))
	var text strings.Builder
	for n := c.int(); n > 0 && !c.bad; n-- {
		text.WriteString(c.take(1000))
	}
	e.Text = decodeUnicode(text.String())
	if c.bad || e.ArcSymbol < 0 || e.ArcSymbol > 2 {
		return nil
	}
	return e
}

// readDimInsert converts an INSERT carrying CADLITE_DIM extended data, or returns nil.
func readDimInsert(o object, c doc.EntityCommon) doc.Entity {
	if !dimensionConvertible(o) {
		return nil
	}
	e := parseDimXData(o)
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
	if e.Style.Name == "" {
		return doc.DefaultDimStyle().Name
	}
	return e.Style.Name
}

// generatedDimStyle is the DIMSTYLE record written for a style name the drawing's table lacks:
// the ISO-25 values under that name (R12 has no DIMDEC or DIMDSEP).
func generatedDimStyle(name string, modern bool) []doc.Pair {
	s := doc.DefaultDimStyle()
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	ps := []doc.Pair{{Code: 0, Value: "DIMSTYLE"}, {Code: 2, Value: name}, {Code: 70, Value: "0"},
		{Code: 40, Value: f(s.Scale)}, {Code: 41, Value: f(s.ArrowSize)}, {Code: 42, Value: f(s.ExtOffset)},
		{Code: 44, Value: f(s.ExtExtend)}, {Code: 77, Value: strconv.Itoa(boolInt(s.TextAbove))},
		{Code: 78, Value: strconv.Itoa(s.ZeroSupp)}, {Code: 140, Value: f(s.TextHeight)}, {Code: 147, Value: f(s.Gap)}}
	if modern {
		ps = append(ps, doc.Pair{Code: 271, Value: strconv.Itoa(s.Decimals)}, doc.Pair{Code: 278, Value: strconv.Itoa(int(s.DecSep[0]))})
	}
	return ps
}

// dimStyleTable returns the DIMSTYLE records to write after the preserved ones, one for each
// style a linear dimension names that the table lacks, and records in w how a reader will see
// every record, so the dimensions write only their differences as overrides.
func (w *writer) dimStyleTable(d *doc.Drawing) [][]doc.Pair {
	w.dimStyles = dimStyles(d)
	var add [][]doc.Pair
	for _, o := range w.dims {
		if o.e.Kind == doc.DimArcLength {
			continue
		}
		name := dimStyleName(o.e)
		if _, ok := w.dimStyles[strings.ToUpper(name)]; ok {
			continue
		}
		ps := generatedDimStyle(name, w.modern)
		add = append(add, ps)
		w.dimStyles[strings.ToUpper(name)] = parseDimStyle(objectsFromRaw(ps)[0])
	}
	return add
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
	w.point(11, e.TextPos)
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
// extended data.
func (w *writer) dstyle(e *doc.Dimension) {
	rec, ok := w.dimStyles[strings.ToUpper(dimStyleName(e))]
	if !ok {
		rec = dimStyleRec{style: doc.DefaultDimStyle()}
	}
	r, s := rec.style, e.Style
	type ov struct {
		code, xcode int
		val         string
	}
	var ovs []ov
	f := func(code int, a, b float64) {
		if a != b {
			ovs = append(ovs, ov{code, 1040, strconv.FormatFloat(a, 'f', -1, 64)})
		}
	}
	i := func(code, a, b int) {
		if a != b {
			ovs = append(ovs, ov{code, 1070, strconv.Itoa(a)})
		}
	}
	f(40, s.Scale, r.Scale)
	f(41, s.ArrowSize, r.ArrowSize)
	f(42, s.ExtOffset, r.ExtOffset)
	f(44, s.ExtExtend, r.ExtExtend)
	f(140, s.TextHeight, r.TextHeight)
	f(147, s.Gap, r.Gap)
	i(271, s.Decimals, r.Decimals)
	i(278, decSepCode(s.DecSep), decSepCode(r.DecSep))
	i(78, s.ZeroSupp, r.ZeroSupp)
	i(77, boolInt(s.TextAbove), boolInt(r.TextAbove))
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

// decSepCode is DIMDSEP as DXF stores it, a character code ("" is ".").
func decSepCode(s string) int {
	if s == "" {
		return '.'
	}
	return int(s[0])
}

// dimXData writes an arc length dimension's CADLITE_DIM extended data (see dimXDataKind).
func (w *writer) dimXData(e *doc.Dimension) {
	w.pair(1001, dimApp)
	w.pair(1000, dimXDataKind)
	w.int(1070, 1)
	for _, p := range []geom.Vec2{e.Ext1, e.Ext2, e.DimLine, e.TextPos, e.Center} {
		w.point(1010, p)
	}
	for _, v := range []float64{e.Radius, e.StartAngle, e.EndAngle, e.TextAngle} {
		w.num(1040, v)
	}
	w.int(1070, boolInt(e.TextMoved)|boolInt(e.Partial)<<1)
	w.int(1070, e.ArcSymbol)
	s := e.Style
	w.pair(1000, encodeText(s.Name))
	for _, v := range []float64{s.Scale, s.ArrowSize, s.TextHeight, s.ExtOffset, s.ExtExtend, s.Gap} {
		w.num(1040, v)
	}
	w.int(1070, s.Decimals)
	w.int(1070, s.ZeroSupp)
	w.int(1070, boolInt(s.TextAbove))
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
}

// dimBlockEntities returns the drawing of a dimension as block entities owned by the block
// record rec: LINE and ARC for the lines, SOLID arrowheads, a middle-centred TEXT and POINT
// definition points on DEFPOINTS. The lines and text are on layer 0 with colour BYBLOCK, so
// they take the dimension's layer and colour, as AutoCAD's are.
func dimBlockEntities(e *doc.Dimension, rec string, next func() string) []doc.Entity {
	owner, _ := doc.ParseHandle(rec)
	com := func() doc.EntityCommon {
		h, _ := doc.ParseHandle(next())
		return doc.EntityCommon{H: h, Layer: "0", Color: 0, Owner: owner}
	}
	lay := e.Layout()
	var out []doc.Entity
	line := func(l geom.Line) {
		if l.A != l.B {
			out = append(out, &doc.Line{EntityCommon: com(), Start: l.A, End: l.B})
		}
	}
	arc := func(a geom.Arc) {
		if a.R > 0 && a.Start != a.End {
			out = append(out, &doc.Arc{EntityCommon: com(), Center: a.C, Radius: a.R, StartAngle: a.Start, EndAngle: a.End})
		}
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	raw := func(typ, sub string, c doc.EntityCommon, ps ...doc.Pair) {
		r := []doc.Pair{{Code: 5, Value: c.H.String()}, {Code: 330, Value: rec}, {Code: 100, Value: "AcDbEntity"},
			{Code: 8, Value: c.Layer}}
		if c.Color != doc.ByLayer {
			r = append(r, doc.Pair{Code: 62, Value: strconv.Itoa(c.Color)})
		}
		r = append(r, doc.Pair{Code: 100, Value: sub})
		out = append(out, &doc.Unknown{EntityCommon: c, Type: typ, Raw: append(r, ps...)})
	}
	pt := func(code int, p geom.Vec2) []doc.Pair {
		return []doc.Pair{{Code: code, Value: f(p.X)}, {Code: code + 10, Value: f(p.Y)}, {Code: code + 20, Value: "0"}}
	}
	for _, l := range lay.ExtLines {
		line(l)
	}
	if e.Kind == doc.DimArcLength {
		arc(lay.DimArc)
	}
	for _, l := range lay.DimLine {
		line(l)
	}
	for _, a := range lay.Arrows {
		var ps []doc.Pair
		for i, p := range [4]geom.Vec2{a[0], a[1], a[2], a[2]} {
			ps = append(ps, pt(10+i, p)...)
		}
		raw("SOLID", "AcDbTrace", com(), ps...)
	}
	arc(lay.Symbol)
	if lay.Text != nil && lay.Text.Value != "" {
		t := lay.Text.Clone().(*doc.Text)
		t.EntityCommon = com()
		out = append(out, t)
	}
	for _, p := range []geom.Vec2{e.Ext1, e.Ext2, e.DimLine} {
		c := com()
		c.Layer = defpointsLayer
		raw("POINT", "AcDbPoint", c, pt(10, p)...)
	}
	return out
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
