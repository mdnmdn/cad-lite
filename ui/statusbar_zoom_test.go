package ui

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

func openZoom(t *testing.T) (*Workspace, *gpui.HeadlessWindow) {
	t.Helper()
	ws := NewWorkspace(doc.New(), nil)
	p := gpui.NewHeadlessPlatform()
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 1200, Height: 700}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	return ws, hw
}

func TestZoomSliderMappingRoundTrips(t *testing.T) {
	for _, s := range []float64{1e-3, 0.05, 1, 7.5, 1e3} {
		if got := sliderToZoom(zoomToSlider(s)); math.Abs(got/s-1) > 1e-9 {
			t.Errorf("scale %v round-trips to %v", s, got)
		}
	}
	if zoomToSlider(1e-9) != zoomSliderMinExp || zoomToSlider(1e9) != zoomSliderMaxExp {
		t.Error("out-of-range scales must pin the thumb to the ends")
	}
	for s, want := range map[float64]string{1: "100%", 5.09: "509%", 0.25: "25.0%", 0.001: "0.1%"} {
		if got := zoomPercent(s); got != want {
			t.Errorf("zoomPercent(%v) = %q, want %q", s, got, want)
		}
	}
}

func TestZoomSliderDragZoomsAboutCentreAndFollowsWheel(t *testing.T) {
	ws, hw := openZoom(t)
	v := ws.View()
	centre := v.Center
	tr := ws.zoom.track
	if tr.Size.W == 0 {
		t.Fatal("slider not painted")
	}
	at := func(frac float64) gpui.Point {
		x0 := float64(tr.Origin.X) + sliderThumb/2
		span := float64(tr.Size.W) - sliderThumb
		return gpui.Point{X: gpui.Pixels(x0 + frac*span), Y: tr.Origin.Y + tr.Size.H/2}
	}
	s0 := v.Scale
	hw.Dispatch(gpui.MouseMoveEvent{Position: at(0.5)})
	hw.Dispatch(gpui.MouseDownEvent{Position: at(0.5), Button: gpui.MouseLeft, ClickCount: 1})
	if !near(v.Scale, 1, 1e-9) {
		t.Fatalf("middle of the slider is scale 1, got %v", v.Scale)
	}
	hw.Dispatch(gpui.MouseMoveEvent{Position: at(0.75)})
	if !near(math.Log10(v.Scale), 1.5, 1e-6) {
		t.Fatalf("drag to 75%% must set 10^1.5, got %v", v.Scale)
	}
	if !v.Center.Near(centre, 1e-9) {
		t.Fatalf("slider zoom moved the centre %v -> %v", centre, v.Center)
	}
	hw.Dispatch(gpui.MouseUpEvent{Position: at(0.75), Button: gpui.MouseLeft, ClickCount: 1})
	hw.Pixels()
	if ws.zoom.Dragging() {
		t.Fatal("drag must end on mouse up")
	}
	// One step on the ZOOM Previous stack for the whole drag (the click at
	// the middle and the move share it), holding the scale before it.
	if n := v.History().Len(); n != 1 {
		t.Fatalf("drag recorded %d previous views, want 1", n)
	}
	if prev, ok := v.History().Pop(*v); !ok || !near(prev.Scale, s0, 1e-9) {
		t.Fatalf("previous view %v, want scale %v", prev.Scale, s0)
	}
	// Wheel zoom moves the thumb.
	ws.view.Scale = 1.5
	hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: 300, Y: 250}, DeltaY: -20})
	hw.Pixels()
	if want := zoomToSlider(v.Scale); !near(ws.zoom.value, want, 1e-9) {
		t.Fatalf("slider at %v after wheel zoom, want %v", ws.zoom.value, want)
	}
}

func TestZoomStepButtons(t *testing.T) {
	ws, hw := openZoom(t)
	v := ws.View()
	s0 := v.Scale
	ws.setZoom(v.Scale*zoomButtonFactor, false)
	hw.Pixels()
	if !near(v.Scale, s0*zoomButtonFactor, 1e-9) || v.Scale > render.MaxScale {
		t.Fatalf("scale %v", v.Scale)
	}
	if v.History().Len() != 1 {
		t.Fatalf("button step must be one previous view, got %d", v.History().Len())
	}
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }
