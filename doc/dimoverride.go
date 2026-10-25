package doc

// A dimension does not own a copy of its style: it names a DIMSTYLE record (Dimension.StyleName)
// and keeps only the variables it overrides (Dimension.Overrides), as AutoCAD's DSTYLE overrides
// do. Its effective style (Dimension.Style) is the record with the overrides applied, so editing
// the record (Txn.SetDimStyle) changes every dimension that does not override what changed.

// DimVar is a set of dimension variables, one bit each: which fields of a DimStyle a dimension
// overrides.
type DimVar uint32

// The dimension variables, one per DimStyle field except Name.
const (
	DimVarScale                 DimVar = 1 << iota // DIMSCALE
	DimVarArrowSize                                // DIMASZ
	DimVarTextHeight                               // DIMTXT
	DimVarExtOffset                                // DIMEXO
	DimVarExtExtend                                // DIMEXE
	DimVarGap                                      // DIMGAP
	DimVarDecimals                                 // DIMDEC
	DimVarDecSep                                   // DIMDSEP
	DimVarZeroSupp                                 // DIMZIN
	DimVarTextVertical                             // DIMTAD
	DimVarTextJust                                 // DIMJUST
	DimVarTextInsideHorizontal                     // DIMTIH
	DimVarTextOutsideHorizontal                    // DIMTOH
	DimVarTextStyle                                // DIMTXSTY
	DimVarArrowBlock                               // DIMBLK
	DimVarSeparateArrows                           // DIMSAH
	DimVarArrowBlock1                              // DIMBLK1
	DimVarArrowBlock2                              // DIMBLK2
	DimVarLineColor                                // DIMCLRD
	DimVarExtColor                                 // DIMCLRE
	DimVarTextColor                                // DIMCLRT
	DimVarFit                                      // DIMATFIT
	DimVarTextInside                               // DIMTIX
	DimVarLineInside                               // DIMTOFL
	dimVarEnd
)

// DimVarAll is every dimension variable.
const DimVarAll = dimVarEnd - 1

// copyDimVar copies the field of variable v from src to dst.
func copyDimVar(v DimVar, dst *DimStyle, src DimStyle) {
	switch v {
	case DimVarScale:
		dst.Scale = src.Scale
	case DimVarArrowSize:
		dst.ArrowSize = src.ArrowSize
	case DimVarTextHeight:
		dst.TextHeight = src.TextHeight
	case DimVarExtOffset:
		dst.ExtOffset = src.ExtOffset
	case DimVarExtExtend:
		dst.ExtExtend = src.ExtExtend
	case DimVarGap:
		dst.Gap = src.Gap
	case DimVarDecimals:
		dst.Decimals = src.Decimals
	case DimVarDecSep:
		dst.DecSep = src.DecSep
	case DimVarZeroSupp:
		dst.ZeroSupp = src.ZeroSupp
	case DimVarTextVertical:
		dst.TextVertical = src.TextVertical
	case DimVarTextJust:
		dst.TextJust = src.TextJust
	case DimVarTextInsideHorizontal:
		dst.TextInsideHorizontal = src.TextInsideHorizontal
	case DimVarTextOutsideHorizontal:
		dst.TextOutsideHorizontal = src.TextOutsideHorizontal
	case DimVarTextStyle:
		dst.TextStyle = src.TextStyle
	case DimVarArrowBlock:
		dst.ArrowBlock = src.ArrowBlock
	case DimVarSeparateArrows:
		dst.SeparateArrows = src.SeparateArrows
	case DimVarArrowBlock1:
		dst.ArrowBlock1 = src.ArrowBlock1
	case DimVarArrowBlock2:
		dst.ArrowBlock2 = src.ArrowBlock2
	case DimVarLineColor:
		dst.LineColor = src.LineColor
	case DimVarExtColor:
		dst.ExtColor = src.ExtColor
	case DimVarTextColor:
		dst.TextColor = src.TextColor
	case DimVarFit:
		dst.Fit = src.Fit
	case DimVarTextInside:
		dst.TextInside = src.TextInside
	case DimVarLineInside:
		dst.LineInside = src.LineInside
	}
}

// DimStyleDiff returns the variables whose values differ between a and b (Name aside).
func DimStyleDiff(a, b DimStyle) DimVar {
	var out DimVar
	for v := DimVar(1); v < dimVarEnd; v <<= 1 {
		t := a
		copyDimVar(v, &t, b)
		if t != a {
			out |= v
		}
	}
	return out
}

// DimOverrides are a dimension's own values for some dimension variables: Set says which, and
// Values holds them (its other fields, and Name, mean nothing). The zero value overrides nothing.
type DimOverrides struct {
	Set    DimVar
	Values DimStyle
}

// Apply returns s with the overridden variables replaced; the name stays s's.
func (o DimOverrides) Apply(s DimStyle) DimStyle {
	for v := DimVar(1); v < dimVarEnd; v <<= 1 {
		if o.Set&v != 0 {
			copyDimVar(v, &s, o.Values)
		}
	}
	return s
}

// With returns o overriding the variables vars with their values in s too.
func (o DimOverrides) With(vars DimVar, s DimStyle) DimOverrides {
	for v := DimVar(1); v < dimVarEnd; v <<= 1 {
		if vars&v != 0 {
			copyDimVar(v, &o.Values, s)
		}
	}
	o.Set |= vars & DimVarAll
	return o
}

// Without returns o no longer overriding vars: they inherit from the style again.
func (o DimOverrides) Without(vars DimVar) DimOverrides {
	var zero DimStyle
	for v := DimVar(1); v < dimVarEnd; v <<= 1 {
		if vars&v != 0 {
			copyDimVar(v, &o.Values, zero)
		}
	}
	o.Set &^= vars
	return o
}

// DimStyleBase returns the record a dimension naming name inherits from: the table's record, or
// DefaultDimStyle under that name when the table lacks it ("" is ISO-25).
func (d *Drawing) DimStyleBase(name string) DimStyle {
	if d != nil {
		if s, ok := d.DimStyle(name); ok {
			return s
		}
	}
	s := DefaultDimStyle()
	if name != "" {
		s.Name = name
	}
	return s
}

// Style returns the dimension's effective style: its DIMSTYLE record (DimStyleBase of StyleName in
// the drawing that holds it, ISO-25 values for a dimension not in a drawing) with Overrides
// applied. Its Name is StyleName.
func (e *Dimension) Style() DimStyle {
	return e.Overrides.Apply(e.d.DimStyleBase(e.StyleName))
}

// SetStyle makes the dimension draw with s: StyleName becomes s.Name and Overrides the variables
// where s differs from the record it names in d (the drawing that holds or will hold the
// dimension; nil for the drawing that holds it, or ISO-25 when none does). A value equal to the
// record's is not an override, so it follows later edits of the record.
func (e *Dimension) SetStyle(d *Drawing, s DimStyle) {
	if d == nil {
		d = e.d
	}
	base := d.DimStyleBase(s.Name)
	e.StyleName = base.Name
	e.Overrides = DimOverrides{}.With(DimStyleDiff(base, s), s)
}
