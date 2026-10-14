package dwg

import (
	"math"

	"cad-lite/doc"
	"cad-lite/geom"
)

// lineweights maps the lineweight index of a layer (spec 20.4.67) to hundredths of a millimetre;
// 0 stands for the default, which is also what the by-layer, by-block and default codes mean.
var lineweights = [...]int{0, 5, 9, 13, 15, 18, 20, 25, 30, 35, 40, 50, 53, 60, 70, 80, 90, 100, 106, 120, 140, 158, 200, 211}

// layerRec is a LAYER table entry as read, before its handles are resolved.
type layerRec struct {
	handle uint64
	layer  doc.Layer
	ltype  uint64
}

// readLayer parses a LAYER object (R2000 and later).
func (o *object) readLayer() layerRec {
	m := o.main
	v := o.src.ver
	o.nonEntityHeader()
	rec := layerRec{handle: o.handle}
	rec.layer.Name = o.str()
	if v >= verR21 {
		m.bs() // xref index
	} else {
		m.bit()
		m.bs()
		m.bit()
	}
	if v <= verR14 {
		rec.layer.Frozen = m.bit()
		rec.layer.Off = m.bit()
		m.bit() // frozen in new viewports
		rec.layer.Locked = m.bit()
	} else {
		vals := int(m.bs())
		rec.layer.Frozen = vals&1 != 0
		rec.layer.Off = vals&2 != 0
		rec.layer.Locked = vals&8 != 0
		rec.layer.NoPlot = vals&16 == 0
		if lw := (vals & 0x3E0) >> 5; lw < len(lineweights) {
			rec.layer.Lineweight = lineweights[lw]
		}
	}
	// Colour: an index; from R2004 also an RGB value, a flag byte and optional names.
	idx := int(m.bs())
	if idx < 0 {
		idx = -idx
	}
	rec.layer.Color = 7
	if v < verR18 {
		if idx >= 1 && idx <= 255 {
			rec.layer.Color = idx
		}
	} else {
		rgb := uint32(m.bl())
		flag := m.rc()
		if flag&1 != 0 {
			o.str()
		}
		if flag&2 != 0 {
			o.str()
		}
		switch {
		case rgb>>24 == 0xC3 || rgb&0x01000000 != 0: // an indexed colour
			if i := int(rgb & 0xFF); i >= 1 && i <= 255 {
				rec.layer.Color = i
			}
		case rgb>>24 == 0xC2: // a true colour: the nearest index, as AutoCAD writes it to DXF
			rec.layer.Color = doc.NearestACI(uint8(rgb>>16), uint8(rgb>>8), uint8(rgb))
		default:
			if idx >= 1 && idx <= 255 {
				rec.layer.Color = idx
			}
		}
	}
	o.hnd.handle(0) // external reference block
	if v >= verR15 {
		o.hnd.handle(0) // plot style
	}
	if v >= verR21 {
		o.hnd.handle(0) // material
	}
	rec.ltype = o.hnd.handle(0)
	return rec
}

// readName reads the entry name that follows the common non-entity data of a table record.
func (o *object) readName() string {
	o.nonEntityHeader()
	return o.str()
}

// colorOf is the entity colour the document stores: an ACI index, or by layer for by-block and
// by-layer (the document has no true colour; a true colour keeps its nearest index).
func colorOf(h entityHeader) int {
	if h.color >= 1 && h.color <= 255 {
		return h.color
	}
	return doc.ByLayer
}

// ocs is the object coordinate system of a planar entity, reduced to what the 2D document can
// hold: a normal of (0,0,-1) mirrors X, any other normal is ignored (the entity is read as if
// its normal were +Z, as the DXF reader does).
type ocs struct{ mirror bool }

func newOCS(nx, ny, nz float64) ocs {
	return ocs{mirror: nz < 0 && math.Abs(nx) < 1e-9 && math.Abs(ny) < 1e-9}
}

func (c ocs) point(x, y float64) geom.Vec2 {
	if c.mirror {
		x = -x
	}
	return geom.V(x, y)
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func (o *object) common(h entityHeader, layers func(uint64) string) doc.EntityCommon {
	return doc.EntityCommon{H: doc.Handle(o.handle), Layer: layers(h.layer), Color: colorOf(h)}
}

// readLine parses a LINE (R2000 and later).
func (o *object) readLine(h entityHeader, layers func(uint64) string) doc.Entity {
	m := o.main
	var sx, sy, ex, ey float64
	if o.src.ver <= verR14 {
		sx, sy, _ = m.point3bd()
		ex, ey, _ = m.point3bd()
	} else {
		zZero := m.bit()
		sx = m.rd()
		ex = m.dd(sx)
		sy = m.rd()
		ey = m.dd(sy)
		if !zZero {
			sz := m.rd()
			m.dd(sz)
		}
	}
	m.bitThickness()
	m.bitExtrusion() // a LINE is stored in world coordinates
	if !finite(sx, sy, ex, ey) {
		panic(errCorrupt)
	}
	return &doc.Line{EntityCommon: o.common(h, layers), Start: geom.V(sx, sy), End: geom.V(ex, ey)}
}

// readCircle parses a CIRCLE.
func (o *object) readCircle(h entityHeader, layers func(uint64) string) doc.Entity {
	m := o.main
	x, y, _ := m.point3bd()
	r := m.bd()
	m.bitThickness()
	nx, ny, nz := m.bitExtrusion()
	if !finite(x, y, r) {
		panic(errCorrupt)
	}
	return &doc.Circle{EntityCommon: o.common(h, layers), Center: newOCS(nx, ny, nz).point(x, y), Radius: math.Abs(r)}
}

// readArc parses an ARC.
func (o *object) readArc(h entityHeader, layers func(uint64) string) doc.Entity {
	m := o.main
	x, y, _ := m.point3bd()
	r := m.bd()
	m.bitThickness()
	nx, ny, nz := m.bitExtrusion()
	a0, a1 := m.bd(), m.bd()
	if !finite(x, y, r, a0, a1) {
		panic(errCorrupt)
	}
	c := newOCS(nx, ny, nz)
	if c.mirror {
		a0, a1 = math.Pi-a1, math.Pi-a0
	}
	return &doc.Arc{EntityCommon: o.common(h, layers), Center: c.point(x, y), Radius: math.Abs(r),
		StartAngle: geom.NormAngle(a0), EndAngle: geom.NormAngle(a1)}
}

// readLWPolyline parses an LWPOLYLINE (R2000 and later).
func (o *object) readLWPolyline(h entityHeader, layers func(uint64) string) doc.Entity {
	m := o.main
	flags := int(uint16(m.bs()))
	var constWidth float64
	if flags&4 != 0 {
		constWidth = m.bd()
	}
	if flags&8 != 0 {
		m.bd() // elevation
	}
	if flags&2 != 0 {
		m.bd() // thickness
	}
	nx, ny, nz := 0.0, 0.0, 1.0
	if flags&1 != 0 {
		nx, ny, nz = m.point3bd()
	}
	nv := int(m.bl())
	nb, nids, nw := 0, 0, 0
	if flags&0x10 != 0 {
		nb = int(m.bl())
	}
	if flags&0x400 != 0 {
		nids = int(m.bl())
	}
	if flags&0x20 != 0 {
		nw = int(m.bl())
	}
	// Every count is checked against the bits left before anything is allocated.
	left := m.bitsLeft()
	if nv < 0 || nv > left/2 || nb < 0 || nb > nv || nids < 0 || nids > nv || nw < 0 || nw > nv {
		panic(errCorrupt)
	}
	p := &doc.LWPolyline{EntityCommon: o.common(h, layers), Closed: flags&0x200 != 0}
	c := newOCS(nx, ny, nz)
	pts := make([]geom.Vec2, 0, nv)
	if o.src.ver <= verR14 {
		for range nv {
			x, y := m.point2()
			pts = append(pts, c.point(x, y))
		}
	} else if nv > 0 {
		x, y := m.point2()
		pts = append(pts, c.point(x, y))
		for i := 1; i < nv; i++ {
			x = m.dd(x)
			y = m.dd(y)
			pts = append(pts, c.point(x, y))
		}
	}
	bulges := make([]float64, nv)
	for i := range nb {
		bulges[i] = m.bd()
		if c.mirror {
			bulges[i] = -bulges[i]
		}
	}
	for range nids {
		m.bl()
	}
	var widths [][2]float64
	if nw > 0 || constWidth != 0 {
		widths = make([][2]float64, nv)
		for i := range widths {
			widths[i] = [2]float64{constWidth, constWidth}
		}
		for i := range nw {
			widths[i] = [2]float64{m.bd(), m.bd()}
		}
	}
	for _, pt := range pts {
		if !finite(pt.X, pt.Y) {
			panic(errCorrupt)
		}
	}
	p.Points, p.Bulges, p.Widths = pts, bulges, widths
	return p
}

// polyline2D is a POLYLINE_2D header: the vertex handles are resolved by the importer.
type polyline2D struct {
	flags        int
	startW, endW float64
	norm         ocs
	owned        []uint64 // R2004 and later: the vertex handles in order
	first, last  uint64   // R2000: the first and last vertex handles
	header       entityHeader
	closed       bool
}

// readPolyline2D parses a POLYLINE_2D header.
func (o *object) readPolyline2D(h entityHeader) polyline2D {
	m := o.main
	p := polyline2D{header: h}
	p.flags = int(m.bs())
	m.bs() // curve type
	p.startW, p.endW = m.bd(), m.bd()
	m.bitThickness()
	m.bd() // elevation
	nx, ny, nz := m.bitExtrusion()
	p.norm = newOCS(nx, ny, nz)
	nOwned := 0
	if o.src.ver >= verR18 {
		nOwned = int(m.bl())
		if nOwned < 0 || nOwned > m.bitsLeft() {
			panic(errCorrupt)
		}
	}
	if o.src.ver < verR18 {
		p.first = o.hnd.handle(0)
		p.last = o.hnd.handle(0)
	}
	for range nOwned {
		p.owned = append(p.owned, o.hnd.handle(0))
	}
	p.closed = p.flags&1 != 0
	return p
}

// vertex2D is a POLYLINE vertex: position, bulge, widths and flags.
type vertex2D struct {
	pt           geom.Vec2
	bulge        float64
	startW, endW float64
	flags        int
	next         uint64
}

// readVertex2D parses a VERTEX_2D.
func (o *object) readVertex2D(h entityHeader) vertex2D {
	m := o.main
	var v vertex2D
	v.flags = int(m.rc())
	x, y, _ := m.point3bd()
	v.pt = geom.V(x, y)
	w := m.bd()
	if w < 0 {
		v.startW, v.endW = -w, -w
	} else {
		v.startW, v.endW = w, m.bd()
	}
	v.bulge = m.bd()
	v.next = h.next
	if !finite(x, y, v.bulge) {
		panic(errCorrupt)
	}
	return v
}

// polylineEntity assembles a POLYLINE_2D and its vertices into a document polyline.
func polylineEntity(c doc.EntityCommon, p polyline2D, vs []vertex2D) doc.Entity {
	e := &doc.LWPolyline{EntityCommon: c, Closed: p.closed}
	var widths [][2]float64
	anyWidth := p.startW != 0 || p.endW != 0
	for _, v := range vs {
		if v.flags&16 != 0 { // a spline frame control point, not on the curve
			continue
		}
		pt, b := p.norm.point(v.pt.X, v.pt.Y), v.bulge
		if p.norm.mirror {
			b = -b
		}
		e.Points = append(e.Points, pt)
		e.Bulges = append(e.Bulges, b)
		sw, ew := v.startW, v.endW
		if sw == 0 && ew == 0 {
			sw, ew = p.startW, p.endW
		}
		anyWidth = anyWidth || sw != 0 || ew != 0
		widths = append(widths, [2]float64{sw, ew})
	}
	if anyWidth {
		e.Widths = widths
	}
	return e
}
