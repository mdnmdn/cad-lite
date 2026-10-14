// Package spatial indexes entity bounding boxes for viewport culling,
// selection and snap-candidate gathering. It is updated incrementally from
// the document's change sets.
//
// The index is a uniform grid laid over the drawing's extent. An entity is
// filed in every cell its box overlaps; one whose box spans many cells, or
// is unbounded, goes to a short list every query checks instead. Beside the
// grid the index keeps every entry in ascending handle order, which serves
// queries that cover most of the grid without a sort and keeps the order of
// every result deterministic. All maintenance happens in Build and Apply, so
// Query and Near do not mutate the index.
package spatial

import (
	"math"
	"slices"

	"cad-lite/doc"
	"cad-lite/geom"
)

const (
	// linearMax is the entry count up to which queries scan the sorted
	// list and the grid is not built.
	linearMax = 64
	// bigCells is the cell count above which an entity is filed in the
	// big list instead of the cells it overlaps.
	bigCells = 64
	// maxSide caps the grid's cells per axis.
	maxSide = 1024
	// cellFactor scales the cell side relative to the mean area per
	// entity; a few entities per cell keeps cells short and few.
	cellFactor = 1.5
	// batchErase is the erase count above which Apply compacts the sorted
	// list in one pass instead of deleting entry by entry.
	batchErase = 32
)

// item is one indexed box: a handle, its box and, for an entry filed in
// the grid, the first cell of its cell range (used to report it once).
type item struct {
	h      doc.Handle
	box    geom.Box2
	cx, cy int32
}

// Index maps entity handles to their bounding boxes.
type Index struct {
	boxes map[doc.Handle]geom.Box2
	all   []item // every entry, ascending handle

	// The grid. It is laid out when len(all) exceeds linearMax and
	// relaid when the drawing outgrows it; nx == 0 means no grid.
	ox, oy, inv float64 // origin and 1/cell side
	nx, ny      int
	cells       [][]item
	big         []item
	gridN       int // entries when the grid was laid out
	outside     int // entries filed since layout that overflow its extent
	extent      geom.Box2
}

// New returns an empty index.
func New() *Index { return &Index{boxes: map[doc.Handle]geom.Box2{}} }

// Build replaces the index contents with every entity in d.
func (ix *Index) Build(d *doc.Drawing) {
	clear(ix.boxes)
	hs := d.Handles()
	ix.all = make([]item, 0, len(hs))
	for _, h := range hs {
		b := d.Entity(h).Bounds()
		ix.boxes[h] = b
		ix.all = append(ix.all, item{h: h, box: b})
	}
	slices.SortFunc(ix.all, cmpItem)
	ix.layout()
}

// Apply updates the index for one change set of d.
func (ix *Index) Apply(d *doc.Drawing, cs doc.ChangeSet) {
	ix.erase(cs.Erased)
	unsorted := false
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified} {
		for _, h := range hs {
			e := d.Entity(h)
			if e == nil {
				continue
			}
			b := e.Bounds()
			if old, ok := ix.boxes[h]; ok {
				if unsorted {
					slices.SortFunc(ix.all, cmpItem)
					unsorted = false
				}
				ix.boxes[h] = b
				i, _ := ix.find(h)
				ix.all[i].box = b
				ix.unfile(h, old)
				ix.file(item{h: h, box: b})
				continue
			}
			ix.boxes[h] = b
			if n := len(ix.all); n > 0 && ix.all[n-1].h > h {
				unsorted = true
			}
			ix.all = append(ix.all, item{h: h, box: b})
			ix.file(item{h: h, box: b})
		}
	}
	if unsorted {
		slices.SortFunc(ix.all, cmpItem)
	}
	ix.maybeRelayout()
}

// erase removes hs from the index.
func (ix *Index) erase(hs []doc.Handle) {
	gone := 0
	for _, h := range hs {
		old, ok := ix.boxes[h]
		if !ok {
			continue
		}
		delete(ix.boxes, h)
		ix.unfile(h, old)
		gone++
		if len(hs) <= batchErase {
			if i, ok := ix.find(h); ok {
				ix.all = slices.Delete(ix.all, i, i+1)
			}
		}
	}
	if gone > 0 && len(hs) > batchErase {
		ix.all = slices.DeleteFunc(ix.all, func(it item) bool {
			_, ok := ix.boxes[it.h]
			return !ok
		})
	}
}

// Len returns the number of indexed entities.
func (ix *Index) Len() int { return len(ix.boxes) }

// Query returns the handles whose boxes intersect box, in ascending order.
func (ix *Index) Query(box geom.Box2) []doc.Handle {
	if box.IsEmpty() || len(ix.all) == 0 {
		return nil
	}
	if ix.nx == 0 {
		return ix.scan(box)
	}
	x0, y0, x1, y1 := ix.cellRange(box)
	if (x1-x0+1)*(y1-y0+1)*2 >= ix.nx*ix.ny {
		return ix.scan(box)
	}
	out := ix.reserve(box)
	for _, it := range ix.big {
		if it.box.Intersects(box) {
			out = append(out, it.h)
		}
	}
	for cy := y0; cy <= y1; cy++ {
		row := ix.cells[cy*ix.nx:]
		for cx := x0; cx <= x1; cx++ {
			for _, it := range row[cx] {
				// An entity spanning several cells is reported from the
				// first cell of its range that the query visits.
				if int(max(it.cx, int32(x0))) != cx || int(max(it.cy, int32(y0))) != cy {
					continue
				}
				if it.box.Intersects(box) {
					out = append(out, it.h)
				}
			}
		}
	}
	sortHandles(out)
	return out
}

// sortHandles sorts hs ascending: a least-significant-digit radix sort for
// long results, whose cost is linear in len(hs) and in the bits the largest
// handle uses, and the standard sort for short ones.
func sortHandles(hs []doc.Handle) {
	if len(hs) < 1024 {
		slices.Sort(hs)
		return
	}
	var top doc.Handle
	for _, h := range hs {
		top = max(top, h)
	}
	const bits = 11
	src, dst := hs, make([]doc.Handle, len(hs))
	for shift := uint(0); shift < 64 && top>>shift != 0; shift += bits {
		var count [1 << bits]int
		for _, h := range src {
			count[(h>>shift)&(1<<bits-1)]++
		}
		pos := 0
		for i, c := range count {
			count[i] = pos
			pos += c
		}
		for _, h := range src {
			d := (h >> shift) & (1<<bits - 1)
			dst[count[d]] = h
			count[d]++
		}
		src, dst = dst, src
	}
	if &src[0] != &hs[0] {
		copy(hs, src)
	}
}

// scan is Query by a pass over the sorted list, which needs no sort.
func (ix *Index) scan(box geom.Box2) []doc.Handle {
	out := ix.reserve(box)
	for i := range ix.all {
		if ix.all[i].box.Intersects(box) {
			out = append(out, ix.all[i].h)
		}
	}
	return out
}

// Near returns the handles whose boxes come within r of p, in ascending
// order: the candidates for picking and snapping at p.
func (ix *Index) Near(p geom.Vec2, r float64) []doc.Handle {
	return ix.Query(geom.Box2{Min: p, Max: p}.Grow(r))
}

// reserve returns an empty result slice with room for the share of the
// entries that the part of box inside the grid's extent suggests, so a
// large query allocates about once.
func (ix *Index) reserve(box geom.Box2) []doc.Handle {
	if ix.nx == 0 {
		return nil
	}
	c := clip(box, ix.extent)
	ew, eh := ix.extent.Size()
	cw, ch := c.Size()
	if c.IsEmpty() || ew <= 0 || eh <= 0 {
		return nil
	}
	return make([]doc.Handle, 0, int(float64(len(ix.all))*min(1, cw*ch/(ew*eh)*1.1))+16)
}

// clip returns the overlap of a and b, empty when they do not overlap.
func clip(a, b geom.Box2) geom.Box2 {
	return geom.Box2{
		Min: geom.V(max(a.Min.X, b.Min.X), max(a.Min.Y, b.Min.Y)),
		Max: geom.V(min(a.Max.X, b.Max.X), min(a.Max.Y, b.Max.Y)),
	}
}

func cmpItem(a, b item) int {
	switch {
	case a.h < b.h:
		return -1
	case a.h > b.h:
		return 1
	}
	return 0
}

// find returns the position of h in the sorted list.
func (ix *Index) find(h doc.Handle) (int, bool) {
	return slices.BinarySearchFunc(ix.all, h, func(it item, h doc.Handle) int {
		return cmpItem(it, item{h: h})
	})
}

// finite reports whether b is a non-empty box with finite corners.
func finite(b geom.Box2) bool {
	for _, v := range [...]float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return !b.IsEmpty()
}

// placeable reports whether b can ever intersect a box: it is not empty
// and has no NaN corner. Such a box goes to the grid or the big list.
func placeable(b geom.Box2) bool {
	for _, v := range [...]float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
		if math.IsNaN(v) {
			return false
		}
	}
	return !b.IsEmpty()
}

// layout chooses the grid for the current entries and files them all.
func (ix *Index) layout() {
	ix.nx, ix.ny, ix.cells, ix.big, ix.outside = 0, 0, nil, nil, 0
	ix.gridN = len(ix.all)
	if len(ix.all) <= linearMax {
		return
	}
	ext := geom.EmptyBox()
	n := 0
	for _, it := range ix.all {
		if finite(it.box) {
			ext = ext.Union(it.box)
			n++
		}
	}
	if n == 0 {
		ext = geom.Box2{}
		n = 1
	}
	w, h := ext.Size()
	side := math.Sqrt(w*h/float64(n)) * cellFactor
	side = max(side, max(w, h)/maxSide, 1e-9)
	ix.ox, ix.oy, ix.inv = ext.Min.X, ext.Min.Y, 1/side
	ix.nx = min(int(w/side)+1, maxSide)
	ix.ny = min(int(h/side)+1, maxSide)
	ix.extent = ext
	ix.cells = make([][]item, ix.nx*ix.ny)
	for _, it := range ix.all {
		ix.file(it)
	}
	ix.outside = 0
}

// maybeRelayout lays the grid out again once the drawing has grown well
// past, shrunk well below, or spread beyond the grid it was laid out for.
func (ix *Index) maybeRelayout() {
	n := len(ix.all)
	switch {
	case ix.nx == 0 && n > linearMax,
		ix.nx != 0 && n <= linearMax/2,
		n > 2*ix.gridN+linearMax,
		n < ix.gridN/4,
		ix.outside > n/4+linearMax:
		ix.layout()
	}
}

// cell returns the clamped cell index of coordinate v along an axis with
// origin o and n cells. Clamping files everything beyond the extent in the
// border cells, which stays correct because queries clamp the same way.
func (ix *Index) cell(v, o float64, n int) int {
	f := (v - o) * ix.inv
	if !(f > 0) {
		return 0
	}
	if f >= float64(n-1) {
		return n - 1
	}
	return int(f)
}

// cellRange returns the clamped cell range that box covers.
func (ix *Index) cellRange(b geom.Box2) (x0, y0, x1, y1 int) {
	return ix.cell(b.Min.X, ix.ox, ix.nx), ix.cell(b.Min.Y, ix.oy, ix.ny),
		ix.cell(b.Max.X, ix.ox, ix.nx), ix.cell(b.Max.Y, ix.oy, ix.ny)
}

// isBig reports whether b goes to the big list rather than the cells.
func (ix *Index) isBig(x0, y0, x1, y1 int, b geom.Box2) bool {
	return !finite(b) || (x1-x0+1)*(y1-y0+1) > bigCells
}

// file adds it to the grid, if there is one.
func (ix *Index) file(it item) {
	if ix.nx == 0 || !placeable(it.box) {
		return
	}
	if !ix.extent.ContainsBox(it.box) {
		ix.outside++
	}
	x0, y0, x1, y1 := ix.cellRange(it.box)
	if ix.isBig(x0, y0, x1, y1, it.box) {
		ix.big = append(ix.big, it)
		return
	}
	it.cx, it.cy = int32(x0), int32(y0)
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			c := &ix.cells[cy*ix.nx+cx]
			*c = append(*c, it)
		}
	}
}

// unfile removes h, last filed with box b, from the grid.
func (ix *Index) unfile(h doc.Handle, b geom.Box2) {
	if ix.nx == 0 || !placeable(b) {
		return
	}
	x0, y0, x1, y1 := ix.cellRange(b)
	if ix.isBig(x0, y0, x1, y1, b) {
		ix.big = removeItem(ix.big, h)
		return
	}
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			c := &ix.cells[cy*ix.nx+cx]
			*c = removeItem(*c, h)
		}
	}
}

// removeItem deletes h from s by swapping the last element into its place;
// order within a cell does not matter because results are sorted.
func removeItem(s []item, h doc.Handle) []item {
	for i := range s {
		if s[i].h == h {
			last := len(s) - 1
			s[i] = s[last]
			return s[:last]
		}
	}
	return s
}
