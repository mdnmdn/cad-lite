package doc

import (
	"cad-lite/geom"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Block is a shared named definition with geometry in definition coordinates.
type Block struct {
	// H is the persistent definition handle.
	H Handle
	// Name identifies this definition without regard to case.
	Name string
	// Base is the placement origin in definition coordinates.
	Base geom.Vec2
	// Entities are definition-owned objects outside model space.
	Entities []Entity
}

func (b *Block) clone() *Block {
	c := *b
	c.Entities = make([]Entity, len(b.Entities))
	for i, e := range b.Entities {
		if e != nil {
			c.Entities[i] = e.Clone().(Entity)
		}
	}
	return &c
}
func cloneBlocks(bs map[string]*Block) map[string]*Block {
	out := map[string]*Block{}
	for n, b := range bs {
		out[n] = b.clone()
	}
	return out
}

// Blocks returns deep copies of definitions in name order.
func (d *Drawing) Blocks() []*Block {
	var ns []string
	for n := range d.blocks {
		ns = append(ns, n)
	}
	slices.Sort(ns)
	var out []*Block
	for _, n := range ns {
		out = append(out, d.blocks[n].clone())
	}
	return out
}

// Block returns a deep copy of a definition by case-insensitive name.
func (d *Drawing) Block(name string) *Block {
	if b := d.blocks[strings.ToUpper(name)]; b != nil {
		return b.clone()
	}
	return nil
}

// DefineBlock validates and creates or replaces a shared definition.
func (t *Txn) DefineBlock(b *Block) error {
	if b == nil {
		return errors.New("invalid block geometry")
	}
	c := b.clone()
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || strings.ContainsAny(c.Name, "<>/\\\":;?*|,=") {
		return errors.New("invalid block name")
	}
	if !finitePoint(c.Base) {
		return errors.New("invalid block base point")
	}
	key := strings.ToUpper(c.Name)
	proposed := make(map[string]*Block, len(t.d.blocks)+1)
	for n, b := range t.d.blocks {
		proposed[n] = b
	}
	proposed[key] = c
	affected := map[string]bool{key: true}
	changed := true
	for changed {
		changed = false
		for name, b := range proposed {
			if affected[name] {
				continue
			}
			for _, e := range b.Entities {
				if in, ok := e.(*Insert); ok && affected[strings.ToUpper(in.BlockName)] {
					affected[name] = true
					changed = true
					break
				}
			}
		}
	}
	for name := range affected {
		if err := validateBlockGraph(proposed, name); err != nil {
			return err
		}
	}
	for _, e := range t.d.entities {
		if in, ok := e.(*Insert); ok && affected[strings.ToUpper(in.BlockName)] {
			if _, err := referenceAttributeLimit(proposed, in); err != nil {
				return err
			}
		}
	}
	t.touchBlocks()
	if old := t.d.blocks[key]; old != nil {
		c.H = old.H
	} else {
		c.H = t.d.allocHandle()
	}
	for _, e := range c.Entities {
		e.Common().H = t.d.allocHandle()
		e.Common().Owner = c.H
		if in, ok := e.(*Insert); ok {
			for i := range in.Attributes {
				if in.Attributes[i].Text != nil {
					in.Attributes[i].Text.H = t.d.allocHandle()
					in.Attributes[i].Text.Owner = in.H
				}
			}
		}
		if e.Common().Layer == "" {
			e.Common().Layer = "0"
		}

	}
	t.d.blocks[key] = c
	t.d.bindBlocks()
	t.invalidateInserts()
	return nil
}

// EraseBlock removes an unreferenced definition.
func (t *Txn) EraseBlock(name string) error {
	key := strings.ToUpper(name)
	for _, b := range t.d.blocks {
		for _, e := range b.Entities {
			if in, ok := e.(*Insert); ok && strings.EqualFold(in.BlockName, name) {
				return errors.New("block is referenced")
			}
		}
	}
	for _, e := range t.d.entities {
		if in, ok := e.(*Insert); ok && strings.EqualFold(in.BlockName, name) {
			return errors.New("block is referenced")
		}
	}
	if t.d.blocks[key] != nil {
		t.touchBlocks()
		delete(t.d.blocks, key)
		t.invalidateInserts()
	}
	return nil
}

// LoadBlock installs a file definition without a transaction, retaining handles.
func (d *Drawing) LoadBlock(b *Block) {
	c := b.clone()
	if c.H == 0 {
		c.H = d.allocHandle()
	}
	if c.H >= d.Header.HandSeed {
		d.Header.HandSeed = c.H + 1
	}
	for _, e := range c.Entities {
		if e.Handle() == 0 {
			e.Common().H = d.allocHandle()
		}
		if e.Handle() >= d.Header.HandSeed {
			d.Header.HandSeed = e.Handle() + 1
		}
		e.Common().Owner = c.H
	}
	d.blocks[strings.ToUpper(c.Name)] = c
	d.bindBlocks()
}
func (d *Drawing) bindBlocks() {
	for _, b := range d.blocks {
		for _, e := range b.Entities {
			d.bindEntity(e)
		}
	}
	for _, e := range d.entities {
		d.bindEntity(e)
	}
}
func (d *Drawing) bindEntity(e Entity) {
	if in, ok := e.(*Insert); ok {
		in.d = d
	}
}
func (t *Txn) touchBlocks() {
	if t.blocksBefore == nil {
		t.blocksBefore = cloneBlocks(t.d.blocks)
	}
}
func (t *Txn) invalidateInserts() {
	for h, e := range t.d.entities {
		if _, ok := e.(*Insert); ok {
			t.edits = append(t.edits, h)
		}
	}
}
func (d *Drawing) insertHandles() []Handle {
	var out []Handle
	for h, e := range d.entities {
		if _, ok := e.(*Insert); ok {
			out = append(out, h)
		}
	}
	slices.Sort(out)
	return out
}
func finitePoint(p geom.Vec2) bool {
	return !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0) && !math.IsNaN(p.X) && !math.IsNaN(p.Y)
}

// MaxBlockDepth bounds nested block resolution.
const MaxBlockDepth = 32

// MaxBlockEntities bounds the expanded entity count of one reference.
const MaxBlockEntities = 100000

// BlockError reports missing, cyclic or over-budget definition dependencies.
func (d *Drawing) BlockError(name string) error {
	return validateBlockGraph(d.blocks, strings.ToUpper(name))
}

// InsertError validates a root reference's transform, dependencies and total expansion budget.
// Root attributes and every visited definition entity share MaxBlockEntities.
func (d *Drawing) InsertError(e *Insert) error { _, err := d.rootAttributeLimit(e); return err }
func (d *Drawing) rootAttributeLimit(e *Insert) (int, error) {
	return referenceAttributeLimit(d.blocks, e)
}
func referenceAttributeLimit(blocks map[string]*Block, e *Insert) (int, error) {
	if e == nil || !e.valid() {
		return 0, errors.New("invalid insertion transform")
	}
	if len(e.Attributes) >= MaxBlockEntities {
		return 0, errors.New("block expansion limit exceeded")
	}
	count, err := countBlockGraph(blocks, strings.ToUpper(e.BlockName))
	if err != nil {
		return 0, err
	}
	limit := MaxBlockEntities - count
	if len(e.Attributes) > limit {
		return 0, errors.New("block expansion limit exceeded")
	}
	return limit, nil
}
func validateBlockGraph(blocks map[string]*Block, root string) error {
	_, err := countBlockGraph(blocks, root)
	return err
}
func countBlockGraph(blocks map[string]*Block, root string) (int, error) {
	type info struct{ depth, count int }
	memo := map[string]info{}
	visiting := map[string]bool{}
	var visit func(string) (info, error)
	visit = func(name string) (info, error) {
		if v, ok := memo[name]; ok {
			return v, nil
		}
		if len(visiting) >= MaxBlockDepth {
			return info{}, errors.New("block expansion limit exceeded")
		}
		if visiting[name] {
			return info{}, errors.New("cyclic block definition")
		}
		b := blocks[name]
		if b == nil {
			return info{}, fmt.Errorf("block %s does not exist", name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		v := info{depth: 1, count: 1}
		tags := map[string]bool{}
		for _, e := range b.Entities {
			if e == nil {
				return info{}, errors.New("invalid block geometry")
			}
			if a, ok := e.(*AttributeDefinition); ok {
				tag := strings.ToUpper(strings.TrimSpace(a.Tag))
				if tag == "" || tags[tag] {
					return info{}, errors.New("invalid attribute tag")
				}
				tags[tag] = true
			}
			if i, ok := e.(*Insert); ok {
				if !i.valid() {
					return info{}, errors.New("invalid insertion transform")
				}
				child, err := visit(strings.ToUpper(i.BlockName))
				if err != nil {
					return info{}, err
				}
				v.depth = max(v.depth, child.depth+1)
				if child.count > MaxBlockEntities-v.count || len(i.Attributes) > MaxBlockEntities-v.count-child.count {
					return info{}, errors.New("block expansion limit exceeded")
				}
				v.count += child.count + len(i.Attributes)
			} else {
				if _, unknown := e.(*Unknown); !unknown {
					box := e.Bounds()
					if !box.IsEmpty() && (!finitePoint(box.Min) || !finitePoint(box.Max)) {
						return info{}, errors.New("invalid block geometry")
					}
				}
				v.count++
			}
			if v.depth > MaxBlockDepth || v.count > MaxBlockEntities {
				return info{}, errors.New("block expansion limit exceeded")
			}
		}
		memo[name] = v
		return v, nil
	}
	result, err := visit(root)
	return result.count, err
}

type blockSupport struct {
	layers    []*Layer
	preserved Preserved
}

func (d *Drawing) snapshotSupport() *blockSupport {
	s := &blockSupport{preserved: d.Preserved}
	for _, l := range d.layers {
		c := *l
		s.layers = append(s.layers, &c)
	}
	s.preserved.Tables = map[string][]RawRecord{}
	for table, records := range d.Preserved.Tables {
		for _, r := range records {
			s.preserved.Tables[table] = append(s.preserved.Tables[table], RawRecord{Pairs: append([]Pair(nil), r.Pairs...)})
		}
	}
	return s
}
func (d *Drawing) restoreSupport(target, previous *blockSupport) {
	if target == nil || previous == nil {
		return
	}
	before, after := map[string]*Layer{}, map[string]*Layer{}
	for _, l := range target.layers {
		before[l.Name] = l
	}
	for _, l := range previous.layers {
		after[l.Name] = l
	}
	names := map[string]bool{}
	for n := range before {
		names[n] = true
	}
	for n := range after {
		names[n] = true
	}
	for name := range names {
		a, b := before[name], after[name]
		if a != nil && b != nil && *a == *b {
			continue
		}
		if a == nil {
			d.layers = slices.DeleteFunc(d.layers, func(l *Layer) bool { return l.Name == name })
		} else if l := d.Layer(name); l != nil {
			*l = *a
		} else {
			c := *a
			d.layers = append(d.layers, &c)
		}
	}
	d.reindexLayers()
	if d.Layer(d.CurrentLayer) == nil {
		d.CurrentLayer = "0"
	}
	if d.Preserved.Tables == nil {
		d.Preserved.Tables = map[string][]RawRecord{}
	}
	for _, table := range []string{"STYLE", "LTYPE"} {
		a, b := map[string]RawRecord{}, map[string]RawRecord{}
		for _, r := range target.preserved.Tables[table] {
			a[strings.ToUpper(r.Name())] = r
		}
		for _, r := range previous.preserved.Tables[table] {
			b[strings.ToUpper(r.Name())] = r
		}
		keys := map[string]bool{}
		for n := range a {
			keys[n] = true
		}
		for n := range b {
			keys[n] = true
		}
		for n := range keys {
			want, had := a[n], b[n]
			if slices.Equal(want.Pairs, had.Pairs) {
				continue
			}
			rs := d.Preserved.Tables[table]
			rs = slices.DeleteFunc(rs, func(r RawRecord) bool { return strings.EqualFold(r.Name(), n) })
			if len(want.Pairs) > 0 {
				rs = append(rs, RawRecord{Pairs: append([]Pair(nil), want.Pairs...)})
			}
			d.Preserved.Tables[table] = rs
		}
	}
}

// ImportBlockSupport imports missing layers and STYLE/LTYPE records in this transaction.
// Imported handles and cross-record pointers are remapped into the destination drawing.
func (t *Txn) ImportBlockSupport(src *Drawing) {
	changed := false
	for _, l := range src.Layers() {
		if t.d.Layer(l.Name) == nil {
			changed = true
		}
	}
	for _, table := range []string{"STYLE", "LTYPE"} {
		for _, r := range src.Preserved.Tables[table] {
			found := false
			for _, old := range t.d.Preserved.Tables[table] {
				if strings.EqualFold(old.Name(), r.Name()) {
					found = true
					break
				}
			}
			if !found {
				changed = true
			}
		}
	}
	if !changed {
		return
	}

	if t.supportBefore == nil {
		t.supportBefore = t.d.snapshotSupport()
	}
	for _, l := range src.Layers() {
		if t.d.Layer(l.Name) == nil {
			*t.d.AddLayer(l.Name, l.Color) = *l
		}
	}
	if t.d.Preserved.Tables == nil {
		t.d.Preserved.Tables = map[string][]RawRecord{}
	}
	remap := map[Handle]Handle{}
	type addition struct {
		table  string
		record RawRecord
		h      Handle
	}
	var add []addition
	for _, name := range []string{"STYLE", "LTYPE"} {
		for _, r := range src.Preserved.Tables[name] {
			h := Handle(0)
			for _, p := range r.Pairs {
				if p.Code == 5 || p.Code == 105 {
					h, _ = ParseHandle(p.Value)
					break
				}
			}
			var existing *RawRecord
			for i := range t.d.Preserved.Tables[name] {
				if strings.EqualFold(t.d.Preserved.Tables[name][i].Name(), r.Name()) {
					existing = &t.d.Preserved.Tables[name][i]
					break
				}
			}
			if existing != nil {
				var target Handle
				for _, p := range existing.Pairs {
					if p.Code == 5 || p.Code == 105 {
						target, _ = ParseHandle(p.Value)
					}
				}
				if target == 0 {
					target = t.d.allocHandle()
					existing.Pairs = append(existing.Pairs[:1], append([]Pair{{Code: 5, Value: target.String()}}, existing.Pairs[1:]...)...)
				}
				if h != 0 {
					remap[h] = target
				}
				continue
			}
			target := t.d.allocHandle()
			if h != 0 {
				remap[h] = target
			}
			add = append(add, addition{name, r, target})
		}
	}
	for _, a := range add {
		var pairs []Pair
		for i, p := range a.record.Pairs {
			if p.Code == 5 || p.Code == 105 || p.Code == 330 {
				continue
			}
			if p.Code >= 320 && p.Code <= 369 {
				h, _ := ParseHandle(p.Value)
				if n := remap[h]; n != 0 {
					p.Value = n.String()
				}
			}
			pairs = append(pairs, p)
			if i == 0 {
				pairs = append(pairs, Pair{Code: 5, Value: a.h.String()})
			}
		}
		t.d.Preserved.Tables[a.table] = append(t.d.Preserved.Tables[a.table], RawRecord{Pairs: pairs})
	}
	t.invalidateInserts()
}
