package dxf

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"fmt"
	"math"
	"strings"
)

func readModeledBlocks(d *doc.Drawing, objs []object, st *state) {
	for i := 0; i < len(objs); i++ {
		o := objs[i]
		if o.typ != "BLOCK" {
			continue
		}
		start := i
		i++
		for i < len(objs) && objs[i].typ != "ENDBLK" {
			i++
		}
		if i == len(objs) {
			st.warns = append(st.warns, "Incomplete block definition preserved")
			break
		}
		name := o.str(2)
		if strings.HasPrefix(strings.ToUpper(name), "*MODEL_SPACE") || strings.HasPrefix(strings.ToUpper(name), "*PAPER_SPACE") {
			continue
		}
		if o.int(70)&(4|8|16) != 0 {
			var raw []doc.Pair
			for _, r := range objs[start : i+1] {
				raw = append(raw, rawPairs(r)...)
			}
			d.Preserved.Blocks = append(d.Preserved.Blocks, doc.RawRecord{Pairs: raw})
			bumpSeed(d, raw)
			continue
		}
		tmp := doc.New()
		readEntities(tmp, objs[start+1:i], st)
		b := &doc.Block{Name: name, Base: vec(o, 10)}
		b.H, _ = doc.ParseHandle(o.str(330))
		if b.H == 0 {
			b.H, _ = doc.ParseHandle(o.str(5))
		}
		for _, h := range tmp.Handles() {
			b.Entities = append(b.Entities, tmp.Entity(h))
		}
		d.LoadBlock(b)
	}
}
func readInsert(o object, subs []object, c doc.EntityCommon) *doc.Insert {
	e := doc.NewInsert(o.str(2), vec(o, 10))
	e.EntityCommon = c
	e.Rotation = deg(o.float(50))
	e.Extra = extraFields(o, map[int]bool{2: true, 10: true, 20: true, 30: true, 41: true, 42: true, 43: true, 50: true, 66: true, 70: true, 71: true, 210: true, 220: true, 230: true})
	if o.has(41) {
		e.ScaleX = o.float(41)
	}
	if o.has(42) {
		e.ScaleY = o.float(42)
	}
	for _, a := range subs {
		if a.typ == "ATTRIB" {
			e.Attributes = append(e.Attributes, doc.Attribute{Tag: a.str(2), Value: decodeUnicode(a.str(1)), Text: readText(a, common(a)), Flags: a.int(70), Extra: extraFields(a, map[int]bool{1: true, 2: true, 7: true, 10: true, 20: true, 30: true, 11: true, 21: true, 31: true, 40: true, 41: true, 50: true, 51: true, 70: true, 71: true, 72: true, 73: true, 74: true})})
		}
	}
	return e
}
func readAttdef(o object, c doc.EntityCommon) *doc.AttributeDefinition {
	t := readText(o, c)
	t.VAlign = doc.TextVAlign(o.int(74))
	return &doc.AttributeDefinition{Text: *t, Tag: o.str(2), Prompt: o.str(3), Constant: o.int(70)&2 != 0, Invisible: o.int(70)&1 != 0}
}
func supportedInsert(o object) bool {
	return (!o.has(230) || o.float(230) == 1) && o.float(210) == 0 && o.float(220) == 0 && o.float(30) == 0 && o.int(70) <= 1 && o.int(71) <= 1 && (!o.has(41) || o.float(41) != 0) && (!o.has(42) || o.float(42) != 0)
}
func (w *writer) insert(e *doc.Insert, next func() string) {
	w.head("INSERT", e)
	if len(e.Attributes) > 0 {
		w.int(66, 1)
	}
	w.pair(2, e.BlockName)
	w.point(10, e.Position)
	w.num(41, e.ScaleX)
	w.num(42, e.ScaleY)
	w.num(43, 1)
	w.num(50, e.Rotation*180/math.Pi)
	w.raw(e.Extra)
	for _, a := range e.Attributes {
		var t *doc.Text
		if a.Text != nil {
			t = a.Text.Clone().(*doc.Text)
			t.Value = a.Value
		} else {
			t = doc.NewText(e.Position, 1, e.Rotation, a.Value)
			if w.d != nil {
				if b := w.d.Block(e.BlockName); b != nil {
					for _, child := range b.Entities {
						if def, ok := child.(*doc.AttributeDefinition); ok && strings.EqualFold(def.Tag, a.Tag) {
							if math.Abs(math.Abs(e.ScaleX)-math.Abs(e.ScaleY)) > 1e-10 {
								w.err = fmt.Errorf("cannot serialize new attributes on nonuniform insertion")
								return
							}
							m := geom.Translate(e.Position).Mul(geom.Rotate(geom.V(0, 0), e.Rotation)).Mul(geom.Mat3{e.ScaleX, 0, 0, e.ScaleY, 0, 0}).Mul(geom.Translate(b.Base.Scale(-1)))
							t = attributeText(def, a.Value, m)
							t.H = 0
							break
						}
					}
				}
			}
		}
		t.Owner = e.Handle()
		if t.Layer == "" || t.Layer == "0" {
			t.Layer = e.Layer
		}
		if t.H == 0 {
			t.H, _ = doc.ParseHandle(next())
		}
		w.typeOverride = "ATTRIB"
		w.text(t)
		if w.modern {
			w.pair(100, "AcDbAttribute")
		}
		w.pair(2, a.Tag)
		w.int(70, a.Flags)
		w.int(73, 0)
		w.int(74, int(t.VAlign))
		w.raw(a.Extra)
	}
	if len(e.Attributes) > 0 {
		c := *e.Common()
		c.Owner = e.Handle()
		w.headAs("SEQEND", next(), &c)
	}
}
func extraFields(o object, known map[int]bool) []doc.Pair {
	base := map[int]bool{5: true, 8: true, 62: true, 330: true, 100: true, 102: true}
	var out []doc.Pair
	for _, p := range o.pairs {
		if !base[p.code] && !known[p.code] {
			out = append(out, doc.Pair{Code: p.code, Value: p.val})
		}
	}
	return out
}
func (w *writer) attdef(e *doc.AttributeDefinition) {
	w.text(&e.Text)
	if w.modern {
		w.pair(100, "AcDbAttributeDefinition")
	}
	w.pair(3, e.Prompt)
	w.pair(2, e.Tag)
	flags := 0
	if e.Invisible {
		flags |= 1
	}
	if e.Constant {
		flags |= 2
	}
	w.int(70, flags)
	w.int(74, int(e.VAlign))
}
func (w *writer) nativeMText(e *doc.MText) {
	w.head("MTEXT", e)
	w.point(10, e.Insert)
	w.num(40, e.Height)
	w.num(41, e.Width)
	w.int(71, e.Attachment)
	w.num(50, e.Rotation*180/3.141592653589793)
	w.num(44, e.LineSpacing)
	w.pair(7, e.Style)
	value := e.Value
	for len(value) > 200 {
		w.pair(3, value[:200])
		value = value[200:]
	}
	w.pair(1, value)
}
func attributeText(e *doc.AttributeDefinition, value string, m geom.Mat3) *doc.Text {
	t := e.Text.Clone().(*doc.Text)
	t.Value = value
	t.Transform(m)
	return t
}
