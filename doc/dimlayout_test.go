package doc_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

// styled returns the horizontal 120 dimension (line at y = 15) with mod applied to its style.
func styled(length float64, text string, mod func(*doc.DimStyle)) *doc.Dimension {
	d := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(length, 0), DimLine: geom.V(0, 15), Text: text}
	s := doc.DefaultDimStyle()
	mod(&s)
	d.SetStyle(nil, s)
	d.Recompute()
	return d
}

func TestDimensionArrowKinds(t *testing.T) {
	if doc.ArrowKindOf("archtick") != doc.ArrowArchTick || doc.ArrowKindOf("_ClosedFilled") != doc.ArrowClosedFilled ||
		doc.ArrowKindOf("MYARROW") != doc.ArrowUser || doc.ArrowDotBlank.BlockName() != "_DotBlank" {
		t.Fatal("arrow names")
	}
	blank := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "_ClosedBlank" }).Layout()
	a := blank.Arrows[0]
	if a.Kind != doc.ArrowClosedBlank || a.Filled || len(a.Outline) != 3 {
		t.Fatalf("blank %+v", a)
	}
	// The dimension line stops at the blank arrows' bases.
	nearV(t, "line start", blank.DimLine[0].A, geom.V(2.5, 15), 1e-12)
	nearV(t, "line end", blank.DimLine[0].B, geom.V(117.5, 15), 1e-12)

	dot := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "_Dot" }).Layout().Arrows[1]
	if len(dot.Circles) != 1 || !dot.Filled || math.Abs(dot.Circles[0].R-0.625) > 1e-12 || dot.Circles[0].C != geom.V(120, 15) {
		t.Fatalf("dot %+v", dot)
	}
	tick := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "_ArchTick" }).Layout().Arrows[0]
	if len(tick.Lines) != 1 || tick.Width <= 0 || len(tick.Outline) != 0 {
		t.Fatalf("tick %+v", tick)
	}
	// A 45° stroke through the tip.
	if l := tick.Lines[0]; math.Abs(l.A.Lerp(l.B, 0.5).Dist(geom.V(0, 15))) > 1e-12 || math.Abs(math.Abs(l.B.Sub(l.A).Norm().X)-math.Sqrt2/2) > 1e-12 {
		t.Fatalf("tick line %+v", l)
	}
	none := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "_None" }).Layout().Arrows[0]
	if len(none.Curves()) != 0 {
		t.Fatalf("none %+v", none)
	}
	open := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "_Open" }).Layout().Arrows[0]
	if len(open.Lines) != 2 || open.Lines[0].B != geom.V(0, 15) {
		t.Fatalf("open %+v", open)
	}
	sep := styled(120, "", func(s *doc.DimStyle) {
		s.ArrowBlock, s.SeparateArrows, s.ArrowBlock1, s.ArrowBlock2 = "_Dot", true, "_Oblique", "_BoxFilled"
	}).Layout()
	if sep.Arrows[0].Kind != doc.ArrowOblique || sep.Arrows[1].Kind != doc.ArrowBoxFilled || len(sep.Arrows[1].Outline) != 4 {
		t.Fatalf("DIMSAH %v %v", sep.Arrows[0].Kind, sep.Arrows[1].Kind)
	}
	user := styled(120, "", func(s *doc.DimStyle) { s.ArrowBlock = "MYARROW" }).Layout().Arrows[0]
	if user.Kind != doc.ArrowUser || !user.Filled || len(user.Outline) != 3 {
		t.Fatalf("user block %+v", user)
	}
}

func TestDimensionFit(t *testing.T) {
	long := "LONG TEXT HERE"
	// Best fit: the arrows fit, the text does not: the text moves out past Ext2, the line runs
	// out under it.
	d := styled(10, long, func(*doc.DimStyle) {})
	l := d.Layout()
	if l.Flipped || !l.TextOutside || d.TextPos.X <= 10 {
		t.Fatalf("best fit: flipped %v outside %v at %v", l.Flipped, l.TextOutside, d.TextPos)
	}
	ext := l.DimLine[len(l.DimLine)-1]
	if ext.A != geom.V(10, 15) || ext.B.X < d.TextPos.X {
		t.Fatalf("line extension %+v", ext)
	}
	// DIMATFIT 0: both out.
	if l := styled(10, long, func(s *doc.DimStyle) { s.Fit = doc.DimFitBoth }).Layout(); !l.Flipped || !l.TextOutside {
		t.Fatal("DIMATFIT 0")
	}
	// DIMATFIT 1 with text that fits alone: only the arrows move.
	if l := styled(10, "", func(s *doc.DimStyle) { s.Fit = doc.DimFitArrowsFirst; s.ArrowSize = 4.5 }).Layout(); !l.Flipped || l.TextOutside {
		t.Fatal("DIMATFIT 1")
	}
	// DIMTIX keeps the text inside; the arrows go out.
	d = styled(10, long, func(s *doc.DimStyle) { s.TextInside = true })
	if l := d.Layout(); !l.Flipped || l.TextOutside || math.Abs(d.TextPos.X-5) > 1e-9 {
		t.Fatalf("DIMTIX: %v %v %v", l.Flipped, l.TextOutside, d.TextPos)
	}
	// DIMTOFL off: no line between the extension lines when the arrows are outside.
	l = styled(4, "", func(s *doc.DimStyle) { s.LineInside = false }).Layout()
	if !l.Flipped || len(l.DimLine) != 2 {
		t.Fatalf("DIMTOFL off: %d lines", len(l.DimLine))
	}
	if l := styled(4, "", func(*doc.DimStyle) {}).Layout(); len(l.DimLine) != 3 {
		t.Fatalf("DIMTOFL on: %d lines", len(l.DimLine))
	}
}

func TestDimensionTextPlacement(t *testing.T) {
	// DIMTAD 0: centred on the line, which breaks around the text.
	d := styled(120, "", func(s *doc.DimStyle) { s.TextVertical = doc.DimTextCentered })
	l := d.Layout()
	if math.Abs(d.TextPos.Y-15) > 1e-12 || len(l.DimLine) != 2 || l.DimLine[0].B.X >= 60 || l.DimLine[1].A.X <= 60 {
		t.Fatalf("centred: pos %v lines %+v", d.TextPos, l.DimLine)
	}
	// DIMTAD 4: below.
	if d := styled(120, "", func(s *doc.DimStyle) { s.TextVertical = doc.DimTextBelow }); math.Abs(d.TextPos.Y-(15-1.875)) > 1e-12 {
		t.Fatalf("below: %v", d.TextPos)
	}
	// DIMTAD 2: away from the origins (the line is above them, so above).
	if d := styled(120, "", func(s *doc.DimStyle) { s.TextVertical = doc.DimTextOutside }); d.TextPos.Y <= 15 {
		t.Fatalf("outside: %v", d.TextPos)
	}
	// DIMTIH on a vertical dimension: horizontal text, centred on the line whatever DIMTAD says.
	v := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(0, 100), DimLine: geom.V(20, 0), Rotation: math.Pi / 2}
	s := doc.DefaultDimStyle()
	s.TextInsideHorizontal = true
	v.SetStyle(nil, s)
	v.Recompute()
	if lv := v.Layout(); lv.Text.Rotation != 0 || math.Abs(v.TextPos.X-20) > 1e-12 || len(lv.DimLine) != 2 {
		t.Fatalf("DIMTIH: rot %v pos %v lines %d", lv.Text.Rotation, v.TextPos, len(lv.DimLine))
	}
	// DIMJUST 1: next to the first extension line.
	if d := styled(120, "", func(s *doc.DimStyle) { s.TextJust = doc.DimJustExt1 }); d.TextPos.X > 20 || d.TextPos.X < 2.5 {
		t.Fatalf("DIMJUST 1: %v", d.TextPos)
	}
	// DIMJUST 4: over the second extension line, aligned with it.
	d = styled(120, "", func(s *doc.DimStyle) { s.TextJust = doc.DimJustOverExt2 })
	if l := d.Layout(); math.Abs(l.Text.Rotation-math.Pi/2) > 1e-12 || d.TextPos.Y <= 15 || d.TextPos.X <= 120 {
		t.Fatalf("DIMJUST 4: rot %v pos %v", l.Text.Rotation, d.TextPos)
	}
	// The text style and colours reach the layout and the style.
	d = styled(120, "", func(s *doc.DimStyle) { s.TextStyle = "Titles" })
	if d.Layout().Text.Style != "Titles" {
		t.Fatal("DIMTXSTY")
	}
}
