package cmdsys

import "go-draw/doc"

// selMemory is the Previous selection: the set the latest command took, which the P selection
// option reselects. One per session; entities erased since are pruned.
type selMemory struct{ hs []doc.Handle }

func (m *selMemory) prune(d *doc.Drawing) {
	out := m.hs[:0]
	for _, h := range m.hs {
		if d.Entity(h) != nil {
			out = append(out, h)
		}
	}
	m.hs = out
}

// PreviousSelection returns the handles of the latest selection a command took (the P option),
// minus entities erased since. The slice is a copy.
func (c *Context) PreviousSelection() []doc.Handle {
	return append([]doc.Handle(nil), c.prev.hs...)
}

// SetPrevious records hs as the Previous selection. A command that takes a selection calls it
// through the shared selecting helper; an empty hs leaves the memory unchanged, as AutoCAD does.
func (c *Context) SetPrevious(hs []doc.Handle) {
	if len(hs) > 0 {
		c.prev.hs = append(c.prev.hs[:0], hs...)
	}
}
