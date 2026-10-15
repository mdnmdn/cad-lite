package doc

// Visible reports whether the layer's entities are drawn and can be snapped to or picked: the
// layer is neither off nor frozen. A nil layer (an entity whose layer is missing from the table)
// is visible. Render, snap and selection all read this one rule.
func (l *Layer) Visible() bool { return l == nil || (!l.Off && !l.Frozen) }

// Editable reports whether the layer's entities may be changed: visible and not locked.
func (l *Layer) Editable() bool { return l.Visible() && (l == nil || !l.Locked) }

// EntityVisible reports whether e is on a visible layer (see Layer.Visible).
func (d *Drawing) EntityVisible(e Entity) bool { return d.Layer(e.Common().Layer).Visible() }

// EntityEditable reports whether e is on an editable layer (see Layer.Editable).
func (d *Drawing) EntityEditable(e Entity) bool {
	return e != nil && d.Layer(e.Common().Layer).Editable()
}

// RemoveLayer deletes the named layer from the table and reports whether it did. Layer 0, the
// current layer and a layer that does not exist are kept, and so is a layer an entity is on:
// callers purge the unreferenced ones. Layer table edits are not journalled.
func (d *Drawing) RemoveLayer(name string) bool {
	if name == "0" || name == d.CurrentLayer {
		return false
	}
	for _, e := range d.entities {
		if e.Common().Layer == name {
			return false
		}
	}
	for _, b := range d.blocks {
		for _, e := range b.Entities {
			if e.Common().Layer == name {
				return false
			}
		}
	}
	for i, l := range d.layers {
		if l.Name == name {
			d.layers = append(d.layers[:i], d.layers[i+1:]...)
			d.reindexLayers()
			return true
		}
	}
	return false
}
