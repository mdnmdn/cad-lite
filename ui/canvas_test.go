package ui_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

const notch = 20 // one mouse-wheel notch in logical pixels

func (h *harness) wheel(e gpui.ScrollWheelEvent) {
	if e.Position == (gpui.Point{}) {
		e.Position = gpui.Point{X: 300, Y: 250}
	}
	h.hw.Dispatch(e)
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestWheelNotchZoomsAboutPointer(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	m := h.ws.View().ToModel(200, 150)
	p := h.at(m)
	s0 := v.Scale
	h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: -notch})
	if v.Scale <= s0 {
		t.Fatalf("notch up must zoom in: %v -> %v", s0, v.Scale)
	}
	if got := h.at(m); abs(got.X-p.X) > 0.01 || abs(got.Y-p.Y) > 0.01 {
		t.Fatalf("point under pointer moved %v -> %v", p, got)
	}
	s1 := v.Scale
	h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: notch})
	if !near(v.Scale, s0, 1e-9) || s1 <= s0 {
		t.Fatalf("notch down did not undo notch up: %v", v.Scale)
	}
}

func TestWheelPreciseScrollPans(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	c0, s0 := v.Center, v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaX: 30, DeltaY: 12, Precise: true})
	if v.Scale != s0 {
		t.Fatalf("precise scroll must not zoom: scale %v -> %v", s0, v.Scale)
	}
	// Scrolling down by 12 px moves content up, so the view centre moves down
	// in model space (y-up) and a positive DeltaX moves it right.
	want := c0.Add(geom.V(30/s0, -12/s0))
	if !v.Center.Near(want, 1e-9) {
		t.Fatalf("centre %v, want %v", v.Center, want)
	}
}

func TestWheelZoomSettingMakesPreciseScrollZoom(t *testing.T) {
	h := open(t)
	h.ws.WheelZoom = true
	v := h.ws.View()
	s0 := v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaY: -notch, Precise: true})
	if v.Scale <= s0 {
		t.Fatalf("WheelZoom: precise scroll up should zoom in, scale %v -> %v", s0, v.Scale)
	}
}

func TestWheelShiftPansNotch(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	c0, s0 := v.Center, v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaY: notch, Modifiers: gpui.Shift})
	if v.Scale != s0 || !v.Center.Near(c0.Add(geom.V(0, -notch/s0)), 1e-9) {
		t.Fatalf("shift+notch: centre %v scale %v", v.Center, v.Scale)
	}
}

func TestWheelHorizontalNotchPans(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	c0, s0 := v.Center, v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaX: notch})
	if v.Scale != s0 || !v.Center.Near(c0.Add(geom.V(notch/s0, 0)), 1e-9) {
		t.Fatalf("horizontal notch: centre %v scale %v", v.Center, v.Scale)
	}
}

func TestWheelCtrlZoomsSmoothlyAboutPointer(t *testing.T) {
	for _, mod := range []gpui.Modifiers{gpui.Control, gpui.ModPlatform} {
		h := open(t)
		v := h.ws.View()
		m := v.ToModel(200, 150)
		p := h.at(m)
		s0 := v.Scale
		h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: -5, Precise: true, Modifiers: mod})
		want := s0 * math.Exp(5.0/100)
		if !near(v.Scale, want, 1e-9) {
			t.Fatalf("mod %v: pinch scale %v, want %v", mod, v.Scale, want)
		}
		if got := h.at(m); abs(got.X-p.X) > 0.01 || abs(got.Y-p.Y) > 0.01 {
			t.Fatalf("mod %v: point under pointer moved %v -> %v", mod, p, got)
		}
		h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: 5, Precise: true, Modifiers: mod})
		if !near(v.Scale, s0, 1e-9) {
			t.Fatalf("mod %v: pinch out did not undo pinch in: %v", mod, v.Scale)
		}
	}
}

func TestZoomLimits(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	for i := 0; i < 400; i++ {
		h.wheel(gpui.ScrollWheelEvent{DeltaY: -notch * 5})
	}
	if v.Scale > 1e6 || v.Scale < 1e5 {
		t.Fatalf("zoom in must stop at MaxScale, got %v", v.Scale)
	}
	for i := 0; i < 800; i++ {
		h.wheel(gpui.ScrollWheelEvent{DeltaY: notch * 5})
	}
	if v.Scale < 1e-6 || v.Scale > 1e-5 {
		t.Fatalf("zoom out must stop at MinScale, got %v", v.Scale)
	}
}

func TestDoubleMiddleClickZoomsExtents(t *testing.T) {
	h := open(t)
	h.typ("line")
	h.key("enter")
	h.click(h.at(geom.V(0, 0)), gpui.MouseLeft)
	h.click(h.at(geom.V(100, 50)), gpui.MouseLeft)
	h.click(h.at(geom.V(0, 0)), gpui.MouseRight)
	v := h.ws.View()
	p := gpui.Point{X: 300, Y: 250}
	h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: -notch * 8})
	h.wheel(gpui.ScrollWheelEvent{DeltaX: 3000, Precise: true})
	box := h.ws.Session().Drawing().Extents()
	vis := v.Visible()
	if vis.Min.X <= box.Min.X && vis.Max.X >= box.Max.X && vis.Min.Y <= box.Min.Y && vis.Max.Y >= box.Max.Y {
		t.Fatal("setup: the drawing should be out of view before the double click")
	}
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 2})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseMiddle, ClickCount: 2})
	vis = v.Visible()
	if !(vis.Min.X <= box.Min.X && vis.Max.X >= box.Max.X && vis.Min.Y <= box.Min.Y && vis.Max.Y >= box.Max.Y) {
		t.Fatalf("double middle click did not frame %v: visible %v", box, vis)
	}
	if !v.Center.Near(box.Center(), 1e-6) {
		t.Fatalf("centre %v, want %v", v.Center, box.Center())
	}
	c := v.Center
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: p.X + 40, Y: p.Y}})
	if !v.Center.Near(c, 1e-9) {
		t.Fatal("the double click must not leave the view panning")
	}
}

func TestNoCrosshairUntilPointerKnown(t *testing.T) {
	h := open(t)
	if h.ws.PointerOnCanvas() {
		t.Fatal("no pointer position is known yet; the crosshair and coordinates must be hidden")
	}
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	if !h.ws.PointerOnCanvas() {
		t.Fatal("a pointer over the canvas shows the crosshair")
	}
}

func TestCrosshairHidesWhenPointerLeavesCanvas(t *testing.T) {
	h := open(t)
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 5}}) // toolbar
	if h.ws.PointerOnCanvas() {
		t.Fatal("pointer over the toolbar: crosshair must be hidden")
	}
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: -5, Y: -5}}) // outside the window
	if h.ws.PointerOnCanvas() {
		t.Fatal("pointer outside the window: crosshair must be hidden")
	}
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	if !h.ws.PointerOnCanvas() {
		t.Fatal("pointer back on the canvas shows the crosshair again")
	}
}

func boxIn(vis, box geom.Box2) bool {
	return vis.Min.X <= box.Min.X+1e-6 && vis.Max.X >= box.Max.X-1e-6 &&
		vis.Min.Y <= box.Min.Y+1e-6 && vis.Max.Y >= box.Max.Y-1e-6
}

func TestCanvasResizeKeepsVisibleBoxFramed(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	w0, s0, c0 := v.Width, v.Scale, v.Center
	box0 := v.Visible()
	h.key("f2") // the agent panel opens and the canvas narrows
	if v.Width >= w0 {
		t.Fatalf("setup: canvas did not narrow (%v -> %v)", w0, v.Width)
	}
	if !boxIn(v.Visible(), box0) {
		t.Fatalf("narrower canvas clips the old view: visible %v, was %v", v.Visible(), box0)
	}
	if !v.Center.Near(c0, 1e-9) || v.Scale >= s0 {
		t.Fatalf("centre %v (was %v), scale %v (was %v): want same centre, smaller scale", v.Center, c0, v.Scale, s0)
	}
	h.key("f2")
	if !near(v.Width, w0, 1e-9) || !near(v.Scale, s0, 1e-9) || !v.Center.Near(c0, 1e-9) {
		t.Fatalf("closing the panel must restore the view: scale %v centre %v", v.Scale, v.Center)
	}
}

func (h *harness) wheelMode(mode string) {
	st := h.ws.Settings()
	st.Wheel = mode
	h.ws.SetSettings(st, nil)
}

func TestWheelModeZoomMakesPreciseScrollZoom(t *testing.T) {
	h := open(t)
	h.wheelMode(settings.WheelZoom)
	v := h.ws.View()
	s0 := v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaY: -notch, Precise: true})
	if v.Scale <= s0 {
		t.Fatalf("zoom mode: precise scroll up should zoom in, scale %v -> %v", s0, v.Scale)
	}
	h.wheel(gpui.ScrollWheelEvent{DeltaY: notch, Modifiers: gpui.Shift})
	if !near(v.Scale, s0*math.Exp(notch/240.0), 1e-9) {
		t.Fatalf("zoom mode: shift must pan, scale %v", v.Scale)
	}
}

func TestWheelModePanPansAndCtrlZooms(t *testing.T) {
	h := open(t)
	h.wheelMode(settings.WheelPan)
	v := h.ws.View()
	c0, s0 := v.Center, v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaY: notch})
	if v.Scale != s0 || !v.Center.Near(c0.Add(geom.V(0, -notch/s0)), 1e-9) {
		t.Fatalf("pan mode notch: centre %v scale %v", v.Center, v.Scale)
	}
	h.wheel(gpui.ScrollWheelEvent{DeltaX: 8, DeltaY: 4, Precise: true})
	if v.Scale != s0 {
		t.Fatalf("pan mode precise: scale %v", v.Scale)
	}
	h.wheel(gpui.ScrollWheelEvent{DeltaY: -notch, Modifiers: gpui.Control})
	if v.Scale <= s0 {
		t.Fatalf("pan mode ctrl+notch should zoom in: %v -> %v", s0, v.Scale)
	}
}

func TestWheelModeAutoKeepsHeuristics(t *testing.T) {
	h := open(t)
	h.wheelMode(settings.WheelAuto)
	v := h.ws.View()
	s0 := v.Scale
	h.wheel(gpui.ScrollWheelEvent{DeltaY: -notch})
	if v.Scale <= s0 {
		t.Fatalf("auto: notch should zoom, %v -> %v", s0, v.Scale)
	}
}

func TestPinchZoomsAboutPointer(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	m := v.ToModel(200, 150)
	p := h.at(m)
	for _, d := range []float32{0.25, -0.2} {
		s0 := v.Scale
		h.hw.Dispatch(gpui.PinchEvent{Position: p, Delta: d})
		if !near(v.Scale/s0, 1+float64(d), 1e-6) {
			t.Fatalf("pinch %v: scale %v -> %v", d, s0, v.Scale)
		}
		if got := h.at(m); abs(got.X-p.X) > 0.01 || abs(got.Y-p.Y) > 0.01 {
			t.Fatalf("point under pointer moved %v -> %v", p, got)
		}
	}
}

// strokeBetween reports whether the frame holds a two-point stroke from a to b (either way),
// within a pixel.
func strokeBetween(h *harness, a, b gpui.Point) bool {
	close := func(p, q gpui.Point) bool { return abs(p.X-q.X) <= 1 && abs(p.Y-q.Y) <= 1 }
	for _, s := range h.hw.Window().Scene().Strokes {
		if len(s.Points) == 2 && (close(s.Points[0], a) && close(s.Points[1], b) || close(s.Points[0], b) && close(s.Points[1], a)) {
			return true
		}
	}
	return false
}

// The segments LINE placed draw while LINE still runs, as in AutoCAD, and the snap marker shows
// on them.
func TestCanvasDrawsSegmentsOfTheRunningLine(t *testing.T) {
	h := open(t)
	s := h.ws.Session()
	s.Script("LINE 3,7 43,7 43,27")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: h.at(geom.V(13, 37))})
	if s.Active() != "LINE" {
		t.Fatalf("LINE ended: %q", s.Active())
	}
	for _, seg := range [][2]geom.Vec2{{geom.V(3, 7), geom.V(43, 7)}, {geom.V(43, 7), geom.V(43, 27)}} {
		if !strokeBetween(h, h.at(seg[0]), h.at(seg[1])) {
			t.Fatalf("segment %v-%v is not drawn mid-command", seg[0], seg[1])
		}
	}
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: h.at(geom.V(23.3, 7.2))})
	if hit, ok := s.Snap(); !ok || hit.Point != geom.V(23, 7) {
		t.Fatalf("no midpoint snap on the placed segment: %+v %v", hit, ok)
	}
	s.Submit("U")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: h.at(geom.V(13, 37))})
	if strokeBetween(h, h.at(geom.V(43, 7)), h.at(geom.V(43, 27))) {
		t.Fatal("the undone segment is still drawn")
	}
	if !strokeBetween(h, h.at(geom.V(3, 7)), h.at(geom.V(43, 7))) {
		t.Fatal("Undo took the first segment off the canvas too")
	}
}

// pickbox reports whether the frame holds the closed square pickbox centred on p.
func pickbox(h *harness, p gpui.Point) bool {
	for _, s := range h.hw.Window().Scene().Strokes {
		// An axis-aligned square: the quadrant marker is a diamond.
		if !s.Closed || len(s.Points) != 4 || abs(s.Points[0].Y-s.Points[1].Y) > 0.01 {
			continue
		}
		c := gpui.Point{X: (s.Points[0].X + s.Points[2].X) / 2, Y: (s.Points[0].Y + s.Points[2].Y) / 2}
		if abs(c.X-p.X) <= 1 && abs(c.Y-p.Y) <= 1 {
			return true
		}
	}
	return false
}

// AREA's "Select object:" picks an object, so the cursor shows AutoCAD's pickbox there and no
// snap marker, while its corner prompts show the crosshair alone and snap.
func TestCanvasPickboxAtObjectPickPrompts(t *testing.T) {
	h := open(t)
	s := h.ws.Session()
	s.Script("CIRCLE 0,0 40")
	s.Script("AREA O")
	p := h.at(geom.V(40.5, 0.5))
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	if _, ok := s.Snap(); ok || !pickbox(h, p) {
		t.Fatalf("Select object: snapped %v, pickbox %v", ok, pickbox(h, p))
	}
	s.Escape()
	s.Script("AREA")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	if hit, ok := s.Snap(); !ok || hit.Point != geom.V(40, 0) || pickbox(h, h.at(hit.Point)) {
		t.Fatalf("first corner prompt: snap %+v %v", hit, ok)
	}
}

// A click on the rulers is a click on the drawing under them: the rulers change no coordinate,
// and the wheel still zooms about the pointer there.
func TestRulersKeepClicksAndZoomOnTheModel(t *testing.T) {
	h := open(t)
	s := h.ws.Session()
	if !h.ws.Rulers() {
		t.Fatal("rulers are off")
	}
	h.typ("line")
	h.key("enter")
	v := h.ws.View()
	// A pixel 10 px into the top ruler and one 10 px into the left ruler.
	for _, px := range [][2]float64{{v.Width / 2, 10}, {10, v.Height / 2}} {
		m := v.ToModel(px[0], px[1])
		p := h.at(m)
		h.click(p, gpui.MouseLeft)
		got := s.History()[len(s.History())-1]
		if want := cmdsys.FormatPoint(m); !strings.HasSuffix(got, want) {
			t.Fatalf("click on the ruler answered %q, want %s", got, want)
		}
		h.wheel(gpui.ScrollWheelEvent{Position: p, DeltaY: -notch})
		if got := h.at(m); abs(got.X-p.X) > 0.01 || abs(got.Y-p.Y) > 0.01 {
			t.Fatalf("zoom on the ruler moved the point under the pointer: %v -> %v", p, got)
		}
	}
}
