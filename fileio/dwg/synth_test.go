package dwg

import (
	"math"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

// synth builds object records of R2000 (ver verR15) or R2004 (verR18) for the importer without a
// DWG file: the fixtures hold no 2D POLYLINE, no mirrored entity and no paper space entity.
type synth struct {
	ver  int
	data []byte
	omap map[uint64]int
	// next gives an entity explicit links to its neighbours (R2000 only): the value is its
	// next handle, the previous one being the handle before it.
	next map[uint64]uint64
}

func newSynth(ver int) *synth {
	// Offset 0 is never an object, so a zero offset cannot be mistaken for one.
	return &synth{ver: ver, data: make([]byte, 8), omap: map[uint64]int{}, next: map[uint64]uint64{}}
}

const synthLayer = 0x10 // the handle every synthetic entity names as its layer

// entity adds an entity record: body writes its type-specific data, handles its handle stream
// after the layer handle.
func (s *synth) entity(handle uint64, typ int, mode byte, owner uint64, body func(w *bitWriter), handles func(w *bitWriter)) {
	w := &bitWriter{}
	w.bs(int16(typ))
	sizePos := w.n
	w.rl(0)
	w.handle(0, handle)
	w.bs(0)      // no extended data
	w.bit(false) // no graphic
	w.bb(mode)
	w.bl(0) // reactors
	nxt, linked := s.next[handle]
	if s.ver >= verR18 {
		w.bit(true) // no extension dictionary
	} else {
		w.bit(!linked) // no links
	}
	w.bs(256)
	w.bd(1) // linetype scale
	w.bb(0) // linetype flags
	w.bb(0) // plot style flags
	w.bs(0) // invisibility
	w.rc(0) // line weight
	body(w)
	handleStart := w.n
	if mode == 0 {
		w.handle(4, owner)
	}
	if s.ver < verR18 {
		w.rc(0) // extension dictionary handle (null)
		if linked {
			w.handle(4, handle-1)
			w.handle(4, nxt)
		}
	}
	w.handle(5, synthLayer)
	if handles != nil {
		handles(w)
	}
	// Patch the size of the data before the handles.
	patch := &bitWriter{buf: w.buf, n: sizePos}
	patch.rl(int32(handleStart))
	w.pad()
	prefix := &bitWriter{}
	prefix.ms(len(w.buf))
	rec := append(append([]byte(nil), prefix.buf...), w.buf...)
	crc := crc16(0xC0C1, rec)
	rec = append(rec, byte(crc), byte(crc>>8))
	s.omap[handle] = len(s.data)
	s.data = append(s.data, rec...)
}

// read runs the importer over the records.
func (s *synth) read(msBlock uint64) (*doc.Drawing, []string, *importer) {
	imp := &importer{
		src:  &objSource{data: s.data, ver: s.ver, cp: 30, classes: map[int]class{}},
		omap: s.omap, ltNames: map[uint64]string{}, skipped: map[string]int{}, budget: 2*len(s.omap) + 16,
		msBlock: msBlock,
		layers:  []layerRec{{handle: synthLayer, layer: doc.Layer{Name: "walls", Color: 3}}},
	}
	imp.run()
	d, warns := imp.build(headerVars{})
	return d, warns, imp
}

func (s *synth) lwpolyline(handle uint64, flags int16, constWidth float64, normal [3]float64, pts [][2]float64, bulges []float64, widths [][2]float64) {
	s.entity(handle, tLWPolyline, 2, 0, func(w *bitWriter) {
		w.bs(flags)
		if flags&4 != 0 {
			w.bd(constWidth)
		}
		if flags&1 != 0 {
			w.bd(normal[0])
			w.bd(normal[1])
			w.bd(normal[2])
		}
		w.bl(int32(len(pts)))
		if flags&0x10 != 0 {
			w.bl(int32(len(bulges)))
		}
		if flags&0x20 != 0 {
			w.bl(int32(len(widths)))
		}
		for i, p := range pts {
			if i == 0 {
				w.rd(p[0])
				w.rd(p[1])
				continue
			}
			w.dd(p[0], pts[i-1][0])
			w.dd(p[1], pts[i-1][1])
		}
		for _, b := range bulges {
			w.bd(b)
		}
		for _, wd := range widths {
			w.bd(wd[0])
			w.bd(wd[1])
		}
	}, nil)
}

func TestSynthLWPolyline(t *testing.T) {
	for _, ver := range []int{verR15, verR18} {
		s := newSynth(ver)
		// A closed polyline with bulges and per-vertex widths.
		s.lwpolyline(0x100, 0x200|0x10|0x20, 0, [3]float64{}, [][2]float64{{0, 0}, {10, 0}, {10, 5}},
			[]float64{0, 0.5, -1}, [][2]float64{{1, 2}, {0, 0}, {3, 3}})
		// A constant width and no bulges.
		s.lwpolyline(0x101, 4, 0.25, [3]float64{}, [][2]float64{{1, 1}, {2, 2}}, nil, nil)
		d, warns, _ := s.read(0)
		if len(warns) != 0 {
			t.Errorf("R%d: warnings %v", ver, warns)
		}
		p, _ := d.Entity(0x100).(*doc.LWPolyline)
		if p == nil {
			t.Fatalf("R%d: no LWPOLYLINE 100 among %d entities", ver, d.Len())
		}
		if !p.Closed || len(p.Points) != 3 || p.Points[2] != geom.V(10, 5) || p.Bulges[1] != 0.5 || p.Bulges[2] != -1 {
			t.Errorf("R%d: polyline = %+v", ver, p)
		}
		if p.Widths[0] != [2]float64{1, 2} || p.Widths[2] != [2]float64{3, 3} || p.Layer != "walls" || p.Color != doc.ByLayer {
			t.Errorf("R%d: widths %v layer %q colour %d", ver, p.Widths, p.Layer, p.Color)
		}
		c, _ := d.Entity(0x101).(*doc.LWPolyline)
		if c == nil || c.Closed || len(c.Widths) != 2 || c.Widths[1] != [2]float64{0.25, 0.25} || c.Bulges[0] != 0 {
			t.Errorf("R%d: constant width polyline = %+v", ver, c)
		}
	}
}

func TestSynthMirroredEntities(t *testing.T) {
	// An extrusion of (0,0,-1) mirrors X: the entity is stored in its own coordinates.
	s := newSynth(verR15)
	s.lwpolyline(0x100, 1|0x10, 0, [3]float64{0, 0, -1}, [][2]float64{{2, 3}, {5, 3}}, []float64{0.5, 0}, nil)
	s.entity(0x101, tCircle, 2, 0, func(w *bitWriter) {
		w.bd(4)
		w.bd(7)
		w.bd(0)
		w.bd(2.5)
		w.bit(true) // thickness 0
		w.bit(false)
		w.bd(0)
		w.bd(0)
		w.bd(-1)
	}, nil)
	s.entity(0x102, tArc, 2, 0, func(w *bitWriter) {
		w.bd(4)
		w.bd(7)
		w.bd(0)
		w.bd(2.5)
		w.bit(true)
		w.bit(false)
		w.bd(0)
		w.bd(0)
		w.bd(-1)
		w.bd(0.5)
		w.bd(1)
	}, nil)
	// An arc with a tilted extrusion is read as if the normal were +Z, as the DXF reader does.
	s.entity(0x103, tArc, 2, 0, func(w *bitWriter) {
		w.bd(4)
		w.bd(7)
		w.bd(0)
		w.bd(2.5)
		w.bit(true)
		w.bit(false)
		w.bd(0.6)
		w.bd(0)
		w.bd(-0.8)
		w.bd(0.5)
		w.bd(1)
	}, nil)
	d, _, _ := s.read(0)
	p := d.Entity(0x100).(*doc.LWPolyline)
	if p.Points[0] != geom.V(-2, 3) || p.Points[1] != geom.V(-5, 3) || p.Bulges[0] != -0.5 {
		t.Errorf("mirrored polyline = %+v", p)
	}
	if c := d.Entity(0x101).(*doc.Circle); c.Center != geom.V(-4, 7) || c.Radius != 2.5 {
		t.Errorf("mirrored circle = %+v", c)
	}
	a := d.Entity(0x102).(*doc.Arc)
	if a.Center != geom.V(-4, 7) || math.Abs(a.StartAngle-(math.Pi-1)) > 1e-12 || math.Abs(a.EndAngle-(math.Pi-0.5)) > 1e-12 {
		t.Errorf("mirrored arc = %+v", a)
	}
	if a := d.Entity(0x103).(*doc.Arc); a.Center != geom.V(4, 7) || a.StartAngle != 0.5 || a.EndAngle != 1 {
		t.Errorf("tilted arc = %+v", a)
	}
}

// polyline2D writes a POLYLINE_2D at handle h with vertex records following it, in the form of
// the version (a first/last pair before R2004, an owned list from it).
func (s *synth) polyline2D(h uint64, flags int16, startW, endW float64, vs []vertexSpec) {
	n := len(vs)
	s.entity(h, tPolyline2D, 2, 0, func(w *bitWriter) {
		w.bs(flags)
		w.bs(0)
		w.bd(startW)
		w.bd(endW)
		w.bit(true) // thickness
		w.bd(0)     // elevation
		w.bit(true) // extrusion
		if s.ver >= verR18 {
			w.bl(int32(n))
		}
	}, func(w *bitWriter) {
		if s.ver < verR18 {
			w.handle(4, h+1)
			w.handle(4, h+uint64(n))
		} else {
			for i := range n {
				w.handle(4, h+1+uint64(i))
			}
		}
		w.handle(4, h+uint64(n)+1) // SEQEND
	})
	for i, v := range vs {
		s.entity(h+1+uint64(i), tVertex2D, 0, h, func(w *bitWriter) {
			w.rc(v.flags)
			w.bd(v.x)
			w.bd(v.y)
			w.bd(0)
			if v.startW < 0 {
				w.bd(v.startW)
			} else {
				w.bd(v.startW)
				w.bd(v.endW)
			}
			w.bd(v.bulge)
			w.bd(0) // tangent direction
		}, nil)
	}
	s.entity(h+uint64(n)+1, tSeqend, 0, h, func(w *bitWriter) {}, nil)
}

type vertexSpec struct {
	x, y, bulge  float64
	startW, endW float64
	flags        byte
}

func TestSynthPolyline2D(t *testing.T) {
	for _, ver := range []int{verR15, verR18} {
		s := newSynth(ver)
		s.polyline2D(0x200, 1, 0.5, 0.5, []vertexSpec{
			{x: 0, y: 0, bulge: 0.25},
			{x: 4, y: 0, startW: 2, endW: 1},
			{x: 4, y: 3, startW: -1.5}, // a negative start width stands for both
			{x: 9, y: 9, flags: 16},    // a spline frame control point is not on the curve
			{x: 0, y: 3},
		})
		d, warns, _ := s.read(0)
		if len(warns) != 0 {
			t.Errorf("R%d: warnings %v", ver, warns)
		}
		if d.Len() != 1 {
			t.Fatalf("R%d: %d entities, want 1 (vertices and SEQEND are not entities of their own)", ver, d.Len())
		}
		p, _ := d.Entity(0x200).(*doc.LWPolyline)
		if p == nil {
			t.Fatalf("R%d: entity is %T", ver, d.Entity(0x200))
		}
		if !p.Closed || len(p.Points) != 4 {
			t.Fatalf("R%d: closed %v, %d points: %+v", ver, p.Closed, len(p.Points), p)
		}
		if p.Points[1] != geom.V(4, 0) || p.Points[3] != geom.V(0, 3) || p.Bulges[0] != 0.25 || p.Bulges[1] != 0 {
			t.Errorf("R%d: polyline = %+v", ver, p)
		}
		want := [][2]float64{{0.5, 0.5}, {2, 1}, {1.5, 1.5}, {0.5, 0.5}}
		for i, w := range want {
			if p.Widths[i] != w {
				t.Errorf("R%d: widths = %v, want %v", ver, p.Widths, want)
				break
			}
		}
	}
}

func TestSynthPolylineKinds(t *testing.T) {
	s := newSynth(verR15)
	s.polyline2D(0x200, 8, 0, 0, []vertexSpec{{x: 0, y: 0}, {x: 1, y: 1}})  // 3D polyline flag
	s.polyline2D(0x300, 64, 0, 0, []vertexSpec{{x: 0, y: 0}, {x: 1, y: 1}}) // polyface mesh flag
	s.polyline2D(0x400, 0, 0, 0, []vertexSpec{{x: 0, y: 0}, {x: 1, y: 1}})
	d, warns, _ := s.read(0)
	if d.Len() != 1 || d.Entity(0x400) == nil {
		t.Errorf("%d entities; only the plain polyline belongs", d.Len())
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "2 POLYLINE") {
		t.Errorf("warnings = %q, want a summary of 2 POLYLINE", warns)
	}
	if p := d.Entity(0x400).(*doc.LWPolyline); p.Closed || len(p.Widths) != 0 {
		t.Errorf("open, zero-width polyline = %+v", p)
	}
}

func TestSynthPolylineChainCycle(t *testing.T) {
	// Vertices whose next links point at each other must end the chain, not loop.
	s := newSynth(verR15)
	s.entity(0x200, tPolyline2D, 2, 0, func(w *bitWriter) {
		w.bs(0)
		w.bs(0)
		w.bd(0)
		w.bd(0)
		w.bit(true)
		w.bd(0)
		w.bit(true)
	}, func(w *bitWriter) {
		w.handle(4, 0x201)
		w.handle(4, 0x2FF) // a last vertex the chain never reaches
		w.handle(4, 0x202)
	})
	s.next[0x201], s.next[0x202] = 0x202, 0x201
	for i := range 2 {
		s.entity(uint64(0x201+i), tVertex2D, 0, 0x200, func(w *bitWriter) {
			w.rc(0)
			w.bd(float64(i + 1))
			w.bd(0)
			w.bd(0)
			w.bd(0)
			w.bd(0)
			w.bd(0)
			w.bd(0)
		}, nil)
	}
	d, _, _ := s.read(0)
	p, _ := d.Entity(0x200).(*doc.LWPolyline)
	if p == nil || len(p.Points) != 2 || p.Points[1] != geom.V(2, 0) {
		t.Errorf("polyline = %+v", p)
	}
}

func TestSynthModelAndPaperSpace(t *testing.T) {
	s := newSynth(verR15)
	line := func(w *bitWriter) {
		w.bit(true)
		w.rd(0)
		w.dd(1, 0)
		w.rd(0)
		w.dd(1, 0)
		w.bit(true)
		w.bit(true)
	}
	s.entity(0x100, tLine, 2, 0, line, nil)    // model space
	s.entity(0x101, tLine, 1, 0, line, nil)    // paper space
	s.entity(0x102, tLine, 0, 0x1F, line, nil) // owner given: the model space block record
	s.entity(0x103, tLine, 0, 0x99, line, nil) // owned by some block definition
	d, warns, _ := s.read(0x1F)
	if d.Entity(0x100) == nil || d.Entity(0x102) == nil || d.Entity(0x101) != nil || d.Entity(0x103) != nil || d.Len() != 2 {
		t.Errorf("model space entities = %d, want 0x100 and 0x102 only", d.Len())
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "1 paper space") {
		t.Errorf("warnings = %q, want one paper space entity", warns)
	}
	// Without the model space block record the owner rule cannot apply.
	d, _, _ = s.read(0)
	if d.Entity(0x102) != nil {
		t.Error("an entity owned by an unknown block was taken for model space")
	}
}

func TestSynthDamagedObjects(t *testing.T) {
	s := newSynth(verR15)
	s.entity(0x100, tLine, 2, 0, func(w *bitWriter) { w.bit(true); w.rd(0) }, nil) // ends mid-record
	s.entity(0x101, tLine, 2, 0, func(w *bitWriter) {
		w.bit(true)
		w.rd(math.NaN())
		w.dd(1, 0)
		w.rd(0)
		w.dd(1, 0)
		w.bit(true)
		w.bit(true)
	}, nil)
	ok := func(w *bitWriter) {
		w.bit(true)
		w.rd(0)
		w.dd(5, 0)
		w.rd(0)
		w.dd(5, 0)
		w.bit(true)
		w.bit(true)
	}
	s.entity(0x102, tLine, 2, 0, ok, nil)
	// A bad CRC drops the object.
	s.entity(0x103, tLine, 2, 0, ok, nil)
	s.data[s.omap[0x103]+10] ^= 0xFF
	d, warns, _ := s.read(0)
	if d.Len() != 1 || d.Entity(0x102) == nil {
		t.Errorf("%d entities, want only 0x102", d.Len())
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "CRC") || !strings.Contains(joined, "could not be read") {
		t.Errorf("warnings = %q", warns)
	}
}

func TestLayerTableOrder(t *testing.T) {
	imp := &importer{layers: []layerRec{
		{handle: 0x10, layer: doc.Layer{Name: "a"}},
		{handle: 0x11, layer: doc.Layer{Name: "b"}},
		{handle: 0x12, layer: doc.Layer{Name: "c"}},
		{handle: 0x13, layer: doc.Layer{Name: "d"}},
	}}
	names := func() string {
		var s []string
		for _, l := range imp.orderedLayers() {
			s = append(s, l.layer.Name)
		}
		return strings.Join(s, "")
	}
	if got := names(); got != "abcd" {
		t.Errorf("without a control object: %s", got)
	}
	// The control object's list decides the order; layers it omits follow, in handle order; handles
	// that name no layer and repeated handles are ignored.
	imp.layerOrder = []uint64{0x12, 0x99, 0x10, 0x12}
	if got := names(); got != "cabd" {
		t.Errorf("with a control object: %s", got)
	}
}
