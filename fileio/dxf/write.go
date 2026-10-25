package dxf

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
)

type writer struct {
	w            *bufio.Writer
	err          error
	modern       bool
	d            *doc.Drawing
	modelOwner   string
	typeOverride string
	reactors     map[doc.Handle][]doc.Handle
	// mtexts counts the MTEXT written as TEXT by the R12 symbol path (r12Losses reports them).
	mtexts int
	// dims are the dimensions being written with their generated blocks, and dimNames those
	// blocks' names by dimension handle (planDimensions).
	dims     []dimOut
	dimNames map[doc.Handle]string
	// dimStyles are the written DIMSTYLE records as a reader sees them (dimStyleTable).
	dimStyles map[string]doc.DimStyle
	// dimStylesOwn are the same records as this reader sees them (with CADLITE_DIMSTYLE).
	dimStylesOwn map[string]doc.DimStyle
	// arrowHandles are the block record handles of the arrow blocks by upper-cased name, and
	// arrowGen the arrow blocks to generate (planArrows; AC1015 handles, R12 names only).
	arrowHandles map[string]string
	arrowGen     []arrowOut
	// styleHandles are the STYLE record handles by upper-cased name (AC1015).
	styleHandles map[string]string
	// ltNames are the written linetype names by upper-cased name, and ltRecords the records
	// (planLinetypes).
	ltNames   map[string]string
	ltRecords []doc.Linetype
}

func (w *writer) pair(code int, val string) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, "%3d\n%s\n", code, val)
}

func (w *writer) num(code int, v float64) { w.pair(code, strconv.FormatFloat(v, 'f', -1, 64)) }

func (w *writer) int(code, v int) { w.pair(code, strconv.Itoa(v)) }

func (w *writer) point(code int, p geom.Vec2) {
	w.num(code, p.X)
	w.num(code+10, p.Y)
	w.num(code+20, 0)
}

// Write implements fileio.Writer: WriteVersion with no version, which picks R12 (AC1009) unless
// the drawing needs AutoCAD 2000 (AC1015). writeR12 writes R12: LWPOLYLINE becomes
// POLYLINE/VERTEX/SEQEND and entity handles are kept. VERTEX and SEQEND get
// handles above the drawing's seed, which are not part of the document.
// Header variables the document does not model are not written.
//
// Unknown entities, and the tables and blocks in Drawing.Preserved, are
// written verbatim, group for group. R12 has no SPLINE, MTEXT, HATCH and the
// like, so an unknown entity whose class is not in r12Entities is not
// written and a warning names it; the other classes are written as read,
// without the groups R12 lacks (see pairs). The LTYPE (from the linetype table),
// LAYER, STYLE, DIMSTYLE and APPID tables, the modelled header variables and the entities
// are generated.
func (c Codec) Write(out io.Writer, d *doc.Drawing) ([]string, error) {
	return c.WriteVersion(out, d, "")
}

func writeR12(out io.Writer, d *doc.Drawing) ([]string, error) {
	w := &writer{w: bufio.NewWriter(out)}
	var warns []string
	sub := d.Header.HandSeed
	seed := sub
	for _, h := range d.Handles() {
		switch e := d.Entity(h).(type) {
		case *doc.LWPolyline:
			seed += doc.Handle(len(e.Points) + 1)
		case *doc.MText:
			seed += doc.Handle(len(e.Layout().Lines))
		}
	}
	nextSub := func() string { h := sub; sub++; return h.String() }
	mtexts := 0

	w.pair(0, "SECTION")
	w.pair(2, "HEADER")
	w.pair(9, "$ACADVER")
	w.pair(1, "AC1009")
	w.pair(9, "$CLAYER")
	w.pair(8, d.CurrentLayer)
	w.planLinetypes(d)
	w.headerProps(d)
	w.pair(9, "$HANDLING")
	w.int(70, 1)
	w.pair(9, "$HANDSEED")
	w.pair(5, seed.String())
	w.pair(0, "ENDSEC")

	w.pair(0, "SECTION")
	w.pair(2, "TABLES")
	w.rawTable(d, "VPORT")
	lts := w.ltypeRecords()
	w.pair(0, "TABLE")
	w.pair(2, "LTYPE")
	w.int(70, len(lts))
	for _, r := range lts {
		w.pairs(r)
	}
	w.pair(0, "ENDTAB")
	w.pair(0, "TABLE")
	w.pair(2, "LAYER")
	w.int(70, len(d.Layers()))
	for _, l := range d.Layers() {
		w.pair(0, "LAYER")
		w.pair(2, l.Name)
		flags := 0
		if l.Frozen {
			flags |= 1
		}
		if l.Locked {
			flags |= 4
		}
		w.int(70, flags)
		c := l.Color
		if l.Off {
			c = -c
		}
		w.int(62, c)
		w.pair(6, w.layerLinetype(l))
		if l.NoPlot || l.Lineweight != 0 || l.Description != "" {
			// R12 has no plot flag, lineweight or description: they ride along as extended data of
			// the application CADLITE_LAYER, which the reader restores.
			w.pair(1001, layerApp)
			w.int(1070, boolInt(l.NoPlot))
			w.int(1070, l.Lineweight)
			w.pair(1000, l.Description)
		}
	}
	w.pair(0, "ENDTAB")
	w.styleTable(d)
	for _, t := range []string{"VIEW", "UCS"} {
		w.rawTable(d, t)
	}
	w.appidTable(d)
	dims := w.dimStyleTable(d)
	w.pair(0, "TABLE")
	w.pair(2, "DIMSTYLE")
	w.int(70, len(dims))
	for _, r := range dims {
		w.pairs(r)
	}
	w.pair(0, "ENDTAB")
	w.pair(0, "ENDSEC")

	if len(d.Preserved.Blocks) > 0 {
		w.pair(0, "SECTION")
		w.pair(2, "BLOCKS")
		for _, b := range d.Preserved.Blocks {
			warns = w.block(warns, b)
		}
		w.pair(0, "ENDSEC")
	}

	w.pair(0, "SECTION")
	w.pair(2, "ENTITIES")
	for _, h := range d.Handles() {
		e := d.Entity(h)
		switch e := e.(type) {
		case *doc.Line:
			w.head("LINE", e)
			w.point(10, e.Start)
			w.point(11, e.End)
		case *doc.Circle:
			w.head("CIRCLE", e)
			w.point(10, e.Center)
			w.num(40, e.Radius)
		case *doc.Arc:
			w.head("ARC", e)
			w.point(10, e.Center)
			w.num(40, e.Radius)
			w.num(50, e.StartAngle*180/math.Pi)
			w.num(51, e.EndAngle*180/math.Pi)
		case *doc.LWPolyline:
			w.head("POLYLINE", e)
			w.int(66, 1)
			w.point(10, geom.V(0, 0))
			flags := 0
			if e.Closed {
				flags = 1
			}
			w.int(70, flags)
			for i, p := range e.Points {
				w.pair(0, "VERTEX")
				w.pair(5, nextSub())
				w.pair(8, e.Layer)
				w.point(10, p)
				if i < len(e.Bulges) && e.Bulges[i] != 0 {
					w.num(42, e.Bulges[i])
				}
			}
			w.pair(0, "SEQEND")
			w.pair(5, nextSub())
			w.pair(8, e.Layer)
		case *doc.Text:
			w.text(e)
		case *doc.MText:
			w.mtextAsText(e, nextSub)
			mtexts++
		case *doc.Unknown:
			if !r12Entities[e.Type] {
				warns = append(warns, i18n.T("dxf.warn.not_written_r12", e.Type, h))
				continue
			}
			w.pairs(e.Source())
		default:
			warns = append(warns, i18n.T("dxf.warn.not_written_unsupported", e.Class(), h))
		}
	}
	w.pair(0, "ENDSEC")
	w.pair(0, "EOF")
	if mtexts > 0 {
		warns = append(warns, i18n.T("dxf.warn.mtext_as_text", mtexts))
	}
	if w.err == nil {
		w.err = w.w.Flush()
	}
	return warns, w.err
}

// text writes a TEXT entity as AutoCAD does: group 10 is the computed baseline start (for Aligned
// and Fit the first baseline point, Insert), 11 the
// alignment point when the justification is not left-baseline, and the height, rotation and
// width factor are the ones in effect (Aligned and Fit derive them from the two points).
func (w *writer) text(e *doc.Text) {
	lay := e.Layout()
	w.head("TEXT", e)
	if e.HAlign == doc.TextAligned || e.HAlign == doc.TextFit {
		// The baseline's first point, which a backward text's layout origin is not.
		w.point(10, e.Insert)
	} else {
		w.point(10, lay.Origin)
	}
	w.num(40, lay.Height)
	w.pair(1, encodeText(e.Value))
	if lay.Rotation != 0 {
		w.num(50, lay.Rotation*180/math.Pi)
	}
	if lay.WidthFactor != 1 {
		w.num(41, lay.WidthFactor)
	}
	if e.Oblique != 0 {
		w.num(51, e.Oblique*180/math.Pi)
	}
	w.pair(7, textStyle(e))
	if e.Generation != 0 {
		w.int(71, e.Generation)
	}
	if e.HAlign != doc.TextLeft {
		w.int(72, int(e.HAlign))
	}
	if e.HAlign != doc.TextLeft || e.VAlign != doc.TextBaseline {
		w.point(11, e.Align)
	}
	if e.VAlign != doc.TextBaseline {
		w.int(73, int(e.VAlign))
	}
}

// mtextAsText writes an MTEXT as R12 can hold it: one left-justified TEXT per line of its
// layout, the first with the MTEXT's handle and the others with handles above the seed. The
// formatting codes are already gone from the lines; the box, the attachment and the wrapping
// width are lost.
func (w *writer) mtextAsText(e *doc.MText, nextSub func() string) {
	lay := e.Layout()
	style := e.Style
	if style == "" {
		style = doc.DefaultTextStyle
	}
	c := e.Common()
	first := true
	for i, l := range lay.Lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		h := c.H.String()
		if !first {
			h = nextSub()
		}
		first = false
		w.headAs("TEXT", h, c)
		w.point(10, lay.Starts[i])
		w.num(40, e.Height)
		w.pair(1, encodeText(l))
		if e.Rotation != 0 {
			w.num(50, e.Rotation*180/math.Pi)
		}
		w.pair(7, style)
	}
}

// textStyle is the style name a text is written with.
func textStyle(e *doc.Text) string {
	if e.Style == "" {
		return doc.DefaultTextStyle
	}
	return e.Style
}

// encodeText makes a text value safe for a DXF line: line breaks become spaces and characters
// outside ASCII are written as \U+XXXX, which every AutoCAD reads whatever the code page.
func encodeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < 0x80:
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, "\\U+%04X", r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// styleTable writes the STYLE table: the preserved records, plus a record for the default style
// and for every style a text names that the table lacks (each drawing with the built-in txt font,
// which is all Cad-lite draws text with).
func (w *writer) styleTable(d *doc.Drawing) {
	rs := d.Preserved.Tables["STYLE"]
	have := map[string]bool{}
	for _, r := range rs {
		have[strings.ToUpper(r.Name())] = true
	}
	var add []string
	need := func(name string) {
		if u := strings.ToUpper(name); !have[u] {
			have[u] = true
			add = append(add, name)
		}
	}
	need(doc.DefaultTextStyle)
	for _, h := range d.Handles() {
		switch t := d.Entity(h).(type) {
		case *doc.Text:
			need(textStyle(t))
		case *doc.MText:
			if t.Style != "" {
				need(t.Style)
			}
		}
	}
	w.pair(0, "TABLE")
	w.pair(2, "STYLE")
	w.int(70, len(rs)+len(add))
	w.records(rs)
	for _, name := range add {
		w.pair(0, "STYLE")
		w.pair(2, name)
		w.int(70, 0)
		w.num(40, 0)
		w.num(41, 1)
		w.num(50, 0)
		w.int(71, 0)
		w.num(42, 2.5)
		w.pair(3, "txt")
		w.pair(4, "")
	}
	w.pair(0, "ENDTAB")
}

func (w *writer) head(typ string, e doc.Entity) {
	c := e.Common()
	w.headAs(typ, c.H.String(), c)
}

// headAs writes an entity's type, the handle h, the layer, a linetype other than ByLayer (6), a
// colour other than ByLayer (62, 0 for ByBlock) and, in AC1015, a lineweight other than ByLayer
// (370) and a linetype scale other than 1 (48). R12 has no 370 or 48.
func (w *writer) headAs(typ, h string, c *doc.EntityCommon) {
	if w.typeOverride != "" {
		typ = w.typeOverride
		w.typeOverride = ""
	}
	w.pair(0, typ)
	w.pair(5, h)
	if w.modern {
		if hs := w.reactors[c.H]; len(hs) > 0 {
			w.pair(102, "{ACAD_REACTORS")
			for _, g := range hs {
				w.pair(330, g.String())
			}
			w.pair(102, "}")
		}
		owner := c.Owner
		if owner == 0 {
			owner = doc.ModelSpace
		}
		if owner == doc.ModelSpace && w.modelOwner != "" {
			w.pair(330, w.modelOwner)
		} else {
			w.pair(330, owner.String())
		}
		w.pair(100, "AcDbEntity")
	}
	w.pair(8, c.Layer)
	if lt := w.entityLinetype(c.Linetype); lt != "" {
		w.pair(6, lt)
	}
	if c.Color != doc.ByLayer && c.Color >= 0 && c.Color < doc.ByLayer {
		w.int(62, c.Color)
	}
	if w.modern {
		if c.Lineweight != doc.LineweightByLayer {
			w.int(370, c.Lineweight.DXF())
		}
		if sc := c.LtScale(); sc != 1 {
			w.num(48, sc)
		}
		sub := map[string]string{"LINE": "AcDbLine", "CIRCLE": "AcDbCircle", "ARC": "AcDbCircle", "TEXT": "AcDbText", "MTEXT": "AcDbMText", "ATTDEF": "AcDbText", "ATTRIB": "AcDbText", "LWPOLYLINE": "AcDbPolyline", "INSERT": "AcDbBlockReference", "DIMENSION": "AcDbDimension"}[typ]
		if sub != "" {
			w.pair(100, sub)
		}
	}
}

// r12Entities are the entity classes R12 (AC1009) defines. An unknown entity
// of another class cannot be written to an R12 file.
var r12Entities = map[string]bool{
	"LINE": true, "POINT": true, "CIRCLE": true, "ARC": true, "TRACE": true, "SOLID": true,
	"TEXT": true, "SHAPE": true, "INSERT": true, "ATTDEF": true, "ATTRIB": true,
	"POLYLINE": true, "VERTEX": true, "SEQEND": true, "3DLINE": true, "3DFACE": true,
	"VIEWPORT": true, "DIMENSION": true,
}

// pairs writes ps, leaving out the groups that only exist from R13 on:
// subclass markers (100), reactor braces (102), owner and extension
// dictionary handles (330, 360). A strict R12 reader cannot place a record
// that has them. Every other group is written as it was read.
var r12Blocks = map[string]bool{"BLOCK": true, "ENDBLK": true}

func (w *writer) pairs(ps []doc.Pair) {
	for _, p := range ps {
		switch p.Code {
		case 100, 102, 330, 360:
		default:
			w.pair(p.Code, p.Value)
		}
	}
}

func (w *writer) records(rs []doc.RawRecord) {
	for _, r := range rs {
		w.pairs(r.Pairs)
	}
}

// block writes a preserved block, leaving out the entities whose class R12
// does not have; each one adds a warning.
func (w *writer) block(warns []string, b doc.RawRecord) []string {
	ps := b.Pairs
	for len(ps) > 0 {
		n := 1
		for n < len(ps) && ps[n].Code != 0 {
			n++
		}
		typ := strings.TrimSpace(ps[0].Value)
		if r12Blocks[typ] || r12Entities[typ] {
			w.pairs(ps[:n])
		} else {
			warns = append(warns, i18n.T("dxf.warn.block_not_written", typ, b.Name()))
		}
		ps = ps[n:]
	}
	return warns
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// planLinetypes decides the LTYPE records to write: the drawing's table, then, for a name a
// layer, an entity (in model space or a block) or $CELTYPE uses that the table lacks, the built-in
// library's definition, or a solid record of that name when the library lacks it too, so every
// reference resolves.
func (w *writer) planLinetypes(d *doc.Drawing) {
	w.ltNames, w.ltRecords = map[string]string{}, nil
	add := func(l doc.Linetype) {
		u := strings.ToUpper(l.Name)
		if _, ok := w.ltNames[u]; !ok {
			w.ltNames[u] = l.Name
			w.ltRecords = append(w.ltRecords, l)
		}
	}
	for _, l := range d.Linetypes() {
		add(l)
	}
	need := func(name string) {
		if doc.IsByLayerLinetype(name) || doc.IsByBlockLinetype(name) {
			return
		}
		if _, ok := w.ltNames[strings.ToUpper(name)]; ok {
			return
		}
		l, ok := doc.LibraryLinetype(name)
		if !ok {
			l = doc.Linetype{Description: "Solid line"}
		}
		l.Name = name
		add(l)
	}
	for _, l := range d.Layers() {
		need(l.Linetype)
	}
	for _, h := range d.Handles() {
		need(d.Entity(h).Common().Linetype)
	}
	for _, b := range d.Blocks() {
		for _, e := range b.Entities {
			need(e.Common().Linetype)
		}
	}
	need(d.CurrentLinetype)
}

// ltypeRecords returns the LTYPE records to write: in AC1015 BYBLOCK and BYLAYER first, then the
// planned linetypes. A simple linetype is generated (2, 70, 3, 72 65, 73 the element count, 40 the
// length, then 49 per element, each followed by 74 0 in AC1015); a complex one is written as read
// in AC1015 and as its dashes alone in R12, which has no complex linetypes.
func (w *writer) ltypeRecords() [][]doc.Pair {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	gen := func(l doc.Linetype) []doc.Pair {
		ps := []doc.Pair{{Code: 0, Value: "LTYPE"}, {Code: 2, Value: l.Name}, {Code: 70, Value: "0"},
			{Code: 3, Value: l.Description}, {Code: 72, Value: "65"}, {Code: 73, Value: strconv.Itoa(len(l.Pattern))},
			{Code: 40, Value: f(l.Length())}}
		for _, v := range l.Pattern {
			ps = append(ps, doc.Pair{Code: 49, Value: f(v)})
			if w.modern {
				ps = append(ps, doc.Pair{Code: 74, Value: "0"})
			}
		}
		return ps
	}
	var out [][]doc.Pair
	if w.modern {
		out = append(out, gen(doc.Linetype{Name: "ByBlock"}), gen(doc.Linetype{Name: "ByLayer"}))
	}
	for _, l := range w.ltRecords {
		if w.modern && l.Raw != nil {
			out = append(out, l.Raw)
		} else {
			out = append(out, gen(l))
		}
	}
	return out
}

// layerLinetype is the linetype a layer record names (6): CONTINUOUS for none.
func (w *writer) layerLinetype(l *doc.Layer) string {
	if l.Linetype == "" {
		return doc.LinetypeContinuous
	}
	if n, ok := w.ltNames[strings.ToUpper(l.Linetype)]; ok {
		return n
	}
	return l.Linetype
}

// entityLinetype is an entity's group 6, "" for ByLayer (not written).
func (w *writer) entityLinetype(name string) string {
	switch {
	case doc.IsByLayerLinetype(name):
		return ""
	case doc.IsByBlockLinetype(name):
		return "BYBLOCK"
	}
	if n, ok := w.ltNames[strings.ToUpper(name)]; ok {
		return n
	}
	return name
}

// headerProps writes the current properties: $CECOLOR, $CELTYPE, $LTSCALE and $DIMSTYLE, and in
// AC1015 $CELTSCALE and $CELWEIGHT, which R12 lacks.
func (w *writer) headerProps(d *doc.Drawing) {
	c := d.CurrentColor
	if c < 0 || c > doc.ByLayer {
		c = doc.ByLayer
	}
	w.pair(9, "$CECOLOR")
	w.int(62, c)
	w.pair(9, "$CELTYPE")
	lt := w.entityLinetype(d.CurrentLinetype)
	if lt == "" {
		lt = "BYLAYER"
	}
	w.pair(6, lt)
	w.pair(9, "$LTSCALE")
	w.num(40, d.LTScaleValue())
	w.pair(9, "$DIMSTYLE")
	w.pair(2, d.CurrentDimStyle)
	if w.modern {
		w.pair(9, "$CELTSCALE")
		w.num(40, doc.LinetypeScaleOf(d.CurrentLinetypeScale))
		w.pair(9, "$CELWEIGHT")
		w.int(370, d.CurrentLineweight.DXF())
	}
}

// appidTable writes the APPID table: the preserved records plus CADLITE_LAYER when a layer
// carries extended data and CADLITE_DIMSTYLE for the DIMSTYLE records' extended data.
func (w *writer) appidTable(d *doc.Drawing) {
	rs := d.Preserved.Tables["APPID"]
	var apps []string
	for _, l := range d.Layers() {
		if l.NoPlot || l.Lineweight != 0 || l.Description != "" {
			apps = append(apps, layerApp)
			break
		}
	}
	if !hasRecord(d, "APPID", dimStyleApp) {
		apps = append(apps, dimStyleApp)
	}
	w.pair(0, "TABLE")
	w.pair(2, "APPID")
	w.int(70, len(rs)+len(apps))
	w.records(rs)
	for _, a := range apps {
		w.pair(0, "APPID")
		w.pair(2, a)
		w.int(70, 0)
	}
	w.pair(0, "ENDTAB")
}

// rawTable writes the preserved records of table name as a table, or
// nothing when there are none.
func (w *writer) rawTable(d *doc.Drawing, name string) {
	rs := d.Preserved.Tables[name]
	if len(rs) == 0 {
		return
	}
	w.pair(0, "TABLE")
	w.pair(2, name)
	w.int(70, len(rs))
	w.records(rs)
	w.pair(0, "ENDTAB")
}
