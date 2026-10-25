package cmdsys

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"cad-lite/doc"
)

// This file registers the drawing-backed system variables: the current properties (CLAYER,
// CECOLOR, CELTYPE, CELTSCALE, CELWEIGHT), the global LTSCALE and LWDEFAULT, INSUNITS, DIMSTYLE and
// every DIM* variable the DIMSTYLE record models. They read the drawing (doc.Drawing.Current, ActiveDimStyle) and
// write it through a transaction (Txn.SetCurrent, Txn.SetDimStyle), so SETVAR, the property
// commands, the Properties panel and scripts edit one value and every change is undoable.
func init() { RegisterVarGroup(propVars) }

// varTxn runs fn, a drawing-backed variable's write, in the transaction that should hold it: the
// running command's (opened on demand; the one a transparent command interrupted when there is
// one), so the change is part of the command's undo step; or, with no command running, a
// transaction of its own committed at once as an undo step named after the variable. It fails
// with ErrVarBusy when the drawing is held by a transaction it cannot reach.
func (s *Session) varTxn(name string, fn func(*doc.Txn) error) error {
	if s.active != nil {
		tx, err := s.commandTxn()
		if err != nil {
			return err
		}
		return fn(tx)
	}
	tx, err := s.ctx.Doc.Begin()
	if err != nil {
		return ErrVarBusy
	}
	if err := fn(tx); err != nil {
		tx.Abort()
		return err
	}
	if cs := tx.Commit(); !cs.Empty() {
		s.ctx.Doc.NameLastStep(name)
	}
	return nil
}

// commandTxn returns the running command's transaction, or the one of the command a transparent
// command interrupted, opening the running command's own when neither exists (Context.TableTx).
func (s *Session) commandTxn() (*doc.Txn, error) {
	tx := s.ctx.txn
	for i := len(s.cl.frames) - 1; tx == nil && i >= 0; i-- {
		tx = s.cl.frames[i].txn
	}
	if tx == nil {
		t, err := s.ctx.Doc.Begin()
		if err != nil {
			return nil, ErrVarBusy
		}
		s.ctx.txn, tx = t, t
	}
	return tx, nil
}

// setCurrent changes the drawing's current properties by fn inside tx. A current value the
// table no longer backs (a file naming a missing linetype or style) is reset first, so it cannot
// block an unrelated change.
func setCurrent(tx *doc.Txn, d *doc.Drawing, fn func(*doc.CurrentProps)) error {
	p := d.Current()
	if d.Layer(p.Layer) == nil {
		p.Layer = "0"
	}
	if p.Color < 0 || p.Color > doc.ByLayer {
		p.Color = doc.ByLayer
	}
	if !doc.IsByLayerLinetype(p.Linetype) && !doc.IsByBlockLinetype(p.Linetype) {
		if _, ok := d.Linetype(p.Linetype); !ok {
			p.Linetype = ""
		}
	}
	if !(p.LinetypeScale >= 0) || math.IsInf(p.LinetypeScale, 0) {
		p.LinetypeScale = 0
	}
	if !(p.LTScale >= 0) || math.IsInf(p.LTScale, 0) {
		p.LTScale = 0
	}
	if _, ok := d.DimStyle(p.DimStyle); !ok {
		if st := d.DimStyles(); len(st) > 0 {
			p.DimStyle = st[0].Name
		}
	}
	fn(&p)
	if err := tx.SetCurrent(p); err != nil {
		return errors.Join(ErrVarInvalid, err)
	}
	return nil
}

func propVars(s *Session, v *Vars) {
	d := func() *doc.Drawing { return s.ctx.Doc }
	v.Register(Var{Name: "CLAYER", Kind: VarText, Default: "0",
		Read: func() any { return d().CurrentLayer },
		Write: func(tx *doc.Txn, n any) error {
			l := d().Layer(strings.TrimSpace(n.(string)))
			if l == nil || l.Frozen {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.Layer = l.Name })
		}})
	v.Register(Var{Name: "CECOLOR", Kind: VarText, Default: "BYLAYER",
		Read: func() any { return ColorText(d().CurrentColor) },
		Write: func(tx *doc.Txn, n any) error {
			c, ok := ParseColor(n.(string))
			if !ok {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.Color = c })
		}})
	v.Register(Var{Name: "CELTYPE", Kind: VarText, Default: doc.LinetypeByLayer,
		Read: func() any { return LinetypeText(d().CurrentLinetype) },
		Write: func(tx *doc.Txn, n any) error {
			name, ok := CanonicalLinetype(d(), n.(string))
			if !ok {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.Linetype = name })
		}})
	v.Register(Var{Name: "CELTSCALE", Kind: VarReal, Default: 1.0,
		Read: func() any { return doc.LinetypeScaleOf(d().CurrentLinetypeScale) },
		Write: func(tx *doc.Txn, n any) error {
			f := n.(float64)
			if !(f > 0) {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.LinetypeScale = f })
		}})
	v.Register(Var{Name: "CELWEIGHT", Kind: VarInt, Default: -1, Min: -3, Max: 211,
		Read: func() any { return d().CurrentLineweight.DXF() },
		Write: func(tx *doc.Txn, n any) error {
			w, ok := LineweightFromVar(n.(int))
			if !ok {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.Lineweight = w })
		}})
	v.Register(Var{Name: "LTSCALE", Kind: VarReal, Default: 1.0,
		Read: func() any { return d().LTScaleValue() },
		Write: func(tx *doc.Txn, n any) error {
			f := n.(float64)
			if !(f > 0) {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.LTScale = f })
		}})
	// LWDEFAULT is a per-user registry value in AutoCAD; here it is Drawing.LWDefault, which no
	// file stores, so it is undoable like the other current properties but not saved.
	v.Register(Var{Name: "LWDEFAULT", Kind: VarInt, Default: doc.DefaultLineweightHundredths, Min: 0, Max: 211,
		Read: func() any { return d().LWDefaultValue().DXF() },
		Write: func(tx *doc.Txn, n any) error {
			w, ok := LineweightFromVar(n.(int))
			if !ok || n.(int) < 0 {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.LWDefault = w })
		}})
	v.Register(Var{Name: "DIMSTYLE", Kind: VarText, Default: doc.DefaultDimStyle().Name,
		Read: func() any { return d().ActiveDimStyle().Name },
		Write: func(tx *doc.Txn, n any) error {
			st, ok := d().DimStyle(strings.TrimSpace(n.(string)))
			if !ok {
				return ErrVarInvalid
			}
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.DimStyle = st.Name })
		}})
	v.Register(Var{Name: "INSUNITS", Kind: VarInt, Default: 4, Min: 0, Max: 24,
		Read: func() any { return d().Header.InsUnits },
		Write: func(tx *doc.Txn, n any) error {
			return setCurrent(tx, d(), func(p *doc.CurrentProps) { p.InsUnits = n.(int) })
		}})
	for _, sp := range dimVarSpecs {
		v.Register(Var{Name: sp.Name, Kind: sp.Kind, Default: sp.Get(doc.DefaultDimStyle()), Min: sp.Min, Max: sp.Max,
			Read: func() any { return sp.Get(d().ActiveDimStyle()) },
			Write: func(tx *doc.Txn, n any) error {
				st := d().ActiveDimStyle()
				if err := sp.Set(d(), &st, n); err != nil {
					return err
				}
				return tx.SetDimStyle(st)
			}})
	}
}

// --- colours, linetypes and lineweights as typed ------------------------

var colorNames = map[string]int{
	"RED": 1, "YELLOW": 2, "GREEN": 3, "CYAN": 4, "BLUE": 5, "MAGENTA": 6, "WHITE": 7,
	"ROSSO": 1, "GIALLO": 2, "VERDE": 3, "CIANO": 4, "BLU": 5, "BIANCO": 7,
}

// ParseColor reads a colour answer: an AutoCAD Color Index 1-255, a name (red, yellow, green,
// cyan, blue, magenta, white, or their Italian forms), BYLAYER (doc.ByLayer) or BYBLOCK
// (doc.ByBlock), also in Italian (DALAYER, DABLOCCO).
func ParseColor(text string) (int, bool) {
	t := strings.ToUpper(strings.TrimSpace(text))
	switch t {
	case "BYLAYER", "DALAYER", "DA LAYER":
		return doc.ByLayer, true
	case "BYBLOCK", "DABLOCCO", "DA BLOCCO":
		return doc.ByBlock, true
	}
	if n, ok := colorNames[t]; ok {
		return n, true
	}
	n, err := strconv.Atoi(t)
	if err != nil || n < 1 || n > 255 {
		return 0, false
	}
	return n, true
}

// ColorText renders a colour as the CECOLOR value: BYBLOCK (0), BYLAYER, or the number.
func ColorText(c int) string {
	if c == doc.ByBlock {
		return "BYBLOCK"
	}
	if c < 1 || c > 255 {
		return "BYLAYER"
	}
	return strconv.Itoa(c)
}

// LinetypeText renders an entity or current linetype as the CELTYPE value: BYLAYER, BYBLOCK or the
// name.
func LinetypeText(name string) string {
	switch {
	case doc.IsByLayerLinetype(name):
		return doc.LinetypeByLayer
	case doc.IsByBlockLinetype(name):
		return doc.LinetypeByBlock
	}
	return name
}

// CanonicalLinetype returns the value an entity or CELTYPE stores for a typed linetype: "" for
// BYLAYER (also DALAYER), doc.LinetypeByBlock for BYBLOCK (DABLOCCO), or the drawing's spelling
// of a loaded linetype; false when the drawing has no such linetype.
func CanonicalLinetype(d *doc.Drawing, text string) (string, bool) {
	t := strings.TrimSpace(text)
	switch strings.ToUpper(t) {
	case "", doc.LinetypeByLayer, "DALAYER":
		return "", true
	case doc.LinetypeByBlock, "DABLOCCO":
		return doc.LinetypeByBlock, true
	}
	if l, ok := d.Linetype(t); ok {
		return l.Name, true
	}
	return "", false
}

// LineweightFromVar converts a CELWEIGHT value (-1 ByLayer, -2 ByBlock, -3 Default, else a
// standard width in hundredths of a millimetre) and reports whether it is one.
func LineweightFromVar(v int) (doc.Lineweight, bool) {
	if v < -3 || v >= 0 && !slices.Contains(doc.StandardLineweights, v) {
		return 0, false
	}
	return doc.LineweightFromDXF(v), true
}

// ParseLineweight reads a typed lineweight: BYLAYER, BYBLOCK, DEFAULT (or their Italian forms
// DALAYER, DABLOCCO, PREDEFINITO) or a standard width in millimetres ("0.25", "0,25", "0.25mm");
// 0 is AutoCAD's 0.00 mm (doc.LineweightZero).
func ParseLineweight(text string) (doc.Lineweight, bool) {
	t := strings.ToUpper(strings.TrimSpace(text))
	switch t {
	case "BYLAYER", "DALAYER", "DA LAYER":
		return doc.LineweightByLayer, true
	case "BYBLOCK", "DABLOCCO", "DA BLOCCO":
		return doc.LineweightByBlock, true
	case "DEFAULT", "PREDEFINITO":
		return doc.LineweightDefault, true
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "MM"))
	v, err := strconv.ParseFloat(strings.ReplaceAll(t, ",", "."), 64)
	if err != nil || math.IsNaN(v) || v < 0 {
		return 0, false
	}
	w := int(math.Round(v * 100))
	if !slices.Contains(doc.StandardLineweights, w) {
		return 0, false
	}
	if w == 0 {
		return doc.LineweightZero, true
	}
	return doc.Lineweight(w), true
}

// LineweightText renders a lineweight as the command line shows it: BYLAYER, BYBLOCK, DEFAULT or
// the width in millimetres with two decimals ("0.25").
func LineweightText(w doc.Lineweight) string {
	switch w {
	case doc.LineweightByLayer:
		return "BYLAYER"
	case doc.LineweightByBlock:
		return "BYBLOCK"
	case doc.LineweightDefault:
		return "DEFAULT"
	}
	h, _ := w.Hundredths()
	return strconv.FormatFloat(float64(h)/100, 'f', 2, 64)
}

// --- the DIM* variables -------------------------------------------------

// DimVarSpec describes one DIM* system variable, a field of the current DIMSTYLE record: the
// variable reads ActiveDimStyle and writes the record (Txn.SetDimStyle), so a change applies to
// every dimension that follows the style and does not override it.
type DimVarSpec struct {
	// Name is the variable (DIMASZ); Var the doc.DimVar bit of the field.
	Name string
	Var  doc.DimVar
	// Kind is VarReal, VarInt or VarText; Min and Max bound an Int when Max > Min.
	Kind     VarKind
	Min, Max float64
	// Get returns the field of s as the variable's value (float64, int or string).
	Get func(s doc.DimStyle) any
	set func(d *doc.Drawing, s *doc.DimStyle, v any) error
}

// Set stores the value v (of the spec's Kind) into s, validated against d: a text style, an arrow
// block or a separator the drawing cannot use is ErrVarInvalid.
func (sp DimVarSpec) Set(d *doc.Drawing, s *doc.DimStyle, v any) error {
	val, err := coerce(sp.Kind, v)
	if err != nil {
		return err
	}
	return sp.set(d, s, val)
}

// DimVarSpecs returns the DIM* variables of the DIMSTYLE record, in AutoCAD's dialog order.
func DimVarSpecs() []DimVarSpec { return slices.Clone(dimVarSpecs) }

// DimVarSpecOf returns the spec of the named variable (any case).
func DimVarSpecOf(name string) (DimVarSpec, bool) {
	for _, sp := range dimVarSpecs {
		if strings.EqualFold(sp.Name, name) {
			return sp, true
		}
	}
	return DimVarSpec{}, false
}

// realSpec is a non-negative length or factor.
func realSpec(name string, v doc.DimVar, get func(doc.DimStyle) float64, set func(*doc.DimStyle, float64)) DimVarSpec {
	return DimVarSpec{Name: name, Var: v, Kind: VarReal, Get: func(s doc.DimStyle) any { return get(s) },
		set: func(_ *doc.Drawing, s *doc.DimStyle, x any) error {
			f := x.(float64)
			if f < 0 {
				return ErrVarInvalid
			}
			set(s, f)
			return nil
		}}
}

func intSpec(name string, v doc.DimVar, lo, hi int, get func(doc.DimStyle) int, set func(*doc.DimStyle, int)) DimVarSpec {
	return DimVarSpec{Name: name, Var: v, Kind: VarInt, Min: float64(lo), Max: float64(hi),
		Get: func(s doc.DimStyle) any { return get(s) },
		set: func(_ *doc.Drawing, s *doc.DimStyle, x any) error {
			n := x.(int)
			if n < lo || n > hi {
				return ErrVarRange
			}
			set(s, n)
			return nil
		}}
}

func boolSpec(name string, v doc.DimVar, get func(doc.DimStyle) bool, set func(*doc.DimStyle, bool)) DimVarSpec {
	return intSpec(name, v, 0, 1, func(s doc.DimStyle) int {
		if get(s) {
			return 1
		}
		return 0
	}, func(s *doc.DimStyle, n int) { set(s, n != 0) })
}

// colorSpec is DIMCLRD, DIMCLRE or DIMCLRT: 0 ByBlock, 1–255, 256 ByLayer.
func colorSpec(name string, v doc.DimVar, get func(doc.DimStyle) int, set func(*doc.DimStyle, int)) DimVarSpec {
	return intSpec(name, v, 0, 256, get, set)
}

// arrowSpec is DIMBLK, DIMBLK1 or DIMBLK2: an AutoCAD arrow name (with or without the
// underscore; "." or "" is closed filled) or a block of the drawing.
func arrowSpec(name string, v doc.DimVar, get func(doc.DimStyle) string, set func(*doc.DimStyle, string)) DimVarSpec {
	return DimVarSpec{Name: name, Var: v, Kind: VarText, Get: func(s doc.DimStyle) any { return get(s) },
		set: func(d *doc.Drawing, s *doc.DimStyle, x any) error {
			b, ok := CanonicalArrow(d, x.(string))
			if !ok {
				return ErrVarInvalid
			}
			set(s, b)
			return nil
		}}
}

// CanonicalArrow returns the DIMBLK value for a typed arrowhead: an AutoCAD arrow name in the
// form ArrowKind.BlockName gives ("" for closed filled, also "." and "_ClosedFilled"), or the
// drawing's spelling of one of its own blocks; false for anything else.
func CanonicalArrow(d *doc.Drawing, text string) (string, bool) {
	t := strings.TrimSpace(text)
	if t == "." {
		return "", true
	}
	if k := doc.ArrowKindOf(t); k != doc.ArrowUser {
		return k.BlockName(), true
	}
	if d != nil {
		if b := d.Block(t); b != nil {
			return b.Name, true
		}
	}
	return "", false
}

var dimVarSpecs = []DimVarSpec{
	// Lines and arrows.
	colorSpec("DIMCLRD", doc.DimVarLineColor, func(s doc.DimStyle) int { return s.LineColor }, func(s *doc.DimStyle, n int) { s.LineColor = n }),
	colorSpec("DIMCLRE", doc.DimVarExtColor, func(s doc.DimStyle) int { return s.ExtColor }, func(s *doc.DimStyle, n int) { s.ExtColor = n }),
	realSpec("DIMEXE", doc.DimVarExtExtend, func(s doc.DimStyle) float64 { return s.ExtExtend }, func(s *doc.DimStyle, f float64) { s.ExtExtend = f }),
	realSpec("DIMEXO", doc.DimVarExtOffset, func(s doc.DimStyle) float64 { return s.ExtOffset }, func(s *doc.DimStyle, f float64) { s.ExtOffset = f }),
	arrowSpec("DIMBLK", doc.DimVarArrowBlock, func(s doc.DimStyle) string { return s.ArrowBlock }, func(s *doc.DimStyle, b string) { s.ArrowBlock = b }),
	boolSpec("DIMSAH", doc.DimVarSeparateArrows, func(s doc.DimStyle) bool { return s.SeparateArrows }, func(s *doc.DimStyle, b bool) { s.SeparateArrows = b }),
	arrowSpec("DIMBLK1", doc.DimVarArrowBlock1, func(s doc.DimStyle) string { return s.ArrowBlock1 }, func(s *doc.DimStyle, b string) { s.ArrowBlock1 = b }),
	arrowSpec("DIMBLK2", doc.DimVarArrowBlock2, func(s doc.DimStyle) string { return s.ArrowBlock2 }, func(s *doc.DimStyle, b string) { s.ArrowBlock2 = b }),
	realSpec("DIMASZ", doc.DimVarArrowSize, func(s doc.DimStyle) float64 { return s.ArrowSize }, func(s *doc.DimStyle, f float64) { s.ArrowSize = f }),
	// Text.
	{Name: "DIMTXSTY", Var: doc.DimVarTextStyle, Kind: VarText,
		Get: func(s doc.DimStyle) any {
			if s.TextStyle == "" {
				return doc.DefaultTextStyle
			}
			return s.TextStyle
		},
		set: func(d *doc.Drawing, s *doc.DimStyle, x any) error {
			n, ok := doc.FindTextStyle(d, x.(string))
			if !ok {
				return ErrVarInvalid
			}
			s.TextStyle = n
			return nil
		}},
	colorSpec("DIMCLRT", doc.DimVarTextColor, func(s doc.DimStyle) int { return s.TextColor }, func(s *doc.DimStyle, n int) { s.TextColor = n }),
	realSpec("DIMTXT", doc.DimVarTextHeight, func(s doc.DimStyle) float64 { return s.TextHeight }, func(s *doc.DimStyle, f float64) { s.TextHeight = f }),
	intSpec("DIMTAD", doc.DimVarTextVertical, 0, 4, func(s doc.DimStyle) int { return s.TextVertical }, func(s *doc.DimStyle, n int) { s.TextVertical = n }),
	intSpec("DIMJUST", doc.DimVarTextJust, 0, 4, func(s doc.DimStyle) int { return s.TextJust }, func(s *doc.DimStyle, n int) { s.TextJust = n }),
	// A negative DIMGAP boxes the text in AutoCAD; the layout does not draw the box, so it is
	// refused here (a file's negative value is kept and draws with no gap).
	realSpec("DIMGAP", doc.DimVarGap, func(s doc.DimStyle) float64 { return s.Gap }, func(s *doc.DimStyle, f float64) { s.Gap = f }),
	boolSpec("DIMTIH", doc.DimVarTextInsideHorizontal, func(s doc.DimStyle) bool { return s.TextInsideHorizontal }, func(s *doc.DimStyle, b bool) { s.TextInsideHorizontal = b }),
	boolSpec("DIMTOH", doc.DimVarTextOutsideHorizontal, func(s doc.DimStyle) bool { return s.TextOutsideHorizontal }, func(s *doc.DimStyle, b bool) { s.TextOutsideHorizontal = b }),
	// Fit.
	intSpec("DIMATFIT", doc.DimVarFit, 0, 3, func(s doc.DimStyle) int { return s.Fit }, func(s *doc.DimStyle, n int) { s.Fit = n }),
	boolSpec("DIMTIX", doc.DimVarTextInside, func(s doc.DimStyle) bool { return s.TextInside }, func(s *doc.DimStyle, b bool) { s.TextInside = b }),
	// DIMSCALE 0 is AutoCAD's scale computed from the paper space viewport; with no paper space
	// it is 1, as the layout reads it (doc.DimStyle.size).
	realSpec("DIMSCALE", doc.DimVarScale, func(s doc.DimStyle) float64 { return s.Scale }, func(s *doc.DimStyle, f float64) { s.Scale = f }),
	boolSpec("DIMTOFL", doc.DimVarLineInside, func(s doc.DimStyle) bool { return s.LineInside }, func(s *doc.DimStyle, b bool) { s.LineInside = b }),
	// Primary units.
	intSpec("DIMDEC", doc.DimVarDecimals, 0, 8, func(s doc.DimStyle) int { return s.Decimals }, func(s *doc.DimStyle, n int) { s.Decimals = n }),
	{Name: "DIMDSEP", Var: doc.DimVarDecSep, Kind: VarText,
		Get: func(s doc.DimStyle) any {
			if s.DecSep == "" {
				return "."
			}
			return s.DecSep
		},
		set: func(_ *doc.Drawing, s *doc.DimStyle, x any) error {
			t := x.(string)
			r, n := utf8.DecodeRuneInString(t)
			switch {
			case t == "":
				s.DecSep = "."
			case n == len(t) && r != utf8.RuneError:
				s.DecSep = t
			default:
				return ErrVarInvalid
			}
			return nil
		}},
	intSpec("DIMZIN", doc.DimVarZeroSupp, 0, 15, func(s doc.DimStyle) int { return s.ZeroSupp }, func(s *doc.DimStyle, n int) { s.ZeroSupp = n }),
}
