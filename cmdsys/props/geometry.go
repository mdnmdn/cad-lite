package props

import (
	"errors"
	"math"
	"strings"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
)

// This file holds the type-specific rows: geometry for lines, circles, arcs and polylines, the
// text rows, and block reference placement and attributes. Text edits follow ui's text editor
// (ui/textedit.go) exactly, so the panel and the editor agree.

// errInvalid is a value a row's set refuses.
var errInvalid = errors.New("props: invalid value")

// num builds a numeric row of kind k over a getter and an optional setter (nil: read-only).
func num(id string, k Kind, get func(doc.Entity) float64, set func(doc.Entity, float64) error) *def {
	d := &def{id: id, kind: k, num: get, get: func(_ *env, e doc.Entity) Value { return NumValue(get(e)) }}
	if set == nil {
		d.readOnly = true
	} else {
		d.set = func(_ *env, e doc.Entity, v Value) error { return set(e, v.Num) }
	}
	return d
}

func positive(d *def) *def { d.positive = true; return d }

func bounded(d *def, lo, hi float64) *def { d.min, d.max = lo, hi; return d }

// point builds the X and Y rows of a point.
func point(prefix string, get func(doc.Entity) geom.Vec2, set func(doc.Entity, geom.Vec2)) []*def {
	return []*def{
		num(prefix+"_x", KindLength, func(e doc.Entity) float64 { return get(e).X },
			func(e doc.Entity, v float64) error { p := get(e); p.X = v; set(e, p); return nil }),
		num(prefix+"_y", KindLength, func(e doc.Entity) float64 { return get(e).Y },
			func(e doc.Entity, v float64) error { p := get(e); p.Y = v; set(e, p); return nil }),
	}
}

func cat(id string, defs ...[]*def) catDef {
	c := catDef{id: id, title: "ui.props.cat." + id}
	for _, d := range defs {
		c.defs = append(c.defs, d...)
	}
	return c
}

func one(d ...*def) []*def { return d }

// --- LINE ----------------------------------------------------------------

func line(e doc.Entity) *doc.Line { return e.(*doc.Line) }

var lineCats = []catDef{cat("geometry",
	point("start", func(e doc.Entity) geom.Vec2 { return line(e).Start }, func(e doc.Entity, p geom.Vec2) { line(e).Start = p }),
	point("end", func(e doc.Entity) geom.Vec2 { return line(e).End }, func(e doc.Entity, p geom.Vec2) { line(e).End = p }),
	one(
		num("delta_x", KindLength, func(e doc.Entity) float64 { return line(e).End.X - line(e).Start.X }, nil),
		num("delta_y", KindLength, func(e doc.Entity) float64 { return line(e).End.Y - line(e).Start.Y }, nil),
		num("length", KindLength, func(e doc.Entity) float64 { return line(e).Start.Dist(line(e).End) }, nil),
		num("angle", KindAngle, func(e doc.Entity) float64 { return geom.NormAngle(line(e).End.Sub(line(e).Start).Angle()) }, nil),
	))}

// --- CIRCLE --------------------------------------------------------------

func circle(e doc.Entity) *doc.Circle { return e.(*doc.Circle) }

// setRadius is the one write of every circle size row.
func setRadius(e doc.Entity, r float64) error {
	if !(r > 0) || math.IsInf(r, 0) {
		return errInvalid
	}
	circle(e).Radius = r
	return nil
}

var circleCats = []catDef{cat("geometry",
	point("center", func(e doc.Entity) geom.Vec2 { return circle(e).Center }, func(e doc.Entity, p geom.Vec2) { circle(e).Center = p }),
	one(
		positive(num("radius", KindLength, func(e doc.Entity) float64 { return circle(e).Radius }, setRadius)),
		positive(num("diameter", KindLength, func(e doc.Entity) float64 { return 2 * circle(e).Radius },
			func(e doc.Entity, v float64) error { return setRadius(e, v/2) })),
		positive(num("circumference", KindLength, func(e doc.Entity) float64 { return 2 * math.Pi * circle(e).Radius },
			func(e doc.Entity, v float64) error { return setRadius(e, v/(2*math.Pi)) })),
		positive(num("area", KindNumber, func(e doc.Entity) float64 { return math.Pi * circle(e).Radius * circle(e).Radius },
			func(e doc.Entity, v float64) error { return setRadius(e, math.Sqrt(v/math.Pi)) })),
	))}

// --- ARC -----------------------------------------------------------------

func arc(e doc.Entity) *doc.Arc { return e.(*doc.Arc) }

func arcCurve(e doc.Entity) geom.Arc {
	a := arc(e)
	return geom.Arc{C: a.Center, R: a.Radius, Start: a.StartAngle, End: a.EndAngle}
}

var arcCats = []catDef{cat("geometry",
	point("center", func(e doc.Entity) geom.Vec2 { return arc(e).Center }, func(e doc.Entity, p geom.Vec2) { arc(e).Center = p }),
	one(
		positive(num("radius", KindLength, func(e doc.Entity) float64 { return arc(e).Radius },
			func(e doc.Entity, v float64) error { arc(e).Radius = v; return nil })),
		num("start_angle", KindAngle, func(e doc.Entity) float64 { return arc(e).StartAngle },
			func(e doc.Entity, v float64) error { arc(e).StartAngle = geom.NormAngle(v); return nil }),
		num("end_angle", KindAngle, func(e doc.Entity) float64 { return arc(e).EndAngle },
			func(e doc.Entity, v float64) error { arc(e).EndAngle = geom.NormAngle(v); return nil }),
		num("total_angle", KindAngle, func(e doc.Entity) float64 { return arcCurve(e).Sweep() }, nil),
		num("arc_length", KindLength, func(e doc.Entity) float64 { return arcCurve(e).Length() }, nil),
		num("area", KindNumber, func(e doc.Entity) float64 {
			a := arcCurve(e)
			return geom.ArcSegmentArea(a.R, a.Sweep())
		}, nil),
	))}

// --- LWPOLYLINE ----------------------------------------------------------

func pline(e doc.Entity) *doc.LWPolyline { return e.(*doc.LWPolyline) }

// hasVertex limits the vertex rows to polylines that have the selected vertex.
func (c *env) hasVertex(e doc.Entity) bool { return c.vertex < len(pline(e).Points) }

// width returns the start (0) or end (1) width of the segment leaving vertex i.
func width(p *doc.LWPolyline, i, side int) float64 {
	if i < len(p.Widths) {
		return p.Widths[i][side]
	}
	return 0
}

// setWidth sets one width, growing Widths to the vertex count.
func setWidth(p *doc.LWPolyline, i, side int, w float64) error {
	if !(w >= 0) || math.IsInf(w, 0) {
		return errInvalid
	}
	for len(p.Widths) < len(p.Points) {
		p.Widths = append(p.Widths, [2]float64{})
	}
	p.Widths[i][side] = w
	return nil
}

func vertexNum(id string, get func(c *env, p *doc.LWPolyline) float64, set func(c *env, p *doc.LWPolyline, v float64) error) *def {
	return &def{id: id, kind: KindLength,
		get:  func(c *env, e doc.Entity) Value { return NumValue(get(c, pline(e))) },
		set:  func(c *env, e doc.Entity, v Value) error { return set(c, pline(e), v.Num) },
		only: func(c *env, e doc.Entity) bool { return c.hasVertex(e) }}
}

func plineCats() []catDef {
	vx := vertexNum("vertex_x", func(c *env, p *doc.LWPolyline) float64 { return p.Points[c.vertex].X },
		func(c *env, p *doc.LWPolyline, v float64) error { p.Points[c.vertex].X = v; return nil })
	vy := vertexNum("vertex_y", func(c *env, p *doc.LWPolyline) float64 { return p.Points[c.vertex].Y },
		func(c *env, p *doc.LWPolyline, v float64) error { p.Points[c.vertex].Y = v; return nil })
	sw := vertexNum("start_width", func(c *env, p *doc.LWPolyline) float64 { return width(p, c.vertex, 0) },
		func(c *env, p *doc.LWPolyline, v float64) error { return setWidth(p, c.vertex, 0, v) })
	ew := vertexNum("end_width", func(c *env, p *doc.LWPolyline) float64 { return width(p, c.vertex, 1) },
		func(c *env, p *doc.LWPolyline, v float64) error { return setWidth(p, c.vertex, 1, v) })
	vertexRows := []*def{vx, vy, sw, ew}
	selector := &def{id: "vertex", kind: KindInt, selector: true,
		get: func(c *env, e doc.Entity) Value { return IntValue(min(c.vertex, max(len(pline(e).Points)-1, 0))) }}
	global := &def{id: "global_width", kind: KindLength,
		get: func(_ *env, e doc.Entity) Value {
			p := pline(e)
			w := width(p, 0, 0)
			for i := range p.Points {
				if width(p, i, 0) != w || width(p, i, 1) != w {
					return Value{Varies: true}
				}
			}
			return NumValue(w)
		},
		set: func(_ *env, e doc.Entity, v Value) error {
			p := pline(e)
			if !(v.Num >= 0) || math.IsInf(v.Num, 0) {
				return errInvalid
			}
			p.Widths = nil
			if v.Num > 0 {
				p.Widths = make([][2]float64, len(p.Points))
				for i := range p.Widths {
					p.Widths[i] = [2]float64{v.Num, v.Num}
				}
			}
			return nil
		}}
	closed := &def{id: "closed", kind: KindBool,
		get: func(_ *env, e doc.Entity) Value { return BoolValue(pline(e).Closed) },
		set: func(_ *env, e doc.Entity, v Value) error { pline(e).Closed = v.Bool; return nil }}
	curve := func(e doc.Entity) geom.Polyline { return e.Geometry()[0].(geom.Polyline) }
	length := num("length", KindLength, func(e doc.Entity) float64 { return curve(e).Length() }, nil)
	area := num("area", KindNumber, func(e doc.Entity) float64 { return math.Abs(geom.PolylineArea(curve(e))) }, nil)
	return []catDef{cat("geometry", one(selector), vertexRows, one(global, closed, length, area))}
}

// --- TEXT and MTEXT ------------------------------------------------------

func text(e doc.Entity) *doc.Text { return e.(*doc.Text) }

func mtext(e doc.Entity) *doc.MText { return e.(*doc.MText) }

// validContent reports whether s may be the content of a TEXT (single) or MTEXT: no control
// characters but tab, and line breaks only in MTEXT; the text editor's rule.
func validContent(s string, multi bool) bool {
	for _, r := range s {
		if !multi && (r == '\n' || r == '\r') || r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}

func textSize(v float64) bool { return v > 0 && v <= doc.TextMaxSize }

func textStyleRow() *def {
	return &def{id: "style", kind: KindTextStyle, options: textStyleOptions,
		get: func(_ *env, e doc.Entity) Value {
			s := ""
			switch t := e.(type) {
			case *doc.Text:
				s = t.Style
			case *doc.MText:
				s = t.Style
			}
			if s == "" {
				s = doc.DefaultTextStyle
			}
			return StrValue(s)
		},
		set: func(c *env, e doc.Entity, v Value) error {
			n, ok := doc.FindTextStyle(c.d, v.Str)
			if !ok {
				return errInvalid
			}
			switch t := e.(type) {
			case *doc.Text:
				t.Style = n
			case *doc.MText:
				t.Style = n
			}
			return nil
		}}
}

// justifyNames are the TEXT justifications in AutoCAD's order; the MTEXT ones are the last nine.
var justifyNames = []string{"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"}

func justifyOptions(names []string) []Option {
	out := make([]Option, len(names))
	for i, n := range names {
		out[i] = Option{Value: StrValue(n), Label: i18n.T("ui.props.justify." + strings.ToLower(n))}
	}
	return out
}

var textCats = []catDef{
	cat("text", one(
		&def{id: "contents", kind: KindText,
			get: func(_ *env, e doc.Entity) Value { return StrValue(text(e).Value) },
			set: func(_ *env, e doc.Entity, v Value) error {
				if !validContent(v.Str, false) {
					return errInvalid
				}
				text(e).Value = v.Str
				return nil
			}},
		textStyleRow(),
		&def{id: "justify", kind: KindEnum, options: func(*env) []Option { return justifyOptions(justifyNames) },
			get: func(_ *env, e doc.Entity) Value { return StrValue(text(e).Justify()) },
			set: func(_ *env, e doc.Entity, v Value) error {
				h, vv, ok := doc.ParseJustify(v.Str)
				if !ok {
					return errInvalid
				}
				if h != text(e).HAlign || vv != text(e).VAlign {
					text(e).SetJustify(h, vv)
				}
				return nil
			}},
		bounded(positive(num("height", KindLength, func(e doc.Entity) float64 { return text(e).Layout().Height },
			func(e doc.Entity, v float64) error {
				t := text(e)
				if !textSize(v) {
					return errInvalid
				}
				if t.HAlign == doc.TextAligned {
					lay := t.Layout()
					t.Align = t.Insert.Add(lay.Dir().Scale(doc.TextAdvance(t.Value) * v * lay.WidthFactor))
				}
				t.Height = v
				return nil
			})), 0, doc.TextMaxSize),
		num("rotation", KindAngle, func(e doc.Entity) float64 { return text(e).Layout().Rotation },
			func(e doc.Entity, v float64) error {
				t := text(e)
				v = geom.NormAngle(v)
				if t.HAlign == doc.TextAligned || t.HAlign == doc.TextFit {
					t.Align = t.Insert.Add(geom.V(math.Cos(v), math.Sin(v)).Scale(t.Insert.Dist(t.Align)))
				}
				t.Rotation = v
				return nil
			}),
		positive(num("width_factor", KindNumber, func(e doc.Entity) float64 { return text(e).Layout().WidthFactor },
			func(e doc.Entity, v float64) error { text(e).WidthFactor = v; return nil })),
		bounded(num("oblique", KindAngle, func(e doc.Entity) float64 { return text(e).Oblique },
			func(e doc.Entity, v float64) error { text(e).Oblique = v; return nil }), -85*math.Pi/180, 85*math.Pi/180),
	)),
	cat("geometry", point("position", func(e doc.Entity) geom.Vec2 { return text(e).Anchor() }, func(e doc.Entity, p geom.Vec2) { text(e).SetAnchor(p) })),
}

// mtextJustify returns an MTEXT attachment as its justification name (TL ... BR).
func mtextJustify(m *doc.MText) string {
	a := m.Attachment
	if a < 1 || a > 9 {
		a = 1
	}
	return string("TMB"[(a-1)/3]) + string("LCR"[(a-1)%3])
}

var mtextCats = []catDef{
	cat("text", one(
		&def{id: "contents", kind: KindMultiText,
			get: func(_ *env, e doc.Entity) Value { return StrValue(doc.MTextPlain(mtext(e).Value)) },
			set: func(_ *env, e doc.Entity, v Value) error {
				if !validContent(v.Str, true) {
					return errInvalid
				}
				mtext(e).Value = strings.NewReplacer("\r\n", `\P`, "\r", `\P`, "\n", `\P`).Replace(doc.MTextEscape(v.Str))
				return nil
			}},
		textStyleRow(),
		&def{id: "justify", kind: KindEnum, options: func(*env) []Option { return justifyOptions(justifyNames[6:]) },
			get: func(_ *env, e doc.Entity) Value { return StrValue(mtextJustify(mtext(e))) },
			set: func(_ *env, e doc.Entity, v Value) error {
				h, vv, ok := doc.ParseJustify(v.Str)
				if !ok || vv == doc.TextBaseline {
					return errInvalid
				}
				mtext(e).Attachment = int(doc.TextTop-vv)*3 + int(h) + 1
				return nil
			}},
		bounded(positive(num("height", KindLength, func(e doc.Entity) float64 { return mtext(e).Height },
			func(e doc.Entity, v float64) error {
				if !textSize(v) {
					return errInvalid
				}
				mtext(e).Height = v
				return nil
			})), 0, doc.TextMaxSize),
		num("rotation", KindAngle, func(e doc.Entity) float64 { return mtext(e).Rotation },
			func(e doc.Entity, v float64) error { mtext(e).Rotation = geom.NormAngle(v); return nil }),
		bounded(num("width", KindLength, func(e doc.Entity) float64 { return mtext(e).Width },
			func(e doc.Entity, v float64) error { mtext(e).Width = v; return nil }), 0, doc.TextMaxSize),
	)),
	cat("geometry", point("position", func(e doc.Entity) geom.Vec2 { return mtext(e).Insert }, func(e doc.Entity, p geom.Vec2) { mtext(e).Insert = p })),
}

// --- INSERT --------------------------------------------------------------

func insert(e doc.Entity) *doc.Insert { return e.(*doc.Insert) }

func nonZero(v float64) error {
	if v == 0 || math.IsInf(v, 0) {
		return errInvalid
	}
	return nil
}

var insertCats = []catDef{
	cat("geometry",
		point("position", func(e doc.Entity) geom.Vec2 { return insert(e).Position }, func(e doc.Entity, p geom.Vec2) { insert(e).Position = p }),
		one(
			num("scale_x", KindNumber, func(e doc.Entity) float64 { return insert(e).ScaleX },
				func(e doc.Entity, v float64) error { insert(e).ScaleX = v; return nonZero(v) }),
			num("scale_y", KindNumber, func(e doc.Entity) float64 { return insert(e).ScaleY },
				func(e doc.Entity, v float64) error { insert(e).ScaleY = v; return nonZero(v) }),
			num("rotation", KindAngle, func(e doc.Entity) float64 { return insert(e).Rotation },
				func(e doc.Entity, v float64) error { insert(e).Rotation = geom.NormAngle(v); return nil }),
		)),
	cat("misc", one(&def{id: "name", kind: KindBlock, options: blockOptions,
		get: func(_ *env, e doc.Entity) Value { return StrValue(insert(e).BlockName) },
		set: func(c *env, e doc.Entity, v Value) error {
			b := c.d.Block(v.Str)
			if b == nil {
				return errInvalid
			}
			in := insert(e)
			old := in.BlockName
			in.BlockName = b.Name
			if c.d.InsertError(in) != nil {
				in.BlockName = old
				return errInvalid
			}
			return nil
		}})),
}

// attributeCat returns the Attributes category of the selected block references: one row per
// tag, in the order they first appear; a constant attribute is read-only.
func attributeCat(ents []doc.Entity) []catDef {
	var tags []string
	constant := map[string]bool{}
	for _, e := range ents {
		for _, a := range insert(e).Attributes {
			u := strings.ToUpper(a.Tag)
			if _, seen := constant[u]; !seen {
				tags = append(tags, a.Tag)
				constant[u] = a.Flags&2 != 0
			}
			constant[u] = constant[u] && a.Flags&2 != 0
		}
	}
	if len(tags) == 0 {
		return nil
	}
	c := catDef{id: "attributes", title: "ui.props.cat.attributes"}
	for _, tag := range tags {
		find := func(e doc.Entity) *doc.Attribute {
			in := insert(e)
			for i := range in.Attributes {
				if strings.EqualFold(in.Attributes[i].Tag, tag) {
					return &in.Attributes[i]
				}
			}
			return nil
		}
		c.defs = append(c.defs, &def{id: "attr:" + tag, label: "ui.props.p.attribute", name: tag, kind: KindText, readOnly: constant[strings.ToUpper(tag)],
			only: func(_ *env, e doc.Entity) bool { return find(e) != nil },
			get:  func(_ *env, e doc.Entity) Value { return StrValue(find(e).Value) },
			set: func(_ *env, e doc.Entity, v Value) error {
				if !validContent(v.Str, false) || !insert(e).SetAttribute(tag, v.Str) {
					return errInvalid
				}
				return nil
			}})
	}
	return []catDef{c}
}
