package doc

import (
	"errors"
	"slices"
)

// LayerRank returns a layer's front-to-back position, or the position behind known layers.
func (d *Drawing) LayerRank(name string) int {
	if i, ok := d.layerRanks[name]; ok {
		return i
	}
	return len(d.layers)
}
func (d *Drawing) layerNames() []string {
	out := make([]string, len(d.layers))
	for i, l := range d.layers {
		out[i] = l.Name
	}
	return out
}
func (d *Drawing) validateLayerOrder(names []string) error {
	if len(names) != len(d.layers) {
		return errors.New("doc: layer order must contain every layer")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if d.Layer(name) == nil || seen[name] {
			return errors.New("doc: layer order contains unknown or duplicate names")
		}
		seen[name] = true
	}
	return nil
}

// SetLayerOrder changes the canonical front-to-back order as part of this transaction.
// Names must be an exact permutation; no-op or invalid input changes nothing.
func (t *Txn) SetLayerOrder(names []string) error {
	if t.done {
		return errors.New("doc: transaction is closed")
	}
	if err := t.d.validateLayerOrder(names); err != nil {
		return err
	}
	if slices.Equal(names, t.d.layerNames()) {
		return nil
	}
	if t.layerOrderBefore == nil {
		t.layerOrderBefore = t.d.layerNames()
	}
	t.d.restoreLayerOrder(names)
	return nil
}

// LoadLayerOrder applies a validated order outside the journal, for file readers.
func (d *Drawing) LoadLayerOrder(names []string) error {
	if d.open != nil {
		return ErrTxnOpen
	}
	if err := d.validateLayerOrder(names); err != nil {
		return err
	}
	d.restoreLayerOrder(names)
	return nil
}

// restoreLayerOrder rearranges only surviving named layers in their current slots.
func (d *Drawing) restoreLayerOrder(names []string) bool {
	wanted := make([]*Layer, 0, len(names))
	included := map[string]bool{}
	for _, name := range names {
		if l := d.Layer(name); l != nil && !included[name] {
			wanted = append(wanted, l)
			included[name] = true
		}
	}
	next := 0
	changed := false
	for i, l := range d.layers {
		if included[l.Name] {
			if d.layers[i] != wanted[next] {
				changed = true
			}
			d.layers[i] = wanted[next]
			next++
		}
	}
	if changed {
		d.reindexLayers()
	}
	return changed
}

// TransactionOpen reports whether another edit transaction is active.
func (d *Drawing) TransactionOpen() bool { return d.open != nil }
