package cadmcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// MaxBatchOps caps the operations of one batch call.
const MaxBatchOps = 500

// MaxBatchPoints caps the polyline vertices of one batch call, summed over its add_polyline ops.
const MaxBatchPoints = 200000

// batchRef prefixes a client id where a handle is expected: "$door1" is the entity created by the
// op of this batch whose id is "door1". Hexadecimal handles never start with it.
const batchRef = "$"

var clientID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// opSpec is one operation batch accepts: the single tool's schema and its shared builder. Exactly
// one of add and edit is set.
type opSpec struct {
	schema string
	// add decodes the arguments and builds the entity and its layer.
	add func(raw json.RawMessage, required []string) (doc.Entity, string, error)
	// edit decodes the arguments and builds the edit and its raw handles.
	edit func(raw json.RawMessage, required []string) (editOp, []string, error)
}

func addSpec[A any](schema string, build func(A) (doc.Entity, error), layer func(A) string) opSpec {
	return opSpec{schema: schema, add: func(raw json.RawMessage, required []string) (doc.Entity, string, error) {
		var a A
		if err := decodeArgs(raw, &a, required); err != nil {
			return nil, "", fmt.Errorf("invalid arguments: %w", err)
		}
		e, err := build(a)
		return e, layer(a), err
	}}
}

func editSpec[A any](schema string, build func(A) (editOp, error), handles func(A) []string) opSpec {
	return opSpec{schema: schema, edit: func(raw json.RawMessage, required []string) (editOp, []string, error) {
		var a A
		if err := decodeArgs(raw, &a, required); err != nil {
			return editOp{}, nil, fmt.Errorf("invalid arguments: %w", err)
		}
		op, err := build(a)
		return op, handles(a), err
	}}
}

// batchOps lists the operations batch accepts, named after the single tools they mirror.
var batchOps = map[string]opSpec{
	"add_line":         addSpec(schemaAddLine, lineEntity, func(a lineArgs) string { return a.Layer }),
	"add_circle":       addSpec(schemaAddCircle, circleEntity, func(a circleArgs) string { return a.Layer }),
	"add_arc":          addSpec(schemaAddArc, arcEntity, func(a arcArgs) string { return a.Layer }),
	"add_polyline":     addSpec(schemaAddPolyline, polylineEntity, func(a polylineArgs) string { return a.Layer }),
	"add_rectangle":    addSpec(schemaAddRect, rectEntity, func(a rectArgs) string { return a.Layer }),
	"add_polygon":      addSpec(schemaAddPolygon, polygonEntity, func(a polygonArgs) string { return a.Layer }),
	"add_text":         addSpec(schemaAddText, textEntity, func(a textArgs) string { return a.Layer }),
	"add_mtext":        addSpec(schemaAddMText, mtextEntity, func(a mtextArgs) string { return a.Layer }),
	"update_text":      editSpec(schemaUpdateText, updateTextEdit, func(a updateTextArgs) []string { return []string{a.Handle} }),
	"erase":            editSpec(schemaHandles, eraseEdit, func(a handlesArgs) []string { return a.Handles }),
	"move":             editSpec(schemaMove, moveEdit, func(a moveArgs) []string { return a.Handles }),
	"rotate":           editSpec(schemaRotate, rotateEdit, func(a rotateArgs) []string { return a.Handles }),
	"scale":            editSpec(schemaScale, scaleEdit, func(a scaleArgs) []string { return a.Handles }),
	"set_entity_layer": editSpec(schemaEntityLayer, entityLayerEdit, func(a entityLayerArgs) []string { return a.Handles }),
}

var batchOpNames = []string{"add_line", "add_circle", "add_arc", "add_polyline", "add_rectangle", "add_polygon", "add_text", "add_mtext", "erase", "move", "rotate", "scale", "set_entity_layer", "update_text"}

var schemaBatch = `{"type":"object","properties":{"ops":{"type":"array","minItems":1,"maxItems":` + fmt.Sprint(MaxBatchOps) +
	`,"items":{"type":"object","properties":{` +
	`"op":{"type":"string","enum":["` + strings.Join(batchOpNames, `","`) + `"]},` +
	`"id":{"type":"string","pattern":"^[A-Za-z0-9_.-]{1,64}$","description":"client id, add_* ops only; later ops name the new entity as \"$<id>\" in handles, and the result maps it to its handle"},` +
	`"args":{"type":"object","description":"the arguments of the single tool of the same name"}` +
	`},"required":["op","args"],"additionalProperties":false}}},"required":["ops"],"additionalProperties":false}`

// batchArgs decodes and checks the ops on the MCP handler's goroutine, so the UI goroutine only
// resolves layers and handles and applies the plan.
type batchArgs struct {
	Ops  []batchOp
	plan []plannedOp
}

type batchOp struct {
	Op   string
	ID   string `json:"id"`
	Args json.RawMessage
}

// UnmarshalJSON implements json.Unmarshaler: it decodes the ops strictly, then validates every op
// that does not need the drawing (op name, client ids, arguments, numbers, references to earlier
// ops) and builds the entities to add.
func (a *batchArgs) UnmarshalJSON(b []byte) error {
	var in struct{ Ops []batchOp }
	if err := decodeArgs(b, &in, []string{"ops"}); err != nil {
		return err
	}
	plan, err := decodeBatch(in.Ops)
	if err != nil {
		return err
	}
	a.Ops, a.plan = in.Ops, plan
	return nil
}

// target is one entity an edit op names: an existing handle, or the entity op ref of this batch
// creates (ref >= 0).
type target struct {
	h   doc.Handle
	ref int
}

type plannedOp struct {
	entity  doc.Entity
	layer   string
	edit    editOp
	targets []target
	// raw holds the handle strings, parallel to targets.
	raw []string
	// resolved is targets with existing handles filled in and repeats dropped, set by resolveBatch.
	resolved []target
}

type batchCreated struct {
	created
	ID string `json:"id,omitempty"`
}

type batchResult struct {
	Ops      int               `json:"ops"`
	Handles  map[string]string `json:"handles"`
	Created  []batchCreated    `json:"created"`
	Modified []string          `json:"modified"`
	Erased   []string          `json:"erased"`
}

// opError names the failing op of a batch.
func opError(i int, o batchOp, err error) error {
	name := o.Op
	if o.ID != "" {
		name += " " + batchRef + o.ID
	}
	return fmt.Errorf("ops[%d] (%s): %v; nothing was changed", i, name, err)
}

// runBatch validates every op, then applies them all in one transaction, which is one undo step.
// Any failure aborts the transaction, so the drawing, its handle seed and the undo history stay as
// they were.
func runBatch(s *cmdsys.Session, a batchArgs) (any, error) {
	plan := a.plan
	if plan == nil { // not built by UnmarshalJSON
		var err error
		if plan, err = decodeBatch(a.Ops); err != nil {
			return nil, err
		}
	}
	// Check first, so a refused batch leaves a running command alone; cancelling it (idle) may
	// change the drawing, so check again after.
	if err := resolveBatch(s, a.Ops, plan); err != nil {
		return nil, err
	}
	if idle(s) != "" {
		if err := resolveBatch(s, a.Ops, plan); err != nil {
			return nil, err
		}
	}
	d := s.Drawing()
	seed := d.Header.HandSeed
	committed := false
	defer func() { // also on a panic while applying
		if !committed {
			d.Header.HandSeed = seed
		}
	}()
	made := make([]doc.Handle, len(plan))
	cs, err := withTxChanges(s, func(tx *doc.Txn) error {
		for i, p := range plan {
			if p.entity != nil {
				p.entity.Common().Layer = p.layer
				made[i] = tx.Add(p.entity)
				if err := checkBounds(d, made[i]); err != nil {
					return opError(i, a.Ops[i], err)
				}
				continue
			}
			hs := make([]doc.Handle, 0, len(p.resolved))
			seen := make(map[doc.Handle]bool, len(p.resolved))
			for _, t := range p.resolved {
				h := t.h
				if t.ref >= 0 {
					h = made[t.ref]
				}
				if d.Entity(h) == nil {
					return opError(i, a.Ops[i], fmt.Errorf("entity %s no longer exists", h))
				}
				if err := checkEditable(d, h); err != nil {
					return opError(i, a.Ops[i], err)
				}
				if !seen[h] {
					seen[h] = true
					hs = append(hs, h)
				}
			}
			if err := p.edit.apply(tx, hs); err != nil {
				return opError(i, a.Ops[i], err)
			}
			if err := checkBounds(d, hs...); err != nil {
				return opError(i, a.Ops[i], err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	committed = true
	res := batchResult{Ops: len(plan), Handles: map[string]string{}, Created: []batchCreated{}, Modified: handleStrings(cs.Modified), Erased: handleStrings(cs.Erased)}
	ids := map[doc.Handle]string{}
	for i, h := range made {
		if h != 0 && a.Ops[i].ID != "" && d.Entity(h) != nil {
			res.Handles[a.Ops[i].ID] = h.String()
			ids[h] = a.Ops[i].ID
		}
	}
	for _, h := range cs.Added {
		e := d.Entity(h)
		res.Created = append(res.Created, batchCreated{created{Handle: h.String(), Class: e.Class(), Layer: e.Common().Layer}, ids[h]})
	}
	return res, nil
}

// decodeBatch checks what needs no drawing: the op count, op names, client ids, each op's
// arguments through the single tool's builder, the point cap, and that every "$id" names an
// earlier add_* op.
func decodeBatch(ops []batchOp) ([]plannedOp, error) {
	if len(ops) == 0 {
		return nil, errors.New("ops must not be empty")
	}
	if len(ops) > MaxBatchOps {
		return nil, fmt.Errorf("too many ops: %d (max %d per batch; split the work)", len(ops), MaxBatchOps)
	}
	ids := map[string]int{}
	plan := make([]plannedOp, len(ops))
	points := 0
	for i, o := range ops {
		spec, ok := batchOps[o.Op]
		if !ok {
			return nil, opError(i, o, fmt.Errorf("unknown op %q (want one of %s)", o.Op, strings.Join(batchOpNames, ", ")))
		}
		if o.ID != "" {
			if spec.add == nil {
				return nil, opError(i, o, errors.New("id is only for add_* ops"))
			}
			if !clientID.MatchString(o.ID) {
				return nil, opError(i, o, fmt.Errorf("id %q must be 1-64 letters, digits, '_', '.' or '-'", o.ID))
			}
			if j, dup := ids[o.ID]; dup {
				return nil, opError(i, o, fmt.Errorf("id %q is already used by ops[%d]", o.ID, j))
			}
		}
		required := requiredKeys(spec.schema)
		if spec.add != nil {
			e, layer, err := spec.add(o.Args, required)
			if err != nil {
				return nil, opError(i, o, err)
			}
			if pl, ok := e.(*doc.LWPolyline); ok {
				if points += len(pl.Points); points > MaxBatchPoints {
					return nil, opError(i, o, fmt.Errorf("the batch has more than %d polyline points; split it", MaxBatchPoints))
				}
			}
			plan[i] = plannedOp{entity: e, layer: layer}
			if o.ID != "" {
				ids[o.ID] = i
			}
			continue
		}
		op, raw, err := spec.edit(o.Args, required)
		if err == nil && len(raw) == 0 {
			err = errors.New("handles must not be empty")
		}
		if err != nil {
			return nil, opError(i, o, err)
		}
		plan[i].edit = op
		for _, str := range raw {
			t := target{ref: -1}
			if id, isRef := strings.CutPrefix(strings.TrimSpace(str), batchRef); isRef {
				j, known := ids[id]
				if !known {
					later := ""
					for k := i + 1; k < len(ops); k++ {
						if ops[k].ID == id {
							later = fmt.Sprintf(" before ops[%d] creates it", k)
						}
					}
					return nil, opError(i, o, fmt.Errorf("%q names no earlier add_* op%s", str, later))
				}
				t.ref = j
			}
			plan[i].raw = append(plan[i].raw, str)
			plan[i].targets = append(plan[i].targets, t)
		}
	}
	return plan, nil
}

// resolveBatch checks what needs the drawing, without changing it: layers exist, each handle
// names an entity a tool can act on, no earlier op of the batch erased it, and no edit touches an
// entity on a locked layer, following the layer changes of earlier ops.
func resolveBatch(s *cmdsys.Session, ops []batchOp, plan []plannedOp) error {
	d := s.Drawing()
	erasedBy := map[target]int{}
	layerOf := map[target]string{}
	styles := &styleResolver{d: d}
	// texts holds a copy of each text an update_text names, updated op by op, so every update is
	// checked against the text as earlier ops of the batch leave it.
	texts := map[target]doc.Entity{}
	for i := range plan {
		p := &plan[i]
		layer := p.layer
		if p.entity == nil {
			layer = p.edit.layer
		}
		if err := checkLayer(s, layer); err != nil {
			return opError(i, ops[i], err)
		}
		if err := resolveStyles(styles, p.entity, p.edit); err != nil {
			return opError(i, ops[i], err)
		}
		if p.entity != nil {
			continue
		}
		resolved := make([]target, 0, len(p.targets))
		seen := make(map[target]bool, len(p.targets))
		for k, t := range p.targets {
			if t.ref < 0 {
				h, err := existingHandle(d, p.raw[k])
				if err != nil {
					return opError(i, ops[i], err)
				}
				t.h = h
			}
			if j, gone := erasedBy[t]; gone {
				return opError(i, ops[i], fmt.Errorf("%s was erased by ops[%d]", p.raw[k], j))
			}
			on, known := layerOf[t]
			switch {
			case known:
			case t.ref >= 0:
				if on = plan[t.ref].layer; on == "" {
					on = d.CurrentLayer
				}
			default:
				on = d.Entity(t.h).Common().Layer
			}
			if l := d.Layer(on); l != nil && l.Locked {
				return opError(i, ops[i], fmt.Errorf("entity %s is on locked layer %q", p.raw[k], l.Name))
			}
			if p.edit.layer != "" {
				on = p.edit.layer
			}
			layerOf[t] = on
			if p.edit.text != nil {
				sim, ok := texts[t]
				if !ok {
					if t.ref >= 0 {
						sim = plan[t.ref].entity.Clone().(doc.Entity)
					} else {
						sim = d.Entity(t.h).Clone().(doc.Entity)
					}
					texts[t] = sim
				}
				if err := p.edit.text.apply(sim); err != nil {
					return opError(i, ops[i], fmt.Errorf("%s: %w", p.raw[k], err))
				}
			}
			if !seen[t] {
				seen[t] = true
				resolved = append(resolved, t)
			}
		}
		p.resolved = resolved
		if p.edit.erase {
			for _, t := range resolved {
				erasedBy[t] = i
			}
		}
	}
	return nil
}
