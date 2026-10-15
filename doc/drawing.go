package doc

import (
	"slices"

	"cad-lite/geom"
)

// Layer is a LAYER table record.
type Layer struct {
	// Name is the layer's key in the table; Drawing.Layer matches it exactly (case-sensitive).
	// It must not change while the layer is in a drawing's table.
	Name string
	// Color is the layer's AutoCAD Color Index (1–255).
	Color int
	// Off hides the layer's entities; they stay selectable by handle.
	Off bool
	// Locked layers display but cannot be selected for editing.
	Locked bool
	// Frozen hides the layer like Off and, in AutoCAD, also leaves it out of regeneration. The
	// field is stored and round-trips in memory only: render, snap and selection do not honour
	// it yet (the layer commands land that).
	Frozen bool
	// NoPlot excludes the layer from plotting (negated so the zero value plots, as new layers do).
	NoPlot bool
	// Linetype is the layer's linetype name; "" means CONTINUOUS.
	Linetype string
	// Lineweight is the layer's lineweight in hundredths of a millimetre; 0 means the default.
	Lineweight int
	// Description is the layer's free-text description.
	Description string
}

// Header holds the drawing's header variables.
type Header struct {
	// HandSeed is the next handle to allocate ($HANDSEED).
	HandSeed Handle
	// InsUnits is the drawing unit code ($INSUNITS; 4 = millimetres).
	InsUnits int
	// Extra keeps header variables the app does not model, verbatim.
	Extra map[string][]string
}

// ModelSpace is the handle of the model-space block record, which owns
// every entity in this model.
const ModelSpace Handle = 0x1F

// Drawing is one open document: header, layer table and the entities of
// model space, plus the undo journal.
type Drawing struct {
	Header Header
	// Preserved is what a file reader kept of tables and blocks the document
	// does not model, for the writer to emit again.
	Preserved Preserved
	// CurrentLayer is the layer new entities go on ($CLAYER).
	CurrentLayer string
	// CurrentColor is the colour new entities get ($CECOLOR): an AutoCAD Color Index (1–255), or
	// 0 or ByLayer for the layer's colour.
	CurrentColor int

	groups map[Handle]*Group
	blocks map[string]*Block
	layers []*Layer
	// layerIndex maps a layer name to its record in layers, so Layer is O(1).
	layerIndex map[string]*Layer
	layerRanks map[string]int
	entities   map[Handle]Entity
	// order holds the handles for Handles. Erasing leaves a handle behind (a tombstone) and
	// re-inserting may append it again; Handles drops both when it next sorts, so erasing is
	// O(1). stale counts the tombstones and duplicates order holds.
	order  []Handle
	sorted bool
	stale  int

	journal   Journal
	listeners []func(ChangeSet)
	open      *Txn
}

// New returns an empty drawing with layer "0" current.
func New() *Drawing {
	d := &Drawing{
		Header:       Header{HandSeed: 0x100, InsUnits: 4},
		CurrentLayer: "0",
		entities:     map[Handle]Entity{},
		groups:       map[Handle]*Group{}, blocks: map[string]*Block{},
		sorted: true,
	}
	d.layers = []*Layer{{Name: "0", Color: 7}}
	d.reindexLayers()
	return d
}

// Layers returns the canonical front-to-back layer order.
// The slice order is read-only; change it through Txn.SetLayerOrder.
func (d *Drawing) Layers() []*Layer { return d.layers }

// Layer returns the named layer, or nil. The name matches exactly; the lookup is a map read,
// cheap enough for render, snap and selection to call per entity.
func (d *Drawing) Layer(name string) *Layer {
	l := d.layerIndex[name]
	if l != nil && l.Name != name {
		// A caller renamed a layer in place; the index is stale, so rebuild it.
		d.reindexLayers()
		l = d.layerIndex[name]
	}
	return l
}

// reindexLayers rebuilds layerIndex from layers. The first layer of a name wins, as the linear
// scan it replaces did.
func (d *Drawing) reindexLayers() {
	d.layerIndex = make(map[string]*Layer, len(d.layers))
	d.layerRanks = make(map[string]int, len(d.layers))
	for i, l := range d.layers {
		if _, ok := d.layerIndex[l.Name]; !ok {
			d.layerIndex[l.Name] = l
			d.layerRanks[l.Name] = i
		}
	}
}

// AddLayer returns the named layer, creating it with color when missing.
// Layer table edits are not journalled.
func (d *Drawing) AddLayer(name string, color int) *Layer {
	if l := d.Layer(name); l != nil {
		return l
	}
	l := &Layer{Name: name, Color: color}
	d.layers = append(d.layers, l)
	d.layerIndex[name] = l
	d.layerRanks[name] = len(d.layers) - 1
	return l
}

// Entity returns the entity with handle h, or nil.
func (d *Drawing) Entity(h Handle) Entity { return d.entities[h] }

// Len returns the number of entities.
func (d *Drawing) Len() int { return len(d.entities) }

// Handles returns the entity handles in ascending storage and spatial-index order.
// Rendering orders candidates by layer, then ascending handle within each layer. The slice is shared; do not modify it, and
// do not keep it across an edit.
func (d *Drawing) Handles() []Handle {
	if d.stale > 0 {
		d.compact()
	}
	if !d.sorted {
		slices.Sort(d.order)
		d.sorted = true
	}
	return d.order
}

// compact replaces order with a sorted copy free of the tombstones and
// duplicates erasing and re-inserting left in it. It copies rather than
// filtering in place, so a slice an earlier Handles returned is left intact.
func (d *Drawing) compact() {
	all := slices.Clone(d.order)
	slices.Sort(all)
	out := make([]Handle, 0, len(d.entities))
	for i, h := range all {
		if _, live := d.entities[h]; live && (i == 0 || all[i-1] != h) {
			out = append(out, h)
		}
	}
	d.order = out
	d.sorted = true
	d.stale = 0
}

// Extents returns the bounding box of every displayed entity, or an empty
// box. Unknown entities are not displayed and do not count.
func (d *Drawing) Extents() geom.Box2 {
	b := geom.EmptyBox()
	for _, e := range d.entities {
		if _, ok := e.(*Unknown); ok {
			continue
		}
		if in, ok := e.(*Insert); ok {
			b = b.Union(in.VisibleBounds())
		} else {
			b = b.Union(e.Bounds())
		}
	}
	return b
}

// OnChange registers f to receive every committed, undone or redone change
// set.
func (d *Drawing) OnChange(f func(ChangeSet)) { d.listeners = append(d.listeners, f) }

// Journal returns the undo journal.
func (d *Drawing) Journal() *Journal { return &d.journal }

func (d *Drawing) allocHandle() Handle {
	h := d.Header.HandSeed
	d.Header.HandSeed++
	return h
}

// insert stores e (whose handle is set) without journalling.
func (d *Drawing) insert(e Entity) {
	h := e.Handle()
	if _, ok := d.entities[h]; !ok {
		d.order = append(d.order, h)
		d.sorted = false
		// A handle erased earlier may still sit in order as a tombstone: the
		// new entry is then a duplicate for compact to drop.
		if d.stale > 0 {
			d.stale++
		}
	}
	d.entities[h] = e
	d.bindEntity(e)
	if h >= d.Header.HandSeed {
		d.Header.HandSeed = h + 1
	}
}

// remove deletes the entity with handle h without journalling. Its handle
// stays in order as a tombstone until Handles compacts it; a drawing that
// is edited without being drawn compacts once tombstones outnumber the
// live entities.
func (d *Drawing) remove(h Handle) {
	if _, ok := d.entities[h]; !ok {
		return
	}
	delete(d.entities, h)
	d.stale++
	if d.stale > len(d.entities)+64 {
		d.compact()
	}
}

// Load inserts entities read from a file, keeping their handles, outside
// any transaction and journal. It is for file readers.
func (d *Drawing) Load(e Entity) {
	if e.Handle() == 0 {
		e.Common().H = d.allocHandle()
	}
	if e.OwnerHandle() == 0 {
		e.Common().Owner = ModelSpace
	}
	d.insert(e)
}

func (d *Drawing) publish(cs ChangeSet) {
	if cs.Empty() {
		return
	}
	for _, f := range d.listeners {
		f(cs)
	}
}
