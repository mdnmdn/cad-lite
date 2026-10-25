package props

import (
	"runtime"
	"strconv"
	"strings"
	"sync"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
)

// Build returns the sheet of target t: for a selection the General category and, when the objects
// (after the class filter) share one class, that class's categories; for a layer its record; for
// the defaults the current properties, the drawing's global values and the current dimension
// style. Values that differ across the selection are Varies.
func Build(h Host, t Target) Sheet {
	c := newEnv(h.Drawing(), h.Vars(), t.Vertex)
	s := Sheet{Target: t, Editable: h.Active() == ""}
	switch t.Kind {
	case TargetLayer:
		c.buildLayer(&s)
	case TargetDefaults:
		c.buildDefaults(&s)
	default:
		c.buildSelection(&s)
	}
	return s
}

// selection returns the live objects of t that pass the class filter, the per-class counts of
// all of them, and how many handles no longer name an object. A large selection is looked up in
// parallel parts.
func selection(d *doc.Drawing, t Target) (ents []doc.Entity, types []TypeCount, vanished int) {
	parts := chunks(len(t.Handles))
	type part struct {
		ents     []doc.Entity
		types    []TypeCount
		vanished int
	}
	ps := make([]part, len(parts))
	inParallel(parts, func(i, lo, hi int) {
		ps[i].ents, ps[i].types, ps[i].vanished = lookup(d, t.Handles[lo:hi], t.Class)
	})
	if len(ps) == 1 {
		ents, types, vanished = ps[0].ents, ps[0].types, ps[0].vanished
	} else {
		n := 0
		for _, p := range ps {
			n += len(p.ents)
		}
		ents = make([]doc.Entity, 0, n)
		idx := map[string]int{}
		for _, p := range ps {
			ents = append(ents, p.ents...)
			vanished += p.vanished
			for _, tc := range p.types {
				if i, ok := idx[tc.Class]; ok {
					types[i].Count += tc.Count
				} else {
					idx[tc.Class] = len(types)
					types = append(types, tc)
				}
			}
		}
	}
	for i := range types {
		types[i].Label = classLabel(types[i].Class)
	}
	return ents, types, vanished
}

// lookup is selection over one part of the handles.
func lookup(d *doc.Drawing, hs []doc.Handle, class string) (ents []doc.Entity, types []TypeCount, vanished int) {
	idx := map[string]int{}
	last, lastIdx := "", -1
	ents = make([]doc.Entity, 0, len(hs))
	for _, h := range hs {
		e := d.Entity(h)
		if e == nil {
			vanished++
			continue
		}
		cl := e.Class()
		if cl == last {
			types[lastIdx].Count++
		} else if i, ok := idx[cl]; ok {
			types[i].Count++
			last, lastIdx = cl, i
		} else {
			last, lastIdx = cl, len(types)
			idx[cl] = len(types)
			types = append(types, TypeCount{Class: cl, Count: 1})
		}
		if class == "" || class == cl {
			ents = append(ents, e)
		}
	}
	return ents, types, vanished
}

// classLabel names a DXF class in the current language, or returns it as is.
func classLabel(class string) string {
	k := "ui.props.class." + strings.ToLower(class)
	if i18n.Has(i18n.English, k) {
		return i18n.T(k)
	}
	return class
}

var (
	plineCatsOnce = sync.OnceValue(plineCats)
	dimCatsOnce   = sync.OnceValue(dimensionCats)
)

// classCats returns the categories of objects all of one class.
func classCats(class string, ents []doc.Entity) []catDef {
	switch class {
	case "LINE":
		return lineCats
	case "CIRCLE":
		return circleCats
	case "ARC":
		return arcCats
	case "LWPOLYLINE":
		return plineCatsOnce()
	case "TEXT":
		return textCats
	case "MTEXT":
		return mtextCats
	case "INSERT":
		return append(append([]catDef(nil), insertCats...), attributeCat(ents)...)
	case "DIMENSION":
		return dimCatsOnce()
	}
	return nil
}

// catsFor returns every category of ents: General, then the class's when they share one.
func catsFor(ents []doc.Entity) []catDef {
	out := []catDef{{id: "general", title: "ui.props.cat.general", defs: generalDefs}}
	if len(ents) == 0 {
		return out
	}
	cl := ents[0].Class()
	for _, e := range ents[1:] {
		if e.Class() != cl {
			return out
		}
	}
	return append(out, classCats(cl, ents)...)
}

func (c *env) buildSelection(s *Sheet) {
	ents, types, _ := selection(c.d, s.Target)
	s.Types, s.Count = types, len(ents)
	if len(ents) == 0 {
		s.Title = i18n.T("ui.props.none")
		return
	}
	s.Title = i18n.Plural("ui.props.selected", len(ents))
	c.first = ents[0]
	if p, ok := ents[0].(*doc.LWPolyline); ok {
		s.Vertices = len(p.Points)
		c.vertex = min(c.vertex, max(len(p.Points)-1, 0))
		s.Target.Vertex = c.vertex
	}
	cats := catsFor(ents)
	var all []*def
	for _, cd := range cats {
		all = append(all, cd.defs...)
	}
	rows := c.rows(all, ents)
	for _, cd := range cats {
		cat := Category{ID: cd.id, Title: cd.title}
		for range cd.defs {
			if p := rows[0]; p != nil {
				cat.Properties = append(cat.Properties, *p)
			}
			rows = rows[1:]
		}
		if len(cat.Properties) > 0 {
			s.Categories = append(s.Categories, cat)
		}
	}
}

// agg is one row's aggregate over a selection.
type agg struct {
	dd                *def
	first             doc.Entity
	v                 Value
	varies, clearable bool
}

// settled reports that more objects cannot change the aggregate.
func (a *agg) settled() bool { return a.varies && (a.clearable || a.dd.clearable == nil) }

// sameValue is Value equality, spelled out so the hot loop of rows does not call the generated
// comparison.
func sameValue(a, b *Value) bool {
	return a.Num == b.Num && a.Int == b.Int && a.Bool == b.Bool && a.Varies == b.Varies && a.Clear == b.Clear && a.Str == b.Str
}

// commonAgg aggregates the General rows that read EntityCommon fields directly: one comparison of
// the fields per object instead of a call per row.
type commonAgg struct {
	first                              doc.EntityCommon
	n                                  int
	color, layer, ltype, scale, weight bool // varies
	clrColor, clrType, clrScale, clrW  bool // clearable somewhere
}

func (g *commonAgg) add(c *doc.EntityCommon) {
	if g.n == 0 {
		g.first = *c
	} else {
		f := &g.first
		if !g.color && c.Color != f.Color {
			g.color = normColor(c.Color) != normColor(f.Color)
		}
		g.layer = g.layer || c.Layer != f.Layer
		if !g.ltype && c.Linetype != f.Linetype {
			g.ltype = cmdsys.LinetypeText(c.Linetype) != cmdsys.LinetypeText(f.Linetype)
		}
		g.scale = g.scale || c.LinetypeScale != f.LinetypeScale && c.LtScale() != f.LtScale()
		g.weight = g.weight || c.Lineweight != f.Lineweight
	}
	g.n++
	g.clrColor = g.clrColor || c.Color != doc.ByLayer && normColor(c.Color) != doc.ByLayer
	g.clrType = g.clrType || c.Linetype != "" && !doc.IsByLayerLinetype(c.Linetype)
	g.clrScale = g.clrScale || c.LinetypeScale != 0 && c.LtScale() != 1
	g.clrW = g.clrW || c.Lineweight != doc.LineweightByLayer
}

// commonRows are the General rows commonAgg covers, with how to read its result.
var commonRows = map[string]func(g *commonAgg) (varies, clearable bool){
	"color":      func(g *commonAgg) (bool, bool) { return g.color, g.clrColor },
	"layer":      func(g *commonAgg) (bool, bool) { return g.layer, false },
	"linetype":   func(g *commonAgg) (bool, bool) { return g.ltype, g.clrType },
	"ltscale":    func(g *commonAgg) (bool, bool) { return g.scale, g.clrScale },
	"lineweight": func(g *commonAgg) (bool, bool) { return g.weight, g.clrW },
}

// parallelMin is the selection size from which Build reads the objects on several goroutines.
// The rows only read the drawing (a TEXT may fill its own layout cache, touched by one goroutine).
const parallelMin = 16384

// chunks splits n items into ranges for the CPUs, one range below parallelMin.
func chunks(n int) [][2]int {
	k := 1
	if n >= parallelMin {
		k = min(runtime.GOMAXPROCS(0), n/(parallelMin/2))
	}
	out := make([][2]int, k)
	for i := range k {
		out[i] = [2]int{n * i / k, n * (i + 1) / k}
	}
	return out
}

// inParallel runs fn on each range, concurrently when there are several.
func inParallel(rs [][2]int, fn func(i, lo, hi int)) {
	if len(rs) == 1 {
		fn(0, rs[0][0], rs[0][1])
		return
	}
	var wg sync.WaitGroup
	for i, r := range rs {
		wg.Go(func() { fn(i, r[0], r[1]) })
	}
	wg.Wait()
}

// isCommonRow reports whether row i of defs is a General row commonAgg covers.
func isCommonRow(defs []*def, i int) bool {
	_, ok := commonRows[defs[i].id]
	return ok && defs[i].only == nil && i < len(generalDefs) && defs[i] == generalDefs[i]
}

// scan aggregates defs over ents, dropping a row from the pass once it is settled.
func (c *env) scan(defs []*def, ents []doc.Entity) ([]agg, commonAgg) {
	aggs := make([]agg, len(defs))
	active := make([]*agg, 0, len(defs))
	var g commonAgg
	for i, dd := range defs {
		aggs[i].dd = dd
		if !isCommonRow(defs, i) {
			active = append(active, &aggs[i])
		}
	}
	for _, e := range ents {
		g.add(e.Common())
		n := 0
		for _, a := range active {
			dd := a.dd
			switch {
			case dd.num != nil && dd.only == nil && a.first != nil:
				if !a.varies && dd.num(e) != a.v.Num {
					a.varies = true
				}
			case dd.only == nil || dd.only(c, e):
				x := dd.get(c, e)
				switch {
				case a.first == nil:
					a.first, a.v, a.varies = e, x, x.Varies
				case !a.varies && !sameValue(&x, &a.v):
					a.varies = true
				}
			}
			if !a.clearable && dd.clearable != nil && (dd.only == nil || dd.only(c, e)) && dd.clearable(e) {
				a.clearable = true
			}
			if !a.settled() {
				active[n] = a
				n++
			}
		}
		active = active[:n]
	}
	return aggs, g
}

// merge folds the aggregate of a later part of the selection into a.
func (a *agg) merge(b *agg) {
	switch {
	case b.first == nil:
	case a.first == nil:
		*a = *b
		return
	case b.varies || !sameValue(&a.v, &b.v):
		a.varies = true
	}
	a.clearable = a.clearable || b.clearable
}

// merge folds the General aggregate of a later part of the selection into g.
func (g *commonAgg) merge(o *commonAgg) {
	if o.n == 0 {
		return
	}
	if g.n == 0 {
		*g = *o
		return
	}
	n := g.n
	g.add(&o.first)
	g.n = n + o.n
	g.color, g.layer, g.ltype = g.color || o.color, g.layer || o.layer, g.ltype || o.ltype
	g.scale, g.weight = g.scale || o.scale, g.weight || o.weight
	g.clrColor, g.clrType = g.clrColor || o.clrColor, g.clrType || o.clrType
	g.clrScale, g.clrW = g.clrScale || o.clrScale, g.clrW || o.clrW
}

// rows aggregates every row over ents in one pass over the objects (a selection of many objects
// is read once, not once per row, and in parallel parts when it is large). A row that applies to
// none of ents is nil.
func (c *env) rows(defs []*def, ents []doc.Entity) []*Property {
	parts := chunks(len(ents))
	aggs := make([][]agg, len(parts))
	gs := make([]commonAgg, len(parts))
	inParallel(parts, func(i, lo, hi int) { aggs[i], gs[i] = c.scan(defs, ents[lo:hi]) })
	all, g := aggs[0], gs[0]
	for i := 1; i < len(parts); i++ {
		for j := range all {
			all[j].merge(&aggs[i][j])
		}
		g.merge(&gs[i])
	}
	out := make([]*Property, len(defs))
	for i := range all {
		a := &all[i]
		if isCommonRow(defs, i) && len(ents) > 0 {
			f := commonRows[a.dd.id]
			a.first = ents[0]
			a.v = a.dd.get(c, ents[0])
			a.varies, a.clearable = f(&g)
		}
		if a.first != nil {
			p := c.property(a)
			out[i] = &p
		}
	}
	return out
}

// property turns an aggregate into its row.
func (c *env) property(a *agg) Property {
	dd := a.dd
	p := Property{ID: dd.id, Label: dd.label, Tip: dd.tip, Name: dd.name, Kind: dd.kind, ReadOnly: dd.readOnly || dd.set == nil && !dd.selector,
		Min: dd.min, Max: dd.max, Positive: dd.positive, Var: dd.varName, Selector: dd.selector, Clearable: a.clearable && !dd.readOnly,
		Overridden: dd.dimVars != 0 && a.clearable}
	if p.Label == "" {
		p.Label = label(dd.id)
	}
	if dd.options != nil {
		p.Options = dd.options(c)
	}
	if a.varies {
		p.Value = Value{Varies: true}
	} else {
		p.Value = a.v
		if dd.source != nil {
			p.Source, p.From = dd.source(c, a.first)
		}
		if dd.sample != nil {
			p.Sample = dd.sample(c, a.first)
		}
	}
	p.Display = c.display(p)
	p.Inherited = inheritedText(p.Source, p.From)
	return p
}

// inheritedText is the tip that says where an inherited value comes from.
func inheritedText(s Source, from string) string {
	var what string
	switch s {
	case SourceLayer:
		what = i18n.T("ui.props.source.layer", from)
	case SourceBlock:
		what = i18n.T("ui.props.source.block")
	case SourceStyle:
		what = i18n.T("ui.props.source.style", from)
	case SourceDefault:
		what = i18n.T("ui.props.source.default")
	default:
		return ""
	}
	return i18n.T("ui.props.inherited_tip", what)
}

// --- display ------------------------------------------------------------

func (c *env) intVar(name string, def int) int {
	if c.vars == nil || !c.vars.Has(name) {
		return def
	}
	return c.vars.Int(name)
}

// fmtLength formats a length in LUNITS with LUPREC.
func (c *env) fmtLength(v float64) string {
	return cmdsys.FormatLength(v, c.intVar("LUNITS", cmdsys.LUnitsDecimal), c.intVar("LUPREC", 4))
}

// fmtAngle formats an angle in AUNITS with AUPREC, measured from the X axis.
func (c *env) fmtAngle(v float64) string {
	return cmdsys.FormatAngle(v, c.intVar("AUNITS", cmdsys.AUnitsDegrees), c.intVar("AUPREC", 0))
}

// display renders p's value.
func (c *env) display(p Property) string {
	v := p.Value
	if v.Varies {
		return i18n.T("ui.props.varies")
	}
	for _, o := range p.Options {
		if o.Value == v {
			return o.Label
		}
	}
	switch p.Kind {
	case KindLength:
		return c.fmtLength(v.Num)
	case KindAngle:
		return c.fmtAngle(v.Num)
	case KindNumber:
		return cmdsys.FormatFixed(v.Num, c.intVar("LUPREC", 4))
	case KindInt:
		if p.Selector {
			return strconv.Itoa(v.Int + 1)
		}
		return strconv.Itoa(v.Int)
	case KindBool:
		if v.Bool {
			return i18n.T("ui.props.yes")
		}
		return i18n.T("ui.props.no")
	case KindColor:
		return colorLabel(v.Int)
	case KindLineweight:
		return lineweightLabel(doc.Lineweight(v.Int))
	case KindLinetype:
		switch {
		case doc.IsByLayerLinetype(v.Str):
			return i18n.T("ui.props.opt.bylayer")
		case doc.IsByBlockLinetype(v.Str):
			return i18n.T("ui.props.opt.byblock")
		}
	case KindArrow:
		return arrowLabel(v.Str)
	}
	return v.Str
}

// --- the layer target ---------------------------------------------------

func (c *env) buildLayer(s *Sheet) {
	l := c.d.Layer(s.Target.Layer)
	if l == nil {
		s.Title = i18n.T("ui.props.none")
		return
	}
	s.Title = i18n.T("ui.props.layer_title", l.Name)
	lt := l.Linetype
	if lt == "" {
		lt = doc.LinetypeContinuous
	}
	if t, ok := c.d.Linetype(lt); ok {
		lt = t.Name
	}
	lw := doc.LineweightFromLayer(l.Lineweight)
	lwShown := lw
	if lw == doc.LineweightDefault {
		lwShown = c.d.LWDefaultValue()
	}
	rows := []Property{
		{ID: "name", Kind: KindText, ReadOnly: true, Value: StrValue(l.Name)},
		{ID: "current", Kind: KindBool, ReadOnly: true, Value: BoolValue(l.Name == c.d.CurrentLayer)},
		{ID: "color", Kind: KindColor, Value: IntValue(c.layerColor(l.Name)), Sample: Sample{ACI: c.layerColor(l.Name)}, Options: colorOptions(c, l, false)},
		{ID: "linetype", Kind: KindLinetype, Value: StrValue(lt), Sample: c.linetypeSample(lt), Options: linetypeOptions(c, l, false)},
		{ID: "lineweight", Kind: KindLineweight, Value: IntValue(int(lw)), Sample: Sample{Lineweight: lwShown}, Options: lineweightOptions(c, l, false, true)},
		{ID: "on", Kind: KindBool, Value: BoolValue(!l.Off)},
		{ID: "frozen", Kind: KindBool, Value: BoolValue(l.Frozen), ReadOnly: l.Name == c.d.CurrentLayer && !l.Frozen},
		{ID: "locked", Kind: KindBool, Value: BoolValue(l.Locked)},
		{ID: "plot", Kind: KindBool, Value: BoolValue(!l.NoPlot)},
		{ID: "description", Kind: KindText, Value: StrValue(l.Description)},
	}
	if lw == doc.LineweightDefault {
		rows[4].Source = SourceDefault
		rows[4].Inherited = inheritedText(SourceDefault, "")
	}
	if rows[6].ReadOnly {
		rows[6].Tip = "ui.props.tip.freeze_current"
	}
	cat := Category{ID: "layer", Title: "ui.props.cat.layer"}
	for _, p := range rows {
		p.Label = label("layer_" + p.ID)
		p.Display = c.display(p)
		cat.Properties = append(cat.Properties, p)
	}
	s.Categories = []Category{cat}
}

// --- the defaults target ------------------------------------------------

// defaultsRow is a Defaults row before display: the property, and its value read from Vars.
func (c *env) defaultsRow(p Property) Property {
	if p.Label == "" {
		p.Label = label(p.ID)
	}
	p.Display = c.display(p)
	p.Inherited = inheritedText(p.Source, p.From)
	if c.vars != nil && p.Var != "" && !c.vars.Undoable(p.Var) {
		p.NotUndoable = true
	}
	return p
}

func (c *env) buildDefaults(s *Sheet) {
	d := c.d
	s.Title = i18n.T("ui.props.defaults_title")
	cur := d.Current()
	l := d.Layer(cur.Layer)
	resolved := d.Resolve(&doc.EntityCommon{Layer: cur.Layer, Color: normColor(cur.Color), Linetype: cur.Linetype, LinetypeScale: cur.LinetypeScale, Lineweight: cur.Lineweight}, nil)
	byLevel := func(layerLevel, blockLevel bool) (Source, string) {
		switch {
		case layerLevel:
			return SourceLayer, cur.Layer
		case blockLevel:
			return SourceBlock, ""
		}
		return SourceOwn, ""
	}
	color := Property{ID: "color", Kind: KindColor, Var: "CECOLOR", Value: IntValue(normColor(cur.Color)), Sample: Sample{ACI: resolved.Color}, Options: colorOptions(c, l, true)}
	color.Source, color.From = byLevel(normColor(cur.Color) == doc.ByLayer, cur.Color == doc.ByBlock)
	linetype := Property{ID: "linetype", Kind: KindLinetype, Var: "CELTYPE", Value: StrValue(cmdsys.LinetypeText(cur.Linetype)), Sample: c.linetypeSample(resolved.Linetype), Options: linetypeOptions(c, l, true)}
	linetype.Source, linetype.From = byLevel(doc.IsByLayerLinetype(cur.Linetype), doc.IsByBlockLinetype(cur.Linetype))
	lineweight := Property{ID: "lineweight", Kind: KindLineweight, Var: "CELWEIGHT", Value: IntValue(int(cur.Lineweight)), Sample: Sample{Lineweight: resolved.Lineweight}, Options: lineweightOptions(c, l, true, true)}
	lineweight.Source, lineweight.From = byLevel(cur.Lineweight == doc.LineweightByLayer, cur.Lineweight == doc.LineweightByBlock)
	general := []Property{
		color,
		{ID: "layer", Kind: KindLayer, Var: "CLAYER", Value: StrValue(cur.Layer), Sample: Sample{ACI: c.layerColor(cur.Layer)}, Options: layerOptions(c)},
		linetype,
		{ID: "ltscale", Kind: KindNumber, Var: "CELTSCALE", Positive: true, Value: NumValue(doc.LinetypeScaleOf(cur.LinetypeScale))},
		lineweight,
	}
	var units []Option
	for code := 0; code <= 24; code++ {
		n := cmdsys.UnitName(code)
		if n == "" {
			n = i18n.T("ui.props.unitless")
		}
		units = append(units, Option{Value: IntValue(code), Label: n})
	}
	drawing := []Property{
		{ID: "global_ltscale", Kind: KindNumber, Var: "LTSCALE", Positive: true, Value: NumValue(d.LTScaleValue())},
		{ID: "lwdefault", Kind: KindLineweight, Var: "LWDEFAULT", Value: IntValue(int(d.LWDefaultValue())), Sample: Sample{Lineweight: d.LWDefaultValue()}, Options: widthOptions(c)},
		{ID: "insunits", Kind: KindEnum, Var: "INSUNITS", Value: IntValue(c.intVar("INSUNITS", d.Header.InsUnits)), Options: units},
	}
	texts := []Property{{ID: "textsize", Kind: KindLength, Var: "TEXTSIZE", Positive: true, Value: NumValue(c.realVar("TEXTSIZE", 2.5))}}
	add := func(id string, rows []Property) {
		cat := Category{ID: id, Title: "ui.props.cat." + id}
		for _, p := range rows {
			cat.Properties = append(cat.Properties, c.defaultsRow(p))
		}
		s.Categories = append(s.Categories, cat)
	}
	add("general", general)
	add("drawing", drawing)
	add("text", texts)
	st := d.ActiveDimStyle()
	add("dim_misc", []Property{{ID: "dimstyle", Kind: KindDimStyle, Var: "DIMSTYLE", Value: StrValue(st.Name), Options: dimStyleOptions(c)}})
	for _, dc := range dimCategories {
		var rows []Property
		for _, r := range dc.style {
			p := Property{ID: r.id, Kind: r.kind, Var: r.varName, Min: r.min, Max: r.max, Positive: r.pos, Value: r.get(st)}
			if r.options != nil {
				p.Options = r.options(c)
			}
			switch r.kind {
			case KindColor:
				p.Sample = Sample{ACI: d.ResolveColorIn(p.Value.Int, resolved)}
			case KindArrow:
				p.Sample = Sample{Arrow: p.Value.Str, HasArrow: true}
			}
			rows = append(rows, p)
		}
		if dc.id == "dim_lines" {
			rows = append(rows, Property{ID: "arc_symbol", Kind: KindEnum, Var: "DIMARCSYM", Value: IntValue(c.intVar("DIMARCSYM", 0)), Options: arcSymbolOptions(c)})
		}
		add(dc.id, rows)
	}
}

func (c *env) realVar(name string, def float64) float64 {
	if c.vars == nil || !c.vars.Has(name) {
		return def
	}
	return c.vars.Real(name)
}
