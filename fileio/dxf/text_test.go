package dxf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
)

// TestTextRoundTrip writes TEXT of every justification, with rotation, width factor, oblique
// angle, generation flags, a style and characters outside ASCII, reads it back and checks every
// field and the drawn geometry.
func TestTextRoundTrip(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	var want []*doc.Text
	for i, j := range []string{"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"} {
		h, v, _ := doc.ParseJustify(j)
		p := geom.V(float64(i*30), float64(i)*-7.5)
		e := doc.NewText(p, 2.5, 0.25*float64(i), j+" caffè %%d ±")
		e.HAlign, e.VAlign, e.Align = h, v, p
		if h == doc.TextAligned || h == doc.TextFit {
			e.Align = p.Add(geom.V(25, 5))
		}
		if i%3 == 1 {
			e.WidthFactor, e.Oblique, e.Style = 0.8, 15*math.Pi/180, "NOTES"
		}
		if i%4 == 2 {
			e.Generation = doc.TextBackward | doc.TextUpsideDown
		}
		e.Color = 1 + i
		tx.Add(e)
		want = append(want, e)
	}
	tx.Commit()

	var buf bytes.Buffer
	warns, err := (dxf.Codec{}).Write(&buf, d)
	if err != nil || len(warns) != 0 {
		t.Fatalf("write: %v %q", err, warns)
	}
	out := buf.String()
	if !strings.Contains(out, "caff\\U+00E8") || !strings.Contains(out, "\n  2\nStandard\n") || !strings.Contains(out, "\n  2\nNOTES\n") {
		t.Fatalf("output lacks the escaped value or a style record:\n%s", out)
	}
	back, warns, err := (dxf.Codec{}).Read(strings.NewReader(out))
	if err != nil || len(warns) != 0 {
		t.Fatalf("read: %v %q", err, warns)
	}
	for _, w := range want {
		g, ok := back.Entity(w.Handle()).(*doc.Text)
		if !ok {
			t.Fatalf("%s: %T", w.Handle(), back.Entity(w.Handle()))
		}
		if g.Value != w.Value || g.HAlign != w.HAlign || g.VAlign != w.VAlign || g.Generation != w.Generation ||
			g.Color != w.Color || !strings.EqualFold(g.Style, w.Style) || math.Abs(g.Oblique-w.Oblique) > 1e-12 {
			t.Fatalf("%s: got %+v\nwant %+v", w.Justify(), g, w)
		}
		wl, gl := w.Layout(), g.Layout()
		if !gl.Origin.Near(wl.Origin, 1e-9) || math.Abs(gl.Height-wl.Height) > 1e-9 || math.Abs(gl.Rotation-wl.Rotation) > 1e-9 ||
			math.Abs(gl.WidthFactor-wl.WidthFactor) > 1e-9 || !g.Anchor().Near(w.Anchor(), 1e-9) {
			t.Fatalf("%s: layout %+v, want %+v", w.Justify(), gl, wl)
		}
		gb, wb := g.Bounds(), w.Bounds()
		if !gb.Min.Near(wb.Min, 1e-9) || !gb.Max.Near(wb.Max, 1e-9) {
			t.Fatalf("%s: bounds %+v, want %+v", w.Justify(), gb, wb)
		}
	}
}

// TestReadAutoCADText reads TEXT as AutoCAD writes it: group 10 holds the computed start and is
// ignored for justified text, 11 places it, and a -Z extrusion mirrors it.
func TestReadAutoCADText(t *testing.T) {
	src := richFile(group("0", "TEXT", "5", "70", "8", "0", "10", "999", "20", "999", "30", "0", "40", "2", "1", "  Hi  ",
		"72", "2", "11", "10", "21", "20", "31", "0", "73", "3") +
		group("0", "TEXT", "5", "71", "8", "0", "10", "5", "20", "1", "30", "0", "40", "1", "1", "M", "50", "30",
			"210", "0", "220", "0", "230", "-1"))
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %q", err, warns)
	}
	tr := d.Entity(0x70).(*doc.Text)
	if tr.Justify() != "TR" || tr.Value != "  Hi  " || tr.Style != doc.DefaultTextStyle || tr.WidthFactor != 1 {
		t.Fatalf("text = %+v", tr)
	}
	if c := tr.Layout().Corners[2]; !c.Near(geom.V(10, 20), 1e-9) {
		t.Fatalf("top-right corner = %v", c)
	}
	m := d.Entity(0x71).(*doc.Text)
	// In the mirrored coordinate system the text starts at (-5, 1) and runs at 180-30 degrees.
	if !m.Insert.Near(geom.V(-5, 1), 1e-12) || math.Abs(m.Rotation-150*math.Pi/180) > 1e-12 || m.Generation != doc.TextUpsideDown {
		t.Fatalf("mirrored text = %+v", m)
	}
	// Its strokes are the OCS strokes with x negated.
	ocs := doc.NewText(geom.V(5, 1), 1, 30*math.Pi/180, "M")
	g, o := m.Geometry(), ocs.Geometry()
	for i := range g {
		for k, p := range g[i].(geom.Polyline).Pts {
			q := o[i].(geom.Polyline).Pts[k]
			if !p.Near(geom.V(-q.X, q.Y), 1e-9) {
				t.Fatalf("stroke %d point %d = %v, want %v", i, k, p, geom.V(-q.X, q.Y))
			}
		}
	}
}

// TestMTextReadAndR12Degrade reads MTEXT (value chunks in group 3, a direction vector) and writes
// it to R12 as one TEXT per line, with a warning, at the lines' baselines.
func TestMTextReadAndR12Degrade(t *testing.T) {
	src := richFile(group("0", "MTEXT", "5", "80", "8", "0", "62", "3", "10", "1", "20", "2", "30", "0", "40", "2", "41", "40",
		"71", "1", "11", "0", "21", "1", "31", "0", "7", "NOTES", "3", "Ground floor plan ", "1", `\Pscale 1:100 caff\U+00E8`))
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %q", err, warns)
	}
	m, ok := d.Entity(0x80).(*doc.MText)
	if !ok || m.Value != `Ground floor plan \Pscale 1:100 caff`+"è" || math.Abs(m.Rotation-math.Pi/2) > 1e-12 || m.Width != 40 || m.Style != "NOTES" {
		t.Fatalf("mtext = %+v", d.Entity(0x80))
	}
	lay := m.Layout()
	var buf bytes.Buffer
	warns, err = (dxf.Codec{}).Write(&buf, d)
	if err != nil || len(warns) != 1 || !strings.Contains(warns[0], "MTEXT written as TEXT") {
		t.Fatalf("write: %v %q", err, warns)
	}
	back, _, err := (dxf.Codec{}).Read(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	var texts []*doc.Text
	for _, h := range back.Handles() {
		if x, ok := back.Entity(h).(*doc.Text); ok {
			texts = append(texts, x)
		}
	}
	if len(texts) != len(lay.Lines) || texts[0].Handle() != 0x80 || texts[0].Color != 3 {
		t.Fatalf("%d texts for %d lines: %+v", len(texts), len(lay.Lines), texts)
	}
	for i, x := range texts {
		if x.Value != lay.Lines[i] || !x.Insert.Near(lay.Starts[i], 1e-9) || x.Height != 2 || math.Abs(x.Rotation-math.Pi/2) > 1e-12 {
			t.Fatalf("line %d: %+v, want %q at %v", i, x, lay.Lines[i], lay.Starts[i])
		}
	}
}

// TestBackwardAlignedTextKeepsItsBaseline: group 10 of Aligned and Fit text is the first
// baseline point even when the text is backward, so the baseline survives a round trip.
func TestBackwardAlignedTextKeepsItsBaseline(t *testing.T) {
	for _, h := range []doc.TextHAlign{doc.TextAligned, doc.TextFit} {
		d := doc.New()
		tx, _ := d.Begin()
		e := doc.NewText(geom.V(1, 2), 2, 0, "Backward")
		e.HAlign, e.Align, e.Generation = h, geom.V(31, 12), doc.TextBackward
		hd := tx.Add(e)
		tx.Commit()
		var buf bytes.Buffer
		if _, err := (dxf.Codec{}).Write(&buf, d); err != nil {
			t.Fatal(err)
		}
		back, _, err := (dxf.Codec{}).Read(strings.NewReader(buf.String()))
		if err != nil {
			t.Fatal(err)
		}
		g := back.Entity(hd).(*doc.Text)
		if !g.Insert.Near(geom.V(1, 2), 1e-9) || !g.Align.Near(geom.V(31, 12), 1e-9) {
			t.Fatalf("%v: baseline %v..%v, want (1,2)..(31,12)", h, g.Insert, g.Align)
		}
	}
}

// TestMTextWarnsAboutWhatItIgnores: an MTEXT extrusion other than +Z or a top-to-bottom drawing
// direction is not modelled, and reading says so.
func TestMTextWarnsAboutWhatItIgnores(t *testing.T) {
	src := richFile(group("0", "MTEXT", "5", "81", "8", "0", "10", "1", "20", "2", "40", "2", "1", "a", "210", "0", "220", "0", "230", "-1") +
		group("0", "MTEXT", "5", "82", "8", "0", "10", "1", "20", "2", "40", "2", "1", "b", "72", "3") +
		group("0", "MTEXT", "5", "83", "8", "0", "10", "1", "20", "2", "40", "2", "1", "c", "72", "1", "230", "1"))
	_, warns, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil || len(warns) != 1 || !strings.Contains(warns[0], "2 MTEXT") {
		t.Fatalf("%v %q", err, warns)
	}
}
