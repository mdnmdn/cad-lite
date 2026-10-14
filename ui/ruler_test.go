package ui

import (
	"math"
	"strconv"
	"testing"

	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// The labelled step is the smallest 1, 2 or 5 times a power of ten at least 80 px (horizontal)
// or 48 px (vertical) apart; the minor step splits it into fifths, or quarters for a 2.
func TestRulerStepsFollowZoom(t *testing.T) {
	cases := []struct {
		scale          float64
		sx, sy, mx, my float64
	}{
		{1, 100, 50, 20, 10},
		{3.05, 50, 20, 10, 5},
		{4, 20, 20, 5, 5},
		{0.005, 20000, 10000, 5000, 2000},
		{660, 0.2, 0.1, 0.05, 0.02},
	}
	for _, c := range cases {
		sx, sy := rulerSteps(render.View{Scale: c.scale})
		mx, my := minorStep(sx), minorStep(sy)
		for _, p := range [][2]float64{{sx, c.sx}, {sy, c.sy}, {mx, c.mx}, {my, c.my}} {
			if math.Abs(p[0]-p[1]) > 1e-9*p[1] {
				t.Fatalf("scale %v: steps %v %v minor %v %v, want %v %v %v %v", c.scale, sx, sy, mx, my, c.sx, c.sy, c.mx, c.my)
			}
		}
		if sx*c.scale < 80 || sy*c.scale < 48 {
			t.Fatalf("scale %v: labelled ticks closer than their minimum", c.scale)
		}
	}
}

// Ticks fall on multiples of the step, and a label is the tick's value.
func TestRulerTicksOnMultiples(t *testing.T) {
	v := render.View{Center: geom.V(1234.5, -77), Scale: 2.7, Width: 900, Height: 500}
	xs, ys := rulerTicks(v, true)
	sx, sy := rulerSteps(v)
	if len(xs) < 5 || len(ys) < 5 {
		t.Fatalf("ticks %d x %d", len(xs), len(ys))
	}
	for _, tk := range xs {
		if r := math.Mod(math.Abs(tk.value), sx); r > 1e-9 && sx-r > 1e-9 {
			t.Fatalf("x tick %v off the %v step", tk.value, sx)
		}
		if px, _ := v.ToScreen(geom.V(tk.value, 0)); math.Abs(px-tk.px) > 1e-9 {
			t.Fatalf("x tick %v at %v px, want %v", tk.value, tk.px, px)
		}
	}
	for _, tk := range ys {
		if r := math.Mod(math.Abs(tk.value), sy); r > 1e-9 && sy-r > 1e-9 {
			t.Fatalf("y tick %v off the %v step", tk.value, sy)
		}
	}
}

// Rulers overlay the canvas: turning them off changes no coordinate, and the viewport label and
// zoom buttons sit below the top ruler and right of the left one while they show.
func TestRulersMoveOverlaysNotTheView(t *testing.T) {
	h := dialogRig(t, nil)
	if !h.ws.Rulers() {
		t.Fatal("rulers are not on by default")
	}
	h.hw.Pixels()
	v := *h.ws.View()
	p := h.ws.ScreenPoint(geom.V(10, 20))
	c := h.ws.canvas.bounds.Origin
	left, top := h.ws.rulerInsets()
	if top != rulerBand || left < 20 {
		t.Fatalf("insets %v, %v", left, top)
	}
	lbl, ok := h.locate("viewport-label")
	zoom, ok2 := h.locate("canvas-zoom")
	if !ok || !ok2 {
		t.Fatal("viewport label or zoom buttons not found")
	}
	f := h.hw.Window().UIScale()
	if float32(lbl.Y)/f-float32(c.Y) < top+8 || float32(lbl.X)/f-float32(c.X) < left || float32(zoom.Y)/f-float32(c.Y) < top+30 {
		t.Fatalf("overlays under the rulers: label %v, zoom %v, canvas at %v", lbl, zoom, c)
	}
	h.ws.toggleRulers(h.hw.Window())
	h.hw.Pixels()
	if h.ws.Rulers() || *h.ws.View() != v || h.ws.ScreenPoint(geom.V(10, 20)) != p {
		t.Fatalf("turning the rulers off moved the view: %+v -> %+v", v, *h.ws.View())
	}
	if lbl2, _ := h.locate("viewport-label"); lbl2.Y >= lbl.Y || lbl2.X >= lbl.X {
		t.Fatalf("label did not move back to the corner: %v -> %v", lbl, lbl2)
	}
}

// BenchmarkCanvasFrameRulers measures one canvas frame (pointer move, layout, paint and raster)
// with the rulers on and off.
func BenchmarkCanvasFrameRulers(b *testing.B) {
	for _, on := range []bool{false, true} {
		name := "off"
		if on {
			name = "on"
		}
		b.Run(name, func(b *testing.B) {
			h := dialogRig(&testing.T{}, nil)
			h.ws.Session().Script("RECTANG 0,0 220,130 CIRCLE 110,50 20 LINE 0,0 220,130 ")
			h.ws.SetRulers(on)
			h.ws.FitOnFirstFrame()
			h.hw.Pixels()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p := h.ws.ScreenPoint(geom.V(float64(50+i%100), 60))
				h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
				h.hw.Pixels()
			}
		})
	}
}

// At the deepest zooms the vertical step is 5e-5: the canvas labels carry the decimals the step
// needs, so no two read the same and each reads its value.
func TestRulerLabelsKeepTheirDecimals(t *testing.T) {
	for _, scale := range []float64{960000, 1e6, 900, 3.5, 0.02} {
		v := render.View{Center: geom.V(12.34567, -0.00321), Scale: scale, Width: 900, Height: 500}
		xs, ys := rulerTicks(v, true)
		for _, set := range [][]tick{xs, ys} {
			seen := map[string]bool{}
			for _, tk := range set {
				l := tk.label()
				if seen[l] {
					t.Fatalf("scale %v: label %q repeats", scale, l)
				}
				seen[l] = true
				got, err := strconv.ParseFloat(l, 64)
				if err != nil || math.Abs(got-tk.value) > 1e-9*max(1, math.Abs(tk.value)) {
					t.Fatalf("scale %v: label %q for %v", scale, l, tk.value)
				}
			}
		}
	}
	if got := rulerNumber(-0.0000001, 2); got != "0" {
		t.Errorf("rulerNumber gave %q for a negative zero", got)
	}
}

// Panning by less than a change of digits keeps the left ruler's width, so the overlays inside it
// stay put.
func TestRulerLeftBandHoldsWhilePanning(t *testing.T) {
	var lb float64
	for i, y := range []float64{200, 263.7, 401.25, 555, 700} {
		v := render.View{Center: geom.V(0, y), Scale: 1, Width: 900, Height: 500}
		f := canvasRulers().frame(v)
		if i > 0 && f.lb != lb {
			t.Fatalf("pan to y=%v: left ruler %v px, was %v", y, f.lb, lb)
		}
		lb = f.lb
	}
	// It still grows when the labels need more digits.
	far := canvasRulers().frame(render.View{Center: geom.V(0, 1e7), Scale: 1, Width: 900, Height: 500})
	if far.lb <= lb {
		t.Fatalf("left ruler at 1e7 is %v px, not wider than %v", far.lb, lb)
	}
}

// The canvas works its ruler layout out once per view, however many overlays ask for it.
func TestRulerFrameComputedOncePerView(t *testing.T) {
	h := dialogRig(t, nil)
	h.hw.Pixels()
	a, b := h.ws.rulerFrame(), h.ws.rulerFrame()
	if len(a.xs) == 0 || &a.xs[0] != &b.xs[0] {
		t.Fatal("the ruler layout was worked out twice for one view")
	}
	h.ws.View().Scale *= 2
	c := h.ws.rulerFrame()
	if len(c.xs) == 0 || &c.xs[0] == &a.xs[0] {
		t.Fatal("the ruler layout did not follow a zoom")
	}
}
