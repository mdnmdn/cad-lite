package props

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
)

// This file holds the dimension rows. Most are dimension variables: on a dimension they read its
// effective style (Dimension.Style) and write an override (DimOverrides), which Value.Clear drops
// so the variable follows the DIMSTYLE record again; on the Defaults target they read and write the
// current DIMSTYLE record through the DIM* system variables, so every dimension following the
// style changes. The rest (text rotation, text override, measurement, arc symbol, style name) are
// fields of the dimension itself.

// styleRow is a row over dimension variables.
type styleRow struct {
	id      string
	kind    Kind
	vars    doc.DimVar
	varName string
	min     float64
	max     float64
	pos     bool
	get     func(s doc.DimStyle) Value
	set     func(d *doc.Drawing, s *doc.DimStyle, v Value) error
	options func(c *env) []Option
}

// varRow is a row over one DIM* variable (cmdsys.DimVarSpec), converting between the Value of
// kind k and the variable's value.
func varRow(id string, k Kind, name string) *styleRow {
	sp, ok := cmdsys.DimVarSpecOf(name)
	if !ok {
		panic("props: no dimension variable " + name)
	}
	r := &styleRow{id: id, kind: k, vars: sp.Var, varName: name}
	if sp.Max > sp.Min {
		r.min, r.max = sp.Min, sp.Max
	}
	r.get = func(s doc.DimStyle) Value {
		switch x := sp.Get(s).(type) {
		case float64:
			return NumValue(x)
		case int:
			if k == KindBool {
				return BoolValue(x != 0)
			}
			return IntValue(x)
		case string:
			return StrValue(x)
		}
		return Value{}
	}
	r.set = func(d *doc.Drawing, s *doc.DimStyle, v Value) error { return sp.Set(d, s, varValue(k, sp.Kind, v)) }
	return r
}

// varValue converts a row Value to a system variable value of kind vk.
func varValue(k Kind, vk cmdsys.VarKind, v Value) any {
	switch vk {
	case cmdsys.VarReal:
		return v.Num
	case cmdsys.VarInt:
		if k == KindBool {
			if v.Bool {
				return 1
			}
			return 0
		}
		return v.Int
	}
	return v.Str
}

func nonNeg(r *styleRow) *styleRow { r.min, r.max = 0, doc.TextMaxSize; return r }

func withOptions(r *styleRow, f func(c *env) []Option) *styleRow { r.options = f; return r }

// arrowRow is Arrow 1 or Arrow 2: setting one end makes the arrows separate (DIMSAH, DIMBLK1,
// DIMBLK2) as AutoCAD's palette does, or joins them again (DIMBLK) when both ends then match.
func arrowRow(id string, end int) *styleRow {
	return &styleRow{id: id, kind: KindArrow, varName: []string{"DIMBLK1", "DIMBLK2"}[end],
		vars:    doc.DimVarArrowBlock | doc.DimVarSeparateArrows | doc.DimVarArrowBlock1 | doc.DimVarArrowBlock2,
		options: arrowOptions,
		get: func(s doc.DimStyle) Value {
			a, b := s.ArrowBlocks()
			return StrValue([]string{a, b}[end])
		},
		set: func(d *doc.Drawing, s *doc.DimStyle, v Value) error {
			name, ok := cmdsys.CanonicalArrow(d, v.Str)
			if !ok {
				return errInvalid
			}
			a, b := s.ArrowBlocks()
			ends := [2]string{a, b}
			ends[end] = name
			if ends[0] == ends[1] {
				s.SeparateArrows, s.ArrowBlock = false, ends[0]
				return nil
			}
			s.SeparateArrows, s.ArrowBlock1, s.ArrowBlock2 = true, ends[0], ends[1]
			return nil
		}}
}

// zeroRow is a DIMZIN bit: 4 suppresses leading zeros, 8 trailing ones.
func zeroRow(id string, bit int) *styleRow {
	return &styleRow{id: id, kind: KindBool, vars: doc.DimVarZeroSupp, varName: "DIMZIN",
		get: func(s doc.DimStyle) Value { return BoolValue(s.ZeroSupp&bit != 0) },
		set: func(_ *doc.Drawing, s *doc.DimStyle, v Value) error {
			if v.Bool {
				s.ZeroSupp |= bit
			} else {
				s.ZeroSupp &^= bit
			}
			return nil
		}}
}

func dimColorOptions(c *env) []Option {
	o := colorOptions(c, c.layerOf(c.first), true)
	if c.first != nil {
		o[1].Sample.ACI = c.d.ResolveEntity(c.first).Color
	}
	return o
}

func decSepOptions(*env) []Option {
	return []Option{
		{Value: StrValue("."), Label: i18n.T("ui.props.dimdsep.period")},
		{Value: StrValue(","), Label: i18n.T("ui.props.dimdsep.comma")},
		{Value: StrValue(" "), Label: i18n.T("ui.props.dimdsep.space")},
	}
}

// dimCategory is one dimension category: style rows, and the dimension's own rows after them.
type dimCategory struct {
	id    string
	style []*styleRow
}

var dimCategories = []dimCategory{
	{id: "dim_lines", style: []*styleRow{
		arrowRow("arrow1", 0), arrowRow("arrow2", 1),
		nonNeg(varRow("dimasz", KindLength, "DIMASZ")),
		withOptions(varRow("dimclrd", KindColor, "DIMCLRD"), dimColorOptions),
		withOptions(varRow("dimclre", KindColor, "DIMCLRE"), dimColorOptions),
		nonNeg(varRow("dimexo", KindLength, "DIMEXO")),
		nonNeg(varRow("dimexe", KindLength, "DIMEXE")),
	}},
	{id: "dim_text", style: []*styleRow{
		withOptions(varRow("dimclrt", KindColor, "DIMCLRT"), dimColorOptions),
		func() *styleRow { r := nonNeg(varRow("dimtxt", KindLength, "DIMTXT")); r.pos = true; return r }(),
		nonNeg(varRow("dimgap", KindLength, "DIMGAP")),
		withOptions(varRow("dimtxsty", KindTextStyle, "DIMTXSTY"), textStyleOptions),
		withOptions(varRow("dimtad", KindEnum, "DIMTAD"), func(*env) []Option {
			return enumOptions("ui.props.dimtad.", "centered", "above", "outside", "jis", "below")
		}),
		withOptions(varRow("dimjust", KindEnum, "DIMJUST"), func(*env) []Option {
			return enumOptions("ui.props.dimjust.", "centered", "ext1", "ext2", "over1", "over2")
		}),
		varRow("dimtih", KindBool, "DIMTIH"),
		varRow("dimtoh", KindBool, "DIMTOH"),
	}},
	{id: "dim_fit", style: []*styleRow{
		nonNeg(varRow("dimscale", KindNumber, "DIMSCALE")),
		withOptions(varRow("dimatfit", KindEnum, "DIMATFIT"), func(*env) []Option {
			return enumOptions("ui.props.dimatfit.", "both", "arrows", "text", "best")
		}),
		varRow("dimtix", KindBool, "DIMTIX"),
		varRow("dimtofl", KindBool, "DIMTOFL"),
	}},
	{id: "dim_units", style: []*styleRow{
		varRow("dimdec", KindInt, "DIMDEC"),
		withOptions(varRow("dimdsep", KindEnum, "DIMDSEP"), decSepOptions),
		zeroRow("zero_leading", 4),
		zeroRow("zero_trailing", 8),
	}},
}

func dim(e doc.Entity) *doc.Dimension { return e.(*doc.Dimension) }

func arcSymbolOptions(*env) []Option {
	return enumOptions("ui.props.arcsym.", "before", "above", "none")
}

// dimOwnRows are the rows of a dimension's own fields, by category.
var dimOwnRows = map[string][]*def{
	"dim_lines": {{id: "arc_symbol", kind: KindEnum, options: arcSymbolOptions,
		only: func(_ *env, e doc.Entity) bool { return dim(e).Kind == doc.DimArcLength },
		get:  func(_ *env, e doc.Entity) Value { return IntValue(dim(e).ArcSymbol) },
		set: func(_ *env, e doc.Entity, v Value) error {
			if v.Int < 0 || v.Int > 2 {
				return errInvalid
			}
			dim(e).ArcSymbol = v.Int
			dim(e).Recompute()
			return nil
		}}},
	"dim_text": {
		{id: "text_rotation", kind: KindAngle,
			get: func(_ *env, e doc.Entity) Value { return NumValue(dim(e).TextAngle) },
			set: func(_ *env, e doc.Entity, v Value) error {
				dim(e).TextAngle = v.Num
				dim(e).Recompute()
				return nil
			}},
		{id: "text_override", kind: KindText,
			get: func(_ *env, e doc.Entity) Value { return StrValue(dim(e).Text) },
			set: func(_ *env, e doc.Entity, v Value) error {
				if !validContent(v.Str, false) {
					return errInvalid
				}
				dim(e).Text = v.Str
				dim(e).Recompute()
				return nil
			},
			clear:     func(_ *env, e doc.Entity) { dim(e).Text = ""; dim(e).Recompute() },
			clearable: func(e doc.Entity) bool { return dim(e).Text != "" }},
		{id: "measurement", kind: KindLength, readOnly: true,
			get: func(_ *env, e doc.Entity) Value { return NumValue(dim(e).Measurement()) }},
	},
	"dim_misc": {{id: "dimstyle", kind: KindDimStyle, options: dimStyleOptions,
		get: func(_ *env, e doc.Entity) Value { return StrValue(dim(e).StyleName) },
		set: func(c *env, e doc.Entity, v Value) error {
			s, ok := c.d.DimStyle(v.Str)
			if !ok {
				return errInvalid
			}
			dim(e).StyleName = s.Name
			dim(e).Recompute()
			return nil
		}}},
}

// entityDef turns a style row into a dimension row that reads the effective style and writes an
// override.
func (r *styleRow) entityDef() *def {
	d := &def{id: r.id, kind: r.kind, positive: r.pos, min: r.min, max: r.max, options: r.options, dimVars: r.vars, varName: r.varName,
		get: func(_ *env, e doc.Entity) Value { return r.get(dim(e).Style()) },
		set: func(c *env, e doc.Entity, v Value) error {
			dm := dim(e)
			st := dm.Style()
			if err := r.set(c.d, &st, v); err != nil {
				return err
			}
			setOverride(c.d, dm, st, r.vars)
			return nil
		},
		clear: func(_ *env, e doc.Entity) {
			dim(e).Overrides = dim(e).Overrides.Without(r.vars)
			dim(e).Recompute()
		},
		clearable: func(e doc.Entity) bool { return dim(e).Overrides.Set&r.vars != 0 },
		source: func(_ *env, e doc.Entity) (Source, string) {
			if dim(e).Overrides.Set&r.vars != 0 {
				return SourceOwn, ""
			}
			return SourceStyle, dim(e).StyleName
		}}
	if r.kind == KindColor {
		d.sample = func(c *env, e doc.Entity) Sample {
			return Sample{ACI: c.d.ResolveColorIn(r.get(dim(e).Style()).Int, c.d.ResolveEntity(e))}
		}
	}
	if r.kind == KindArrow {
		d.sample = func(_ *env, e doc.Entity) Sample { return Sample{Arrow: r.get(dim(e).Style()).Str, HasArrow: true} }
	}
	return d
}

// setOverride makes dm draw with st for the variables vars (and any other st changed): they
// become overrides, except where st equals the DIMSTYLE record, which the dimension then follows
// again (Dimension.SetStyle's rule).
func setOverride(d *doc.Drawing, dm *doc.Dimension, st doc.DimStyle, vars doc.DimVar) {
	vars |= doc.DimStyleDiff(dm.Style(), st)
	base := d.DimStyleBase(dm.StyleName)
	o := dm.Overrides.With(vars, st)
	dm.Overrides = o.Without(vars &^ doc.DimStyleDiff(base, st))
	dm.Recompute()
}

// dimensionCats are the categories of a dimension selection.
func dimensionCats() []catDef {
	var out []catDef
	for _, dc := range dimCategories {
		c := catDef{id: dc.id, title: "ui.props.cat." + dc.id}
		for _, r := range dc.style {
			c.defs = append(c.defs, r.entityDef())
		}
		c.defs = append(c.defs, dimOwnRows[dc.id]...)
		out = append(out, c)
	}
	return append(out, catDef{id: "dim_misc", title: "ui.props.cat.dim_misc", defs: dimOwnRows["dim_misc"]})
}
