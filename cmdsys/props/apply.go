package props

import (
	"errors"
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// StepName is the undo step name of a Properties edit (doc.Drawing.NameLastStep).
const StepName = "PROPERTIES"

// Apply sets property id of target t to v (or clears it, v.Clear) as one undo step named
// PROPERTIES. It refuses with ErrBusy while a command runs or a transaction is open. On a
// selection it changes every filtered object the row applies to, skipping objects on locked, off
// or frozen layers, objects the document does not model and vanished handles (Result.Skipped);
// an edit that changes nothing records no step. Defaults are set through their system variables
// (cmdsys.Vars.SetTx), so SETVAR and the panel agree; TEXTSIZE and DIMARCSYM are session variables and are not journalled.
// A Selector row ("vertex") changes nothing: the caller moves Target.Vertex.
func Apply(h Host, t Target, id string, v Value) (Result, error) {
	if h.Active() != "" {
		return Result{}, ErrBusy
	}
	switch t.Kind {
	case TargetLayer:
		return applyLayer(h, t.Layer, id, v)
	case TargetDefaults:
		return applyDefaults(h, id, v)
	}
	d := h.Drawing()
	ents, _, vanished := selection(d, t)
	if len(ents) == 0 {
		return Result{Skipped: vanished}, ErrStale
	}
	return inTxn(d, func(tx *doc.Txn) (Result, error) {
		r, err := applyEntities(tx, newEnv(d, h.Vars(), t.Vertex), ents, id, v)
		r.Skipped += vanished
		return r, err
	})
}

// ApplyTx is Apply for a host that holds its own transaction, the engine cadmcp's update_entity
// will use (backlog N13; nothing calls it yet): it sets
// property id of the objects handles to v inside tx, which the caller commits. Vertex rows act
// on the first vertex. Handles that name no object count as skipped.
func ApplyTx(tx *doc.Txn, d *doc.Drawing, handles []doc.Handle, id string, v Value) (Result, error) {
	ents, _, vanished := selection(d, Target{Handles: handles})
	if len(ents) == 0 {
		return Result{Skipped: vanished}, ErrStale
	}
	r, err := applyEntities(tx, newEnv(d, nil, 0), ents, id, v)
	r.Skipped += vanished
	return r, err
}

// inTxn runs fn in a new transaction, committing it as a PROPERTIES step when fn succeeds and
// aborting it otherwise.
func inTxn(d *doc.Drawing, fn func(tx *doc.Txn) (Result, error)) (Result, error) {
	tx, err := d.Begin()
	if err != nil {
		return Result{}, ErrBusy
	}
	r, err := fn(tx)
	if err != nil {
		tx.Abort()
		return Result{}, err
	}
	if cs := tx.Commit(); !cs.Empty() {
		d.NameLastStep(StepName)
		r.Step = true
	}
	return r, nil
}

// findDef returns the row id among the categories of ents.
func findDef(ents []doc.Entity, id string) *def {
	for _, c := range catsFor(ents) {
		for _, d := range c.defs {
			if d.id == id {
				return d
			}
		}
	}
	return nil
}

func applyEntities(tx *doc.Txn, c *env, ents []doc.Entity, id string, v Value) (Result, error) {
	d := c.d
	c.first = ents[0]
	if p, ok := ents[0].(*doc.LWPolyline); ok {
		c.vertex = min(c.vertex, max(len(p.Points)-1, 0))
	}
	dd := findDef(ents, id)
	switch {
	case dd == nil:
		return Result{}, newError(Invalid, "ui.props.err.unknown", id)
	case dd.selector:
		return Result{}, nil
	case dd.readOnly || dd.set == nil:
		return Result{}, ErrReadOnly
	case v.Varies:
		return Result{}, ErrInvalid
	case v.Clear && dd.clear == nil:
		return Result{}, ErrReadOnly
	}
	if !v.Clear {
		var err error
		if v, err = c.canon(Property{Kind: dd.kind, Min: dd.min, Max: dd.max, Positive: dd.positive, Options: optionsOf(c, dd)}, v); err != nil {
			return Result{}, err
		}
		if err := loadLinetype(tx, d, dd.kind, v); err != nil {
			return Result{}, err
		}
	}
	write := func(e doc.Entity) error {
		if v.Clear {
			dd.clear(c, e)
			return nil
		}
		return dd.set(c, e, v)
	}
	var r Result
	for _, e := range ents {
		if _, unknown := e.(*doc.Unknown); unknown || !d.EntityEditable(e) {
			r.Skipped++
			continue
		}
		if dd.only != nil && !dd.only(c, e) {
			continue
		}
		probe := e.Clone().(doc.Entity)
		if err := write(probe); err != nil {
			return Result{}, ErrInvalid
		}
		if sameEntity(e, probe) {
			continue
		}
		tx.Modify(e.Handle(), func(x doc.Entity) { _ = write(x) })
		r.Changed++
	}
	return r, nil
}

// optionsOf returns a choice row's options when the value must be one of them (enumerations).
func optionsOf(c *env, dd *def) []Option {
	if dd.kind == KindEnum && dd.options != nil {
		return dd.options(c)
	}
	return nil
}

// loadLinetype loads a library linetype a Linetype value names when the drawing lacks it.
func loadLinetype(tx *doc.Txn, d *doc.Drawing, k Kind, v Value) error {
	if k != KindLinetype || doc.IsByLayerLinetype(v.Str) || doc.IsByBlockLinetype(v.Str) {
		return nil
	}
	if _, ok := d.Linetype(v.Str); ok {
		return nil
	}
	if err := tx.LoadLibraryLinetype(v.Str); err != nil {
		return ErrInvalid
	}
	return nil
}

// canon validates v for a row of p's kind and bounds and returns it in canonical form: names in
// the drawing's spelling, BYLAYER and BYBLOCK upper-case, an enumeration one of its options.
func (c *env) canon(p Property, v Value) (Value, error) {
	d := c.d
	bad := func() (Value, error) { return Value{}, ErrInvalid }
	switch p.Kind {
	case KindNumber, KindLength, KindAngle:
		if math.IsNaN(v.Num) || math.IsInf(v.Num, 0) {
			return bad()
		}
		if err := checkBounds(p, v.Num); err != nil {
			return Value{}, err
		}
		return NumValue(v.Num), nil
	case KindInt:
		if err := checkBounds(p, float64(v.Int)); err != nil {
			return Value{}, err
		}
		return IntValue(v.Int), nil
	case KindBool:
		return BoolValue(v.Bool), nil
	case KindText, KindMultiText:
		return StrValue(v.Str), nil
	case KindEnum:
		for _, o := range p.Options {
			if o.Value.Int == v.Int && o.Value.Str == v.Str {
				return o.Value, nil
			}
		}
		if len(p.Options) > 0 {
			return bad()
		}
		return v, nil
	case KindColor:
		if v.Int < 0 || v.Int > doc.ByLayer {
			return bad()
		}
		return IntValue(v.Int), nil
	case KindLineweight:
		w := doc.Lineweight(v.Int)
		if w != doc.LineweightByLayer && w != doc.LineweightByBlock && w != doc.LineweightDefault {
			if _, ok := cmdsys.LineweightFromVar(w.DXF()); !ok {
				return bad()
			}
		}
		return IntValue(v.Int), nil
	case KindLinetype:
		switch {
		case doc.IsByLayerLinetype(v.Str):
			return StrValue(doc.LinetypeByLayer), nil
		case doc.IsByBlockLinetype(v.Str):
			return StrValue(doc.LinetypeByBlock), nil
		}
		if l, ok := d.Linetype(v.Str); ok {
			return StrValue(l.Name), nil
		}
		if l, ok := doc.LibraryLinetype(v.Str); ok {
			return StrValue(l.Name), nil
		}
		return bad()
	case KindLayer:
		if l := d.Layer(strings.TrimSpace(v.Str)); l != nil {
			return StrValue(l.Name), nil
		}
		return bad()
	case KindTextStyle:
		if n, ok := doc.FindTextStyle(d, v.Str); ok {
			return StrValue(n), nil
		}
		return bad()
	case KindDimStyle:
		if s, ok := d.DimStyle(strings.TrimSpace(v.Str)); ok {
			return StrValue(s.Name), nil
		}
		return bad()
	case KindBlock:
		if b := d.Block(strings.TrimSpace(v.Str)); b != nil {
			return StrValue(b.Name), nil
		}
		return bad()
	case KindArrow:
		if b, ok := cmdsys.CanonicalArrow(d, v.Str); ok {
			return StrValue(b), nil
		}
		return bad()
	}
	return v, nil
}

// checkBounds applies a row's Positive, Min and Max.
func checkBounds(p Property, f float64) error {
	if p.Positive && !(f > 0) {
		return newError(Invalid, "ui.props.err.positive")
	}
	if p.Max > p.Min && (f < p.Min || f > p.Max) {
		lo, hi := cmdsys.FormatNumber(p.Min), cmdsys.FormatNumber(p.Max)
		if p.Kind == KindAngle {
			lo, hi = cmdsys.FormatNumber(p.Min*180/math.Pi), cmdsys.FormatNumber(p.Max*180/math.Pi)
		}
		return newError(Invalid, "ui.props.err.range", lo, hi)
	}
	return nil
}

// --- layers ---------------------------------------------------------------

func applyLayer(h Host, name, id string, v Value) (Result, error) {
	d := h.Drawing()
	l := d.Layer(name)
	if l == nil {
		return Result{}, ErrStale
	}
	c := newEnv(d, h.Vars(), 0)
	var kind Kind
	switch id {
	case "color":
		kind = KindColor
	case "linetype":
		kind = KindLinetype
	case "lineweight":
		kind = KindLineweight
	case "on", "frozen", "locked", "plot":
		kind = KindBool
	case "description":
		kind = KindText
	case "name", "current":
		return Result{}, ErrReadOnly
	default:
		return Result{}, newError(Invalid, "ui.props.err.unknown", id)
	}
	if v.Clear || v.Varies {
		return Result{}, ErrInvalid
	}
	v, err := c.canon(Property{Kind: kind}, v)
	if err != nil {
		return Result{}, err
	}
	next := *l
	switch id {
	case "color":
		if v.Int < 1 || v.Int > 255 {
			return Result{}, ErrInvalid
		}
		next.Color = v.Int
	case "linetype":
		if doc.IsByLayerLinetype(v.Str) || doc.IsByBlockLinetype(v.Str) {
			return Result{}, ErrInvalid
		}
		next.Linetype = v.Str
		if strings.EqualFold(v.Str, doc.LinetypeContinuous) {
			next.Linetype = ""
		}
	case "lineweight":
		switch w := doc.Lineweight(v.Int); {
		case w == doc.LineweightDefault, w == doc.LineweightZero, w > 0:
			next.Lineweight = w.LayerValue()
		default:
			return Result{}, ErrInvalid
		}
	case "on":
		next.Off = !v.Bool
	case "frozen":
		if v.Bool && l.Name == d.CurrentLayer {
			return Result{}, newError(Refused, "ui.props.err.freeze_current", l.Name)
		}
		next.Frozen = v.Bool
	case "locked":
		next.Locked = v.Bool
	case "plot":
		next.NoPlot = !v.Bool
	case "description":
		next.Description = v.Str
	}
	if next == *l {
		return Result{}, nil
	}
	return inTxn(d, func(tx *doc.Txn) (Result, error) {
		if id == "linetype" {
			if err := loadLinetype(tx, d, KindLinetype, v); err != nil {
				return Result{}, err
			}
		}
		if err := tx.ModifyLayer(name, func(x *doc.Layer) { *x = next }); err != nil {
			return Result{}, ErrStale
		}
		return Result{Changed: 1}, nil
	})
}

// --- defaults -------------------------------------------------------------

// defaultsVar returns the system variable a Defaults row edits and the row's kind and bounds.
func defaultsVar(id string) (string, Property, bool) {
	switch id {
	case "color":
		return "CECOLOR", Property{Kind: KindColor}, true
	case "layer":
		return "CLAYER", Property{Kind: KindLayer}, true
	case "linetype":
		return "CELTYPE", Property{Kind: KindLinetype}, true
	case "ltscale":
		return "CELTSCALE", Property{Kind: KindNumber, Positive: true}, true
	case "lineweight":
		return "CELWEIGHT", Property{Kind: KindLineweight}, true
	case "global_ltscale":
		return "LTSCALE", Property{Kind: KindNumber, Positive: true}, true
	case "lwdefault":
		return "LWDEFAULT", Property{Kind: KindLineweight}, true
	case "insunits":
		return "INSUNITS", Property{Kind: KindInt, Min: 0, Max: 24}, true
	case "textsize":
		return "TEXTSIZE", Property{Kind: KindLength, Positive: true}, true
	case "dimstyle":
		return "DIMSTYLE", Property{Kind: KindDimStyle}, true
	case "arc_symbol":
		return "DIMARCSYM", Property{Kind: KindInt, Min: 0, Max: 2}, true
	}
	return "", Property{}, false
}

func applyDefaults(h Host, id string, v Value) (Result, error) {
	d, vars := h.Drawing(), h.Vars()
	c := newEnv(d, vars, 0)
	if v.Clear || v.Varies {
		return Result{}, ErrInvalid
	}
	if r := styleRowOf(id); r != nil {
		v, err := c.canon(Property{Kind: r.kind, Min: r.min, Max: r.max, Positive: r.pos, Options: optionsIf(c, r)}, v)
		if err != nil {
			return Result{}, err
		}
		old := d.ActiveDimStyle()
		st := old
		if err := r.set(d, &st, v); err != nil {
			return Result{}, ErrInvalid
		}
		diff := doc.DimStyleDiff(old, st)
		if diff == 0 {
			return Result{}, nil
		}
		return inTxn(d, func(tx *doc.Txn) (Result, error) {
			for _, sp := range cmdsys.DimVarSpecs() {
				if sp.Var&diff != 0 {
					if err := vars.SetTx(tx, sp.Name, sp.Get(st)); err != nil {
						return Result{}, varError(err)
					}
				}
			}
			return Result{Changed: 1}, nil
		})
	}
	name, p, ok := defaultsVar(id)
	if !ok {
		return Result{}, newError(Invalid, "ui.props.err.unknown", id)
	}
	v, err := c.canon(p, v)
	if err != nil {
		return Result{}, err
	}
	var value any
	switch p.Kind {
	case KindColor:
		value = cmdsys.ColorText(v.Int)
	case KindLineweight:
		value = doc.Lineweight(v.Int).DXF()
	case KindNumber, KindLength:
		value = v.Num
	case KindInt:
		value = v.Int
	default:
		value = v.Str
	}
	before, _ := vars.Get(name)
	if !vars.Undoable(name) {
		if err := vars.Set(name, value); err != nil {
			return Result{}, varError(err)
		}
		after, _ := vars.Get(name)
		if after == before {
			return Result{}, nil
		}
		return Result{Changed: 1, NotUndoable: true}, nil
	}
	return inTxn(d, func(tx *doc.Txn) (Result, error) {
		if err := loadLinetype(tx, d, p.Kind, v); err != nil {
			return Result{}, err
		}
		if err := vars.SetTx(tx, name, value); err != nil {
			return Result{}, varError(err)
		}
		if after, _ := vars.Get(name); after == before {
			return Result{}, nil
		}
		return Result{Changed: 1}, nil
	})
}

// styleRowOf returns the dimension style row id, or nil.
func styleRowOf(id string) *styleRow {
	for _, dc := range dimCategories {
		for _, r := range dc.style {
			if r.id == id {
				return r
			}
		}
	}
	return nil
}

func optionsIf(c *env, r *styleRow) []Option {
	if r.kind == KindEnum && r.options != nil {
		return r.options(c)
	}
	return nil
}

// varError maps a system variable error to the panel's.
func varError(err error) error {
	switch {
	case errors.Is(err, cmdsys.ErrVarBusy):
		return ErrBusy
	case errors.Is(err, cmdsys.ErrVarReadOnly):
		return ErrReadOnly
	case errors.Is(err, cmdsys.ErrVarInvalid):
		return ErrRefused
	}
	return ErrInvalid
}
