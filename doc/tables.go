package doc

import (
	"errors"
	"math"
	"slices"
	"strings"
)

// This file makes the drawing's tables (layers, linetypes, dimension styles) and its current
// properties editable inside a transaction. Every such edit snapshots them once per transaction
// (the same snapshot Txn.ImportBlockSupport takes), so commit records them in the journal step,
// abort restores them, and undo and redo restore exactly the records and fields the step changed:
// a direct edit made since (Drawing.AddLayer, a field written in place) to anything else survives.
// The change set reports them as ChangeSet.Support.

// CurrentProps are the drawing's current properties, what new entities and dimensions get, and
// its global defaults (LTSCALE, LWDEFAULT, INSUNITS), the last level of property resolution.
type CurrentProps struct {
	// Layer is CLAYER (Drawing.CurrentLayer).
	Layer string
	// Color is CECOLOR (Drawing.CurrentColor): 1–255, ByLayer or ByBlock.
	Color int
	// Linetype is CELTYPE (Drawing.CurrentLinetype).
	Linetype string
	// LinetypeScale is CELTSCALE (Drawing.CurrentLinetypeScale).
	LinetypeScale float64
	// Lineweight is CELWEIGHT (Drawing.CurrentLineweight).
	Lineweight Lineweight
	// LTScale is LTSCALE (Drawing.LTScale).
	LTScale float64
	// DimStyle is DIMSTYLE (Drawing.CurrentDimStyle).
	DimStyle string
	// LWDefault is LWDEFAULT (Drawing.LWDefault), the global default lineweight.
	LWDefault Lineweight
	// InsUnits is INSUNITS ($INSUNITS, Drawing.Header.InsUnits): the drawing unit code, 0 to 24
	// (0 unitless, 4 millimetres). A CurrentProps built from a literal must set it, or it sets 0.
	InsUnits int
}

// Current returns the drawing's current properties.
func (d *Drawing) Current() CurrentProps {
	return CurrentProps{Layer: d.CurrentLayer, Color: d.CurrentColor, Linetype: d.CurrentLinetype,
		LinetypeScale: d.CurrentLinetypeScale, Lineweight: d.CurrentLineweight, LTScale: d.LTScale, DimStyle: d.CurrentDimStyle, LWDefault: d.LWDefault, InsUnits: d.Header.InsUnits}
}

func (d *Drawing) setCurrent(p CurrentProps) {
	d.CurrentLayer, d.CurrentColor, d.CurrentLinetype = p.Layer, p.Color, p.Linetype
	d.CurrentLinetypeScale, d.CurrentLineweight, d.LTScale, d.CurrentDimStyle = p.LinetypeScale, p.Lineweight, p.LTScale, p.DimStyle
	d.LWDefault = p.LWDefault
	d.Header.InsUnits = p.InsUnits
}

// LWDefaultValue returns LWDEFAULT as it applies: LWDefault when it is a width (positive, up to
// 211, or LineweightZero), else DefaultLineweightHundredths.
func (d *Drawing) LWDefaultValue() Lineweight {
	if w := d.LWDefault; w == LineweightZero || w > 0 && w <= 211 {
		return w
	}
	return DefaultLineweightHundredths
}

// LTScaleValue returns LTSCALE as it applies: LTScale, or 1 when it is unset or invalid.
func (d *Drawing) LTScaleValue() float64 { return LinetypeScaleOf(d.LTScale) }

// SetCurrent changes the current properties as one undoable change of this transaction. It
// fails, changing nothing, when the layer or the dimension style does not exist, the colour is
// not 0–256, the linetype is neither ByLayer, ByBlock nor in the table, a scale is negative or
// not finite (0 is unset, reading as 1), the lineweight is not ByLayer, ByBlock, Default,
// LineweightZero or 1–211, or INSUNITS is not 0–24.
func (t *Txn) SetCurrent(p CurrentProps) error {
	if t.done {
		return errTxnClosed
	}
	d := t.d
	switch {
	case d.Layer(p.Layer) == nil:
		return errors.New("doc: no such layer: " + p.Layer)
	case p.Color < 0 || p.Color > ByLayer:
		return errors.New("doc: invalid colour")
	case !IsByLayerLinetype(p.Linetype) && !IsByBlockLinetype(p.Linetype) && d.linetypeIndex(p.Linetype) < 0:
		return errors.New("doc: no such linetype: " + p.Linetype)
	case !validScale(p.LinetypeScale) || !validScale(p.LTScale):
		return errors.New("doc: invalid linetype scale")
	case !p.Lineweight.valid() || !(p.LWDefault == 0 || p.LWDefault == LineweightZero || p.LWDefault > 0 && p.LWDefault <= 211):
		return errors.New("doc: invalid lineweight")
	case d.dimStyleIndex(p.DimStyle) < 0:
		return errors.New("doc: no such dimension style: " + p.DimStyle)
	case p.InsUnits < 0 || p.InsUnits > 24:
		return errors.New("doc: invalid insertion units")
	}
	cur := d.Current()
	if p == cur {
		return nil
	}
	// Only LTSCALE and LWDEFAULT change how existing entities draw; the rest apply to new ones.
	t.touchSupportLook(p.LTScale != cur.LTScale || p.LWDefault != cur.LWDefault)
	d.setCurrent(p)
	return nil
}

func validScale(v float64) bool { return v >= 0 && !math.IsInf(v, 0) }

func (w Lineweight) valid() bool {
	return w == LineweightByLayer || w == LineweightByBlock || w == LineweightDefault || w == LineweightZero || w > 0 && w <= 211
}

// AddLayer returns the named layer, creating it with color (1–255; anything else is 7) as one
// undoable change of this transaction when it is missing. The pointer is the live record: change
// it through ModifyLayer.
func (t *Txn) AddLayer(name string, color int) (*Layer, error) {
	if t.done {
		return nil, errTxnClosed
	}
	if l := t.d.Layer(name); l != nil {
		return l, nil
	}
	if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name, "<>/\\\":;?*|,=`") {
		return nil, errors.New("doc: invalid layer name")
	}
	if color < 1 || color > 255 {
		color = 7
	}
	t.touchSupport()
	return t.d.AddLayer(name, color), nil
}

// ModifyLayer calls fn on the named layer as one undoable change of this transaction. fn must not
// rename the layer: a changed Name is put back and ModifyLayer fails.
func (t *Txn) ModifyLayer(name string, fn func(*Layer)) error {
	if t.done {
		return errTxnClosed
	}
	l := t.d.Layer(name)
	if l == nil {
		return errors.New("doc: no such layer: " + name)
	}
	t.touchSupport()
	fn(l)
	if l.Name != name {
		l.Name = name
		return errors.New("doc: ModifyLayer cannot rename a layer")
	}
	return nil
}

// RemoveLayer deletes the named layer as one undoable change of this transaction, under the rules
// of Drawing.RemoveLayer (not 0, not current, not in use); it reports whether it did.
func (t *Txn) RemoveLayer(name string) bool {
	if t.done || t.d.Layer(name) == nil {
		return false
	}
	before := t.d.snapshotSupport()
	if !t.d.RemoveLayer(name) {
		return false
	}
	if t.supportBefore == nil {
		t.supportBefore = before
	}
	return true
}

// touchSupport snapshots the tables and current properties at the transaction's first table edit,
// and queues the entities whose appearance a table edit can change (block references and
// dimensions) for TakeEdits, as the commit reports them modified.
func (t *Txn) touchSupport() { t.touchSupportLook(true) }

// touchSupportLook is touchSupport for an edit that changes how existing entities draw (look) or
// only what new ones get, which queues nothing. The dependents are queued once until TakeEdits.
func (t *Txn) touchSupportLook(look bool) {
	if t.supportBefore == nil {
		t.supportBefore = t.d.snapshotSupport()
	}
	if look && !t.dependentsQueued {
		t.dependentsQueued = true
		t.edits = append(t.edits, t.d.dependentHandles()...)
	}
}

// dependentHandles returns, in ascending order, the entities whose drawing depends on the tables
// beyond their layer's look: block references and dimensions.
func (d *Drawing) dependentHandles() []Handle {
	var out []Handle
	for h, e := range d.entities {
		switch e.(type) {
		case *Insert, *Dimension:
			out = append(out, h)
		}
	}
	slices.Sort(out)
	return out
}

// dimStyleEntry is a DIMSTYLE record: the modelled variables and, for writers, the groups of the
// variables the document does not model, as a file held them.
type dimStyleEntry struct {
	style DimStyle
	extra []Pair
}

func (e dimStyleEntry) clone() dimStyleEntry {
	e.extra = slices.Clone(e.extra)
	return e
}

// DimStyles returns the dimension style table in table order.
func (d *Drawing) DimStyles() []DimStyle {
	out := make([]DimStyle, len(d.dimStyles))
	for i, e := range d.dimStyles {
		out[i] = e.style
	}
	return out
}

// DimStyle returns the named dimension style (any case) and whether the table has it.
func (d *Drawing) DimStyle(name string) (DimStyle, bool) {
	if i := d.dimStyleIndex(name); i >= 0 {
		return d.dimStyles[i].style, true
	}
	return DimStyle{}, false
}

// DimStyleExtra returns the groups of the named record's variables the document does not model
// (DIMPOST, DIMTOL, DIMALT and the like), as read, for file writers. The slice is shared.
func (d *Drawing) DimStyleExtra(name string) []Pair {
	if i := d.dimStyleIndex(name); i >= 0 {
		return d.dimStyles[i].extra
	}
	return nil
}

// ActiveDimStyle returns the current dimension style, CurrentDimStyle's record, or
// DefaultDimStyle when the table lacks it.
func (d *Drawing) ActiveDimStyle() DimStyle {
	if s, ok := d.DimStyle(d.CurrentDimStyle); ok {
		return s
	}
	return DefaultDimStyle()
}

func (d *Drawing) dimStyleIndex(name string) int {
	for i, e := range d.dimStyles {
		if strings.EqualFold(e.style.Name, name) {
			return i
		}
	}
	return -1
}

// LoadDimStyles replaces the dimension style table outside any transaction, for file readers:
// styles[i] with the unmodelled groups extras[i] (extras may be shorter). An empty list keeps the
// table. CurrentDimStyle moves to the first style when the new table lacks it.
func (d *Drawing) LoadDimStyles(styles []DimStyle, extras [][]Pair) {
	if len(styles) == 0 {
		return
	}
	d.dimStyles = d.dimStyles[:0]
	for i, s := range styles {
		if s.Name == "" || d.dimStyleIndex(s.Name) >= 0 {
			continue
		}
		e := &dimStyleEntry{style: s}
		if i < len(extras) {
			e.extra = slices.Clone(extras[i])
		}
		d.dimStyles = append(d.dimStyles, e)
	}
	if d.dimStyleIndex(d.CurrentDimStyle) < 0 && len(d.dimStyles) > 0 {
		d.CurrentDimStyle = d.dimStyles[0].style.Name
	}
}

// SetDimStyle adds s to the dimension style table, or replaces the record of the same name (any
// case, keeping its unmodelled groups), as one undoable change of this transaction. Dimensions
// reference their style by name, so every dimension that does not override what changed follows
// the edit.
func (t *Txn) SetDimStyle(s DimStyle) error {
	if t.done {
		return errTxnClosed
	}
	if n := strings.TrimSpace(s.Name); n == "" || n != s.Name || strings.ContainsAny(n, "<>/\\\":;?*|,=`") {
		return errors.New("doc: invalid dimension style name")
	}
	if i := t.d.dimStyleIndex(s.Name); i >= 0 {
		if t.d.dimStyles[i].style == s {
			return nil
		}
		t.touchSupport()
		t.d.dimStyles[i].style = s
		return nil
	}
	t.touchSupport()
	t.d.dimStyles = append(t.d.dimStyles, &dimStyleEntry{style: s})
	return nil
}

// RemoveDimStyle deletes the named dimension style as one undoable change of this transaction.
// The current style, the last one and a style a dimension names are kept, with an error.
func (t *Txn) RemoveDimStyle(name string) error {
	if t.done {
		return errTxnClosed
	}
	d := t.d
	i := d.dimStyleIndex(name)
	switch {
	case i < 0:
		return errors.New("doc: no such dimension style: " + name)
	case strings.EqualFold(d.CurrentDimStyle, name) || len(d.dimStyles) == 1:
		return errors.New("doc: the current dimension style cannot be removed")
	}
	uses := func(e Entity) bool {
		dm, ok := e.(*Dimension)
		return ok && strings.EqualFold(dm.StyleName, name)
	}
	for _, e := range d.entities {
		if uses(e) {
			return errors.New("doc: dimension style is in use")
		}
	}
	for _, b := range d.blocks {
		for _, e := range b.Entities {
			if uses(e) {
				return errors.New("doc: dimension style is in use")
			}
		}
	}
	t.touchSupport()
	d.dimStyles = slices.Delete(d.dimStyles, i, i+1)
	return nil
}

// restoreTables applies the linetype, dimension style and current-property differences between
// previous and target to the drawing, leaving everything that did not change between them alone.
func (d *Drawing) restoreTables(target, previous *blockSupport) {
	lts := func(s *blockSupport) map[string]Linetype {
		m := map[string]Linetype{}
		for _, l := range s.linetypes {
			m[strings.ToUpper(l.Name)] = l
		}
		return m
	}
	want, had := lts(target), lts(previous)
	for _, n := range unionKeys(want, had) {
		w, inW := want[n]
		h, inH := had[n]
		if inW == inH && (!inW || w.equal(h)) {
			continue
		}
		if !inW {
			if i := d.linetypeIndex(n); i >= 0 {
				d.linetypes = slices.Delete(d.linetypes, i, i+1)
			}
			continue
		}
		d.putLinetype(w.Clone())
	}
	dss := func(s *blockSupport) map[string]dimStyleEntry {
		m := map[string]dimStyleEntry{}
		for _, e := range s.dimStyles {
			m[strings.ToUpper(e.style.Name)] = e
		}
		return m
	}
	wantS, hadS := dss(target), dss(previous)
	for _, n := range unionKeys(wantS, hadS) {
		w, inW := wantS[n]
		h, inH := hadS[n]
		if inW == inH && (!inW || w.style == h.style && slices.Equal(w.extra, h.extra)) {
			continue
		}
		i := d.dimStyleIndex(n)
		switch {
		case !inW && i >= 0:
			d.dimStyles = slices.Delete(d.dimStyles, i, i+1)
		case inW && i >= 0:
			*d.dimStyles[i] = w.clone()
		case inW:
			e := w.clone()
			d.dimStyles = append(d.dimStyles, &e)
		}
	}
	tc, pc, cur := target.current, previous.current, d.Current()
	if tc.Layer != pc.Layer {
		cur.Layer = tc.Layer
	}
	if tc.Color != pc.Color {
		cur.Color = tc.Color
	}
	if tc.Linetype != pc.Linetype {
		cur.Linetype = tc.Linetype
	}
	if tc.LinetypeScale != pc.LinetypeScale {
		cur.LinetypeScale = tc.LinetypeScale
	}
	if tc.Lineweight != pc.Lineweight {
		cur.Lineweight = tc.Lineweight
	}
	if tc.LTScale != pc.LTScale {
		cur.LTScale = tc.LTScale
	}
	if tc.DimStyle != pc.DimStyle {
		cur.DimStyle = tc.DimStyle
	}
	if tc.LWDefault != pc.LWDefault {
		cur.LWDefault = tc.LWDefault
	}
	if tc.InsUnits != pc.InsUnits {
		cur.InsUnits = tc.InsUnits
	}
	d.setCurrent(cur)
}

func unionKeys[V any](a, b map[string]V) []string {
	var out []string
	for k := range a {
		out = append(out, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// sameLook reports whether two snapshots draw existing entities alike: the same tables, LTSCALE
// and LWDEFAULT. The other current properties (CLAYER, CECOLOR, CELTYPE, CELTSCALE, CELWEIGHT,
// DIMSTYLE, INSUNITS) only apply to new entities.
func (s *blockSupport) sameLook(o *blockSupport) bool {
	if s.current.LTScale != o.current.LTScale || s.current.LWDefault != o.current.LWDefault {
		return false
	}
	c := *o
	c.current = s.current
	return s.equal(&c)
}

// equal reports whether two snapshots hold the same tables and current properties.
func (s *blockSupport) equal(o *blockSupport) bool {
	if len(s.layers) != len(o.layers) || len(s.linetypes) != len(o.linetypes) || len(s.dimStyles) != len(o.dimStyles) || s.current != o.current {
		return false
	}
	for i := range s.layers {
		if *s.layers[i] != *o.layers[i] {
			return false
		}
	}
	for i := range s.linetypes {
		if !s.linetypes[i].equal(o.linetypes[i]) {
			return false
		}
	}
	for i := range s.dimStyles {
		if s.dimStyles[i].style != o.dimStyles[i].style || !slices.Equal(s.dimStyles[i].extra, o.dimStyles[i].extra) {
			return false
		}
	}
	for _, t := range []string{"STYLE", "LTYPE"} {
		a, b := s.preserved.Tables[t], o.preserved.Tables[t]
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if !slices.Equal(a[i].Pairs, b[i].Pairs) {
				return false
			}
		}
	}
	return true
}
