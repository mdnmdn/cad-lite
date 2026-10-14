package doc

import (
	"math"
	"strings"

	"cad-lite/geom"
)

var (
	_ Annotation = (*Text)(nil)
	_ Annotation = (*MText)(nil)
)

// MText attachment points, DXF group 71: the point of the text box Insert is.
const (
	MTextTopLeft      = 1
	MTextTopCenter    = 2
	MTextTopRight     = 3
	MTextMiddleLeft   = 4
	MTextMiddleCenter = 5
	MTextMiddleRight  = 6
	MTextBottomLeft   = 7
	MTextBottomCenter = 8
	MTextBottomRight  = 9
)

// MText is a multiline MTEXT entity: paragraphs in a box of Width, wrapped at word boundaries,
// placed by the Attachment point of the box at Insert. Value keeps AutoCAD's MTEXT format codes
// (\P ends a paragraph); the document draws the text they carry with the stroke font and ignores
// the formatting (font, height, colour and the like changes inside the text), as MTextPlain
// describes. MTEXT cannot be mirrored: a reflection keeps it readable.
type MText struct {
	EntityCommon
	// Insert is the attachment point (group 10).
	Insert geom.Vec2
	// Height is the capital height (group 40).
	Height float64
	// Width is the box width the paragraphs wrap at (group 41); 0 does not wrap.
	Width float64
	// Attachment is MTextTopLeft to MTextBottomRight (group 71); 0 means top left.
	Attachment int
	// Rotation is the angle of the text direction in radians (group 50, or the direction vector
	// of group 11).
	Rotation float64
	// LineSpacing is the line spacing factor (group 44): 1, or 0, puts baselines 5/3 of the height
	// apart.
	LineSpacing float64
	// Style is the text style name (group 7); "" means DefaultTextStyle.
	Style string
	// Value is the text with its format codes (groups 3 and 1).
	Value string

	cache *mtextCache
}

type mtextKey struct {
	insert                      geom.Vec2
	height, width, rot, spacing float64
	attach                      int
	value                       string
}

type mtextCache struct {
	key    mtextKey
	lay    MTextLayout
	curves []geom.Curve
	bounds geom.Box2
}

// MTextLayout is where an MText's lines sit.
type MTextLayout struct {
	// Lines are the rows of plain text after wrapping, top to bottom.
	Lines []string
	// Starts are the baseline starts of the lines, in model space.
	Starts []geom.Vec2
	// Widths are the baseline lengths of the lines.
	Widths []float64
	// Corners are the text box, counter-clockwise from its bottom-left corner: Width wide (the
	// longest line when Width is 0), from the top of the first line's capitals to the bottom of
	// the last line's descenders.
	Corners [4]geom.Vec2
	// Spacing is the distance between baselines.
	Spacing float64
}

// NewMText returns top-left attached multiline text in the default style.
func NewMText(p geom.Vec2, height, width float64, value string) *MText {
	return &MText{Insert: p, Height: height, Width: width, Attachment: MTextTopLeft, LineSpacing: 1, Style: DefaultTextStyle, Value: value}
}

// Class implements Object.
func (*MText) Class() string { return "MTEXT" }

// Clone implements Object.
func (e *MText) Clone() Object { c := *e; return &c }

// Bounds implements Entity: the box of the strokes, the text box and Insert.
func (e *MText) Bounds() geom.Box2 { return e.layout().bounds }

// Geometry implements Entity: the glyph strokes.
func (e *MText) Geometry() []geom.Curve { return e.layout().curves }

// Anchor implements Annotation: Insert.
func (e *MText) Anchor() geom.Vec2 { return e.Insert }

// HitBox implements Annotation.
func (e *MText) HitBox(p geom.Vec2) bool { return inQuad(e.Layout().Corners, p) }

// Box implements Annotation.
func (e *MText) Box() [4]geom.Vec2 { return e.Layout().Corners }

// CharHeight implements Annotation.
func (e *MText) CharHeight() float64 { return max(e.Height, 0) }

// Greek implements Annotation: a line through the middle of each row.
func (e *MText) Greek() [][]geom.Vec2 {
	lay := e.Layout()
	d := geom.V(math.Cos(e.Rotation), math.Sin(e.Rotation))
	up := d.Perp().Scale(e.CharHeight() * (1 - textDescent) / 2)
	var out [][]geom.Vec2
	for i, s := range lay.Starts {
		if lay.Widths[i] > 0 {
			a := s.Add(up)
			out = append(out, []geom.Vec2{a, a.Add(d.Scale(lay.Widths[i]))})
		}
	}
	return out
}

// Transform implements Entity. A reflection moves the text to its mirrored place and keeps it
// readable, swapping its attachment so its box lands on the mirrored box (MTEXT has no mirrored
// form, in AutoCAD either).
func (e *MText) Transform(m geom.Mat3) {
	d0 := geom.V(math.Cos(e.Rotation), math.Sin(e.Rotation))
	d := m.ApplyVec(d0)
	e.Insert = m.Apply(e.Insert)
	e.Rotation = geom.NormAngle(d.Angle())
	s := m.ScaleFactor()
	e.Height *= s
	e.Width *= s
	if m.Det() >= 0 {
		return
	}
	a := e.Attachment
	if a < 1 || a > 9 {
		a = MTextTopLeft
	}
	row, col := (a-1)/3, (a-1)%3
	if d.Dot(d0) >= 0 {
		row = 2 - row
	} else {
		col = 2 - col
		e.Rotation = geom.NormAngle(e.Rotation + math.Pi)
	}
	e.Attachment = row*3 + col + 1
}

// MirrorReadable implements Annotation: MTEXT is always mirrored readable, as Transform does.
func (e *MText) MirrorReadable(m geom.Mat3) { e.Transform(m) }

// Layout lays the text out.
func (e *MText) Layout() MTextLayout { return e.layout().lay }

// MTextPlain returns the text an MTEXT value shows, one paragraph per line ("\n" between them):
// \P ends a paragraph; \~ is a space; \\, \{ and \} are the characters; \S stacks a fraction,
// shown as num/den; the other format codes (\f, \H, \C, \W, \Q, \T, \A, \p, \L, \O, \K and their
// lower-case forms) and the { } groups are dropped. %% codes and \U+ escapes are kept, as TEXT
// draws them.
func MTextPlain(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '{' || c == '}':
		case c == '\\' && i+1 < len(v):
			n := v[i+1]
			switch n {
			case 'P', 'X':
				b.WriteByte('\n')
				i++
			case '~':
				b.WriteByte(' ')
				i++
			case '\\', '{', '}':
				b.WriteByte(n)
				i++
			case 'U', 'u':
				b.WriteByte(c) // \U+XXXX: kept for the text decoder
			case 'L', 'l', 'O', 'o', 'K', 'k', 'N':
				i++
			case 'S':
				j := strings.IndexByte(v[i+2:], ';')
				if j < 0 {
					i = len(v)
					break
				}
				frac := strings.NewReplacer("^", "/", "#", "/", "~", "/").Replace(v[i+2 : i+2+j])
				b.WriteString(frac)
				i += 2 + j
			default:
				// \f...; \H...; \C...; and the like run to the next ';'.
				j := strings.IndexByte(v[i+1:], ';')
				if j < 0 {
					i = len(v)
				} else {
					i += 1 + j
				}
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// MTextEscape returns plain text as an MTEXT value shows it: a backslash and the braces, which
// MTEXT reads as format codes and grouping, are escaped (\\, \{, \}). A caller joins escaped
// paragraphs with \P; MTextPlain of the result gives the text back.
func MTextEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "{", `\{`, "}", `\}`).Replace(s)
}

// wrapLine splits a paragraph into rows no wider than width (at height h), breaking at spaces; a
// word longer than the width gets a row of its own.
func wrapLine(p string, width, h float64) []string {
	if width <= 0 || h <= 0 || TextAdvance(p)*h <= width {
		return []string{p}
	}
	// Advances add up along a line, so each word is measured once and a row's width is the sum
	// of its words and the spaces between them: linear in the paragraph's length.
	space := TextAdvance(" ")
	var rows []string
	var cur []string
	curW := 0.0
	for _, w := range strings.Split(p, " ") {
		aw := TextAdvance(w)
		if len(cur) > 0 && (curW+space+aw)*h > width {
			rows = append(rows, strings.Join(cur, " "))
			cur, curW = []string{w}, aw
			continue
		}
		if len(cur) > 0 {
			curW += space
		}
		cur = append(cur, w)
		curW += aw
	}
	return append(rows, strings.Join(cur, " "))
}

func (e *MText) key() mtextKey {
	return mtextKey{e.Insert, e.Height, e.Width, e.Rotation, e.LineSpacing, e.Attachment, e.Value}
}

func (e *MText) layout() *mtextCache {
	k := e.key()
	if e.cache != nil && e.cache.key == k {
		return e.cache
	}
	c := &mtextCache{key: k}
	h := clampHeight(e.Height)
	ls := clampSpacing(e.LineSpacing)
	var lines []string
	for _, p := range strings.Split(MTextPlain(e.Value), "\n") {
		lines = append(lines, wrapLine(p, e.Width, h)...)
	}
	lay := MTextLayout{Lines: lines, Spacing: 5.0 / 3 * h * ls}
	strokes := make([]strokeLine, len(lines))
	w := math.Min(max(e.Width, 0), TextMaxSize)
	for i, l := range lines {
		strokes[i] = layoutStrokes(l)
		lay.Widths = append(lay.Widths, strokes[i].advance*h)
		if e.Width <= 0 {
			w = max(w, strokes[i].advance*h)
		}
	}
	top := h
	bottom := -float64(len(lines)-1)*lay.Spacing - textDescent*h
	a := e.Attachment
	if a < 1 || a > 9 {
		a = MTextTopLeft
	}
	row, col := (a-1)/3, (a-1)%3
	ax := []float64{0, w / 2, w}[col]
	ay := []float64{top, (top + bottom) / 2, bottom}[row]
	sin, cos := math.Sincos(finiteAngle(e.Rotation))
	place := func(x, y float64) geom.Vec2 {
		x, y = x-ax, y-ay
		return e.Insert.Add(geom.V(x*cos-y*sin, x*sin+y*cos))
	}
	c.bounds = geom.EmptyBox()
	for i, sl := range strokes {
		x0 := []float64{0, (w - lay.Widths[i]) / 2, w - lay.Widths[i]}[col]
		y0 := -float64(i) * lay.Spacing
		lay.Starts = append(lay.Starts, place(x0, y0))
		if h <= 0 {
			continue
		}
		for _, st := range sl.strokes {
			pts := make([]geom.Vec2, len(st))
			for j, p := range st {
				pts[j] = place(x0+p.X*h, y0+p.Y*h)
				c.bounds = c.bounds.Extend(pts[j])
			}
			c.curves = append(c.curves, geom.Polyline{Pts: pts})
		}
	}
	lay.Corners = [4]geom.Vec2{place(0, bottom), place(w, bottom), place(w, top), place(0, top)}
	c.bounds = c.bounds.Union(geom.BoxOf(lay.Corners[:]...)).Extend(e.Insert)
	c.lay = lay
	e.cache = c
	return c
}
