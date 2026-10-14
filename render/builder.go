package render

import "cad-lite/doc"

// Builder turns entities into display items, caching each entity's
// tessellation per zoom band. Feed it every change set (Invalidate) so the
// cache never serves a stale entity.
type Builder struct {
	d *doc.Drawing
	// Background is the canvas colour; ACI 7 draws in its contrast colour.
	Background Color
	// Weight is the line width of entity geometry in logical pixels.
	Weight float32
	cache  map[doc.Handle]cached
}

type cached struct {
	band  int
	items []DisplayItem
}

// NewBuilder returns a builder for d drawing on background.
func NewBuilder(d *doc.Drawing, background Color) *Builder {
	return &Builder{d: d, Background: background, Weight: 1, cache: map[doc.Handle]cached{}}
}

// Invalidate drops the cached items of every handle in cs.
func (b *Builder) Invalidate(cs doc.ChangeSet) {
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified, cs.Erased} {
		for _, h := range hs {
			delete(b.cache, h)
		}
	}
}

// InvalidateAll drops every cached item, e.g. after a layer colour changes.
func (b *Builder) InvalidateAll() { clear(b.cache) }

// Visible reports whether the entity's layer is on and thawed.
func (b *Builder) Visible(e doc.Entity) bool { return b.d.EntityVisible(e) }

// EntityColor resolves an entity's colour: ByLayer through its layer, and
// ACI 7 to black or white against the background.
func (b *Builder) EntityColor(e doc.Entity) Color {
	idx := e.Common().Color
	if idx == doc.ByLayer || idx == 0 {
		idx = 7
		if l := b.d.Layer(e.Common().Layer); l != nil {
			idx = l.Color
		}
	}
	if idx == 7 {
		if luminance(b.Background) > 0.5 {
			return Color{0, 0, 0, 255}
		}
		return Color{255, 255, 255, 255}
	}
	return ACI(idx)
}

// Items returns the display items for entity h at view v's zoom band. The
// returned slice is shared with the cache; do not modify it.
func (b *Builder) Items(h doc.Handle, v View) []DisplayItem {
	band := Band(v.Scale)
	if c, ok := b.cache[h]; ok && c.band == band {
		return c.items
	}
	e := b.d.Entity(h)
	if e == nil {
		return nil
	}
	tol := BandTolerance(band)
	col := b.EntityColor(e)
	var items []DisplayItem
	for _, c := range e.Geometry() {
		items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: c.Tessellate(tol, nil), Color: col, Weight: b.Weight})
	}
	b.cache[h] = cached{band: band, items: items}
	return items
}

// Build appends to dl the items of every handle in hs whose layer is on.
func (b *Builder) Build(hs []doc.Handle, v View, dl *DisplayList) {
	for _, h := range hs {
		if e := b.d.Entity(h); e != nil && b.Visible(e) {
			dl.Add(b.Items(h, v)...)
		}
	}
}

func luminance(c Color) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// ACI returns the RGB of an AutoCAD Color Index; see doc.ACIRGB for the palette.
func ACI(i int) Color {
	r, g, b := doc.ACIRGB(i)
	return Color{r, g, b, 255}
}
