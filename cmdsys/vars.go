package cmdsys

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"go-draw/geom"
)

// VarKind is the type of a system variable.
type VarKind int

const (
	// VarInt is a whole number; its value is an int.
	VarInt VarKind = iota
	// VarReal is a real number or an angle in radians; its value is a float64.
	VarReal
	// VarText is a string.
	VarText
	// VarPoint is a model-space point; its value is a geom.Vec2.
	VarPoint
)

// Errors Vars.Set and Vars.SetString return, for SETVAR and variables typed by name to word.
var (
	ErrVarUnknown  = errors.New("unknown variable")
	ErrVarReadOnly = errors.New("variable is read-only")
	ErrVarType     = errors.New("value has the wrong type")
	ErrVarRange    = errors.New("value out of range")
)

// Var describes one system variable. Name is the upper-case AutoCAD name.
type Var struct {
	Name string
	Kind VarKind
	// Default is the initial value, of the Go type Kind names (int, float64, string, geom.Vec2).
	Default any
	// Min and Max bound an Int or Real value when Max > Min; otherwise it is unbounded.
	Min, Max float64
	// ReadOnly variables refuse Set; the owner updates them with Vars.Store.
	ReadOnly bool
	// Read, when set, supplies the value instead of the stored one: a live variable that mirrors
	// state kept elsewhere (the session's ortho flag, the current layer).
	Read func() any
	// OnChange runs after a successful Set with the old and new value; a live variable writes
	// its state here.
	OnChange func(old, new any)
}

// Vars is the session's system-variable registry. Every value is session-owned, so two sessions
// never share a default (AutoCAD keeps these in system variables). Packages register their own
// variables, in their own file, through RegisterVarGroup.
type Vars struct {
	defs  map[string]*Var
	vals  map[string]any
	onSet []func(name string, old, new any)
}

// NewVars returns an empty registry.
func NewVars() *Vars { return &Vars{defs: map[string]*Var{}, vals: map[string]any{}} }

// Register adds a variable with its default value. It panics on a duplicate name or a Default of
// the wrong type, so two packages claiming one name fail the first test that builds a session.
func (v *Vars) Register(d Var) {
	d.Name = strings.ToUpper(d.Name)
	if _, dup := v.defs[d.Name]; dup {
		panic("cmdsys: duplicate system variable " + d.Name)
	}
	val, err := coerce(d.Kind, d.Default)
	if err != nil {
		panic(fmt.Sprintf("cmdsys: system variable %s: default %v: %v", d.Name, d.Default, err))
	}
	v.defs[d.Name] = &d
	v.vals[d.Name] = val
}

// Has reports whether name is a registered variable.
func (v *Vars) Has(name string) bool { _, ok := v.defs[strings.ToUpper(name)]; return ok }

// Lookup returns the definition of name.
func (v *Vars) Lookup(name string) (Var, bool) {
	d, ok := v.defs[strings.ToUpper(name)]
	if !ok {
		return Var{}, false
	}
	return *d, true
}

// Names returns every variable name, sorted (SETVAR ? lists them).
func (v *Vars) Names() []string {
	out := make([]string, 0, len(v.defs))
	for n := range v.defs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns the current value of name and whether the variable exists.
func (v *Vars) Get(name string) (any, bool) {
	name = strings.ToUpper(name)
	d, ok := v.defs[name]
	if !ok {
		return nil, false
	}
	if d.Read != nil {
		if val, err := coerce(d.Kind, d.Read()); err == nil {
			return val, true
		}
	}
	return v.vals[name], true
}

// Int returns the value of name as an int; 0 when the variable is missing or not numeric.
func (v *Vars) Int(name string) int {
	x, _ := v.Get(name)
	switch x := x.(type) {
	case int:
		return x
	case float64:
		return int(math.Round(x))
	}
	return 0
}

// Real returns the value of name as a float64; 0 when the variable is missing or not numeric.
func (v *Vars) Real(name string) float64 {
	x, _ := v.Get(name)
	switch x := x.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

// Text returns the value of name as a string; "" when the variable is missing or not text.
func (v *Vars) Text(name string) string {
	s, _ := v.Get(name)
	t, _ := s.(string)
	return t
}

// Point returns the value of name as a point; the origin when the variable is missing or not a
// point.
func (v *Vars) Point(name string) geom.Vec2 {
	x, _ := v.Get(name)
	p, _ := x.(geom.Vec2)
	return p
}

// Set assigns value (an int, float64, string or geom.Vec2 matching the variable's kind; an int
// is accepted for a real) and runs OnChange and the change listeners. It returns ErrVarUnknown,
// ErrVarReadOnly, ErrVarType or ErrVarRange (wrapped with the name) without changing anything.
func (v *Vars) Set(name string, value any) error {
	name = strings.ToUpper(name)
	d, ok := v.defs[name]
	switch {
	case !ok:
		return fmt.Errorf("%w: %s", ErrVarUnknown, name)
	case d.ReadOnly:
		return fmt.Errorf("%w: %s", ErrVarReadOnly, name)
	}
	val, err := coerce(d.Kind, value)
	if err != nil {
		return fmt.Errorf("%w: %s", err, name)
	}
	if err := checkRange(d, val); err != nil {
		return fmt.Errorf("%w: %s", err, name)
	}
	v.store(d, val, true)
	return nil
}

// Store assigns value without the read-only, range and OnChange handling: the owner of a
// read-only variable (DISTANCE, AREA) publishes a result with it. Change listeners still run.
func (v *Vars) Store(name string, value any) error {
	name = strings.ToUpper(name)
	d, ok := v.defs[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrVarUnknown, name)
	}
	val, err := coerce(d.Kind, value)
	if err != nil {
		return fmt.Errorf("%w: %s", err, name)
	}
	v.store(d, val, false)
	return nil
}

func (v *Vars) store(d *Var, val any, callOnChange bool) {
	old, _ := v.Get(d.Name)
	v.vals[d.Name] = val
	if callOnChange && d.OnChange != nil {
		d.OnChange(old, val)
	}
	for _, f := range v.onSet {
		f(d.Name, old, val)
	}
}

// SetString parses text as the variable's kind (a point is "x,y") and assigns it: the entry
// point for SETVAR and a variable typed by name. Parse failures are ErrVarType.
func (v *Vars) SetString(name, text string) error {
	d, ok := v.defs[strings.ToUpper(name)]
	if !ok {
		return fmt.Errorf("%w: %s", ErrVarUnknown, strings.ToUpper(name))
	}
	text = strings.TrimSpace(text)
	var val any
	var err error
	switch d.Kind {
	case VarInt:
		val, err = strconv.Atoi(text)
	case VarReal:
		val, err = strconv.ParseFloat(text, 64)
	case VarText:
		val = text
	case VarPoint:
		x, y, ok := strings.Cut(text, ",")
		var fx, fy float64
		var ex, ey error
		if fx, ex = strconv.ParseFloat(strings.TrimSpace(x), 64); ok {
			fy, ey = strconv.ParseFloat(strings.TrimSpace(strings.SplitN(y, ",", 2)[0]), 64)
		}
		if !ok || ex != nil || ey != nil {
			err = ErrVarType
		}
		val = geom.V(fx, fy)
	}
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVarType, d.Name)
	}
	return v.Set(d.Name, val)
}

// String formats the value of name for SETVAR listings and the "<current>" default: integers
// plain, reals through FormatNumber, text as is, a point as "x,y". It returns "" for an unknown
// variable.
func (v *Vars) String(name string) string {
	x, ok := v.Get(name)
	if !ok {
		return ""
	}
	switch x := x.(type) {
	case int:
		return strconv.Itoa(x)
	case float64:
		return FormatNumber(x)
	case string:
		return x
	case geom.Vec2:
		return FormatPoint(x)
	}
	return fmt.Sprint(x)
}

// Subscribe registers f to run after every change of any variable (Set or Store), with the old
// and new value. The host uses it to refresh the status bar.
func (v *Vars) Subscribe(f func(name string, old, new any)) { v.onSet = append(v.onSet, f) }

func coerce(k VarKind, x any) (any, error) {
	switch k {
	case VarInt:
		switch x := x.(type) {
		case int:
			return x, nil
		case float64:
			if x == math.Trunc(x) && math.Abs(x) < 1<<31 {
				return int(x), nil
			}
		case bool:
			if x {
				return 1, nil
			}
			return 0, nil
		}
	case VarReal:
		switch x := x.(type) {
		case float64:
			if !math.IsNaN(x) && !math.IsInf(x, 0) {
				return x, nil
			}
		case int:
			return float64(x), nil
		}
	case VarText:
		if s, ok := x.(string); ok {
			return s, nil
		}
	case VarPoint:
		if p, ok := x.(geom.Vec2); ok {
			return p, nil
		}
	}
	return nil, ErrVarType
}

func checkRange(d *Var, val any) error {
	if d.Max <= d.Min {
		return nil
	}
	var f float64
	switch x := val.(type) {
	case int:
		f = float64(x)
	case float64:
		f = x
	default:
		return nil
	}
	if f < d.Min || f > d.Max {
		return ErrVarRange
	}
	return nil
}

// VarGroup registers one package's variables on a new session's registry. The session is passed
// so a group can register live variables bound to session state.
type VarGroup func(s *Session, v *Vars)

var varGroups []VarGroup

// RegisterVarGroup adds g to the groups NewSession runs, in registration order. Call it from an
// init function in the file that owns the variables.
func RegisterVarGroup(g VarGroup) { varGroups = append(varGroups, g) }

// Vars returns the session's system variables.
func (s *Session) Vars() *Vars { return s.vars }

// OnVarChange registers f to run after any system variable of the session changes.
func (s *Session) OnVarChange(f func(name string, old, new any)) { s.vars.Subscribe(f) }

// Vars returns the session's system variables.
func (c *Context) Vars() *Vars { return c.vars }
