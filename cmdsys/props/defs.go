package props

import (
	"reflect"
	"slices"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
)

// env is what the rows read while building or applying: the drawing, the units and the vertex.
type env struct {
	d      *doc.Drawing
	vars   *cmdsys.Vars
	vertex int
	// first is the first object of the selection: the samples of ByLayer and ByBlock options
	// resolve against it.
	first doc.Entity
}

func newEnv(d *doc.Drawing, v *cmdsys.Vars, vertex int) *env {
	return &env{d: d, vars: v, vertex: max(vertex, 0)}
}

// def is one entity row: how to read it from an object, write it, clear it and offer choices.
type def struct {
	id, label, tip string
	kind           Kind
	readOnly       bool
	positive       bool
	min, max       float64
	selector       bool
	// get reads the value from e; set writes v (validated by check and canon) into e.
	get func(c *env, e doc.Entity) Value
	set func(c *env, e doc.Entity, v Value) error
	// num, when set, is get for a plain number row, which Build's aggregation reads without
	// building a Value per object.
	num func(e doc.Entity) float64
	// clear drops e's own value (Value.Clear); clearable says whether e has one to drop.
	clear     func(c *env, e doc.Entity)
	clearable func(e doc.Entity) bool
	// options lists the choices; source says where e's value comes from; sample resolves the
	// current value's swatch for e.
	options func(c *env) []Option
	source  func(c *env, e doc.Entity) (Source, string)
	sample  func(c *env, e doc.Entity) Sample
	// only limits the row to the objects it applies to (an attribute tag, a polyline vertex, an
	// arc length dimension's symbol); nil is every object.
	only func(c *env, e doc.Entity) bool
	// name is a literal label (an attribute tag) shown instead of the catalog label.
	name string
	// dimVars are the dimension variables a style row reads; varName the one it shows.
	dimVars doc.DimVar
	varName string
}

// catDef is a category of entity rows.
type catDef struct {
	id, title string
	defs      []*def
}

func label(id string) string { return "ui.props.p." + id }

// --- the General category -------------------------------------------------

func common(e doc.Entity) *doc.EntityCommon { return e.Common() }

// normColor reads an entity colour as stored: 1–255, ByBlock 0, anything else ByLayer.
func normColor(c int) int {
	if c >= 0 && c <= 255 {
		return c
	}
	return doc.ByLayer
}

var generalDefs = []*def{
	{id: "color", kind: KindColor,
		get:       func(_ *env, e doc.Entity) Value { return IntValue(normColor(common(e).Color)) },
		set:       func(_ *env, e doc.Entity, v Value) error { common(e).Color = v.Int; return nil },
		clear:     func(_ *env, e doc.Entity) { common(e).Color = doc.ByLayer },
		clearable: func(e doc.Entity) bool { return normColor(common(e).Color) != doc.ByLayer },
		options:   func(c *env) []Option { return colorOptions(c, c.layerOf(c.first), true) },
		source: func(c *env, e doc.Entity) (Source, string) {
			switch normColor(common(e).Color) {
			case doc.ByLayer:
				return SourceLayer, common(e).Layer
			case doc.ByBlock:
				return SourceBlock, ""
			}
			return SourceOwn, ""
		},
		sample: func(c *env, e doc.Entity) Sample { return Sample{ACI: c.d.ResolveEntity(e).Color} }},
	{id: "layer", kind: KindLayer,
		get:     func(_ *env, e doc.Entity) Value { return StrValue(common(e).Layer) },
		set:     func(_ *env, e doc.Entity, v Value) error { common(e).Layer = v.Str; return nil },
		options: layerOptions,
		sample:  func(c *env, e doc.Entity) Sample { return Sample{ACI: c.layerColor(common(e).Layer)} }},
	{id: "linetype", kind: KindLinetype,
		get:       func(_ *env, e doc.Entity) Value { return StrValue(cmdsys.LinetypeText(common(e).Linetype)) },
		set:       func(_ *env, e doc.Entity, v Value) error { common(e).Linetype = storedLinetype(v.Str); return nil },
		clear:     func(_ *env, e doc.Entity) { common(e).Linetype = "" },
		clearable: func(e doc.Entity) bool { return !doc.IsByLayerLinetype(common(e).Linetype) },
		options:   func(c *env) []Option { return linetypeOptions(c, c.layerOf(c.first), true) },
		source: func(c *env, e doc.Entity) (Source, string) {
			switch lt := common(e).Linetype; {
			case doc.IsByLayerLinetype(lt):
				return SourceLayer, common(e).Layer
			case doc.IsByBlockLinetype(lt):
				return SourceBlock, ""
			}
			return SourceOwn, ""
		},
		sample: func(c *env, e doc.Entity) Sample { return c.linetypeSample(c.d.ResolveEntity(e).Linetype) }},
	{id: "ltscale", kind: KindNumber, positive: true,
		get:       func(_ *env, e doc.Entity) Value { return NumValue(common(e).LtScale()) },
		set:       func(_ *env, e doc.Entity, v Value) error { common(e).LinetypeScale = v.Num; return nil },
		clear:     func(_ *env, e doc.Entity) { common(e).LinetypeScale = 0 },
		clearable: func(e doc.Entity) bool { return common(e).LtScale() != 1 }},
	{id: "lineweight", kind: KindLineweight,
		get:       func(_ *env, e doc.Entity) Value { return IntValue(int(common(e).Lineweight)) },
		set:       func(_ *env, e doc.Entity, v Value) error { common(e).Lineweight = doc.Lineweight(v.Int); return nil },
		clear:     func(_ *env, e doc.Entity) { common(e).Lineweight = doc.LineweightByLayer },
		clearable: func(e doc.Entity) bool { return common(e).Lineweight != doc.LineweightByLayer },
		options:   func(c *env) []Option { return lineweightOptions(c, c.layerOf(c.first), true, true) },
		source: func(c *env, e doc.Entity) (Source, string) {
			switch common(e).Lineweight {
			case doc.LineweightByLayer:
				return SourceLayer, common(e).Layer
			case doc.LineweightByBlock:
				return SourceBlock, ""
			case doc.LineweightDefault:
				return SourceDefault, ""
			}
			return SourceOwn, ""
		},
		sample: func(c *env, e doc.Entity) Sample { return Sample{Lineweight: c.d.ResolveEntity(e).Lineweight} }},
	{id: "handle", kind: KindText, readOnly: true,
		get: func(_ *env, e doc.Entity) Value { return StrValue(e.Handle().String()) }},
}

// storedLinetype converts a linetype Value (BYLAYER, BYBLOCK or a table name) to the entity field.
func storedLinetype(s string) string {
	switch {
	case doc.IsByLayerLinetype(s):
		return ""
	case doc.IsByBlockLinetype(s):
		return doc.LinetypeByBlock
	}
	return s
}

// layerOf returns the layer whose values ByLayer options show: the first object's, else the
// current layer.
func (c *env) layerOf(e doc.Entity) *doc.Layer {
	if e != nil {
		if l := c.d.Layer(e.Common().Layer); l != nil {
			return l
		}
	}
	return c.d.Layer(c.d.CurrentLayer)
}

func (c *env) layerColor(name string) int {
	if l := c.d.Layer(name); l != nil && l.Color >= 1 && l.Color <= 255 {
		return l.Color
	}
	return 7
}

// linetypeSample is the swatch of an effective linetype name.
func (c *env) linetypeSample(name string) Sample {
	if name == "" {
		name = doc.LinetypeContinuous
	}
	s := Sample{Linetype: name}
	if l, ok := c.d.Linetype(name); ok {
		s.Linetype, s.Pattern = l.Name, l.Pattern
	} else if l, ok := doc.LibraryLinetype(name); ok {
		s.Linetype, s.Pattern = l.Name, l.Pattern
	}
	return s
}

// --- choices --------------------------------------------------------------

// standardColors are the named ACIs the colour list offers, as AutoCAD's palette does.
var standardColors = []struct {
	aci int
	key string
}{{1, "red"}, {2, "yellow"}, {3, "green"}, {4, "cyan"}, {5, "blue"}, {6, "magenta"}, {7, "white"}}

// colorLabel is a colour's name: ByLayer, ByBlock, Red ... White, or "Color n".
func colorLabel(c int) string {
	switch c {
	case doc.ByLayer:
		return i18n.T("ui.props.opt.bylayer")
	case doc.ByBlock:
		return i18n.T("ui.props.opt.byblock")
	}
	for _, s := range standardColors {
		if s.aci == c {
			return i18n.T("ui.props.aci." + s.key)
		}
	}
	return i18n.T("ui.props.aci.n", c)
}

// colorOptions lists ByLayer (sampling l's colour) and ByBlock when byLevels, then the seven
// standard colours. More colours come from the UI's picker (ui.props.more_colors).
func colorOptions(_ *env, l *doc.Layer, byLevels bool) []Option {
	var out []Option
	if byLevels {
		lc := 7
		if l != nil && l.Color >= 1 && l.Color <= 255 {
			lc = l.Color
		}
		out = append(out, Option{Value: IntValue(doc.ByLayer), Label: colorLabel(doc.ByLayer), Sample: Sample{ACI: lc}},
			Option{Value: IntValue(doc.ByBlock), Label: colorLabel(doc.ByBlock), Sample: Sample{ACI: 7}})
	}
	for _, s := range standardColors {
		out = append(out, Option{Value: IntValue(s.aci), Label: colorLabel(s.aci), Sample: Sample{ACI: s.aci}})
	}
	return out
}

// layerOptions lists every layer with its colour.
func layerOptions(c *env) []Option {
	ls := c.d.Layers()
	out := make([]Option, 0, len(ls))
	for _, l := range ls {
		out = append(out, Option{Value: StrValue(l.Name), Label: l.Name, Sample: Sample{ACI: c.layerColor(l.Name)}})
	}
	return out
}

// linetypeOptions lists ByLayer (sampling l's linetype) and ByBlock when byLevels, the loaded
// linetypes, then the library ones not loaded yet (Option.Load).
func linetypeOptions(c *env, l *doc.Layer, byLevels bool) []Option {
	var out []Option
	if byLevels {
		lt := ""
		if l != nil {
			lt = l.Linetype
		}
		out = append(out, Option{Value: StrValue(doc.LinetypeByLayer), Label: i18n.T("ui.props.opt.bylayer"), Sample: c.linetypeSample(lt)},
			Option{Value: StrValue(doc.LinetypeByBlock), Label: i18n.T("ui.props.opt.byblock"), Sample: c.linetypeSample("")})
	}
	for _, t := range c.d.Linetypes() {
		out = append(out, Option{Value: StrValue(t.Name), Label: t.Name, Sample: Sample{Linetype: t.Name, Pattern: t.Pattern}})
	}
	for _, t := range doc.LibraryLinetypes() {
		if _, ok := c.d.Linetype(t.Name); !ok {
			out = append(out, Option{Value: StrValue(t.Name), Label: t.Name, Sample: Sample{Linetype: t.Name, Pattern: t.Pattern}, Load: true})
		}
	}
	return out
}

// lineweightLabel is a lineweight's name: ByLayer, ByBlock, Default or "0.25 mm".
func lineweightLabel(w doc.Lineweight) string {
	switch w {
	case doc.LineweightByLayer:
		return i18n.T("ui.props.opt.bylayer")
	case doc.LineweightByBlock:
		return i18n.T("ui.props.opt.byblock")
	case doc.LineweightDefault:
		return i18n.T("ui.props.opt.default")
	}
	return i18n.T("ui.props.opt.mm", cmdsys.LineweightText(w))
}

// lineweightOptions lists ByLayer (sampling l's lineweight) and ByBlock when byLevels, Default,
// then AutoCAD's widths (0.00 mm only when zero, which a layer cannot hold).
func lineweightOptions(c *env, l *doc.Layer, byLevels, zero bool) []Option {
	def := c.d.LWDefaultValue()
	var out []Option
	if byLevels {
		lw := def
		if l != nil {
			if w := doc.LineweightFromLayer(l.Lineweight); w != doc.LineweightDefault {
				lw = w
			}
		}
		out = append(out, Option{Value: IntValue(int(doc.LineweightByLayer)), Label: lineweightLabel(doc.LineweightByLayer), Sample: Sample{Lineweight: lw}},
			Option{Value: IntValue(int(doc.LineweightByBlock)), Label: lineweightLabel(doc.LineweightByBlock), Sample: Sample{Lineweight: def}})
	}
	out = append(out, Option{Value: IntValue(int(doc.LineweightDefault)), Label: lineweightLabel(doc.LineweightDefault), Sample: Sample{Lineweight: def}})
	for _, h := range doc.StandardLineweights {
		w := doc.Lineweight(h)
		if h == 0 {
			if !zero {
				continue
			}
			w = doc.LineweightZero
		}
		out = append(out, Option{Value: IntValue(int(w)), Label: lineweightLabel(w), Sample: Sample{Lineweight: w}})
	}
	return out
}

// widthOptions lists the standard widths, 0.00 mm included, without ByLayer, ByBlock and Default:
// the choices of LWDEFAULT.
func widthOptions(c *env) []Option {
	var out []Option
	for _, o := range lineweightOptions(c, nil, false, true) {
		if doc.Lineweight(o.Value.Int) != doc.LineweightDefault {
			out = append(out, o)
		}
	}
	return out
}

// textStyleOptions lists the text styles the drawing knows.
func textStyleOptions(c *env) []Option {
	var out []Option
	for _, n := range doc.TextStyles(c.d) {
		out = append(out, Option{Value: StrValue(n), Label: n})
	}
	return out
}

// dimStyleOptions lists the dimension style table.
func dimStyleOptions(c *env) []Option {
	var out []Option
	for _, s := range c.d.DimStyles() {
		out = append(out, Option{Value: StrValue(s.Name), Label: s.Name})
	}
	return out
}

// blockOptions lists the drawing's block definitions.
func blockOptions(c *env) []Option {
	var out []Option
	for _, b := range c.d.Blocks() {
		out = append(out, Option{Value: StrValue(b.Name), Label: b.Name})
	}
	slices.SortFunc(out, func(a, b Option) int { return strings.Compare(strings.ToUpper(a.Label), strings.ToUpper(b.Label)) })
	return out
}

// arrowOptions lists AutoCAD's arrowheads in its order, each with its sample.
func arrowOptions(_ *env) []Option {
	var out []Option
	for k := doc.ArrowClosedFilled; k < doc.ArrowUser; k++ {
		b := k.BlockName()
		out = append(out, Option{Value: StrValue(b), Label: arrowLabel(b), Sample: Sample{Arrow: b, HasArrow: true}})
	}
	return out
}

// arrowLabel names a DIMBLK value: AutoCAD's arrow name, or the block's own name.
func arrowLabel(b string) string {
	k := doc.ArrowKindOf(b)
	if k == doc.ArrowUser {
		return b
	}
	slug := strings.ToLower(strings.TrimPrefix(k.BlockName(), "_"))
	if slug == "" {
		slug = "closedfilled"
	}
	return i18n.T("ui.props.arrow." + slug)
}

// enumOptions builds the options of an enumeration from catalog keys, valued 0, 1, 2...
func enumOptions(prefix string, keys ...string) []Option {
	out := make([]Option, len(keys))
	for i, k := range keys {
		out[i] = Option{Value: IntValue(i), Label: i18n.T(prefix + k)}
	}
	return out
}

// --- entity comparison ----------------------------------------------------

// sameEntity reports whether a and b hold the same document state: every exported field is equal
// (caches and the drawing back-pointer are ignored), as ui's text editor compares a draft.
func sameEntity(a, b doc.Entity) bool {
	if a == nil || b == nil || reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	x, y := reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem()
	for i := 0; i < x.NumField(); i++ {
		if x.Type().Field(i).IsExported() && !reflect.DeepEqual(x.Field(i).Interface(), y.Field(i).Interface()) {
			return false
		}
	}
	return true
}
