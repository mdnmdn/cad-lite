// Package dxf reads and writes ASCII DXF. The reader accepts R12 through
// current versions and keeps handles; the writer emits R12 (AC1009), the
// most widely readable version. Entities the document does not model are
// kept as doc.Unknown with their raw groups, and the tables and blocks it
// does not model are kept in doc.Preserved, so a file can be written back
// without losing them; the writer says what R12 cannot represent.
package dxf

import (
	"bufio"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
)

// Codec is the DXF reader and writer.
type Codec struct{}

// Extensions implements fileio.Reader and fileio.Writer.
func (Codec) Extensions() []string { return []string{".dxf"} }

// Versions implements fileio.Writer.
func (Codec) Versions() []string { return []string{"AC1009", "AC1015"} }

type pair struct {
	code int
	val  string
}

func readPairs(r io.Reader) ([]pair, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []pair
	line := 0
	for sc.Scan() {
		line++
		cs := strings.TrimSpace(sc.Text())
		if !sc.Scan() {
			return nil, errors.New(i18n.T("dxf.err.no_value", line, cs))
		}
		line++
		code, err := strconv.Atoi(cs)
		if err != nil {
			return nil, errors.New(i18n.T("dxf.err.bad_group_code", line-1, cs))
		}
		out = append(out, pair{code, strings.TrimRight(sc.Text(), "\r")})
	}
	return out, sc.Err()
}

// object is one "0 TYPE" record and its group pairs.
type object struct {
	typ   string
	pairs []pair
}

func (o object) str(code int) string {
	for _, p := range o.pairs {
		if p.code == code {
			return strings.TrimSpace(p.val)
		}
	}
	return ""
}

func (o object) float(code int) float64 {
	v, _ := strconv.ParseFloat(o.str(code), 64)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// zeroNonFinite rewrites the non-finite values (nan, inf, 1e999) of the numeric group codes
// (10-59, 110-149, 210-239) to 0 and returns how many it changed.
func zeroNonFinite(ps []pair) int {
	n := 0
	for i, p := range ps {
		c := p.code
		if !(c >= 10 && c <= 59 || c >= 110 && c <= 149 || c >= 210 && c <= 239) {
			continue
		}
		if v, _ := strconv.ParseFloat(strings.TrimSpace(p.val), 64); math.IsNaN(v) || math.IsInf(v, 0) {
			ps[i].val = "0"
			n++
		}
	}
	return n
}

func (o object) int(code int) int {
	v, _ := strconv.Atoi(o.str(code))
	return v
}

func (o object) has(code int) bool {
	for _, p := range o.pairs {
		if p.code == code {
			return true
		}
	}
	return false
}

// Read implements fileio.Reader.
func (Codec) Read(r io.Reader) (*doc.Drawing, []string, error) {
	ps, err := readPairs(r)
	if err != nil {
		return nil, nil, err
	}
	d := doc.New()
	d.Header.Extra = map[string][]string{}
	st := &state{kept: map[string]int{}}
	if n := zeroNonFinite(ps); n > 0 {
		st.warns = append(st.warns, i18n.T("dxf.warn.non_finite", n))
	}
	var section string
	var open, sawEOF bool
	var objs []object
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if p.code != 0 {
			if section == "" && p.code == 2 && len(objs) > 0 && objs[len(objs)-1].typ == "SECTION" {
				section = strings.TrimSpace(p.val)
				objs = objs[:0]
				if section == "HEADER" {
					// Header variables precede any 0 record; collect them in one.
					objs = append(objs, object{typ: "HEADER"})
				}
				continue
			}
			if len(objs) > 0 {
				objs[len(objs)-1].pairs = append(objs[len(objs)-1].pairs, p)
			}
			continue
		}
		typ := strings.TrimSpace(p.val)
		switch typ {
		case "SECTION":
			if open {
				return nil, nil, errors.New(i18n.T("dxf.err.no_endsec", section))
			}
			open = true
			section = ""
			objs = append(objs[:0], object{typ: typ})
		case "ENDSEC":
			if err := applySection(d, section, objs, st); err != nil {
				return nil, nil, err
			}
			section, objs, open = "", objs[:0], false
		case "EOF":
			sawEOF = true
			i = len(ps)
		default:
			objs = append(objs, object{typ: typ})
		}
	}
	if open {
		return nil, nil, errors.New(i18n.T("dxf.err.no_endsec", section))
	}
	warns := st.warns
	if !sawEOF {
		warns = append(warns, i18n.T("dxf.warn.no_eof"))
	}
	types := make([]string, 0, len(st.kept))
	for t := range st.kept {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		warns = append(warns, i18n.T("dxf.warn.kept_unknown", st.kept[t], t))
	}
	if st.mtextIgnored > 0 {
		warns = append(warns, i18n.T("dxf.warn.mtext_ignored", st.mtextIgnored))
	}
	if d.Layer(d.CurrentLayer) == nil {
		d.CurrentLayer = "0"
	}
	for _, h := range d.Handles() {
		if in, ok := d.Entity(h).(*doc.Insert); ok {
			if err := d.InsertError(in); err != nil {
				d.Load(&doc.Unknown{EntityCommon: *in.Common(), Type: "INSERT", Raw: append([]doc.Pair(nil), in.Source[1:]...)})
				warns = append(warns, i18n.T("dxf.warn.invalid_block_reference", h, err.Error()))
			}
		}
	}
	return d, warns, nil
}

// state is what reading one file accumulates besides the drawing.
type state struct {
	// kept counts the unknown entities kept, by class.
	kept  map[string]int
	warns []string
	// mtextIgnored counts the MTEXT whose extrusion or top-to-bottom direction is not modelled.
	mtextIgnored int
}

// preservedTables are the tables whose records the document does not model
// and the writer emits again.
var preservedTables = map[string]bool{"VPORT": true, "LTYPE": true, "STYLE": true, "VIEW": true, "UCS": true, "APPID": true, "DIMSTYLE": true}

func applySection(d *doc.Drawing, section string, objs []object, st *state) error {
	switch section {
	case "HEADER":
		readHeader(d, objs)
	case "TABLES":
		readTables(d, objs)
	case "BLOCKS":
		readModeledBlocks(d, objs, st)
	case "OBJECTS":
		readObjects(d, objs)
	case "ENTITIES":
		readEntities(d, objs, st)
	}
	return nil
}

func rawPairs(o object) []doc.Pair {
	out := make([]doc.Pair, 0, len(o.pairs)+1)
	out = append(out, doc.Pair{Code: 0, Value: o.typ})
	for _, p := range o.pairs {
		out = append(out, doc.Pair{Code: p.code, Value: p.val})
	}
	return out
}

// bumpSeed raises the handle seed above every handle (codes 5 and 105) in
// ps, so no handle the writer or a command allocates collides with one kept
// verbatim.
func bumpSeed(d *doc.Drawing, ps []doc.Pair) {
	for _, p := range ps {
		if p.Code == 5 || p.Code == 105 {
			if h, err := doc.ParseHandle(strings.TrimSpace(p.Value)); err == nil && h >= d.Header.HandSeed {
				d.Header.HandSeed = h + 1
			}
		}
	}
}

func readTables(d *doc.Drawing, objs []object) {
	var layerOrder []string
	seenLayers := map[string]bool{}
	table := ""
	for _, o := range objs {
		switch o.typ {
		case "TABLE":
			table = o.str(2)
		case "ENDTAB":
			table = ""
		case "BLOCK_RECORD":
			h, _ := doc.ParseHandle(o.str(5))
			switch strings.ToUpper(o.str(2)) {
			case "*MODEL_SPACE":
				d.Preserved.ModelOwner = h
			case "*PAPER_SPACE":
				d.Preserved.PaperOwner = h
			}
		case "LAYER":
			readLayer(d, o)
			name := o.str(2)
			if d.Layer(name) != nil && !seenLayers[name] {
				layerOrder = append(layerOrder, name)
				seenLayers[name] = true
			}
		default:
			if !preservedTables[table] || o.typ != table {
				continue
			}
			if o.typ == "LTYPE" && strings.EqualFold(o.str(2), "CONTINUOUS") {
				continue
			}
			if o.typ == "APPID" && isLayerApp(o.str(2)) {
				continue
			}
			r := doc.RawRecord{Pairs: rawPairs(o)}
			bumpSeed(d, r.Pairs)
			if d.Preserved.Tables == nil {
				d.Preserved.Tables = map[string][]doc.RawRecord{}
			}
			d.Preserved.Tables[table] = append(d.Preserved.Tables[table], r)
		}
	}
	if len(layerOrder) > 0 {
		for _, l := range d.Layers() {
			if !seenLayers[l.Name] {
				layerOrder = append(layerOrder, l.Name)
			}
		}
		_ = d.LoadLayerOrder(layerOrder)
	}
}

// readBlocks keeps each block definition, BLOCK to ENDBLK, except the model
// and paper space blocks, whose entities live in ENTITIES.
func readBlocks(d *doc.Drawing, objs []object) {
	var cur []doc.Pair
	name := ""
	for _, o := range objs {
		if o.typ == "BLOCK" {
			cur, name = nil, o.str(2)
		}
		if cur == nil && o.typ != "BLOCK" {
			continue
		}
		cur = append(cur, rawPairs(o)...)
		if o.typ == "ENDBLK" {
			u := strings.ToUpper(name)
			if !strings.HasPrefix(u, "*MODEL_SPACE") && !strings.HasPrefix(u, "*PAPER_SPACE") {
				bumpSeed(d, cur)
				d.Preserved.Blocks = append(d.Preserved.Blocks, doc.RawRecord{Pairs: cur})
			}
			cur = nil
		}
	}
}

func readHeader(d *doc.Drawing, objs []object) {
	if len(objs) == 0 {
		return
	}
	// The header is one record of "9 $NAME" followed by value groups.
	var name string
	for _, p := range objs[0].pairs {
		if p.code == 9 {
			name = strings.TrimSpace(p.val)
			continue
		}
		switch name {
		case "$HANDSEED":
			if h, err := doc.ParseHandle(strings.TrimSpace(p.val)); err == nil && h > d.Header.HandSeed {
				d.Header.HandSeed = h
			}
		case "$INSUNITS":
			d.Header.InsUnits, _ = strconv.Atoi(strings.TrimSpace(p.val))
		case "$CLAYER":
			d.CurrentLayer = strings.TrimSpace(p.val)
		default:
			d.Header.Extra[name] = append(d.Header.Extra[name], strconv.Itoa(p.code)+" "+p.val)
		}
	}
}

func readLayer(d *doc.Drawing, o object) {
	name := o.str(2)
	if name == "" {
		return
	}
	color := o.int(62)
	l := d.AddLayer(name, 7)
	l.Off = color < 0
	if color < 0 {
		color = -color
	}
	if color >= 1 && color <= 255 {
		l.Color = color
	}
	flags := o.int(70)
	l.Locked = flags&4 != 0
	l.Frozen = flags&1 != 0
	if lt := o.str(6); !strings.EqualFold(lt, "CONTINUOUS") {
		l.Linetype = lt
	}
	// R2000 and later write the plot flag (290) and lineweight (370) natively; the R12 writer here
	// keeps them in extended data of CADLITE_LAYER (see layerApp).
	if o.has(290) {
		l.NoPlot = o.int(290) == 0
	}
	if o.has(370) && o.int(370) > 0 {
		l.Lineweight = o.int(370)
	}
	readLayerXData(l, o)
}

// readLayerXData restores the fields the writer put in CADLITE_LAYER extended data: the plot flag,
// the lineweight and the description, in that order.
func readLayerXData(l *doc.Layer, o object) {
	in := false
	ints := 0
	for _, p := range o.pairs {
		switch {
		case p.code == 1001:
			in = isLayerApp(p.val)
		case !in:
		case p.code == 1070:
			v, _ := strconv.Atoi(strings.TrimSpace(p.val))
			if ints == 0 {
				l.NoPlot = v != 0
			} else if ints == 1 {
				l.Lineweight = v
			}
			ints++
		case p.code == 1000:
			l.Description = p.val
		}
	}
}

// layerApp is the application name of the layer extended data (see the writer).
const layerApp = "CADLITE_LAYER"

// legacyLayerApp is the name the writer used while the project was called go-draw; the reader
// still understands it, and the writer replaces it with layerApp.
const legacyLayerApp = "GODRAW_LAYER"

// isLayerApp reports whether name is one of the layer extended-data application names.
func isLayerApp(name string) bool {
	name = strings.TrimSpace(name)
	return strings.EqualFold(name, layerApp) || strings.EqualFold(name, legacyLayerApp)
}

func common(o object) doc.EntityCommon {
	c := doc.EntityCommon{Layer: o.str(8), Color: doc.ByLayer}
	if c.Layer == "" {
		c.Layer = "0"
	}
	if h, err := doc.ParseHandle(o.str(5)); err == nil {
		c.H = h
	}
	if o.has(62) {
		if v := o.int(62); v >= 0 && v <= 256 {
			c.Color = v
		}
	}
	return c
}

func vec(o object, xc int) geom.Vec2 { return geom.V(o.float(xc), o.float(xc+10)) }

func deg(v float64) float64 { return geom.NormAngle(v * math.Pi / 180) }

func readEntities(d *doc.Drawing, objs []object, st *state) {
	for _, o := range objs {
		bumpSeed(d, rawPairs(o))
	}
	for i := 0; i < len(objs); i++ {
		o := objs[i]
		c := common(o)
		var e doc.Entity
		start := i
		switch o.typ {
		case "LINE":
			e = &doc.Line{EntityCommon: c, Start: vec(o, 10), End: vec(o, 11)}
		case "CIRCLE":
			e = &doc.Circle{EntityCommon: c, Center: vec(o, 10), Radius: o.float(40)}
		case "ARC":
			e = &doc.Arc{EntityCommon: c, Center: vec(o, 10), Radius: o.float(40), StartAngle: deg(o.float(50)), EndAngle: deg(o.float(51))}
		case "LWPOLYLINE":
			e = readLWPolyline(o, c)
		case "ATTDEF":
			e = readAttdef(o, c)
		case "INSERT":
			if o.int(66) == 1 {
				for i+1 < len(objs) {
					i++
					if objs[i].typ == "SEQEND" {
						break
					}
				}
			}
			supported := supportedInsert(o)
			for _, sub := range objs[start+1 : i+1] {
				if sub.typ == "ATTRIB" && (!sub.has(10) || !sub.has(40)) {
					supported = false
				}
			}
			if supported {
				e = readInsert(o, objs[start+1:i+1], c)
				in := e.(*doc.Insert)
				in.Source = rawPairs(o)
				for _, sub := range objs[start+1 : i+1] {
					in.Source = append(in.Source, rawPairs(sub)...)
				}
			} else {
				e = unknown(o, objs[start+1:i+1], c)
			}
		case "TEXT":
			e = readText(o, c)
		case "MTEXT":
			e = readMText(o, c)
			if (o.has(230) && o.float(230) < 0) || o.has(210) && o.float(210) != 0 || o.has(220) && o.float(220) != 0 || o.int(72) == 3 {
				st.mtextIgnored++
			}
		case "POLYLINE":
			var verts []object
			for i+1 < len(objs) && objs[i+1].typ == "VERTEX" {
				i++
				verts = append(verts, objs[i])
			}
			if i+1 < len(objs) && objs[i+1].typ == "SEQEND" {
				i++
			}
			if o.int(70)&(8|16|64) != 0 {
				e = unknown(o, objs[start+1:i+1], c)
				break
			}
			p := &doc.LWPolyline{EntityCommon: c, Closed: o.int(70)&1 != 0}
			for _, v := range verts {
				p.Points = append(p.Points, vec(v, 10))
				p.Bulges = append(p.Bulges, v.float(42))
			}
			e = p
		default:
			// An entity that owns sub-records (code 66, as an INSERT with
			// attributes) runs to its SEQEND.
			if o.int(66) == 1 {
				for j := i + 1; j < len(objs); j++ {
					if objs[j].typ == "SEQEND" {
						i = j
						break
					}
				}
			}
			e = unknown(o, objs[start+1:i+1], c)
		}
		if u, ok := e.(*doc.Unknown); ok {
			st.kept[u.Type]++
		}
		if d.Entity(c.H) != nil {
			old := c.H
			e.Common().H = 0
			d.Load(e)
			if u, ok := e.(*doc.Unknown); ok {
				setHandle(u, e.Handle())
			}
			st.warns = append(st.warns, i18n.T("dxf.warn.handle_renumbered", e.Class(), old, e.Handle()))
			continue
		}
		d.Load(e)
		if u, ok := e.(*doc.Unknown); ok {
			setHandle(u, e.Handle())
		}
	}
}

// unknown keeps o and its sub-records as an Unknown entity.
func unknown(o object, subs []object, c doc.EntityCommon) *doc.Unknown {
	u := &doc.Unknown{EntityCommon: c, Type: o.typ}
	u.Raw = rawPairs(o)[1:]
	for _, s := range subs {
		u.Raw = append(u.Raw, rawPairs(s)...)
	}
	if h, err := doc.ParseHandle(o.str(330)); err == nil {
		u.Owner = h
	}
	return u
}

// setHandle makes Raw carry handle h in the entity's own record: it rewrites
// the code 5 group, or inserts one when the file had none (R12 without
// handles). It is the only change the reader makes to the raw groups.
func setHandle(u *doc.Unknown, h doc.Handle) {
	for i, p := range u.Raw {
		if p.Code == 0 {
			break
		}
		if p.Code == 5 {
			if got, err := doc.ParseHandle(strings.TrimSpace(p.Value)); err != nil || got != h {
				u.Raw[i].Value = h.String()
			}
			return
		}
	}
	u.Raw = append([]doc.Pair{{Code: 5, Value: h.String()}}, u.Raw...)
}

func readLWPolyline(o object, c doc.EntityCommon) *doc.LWPolyline {
	p := &doc.LWPolyline{EntityCommon: c, Closed: o.int(70)&1 != 0}
	var x float64
	haveX := false
	for _, g := range o.pairs {
		v, _ := strconv.ParseFloat(strings.TrimSpace(g.val), 64)
		switch g.code {
		case 10:
			x, haveX = v, true
		case 20:
			if haveX {
				p.Points = append(p.Points, geom.V(x, v))
				p.Bulges = append(p.Bulges, 0)
				haveX = false
			}
		case 42:
			if n := len(p.Bulges); n > 0 {
				p.Bulges[n-1] = v
			}
		}
	}
	return p
}

// raw returns the value of the first group code, untrimmed (text values keep their spaces).
func (o object) raw(code int) (string, bool) {
	for _, p := range o.pairs {
		if p.code == code {
			return p.val, true
		}
	}
	return "", false
}

// readText reads a TEXT entity. Group 11 is the alignment point when group 72 or 73 is set, and
// group 10 is then AutoCAD's computed baseline start, which the document recomputes. A text
// whose extrusion is -Z (a mirrored object coordinate system) is converted to world coordinates
// as upside-down text, which draws the same; any other extrusion is ignored.
func readText(o object, c doc.EntityCommon) *doc.Text {
	t := &doc.Text{EntityCommon: c, Insert: vec(o, 10), Height: o.float(40), Rotation: deg(o.float(50)),
		WidthFactor: 1, Oblique: o.float(51) * math.Pi / 180, Style: o.str(7),
		HAlign: doc.TextHAlign(o.int(72)), VAlign: doc.TextVAlign(o.int(73)), Generation: o.int(71) & (doc.TextBackward | doc.TextUpsideDown)}
	if o.has(41) && o.float(41) > 0 {
		t.WidthFactor = o.float(41)
	}
	if t.HAlign < doc.TextLeft || t.HAlign > doc.TextFit {
		t.HAlign = doc.TextLeft
	}
	if t.VAlign < doc.TextBaseline || t.VAlign > doc.TextTop {
		t.VAlign = doc.TextBaseline
	}
	t.Align = t.Insert
	if o.has(11) {
		t.Align = vec(o, 11)
	}
	if t.Style == "" {
		t.Style = doc.DefaultTextStyle
	}
	v, _ := o.raw(1)
	t.Value = decodeUnicode(v)
	if o.has(230) && o.float(230) < 0 {
		t.Insert.X, t.Align.X = -t.Insert.X, -t.Align.X
		t.Rotation = geom.NormAngle(math.Pi - t.Rotation)
		t.Generation ^= doc.TextUpsideDown
	}
	t.Sanitize()
	return t
}

// decodeUnicode replaces the \U+XXXX escapes DXF uses for characters outside the file's code page
// with the characters.
func decodeUnicode(s string) string {
	if !strings.Contains(s, "\\U+") && !strings.Contains(s, "\\u+") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+6 < len(s) && (s[i+1] == 'U' || s[i+1] == 'u') && s[i+2] == '+' {
			if n, err := strconv.ParseUint(s[i+3:i+7], 16, 32); err == nil {
				b.WriteRune(rune(n))
				i += 6
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// readMText reads an MTEXT entity: the value is the group 3 chunks followed by group 1, and a
// direction vector (group 11) overrides the rotation (group 50). The extrusion (210), the drawing
// direction (72), the line spacing style (73), the column layout and the background fill are not
// modelled and are dropped; the caller warns when the extrusion or a top-to-bottom direction
// would have changed what is drawn.
func readMText(o object, c doc.EntityCommon) *doc.MText {
	t := &doc.MText{EntityCommon: c, Insert: vec(o, 10), Height: o.float(40), Width: o.float(41), Attachment: o.int(71),
		Rotation: deg(o.float(50)), LineSpacing: 1, Style: o.str(7)}
	if o.has(11) {
		if d := vec(o, 11); d.Len() > 0 {
			t.Rotation = geom.NormAngle(d.Angle())
		}
	}
	if o.has(44) && o.float(44) > 0 {
		t.LineSpacing = o.float(44)
	}
	if t.Attachment < 1 || t.Attachment > 9 {
		t.Attachment = doc.MTextTopLeft
	}
	if t.Width < 0 {
		t.Width = 0
	}
	if t.Style == "" {
		t.Style = doc.DefaultTextStyle
	}
	var b strings.Builder
	for _, p := range o.pairs {
		if p.code == 3 {
			b.WriteString(p.val)
		}
	}
	v, _ := o.raw(1)
	b.WriteString(v)
	t.Value = decodeUnicode(b.String())
	t.Sanitize()
	return t
}
