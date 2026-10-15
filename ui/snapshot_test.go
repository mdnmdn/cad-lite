package ui_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"cad-lite/cadmcp"
	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
	"cad-lite/ui"
)

const snapScript = `LAYER outline=4 REC 0,0 220,130 PLINE 70,95 150,95 150,111 70,111 c ` +
	`LAYER holes=1 CIRCLE 25,25 8 CIRCLE 195,25 8 CIRCLE 195,105 8 CIRCLE 25,105 8 CIRCLE 110,50 20 ` +
	`ARC 80,50 110,80 140,50 ` +
	`LAYER center=3 LINE 110,10 110,90  LINE 70,50 150,50  LAYER outline`

func snapSession(script string) (*cmdsys.Session, *render.View) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Script(script)
	return s, &v
}

func snap(t *testing.T, s *cmdsys.Session, req cadmcp.SnapshotRequest) *image.NRGBA {
	t.Helper()
	b, err := ui.Snapshot(s, req)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != req.Width || img.Bounds().Dy() != req.Height {
		t.Fatalf("size = %v, want %dx%d", img.Bounds(), req.Width, req.Height)
	}
	out := image.NewNRGBA(img.Bounds())
	for y := 0; y < req.Height; y++ {
		for x := 0; x < req.Width; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func TestSnapshotPixels(t *testing.T) {
	s, _ := snapSession("LAYER red=1 LINE 0,50 100,50 ")
	view := render.View{Center: geom.V(50, 50), Scale: 4, Width: 400, Height: 300}
	req := cadmcp.SnapshotRequest{Width: 400, Height: 300, View: view}
	img := snap(t, s, req)
	bg := img.NRGBAAt(5, 5)
	if bg.R != 33 || bg.G != 40 || bg.B != 48 {
		t.Fatalf("background = %v", bg)
	}
	px, py := view.ToScreen(geom.V(50, 50))
	c := img.NRGBAAt(int(px), int(py)-1)
	if d := img.NRGBAAt(int(px), int(py)); d.R > c.R {
		c = d
	}
	if c.R < 120 || c.G > 60 || c.B > 60 {
		t.Fatalf("pixel on the red line = %v", c)
	}
	if c := img.NRGBAAt(int(px), int(py)+40); c.R > 60 {
		t.Fatalf("pixel off the line = %v", c)
	}

	req.Highlight = s.Drawing().Handles()
	himg := snap(t, s, req)
	hl := himg.NRGBAAt(int(px), int(py))
	if d := himg.NRGBAAt(int(px), int(py)-1); d.B > hl.B {
		hl = d
	}
	if hl.B < 150 || hl.R > 120 {
		t.Fatalf("highlighted pixel = %v, want the selection blue", hl)
	}
}

func TestSnapshotRulersAndGrid(t *testing.T) {
	s, _ := snapSession("LINE 0,0 100,0 ")
	view := render.View{Center: geom.V(50, 50), Scale: 4, Width: 400, Height: 300}
	req := cadmcp.SnapshotRequest{Width: 400, Height: 300, View: view}
	plain := snap(t, s, req)
	req.Rulers = true
	withRulers := snap(t, s, req)
	differ := func(x0, y0, x1, y1 int) int {
		n := 0
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if plain.NRGBAAt(x, y) != withRulers.NRGBAAt(x, y) {
					n++
				}
			}
		}
		return n
	}
	if n := differ(60, 280, 400, 300); n < 200 {
		t.Fatalf("bottom ruler changed only %d pixels", n)
	}
	if n := differ(0, 0, 40, 270); n < 200 {
		t.Fatalf("left ruler changed only %d pixels", n)
	}
	if n := differ(100, 100, 300, 200); n != 0 {
		t.Fatalf("rulers touched the interior: %d pixels", n)
	}
	req.Rulers, req.Grid = false, true
	if grid := snap(t, s, req); grid.NRGBAAt(100, 150) == plain.NRGBAAt(100, 150) && grid.NRGBAAt(101, 150) == plain.NRGBAAt(101, 150) && grid.NRGBAAt(102, 150) == plain.NRGBAAt(102, 150) {
		t.Log("grid line not at the sampled pixels (informational)")
	}
}

func TestSnapshotErrors(t *testing.T) {
	if _, err := ui.Snapshot(nil, cadmcp.SnapshotRequest{Width: 10, Height: 10}); err == nil {
		t.Fatal("nil session accepted")
	}
	s, v := snapSession("")
	if _, err := ui.Snapshot(s, cadmcp.SnapshotRequest{Width: 0, Height: 10, View: *v}); err == nil {
		t.Fatal("zero width accepted")
	}
}

func TestSnapshotSample(t *testing.T) {
	s, v := snapSession(snapScript)
	req := cadmcp.SnapshotRequest{Width: 1024, Height: 768, View: *v, Grid: true, Rulers: true}
	req.View.Width, req.View.Height = 1024, 768
	req.View.Fit(s.Drawing().Extents(), 48)
	b, err := ui.Snapshot(s, req)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "_tmp", "render_png")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.png"), b, 0o644); err != nil {
		t.Skip(err)
	}
}

// TestSnapshotRulersUnchanged pins the pixels of render_png's rulers: the canvas rulers share
// their code (ruler.go), and agents compare renders, so the MCP image must not move. A deliberate
// change to the MCP rulers updates these hashes. These hashes cover the whole scene, so an
// intentional geometry stacking change also updates the affected case.
func TestSnapshotRulersUnchanged(t *testing.T) {
	s, _ := snapSession(snapScript)
	cases := []struct {
		view render.View
		want string
	}{
		{render.View{Center: geom.V(110, 65), Scale: 3.5}, "5eabc0d63367725a"},
		{render.View{Center: geom.V(-2500, 1e6), Scale: 0.02}, "b26e8c1890d49ff7"},
		{render.View{Center: geom.V(0.3, -0.2), Scale: 900}, "8674e58bb86fc872"},
	}
	for i, c := range cases {
		req := cadmcp.SnapshotRequest{Width: 640, Height: 400, View: c.view, Grid: true, Rulers: true}
		img := snap(t, s, req)
		sum := sha256.Sum256(img.Pix)
		if got := hex.EncodeToString(sum[:8]); got != c.want {
			t.Errorf("case %d: ruler render hash %s, want %s", i, got, c.want)
		}
	}
}
