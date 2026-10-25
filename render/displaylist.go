package render

import (
	"cad-lite/doc"
	"cad-lite/geom"
)

// Color is a non-premultiplied RGBA colour.
type Color struct{ R, G, B, A uint8 }

// WithAlpha returns c with alpha a.
func (c Color) WithAlpha(a uint8) Color { c.A = a; return c }

// ItemKind selects how a display item is drawn.
type ItemKind uint8

const (
	// ItemPolyline strokes Points as a polyline.
	ItemPolyline ItemKind = iota
	// ItemRect fills the axis-aligned box spanned by Points[0] and
	// Points[1], with a Weight-wide outline in Outline when it is opaque.
	ItemRect
	// ItemFill fills the polygon Points (closed implicitly, even-odd) with Color. It has no
	// outline: a builder that wants the edge crisp, or the shape visible when it is under a pixel,
	// adds an ItemPolyline over it.
	ItemFill
)

// DisplayItem is one backend-neutral drawing primitive in model space.
type DisplayItem struct {
	// Owner is the entity the item draws, or 0 for overlays.
	Owner doc.Handle
	Kind  ItemKind
	// Points are model-space vertices, pre-tessellated.
	Points []geom.Vec2
	// Starts, when set, splits an ItemPolyline's Points into separate open polylines (the dashes
	// of a linetype): run i is Points[Starts[i]:Starts[i+1]], the last one running to the end.
	// Starts[0] is 0. Nil is one polyline.
	Starts []int32
	Color  Color
	// Outline is the border colour of an ItemRect.
	Outline Color
	// Weight is the line width in logical pixels; 0 is a one-device-pixel
	// hairline.
	Weight float32
	Closed bool
}

// DisplayList is an ordered list of display items, drawn first to last.
type DisplayList struct {
	Items []DisplayItem
}

// Reset empties the list, keeping its storage.
func (dl *DisplayList) Reset() { dl.Items = dl.Items[:0] }

// Add appends items.
func (dl *DisplayList) Add(items ...DisplayItem) { dl.Items = append(dl.Items, items...) }

// Polyline appends a stroked polyline.
func (dl *DisplayList) Polyline(pts []geom.Vec2, c Color, weight float32, closed bool) {
	dl.Items = append(dl.Items, DisplayItem{Kind: ItemPolyline, Points: pts, Color: c, Weight: weight, Closed: closed})
}

// Curve appends c tessellated at tol.
func (dl *DisplayList) Curve(c geom.Curve, tol float64, col Color, weight float32) {
	dl.Polyline(c.Tessellate(tol, nil), col, weight, false)
}

// Fill appends a filled polygon.
func (dl *DisplayList) Fill(pts []geom.Vec2, c Color) {
	dl.Items = append(dl.Items, DisplayItem{Kind: ItemFill, Points: pts, Color: c})
}

// Rect appends a filled box from a to b with an outline.
func (dl *DisplayList) Rect(a, b geom.Vec2, fill, outline Color, weight float32) {
	dl.Items = append(dl.Items, DisplayItem{Kind: ItemRect, Points: []geom.Vec2{a, b}, Color: fill, Outline: outline, Weight: weight})
}
