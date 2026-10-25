package render

import (
	"math"
	"slices"

	"cad-lite/doc"
)

// textGreekPx is the capital height, in pixels at the lowest scale of a zoom band, below which a
// text draws as a single line: its glyphs would be an unreadable smudge that costs as much to
// draw as a legible label.
const textGreekPx = 3

// Builder turns entities into display items, caching each entity's
// tessellation per zoom band. Feed it every change set (Invalidate) so the
// cache never serves a stale entity.
//
// Every entity draws with its effective appearance, resolved by the document (doc.Drawing.Resolve
// and doc.Insert.Parts): colour, linetype, linetype scale and lineweight from the entity, the
// enclosing block reference (ByBlock), its layer (ByLayer) or the drawing's global defaults.
type Builder struct {
	d *doc.Drawing
	// Background is the canvas colour; ACI 7 draws in its contrast colour.
	Background Color
	// Weight is the line width of entity geometry in logical pixels.
	Weight float32
	// Lineweights draws an entity as wide as its resolved lineweight (never thinner than Weight)
	// instead of at Weight; the owner drops the cache with InvalidateAll when it changes.
	Lineweights bool
	cache       map[doc.Handle]cached
	// look is the support state the cache was built against (see SyncLayers); spare is the
	// previous one, whose storage the next full check reuses.
	look, spare supportLook
	// order is Build's scratch for the draw order, kept between frames.
	order []doc.Handle
	// patterns caches resolved dash patterns by upper-case linetype name until the next sync.
	patterns map[string][]float64
	// dash is scratch for laying dash patterns.
	dash dasher
}

// supportLook is the part of the drawing's tables and global defaults that cached items depend
// on, beyond the entities themselves.
type supportLook struct {
	layers    []layerLook
	linetypes []linetypeLook
	dimStyles []doc.DimStyle
	ltScale   float64
	lwDefault doc.Lineweight
}

// layerLook is the part of a layer that cached items depend on.
type layerLook struct {
	name        string
	color       int
	linetype    string
	lineweight  int
	off, frozen bool
}

// linetypeLook is the part of a linetype record that cached items depend on.
type linetypeLook struct {
	name    string
	pattern []float64
	complex bool
}

func (a supportLook) equal(b supportLook) bool {
	return slices.Equal(a.layers, b.layers) && slices.Equal(a.dimStyles, b.dimStyles) &&
		a.ltScale == b.ltScale && a.lwDefault == b.lwDefault &&
		slices.EqualFunc(a.linetypes, b.linetypes, func(x, y linetypeLook) bool {
			return x.name == y.name && x.complex == y.complex && slices.Equal(x.pattern, y.pattern)
		})
}

type cached struct {
	band  int
	items []DisplayItem
}

// NewBuilder returns a builder for d drawing on background.
func NewBuilder(d *doc.Drawing, background Color) *Builder {
	b := &Builder{d: d, Background: background, Weight: 1, cache: map[doc.Handle]cached{}, patterns: map[string][]float64{}}
	b.look = b.lookSupport(supportLook{})
	return b
}

// lineweightPx converts a lineweight in hundredths of a millimetre to logical pixels at 96 dpi.
const lineweightPx float64 = 96.0 / 25.4 / 100

// weight is the line width of a resolved lineweight: Weight, or with Lineweights the lineweight
// at 96 dpi when that is wider (AutoCAD draws model-space lineweights in pixels, whatever the
// zoom).
func (b *Builder) weight(lw doc.Lineweight) float32 {
	w := b.Weight
	if b.Lineweights {
		if hu, ok := lw.Hundredths(); ok {
			w = max(w, float32(float64(hu)*lineweightPx))
		}
	}
	return w
}

// Invalidate drops the cached items of every handle in cs. A change set that touched the tables
// or the current properties (Support) syncs as SyncLayers does, so an edit of a layer, a
// linetype, a dimension style, LTSCALE or LWDEFAULT redraws every entity that inherits from it.
func (b *Builder) Invalidate(cs doc.ChangeSet) {
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified, cs.Erased} {
		for _, h := range hs {
			delete(b.cache, h)
		}
	}
	if cs.Support {
		b.SyncLayers()
	}
}

// InvalidateAll drops every cached item, e.g. after the background changes.
func (b *Builder) InvalidateAll() {
	clear(b.cache)
	clear(b.patterns)
	b.look = b.lookSupport(b.look)
}

// SyncLayers drops every cached item only when what entities inherit their appearance from changed
// since the cache was last built or synced, and reports whether it did: a layer's colour,
// linetype, lineweight or visibility, the layer table itself, the linetype table, the dimension
// style table, LTSCALE or LWDEFAULT. Direct layer edits (the layers panel) are not journalled, so
// no change set reports them; a caller that may have made one calls SyncLayers instead of
// InvalidateAll so that entity edits (which Invalidate already handles) keep the rest of the
// cache. Invalidate calls it for every change set that reports Support.
func (b *Builder) SyncLayers() bool {
	now := b.lookSupport(b.spare)
	if now.equal(b.look) {
		b.spare = now
		return false
	}
	clear(b.cache)
	clear(b.patterns)
	b.spare, b.look = b.look, now
	return true
}

// directSame reports whether the parts of the support state that can change outside a
// transaction (the layers, which the document hands out as pointers, LTSCALE and LWDEFAULT) still
// match the cache, comparing in place without copying anything. The linetype and dimension style
// tables change only in a transaction (whose change set reports Support, see Invalidate) or when
// a file reader loads a new drawing, so a frame need not compare them.
func (b *Builder) directSame() bool {
	ls := b.d.Layers()
	if len(ls) != len(b.look.layers) {
		return false
	}
	for i, l := range ls {
		if b.look.layers[i] != (layerLook{l.Name, l.Color, l.Linetype, l.Lineweight, l.Off, l.Frozen}) {
			return false
		}
	}
	return b.look.ltScale == b.d.LTScaleValue() && b.look.lwDefault == b.d.LWDefaultValue()
}

// lookSupport returns the support state, reusing dst's storage.
func (b *Builder) lookSupport(dst supportLook) supportLook {
	dst.layers = dst.layers[:0]
	for _, l := range b.d.Layers() {
		dst.layers = append(dst.layers, layerLook{l.Name, l.Color, l.Linetype, l.Lineweight, l.Off, l.Frozen})
	}
	dst.linetypes = dst.linetypes[:0]
	for _, l := range b.d.Linetypes() {
		dst.linetypes = append(dst.linetypes, linetypeLook{l.Name, l.Pattern, l.Raw != nil})
	}
	dst.dimStyles = append(dst.dimStyles[:0], b.d.DimStyles()...)
	dst.ltScale, dst.lwDefault = b.d.LTScaleValue(), b.d.LWDefaultValue()
	return dst
}

// Cached returns the number of entities whose items are cached.
func (b *Builder) Cached() int { return len(b.cache) }

// Visible reports whether the entity's layer is on and thawed.
func (b *Builder) Visible(e doc.Entity) bool { return b.d.EntityVisible(e) }

// EntityColor resolves an entity's colour in model space (doc.Drawing.ResolveEntity): ByLayer
// through its layer, ByBlock to 7, and ACI 7 to black or white against the background.
func (b *Builder) EntityColor(e doc.Entity) Color { return b.color(b.d.ResolveEntity(e).Color) }

// color maps a resolved ACI to RGB, ACI 7 to black or white against the background.
func (b *Builder) color(idx int) Color {
	if idx == 7 {
		if luminance(b.Background) > 0.5 {
			return Color{0, 0, 0, 255}
		}
		return Color{255, 255, 255, 255}
	}
	return ACI(idx)
}

// style is how geometry with appearance a draws; scale multiplies its linetype scale (an
// enclosing block reference's scale).
func (b *Builder) style(a doc.Appearance, scale float64) strokeStyle {
	return strokeStyle{color: b.color(a.Color), weight: b.weight(a.Lineweight), pattern: b.pattern(a.Linetype), ltScale: a.LinetypeScale * scale}
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
	var items []DisplayItem
	switch e := e.(type) {
	case *doc.Insert:
		// Definition geometry is in model space already; a pattern scales with the accumulated
		// scale of the references around the part, and text and arrowheads draw solid.
		for _, part := range e.Parts() {
			s := b.style(doc.Appearance{Layer: part.Layer, Color: part.Color, Linetype: part.Linetype, LinetypeScale: part.LinetypeScale, Lineweight: part.Lineweight}, part.Scale)
			if part.Kind != doc.PartCurve {
				s.pattern = nil
			}
			for _, curve := range part.Geometry {
				items = b.appendCurve(items, h, curve, s, band)
			}
		}
	case *doc.Dimension:
		items = b.dimensionItems(h, e, band)
	case doc.Annotation:
		// Text draws solid whatever its linetype, as in AutoCAD.
		s := b.style(b.d.ResolveEntity(e), 1)
		s.pattern = nil
		items = b.textItems(items, h, e, s, band)
	default:
		s := b.style(b.d.ResolveEntity(e), 1)
		for _, c := range e.Geometry() {
			items = b.appendCurve(items, h, c, s, band)
		}
	}
	b.cache[h] = cached{band: band, items: items}
	return items
}

// textItems appends the glyph strokes of t, or one line per row of text when it is too small to
// read at band.
func (b *Builder) textItems(items []DisplayItem, h doc.Handle, t doc.Annotation, s strokeStyle, band int) []DisplayItem {
	if t.CharHeight()*math.Exp2(float64(band)) < textGreekPx {
		for _, l := range t.Greek() {
			items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: l, Color: s.color, Weight: s.weight})
		}
		return items
	}
	for _, c := range t.Geometry() {
		items = b.appendCurve(items, h, c, s, band)
	}
	return items
}

// Build appends visible entities back to front by their owning layer, then ascending handle.
// The candidate slice is copied into scratch the builder keeps; its storage/index order is
// unchanged. It first checks the layers, LTSCALE and LWDEFAULT in place and syncs (SyncLayers)
// when they moved, so a frame never draws against a layer edited outside a transaction, without
// copying the tables every frame.
func (b *Builder) Build(hs []doc.Handle, v View, dl *DisplayList) {
	// Direct edits of the layers reach the frame without a change set.
	if !b.directSame() {
		b.SyncLayers()
	}
	b.order = append(b.order[:0], hs...)
	ordered := b.order
	slices.SortFunc(ordered, func(a, c doc.Handle) int {
		ea, ec := b.d.Entity(a), b.d.Entity(c)
		ra, rc := len(b.d.Layers()), len(b.d.Layers())
		if ea != nil {
			ra = b.d.LayerRank(ea.Common().Layer)
		}
		if ec != nil {
			rc = b.d.LayerRank(ec.Common().Layer)
		}
		if ra != rc {
			return rc - ra
		}
		if a < c {
			return -1
		}
		if a > c {
			return 1
		}
		return 0
	})
	for _, h := range ordered {
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
