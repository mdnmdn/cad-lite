package doc

import (
	"bufio"
	"bytes"
	_ "embed"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"cad-lite/geom"
)

// The stroke font every text style draws with: Hershey Roman Simplex, the design AutoCAD's
// romans.shx derives from. Glyphs are single-line strokes, so text is polylines like every other
// entity: it draws, picks, selects and exports without a font rasterizer, and its geometry is
// the same in every build. The licence acknowledgements are in fonts/NOTICE.
//
//go:embed fonts/romans.jhf
var romansJHF []byte

// Hershey units of the Roman Simplex font: capitals run from y = -12 to the baseline at y = 9,
// y grows downwards, and a glyph's x is measured from its centre.
const (
	hersheyBaseline = 9.0
	hersheyCap      = 21.0
	// textDescent is the depth of the descenders (g, p, y) below the baseline, in cap heights.
	textDescent = 7.0 / hersheyCap
	// underlineY and overlineY are where %%u and %%o draw, in cap heights above the baseline.
	underlineY = -0.2
	overlineY  = 1.2
)

// glyph is one character of the stroke font, in Hershey units.
type glyph struct {
	left, right float64
	strokes     [][]geom.Vec2
}

var (
	fontOnce sync.Once
	glyphs   map[rune]*glyph
)

// font returns the glyph table, parsing the embedded font on first use.
func font() map[rune]*glyph {
	fontOnce.Do(func() {
		glyphs = parseJHF(romansJHF, ' ')
		// The file's 96th glyph, on U+007F (DEL, not a printing character), is a degree ring.
		delete(glyphs, 0x7f)
		for r, g := range extraGlyphs() {
			glyphs[r] = g
		}
	})
	return glyphs
}

// parseJHF reads Hershey glyphs in James Hurt's format: per glyph a five-column number, a
// three-column vertex count, then that many coordinate pairs (the first is the left and right
// bearing, " R" lifts the pen), each coordinate a letter offset from 'R'. A glyph may continue
// on the next line. Glyphs are assigned to consecutive runes from first.
func parseJHF(data []byte, first rune) map[rune]*glyph {
	var buf []byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		buf = append(buf, strings.TrimRight(sc.Text(), "\r")...)
	}
	out := map[rune]*glyph{}
	r := first
	for len(buf) >= 10 {
		n, err := strconv.Atoi(strings.TrimSpace(string(buf[5:8])))
		if err != nil || n < 1 || len(buf) < 8+2*n {
			break
		}
		body := buf[8 : 8+2*n]
		buf = buf[8+2*n:]
		g := &glyph{left: float64(int(body[0]) - 'R'), right: float64(int(body[1]) - 'R')}
		var cur []geom.Vec2
		for i := 2; i+1 < len(body); i += 2 {
			if body[i] == ' ' && body[i+1] == 'R' {
				if len(cur) > 1 {
					g.strokes = append(g.strokes, cur)
				}
				cur = nil
				continue
			}
			cur = append(cur, geom.V(float64(int(body[i])-'R'), float64(int(body[i+1])-'R')))
		}
		if len(cur) > 1 {
			g.strokes = append(g.strokes, cur)
		}
		out[r] = g
		r++
	}
	return out
}

// ring returns a closed polygon of n sides around c, in Hershey units.
func ring(c geom.Vec2, r float64, n int) []geom.Vec2 {
	pts := make([]geom.Vec2, 0, n+1)
	for k := 0; k <= n; k++ {
		pts = append(pts, c.Polar(r, 2*math.Pi*float64(k)/float64(n)))
	}
	return pts
}

// extraGlyphs are the characters of AutoCAD's %% codes that the ASCII font lacks: degree (%%d),
// plus-minus (%%p) and diameter (%%c, U+2205; U+2300 draws the same).
func extraGlyphs() map[rune]*glyph {
	v := geom.V
	diameter := &glyph{left: -10, right: 10, strokes: [][]geom.Vec2{ring(v(0, -2), 7, 24), {v(-8, 7), v(8, -11)}}}
	return map[rune]*glyph{
		'°': {left: -5, right: 5, strokes: [][]geom.Vec2{ring(v(0, -9), 3, 12)}},
		'±': {left: -12, right: 12, strokes: [][]geom.Vec2{{v(0, -9), v(0, 3)}, {v(-8, -3), v(8, -3)}, {v(-8, 9), v(8, 9)}}},
		'∅': diameter,
		'⌀': diameter,
	}
}

// markStrokes returns the strokes of combining mark m over a base glyph whose ink top is top
// (Hershey units), or nil for a mark the font does not draw.
func markStrokes(m rune, top float64) [][]geom.Vec2 {
	v := geom.V
	t := top
	switch m {
	case '̀': // grave
		return [][]geom.Vec2{{v(-2, t-6), v(1, t-3)}}
	case '́': // acute
		return [][]geom.Vec2{{v(-1, t-3), v(2, t-6)}}
	case '̂': // circumflex
		return [][]geom.Vec2{{v(-3, t-3), v(0, t-6), v(3, t-3)}}
	case '̃': // tilde
		return [][]geom.Vec2{{v(-4, t-3), v(-2, t-5), v(2, t-3), v(4, t-5)}}
	case '̈': // diaeresis
		return [][]geom.Vec2{ring(v(-3, t-4), 1, 4), ring(v(3, t-4), 1, 4)}
	case '̊': // ring above
		return [][]geom.Vec2{ring(v(0, t-5), 2, 8)}
	case '̧': // cedilla
		return [][]geom.Vec2{{v(0, 9), v(0, 11), v(2, 12), v(0, 14)}}
	}
	return nil
}

// textToken is one decoded unit of a TEXT string: a character, or an underline or overline
// switch (%%u, %%o).
type textToken struct {
	r          rune
	under, ovr bool
}

// decodeText expands AutoCAD's control codes: %%d, %%p and %%c (degree, plus-minus, diameter),
// %%% (a percent sign), %%nnn (the character with decimal code nnn), %%u and %%o (underline and
// overline on or off), and \U+XXXX (a Unicode character, as DXF stores characters outside the
// code page). Anything else is literal.
func decodeText(s string) []textToken {
	var out []textToken
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '%' && i+2 < len(rs) && rs[i+1] == '%' {
			c := rs[i+2]
			switch {
			case c == 'd' || c == 'D':
				out = append(out, textToken{r: '°'})
			case c == 'p' || c == 'P':
				out = append(out, textToken{r: '±'})
			case c == 'c' || c == 'C':
				out = append(out, textToken{r: '∅'})
			case c == '%':
				out = append(out, textToken{r: '%'})
			case c == 'u' || c == 'U':
				out = append(out, textToken{under: true})
			case c == 'o' || c == 'O':
				out = append(out, textToken{ovr: true})
			case c >= '0' && c <= '9':
				j := i + 2
				for j < len(rs) && j < i+5 && rs[j] >= '0' && rs[j] <= '9' {
					j++
				}
				n, _ := strconv.Atoi(string(rs[i+2 : j]))
				out = append(out, textToken{r: rune(n)})
				i = j - 1
				continue
			default:
				out = append(out, textToken{r: '%'}, textToken{r: '%'}, textToken{r: c})
			}
			i += 2
			continue
		}
		if rs[i] == '\\' && i+6 < len(rs) && (rs[i+1] == 'U' || rs[i+1] == 'u') && rs[i+2] == '+' {
			if n, err := strconv.ParseUint(string(rs[i+3:i+7]), 16, 32); err == nil {
				out = append(out, textToken{r: rune(n)})
				i += 6
				continue
			}
		}
		out = append(out, textToken{r: rs[i]})
	}
	return out
}

// strokeLine is a laid-out line of text in cap-height units: the baseline is y = 0, capitals
// reach y = 1 and the pen starts at x = 0.
type strokeLine struct {
	strokes [][]geom.Vec2
	advance float64
	// ink is the box of the strokes; empty for a line with nothing visible.
	ink geom.Box2
}

// layoutStrokes lays out s with the stroke font. A character the font lacks is drawn through its
// compatibility decomposition (NFKD) when that leads to characters it has, an accented letter as
// its base letter with the mark; anything else draws as '?'.
func layoutStrokes(s string) strokeLine { return layoutText(s, true) }

// layoutText is layoutStrokes; without draw it only advances the pen, so the result holds the
// advance alone (no strokes, an empty ink box), at a fraction of the cost.
func layoutText(s string, draw bool) strokeLine {
	f := font()
	var ln strokeLine
	ln.ink = geom.EmptyBox()
	add := func(pts []geom.Vec2, pen, left float64) {
		out := make([]geom.Vec2, len(pts))
		for i, p := range pts {
			out[i] = geom.V((pen+p.X-left)/hersheyCap, (hersheyBaseline-p.Y)/hersheyCap)
			ln.ink = ln.ink.Extend(out[i])
		}
		ln.strokes = append(ln.strokes, out)
	}
	if !draw {
		add = nil
	}
	under, over := false, false
	var underStart, overStart float64
	pen := 0.0
	rule := func(from, y float64) {
		if pen > from && add != nil {
			add([]geom.Vec2{geom.V(from, hersheyBaseline-y*hersheyCap), geom.V(pen, hersheyBaseline-y*hersheyCap)}, 0, 0)
		}
	}
	for _, tk := range decodeText(s) {
		switch {
		case tk.under:
			if under {
				rule(underStart, underlineY)
			}
			under, underStart = !under, pen
			continue
		case tk.ovr:
			if over {
				rule(overStart, overlineY)
			}
			over, overStart = !over, pen
			continue
		}
		base, marks := tk.r, []rune(nil)
		if f[base] == nil {
			// A compatibility decomposition turns accented letters into a letter and its marks,
			// and superscripts and ligatures (², ﬁ) into plain letters.
			d := []rune(norm.NFKD.String(string(base)))
			switch {
			case len(d) > 1 && f[d[0]] != nil && unicode.Is(unicode.Mn, d[1]):
				base, marks = d[0], d[1:]
			case len(d) == 1 && f[d[0]] != nil:
				base = d[0]
			case len(d) > 1 && f[d[0]] != nil:
				// A ligature: draw its letters in turn.
				for _, r := range d[:len(d)-1] {
					if f[r] != nil {
						pen = drawGlyph(f[r], nil, pen, add)
					}
				}
				base = d[len(d)-1]
				if f[base] == nil {
					base = '?'
				}
			default:
				base = '?'
			}
		}
		if g := f[base]; g != nil {
			if len(marks) > 0 && add != nil && (base == 'i' || base == 'j') {
				g = dotless(g)
			}
			pen = drawGlyph(g, marks, pen, add)
		}
	}
	if under {
		rule(underStart, underlineY)
	}
	if over {
		rule(overStart, overlineY)
	}
	ln.advance = pen / hersheyCap
	return ln
}

// drawGlyph draws g at pen, with the combining marks over it, and returns the pen after it.
// With a nil add it only advances the pen.
func drawGlyph(g *glyph, marks []rune, pen float64, add func(pts []geom.Vec2, pen, left float64)) float64 {
	if add == nil {
		return pen + g.right - g.left
	}
	top := math.Inf(1)
	for _, st := range g.strokes {
		add(st, pen, g.left)
		for _, p := range st {
			top = min(top, p.Y)
		}
	}
	if math.IsInf(top, 1) {
		top = -5
	}
	for _, m := range marks {
		for _, st := range markStrokes(m, top) {
			// Marks are drawn about the glyph centre, which is x = 0 in Hershey units.
			add(st, pen, g.left)
		}
	}
	return pen + g.right - g.left
}

// dotless returns i or j without its dot, which gives way to an accent.
func dotless(g *glyph) *glyph {
	d := &glyph{left: g.left, right: g.right}
	for _, st := range g.strokes {
		if st[0].Y >= -5 {
			d.strokes = append(d.strokes, st)
		}
	}
	return d
}

// TextAdvance returns the width of s drawn with the stroke font at height 1 and width factor 1,
// control codes expanded: the length of its baseline. It needs no display, so a caller can size
// text or a box around it without the UI.
func TextAdvance(s string) float64 { return layoutText(s, false).advance }
