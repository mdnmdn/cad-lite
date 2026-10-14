// Package dxf reads and writes ASCII DXF. The reader accepts R12 through
// current versions and keeps handles; the writer emits R12 (AC1009), the
// most widely readable version. Entities the document does not model are
// skipped with a warning.
package dxf

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
)

// Codec is the DXF reader and writer.
type Codec struct{}

// Extensions implements fileio.Reader and fileio.Writer.
func (Codec) Extensions() []string { return []string{".dxf"} }

// Versions implements fileio.Writer.
func (Codec) Versions() []string { return []string{"AC1009"} }

type pair struct {
	code int
	val  string
}

func readPairs(r io.Reader) ([]pair, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []pair
	line := 0
	for sc.Scan() {
		line++
		cs := strings.TrimSpace(sc.Text())
		if !sc.Scan() {
			return nil, fmt.Errorf("dxf: line %d: group code %q has no value", line, cs)
		}
		line++
		code, err := strconv.Atoi(cs)
		if err != nil {
			return nil, fmt.Errorf("dxf: line %d: bad group code %q (binary DXF is not supported)", line-1, cs)
		}
		out = append(out, pair{code, strings.TrimRight(sc.Text(), "\r")})
	}
	return out, sc.Err()
}

// object is one "0 TYPE" record and its group pairs.
type object struct {
	typ   string
	pairs []pair
}

func (o object) str(code int) string {
	for _, p := range o.pairs {
		if p.code == code {
			return strings.TrimSpace(p.val)
		}
	}
	return ""
}

func (o object) float(code int) float64 {
	v, _ := strconv.ParseFloat(o.str(code), 64)
	return v
}

func (o object) int(code int) int {
	v, _ := strconv.Atoi(o.str(code))
	return v
}

func (o object) has(code int) bool {
	for _, p := range o.pairs {
		if p.code == code {
			return true
		}
	}
	return false
}

// Read implements fileio.Reader.
func (Codec) Read(r io.Reader) (*doc.Drawing, []string, error) {
	ps, err := readPairs(r)
	if err != nil {
		return nil, nil, err
	}
	d := doc.New()
	d.Header.Extra = map[string][]string{}
	skipped := map[string]int{}
	var warns []string
	var section string
	var objs []object
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if p.code != 0 {
			if section == "" && p.code == 2 && len(objs) > 0 && objs[len(objs)-1].typ == "SECTION" {
				section = strings.TrimSpace(p.val)
				objs = objs[:0]
				if section == "HEADER" {
					// Header variables precede any 0 record; collect them in one.
					objs = append(objs, object{typ: "HEADER"})
				}
				continue
			}
			if len(objs) > 0 {
				objs[len(objs)-1].pairs = append(objs[len(objs)-1].pairs, p)
			}
			continue
		}
		typ := strings.TrimSpace(p.val)
		switch typ {
		case "SECTION":
			section = ""
			objs = append(objs[:0], object{typ: typ})
		case "ENDSEC":
			if err := applySection(d, section, objs, skipped); err != nil {
				return nil, nil, err
			}
			section, objs = "", objs[:0]
		case "EOF":
			i = len(ps)
		default:
			objs = append(objs, object{typ: typ})
		}
	}
	types := make([]string, 0, len(skipped))
	for t := range skipped {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		warns = append(warns, fmt.Sprintf("skipped %d %s entities (not supported)", skipped[t], t))
	}
	if d.Layer(d.CurrentLayer) == nil {
		d.CurrentLayer = "0"
	}
	return d, warns, nil
}

func applySection(d *doc.Drawing, section string, objs []object, skipped map[string]int) error {
	switch section {
	case "HEADER":
		readHeader(d, objs)
	case "TABLES":
		for _, o := range objs {
			if o.typ == "LAYER" {
				readLayer(d, o)
			}
		}
	case "ENTITIES":
		readEntities(d, objs, skipped)
	}
	return nil
}

func readHeader(d *doc.Drawing, objs []object) {
	if len(objs) == 0 {
		return
	}
	// The header is one record of "9 $NAME" followed by value groups.
	var name string
	for _, p := range objs[0].pairs {
		if p.code == 9 {
			name = strings.TrimSpace(p.val)
			continue
		}
		switch name {
		case "$HANDSEED":
			if h, err := doc.ParseHandle(strings.TrimSpace(p.val)); err == nil && h > d.Header.HandSeed {
				d.Header.HandSeed = h
			}
		case "$INSUNITS":
			d.Header.InsUnits, _ = strconv.Atoi(strings.TrimSpace(p.val))
		case "$CLAYER":
			d.CurrentLayer = strings.TrimSpace(p.val)
		default:
			d.Header.Extra[name] = append(d.Header.Extra[name], strconv.Itoa(p.code)+" "+p.val)
		}
	}
}

func readLayer(d *doc.Drawing, o object) {
	name := o.str(2)
	if name == "" {
		return
	}
	color := o.int(62)
	l := d.AddLayer(name, 7)
	l.Off = color < 0
	if color < 0 {
		color = -color
	}
	if color >= 1 && color <= 255 {
		l.Color = color
	}
	l.Locked = o.int(70)&4 != 0
}

func common(o object) doc.EntityCommon {
	c := doc.EntityCommon{Layer: o.str(8), Color: doc.ByLayer}
	if c.Layer == "" {
		c.Layer = "0"
	}
	if h, err := doc.ParseHandle(o.str(5)); err == nil {
		c.H = h
	}
	if o.has(62) {
		if v := o.int(62); v >= 1 && v <= 255 {
			c.Color = v
		}
	}
	return c
}

func vec(o object, xc int) geom.Vec2 { return geom.V(o.float(xc), o.float(xc+10)) }

func deg(v float64) float64 { return geom.NormAngle(v * math.Pi / 180) }

func readEntities(d *doc.Drawing, objs []object, skipped map[string]int) {
	for i := 0; i < len(objs); i++ {
		o := objs[i]
		c := common(o)
		var e doc.Entity
		switch o.typ {
		case "LINE":
			e = &doc.Line{EntityCommon: c, Start: vec(o, 10), End: vec(o, 11)}
		case "CIRCLE":
			e = &doc.Circle{EntityCommon: c, Center: vec(o, 10), Radius: o.float(40)}
		case "ARC":
			e = &doc.Arc{EntityCommon: c, Center: vec(o, 10), Radius: o.float(40), StartAngle: deg(o.float(50)), EndAngle: deg(o.float(51))}
		case "LWPOLYLINE":
			e = readLWPolyline(o, c)
		case "POLYLINE":
			var verts []object
			for i+1 < len(objs) && objs[i+1].typ == "VERTEX" {
				i++
				verts = append(verts, objs[i])
			}
			if i+1 < len(objs) && objs[i+1].typ == "SEQEND" {
				i++
			}
			if o.int(70)&(8|16|64) != 0 {
				skipped["3D POLYLINE/MESH"]++
				continue
			}
			p := &doc.LWPolyline{EntityCommon: c, Closed: o.int(70)&1 != 0}
			for _, v := range verts {
				p.Points = append(p.Points, vec(v, 10))
				p.Bulges = append(p.Bulges, v.float(42))
			}
			e = p
		default:
			skipped[o.typ]++
			continue
		}
		d.Load(e)
	}
}

func readLWPolyline(o object, c doc.EntityCommon) *doc.LWPolyline {
	p := &doc.LWPolyline{EntityCommon: c, Closed: o.int(70)&1 != 0}
	var x float64
	haveX := false
	for _, g := range o.pairs {
		v, _ := strconv.ParseFloat(strings.TrimSpace(g.val), 64)
		switch g.code {
		case 10:
			x, haveX = v, true
		case 20:
			if haveX {
				p.Points = append(p.Points, geom.V(x, v))
				p.Bulges = append(p.Bulges, 0)
				haveX = false
			}
		case 42:
			if n := len(p.Bulges); n > 0 {
				p.Bulges[n-1] = v
			}
		}
	}
	return p
}
