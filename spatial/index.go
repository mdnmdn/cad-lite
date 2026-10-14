// Package spatial indexes entity bounding boxes for viewport culling,
// selection and snap-candidate gathering. It is updated incrementally from
// the document's change sets.
//
// The implementation is a linear scan over cached boxes: the query API is
// the one an R-tree will serve, so callers do not change when it lands.
package spatial

import (
	"slices"

	"cad-lite/doc"
	"cad-lite/geom"
)

// Index maps entity handles to their bounding boxes.
type Index struct {
	boxes map[doc.Handle]geom.Box2
}

// New returns an empty index.
func New() *Index { return &Index{boxes: map[doc.Handle]geom.Box2{}} }

// Build replaces the index contents with every entity in d.
func (ix *Index) Build(d *doc.Drawing) {
	clear(ix.boxes)
	for _, h := range d.Handles() {
		ix.boxes[h] = d.Entity(h).Bounds()
	}
}

// Apply updates the index for one change set of d.
func (ix *Index) Apply(d *doc.Drawing, cs doc.ChangeSet) {
	for _, h := range cs.Erased {
		delete(ix.boxes, h)
	}
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified} {
		for _, h := range hs {
			if e := d.Entity(h); e != nil {
				ix.boxes[h] = e.Bounds()
			}
		}
	}
}

// Len returns the number of indexed entities.
func (ix *Index) Len() int { return len(ix.boxes) }

// Query returns the handles whose boxes intersect box, in ascending order.
func (ix *Index) Query(box geom.Box2) []doc.Handle {
	var out []doc.Handle
	for h, b := range ix.boxes {
		if b.Intersects(box) {
			out = append(out, h)
		}
	}
	slices.Sort(out)
	return out
}

// Near returns the handles whose boxes come within r of p, in ascending
// order: the candidates for picking and snapping at p.
func (ix *Index) Near(p geom.Vec2, r float64) []doc.Handle {
	return ix.Query(geom.Box2{Min: p, Max: p}.Grow(r))
}
