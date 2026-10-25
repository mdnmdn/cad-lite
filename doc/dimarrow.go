package doc

import (
	"math"
	"strings"

	"cad-lite/geom"
)

// DIMTAD values: where the text sits across the dimension line (DimStyle.TextVertical).
const (
	// DimTextCentered centres the text on the dimension line, which is broken around it.
	DimTextCentered = 0
	// DimTextAbove puts the text above the line, in the reading sense (ISO-25).
	DimTextAbove = 1
	// DimTextOutside puts the text on the side of the line away from the definition points.
	DimTextOutside = 2
	// DimTextJIS places the text to JIS; it is drawn as DimTextAbove.
	DimTextJIS = 3
	// DimTextBelow puts the text below the line.
	DimTextBelow = 4
)

// DIMJUST values: where the text sits along the dimension line (DimStyle.TextJust).
const (
	// DimJustCenter centres the text between the extension lines.
	DimJustCenter = 0
	// DimJustExt1 puts the text next to the first extension line.
	DimJustExt1 = 1
	// DimJustExt2 puts the text next to the second extension line.
	DimJustExt2 = 2
	// DimJustOverExt1 puts the text over the first extension line, aligned with it.
	DimJustOverExt1 = 3
	// DimJustOverExt2 puts the text over the second extension line, aligned with it.
	DimJustOverExt2 = 4
)

// DIMATFIT values: what moves outside the extension lines when the text and the arrows do not
// both fit between them (DimStyle.Fit). DIMTIX keeps the text inside whatever DIMATFIT says.
const (
	// DimFitBoth moves the text and the arrows outside.
	DimFitBoth = 0
	// DimFitArrowsFirst moves the arrows outside first, then the text.
	DimFitArrowsFirst = 1
	// DimFitTextFirst moves the text outside first, then the arrows.
	DimFitTextFirst = 2
	// DimFitBest moves whichever fits best: the arrows when the text fits alone, else the text
	// when the arrows fit alone, else both (AutoCAD's default).
	DimFitBest = 3
)

// ArrowKind is the shape of a dimension arrowhead, named by AutoCAD's DIMBLK block names.
type ArrowKind int

// The arrowheads, in the order of AutoCAD's list. ArrowUser is a DIMBLK naming a block of the
// drawing's own, which the layout draws as ArrowClosedFilled.
const (
	ArrowClosedFilled ArrowKind = iota // DIMBLK "": closed filled
	ArrowClosedBlank                   // _ClosedBlank: closed blank, the dimension line stops at its base
	ArrowClosed                        // _Closed: closed outline with the line through it
	ArrowDot                           // _Dot: filled dot
	ArrowArchTick                      // _ArchTick: architectural tick, a thick oblique stroke
	ArrowOblique                       // _Oblique: oblique stroke
	ArrowOpen                          // _Open: open V
	ArrowOrigin                        // _Origin: origin indicator, a circle
	ArrowOrigin2                       // _Origin2: origin indicator 2, two circles
	ArrowOpen90                        // _Open90: right-angle V
	ArrowOpen30                        // _Open30: 30° V
	ArrowDotSmall                      // _DotSmall: small filled dot
	ArrowDotBlank                      // _DotBlank: blank dot
	ArrowSmall                         // _Small: small closed filled
	ArrowBoxBlank                      // _BoxBlank: blank box
	ArrowBoxFilled                     // _BoxFilled: filled box
	ArrowDatumBlank                    // _DatumBlank: blank datum triangle
	ArrowDatumFilled                   // _DatumFilled: filled datum triangle
	ArrowIntegral                      // _Integral: integral sign
	ArrowNone                          // _None: no arrowhead
	ArrowUser                          // any other block name
)

var arrowNames = [...]string{"", "_ClosedBlank", "_Closed", "_Dot", "_ArchTick", "_Oblique", "_Open", "_Origin",
	"_Origin2", "_Open90", "_Open30", "_DotSmall", "_DotBlank", "_Small", "_BoxBlank", "_BoxFilled",
	"_DatumBlank", "_DatumFilled", "_Integral", "_None"}

// BlockName returns the kind's DIMBLK value as AutoCAD writes it ("_ArchTick"; "" for
// ArrowClosedFilled and ArrowUser).
func (k ArrowKind) BlockName() string {
	if k >= 0 && int(k) < len(arrowNames) {
		return arrowNames[k]
	}
	return ""
}

// ArrowKindOf returns the kind a DIMBLK value names: "" (and AutoCAD's "_ClosedFilled") is
// ArrowClosedFilled; names match without regard to case and with or without the leading
// underscore; any other name is ArrowUser.
func ArrowKindOf(block string) ArrowKind {
	b := strings.TrimPrefix(strings.TrimSpace(block), "_")
	if b == "" || strings.EqualFold(b, "ClosedFilled") {
		return ArrowClosedFilled
	}
	for i, n := range arrowNames {
		if i > 0 && strings.EqualFold(n[1:], b) {
			return ArrowKind(i)
		}
	}
	return ArrowUser
}

// ArrowBlocks returns the arrowheads of the first and second ends: DIMBLK1 and DIMBLK2 when
// DIMSAH is on, DIMBLK for both otherwise.
func (s DimStyle) ArrowBlocks() (first, second string) {
	if s.SeparateArrows {
		return s.ArrowBlock1, s.ArrowBlock2
	}
	return s.ArrowBlock, s.ArrowBlock
}

// DimArrow is one laid-out arrowhead, in model space. A renderer draws Outline (filled when
// Filled), Lines (Width wide when Width > 0), Arcs and Circles (filled when Filled).
type DimArrow struct {
	// Kind is the shape and Block the DIMBLK value it came from.
	Kind  ArrowKind
	Block string
	// Tip is where the arrow points: the dimension line's end at the extension line.
	Tip geom.Vec2
	// Dir is the unit direction the arrow points in, from its body to Tip.
	Dir geom.Vec2
	// Size is the arrow size in drawing units (DIMASZ × DIMSCALE).
	Size float64
	// Outline is the closed polygon of a triangle, box or datum arrow; nil for the others.
	Outline []geom.Vec2
	// Filled reports a solid Outline or Circles (closed filled, small, dots, filled box and
	// datum).
	Filled bool
	// Lines are open strokes: the sides of an open arrow, a tick.
	Lines []geom.Line
	// Width is the stroke width of an architectural tick, 0 for the others.
	Width float64
	// Circles are the dots and origin indicators; Arcs the integral sign.
	Circles []geom.Circle
	Arcs    []geom.Arc
	// LineEnd is where the dimension line stops at this arrow: Tip, or the arrow's back for a
	// blank arrow (closed blank, blank dot) so the line does not run through it.
	LineEnd geom.Vec2
}

// LayoutArrow builds the arrowhead DIMBLK block names at tip, pointing along dir (a unit vector),
// size long. LayoutArrow(name, (0,0), (1,0), 1) is AutoCAD's unit arrow block of that name.
func LayoutArrow(block string, tip, dir geom.Vec2, size float64) DimArrow {
	k := ArrowKindOf(block)
	a := DimArrow{Kind: k, Block: block, Tip: tip, Dir: dir, Size: size, LineEnd: tip}
	// at maps a point of AutoCAD's unit arrow block (tip at the origin, pointing along +X) to
	// model space.
	n := dir.Perp()
	at := func(x, y float64) geom.Vec2 { return tip.Add(dir.Scale(x * size)).Add(n.Scale(y * size)) }
	poly := func(pts ...[2]float64) {
		for _, p := range pts {
			a.Outline = append(a.Outline, at(p[0], p[1]))
		}
	}
	vee := func(x, y float64) {
		a.Lines = []geom.Line{{A: at(x, y), B: tip}, {A: at(x, -y), B: tip}}
	}
	circle := func(r float64) { a.Circles = append(a.Circles, geom.Circle{C: tip, R: r * size}) }
	switch k {
	case ArrowClosedFilled, ArrowUser:
		poly([2]float64{0, 0}, [2]float64{-1, 1.0 / 6}, [2]float64{-1, -1.0 / 6})
		a.Filled = true
	case ArrowClosedBlank:
		poly([2]float64{0, 0}, [2]float64{-1, 1.0 / 6}, [2]float64{-1, -1.0 / 6})
		a.LineEnd = at(-1, 0)
	case ArrowClosed:
		poly([2]float64{0, 0}, [2]float64{-1, 1.0 / 6}, [2]float64{-1, -1.0 / 6})
	case ArrowSmall:
		poly([2]float64{0, 0}, [2]float64{-0.5, 1.0 / 12}, [2]float64{-0.5, -1.0 / 12})
		a.Filled = true
	case ArrowDot:
		circle(0.25)
		a.Filled = true
	case ArrowDotSmall:
		circle(1.0 / 16)
		a.Filled = true
	case ArrowDotBlank:
		circle(0.25)
		a.LineEnd = at(-0.25, 0)
	case ArrowOrigin:
		circle(0.5)
	case ArrowOrigin2:
		circle(0.5)
		circle(0.25)
	case ArrowArchTick, ArrowOblique:
		a.Lines = []geom.Line{{A: at(-0.5, -0.5), B: at(0.5, 0.5)}}
		if k == ArrowArchTick {
			a.Width = 0.15 * size
		}
	case ArrowOpen:
		vee(-1, 1.0/6)
	case ArrowOpen90:
		vee(-0.5, 0.5)
	case ArrowOpen30:
		vee(-1, math.Tan(math.Pi/12))
	case ArrowBoxBlank, ArrowBoxFilled:
		poly([2]float64{-0.5, -0.5}, [2]float64{0.5, -0.5}, [2]float64{0.5, 0.5}, [2]float64{-0.5, 0.5})
		a.Filled = k == ArrowBoxFilled
	case ArrowDatumBlank, ArrowDatumFilled:
		poly([2]float64{0, 0.577350269}, [2]float64{-1, 0}, [2]float64{0, -0.577350269})
		a.Filled = k == ArrowDatumFilled
	case ArrowIntegral:
		// Two quarter circles through the tip, an S across the dimension line.
		const m = 0.5 - 0.35355339059327373
		for _, sg := range []float64{1, -1} {
			if arc, ok := ArcFrom3Points(tip, at(sg*m, sg*0.35355339059327373), at(sg*0.5, sg*0.5)); ok {
				a.Arcs = append(a.Arcs, arc)
			}
		}
	}
	return a
}

// Curves returns the arrow's outline as curves: the closed Outline (a polyline with zero
// bulges), then Lines, Circles and Arcs. EXPLODE and the DXF block writer use it.
func (a DimArrow) Curves() []geom.Curve {
	out, _ := a.curves(false)
	return out
}

// curves returns the arrow's strokes: its outline (closed), lines, circles and arcs, and, when
// withFill, the strokes that make a filled or wide arrow read solid. Only block geometry (the
// parts of a dimension in a block, the dimension's block) asks for them; the renderer paints a
// dimension's arrowheads as solids.
func (a DimArrow) curves(withFill bool) (outline, fill []geom.Curve) {
	if len(a.Outline) > 0 {
		outline = append(outline, geom.Polyline{Pts: a.Outline, Bulges: make([]float64, len(a.Outline)), Closed: true})
	}
	for _, l := range a.Lines {
		outline = append(outline, l)
	}
	for _, c := range a.Circles {
		outline = append(outline, c)
	}
	for _, c := range a.Arcs {
		outline = append(outline, c)
	}
	if !withFill {
		return outline, nil
	}
	if a.Filled && len(a.Outline) >= 3 {
		// Strokes from the first vertex to points along the opposite sides.
		p0 := a.Outline[0]
		for i := 1; i+1 < len(a.Outline); i++ {
			for j := 1; j < arrowFill; j++ {
				fill = append(fill, geom.Line{A: p0, B: a.Outline[i].Lerp(a.Outline[i+1], float64(j)/arrowFill)})
			}
		}
	}
	if a.Filled {
		for _, c := range a.Circles {
			for j := 1; j < arrowFill; j++ {
				fill = append(fill, geom.Circle{C: c.C, R: c.R * float64(j) / arrowFill})
			}
		}
	}
	if a.Width > 0 {
		for _, l := range a.Lines {
			off := l.B.Sub(l.A).Norm().Perp().Scale(a.Width / 2)
			for _, s := range []float64{-1, -0.5, 0.5, 1} {
				fill = append(fill, geom.Line{A: l.A.Add(off.Scale(s)), B: l.B.Add(off.Scale(s))})
			}
		}
	}
	return outline, fill
}
