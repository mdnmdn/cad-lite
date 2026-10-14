package doc

import (
	"math"
	"strings"

	"cad-lite/geom"
)

// TextHAlign is the horizontal justification of a TEXT entity, DXF group 72.
type TextHAlign int

// The horizontal justifications, with their DXF values.
const (
	// TextLeft starts the text at its point.
	TextLeft TextHAlign = 0
	// TextCenter centres the baseline on its point.
	TextCenter TextHAlign = 1
	// TextRight ends the text at its point.
	TextRight TextHAlign = 2
	// TextAligned fits the text between Insert and Align, scaling its height with its length.
	TextAligned TextHAlign = 3
	// TextMiddle centres the text horizontally and vertically on the middle of its ink.
	TextMiddle TextHAlign = 4
	// TextFit fits the text between Insert and Align at its height, changing its width factor.
	TextFit TextHAlign = 5
)

// TextVAlign is the vertical justification of a TEXT entity, DXF group 73.
type TextVAlign int

// The vertical justifications, with their DXF values.
const (
	// TextBaseline puts the baseline on the point.
	TextBaseline TextVAlign = 0
	// TextBottom puts the bottom of the descenders on the point.
	TextBottom TextVAlign = 1
	// TextVMiddle puts the middle of the capital height on the point.
	TextVMiddle TextVAlign = 2
	// TextTop puts the top of the capitals on the point.
	TextTop TextVAlign = 3
)

// Text generation flags, DXF group 71.
const (
	// TextBackward mirrors the text in its own X direction.
	TextBackward = 2
	// TextUpsideDown mirrors the text in its own Y direction.
	TextUpsideDown = 4
)

// DefaultTextStyle is the name of the text style every drawing has.
const DefaultTextStyle = "Standard"

// Text is a single-line TEXT entity. The fields follow DXF: Insert is the first alignment point
// (group 10) and Align the second (group 11). With TextLeft and TextBaseline the text sits at
// Insert and Align is unused; TextAligned and TextFit run the baseline from Insert to Align; every
// other justification places the text by Align alone, and the codec writes Insert as the computed
// baseline start, as AutoCAD does. Every style draws with the built-in stroke font (see
// textfont.go); Style is the name kept for the file.
type Text struct {
	EntityCommon
	Insert, Align geom.Vec2
	// Height is the capital height (group 40); TextAligned derives its own from the baseline.
	Height float64
	// Rotation is the baseline angle in radians (group 50 in degrees); TextAligned and TextFit
	// take it from Insert to Align instead.
	Rotation float64
	// WidthFactor stretches the glyphs horizontally (group 41); 0 means 1. TextFit derives it.
	WidthFactor float64
	// Oblique slants the glyphs by this angle from the vertical, in radians (group 51).
	Oblique float64
	// Style is the text style name (group 7); "" means DefaultTextStyle.
	Style  string
	HAlign TextHAlign
	VAlign TextVAlign
	// Generation holds TextBackward and TextUpsideDown (group 71).
	Generation int
	// Value is the text, with AutoCAD's %% control codes (group 1).
	Value string

	cache *textCache
}

// textCache is the laid-out geometry of a Text, valid while its key matches the fields.
type textCache struct {
	key    textKey
	lay    TextLayout
	curves []geom.Curve
	bounds geom.Box2
}

// textKey is every field the geometry depends on.
type textKey struct {
	insert, align            geom.Vec2
	height, rot, wf, oblique float64
	h                        TextHAlign
	v                        TextVAlign
	gen                      int
	value                    string
}

// TextStyles returns the text style names a drawing knows, without duplicates (compared without
// regard to case): DefaultTextStyle, the STYLE records kept from its file, then the styles its
// TEXT and MTEXT entities name, in drawing order. The document has no style table of its own.
func TextStyles(d *Drawing) []string {
	names := []string{DefaultTextStyle}
	add := func(n string) {
		if n == "" {
			return
		}
		for _, o := range names {
			if strings.EqualFold(o, n) {
				return
			}
		}
		names = append(names, n)
	}
	for _, r := range d.Preserved.Tables["STYLE"] {
		add(r.Name())
	}
	for _, h := range d.Handles() {
		switch t := d.Entity(h).(type) {
		case *Text:
			add(t.Style)
		case *MText:
			add(t.Style)
		}
	}
	return names
}

// FindTextStyle returns the drawing's spelling of the text style name (TextStyles, matched
// without regard to case or surrounding spaces) and false when the drawing has no such style.
func FindTextStyle(d *Drawing, name string) (string, bool) {
	name = strings.TrimSpace(name)
	for _, n := range TextStyles(d) {
		if strings.EqualFold(n, name) {
			return n, true
		}
	}
	return "", false
}

// NewText returns left-justified text in the default style with its baseline starting at p.
func NewText(p geom.Vec2, height, rotation float64, value string) *Text {
	return &Text{Insert: p, Height: height, Rotation: rotation, WidthFactor: 1, Style: DefaultTextStyle, Value: value}
}

// Class implements Object.
func (*Text) Class() string { return "TEXT" }

// Clone implements Object.
func (e *Text) Clone() Object { c := *e; return &c }

// Bounds implements Entity: the box of the drawn strokes, the text box (TextLayout.Corners) and
// the anchor point.
func (e *Text) Bounds() geom.Box2 { return e.layout().bounds }

// Geometry implements Entity: the glyph strokes, one open polyline each.
func (e *Text) Geometry() []geom.Curve { return e.layout().curves }

// Transform implements Entity. A reflection mirrors the text (it toggles TextUpsideDown); MIRROR
// with MIRRTEXT 0 uses MirrorReadable instead.
func (e *Text) Transform(m geom.Mat3) {
	d := m.ApplyVec(geom.V(math.Cos(e.Rotation), math.Sin(e.Rotation)))
	e.Insert, e.Align = m.Apply(e.Insert), m.Apply(e.Align)
	e.Rotation = geom.NormAngle(d.Angle())
	e.Height *= m.ScaleFactor()
	if m.Det() < 0 {
		e.Generation ^= TextUpsideDown
	}
}

// MirrorReadable applies the reflection m as AutoCAD's MIRROR does with MIRRTEXT 0: the text
// moves to its mirrored place but still reads normally, and its box lands on the mirrored box. A
// mirror line along the text swaps top and bottom justification and moves any other text across
// its baseline, keeping its justification; one across it
// reverses the reading direction and swaps left and right justification (and the two points of
// Aligned and Fit text). Any other transform is applied as Transform does.
func (e *Text) MirrorReadable(m geom.Mat3) {
	before := e.Layout()
	e.Transform(m)
	if m.Det() >= 0 {
		return
	}
	mirrored := e.Layout().Corners
	e.Generation ^= TextUpsideDown
	d0 := before.Dir()
	if m.ApplyVec(d0).Dot(d0) >= 0 {
		switch e.VAlign {
		case TextTop:
			e.SetJustify(e.HAlign, TextBottom)
		case TextBottom:
			e.SetJustify(e.HAlign, TextTop)
		default:
			// Keep the justification and move the text so its box lands on the mirrored one.
			now := e.Layout().Corners
			e.SetAnchor(e.Anchor().Add(quadCenter(mirrored).Sub(quadCenter(now))))
		}
		return
	}
	switch e.HAlign {
	case TextAligned, TextFit:
		e.Insert, e.Align = e.Align, e.Insert
	case TextLeft:
		e.SetJustify(TextRight, e.VAlign)
	case TextRight:
		e.SetJustify(TextLeft, e.VAlign)
	}
	e.Rotation = geom.NormAngle(e.Rotation + math.Pi)
}

// Anchor returns the point the justification places the text by: Insert for left-baseline,
// Aligned and Fit text, Align otherwise. It is the text's insertion snap point.
func (e *Text) Anchor() geom.Vec2 {
	if e.byInsert() {
		return e.Insert
	}
	return e.Align
}

// SetAnchor moves the text so that its anchor is p.
func (e *Text) SetAnchor(p geom.Vec2) {
	if e.byInsert() {
		e.Align = e.Align.Add(p.Sub(e.Insert))
		e.Insert = p
		return
	}
	e.Align = p
}

func (e *Text) byInsert() bool {
	return e.HAlign == TextAligned || e.HAlign == TextFit || e.HAlign == TextLeft && e.VAlign == TextBaseline
}

// SetJustify changes the justification keeping the anchor point, so the text moves around it
// (AutoCAD's TEXT does this; JUSTIFYTEXT, which keeps the text still, is not modelled). Aligned,
// Fit and Middle force TextBaseline, as DXF requires; switching to Aligned or Fit from another
// justification puts the second point one text length along the rotation.
func (e *Text) SetJustify(h TextHAlign, v TextVAlign) {
	if h == TextAligned || h == TextFit || h == TextMiddle {
		v = TextBaseline
	}
	lay := e.Layout()
	p := e.Anchor()
	wasPair := e.HAlign == TextAligned || e.HAlign == TextFit
	e.HAlign, e.VAlign = h, v
	if h == TextAligned || h == TextFit {
		if !wasPair {
			e.Insert = lay.Origin
			e.Align = lay.Origin.Add(lay.Dir().Scale(lay.Width))
			e.Rotation = lay.Rotation
			e.Height = lay.Height
		}
		return
	}
	if wasPair {
		e.Rotation, e.Height, e.WidthFactor = lay.Rotation, lay.Height, lay.WidthFactor
	}
	e.SetAnchor(p)
}

// Justify returns the AutoCAD justification option name: Left, Center, Right, Align, Middle,
// Fit, or TL TC TR ML MC MR BL BC BR (the baseline ones are Left, Center and Right).
func (e *Text) Justify() string { return JustifyName(e.HAlign, e.VAlign) }

// JustifyName is the AutoCAD option name of a justification pair; see Text.Justify.
func JustifyName(h TextHAlign, v TextVAlign) string {
	switch h {
	case TextAligned:
		return "Align"
	case TextMiddle:
		return "Middle"
	case TextFit:
		return "Fit"
	}
	if v == TextBaseline {
		return [...]string{"Left", "Center", "Right"}[min(max(int(h), 0), 2)]
	}
	return string("BMT"[min(max(int(v), 1), 3)-1]) + string("LCR"[min(max(int(h), 0), 2)])
}

// ParseJustify reads an AutoCAD justification option name (case-insensitive; "A" and "Aligned"
// mean Align, and the first letters of Left, Center, Right, Middle and Fit their option). It
// reports false for anything else.
func ParseJustify(s string) (TextHAlign, TextVAlign, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "L", "LEFT":
		return TextLeft, TextBaseline, true
	case "C", "CENTER", "CENTRE":
		return TextCenter, TextBaseline, true
	case "R", "RIGHT":
		return TextRight, TextBaseline, true
	case "A", "ALIGN", "ALIGNED":
		return TextAligned, TextBaseline, true
	case "M", "MIDDLE":
		return TextMiddle, TextBaseline, true
	case "F", "FIT":
		return TextFit, TextBaseline, true
	}
	u := strings.ToUpper(strings.TrimSpace(s))
	if len(u) != 2 {
		return 0, 0, false
	}
	v := strings.IndexByte("BMT", u[0])
	h := strings.IndexByte("LCR", u[1])
	if v < 0 || h < 0 {
		return 0, 0, false
	}
	return TextHAlign(h), TextVAlign(v + 1), true
}

// TextLayout is where a Text's line sits, after the justification is resolved.
type TextLayout struct {
	// Origin is the start of the baseline: DXF group 10 as AutoCAD writes it.
	Origin geom.Vec2
	// Rotation is the baseline angle in radians.
	Rotation float64
	// Height and WidthFactor are the values in effect (Aligned and Fit derive one of them).
	Height, WidthFactor float64
	// Width is the length of the baseline.
	Width float64
	// Corners are the text box from the bottom of the descenders to the top of the capitals,
	// counter-clockwise from the baseline start's bottom corner (before the generation flags
	// mirror it).
	Corners [4]geom.Vec2
}

// Dir returns the unit vector along the baseline.
func (l TextLayout) Dir() geom.Vec2 { return geom.V(math.Cos(l.Rotation), math.Sin(l.Rotation)) }

// Layout resolves the justification.
func (e *Text) Layout() TextLayout { return e.layout().lay }

// HitBox reports whether p lies inside the text box (see TextLayout.Corners), which AutoCAD
// counts as picking the text.
func (e *Text) HitBox(p geom.Vec2) bool { return inQuad(e.Layout().Corners, p) }

// Box implements Annotation: the corners of the text box.
func (e *Text) Box() [4]geom.Vec2 { return e.Layout().Corners }

// CharHeight implements Annotation: the capital height in effect.
func (e *Text) CharHeight() float64 { return e.Layout().Height }

// Greek implements Annotation: one line through the middle of the text box.
func (e *Text) Greek() [][]geom.Vec2 {
	c := e.Layout().Corners
	return [][]geom.Vec2{{c[0].Lerp(c[3], 0.5), c[1].Lerp(c[2], 0.5)}}
}

// Annotation is a text entity, TEXT or MTEXT. Its strokes are glyphs rather than edges: it snaps
// only at its insertion point, a point anywhere in its box picks it, a mirror keeps it readable,
// and the renderer may draw it as a line per row of text when it is too small to read.
type Annotation interface {
	Entity
	// Anchor is the insertion point: where the justification or attachment places the text.
	Anchor() geom.Vec2
	// HitBox reports whether p lies in the text's box.
	HitBox(p geom.Vec2) bool
	// Box returns the corners of the text's box, counter-clockwise.
	Box() [4]geom.Vec2
	// CharHeight is the capital height of the text.
	CharHeight() float64
	// Greek returns one line per row of text, through the middle of the row.
	Greek() [][]geom.Vec2
	// MirrorReadable reflects the text by m keeping it readable (AutoCAD's MIRRTEXT 0).
	MirrorReadable(m geom.Mat3)
}

// quadCenter returns the centre of the parallelogram c.
func quadCenter(c [4]geom.Vec2) geom.Vec2 { return c[0].Lerp(c[2], 0.5) }

// inQuad reports whether p lies inside the convex quadrilateral c.
func inQuad(c [4]geom.Vec2, p geom.Vec2) bool {
	sign := 0.0
	for i := range 4 {
		s := c[(i+1)%4].Sub(c[i]).Cross(p.Sub(c[i]))
		if s*sign < 0 {
			return false
		}
		if s != 0 {
			sign = s
		}
	}
	return sign != 0
}

func (e *Text) key() textKey {
	return textKey{e.Insert, e.Align, e.Height, e.Rotation, e.WidthFactor, e.Oblique, e.HAlign, e.VAlign, e.Generation, e.Value}
}

// layout returns the cached geometry, rebuilding it when a field changed since. Entities are
// read and written on one goroutine (the document is not safe for concurrent use), so the cache
// needs no lock.
func (e *Text) layout() *textCache {
	k := e.key()
	if e.cache != nil && e.cache.key == k {
		return e.cache
	}
	c := &textCache{key: k}
	ln := layoutStrokes(e.Value)
	h, wf, rot := clampHeight(e.Height), clampWidthFactor(e.WidthFactor), finiteAngle(e.Rotation)
	pair := e.HAlign == TextAligned || e.HAlign == TextFit
	if pair {
		d := e.Align.Sub(e.Insert)
		if l := d.Len(); l > 0 {
			rot = d.Angle()
			if ln.advance > 0 {
				if e.HAlign == TextAligned {
					h = clampHeight(l / (ln.advance * wf))
				} else if h > 0 {
					// Fit may squeeze or stretch past STYLE's range, but not to infinity.
					switch wf = l / (ln.advance * h); {
					case !(wf > 0):
						wf = 1
					case wf > TextMaxSize:
						wf = TextMaxSize
					}
				}
			}
		}
	}
	w := ln.advance * h * wf
	var x0, y0 float64
	switch e.HAlign {
	case TextCenter:
		x0 = -w / 2
	case TextRight:
		x0 = -w
	case TextMiddle:
		x0 = -w / 2
		if !ln.ink.IsEmpty() {
			y0 = -(ln.ink.Min.Y + ln.ink.Max.Y) / 2 * h
		} else {
			y0 = -h / 2
		}
	}
	if e.HAlign != TextMiddle && !pair {
		switch e.VAlign {
		case TextBottom:
			y0 = textDescent * h
		case TextVMiddle:
			y0 = -h / 2
		case TextTop:
			y0 = -h
		}
	}
	anchor := e.Anchor()
	shear := math.Tan(clampOblique(e.Oblique))
	sin, cos := math.Sincos(rot)
	// place maps a point of the line (cap-height units, baseline y = 0) to model space.
	place := func(p geom.Vec2) geom.Vec2 {
		x := x0 + p.X*h*wf + p.Y*h*shear
		y := y0 + p.Y*h
		if e.Generation&TextBackward != 0 {
			if pair {
				x = w - x
			} else {
				x = -x
			}
		}
		if e.Generation&TextUpsideDown != 0 {
			y = -y
		}
		return anchor.Add(geom.V(x*cos-y*sin, x*sin+y*cos))
	}
	c.bounds = geom.EmptyBox()
	if h > 0 {
		for _, st := range ln.strokes {
			pts := make([]geom.Vec2, len(st))
			for i, p := range st {
				pts[i] = place(p)
				c.bounds = c.bounds.Extend(pts[i])
			}
			c.curves = append(c.curves, geom.Polyline{Pts: pts})
		}
	}
	adv := 0.0
	if h > 0 && wf > 0 {
		adv = w / (h * wf)
	}
	c.lay = TextLayout{
		Origin:   place(geom.V(0, 0)),
		Rotation: geom.NormAngle(rot), Height: h, WidthFactor: wf, Width: w,
		Corners: [4]geom.Vec2{place(geom.V(0, -textDescent)), place(geom.V(adv, -textDescent)), place(geom.V(adv, 1)), place(geom.V(0, 1))},
	}
	// The box and the anchor count too: the spatial index finds a text by its bounds, for the
	// insertion snap (outside the ink for bottom or top text) and a pick inside the box.
	c.bounds = c.bounds.Union(geom.BoxOf(c.lay.Corners[:]...)).Extend(anchor)
	e.cache = c
	return c
}
