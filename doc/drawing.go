package doc

import (
	"slices"

	"go-draw/geom"
)

// Layer is a LAYER table record.
type Layer struct {
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

	layers   []*Layer
	entities map[Handle]Entity
	order    []Handle
	sorted   bool

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
		sorted:       true,
	}
	d.layers = []*Layer{{Name: "0", Color: 7}}
	return d
}

// Layers returns the layer table in definition order.
func (d *Drawing) Layers() []*Layer { return d.layers }

// Layer returns the named layer, or nil.
func (d *Drawing) Layer(name string) *Layer {
	for _, l := range d.layers {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// AddLayer returns the named layer, creating it with color when missing.
// Layer table edits are not journalled.
func (d *Drawing) AddLayer(name string, color int) *Layer {
	if l := d.Layer(name); l != nil {
		return l
	}
	l := &Layer{Name: name, Color: color}
	d.layers = append(d.layers, l)
	return l
}

// Entity returns the entity with handle h, or nil.
func (d *Drawing) Entity(h Handle) Entity { return d.entities[h] }

// Len returns the number of entities.
func (d *Drawing) Len() int { return len(d.entities) }

// Handles returns the entity handles in draw order (ascending handle, so
// newer entities draw on top). The slice is shared; do not modify it.
func (d *Drawing) Handles() []Handle {
	if !d.sorted {
		slices.Sort(d.order)
		d.sorted = true
	}
	return d.order
}

// Extents returns the bounding box of every displayed entity, or an empty
// box. Unknown entities are not displayed and do not count.
func (d *Drawing) Extents() geom.Box2 {
	b := geom.EmptyBox()
	for _, e := range d.entities {
		if _, ok := e.(*Unknown); ok {
			continue
		}
		b = b.Union(e.Bounds())
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
	}
	d.entities[h] = e
	if h >= d.Header.HandSeed {
		d.Header.HandSeed = h + 1
	}
}

// remove deletes the entity with handle h without journalling.
func (d *Drawing) remove(h Handle) {
	if _, ok := d.entities[h]; !ok {
		return
	}
	delete(d.entities, h)
	if i := slices.Index(d.order, h); i >= 0 {
		d.order = slices.Delete(d.order, i, i+1)
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
