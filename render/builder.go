package render

import (
	"math"

	"cad-lite/doc"
)

// textGreekPx is the capital height, in pixels at the lowest scale of a zoom band, below which a
// text draws as a single line: its glyphs would be an unreadable smudge that costs as much to
// draw as a legible label.
const textGreekPx = 3

// Builder turns entities into display items, caching each entity's
// tessellation per zoom band. Feed it every change set (Invalidate) so the
// cache never serves a stale entity.
type Builder struct {
	d *doc.Drawing
	// Background is the canvas colour; ACI 7 draws in its contrast colour.
	Background Color
	// Weight is the line width of entity geometry in logical pixels.
	Weight float32
	// Lineweights draws an entity as wide as its layer's lineweight (never thinner than Weight)
	// instead of at Weight; the owner drops the cache with InvalidateAll when it changes.
	Lineweights bool
	cache       map[doc.Handle]cached
	// layers is the layer appearance the cache was built against (see SyncLayers).
	layers []layerLook
}

// layerLook is the part of a layer that cached items depend on.
type layerLook struct {
	name        string
	color       int
	lineweight  int
	off, frozen bool
}

type cached struct {
	band  int
	items []DisplayItem
}

// NewBuilder returns a builder for d drawing on background.
func NewBuilder(d *doc.Drawing, background Color) *Builder {
	b := &Builder{d: d, Background: background, Weight: 1, cache: map[doc.Handle]cached{}}
	b.layers = b.lookLayers(nil)
	return b
}

// lineweightPx converts a lineweight in hundredths of a millimetre to logical pixels at 96 dpi.
const lineweightPx float64 = 96.0 / 25.4 / 100

// weightOf is the line width of entity e: Weight, or with Lineweights its layer's lineweight.
func (b *Builder) weightOf(e doc.Entity) float32 {
	w := b.Weight
	if b.Lineweights {
		if l := b.d.Layer(e.Common().Layer); l != nil && l.Lineweight > 0 {
			w = max(w, float32(float64(l.Lineweight)*lineweightPx))
		}
	}
	return w
}

// Invalidate drops the cached items of every handle in cs.
func (b *Builder) Invalidate(cs doc.ChangeSet) {
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified, cs.Erased} {
		for _, h := range hs {
			delete(b.cache, h)
		}
	}
}

// InvalidateAll drops every cached item, e.g. after the background changes.
func (b *Builder) InvalidateAll() {
	clear(b.cache)
	b.layers = b.lookLayers(b.layers[:0])
}

// SyncLayers drops every cached item only when a layer's colour or lineweight, or the layer table
// itself, changed since the cache was last built or synced, and reports whether it did. Layer table
// edits are not journalled, so no change set reports them; a caller that may have edited layers
// calls SyncLayers instead of InvalidateAll so that entity edits (which Invalidate already handles)
// keep the rest of the cache.
func (b *Builder) SyncLayers() bool {
	ls := b.d.Layers()
	same := len(ls) == len(b.layers)
	for i := 0; same && i < len(ls); i++ {
		same = b.layers[i] == layerLook{ls[i].Name, ls[i].Color, ls[i].Lineweight, ls[i].Off, ls[i].Frozen}
	}
	if same {
		return false
	}
	b.InvalidateAll()
	return true
}

// lookLayers appends the appearance of every layer to dst.
func (b *Builder) lookLayers(dst []layerLook) []layerLook {
	for _, l := range b.d.Layers() {
		dst = append(dst, layerLook{l.Name, l.Color, l.Lineweight, l.Off, l.Frozen})
	}
	return dst
}

// Cached returns the number of entities whose items are cached.
func (b *Builder) Cached() int { return len(b.cache) }

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
	if in, ok := e.(*doc.Insert); ok {
		var items []DisplayItem
		for _, part := range in.Parts() {
			proxy := &doc.Line{EntityCommon: doc.EntityCommon{Layer: part.Layer, Color: part.Color}}
			for _, curve := range part.Geometry {
				items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: curve.Tessellate(tol, nil), Color: b.EntityColor(proxy), Weight: b.weightOf(proxy)})
			}
		}
		b.cache[h] = cached{band: band, items: items}
		return items
	}
	col := b.EntityColor(e)
	var items []DisplayItem
	weight := b.weightOf(e)
	if t, ok := e.(doc.Annotation); ok && t.CharHeight()*math.Exp2(float64(band)) < textGreekPx {
		// Text too small to read draws as one line per row of text.
		for _, l := range t.Greek() {
			items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: l, Color: col, Weight: weight})
		}
		b.cache[h] = cached{band: band, items: items}
		return items
	}
	for _, c := range e.Geometry() {
		items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: c.Tessellate(tol, nil), Color: col, Weight: weight})
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
