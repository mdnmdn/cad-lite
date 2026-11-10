package plot

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"math"
	"regexp"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// sample is a 100×50 rectangle of lines from the origin, in red (ACI 1), and a circle on the
// "hidden" layer the test may turn off or set not to plot.
func sample(t *testing.T) (*doc.Drawing, []doc.Handle) {
	t.Helper()
	d := doc.New()
	d.AddLayer("other", 3)
	tx, _ := d.Begin()
	var hs []doc.Handle
	pts := []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 50), geom.V(0, 50)}
	for i := range pts {
		hs = append(hs, tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Color: 1}, Start: pts[i], End: pts[(i+1)%4]}))
	}
	hs = append(hs, tx.Add(&doc.Circle{EntityCommon: doc.EntityCommon{Layer: "other"}, Center: geom.V(300, 300), Radius: 10}))
	tx.Commit()
	return d, hs
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestComposeFitsExtentsCentredOnLandscapeA4(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("a4")
	s, err := Compose(d, Options{Paper: a4, Landscape: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Width != 297 || s.Height != 210 {
		t.Fatalf("paper %vx%v, want 297x210", s.Width, s.Height)
	}
	// Extents (0,0)-(310,310): 190 mm of printable height over 310 units.
	if !near(s.Scale, 190.0/310) {
		t.Fatalf("scale %v, want %v", s.Scale, 190.0/310)
	}
	first := redItems(s)[0].Points[0]
	if !near(first.X, 297.0/2-155*s.Scale) || !near(first.Y, 10) {
		t.Fatalf("origin plots at %v", first)
	}
}

func TestComposeSelectionPlotsOnlyTheSelectionAndFitsIt(t *testing.T) {
	d, hs := sample(t)
	a4, _ := PaperByName("A4")
	s, err := Compose(d, Options{Paper: a4, Landscape: true, What: WhatSelection, Selection: hs[:4]})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 4 {
		t.Fatalf("%d items, want the 4 selected lines", len(s.Items))
	}
	if !near(s.Scale, 277.0/100) {
		t.Fatalf("scale %v, want the 100-unit width over 277 mm", s.Scale)
	}
	if _, err := Compose(d, Options{Paper: a4, What: WhatSelection}); !errors.Is(err, ErrNothing) {
		t.Fatalf("empty selection: %v, want ErrNothing", err)
	}
}

func TestComposeWindowClipsAndSkipsWhatIsOutside(t *testing.T) {
	d, _ := sample(t)
	a3, _ := PaperByName("A3")
	s, err := Compose(d, Options{Paper: a3, Area: AreaWindow, Window: geom.BoxOf(geom.V(50, 60), geom.V(-10, -10))})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range s.Items {
		if it.Color != (render.Color{R: 255, A: 255}) {
			t.Fatalf("the far circle was plotted: %+v", it.Color)
		}
	}
	w := s.Clip.Max.X - s.Clip.Min.X
	if !near(w, 60*s.Scale) || s.Clip.Min.X < 10-1e-9 {
		t.Fatalf("clip %+v does not frame the window", s.Clip)
	}
	if _, err := Compose(d, Options{Paper: a3, Area: AreaWindow, Window: geom.BoxOf(geom.V(1000, 1000), geom.V(1100, 1100))}); !errors.Is(err, ErrNothing) {
		t.Fatalf("empty window: %v, want ErrNothing", err)
	}
}

func TestComposeSkipsLayersThatAreOffOrDoNotPlot(t *testing.T) {
	d, _ := sample(t)
	d.Layer("other").NoPlot = true
	a4, _ := PaperByName("A4")
	s, err := Compose(d, Options{Paper: a4})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 4 {
		t.Fatalf("%d items, want the circle on the no-plot layer left out", len(s.Items))
	}
	if !near(s.Scale, 190.0/100) {
		t.Fatalf("scale %v: the no-plot layer still counts in the extents", s.Scale)
	}
}

func TestComposeMonoPlotsBlack(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("A4")
	s, _ := Compose(d, Options{Paper: a4, Mono: true})
	for _, it := range s.Items {
		if it.Color != (render.Color{A: 255}) {
			t.Fatalf("mono item in %+v", it.Color)
		}
	}
	s, _ = Compose(d, Options{Paper: a4})
	if len(redItems(s)) != 4 {
		t.Fatal("colour plot lost the red lines")
	}
}

func TestComposeUsesLineweightsInMillimetres(t *testing.T) {
	d, hs := sample(t)
	tx, _ := d.Begin()
	tx.Modify(hs[0], func(e doc.Entity) { e.(*doc.Line).Lineweight = 70 })
	tx.Commit()
	a4, _ := PaperByName("A4")
	s, _ := Compose(d, Options{Paper: a4, What: WhatSelection, Selection: hs[:2]})
	if !near(s.Items[0].Width, 0.70) || !near(s.Items[1].Width, 0.25) {
		t.Fatalf("widths %v and %v, want 0.70 and the default 0.25", s.Items[0].Width, s.Items[1].Width)
	}
}

func TestWritePDFIsAOnePageVectorFile(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("A4")
	s, _ := Compose(d, Options{Paper: a4, Landscape: true})
	var buf bytes.Buffer
	if err := s.WritePDF(&buf, "plate (1)"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"%PDF-1.4", "/MediaBox [0 0 841.890 595.276]", "/Count 1", "/Title (plate \\(1\\))", "%%EOF"} {
		if !strings.Contains(out, want) {
			t.Fatalf("PDF lacks %q", want)
		}
	}
	// The xref offsets point at their objects.
	m := regexp.MustCompile(`startxref\n(\d+)`).FindStringSubmatch(out)
	if m == nil || !strings.HasPrefix(out[atoi(m[1]):], "xref") {
		t.Fatal("startxref does not point at the xref table")
	}
	for i, off := range regexp.MustCompile(`(\d{10}) 00000 n`).FindAllStringSubmatch(out, -1) {
		if !strings.HasPrefix(out[atoi(off[1]):], itoa(i+1)+" 0 obj") {
			t.Fatalf("xref entry %d is off", i+1)
		}
	}
	start := strings.Index(out, "stream\n") + len("stream\n")
	end := strings.Index(out, "\nendstream")
	zr, err := zlib.NewReader(strings.NewReader(out[start:end]))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(zr)
	for _, want := range []string{" re W n", "1.000 0.000 0.000 RG", " m\n", " l\n", "S\n"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("content stream lacks %q:\n%s", want, content)
		}
	}
}

func TestRasterDrawsInkOnWhitePaper(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("A4")
	hs := d.Handles()
	s, _ := Compose(d, Options{Paper: a4, Mono: true, What: WhatSelection, Selection: hs[:4]})
	img := s.Raster(1)
	if img.Bounds().Dx() != 210 || img.Bounds().Dy() != 297 {
		t.Fatalf("raster %v, want 210x297", img.Bounds())
	}
	if c := img.NRGBAAt(2, 2); c.R != 255 || c.G != 255 || c.B != 255 {
		t.Fatalf("margin is %v, want white", c)
	}
	// The 100×50 rectangle fills the 190 mm width, so its bottom edge is 47.5 mm under the
	// middle of the page.
	dark := 0
	for x := 0; x < img.Bounds().Dx(); x++ {
		for y := 194; y < 199; y++ {
			if img.NRGBAAt(x, y).R < 128 {
				dark++
				break
			}
		}
	}
	if dark < 50 {
		t.Fatalf("only %d columns of ink along the bottom edge", dark)
	}
}

func TestPrintCommandPerPlatform(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	if n, args, _ := printCommand("linux", "/t/a.pdf", "a", all); n != "lp" || strings.Join(args, " ") != "-t a -- /t/a.pdf" {
		t.Fatalf("linux: %s %v", n, args)
	}
	if n, _, _ := printCommand("darwin", "/t/a.pdf", "a", func(s string) bool { return s == "lpr" }); n != "lpr" {
		t.Fatalf("lpr fallback: %s", n)
	}
	if _, _, err := printCommand("linux", "/t/a.pdf", "a", none); !errors.Is(err, ErrNoPrinting) {
		t.Fatalf("no spooler: %v", err)
	}
	if n, args, _ := printCommand("windows", `C:\it's.pdf`, "a", none); n != "powershell" || !strings.Contains(args[len(args)-1], `'C:\it''s.pdf' -Verb Print`) {
		t.Fatalf("windows: %s %v", n, args)
	}
	if _, _, err := printCommand("js", "a.pdf", "a", all); !errors.Is(err, ErrNoPrinting) {
		t.Fatalf("browser: %v", err)
	}
}

// redItems are the sample's four red lines.
func redItems(s *Sheet) []Item {
	var out []Item
	for _, it := range s.Items {
		if it.Color == (render.Color{R: 255, A: 255}) {
			out = append(out, it)
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
