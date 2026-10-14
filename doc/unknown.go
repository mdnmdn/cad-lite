package doc

import (
	"strconv"
	"strings"

	"go-draw/geom"
)

// Pair is one DXF group: a group code and its value text, kept exactly as
// read (leading and trailing blanks included).
type Pair struct {
	Code  int
	Value string
}

// Unknown is an entity of a class the app does not model: a SPLINE, a
// vertical-application object, a 3D polyline. It is kept so a file can be
// written back without losing it. It is never drawn, picked, snapped to,
// moved or erased; only a file reader creates one (see Drawing.Load).
//
// Raw holds every group of the entity record after its "0 CLASS" line, in
// file order, including the groups also stored in EntityCommon (handle 5,
// layer 8, colour 62, owner 330). The common fields are a parsed view of
// Raw for lookups, not a second source of truth: the writer emits Raw. An
// entity that owns sub-records (INSERT with attributes, a 3D POLYLINE with
// its VERTEX records) keeps them in Raw as ordinary groups, so each
// sub-record starts with a code 0 pair and the list ends with its SEQEND.
type Unknown struct {
	EntityCommon
	// Type is the DXF class name ("SPLINE", "ACAD_PROXY_ENTITY"...).
	Type string
	// Raw is the entity's group list, verbatim.
	Raw []Pair
}

// Class implements Object.
func (e *Unknown) Class() string { return e.Type }

// Clone implements Object.
func (e *Unknown) Clone() Object {
	c := *e
	c.Raw = append([]Pair(nil), e.Raw...)
	return &c
}

// Bounds implements Entity. It is the box of the point groups 10/20 to
// 18/28 of the entity's own record (not of its sub-records), or an empty
// box when it has none. It is a hint for listing and indexing: some classes
// store a direction or a scale in those codes, and an entity is never drawn,
// so Drawing.Extents ignores it.
func (e *Unknown) Bounds() geom.Box2 {
	b := geom.EmptyBox()
	var xs [9]float64
	var have [9]bool
	for _, p := range e.Raw {
		if p.Code == 0 {
			break
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64)
		switch {
		case err != nil:
		case p.Code >= 10 && p.Code <= 18:
			xs[p.Code-10], have[p.Code-10] = v, true
		case p.Code >= 20 && p.Code <= 28 && have[p.Code-20]:
			b = b.Extend(geom.V(xs[p.Code-20], v))
			have[p.Code-20] = false
		}
	}
	return b
}

// Geometry implements Entity. It is always empty: the entity has no curves
// to snap to, pick or draw.
func (*Unknown) Geometry() []geom.Curve { return nil }

// Transform implements Entity. It does nothing: the entity is not editable.
func (*Unknown) Transform(geom.Mat3) {}

// Source returns the entity as a record: its class followed by Raw, ready to
// be written group by group.
func (e *Unknown) Source() []Pair {
	return append([]Pair{{0, e.Type}}, e.Raw...)
}

// RawRecord is a table record or a block the document does not model, kept
// verbatim: the groups from its "0 TYPE" line (code 0 first) to the group
// before the next record. A block keeps its entities and its ENDBLK as
// further code 0 groups.
type RawRecord struct {
	Pairs []Pair
}

// Type returns the record's class, the value of its leading code 0 group.
func (r RawRecord) Type() string {
	if len(r.Pairs) == 0 {
		return ""
	}
	return strings.TrimSpace(r.Pairs[0].Value)
}

// Name returns the record's name (group code 2).
func (r RawRecord) Name() string {
	for _, p := range r.Pairs[min(1, len(r.Pairs)):] {
		if p.Code == 2 {
			return strings.TrimSpace(p.Value)
		}
		if p.Code == 0 {
			break
		}
	}
	return ""
}

// Preserved holds the parts of a file the document does not model, so a save
// can write them back. Tables maps a table name ("LTYPE", "STYLE", "APPID",
// "DIMSTYLE", "VIEW", "UCS", "VPORT") to its records in file order, without
// the record the writer always generates (the CONTINUOUS linetype). Blocks
// lists every block definition except the model- and paper-space blocks. The
// user cannot change either.
type Preserved struct {
	Tables map[string][]RawRecord
	Blocks []RawRecord
}

// Empty reports whether nothing is preserved.
func (p *Preserved) Empty() bool {
	return len(p.Blocks) == 0 && len(p.Tables) == 0
}
