package dwg

// Object type codes (spec chapter 5). Codes from classTypeBase up are class numbers.
const (
	tText         = 1
	tAttrib       = 2
	tAttdef       = 3
	tBlock        = 4
	tEndblk       = 5
	tSeqend       = 6
	tInsert       = 7
	tMinsert      = 8
	tVertex2D     = 0x0A
	tVertex3D     = 0x0B
	tVertexMesh   = 0x0C
	tVertexPFace  = 0x0D
	tVertexPFaceF = 0x0E
	tPolyline2D   = 0x0F
	tPolyline3D   = 0x10
	tArc          = 0x11
	tCircle       = 0x12
	tLine         = 0x13
	tDimOrdinate  = 0x14
	tDimLinear    = 0x15
	tDimAligned   = 0x16
	tDimAng3Pt    = 0x17
	tDimAng2Ln    = 0x18
	tDimRadius    = 0x19
	tDimDiameter  = 0x1A
	tPoint        = 0x1B
	tFace3D       = 0x1C
	tPolylinePF   = 0x1D
	tPolylineMesh = 0x1E
	tSolid        = 0x1F
	tTrace        = 0x20
	tShape        = 0x21
	tViewport     = 0x22
	tEllipse      = 0x23
	tSpline       = 0x24
	tRegion       = 0x25
	tSolid3D      = 0x26
	tBody         = 0x27
	tRay          = 0x28
	tXLine        = 0x29
	tDictionary   = 0x2A
	tOLEFrame     = 0x2B
	tMText        = 0x2C
	tLeader       = 0x2D
	tTolerance    = 0x2E
	tMLine        = 0x2F
	tBlockHeader  = 0x31
	tLayerControl = 0x32
	tLayer        = 0x33
	tStyle        = 0x35
	tLType        = 0x39
	tOLE2Frame    = 0x4A
	tLWPolyline   = 0x4D
	tHatch        = 0x4E
	tProxyEntity  = 0x1F2
	tProxyObject  = 0x1F3
)

// fixedNames maps the entity type codes that have a fixed number to their DXF class names. It
// is also the list of the fixed entity types.
var fixedNames = map[int]string{
	tText: "TEXT", tAttrib: "ATTRIB", tAttdef: "ATTDEF", tBlock: "BLOCK", tEndblk: "ENDBLK", tSeqend: "SEQEND",
	tInsert: "INSERT", tMinsert: "INSERT", tVertex2D: "VERTEX", tVertex3D: "VERTEX", tVertexMesh: "VERTEX",
	tVertexPFace: "VERTEX", tVertexPFaceF: "VERTEX", tPolyline2D: "POLYLINE", tPolyline3D: "POLYLINE",
	tArc: "ARC", tCircle: "CIRCLE", tLine: "LINE", tDimOrdinate: "DIMENSION", tDimLinear: "DIMENSION",
	tDimAligned: "DIMENSION", tDimAng3Pt: "DIMENSION", tDimAng2Ln: "DIMENSION", tDimRadius: "DIMENSION",
	tDimDiameter: "DIMENSION", tPoint: "POINT", tFace3D: "3DFACE", tPolylinePF: "POLYLINE",
	tPolylineMesh: "POLYLINE", tSolid: "SOLID", tTrace: "TRACE", tShape: "SHAPE", tViewport: "VIEWPORT",
	tEllipse: "ELLIPSE", tSpline: "SPLINE", tRegion: "REGION", tSolid3D: "3DSOLID", tBody: "BODY",
	tRay: "RAY", tXLine: "XLINE", tOLEFrame: "OLEFRAME", tMText: "MTEXT", tLeader: "LEADER",
	tTolerance: "TOLERANCE", tMLine: "MLINE", tOLE2Frame: "OLE2FRAME", tLWPolyline: "LWPOLYLINE", tHatch: "HATCH", tProxyEntity: "ACAD_PROXY_ENTITY",
}

// subEntity reports the entity types that belong to another entity or to a block definition and
// are never counted as drawing content of their own.
func subEntity(t int) bool {
	switch t {
	case tAttrib, tAttdef, tBlock, tEndblk, tSeqend, tVertex2D, tVertex3D, tVertexMesh, tVertexPFace, tVertexPFaceF:
		return true
	}
	return false
}

// objSource is the object data of a file together with what is needed to open its objects.
type objSource struct {
	data    []byte // the bytes the object offsets index
	ver     int
	cp      int
	classes map[int]class
}

// object is one opened object: its type and three readers positioned on the main data, the
// string stream and the handle stream. Before R2007 the string stream is the main data.
type object struct {
	src    *objSource
	handle uint64
	typ    int
	main   *bitReader
	text   *bitReader
	hnd    *bitReader
	crcOK  bool
	// initial is the bit position of the object type, the origin of the object-size field of
	// R13 to R2007.
	initial int
}

// openObject opens the object at offset in the object data. Its size and type are read; the
// readers are bounded to the object, so a corrupt record cannot read its neighbours.
func (s *objSource) openObject(handle uint64, offset int) (o *object, err error) {
	err = safe(func() {
		r := newBitReader(s.data)
		r.seekByte(offset)
		size := r.ms()
		var handleBits int
		if s.ver >= verR24 {
			handleBits = int(r.mc())
		}
		bodyStart := r.tell() / 8
		if size <= 0 || size > len(s.data)-bodyStart {
			panic(errCorrupt)
		}
		bodyEnd := bodyStart + size
		o = &object{src: s, handle: handle}
		if bodyEnd+2 <= len(s.data) {
			stored := uint16(s.data[bodyEnd]) | uint16(s.data[bodyEnd+1])<<8
			o.crcOK = crc16(0xC0C1, s.data[offset:bodyEnd]) == stored
		}
		r.end = bodyEnd * 8
		r.plain = s.ver <= verR14
		o.main = r
		initial := r.tell()
		o.initial = initial
		var handleStart int
		switch {
		case s.ver >= verR24:
			o.typ = readObjectType(r)
			handleStart = bodyStart*8 + size*8 - handleBits
			o.text, _ = stringStream(r, handleStart-1)
		default:
			o.typ = int(r.bs())
			if s.ver >= verR15 {
				handleStart = initial + int(r.rl())
				if s.ver == verR21 {
					o.text, _ = stringStream(r, handleStart-1)
				}
			}
		}
		if o.text == nil {
			o.text = r
		}
		if s.ver >= verR15 {
			if handleStart < r.tell() || handleStart > r.end {
				panic(errCorrupt)
			}
			o.hnd = r.clone()
			o.hnd.seekBit(handleStart)
		}
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// readObjectType reads the type of R2010 and later: a bit pair choosing a byte, a byte plus
// 0x1F0, or a short.
func readObjectType(r *bitReader) int {
	switch r.bb() {
	case 0:
		return int(r.rc())
	case 1:
		return 0x1F0 + int(r.rc())
	default:
		return int(r.rus())
	}
}

// str reads a string of the object: from the string stream in R2007 and later, inline before.
func (o *object) str() string {
	if o.src.ver >= verR21 {
		return o.text.tu()
	}
	return o.text.tv(o.src.cp)
}

// skipEED skips the extended entity data that follows the handle.
func (o *object) skipEED() {
	for {
		n := int(o.main.bs())
		if n == 0 {
			return
		}
		o.main.handle(0) // application handle
		o.main.skipBytes(n)
	}
}

// commonData reads the handle and extended data every object starts with.
func (o *object) commonData() {
	if h := o.main.handle(0); h != o.handle {
		panic(errCorrupt)
	}
	o.skipEED()
}

// startHandles reads the object-size field R13 and R14 put after the handle and extended data
// (R2000 and later put it before) and positions the handle stream on the bits that follow the
// object's data.
func (o *object) startHandles() {
	size := int(o.main.rl())
	start := o.initial + size
	if size < 0 || start < o.main.tell() || start > o.main.end {
		panic(errCorrupt)
	}
	o.hnd = o.main.clone()
	o.hnd.seekBit(start)
}

// nonEntityHeader reads the start common to table entries and other non-graphical objects, up
// to the first type-specific field.
func (o *object) nonEntityHeader() {
	o.commonData()
	v := o.src.ver
	if v <= verR14 {
		o.startHandles()
	}
	n := int(o.main.bl()) // reactors
	if n < 0 || n > o.main.bitsLeft() {
		panic(errCorrupt)
	}
	xdicMissing := false
	if v >= verR18 {
		xdicMissing = o.main.bit()
	}
	if v >= verR27 {
		o.main.bit() // has data-store binary data
	}
	// Handle stream: owner, reactors, extension dictionary.
	o.hnd.handle(o.handle)
	for range n {
		o.hnd.handle(0)
	}
	if !xdicMissing {
		o.hnd.handle(0)
	}
}

// entityHeader is what the start of an entity record holds that the importer uses.
type entityHeader struct {
	mode  byte   // 0 owner given, 1 paper space, 2 model space
	owner uint64 // the owner handle when mode is 0
	color int    // the ACI index (a true colour or colour-book entry carries its nearest index), 0 or 256
	layer uint64
	next  uint64
}

// entityHeader reads the common entity data (spec 20.4.1) of R2000 and later.
func (o *object) entityHeader() (h entityHeader) {
	v := o.src.ver
	m := o.main
	o.commonData()
	if m.bit() { // graphic present
		var n int64
		if v >= verR24 {
			n = int64(m.bll())
		} else {
			n = int64(m.rl())
		}
		if n < 0 || n > int64(m.bitsLeft()/8) {
			panic(errCorrupt)
		}
		m.skipBytes(int(n))
	}
	if v <= verR14 {
		o.startHandles()
	}
	h.mode = m.bb()
	if h.mode == 0 {
		h.owner = o.hnd.handle(o.handle)
	}
	n := int(m.bl()) // reactors
	if n < 0 || n > m.bitsLeft() {
		panic(errCorrupt)
	}
	for range n {
		o.hnd.handle(0)
	}
	xdicMissing := false
	if v >= verR18 {
		xdicMissing = m.bit()
	}
	if !xdicMissing {
		o.hnd.handle(0)
	}
	if v >= verR27 {
		m.bit()
	}
	if v <= verR14 {
		return o.entityHeaderR14(h)
	}
	noLinks := true
	if v < verR18 {
		noLinks = m.bit()
	}
	if noLinks {
		h.next = o.handle + 1
	} else {
		o.hnd.handle(o.handle) // previous entity
		h.next = o.hnd.handle(o.handle)
	}
	var book bool
	h.color, book = o.entityColor()
	if book {
		o.hnd.handle(0)
	}
	m.bd() // linetype scale
	h.layer = o.hnd.handle(0)
	ltFlags := m.bb()
	if ltFlags == 3 {
		o.hnd.handle(0)
	}
	if v >= verR21 {
		if m.bb() == 3 { // material
			o.hnd.handle(0)
		}
		m.rc() // shadow flags
	}
	if m.bb() == 3 { // plot style
		o.hnd.handle(0)
	}
	if v >= verR24 {
		for range 3 { // full, face and edge visual styles
			if m.bit() {
				o.hnd.handle(0)
			}
		}
	}
	m.bs() // invisibility
	m.rc() // line weight
	return h
}

// entityColor reads the entity colour: a plain index up to R2000; from R2004 a flagged value
// whose low 12 bits are the colour index (for a true colour or a colour-book colour, the nearest
// index AutoCAD chose), optionally followed by the RGB value, a colour-book reference and a
// transparency, which the document cannot hold.
func (o *object) entityColor() (color int, book bool) {
	m := o.main
	if o.src.ver < verR18 {
		return int(m.bs()), false
	}
	v := int(uint16(m.bs()))
	flags := v & 0xFF00
	switch {
	case flags&0x4000 != 0:
		book = true
	case flags&0x8000 != 0:
		m.bl() // RGB
	}
	if flags&0x2000 != 0 {
		m.bl() // transparency
	}
	return v & 0x0FFF, book
}

// bitExtrusion reads an extrusion (BE): a single bit for the Z axis, else a 3BD.
func (r *bitReader) bitExtrusion() (x, y, z float64) {
	if !r.plain && r.bit() {
		return 0, 0, 1
	}
	return r.point3bd()
}

// bitThickness reads a thickness (BT): a bit for zero, else a BD.
func (r *bitReader) bitThickness() float64 {
	if !r.plain && r.bit() {
		return 0
	}
	return r.bd()
}

// entityHeaderR14 reads the rest of the common entity data of R13 and R14: the layer and the
// optional linetype come first in the handle stream, then the links to the previous and next
// entity, and the colour is a plain index.
func (o *object) entityHeaderR14(h entityHeader) entityHeader {
	m := o.main
	h.layer = o.hnd.handle(0)
	if !m.bit() { // not "by layer" linetype
		o.hnd.handle(0)
	}
	if m.bit() { // no links
		h.next = o.handle + 1
	} else {
		o.hnd.handle(o.handle) // previous entity
		h.next = o.hnd.handle(o.handle)
	}
	h.color = int(m.bs())
	m.bd() // linetype scale
	m.bs() // invisibility
	return h
}

// readTable reads a table control object: its entry handles in table order.
func (o *object) readTable() []uint64 {
	o.nonEntityHeader()
	n := int(o.main.bl())
	if n < 0 || n > o.hnd.bitsLeft()/8 {
		panic(errCorrupt)
	}
	hs := make([]uint64, 0, n)
	for range n {
		hs = append(hs, o.hnd.handle(0))
	}
	return hs
}
