package dxf

import (
	"bufio"
	"bytes"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"io"
	"math"
)

func hasAttdef(d *doc.Drawing) bool {
	for _, h := range d.Handles() {
		if _, ok := d.Entity(h).(*doc.AttributeDefinition); ok {
			return true
		}
	}
	return false
}
func writeSymbols(out io.Writer, d *doc.Drawing) ([]string, error) {
	modern := len(d.Groups()) > 0 || len(d.Preserved.Objects) > 0
	var payload bytes.Buffer
	w := &writer{w: bufio.NewWriter(&payload), modern: modern, d: d, reactors: map[doc.Handle][]doc.Handle{}}
	sub := d.Header.HandSeed
	for _, block := range d.Preserved.Blocks {
		if h := preservedBlockOwner(block); h >= sub {
			sub = h + 1
		}
	}
	next := func() string { h := sub; sub++; return h.String() }
	var warns []string
	preservedOwners := make([]string, len(d.Preserved.Blocks))
	for i, b := range d.Preserved.Blocks {
		if h := preservedBlockOwner(b); h != 0 {
			preservedOwners[i] = h.String()
		} else {
			preservedOwners[i] = next()
		}
	}
	// Allocate generated owners above every persistent drawing handle.
	root, groupDict, paper := next(), next(), next()
	model := next()
	w.modelOwner = model
	tables := map[string]string{}
	for _, t := range []string{"VPORT", "LTYPE", "LAYER", "STYLE", "VIEW", "UCS", "APPID", "DIMSTYLE", "BLOCK_RECORD"} {
		tables[t] = next()
	}
	section := func(s string) { w.pair(0, "SECTION"); w.pair(2, s) }
	end := func() { w.pair(0, "ENDSEC") }
	section("HEADER")
	w.pair(9, "$ACADVER")
	version := "AC1009"
	if modern {
		version = "AC1015"
	}
	w.pair(1, version)
	w.pair(9, "$CLAYER")
	w.pair(8, d.CurrentLayer)
	w.pair(9, "$INSUNITS")
	w.int(70, d.Header.InsUnits)
	w.pair(9, "$HANDSEED")
	w.pair(5, "CADLITE_HANDSEED")
	end()
	tableStart := func(t string, count int) {
		w.pair(0, "TABLE")
		w.pair(2, t)
		if modern {
			w.pair(5, tables[t])
			w.pair(330, "0")
			w.pair(100, "AcDbSymbolTable")
		}
		w.int(70, count)
	}
	record := func(t, name, subclass string) {
		w.pair(0, t)
		if modern {
			w.pair(5, next())
			w.pair(330, tables[t])
			w.pair(100, "AcDbSymbolTableRecord")
			w.pair(100, subclass)
		}
		w.pair(2, name)
		if t != "LAYER" {
			w.int(70, 0)
		}
	}
	section("TABLES")
	tableStart("LTYPE", 1+len(d.Preserved.Tables["LTYPE"]))
	record("LTYPE", "CONTINUOUS", "AcDbLinetypeTableRecord")
	w.pair(3, "Solid line")
	w.int(72, 65)
	w.int(73, 0)
	w.num(40, 0)
	for _, r := range d.Preserved.Tables["LTYPE"] {
		w.tableRecord(r, tables["LTYPE"], next)
	}
	w.pair(0, "ENDTAB")
	tableStart("LAYER", len(d.Layers()))
	for _, l := range d.Layers() {
		record("LAYER", l.Name, "AcDbLayerTableRecord")
		flags := 0
		if l.Frozen {
			flags |= 1
		}
		if l.Locked {
			flags |= 4
		}
		w.int(70, flags)
		color := l.Color
		if l.Off {
			color = -color
		}
		w.int(62, color)
		lt := l.Linetype
		if lt == "" {
			lt = "CONTINUOUS"
		}
		w.pair(6, lt)
		if modern {
			w.int(290, boolInt(!l.NoPlot))
			w.int(370, l.Lineweight)
		}
		if l.NoPlot || l.Lineweight != 0 || l.Description != "" {
			w.pair(1001, layerApp)
			w.int(1070, boolInt(l.NoPlot))
			w.int(1070, l.Lineweight)
			w.pair(1000, l.Description)
		}
	}
	w.pair(0, "ENDTAB")
	styles := map[string]bool{"Standard": true}
	for _, r := range d.Preserved.Tables["STYLE"] {
		styles[r.Name()] = true
	}
	addStyle := func(e doc.Entity) {
		switch t := e.(type) {
		case *doc.Text:
			styles[t.Style] = true
		case *doc.MText:
			styles[t.Style] = true
		case *doc.AttributeDefinition:
			styles[t.Style] = true
		}
	}
	for _, h := range d.Handles() {
		addStyle(d.Entity(h))
	}
	for _, b := range d.Blocks() {
		for _, e := range b.Entities {
			addStyle(e)
		}
	}
	delete(styles, "")
	tableStart("STYLE", len(styles))
	pres := map[string]bool{}
	for _, r := range d.Preserved.Tables["STYLE"] {
		w.tableRecord(r, tables["STYLE"], next)
		pres[r.Name()] = true
	}
	for name := range styles {
		if pres[name] {
			continue
		}
		record("STYLE", name, "AcDbTextStyleTableRecord")
		w.num(40, 0)
		w.num(41, 1)
		w.num(50, 0)
		w.int(71, 0)
		w.num(42, 2.5)
		w.pair(3, "txt")
		w.pair(4, "")
	}
	w.pair(0, "ENDTAB")
	for _, t := range []string{"VPORT", "VIEW", "UCS", "APPID", "DIMSTYLE"} {
		rs := d.Preserved.Tables[t]
		count := len(rs)
		if t == "APPID" {
			count++
		}
		tableStart(t, count)
		for _, r := range rs {
			w.tableRecord(r, tables[t], next)
		}
		if t == "APPID" {
			record(t, layerApp, "AcDbRegAppTableRecord")
		}
		w.pair(0, "ENDTAB")
	}
	if modern {
		tableStart("BLOCK_RECORD", 2+len(d.Blocks())+len(d.Preserved.Blocks))
		blockRecord := func(h, name string) {
			w.pair(0, "BLOCK_RECORD")
			w.pair(5, h)
			w.pair(330, tables["BLOCK_RECORD"])
			w.pair(100, "AcDbSymbolTableRecord")
			w.pair(100, "AcDbBlockTableRecord")
			w.pair(2, name)
		}
		blockRecord(model, "*Model_Space")
		blockRecord(paper, "*Paper_Space")
		for _, b := range d.Blocks() {
			blockRecord(b.H.String(), b.Name)
		}
		for i, b := range d.Preserved.Blocks {
			blockRecord(preservedOwners[i], b.Name())
		}
		w.pair(0, "ENDTAB")
	}
	end()
	for _, g := range d.Groups() {
		for _, h := range g.Members {
			w.reactors[h] = append(w.reactors[h], g.H)
		}
	}
	section("BLOCKS")
	block := func(name, owner string, base geom.Vec2, es []doc.Entity) {
		w.pair(0, "BLOCK")
		w.pair(5, next())
		if modern {
			w.pair(330, owner)
			w.pair(100, "AcDbEntity")
		}
		w.pair(8, "0")
		if modern {
			w.pair(100, "AcDbBlockBegin")
		}
		w.pair(2, name)
		flags := 0
		for _, e := range es {
			if a, ok := e.(*doc.AttributeDefinition); ok && !a.Constant {
				flags |= 2
			}
		}
		w.int(70, flags)
		w.point(10, base)
		w.pair(3, name)
		w.pair(1, "")
		for _, e := range es {
			w.symbolEntity(e, next, &warns)
		}
		w.pair(0, "ENDBLK")
		w.pair(5, next())
		if modern {
			w.pair(330, owner)
			w.pair(100, "AcDbEntity")
		}
		w.pair(8, "0")
		if modern {
			w.pair(100, "AcDbBlockEnd")
		}
	}
	if modern {
		block("*Model_Space", model, geom.V(0, 0), nil)
		block("*Paper_Space", paper, geom.V(0, 0), nil)
	}
	for _, b := range d.Blocks() {
		block(b.Name, b.H.String(), b.Base, b.Entities)
	}
	for i, b := range d.Preserved.Blocks {
		if modern {
			w.raw(completePreservedBlockOwner(b.Pairs, preservedOwners[i]))
		} else {
			warns = w.block(warns, b)
		}
	}
	end()
	section("ENTITIES")
	for _, h := range d.Handles() {
		w.symbolEntity(d.Entity(h), next, &warns)
	}
	end()
	if modern {
		section("OBJECTS")
		dictionary := func(h, owner string, entries []doc.Pair) {
			w.pair(0, "DICTIONARY")
			w.pair(5, h)
			w.pair(330, owner)
			w.pair(100, "AcDbDictionary")
			w.int(281, 1)
			w.raw(entries)
		}
		entries := []doc.Pair{{Code: 3, Value: "ACAD_GROUP"}, {Code: 350, Value: groupDict}}
		for _, r := range d.Preserved.Objects {
			os := objectsFromRaw(r.Pairs)
			if len(os) == 1 && os[0].typ == "DICTIONARY" && os[0].str(330) == "0" {
				var name string
				for _, p := range os[0].pairs {
					if p.code == 3 {
						name = p.val
					}
					if (p.code == 350 || p.code == 360) && name != "ACAD_GROUP" {
						entries = append(entries, doc.Pair{Code: 3, Value: name}, doc.Pair{Code: p.code, Value: p.val})
					}
				}
			}
		}
		dictionary(root, "0", entries)
		entries = nil
		for _, g := range d.Groups() {
			entries = append(entries, doc.Pair{Code: 3, Value: g.Name}, doc.Pair{Code: 350, Value: g.H.String()})
		}
		dictionary(groupDict, root, entries)
		for _, g := range d.Groups() {
			w.pair(0, "GROUP")
			w.pair(5, g.H.String())
			w.pair(102, "{ACAD_REACTORS")
			w.pair(330, groupDict)
			w.pair(102, "}")
			w.pair(330, groupDict)
			w.pair(100, "AcDbGroup")
			w.pair(300, g.Description)
			w.int(70, boolInt(g.Unnamed))
			w.int(71, boolInt(g.Selectable))
			for _, h := range g.Members {
				w.pair(340, h.String())
			}
		}
		for _, r := range d.Preserved.Objects {
			os := objectsFromRaw(r.Pairs)
			if len(os) == 1 && os[0].typ == "DICTIONARY" {
				if os[0].str(330) == "0" || dictionaryHasGroup(os[0], d) {
					continue
				}
			}
			w.raw(w.remapObjects(r.Pairs, root, model, paper, d))
		}
		end()
	} else if len(d.Preserved.Objects) > 0 {
		warns = append(warns, "R12 output does not include non-graphical object records")
	}
	w.pair(0, "EOF")
	if w.err == nil {
		w.err = w.w.Flush()
	}
	if w.err == nil {
		_, w.err = out.Write(bytes.Replace(payload.Bytes(), []byte("CADLITE_HANDSEED"), []byte(sub.String()), 1))
	}
	return warns, w.err
}
func (w *writer) raw(ps []doc.Pair) {
	for _, p := range ps {
		w.pair(p.Code, p.Value)
	}
}
func objectsFromRaw(ps []doc.Pair) []object {
	var out []object
	for _, p := range ps {
		if p.Code == 0 {
			out = append(out, object{typ: p.Value})
		} else if len(out) > 0 {
			out[len(out)-1].pairs = append(out[len(out)-1].pairs, pair{code: p.Code, val: p.Value})
		}
	}
	return out
}
func dictionaryHasGroup(o object, d *doc.Drawing) bool {
	for _, p := range o.pairs {
		if p.code == 350 {
			h, _ := doc.ParseHandle(p.val)
			for _, g := range d.Groups() {
				if g.H == h {
					return true
				}
			}
		}
	}
	return false
}
func (w *writer) symbolEntity(e doc.Entity, next func() string, warns *[]string) {
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
		if w.modern {
			w.pair(100, "AcDbArc")
		}
		w.num(50, e.StartAngle*180/math.Pi)
		w.num(51, e.EndAngle*180/math.Pi)
	case *doc.Text:
		w.text(e)
	case *doc.MText:
		if w.modern {
			w.nativeMText(e)
		} else {
			w.mtextAsText(e, next)
			*warns = append(*warns, i18n.T("dxf.warn.mtext_as_text", 1))
		}
	case *doc.AttributeDefinition:
		w.typeOverride = "ATTDEF"
		w.attdef(e)
	case *doc.Insert:
		w.insert(e, next)
	case *doc.LWPolyline:
		if w.modern {
			w.head("LWPOLYLINE", e)
			w.int(90, len(e.Points))
			w.int(70, boolInt(e.Closed))
			for i, p := range e.Points {
				w.num(10, p.X)
				w.num(20, p.Y)
				if i < len(e.Bulges) {
					w.num(42, e.Bulges[i])
				}
			}
		} else {
			w.head("POLYLINE", e)
			w.int(66, 1)
			w.point(10, geom.V(0, 0))
			w.int(70, boolInt(e.Closed))
			for i, p := range e.Points {
				w.pair(0, "VERTEX")
				w.pair(5, next())
				w.pair(8, e.Layer)
				w.point(10, p)
				if i < len(e.Bulges) {
					w.num(42, e.Bulges[i])
				}
			}
			w.pair(0, "SEQEND")
			w.pair(5, next())
			w.pair(8, e.Layer)
		}
	case *doc.Unknown:
		if w.modern {
			w.raw(w.remapEntityOwners(e.Source()))
		} else if r12Entities[e.Type] {
			w.pairs(e.Source())
		} else {
			*warns = append(*warns, i18n.T("dxf.warn.not_written_r12", e.Type, e.Handle()))
		}
	default:
		*warns = append(*warns, "Unsupported entity: "+e.Class())
	}
}

func (w *writer) remapObjects(ps []doc.Pair, root, model, paper string, d *doc.Drawing) []doc.Pair {
	out := append([]doc.Pair(nil), ps...)
	roots := map[string]string{}
	for _, r := range d.Preserved.Objects {
		os := objectsFromRaw(r.Pairs)
		if len(os) == 1 && os[0].typ == "DICTIONARY" && os[0].str(330) == "0" {
			roots[os[0].str(5)] = root
		}
	}
	for i, p := range out {
		if p.Code == 330 || p.Code == 350 || p.Code == 360 {
			if n := roots[p.Value]; n != "" {
				out[i].Value = n
			}
			if d.Preserved.ModelOwner != 0 && p.Value == d.Preserved.ModelOwner.String() {
				out[i].Value = model
			}
			if d.Preserved.PaperOwner != 0 && p.Value == d.Preserved.PaperOwner.String() {
				out[i].Value = paper
			}
		}
	}
	return out
}

func (w *writer) tableRecord(r doc.RawRecord, owner string, next func() string) {
	if !w.modern {
		w.pairs(r.Pairs)
		return
	}
	var h string
	hasSubclass := false
	for _, p := range r.Pairs {
		if p.Code == 5 || p.Code == 105 {
			h = p.Value
		}
		if p.Code == 100 {
			hasSubclass = true
		}
	}
	if h == "" {
		h = next()
	}
	for i, p := range r.Pairs {
		if p.Code == 5 || p.Code == 105 || p.Code == 330 {
			continue
		}
		w.pair(p.Code, p.Value)
		if i == 0 {
			code := 5
			if r.Type() == "DIMSTYLE" {
				code = 105
			}
			w.pair(code, h)
			w.pair(330, owner)
			if !hasSubclass {
				w.pair(100, "AcDbSymbolTableRecord")
				sub := map[string]string{"STYLE": "AcDbTextStyleTableRecord", "LTYPE": "AcDbLinetypeTableRecord", "APPID": "AcDbRegAppTableRecord"}[r.Type()]
				if sub != "" {
					w.pair(100, sub)
				}
			}
		}
	}
}

func (w *writer) remapEntityOwners(ps []doc.Pair) []doc.Pair {
	out := append([]doc.Pair(nil), ps...)
	source := w.d.Preserved.ModelOwner
	if source == 0 {
		source = doc.ModelSpace
	}
	for i, p := range out {
		if p.Code == 330 && p.Value == source.String() {
			out[i].Value = w.modelOwner
		}
	}
	return out
}

func preservedBlockOwner(block doc.RawRecord) doc.Handle {
	record := ""
	reactor := false
	for _, p := range block.Pairs {
		if p.Code == 0 {
			if record != "" {
				break
			}
			record = p.Value
		}
		if p.Code == 102 {
			reactor = p.Value != "}"
		}
		if p.Code == 330 && !reactor {
			h, _ := doc.ParseHandle(p.Value)
			return h
		}
	}
	return 0
}
func completePreservedBlockOwner(ps []doc.Pair, owner string) []doc.Pair {
	objects := objectsFromRaw(ps)
	var out []doc.Pair
	for _, o := range objects {
		hasOwner := false
		reactor := false
		for _, p := range o.pairs {
			if p.code == 102 {
				reactor = p.val != "}"
			}
			if p.code == 330 && !reactor {
				hasOwner = true
			}
		}
		out = append(out, doc.Pair{Code: 0, Value: o.typ})
		for _, p := range o.pairs {
			out = append(out, doc.Pair{Code: p.code, Value: p.val})
		}
		if !hasOwner {
			out = append(out, doc.Pair{Code: 330, Value: owner})
		}
	}
	return out
}
