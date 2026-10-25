package doc

import (
	"errors"
	"math"
	"slices"
	"strings"
)

// Linetype is a LTYPE table record: a named dash pattern.
type Linetype struct {
	// Name is the record's key; Drawing.Linetype matches it without regard to case.
	Name string
	// Description is the record's text (DXF group 3), by convention a sketch of the pattern.
	Description string
	// Pattern is the dash sequence in drawing units at linetype scale 1, as DXF group 49 and a
	// .lin file store it: a positive length is a dash, a negative one a gap, 0 a dot. Empty is a
	// solid line (CONTINUOUS).
	Pattern []float64
	// Raw is the record as a file held it, kept for a complex linetype (DXF 74 non-zero: shapes or
	// text embedded in the pattern), which the document does not model; writers emit it instead
	// of generating the record. Pattern still holds its dashes and gaps. Nil for a simple one.
	Raw []Pair
}

// Clone returns a deep copy of l.
func (l Linetype) Clone() Linetype {
	l.Pattern = slices.Clone(l.Pattern)
	l.Raw = slices.Clone(l.Raw)
	return l
}

// Length returns the length of one repetition of the pattern (DXF group 40): the sum of the
// absolute element lengths.
func (l Linetype) Length() float64 {
	n := 0.0
	for _, v := range l.Pattern {
		n += math.Abs(v)
	}
	return n
}

// Continuous reports whether the linetype draws solid: no pattern, or one without a gap.
func (l Linetype) Continuous() bool {
	for _, v := range l.Pattern {
		if v < 0 {
			return false
		}
	}
	return true
}

func (l Linetype) equal(o Linetype) bool {
	return l.Name == o.Name && l.Description == o.Description && slices.Equal(l.Pattern, o.Pattern) && slices.Equal(l.Raw, o.Raw)
}

// validate reports why l cannot be a table record: an empty or reserved name (BYLAYER, BYBLOCK),
// characters AutoCAD forbids in symbol names, a non-finite element, a CONTINUOUS with a pattern,
// or a pattern of only gaps.
func (l Linetype) validate() error {
	n := strings.TrimSpace(l.Name)
	if n == "" || n != l.Name || strings.ContainsAny(n, "<>/\\\":;?*|,=`") || IsByLayerLinetype(n) || IsByBlockLinetype(n) {
		return errors.New("doc: invalid linetype name")
	}
	for _, v := range l.Pattern {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("doc: invalid linetype pattern")
		}
	}
	if strings.EqualFold(n, LinetypeContinuous) && len(l.Pattern) > 0 {
		return errors.New("doc: CONTINUOUS has no pattern")
	}
	if len(l.Pattern) > 0 && l.Length() == 0 {
		return errors.New("doc: invalid linetype pattern")
	}
	return nil
}

// continuousLinetype is the record every drawing has.
func continuousLinetype() *Linetype {
	return &Linetype{Name: "CONTINUOUS", Description: "Solid line"}
}

// Linetypes returns copies of the drawing's linetype records in table order, CONTINUOUS first.
// BYLAYER and BYBLOCK are not records.
func (d *Drawing) Linetypes() []Linetype {
	out := make([]Linetype, len(d.linetypes))
	for i, l := range d.linetypes {
		out[i] = l.Clone()
	}
	return out
}

// Linetype returns a copy of the named record (any case) and whether the table has it.
func (d *Drawing) Linetype(name string) (Linetype, bool) {
	if i := d.linetypeIndex(name); i >= 0 {
		return d.linetypes[i].Clone(), true
	}
	return Linetype{}, false
}

// LinetypePattern returns the dash pattern an Appearance.Linetype draws with: the table record's,
// or nil (solid) when the drawing lacks the name. The slice is shared: do not modify it.
func (d *Drawing) LinetypePattern(name string) []float64 {
	if i := d.linetypeIndex(name); i >= 0 {
		return d.linetypes[i].Pattern
	}
	return nil
}

func (d *Drawing) linetypeIndex(name string) int {
	for i, l := range d.linetypes {
		if strings.EqualFold(l.Name, name) {
			return i
		}
	}
	return -1
}

// LoadLinetype adds or replaces a record outside any transaction, for file readers. A record that
// fails validation is ignored, and so is a CONTINUOUS with a pattern.
func (d *Drawing) LoadLinetype(l Linetype) {
	if l.validate() != nil {
		return
	}
	d.putLinetype(l.Clone())
}

func (d *Drawing) putLinetype(l Linetype) {
	if i := d.linetypeIndex(l.Name); i >= 0 {
		*d.linetypes[i] = l
		return
	}
	d.linetypes = append(d.linetypes, &l)
}

// SetLinetype adds the record l to the table, or replaces the record of the same name (any case),
// as one undoable change of this transaction.
func (t *Txn) SetLinetype(l Linetype) error {
	if t.done {
		return errTxnClosed
	}
	if err := l.validate(); err != nil {
		return err
	}
	if i := t.d.linetypeIndex(l.Name); i >= 0 && t.d.linetypes[i].equal(l) {
		return nil
	}
	t.touchSupport()
	t.d.putLinetype(l.Clone())
	return nil
}

// LoadLibraryLinetype adds the named linetype of the built-in library (LibraryLinetype) to the
// table, as LINETYPE Load does. It does nothing when the table already has the name, and fails
// when the library lacks it.
func (t *Txn) LoadLibraryLinetype(name string) error {
	if _, ok := t.d.Linetype(name); ok {
		return nil
	}
	l, ok := LibraryLinetype(name)
	if !ok {
		return errors.New("doc: no such linetype in the library: " + name)
	}
	return t.SetLinetype(l)
}

// RemoveLinetype deletes the named record as one undoable change of this transaction. CONTINUOUS,
// the current linetype and a linetype a layer or an entity (in model space or a block) uses are
// kept, with an error, as AutoCAD's PURGE keeps them.
func (t *Txn) RemoveLinetype(name string) error {
	if t.done {
		return errTxnClosed
	}
	d := t.d
	i := d.linetypeIndex(name)
	switch {
	case i < 0:
		return errors.New("doc: no such linetype: " + name)
	case strings.EqualFold(name, LinetypeContinuous):
		return errors.New("doc: CONTINUOUS cannot be removed")
	case strings.EqualFold(d.CurrentLinetype, name):
		return errors.New("doc: the current linetype cannot be removed")
	}
	for _, l := range d.layers {
		if strings.EqualFold(l.Linetype, name) {
			return errors.New("doc: linetype is in use")
		}
	}
	uses := func(e Entity) bool { return strings.EqualFold(e.Common().Linetype, name) }
	for _, e := range d.entities {
		if uses(e) {
			return errors.New("doc: linetype is in use")
		}
	}
	for _, b := range d.blocks {
		for _, e := range b.Entities {
			if uses(e) {
				return errors.New("doc: linetype is in use")
			}
		}
	}
	t.touchSupport()
	d.linetypes = slices.Delete(d.linetypes, i, i+1)
	return nil
}

var errTxnClosed = errors.New("doc: transaction is closed")

// LibraryLinetype returns a copy of the named linetype (any case) of the built-in library, which
// matches AutoCAD's acadiso.lin: the ISO 128 types ACAD_ISO02W100 to ACAD_ISO15W100 and the
// classic families BORDER, CENTER, DASHDOT, DASHED, DIVIDE, DOT, HIDDEN and PHANTOM with their 2
// (half size) and X2 (double size) variants, all in millimetres. CONTINUOUS is not in it: every
// drawing has it.
func LibraryLinetype(name string) (Linetype, bool) {
	for _, l := range linetypeLibrary {
		if strings.EqualFold(l.Name, name) {
			return l.Clone(), true
		}
	}
	return Linetype{}, false
}

// LibraryLinetypes returns copies of every linetype of the built-in library, in acadiso.lin order.
func LibraryLinetypes() []Linetype {
	out := make([]Linetype, len(linetypeLibrary))
	for i, l := range linetypeLibrary {
		out[i] = l.Clone()
	}
	return out
}

// linetypeLibrary is acadiso.lin's simple linetypes (the complex ones, with shapes and text,
// are left out).
var linetypeLibrary = []Linetype{
	{Name: "BORDER", Description: "Border __ __ . __ __ . __ __ . __ __ . __ __ .", Pattern: []float64{12.7, -6.35, 12.7, -6.35, 0, -6.35}},
	{Name: "BORDER2", Description: "Border (.5x) __.__.__.__.__.__.__.__.__.__.__.", Pattern: []float64{6.35, -3.175, 6.35, -3.175, 0, -3.175}},
	{Name: "BORDERX2", Description: "Border (2x) ____  ____  .  ____  ____  .  ___", Pattern: []float64{25.4, -12.7, 25.4, -12.7, 0, -12.7}},
	{Name: "CENTER", Description: "Center ____ _ ____ _ ____ _ ____ _ ____ _ ____", Pattern: []float64{31.75, -6.35, 6.35, -6.35}},
	{Name: "CENTER2", Description: "Center (.5x) ___ _ ___ _ ___ _ ___ _ ___ _ ___", Pattern: []float64{19.05, -3.175, 3.175, -3.175}},
	{Name: "CENTERX2", Description: "Center (2x) ________  __  ________  __  _____", Pattern: []float64{63.5, -12.7, 12.7, -12.7}},
	{Name: "DASHDOT", Description: "Dash dot __ . __ . __ . __ . __ . __ . __ . __", Pattern: []float64{12.7, -6.35, 0, -6.35}},
	{Name: "DASHDOT2", Description: "Dash dot (.5x) _._._._._._._._._._._._._._._.", Pattern: []float64{6.35, -3.175, 0, -3.175}},
	{Name: "DASHDOTX2", Description: "Dash dot (2x) ____  .  ____  .  ____  .  ___", Pattern: []float64{25.4, -12.7, 0, -12.7}},
	{Name: "DASHED", Description: "Dashed __ __ __ __ __ __ __ __ __ __ __ __ __ _", Pattern: []float64{12.7, -6.35}},
	{Name: "DASHED2", Description: "Dashed (.5x) _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _", Pattern: []float64{6.35, -3.175}},
	{Name: "DASHEDX2", Description: "Dashed (2x) ____  ____  ____  ____  ____  ___", Pattern: []float64{25.4, -12.7}},
	{Name: "DIVIDE", Description: "Divide ____ . . ____ . . ____ . . ____ . . ____", Pattern: []float64{12.7, -6.35, 0, -6.35, 0, -6.35}},
	{Name: "DIVIDE2", Description: "Divide (.5x) __..__..__..__..__..__..__..__.._", Pattern: []float64{6.35, -3.175, 0, -3.175, 0, -3.175}},
	{Name: "DIVIDEX2", Description: "Divide (2x) ________  .  .  ________  .  .  _", Pattern: []float64{25.4, -12.7, 0, -12.7, 0, -12.7}},
	{Name: "DOT", Description: "Dot . . . . . . . . . . . . . . . . . . . . . . . .", Pattern: []float64{0, -6.35}},
	{Name: "DOT2", Description: "Dot (.5x) ........................................", Pattern: []float64{0, -3.175}},
	{Name: "DOTX2", Description: "Dot (2x) .  .  .  .  .  .  .  .  .  .  .  .  .  .", Pattern: []float64{0, -12.7}},
	{Name: "HIDDEN", Description: "Hidden __ __ __ __ __ __ __ __ __ __ __ __ __ __", Pattern: []float64{6.35, -3.175}},
	{Name: "HIDDEN2", Description: "Hidden (.5x) _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _ _", Pattern: []float64{3.175, -1.5875}},
	{Name: "HIDDENX2", Description: "Hidden (2x) ____ ____ ____ ____ ____ ____ ____", Pattern: []float64{12.7, -6.35}},
	{Name: "PHANTOM", Description: "Phantom ______  __  __  ______  __  __  ______", Pattern: []float64{31.75, -6.35, 6.35, -6.35, 6.35, -6.35}},
	{Name: "PHANTOM2", Description: "Phantom (.5x) ___ _ _ ___ _ _ ___ _ _ ___ _ _ ___", Pattern: []float64{15.875, -3.175, 3.175, -3.175, 3.175, -3.175}},
	{Name: "PHANTOMX2", Description: "Phantom (2x) ____________    ____    ____    _", Pattern: []float64{63.5, -12.7, 12.7, -12.7, 12.7, -12.7}},
	{Name: "ACAD_ISO02W100", Description: "ISO dash __ __ __ __ __ __ __ __ __ __ __ __ __", Pattern: []float64{12, -3}},
	{Name: "ACAD_ISO03W100", Description: "ISO dash space __    __    __    __    __    __", Pattern: []float64{12, -18}},
	{Name: "ACAD_ISO04W100", Description: "ISO long-dash dot ____ . ____ . ____ . ____ . _", Pattern: []float64{24, -3, 0, -3}},
	{Name: "ACAD_ISO05W100", Description: "ISO long-dash double-dot ____ .. ____ .. ____ .", Pattern: []float64{24, -3, 0, -3, 0, -3}},
	{Name: "ACAD_ISO06W100", Description: "ISO long-dash triple-dot ____ ... ____ ... ____", Pattern: []float64{24, -3, 0, -3, 0, -3, 0, -3}},
	{Name: "ACAD_ISO07W100", Description: "ISO dot . . . . . . . . . . . . . . . . . . . .", Pattern: []float64{0, -3}},
	{Name: "ACAD_ISO08W100", Description: "ISO long-dash short-dash ____ __ ____ __ ____ _", Pattern: []float64{24, -3, 6, -3}},
	{Name: "ACAD_ISO09W100", Description: "ISO long-dash double-short-dash ____ __ __ ____", Pattern: []float64{24, -3, 6, -3, 6, -3}},
	{Name: "ACAD_ISO10W100", Description: "ISO dash dot __ . __ . __ . __ . __ . __ . __ .", Pattern: []float64{12, -3, 0, -3}},
	{Name: "ACAD_ISO11W100", Description: "ISO double-dash dot __ __ . __ __ . __ __ . __ _", Pattern: []float64{12, -3, 12, -3, 0, -3}},
	{Name: "ACAD_ISO12W100", Description: "ISO dash double-dot __ . . __ . . __ . . __ . .", Pattern: []float64{12, -3, 0, -3, 0, -3}},
	{Name: "ACAD_ISO13W100", Description: "ISO double-dash double-dot __ __ . . __ __ . . _", Pattern: []float64{12, -3, 12, -3, 0, -3, 0, -3}},
	{Name: "ACAD_ISO14W100", Description: "ISO dash triple-dot __ . . . __ . . . __ . . . _", Pattern: []float64{12, -3, 0, -3, 0, -3, 0, -3}},
	{Name: "ACAD_ISO15W100", Description: "ISO double-dash triple-dot __ __ . . . __ __ . .", Pattern: []float64{12, -3, 12, -3, 0, -3, 0, -3, 0, -3}},
}
