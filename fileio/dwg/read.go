// Package dwg reads Autodesk DWG drawings in pure Go: no cgo, no external library, so it builds
// for every platform the application does, the browser included. It reads R14 (AC1014), R2000
// (AC1015), R2004 (AC1018), R2007 (AC1021) and R2010 to R2018 (AC1024, AC1027, AC1032) into a
// doc.Drawing, and reports every other DWG version as unsupported. It never writes DWG.
//
// The reader is a Go port of the DWG reading logic of ACadSharp
// (https://github.com/DomCR/ACadSharp, MIT licence, Copyright (c) 2021 Albert Domenech; see
// LICENSE-ACadSharp in this directory), written against the Open Design Specification for .dwg
// files. No GPL or LGPL code was used.
//
// What is imported: the layer table, and the LINE, CIRCLE, ARC, LWPOLYLINE and 2D POLYLINE
// entities of model space. Every other entity is skipped and counted in a warning.
package dwg

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"cad-lite/doc"
	"cad-lite/i18n"
)

// Codec is the DWG reader. It implements fileio.Reader only.
type Codec struct{}

// Extensions implements fileio.Reader.
func (Codec) Extensions() []string { return []string{".dwg"} }

// Read implements fileio.Reader. It never panics: a panic in the parser, which a malformed file
// can provoke, comes back as an error.
func (Codec) Read(r io.Reader) (d *doc.Drawing, warns []string, err error) {
	defer func() {
		if p := recover(); p != nil {
			d, warns, err = nil, nil, errors.New(i18n.T("dwg.err.corrupt", fmt.Sprint(p)))
		}
	}()
	data, err := io.ReadAll(io.LimitReader(r, maxFileSize+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxFileSize {
		return nil, nil, errors.New(i18n.T("dwg.err.too_big"))
	}
	return importDrawing(data)
}

// importDrawing reads a whole DWG file into a drawing.
func importDrawing(data []byte) (*doc.Drawing, []string, error) {
	f, err := openFile(data)
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	return importFile(f)
}

// importFile reads the drawing of an opened file.
func importFile(f *file) (*doc.Drawing, []string, error) {
	data := f.data
	classes, err := f.readClasses()
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	omap, err := f.readObjectMap()
	if err != nil {
		return nil, nil, wrapErr(err)
	}
	src := &objSource{data: data, ver: f.ver, cp: f.codepg, classes: classes}
	if f.ver > verR15 {
		if src.data, err = f.section("AcDb:AcDbObjects"); err != nil || src.data == nil {
			return nil, nil, wrapErr(errCorrupt)
		}
	}
	imp := &importer{f: f, src: src, omap: omap, budget: 2*len(omap) + 16, ltNames: map[uint64]string{}, skipped: map[string]int{}}
	imp.run()
	hv, _ := f.readHeader()
	d, warns := imp.build(hv)
	return d, append(f.warns, warns...), nil
}

// wrapErr turns a reader failure into a localized error.
func wrapErr(err error) error {
	var u *unsupportedError
	switch {
	case errors.As(err, &u):
		rel := releaseNames[u.magic]
		if rel == "" {
			rel = "?"
		}
		return errors.New(i18n.T("dwg.err.unsupported", u.magic, rel))
	case errors.Is(err, errNotDWG):
		return errors.New(i18n.T("dwg.err.not_dwg"))
	}
	return errors.New(i18n.T("dwg.err.corrupt", err.Error()))
}

// ignoreCRC makes the importer parse objects whose CRC does not match. Only the fuzz and mutation
// tests set it: with the CRC check on, damaged objects never reach the parsers they are meant to
// exercise.
var ignoreCRC bool

// importer holds the state of one import.
type importer struct {
	f    *file
	src  *objSource
	omap map[uint64]int

	layers []layerRec
	// layerOrder is the layer control object's entry list: the table order.
	layerOrder []uint64
	ltNames    map[uint64]string
	// styleNames are the text style names by STYLE record handle.
	styleNames map[uint64]string
	msBlock    uint64 // the model space block record

	// ents are the entity objects in file order of their handles.
	ents    []entRef
	skipped map[string]int
	bad     int
	// paperSpace counts the paper space entities, which are not imported.
	paperSpace int
	badCRC     int
	// budget bounds the vertex records read for polylines, so crafted vertex chains cannot make
	// the import quadratic.
	budget int
	loaded []doc.Entity
}

type entRef struct {
	handle uint64
	typ    int
}

func (imp *importer) handles() []uint64 {
	hs := make([]uint64, 0, len(imp.omap))
	for h := range imp.omap {
		hs = append(hs, h)
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i] < hs[j] })
	return hs
}

// isEntityType reports whether objects of type t are entities.
func (imp *importer) isEntityType(t int) bool {
	if t >= classTypeBase {
		return imp.src.classes[t].entity
	}
	_, ok := fixedNames[t]
	return ok
}

// typeName is the DXF name of entity type t.
func (imp *importer) typeName(t int) string {
	if t >= classTypeBase {
		if n := imp.src.classes[t].dxfName; n != "" {
			return n
		}
		return fmt.Sprintf("CLASS%d", t)
	}
	if n, ok := fixedNames[t]; ok {
		return n
	}
	return fmt.Sprintf("TYPE%d", t)
}

// className returns the DXF class name of a class-numbered type, or "".
func (imp *importer) className(t int) string {
	if t >= classTypeBase {
		return imp.src.classes[t].dxfName
	}
	return ""
}

// run opens every object once: tables are read at once, entities are queued for the second pass.
func (imp *importer) run() {
	for _, h := range imp.handles() {
		o, err := imp.src.openObject(h, imp.omap[h])
		if err != nil {
			imp.bad++
			continue
		}
		if !o.crcOK && !ignoreCRC {
			imp.badCRC++
			continue
		}
		switch {
		case o.typ == tLayer:
			imp.guard(func() { imp.layers = append(imp.layers, o.readLayer()) })
		case o.typ == tLayerControl:
			imp.guard(func() { imp.layerOrder = o.readTable() })
		case o.typ == tLType:
			imp.guard(func() { imp.ltNames[h] = o.readName() })
		case o.typ == tStyle:
			imp.guard(func() {
				name := o.readName()
				if imp.styleNames == nil {
					imp.styleNames = map[uint64]string{}
				}
				imp.styleNames[h] = name
			})
		case o.typ == tBlockHeader:
			imp.guard(func() {
				if strings.EqualFold(o.readName(), "*Model_Space") {
					imp.msBlock = h
				}
			})
		case imp.isEntityType(o.typ):
			imp.ents = append(imp.ents, entRef{h, o.typ})
		}
	}
}

// guard runs f, counting a failure as an unreadable object.
func (imp *importer) guard(f func()) {
	if err := safe(f); err != nil {
		imp.bad++
	}
}

// layerIndex maps layer handles to names for the entity readers.
func (imp *importer) layerNames() func(uint64) string {
	names := make(map[uint64]string, len(imp.layers))
	for i := range imp.layers {
		names[imp.layers[i].handle] = imp.layers[i].layer.Name
	}
	return func(h uint64) string {
		if n, ok := names[h]; ok && n != "" {
			return n
		}
		return "0"
	}
}

// build runs the entity pass and assembles the drawing.
func (imp *importer) build(hv headerVars) (*doc.Drawing, []string) {
	d := doc.New()
	for _, rec := range imp.orderedLayers() {
		if rec.layer.Name == "" {
			continue
		}
		l := d.AddLayer(rec.layer.Name, 7)
		lt := imp.ltNames[rec.ltype]
		*l = rec.layer
		if strings.EqualFold(lt, "CONTINUOUS") || strings.EqualFold(lt, "BYLAYER") || strings.EqualFold(lt, "BYBLOCK") {
			lt = ""
		}
		l.Linetype = lt
	}
	imp.readEntities()
	var maxH uint64
	for h := range imp.omap {
		maxH = max(maxH, h)
	}
	for _, e := range imp.loaded {
		d.Load(e)
	}
	d.Header.HandSeed = max(d.Header.HandSeed, doc.Handle(maxH+1), doc.Handle(hv.handSeed))
	if hv.hasUnits && hv.insUnits >= 0 && hv.insUnits <= 24 {
		d.Header.InsUnits = hv.insUnits
	}
	if n := imp.layerNames()(hv.clayer); hv.clayer != 0 && d.Layer(n) != nil {
		d.CurrentLayer = n
	}
	if d.Layer(d.CurrentLayer) == nil {
		d.CurrentLayer = "0"
	}
	return d, imp.warnings()
}

// orderedLayers returns the layers in table order: the order of the layer control object's entries,
// then any layer it does not list, in handle order.
func (imp *importer) orderedLayers() []layerRec {
	byHandle := make(map[uint64]int, len(imp.layers))
	for i, l := range imp.layers {
		byHandle[l.handle] = i
	}
	out := make([]layerRec, 0, len(imp.layers))
	used := make([]bool, len(imp.layers))
	for _, h := range imp.layerOrder {
		if i, ok := byHandle[h]; ok && !used[i] {
			used[i] = true
			out = append(out, imp.layers[i])
		}
	}
	for i, l := range imp.layers {
		if !used[i] {
			out = append(out, l)
		}
	}
	return out
}

// warnings summarizes what was skipped.
func (imp *importer) warnings() []string {
	var w []string
	if len(imp.skipped) > 0 {
		type kv struct {
			name string
			n    int
		}
		var l []kv
		for k, v := range imp.skipped {
			l = append(l, kv{k, v})
		}
		sort.Slice(l, func(i, j int) bool {
			if l[i].n != l[j].n {
				return l[i].n > l[j].n
			}
			return l[i].name < l[j].name
		})
		parts := make([]string, len(l))
		for i, e := range l {
			parts[i] = fmt.Sprintf("%d %s", e.n, e.name)
		}
		w = append(w, i18n.T("dwg.warn.skipped", strings.Join(parts, ", ")))
	}
	if imp.paperSpace > 0 {
		w = append(w, i18n.T("dwg.warn.paper_space", imp.paperSpace))
	}
	if imp.bad > 0 {
		w = append(w, i18n.T("dwg.warn.unreadable", imp.bad))
	}
	if imp.badCRC > 0 {
		w = append(w, i18n.T("dwg.warn.crc", imp.badCRC))
	}
	return w
}
