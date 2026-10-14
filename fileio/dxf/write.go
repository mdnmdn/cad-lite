package dxf

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
)

type writer struct {
	w   *bufio.Writer
	err error
}

func (w *writer) pair(code int, val string) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, "%3d\n%s\n", code, val)
}

func (w *writer) num(code int, v float64) { w.pair(code, strconv.FormatFloat(v, 'f', -1, 64)) }

func (w *writer) int(code, v int) { w.pair(code, strconv.Itoa(v)) }

func (w *writer) point(code int, p geom.Vec2) {
	w.num(code, p.X)
	w.num(code+10, p.Y)
	w.num(code+20, 0)
}

// Write implements fileio.Writer. It writes R12 (AC1009): LWPOLYLINE becomes
// POLYLINE/VERTEX/SEQEND and entity handles are kept. VERTEX and SEQEND get
// handles above the drawing's seed, which are not part of the document.
// Header variables the document does not model are not written.
//
// Unknown entities, and the tables and blocks in Drawing.Preserved, are
// written verbatim, group for group. R12 has no SPLINE, MTEXT, HATCH and the
// like, so an unknown entity whose class is not in r12Entities is not
// written and a warning names it; the other classes are written as read,
// without the groups R12 lacks (see pairs). Only the CONTINUOUS linetype, the
// layers and the entities are generated.
func (Codec) Write(out io.Writer, d *doc.Drawing) ([]string, error) {
	w := &writer{w: bufio.NewWriter(out)}
	var warns []string
	sub := d.Header.HandSeed
	seed := sub
	for _, h := range d.Handles() {
		if p, ok := d.Entity(h).(*doc.LWPolyline); ok {
			seed += doc.Handle(len(p.Points) + 1)
		}
	}
	nextSub := func() string { h := sub; sub++; return h.String() }

	w.pair(0, "SECTION")
	w.pair(2, "HEADER")
	w.pair(9, "$ACADVER")
	w.pair(1, "AC1009")
	w.pair(9, "$CLAYER")
	w.pair(8, d.CurrentLayer)
	w.pair(9, "$HANDLING")
	w.int(70, 1)
	w.pair(9, "$HANDSEED")
	w.pair(5, seed.String())
	w.pair(0, "ENDSEC")

	w.pair(0, "SECTION")
	w.pair(2, "TABLES")
	w.rawTable(d, "VPORT")
	w.pair(0, "TABLE")
	w.pair(2, "LTYPE")
	w.int(70, 1+len(d.Preserved.Tables["LTYPE"]))
	w.pair(0, "LTYPE")
	w.pair(2, "CONTINUOUS")
	w.int(70, 0)
	w.pair(3, "Solid line")
	w.int(72, 65)
	w.int(73, 0)
	w.num(40, 0)
	w.records(d.Preserved.Tables["LTYPE"])
	w.pair(0, "ENDTAB")
	w.pair(0, "TABLE")
	w.pair(2, "LAYER")
	w.int(70, len(d.Layers()))
	for _, l := range d.Layers() {
		w.pair(0, "LAYER")
		w.pair(2, l.Name)
		flags := 0
		if l.Locked {
			flags |= 4
		}
		w.int(70, flags)
		c := l.Color
		if l.Off {
			c = -c
		}
		w.int(62, c)
		w.pair(6, "CONTINUOUS")
	}
	w.pair(0, "ENDTAB")
	for _, t := range []string{"STYLE", "VIEW", "UCS", "APPID", "DIMSTYLE"} {
		w.rawTable(d, t)
	}
	w.pair(0, "ENDSEC")

	if len(d.Preserved.Blocks) > 0 {
		w.pair(0, "SECTION")
		w.pair(2, "BLOCKS")
		for _, b := range d.Preserved.Blocks {
			warns = w.block(warns, b)
		}
		w.pair(0, "ENDSEC")
	}

	w.pair(0, "SECTION")
	w.pair(2, "ENTITIES")
	for _, h := range d.Handles() {
		e := d.Entity(h)
		switch e := e.(type) {
		case *doc.Line:
			w.head("LINE", e)
			w.point(10, e.Start)
			w.point(11, e.End)
		case *doc.Circle:
			w.head("CIRCLE", e)
			w.point(10, e.Center)
			w.num(40, e.Radius)
		case *doc.Arc:
			w.head("ARC", e)
			w.point(10, e.Center)
			w.num(40, e.Radius)
			w.num(50, e.StartAngle*180/math.Pi)
			w.num(51, e.EndAngle*180/math.Pi)
		case *doc.LWPolyline:
			w.head("POLYLINE", e)
			w.int(66, 1)
			w.point(10, geom.V(0, 0))
			flags := 0
			if e.Closed {
				flags = 1
			}
			w.int(70, flags)
			for i, p := range e.Points {
				w.pair(0, "VERTEX")
				w.pair(5, nextSub())
				w.pair(8, e.Layer)
				w.point(10, p)
				if i < len(e.Bulges) && e.Bulges[i] != 0 {
					w.num(42, e.Bulges[i])
				}
			}
			w.pair(0, "SEQEND")
			w.pair(5, nextSub())
			w.pair(8, e.Layer)
		case *doc.Unknown:
			if !r12Entities[e.Type] {
				warns = append(warns, fmt.Sprintf("%s %s not written (the class does not exist in R12; it needs the newer writer, backlog G2)", e.Type, h))
				continue
			}
			w.pairs(e.Source())
		default:
			warns = append(warns, fmt.Sprintf("%s %s not written (not supported by the R12 writer)", e.Class(), h))
		}
	}
	w.pair(0, "ENDSEC")
	w.pair(0, "EOF")
	if w.err == nil {
		w.err = w.w.Flush()
	}
	return warns, w.err
}

func (w *writer) head(typ string, e doc.Entity) {
	c := e.Common()
	w.pair(0, typ)
	w.pair(5, c.H.String())
	w.pair(8, c.Layer)
	if c.Color != doc.ByLayer && c.Color > 0 {
		w.int(62, c.Color)
	}
}

// r12Entities are the entity classes R12 (AC1009) defines. An unknown entity
// of another class cannot be written to an R12 file.
var r12Entities = map[string]bool{
	"LINE": true, "POINT": true, "CIRCLE": true, "ARC": true, "TRACE": true, "SOLID": true,
	"TEXT": true, "SHAPE": true, "INSERT": true, "ATTDEF": true, "ATTRIB": true,
	"POLYLINE": true, "VERTEX": true, "SEQEND": true, "3DLINE": true, "3DFACE": true,
	"VIEWPORT": true, "DIMENSION": true,
}

// pairs writes ps, leaving out the groups that only exist from R13 on:
// subclass markers (100), reactor braces (102), owner and extension
// dictionary handles (330, 360). A strict R12 reader cannot place a record
// that has them. Every other group is written as it was read.
var r12Blocks = map[string]bool{"BLOCK": true, "ENDBLK": true}

func (w *writer) pairs(ps []doc.Pair) {
	for _, p := range ps {
		switch p.Code {
		case 100, 102, 330, 360:
		default:
			w.pair(p.Code, p.Value)
		}
	}
}

func (w *writer) records(rs []doc.RawRecord) {
	for _, r := range rs {
		w.pairs(r.Pairs)
	}
}

// block writes a preserved block, leaving out the entities whose class R12
// does not have; each one adds a warning.
func (w *writer) block(warns []string, b doc.RawRecord) []string {
	ps := b.Pairs
	for len(ps) > 0 {
		n := 1
		for n < len(ps) && ps[n].Code != 0 {
			n++
		}
		typ := strings.TrimSpace(ps[0].Value)
		if r12Blocks[typ] || r12Entities[typ] {
			w.pairs(ps[:n])
		} else {
			warns = append(warns, fmt.Sprintf("%s in block %s not written (the class does not exist in R12; it needs the newer writer, backlog G2)", typ, b.Name()))
		}
		ps = ps[n:]
	}
	return warns
}

// rawTable writes the preserved records of table name as a table, or
// nothing when there are none.
func (w *writer) rawTable(d *doc.Drawing, name string) {
	rs := d.Preserved.Tables[name]
	if len(rs) == 0 {
		return
	}
	w.pair(0, "TABLE")
	w.pair(2, name)
	w.int(70, len(rs))
	w.records(rs)
	w.pair(0, "ENDTAB")
}
