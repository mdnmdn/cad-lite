package plot

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

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
	s, _ := Compose(d, Options{Paper: a4, Style: StyleMono})
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
	s, _ := Compose(d, Options{Paper: a4, Style: StyleMono, What: WhatSelection, Selection: hs[:4]})
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
	if n, args, _, _ := printCommand("linux", "/t/a.pdf", Job{Title: "a"}, all); n != "lp" || strings.Join(args, " ") != "-t a -n 1 -- /t/a.pdf" {
		t.Fatalf("linux: %s %v", n, args)
	}
	if n, args, _, _ := printCommand("linux", "/t/a.pdf", Job{Title: "a", Printer: "office", Copies: 3}, all); strings.Join(args, " ") != "-t a -n 3 -d office -- /t/a.pdf" {
		t.Fatalf("linux, printer and copies: %s %v", n, args)
	}
	if n, args, _, _ := printCommand("darwin", "/t/a.pdf", Job{Title: "a", Printer: "p", Copies: 500}, func(s string) bool { return s == "lpr" }); n != "lpr" || strings.Join(args, " ") != "-T a -# 99 -P p /t/a.pdf" {
		t.Fatalf("lpr fallback: %s %v", n, args)
	}
	if _, _, _, err := printCommand("linux", "/t/a.pdf", Job{}, none); !errors.Is(err, ErrNoPrinting) {
		t.Fatalf("no spooler: %v", err)
	}
	// Windows: the path and the printer travel in the environment, never in the script, so quotes
	// of any kind in them cannot change it.
	path, printer := `C:\Users\O’Brien's\a.pdf`, "Bob’s \"HP\"; Remove-Item x"
	n, args, env, _ := printCommand("windows", path, Job{Copies: 2, Printer: printer}, none)
	script := args[len(args)-1]
	if n != "powershell" || strings.Contains(script, "Brien") || strings.Contains(script, "Bob") || !strings.Contains(script, "-lt 2;") ||
		!strings.Contains(script, "-Verb PrintTo") {
		t.Fatalf("windows script: %s", script)
	}
	if !slices.Contains(env, envPlotFile+"="+path) || !slices.Contains(env, envPlotPrinter+"=Bob’s HP; Remove-Item x") {
		t.Fatalf("windows env: %v", env)
	}
	if _, args, env, _ := printCommand("windows", path, Job{}, none); !strings.Contains(args[len(args)-1], "-Verb Print ") || strings.Contains(args[len(args)-1], "PrintTo") || len(env) != 1 {
		t.Fatalf("windows default printer: %v %v", args, env)
	}
	if _, _, _, err := printCommand("js", "a.pdf", Job{}, all); !errors.Is(err, ErrNoPrinting) {
		t.Fatalf("browser: %v", err)
	}
}

func TestListPrintersReadsLpstat(t *testing.T) {
	all := func(string) bool { return true }
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "-e":
			return []byte("Office_Laser\nPlotter_A0\n"), nil
		case "-d":
			return []byte("system default destination: Plotter_A0\n"), nil
		}
		return nil, errors.New("unexpected")
	}
	ps, err := listPrinters(context.Background(), "linux", all, run)
	if err != nil || len(ps) != 2 || ps[0] != (Printer{"Office_Laser", false}) || ps[1] != (Printer{"Plotter_A0", true}) {
		t.Fatalf("printers %+v, %v", ps, err)
	}
	old := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "-a":
			return []byte("hp accepting requests since Mon 01 Jan\nold not accepting requests since Tue\n\tPaused by admin\n"), nil
		case "-d":
			return []byte("no system default destination\n"), nil
		}
		return nil, errors.New("lpstat: invalid option -e")
	}
	if ps, _ := listPrinters(context.Background(), "darwin", all, old); len(ps) != 2 || ps[0] != (Printer{"hp", false}) || ps[1].Name != "old" {
		t.Fatalf("lpstat -a fallback: %+v", ps)
	}
	win := func(_ context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Microsoft Print to PDF\tFalse\r\nHP LaserJet\tTrue\r\n"), nil
	}
	if ps, _ := listPrinters(context.Background(), "windows", all, win); len(ps) != 2 || ps[1] != (Printer{"HP LaserJet", true}) || ps[0].Name != "Microsoft Print to PDF" {
		t.Fatalf("windows: %+v", ps)
	}
	if _, err := listPrinters(context.Background(), "linux", func(string) bool { return false }, run); !errors.Is(err, ErrNoPrinting) {
		t.Fatalf("no CUPS: %v", err)
	}
}

func TestViewerCommandPerPlatform(t *testing.T) {
	for goos, want := range map[string]string{"linux": "xdg-open", "freebsd": "xdg-open", "darwin": "open", "windows": "rundll32"} {
		if n, args, err := viewerCommand(goos, "/t/a.pdf"); err != nil || n != want || args[len(args)-1] != "/t/a.pdf" {
			t.Fatalf("%s: %s %v %v", goos, n, args, err)
		}
	}
	if _, _, err := viewerCommand("js", "a"); !errors.Is(err, ErrNoPrinting) {
		t.Fatal("the browser has no viewer")
	}
}

func TestParseScale(t *testing.T) {
	for in, want := range map[string]Scale{"fit": Fit, "": Fit, "1:50": {1, 50}, " 1 / 100 ": {1, 100}, "2=1": {2, 1}, "1:2.5": {1, 2.5}} {
		if got, ok := ParseScale(in); !ok || got != want {
			t.Fatalf("%q: %v %v, want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"1:0", "-1:5", "abc", "1:x", "50", "1:inf", "1:NaN"} {
		if _, ok := ParseScale(in); ok {
			t.Fatalf("%q parsed", in)
		}
	}
	if (Scale{1, 50}).String() != "1:50" || Fit.String() != "fit" {
		t.Fatal("String")
	}
}

func TestComposeFixedScaleUsesDrawingUnits(t *testing.T) {
	d, hs := sample(t)
	a4, _ := PaperByName("A4")
	opt := Options{Paper: a4, Landscape: true, What: WhatSelection, Selection: hs[:4], Scale: Scale{1, 2}}
	s, err := Compose(d, opt)
	if err != nil {
		t.Fatal(err)
	}
	// Millimetre drawing at 1:2: 0.5 mm of paper per unit, the 100×50 rectangle centred.
	if !near(s.Scale, 0.5) || s.Overflow {
		t.Fatalf("scale %v overflow %v", s.Scale, s.Overflow)
	}
	if p := redItems(s)[0].Points[0]; !near(p.X, 297.0/2-25) || !near(p.Y, 105-12.5) {
		t.Fatalf("origin at %v", p)
	}
	d.Header.InsUnits = 6 // metres: 1 unit is 1000 mm, so 1:100 is 10 mm per unit
	opt.Scale = Scale{1, 100}
	s, _ = Compose(d, opt)
	if !near(s.Scale, 10) || !s.Overflow {
		t.Fatalf("metres at 1:100: scale %v overflow %v", s.Scale, s.Overflow)
	}
}

func TestComposeOffsetPlacesTheLowerLeftCorner(t *testing.T) {
	d, hs := sample(t)
	a4, _ := PaperByName("A4")
	s, err := Compose(d, Options{Paper: a4, What: WhatSelection, Selection: hs[:4], Scale: Scale{1, 1}, NoCenter: true, Offset: geom.V(5, 20)})
	if err != nil {
		t.Fatal(err)
	}
	if p := redItems(s)[0].Points[0]; !near(p.X, 15) || !near(p.Y, 30) {
		t.Fatalf("origin at %v, want 10 mm margin plus the offset", p)
	}
	if s.Overflow {
		t.Fatal("100 mm wide at 15 mm fits on A4 portrait")
	}
	s, _ = Compose(d, Options{Paper: a4, What: WhatSelection, Selection: hs[:4], Scale: Scale{1, 1}, NoCenter: true, Offset: geom.V(150, 0)})
	if !s.Overflow {
		t.Fatal("an offset that pushes the plot off the paper should overflow")
	}
}

func TestComposeGrayscaleAndThinLines(t *testing.T) {
	d, hs := sample(t)
	a4, _ := PaperByName("A4")
	s, _ := Compose(d, Options{Paper: a4, Style: StyleGray, What: WhatSelection, Selection: hs[:4], NoLineweights: true})
	it := s.Items[0]
	if it.Color.R != it.Color.G || it.Color.G != it.Color.B || it.Color.R != 54 || it.Width != ThinWidth {
		t.Fatalf("grey red line: %+v width %v", it.Color, it.Width)
	}
}

func TestTempPDFRemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.pdf")
	fresh := filepath.Join(dir, "fresh.pdf")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(old, past, past)
	cleanTemp(dir, time.Now().Add(-tempAge))
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the two-day-old file should be gone")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("the fresh file should stay")
	}
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

// within reports whether every point of every item lies inside box.
func within(s *Sheet, box geom.Box2) bool {
	for _, it := range s.Items {
		for _, p := range it.Points {
			if !box.Contains(p) {
				return false
			}
		}
	}
	return true
}

func TestComposeAtExtremeScalesStaysOnThePaper(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("A4")
	s, err := Compose(d, Options{Paper: a4, Scale: Scale{MaxRatio, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Overflow || !within(s, s.Clip.Grow(5)) {
		t.Fatal("items far off the paper should be cut at the clip box")
	}
	start := time.Now()
	s.Raster(2)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("raster took %v", el)
	}
	// A georeferenced drawing in metres with a stray entity near the origin, at 1:100.
	g := doc.New()
	g.Header.InsUnits = 6
	tx, _ := g.Begin()
	tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(1, 1)})
	tx.Add(&doc.Circle{Center: geom.V(500000, 5000000), Radius: 50})
	tx.Commit()
	s, err = Compose(g, Options{Paper: a4, Scale: Scale{1, 100}})
	if err != nil {
		t.Fatal(err)
	}
	if !within(s, s.Clip.Grow(5)) {
		t.Fatal("the georeferenced plot left the paper")
	}
	s.Raster(2)
	if _, ok := ParseScale("1e7:1"); ok {
		t.Fatal("a ratio past MaxRatio should be refused")
	}
}

func TestComposeRefusesAWindowWithoutHeight(t *testing.T) {
	d, _ := sample(t)
	a4, _ := PaperByName("A4")
	if _, err := Compose(d, Options{Paper: a4, Area: AreaWindow, Window: geom.BoxOf(geom.V(-10, 0), geom.V(110, 0))}); !errors.Is(err, ErrNothing) {
		t.Fatalf("a window of no height: %v, want ErrNothing", err)
	}
}

func TestClipCutsPolylinesAndPolygons(t *testing.T) {
	box := geom.BoxOf(geom.V(0, 0), geom.V(10, 10))
	// In, out and back in: two runs.
	pts, starts := clipPolyline([]geom.Vec2{geom.V(5, 5), geom.V(20, 5), geom.V(20, 8), geom.V(5, 8)}, box, nil, nil)
	if len(starts) != 2 || pts[1] != geom.V(10, 5) || pts[starts[1]] != geom.V(10, 8) || pts[len(pts)-1] != geom.V(5, 8) {
		t.Fatalf("pts %v starts %v", pts, starts)
	}
	if pts, _ := clipPolyline([]geom.Vec2{geom.V(20, 20), geom.V(30, 30)}, box, nil, nil); len(pts) != 0 {
		t.Fatalf("outside segment kept: %v", pts)
	}
	poly := clipPolygon([]geom.Vec2{geom.V(-5, -5), geom.V(5, -5), geom.V(5, 5), geom.V(-5, 5)}, box)
	b := geom.BoxOf(poly...)
	if b != geom.BoxOf(geom.V(0, 0), geom.V(5, 5)) {
		t.Fatalf("clipped square %v", poly)
	}
	s := &Sheet{Clip: box}
	s.add(Item{Points: []geom.Vec2{geom.V(1, 1), geom.V(9, 1), geom.V(9, 9)}, Closed: true, Width: 0.5})
	s.add(Item{Points: []geom.Vec2{geom.V(1, 1), geom.V(1e9, 1)}, Closed: true, Width: 0.5})
	if len(s.Items) != 2 || !s.Items[0].Closed || s.Items[1].Closed || !within(s, box.Grow(1.5)) {
		t.Fatalf("items %+v", s.Items)
	}
}
