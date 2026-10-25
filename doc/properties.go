package doc

import (
	"math"
	"strings"
)

// This file holds the display properties every entity carries besides its layer: colour (an ACI
// in EntityCommon.Color), linetype, linetype scale and lineweight, and the rules that resolve
// them through the layer and an enclosing block reference, as AutoCAD does.

// Linetype names with a meaning of their own (DXF group 6, $CELTYPE). Names compare without
// regard to case.
const (
	// LinetypeByLayer is the linetype of the entity's layer. EntityCommon.Linetype "" means it too.
	LinetypeByLayer = "BYLAYER"
	// LinetypeByBlock is the linetype of the enclosing block reference (CONTINUOUS outside one).
	LinetypeByBlock = "BYBLOCK"
	// LinetypeContinuous is the solid line, always present in a drawing's linetype table.
	LinetypeContinuous = "CONTINUOUS"
)

// IsByLayerLinetype reports whether an entity linetype name means ByLayer: "" or BYLAYER (any case).
func IsByLayerLinetype(name string) bool {
	return name == "" || strings.EqualFold(name, LinetypeByLayer)
}

// IsByBlockLinetype reports whether an entity linetype name is BYBLOCK (any case).
func IsByBlockLinetype(name string) bool { return strings.EqualFold(name, LinetypeByBlock) }

// Lineweight is an entity lineweight. Its zero value is ByLayer; positive values are hundredths
// of a millimetre (AutoCAD's standard values are StandardLineweights). DXF group 370 and
// $CELWEIGHT store it as AutoCAD does (-1 ByLayer, -2 ByBlock, -3 Default, 0 and up hundredths):
// convert with DXF and LineweightFromDXF. AutoCAD's 0.00 mm weight is LineweightZero, because 0
// is ByLayer here.
type Lineweight int

// The lineweights that are not a width.
const (
	// LineweightByLayer takes the layer's lineweight (DXF -1). It is the zero value.
	LineweightByLayer Lineweight = 0
	// LineweightByBlock takes the enclosing block reference's lineweight (DXF -2).
	LineweightByBlock Lineweight = -2
	// LineweightDefault is the default lineweight, LWDEFAULT (DXF -3; DefaultLineweightHundredths).
	LineweightDefault Lineweight = -3
	// LineweightZero is AutoCAD's 0.00 mm lineweight, the thinnest line (DXF 0).
	LineweightZero Lineweight = -4
)

// DefaultLineweightHundredths is AutoCAD's LWDEFAULT, the width LineweightDefault draws with:
// 0.25 mm.
const DefaultLineweightHundredths = 25

// StandardLineweights are AutoCAD's lineweights in hundredths of a millimetre, 0.00 mm first.
var StandardLineweights = []int{0, 5, 9, 13, 15, 18, 20, 25, 30, 35, 40, 50, 53, 60, 70, 80, 90, 100, 106, 120, 140, 158, 200, 211}

// DXF returns w as DXF group 370 and $CELWEIGHT store it.
func (w Lineweight) DXF() int {
	switch {
	case w == LineweightByLayer:
		return -1
	case w == LineweightZero:
		return 0
	}
	return int(w)
}

// LineweightFromDXF converts a DXF group 370 value. A value outside -3 to 211 is ByLayer.
func LineweightFromDXF(v int) Lineweight {
	switch {
	case v == -1, v < -3, v > 211:
		return LineweightByLayer
	case v == 0:
		return LineweightZero
	}
	return Lineweight(v)
}

// LineweightFromLayer converts a Layer.Lineweight (0 the default, int(LineweightZero) 0.00 mm,
// else hundredths) to a Lineweight: LineweightDefault, LineweightZero or the width.
func LineweightFromLayer(v int) Lineweight {
	switch {
	case v == int(LineweightZero):
		return LineweightZero
	case v <= 0 || v > 211:
		return LineweightDefault
	}
	return Lineweight(v)
}

// LayerValue returns w as a Layer.Lineweight stores it: the width, int(LineweightZero) for
// 0.00 mm, and 0 (the default) for anything else; LineweightFromLayer is the inverse.
func (w Lineweight) LayerValue() int {
	switch {
	case w == LineweightZero:
		return int(LineweightZero)
	case w > 0 && w <= 211:
		return int(w)
	}
	return 0
}

// Hundredths returns the width w draws with in hundredths of a millimetre: 0 for LineweightZero,
// DefaultLineweightHundredths for LineweightDefault. ok is false for ByLayer and ByBlock, which
// have no width until resolved.
func (w Lineweight) Hundredths() (hundredths int, ok bool) {
	switch {
	case w == LineweightZero:
		return 0, true
	case w == LineweightDefault:
		return DefaultLineweightHundredths, true
	case w > 0:
		return int(w), true
	}
	return 0, false
}

// LinetypeScaleOf returns an entity linetype scale (DXF 48, CELTSCALE) as it applies: v, or 1
// when v is 0, negative or not finite.
func LinetypeScaleOf(v float64) float64 {
	if !(v > 0) || math.IsInf(v, 0) {
		return 1
	}
	return v
}

// LtScale returns the entity's linetype scale, 1 when LinetypeScale is unset.
func (c *EntityCommon) LtScale() float64 { return LinetypeScaleOf(c.LinetypeScale) }

// Appearance is an entity's resolved display properties: what it draws with after ByLayer and
// ByBlock are resolved.
type Appearance struct {
	// Layer is the effective layer: the entity's own, or the enclosing reference's for an entity
	// on layer "0" inside a block.
	Layer string
	// Color is an ACI from 1 to 255. ByBlock outside a block and a missing layer resolve to 7.
	Color int
	// Linetype is a linetype name (look it up with Drawing.Linetype): never ByLayer or ByBlock;
	// CONTINUOUS when nothing else applies.
	Linetype string
	// LinetypeScale is the pattern scale: the entity's own scale times the drawing's LTSCALE.
	// Inside a block it applies in the block's coordinates, which the reference then transforms.
	LinetypeScale float64
	// Lineweight is a width: hundredths of a millimetre or LineweightZero. Default has resolved
	// to the drawing's LWDEFAULT (Drawing.LWDefaultValue).
	Lineweight Lineweight
}

// Resolve returns the appearance of an entity with common properties c. parent is the resolved
// appearance of the enclosing block reference, or nil in model space. The rules are AutoCAD's:
// each property comes from the nearest level that sets it: the entity, then for ByBlock the
// enclosing reference (parent, itself resolved through its own chain, so nested references work),
// then for ByLayer the effective layer, then the drawing's global defaults. ByLayer takes the
// effective layer's colour, linetype ("" is CONTINUOUS) and lineweight (0 is Default); ByBlock
// takes parent's, and outside a block colour 7, CONTINUOUS and Default; an entity on layer "0"
// inside a block takes the reference's layer. A layer missing from the table gives colour 7,
// CONTINUOUS and Default. Default lineweight resolves to LWDEFAULT, and the linetype scale is the
// entity's times LTSCALE. This is the one place properties resolve: Insert.Parts uses it too.
func (d *Drawing) Resolve(c *EntityCommon, parent *Appearance) Appearance {
	a := Appearance{Layer: c.Layer}
	if parent != nil && (a.Layer == "0" || a.Layer == "") {
		a.Layer = parent.Layer
	}
	l := d.Layer(a.Layer)
	a.Color = d.resolveColor(c.Color, l, parent)
	switch {
	case IsByLayerLinetype(c.Linetype):
		a.Linetype = LinetypeContinuous
		if l != nil && l.Linetype != "" {
			a.Linetype = l.Linetype
		}
	case IsByBlockLinetype(c.Linetype):
		a.Linetype = LinetypeContinuous
		if parent != nil {
			a.Linetype = parent.Linetype
		}
	default:
		a.Linetype = c.Linetype
	}
	a.LinetypeScale = c.LtScale() * d.LTScaleValue()
	switch c.Lineweight {
	case LineweightByLayer:
		a.Lineweight = LineweightDefault
		if l != nil {
			a.Lineweight = LineweightFromLayer(l.Lineweight)
		}
	case LineweightByBlock:
		a.Lineweight = LineweightDefault
		if parent != nil {
			a.Lineweight = parent.Lineweight
		}
	default:
		a.Lineweight = c.Lineweight
		if _, ok := a.Lineweight.Hundredths(); !ok {
			a.Lineweight = LineweightDefault
		}
	}
	if a.Lineweight == LineweightDefault {
		a.Lineweight = d.LWDefaultValue()
	}
	return a
}

// ResolveEntity returns Resolve for e in model space.
func (d *Drawing) ResolveEntity(e Entity) Appearance { return d.Resolve(e.Common(), nil) }

// ResolveColorIn resolves a colour that a part of an entity carries, such as a dimension's
// DIMCLRD, DIMCLRE or DIMCLRT: ByBlock is the entity's own resolved colour (within.Color),
// ByLayer the colour of within's layer, and 1 to 255 themselves.
func (d *Drawing) ResolveColorIn(color int, within Appearance) int {
	switch {
	case color >= 1 && color <= 255:
		return color
	case color == ByLayer:
		if l := d.Layer(within.Layer); l != nil {
			return l.Color
		}
		return 7
	}
	return within.Color
}

func (d *Drawing) resolveColor(color int, l *Layer, parent *Appearance) int {
	switch {
	case color >= 1 && color <= 255:
		return color
	case color == ByBlock:
		if parent != nil {
			return parent.Color
		}
		return 7
	}
	if l != nil && l.Color >= 1 && l.Color <= 255 {
		return l.Color
	}
	return 7
}
