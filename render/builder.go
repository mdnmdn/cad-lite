package render

import (
	"math"

	"cad-lite/doc"
)

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

// ACI returns the RGB of an AutoCAD Color Index. 1–9 and the grays 250–255
// are the standard values; 10–249 are generated from the palette's
// hue/shade layout and match AutoCAD closely but not exactly.
func ACI(i int) Color {
	switch {
	case i >= 1 && i <= 9:
		return aciBase[i]
	case i >= 250 && i <= 255:
		g := [6]uint8{51, 91, 132, 173, 214, 255}[i-250]
		return Color{g, g, g, 255}
	case i >= 10 && i <= 249:
		hue := float64((i-10)/10) * 15
		shade := (i - 10) % 10
		v := [5]float64{1, 0.8, 0.6, 0.5, 0.3}[shade/2]
		s := 1.0
		if shade%2 == 1 {
			s = 0.5
		}
		return hsv(hue, s, v)
	}
	return Color{255, 255, 255, 255}
}

var aciBase = [10]Color{
	{0, 0, 0, 255},
	{255, 0, 0, 255},
	{255, 255, 0, 255},
	{0, 255, 0, 255},
	{0, 255, 255, 255},
	{0, 0, 255, 255},
	{255, 0, 255, 255},
	{255, 255, 255, 255},
	{128, 128, 128, 255},
	{192, 192, 192, 255},
}

func hsv(h, s, v float64) Color {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	f := func(u float64) uint8 { return uint8(math.Round((u + m) * 255)) }
	return Color{f(r), f(g), f(b), 255}
}
