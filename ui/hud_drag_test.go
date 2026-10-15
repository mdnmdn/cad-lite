package ui

import (
	"fmt"
	"math"
	"os"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

func showDragHUD(t *testing.T) *sHarness {
	t.Helper()
	h := hudRig(t)
	h.ws.togglePropsHUD()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.hw.Pixels()
	return h
}

func dragHUDTo(h *sHarness, x, y float32) {
	h.t.Helper()
	start := h.find("props-hud-handle")
	b := h.ws.hudBox(h.ws.hudPosition())
	to := gpui.Point{X: start.X + gpui.Pixels(x-b.x), Y: start.Y + gpui.Pixels(y-b.y)}
	h.hw.Dispatch(gpui.MouseDownEvent{Position: start, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Pixels()
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: to})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: to, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Pixels()
	h.hw.Pixels()
}

func TestPropertiesHUDDragFreeAndCornerSnap(t *testing.T) {
	h := showDragHUD(t)
	p := h.ws.hudPosition()
	a := h.ws.hudArea()
	dragHUDTo(h, a.x+a.w/2-80, a.y+a.h/2)
	if !p.free || h.ws.propsDragActive() || h.ws.canvas.pressed || h.ws.canvas.panning || h.ws.session.Selection().Len() != 0 {
		t.Fatal("free HUD drag changed canvas state or did not end")
	}
	for corner := 0; corner < 4; corner++ {
		want := *p
		want.free, want.corner = false, corner
		b := h.ws.hudBox(&want)
		dragHUDTo(h, b.x+3, b.y+3)
		if p.free || p.corner != corner {
			t.Fatalf("corner %d did not snap: %+v", corner, *p)
		}
		got := h.ws.hudBox(p)
		if math.Abs(float64(got.x-b.x)) > 1 || math.Abs(float64(got.y-b.y)) > 1 {
			t.Fatalf("corner %d moved: %+v want %+v", corner, got, b)
		}
	}
}

func TestPropertiesHUDReleaseOverChromeAndModalHandoff(t *testing.T) {
	for _, over := range []string{"canvas-zoom-in", "layer-0", "tool-open", "textedit", "hide", "tab"} {
		t.Run(over, func(t *testing.T) {
			h := showDragHUD(t)
			start := h.find("props-hud-handle")
			var to gpui.Point
			if over != "textedit" && over != "hide" && over != "tab" {
				to = h.find(over)
			} else {
				to = gpui.Point{X: 500, Y: 400}
			}
			h.hw.Dispatch(gpui.MouseDownEvent{Position: start, Button: gpui.MouseLeft, ClickCount: 1})
			h.hw.Pixels()
			if !h.ws.propsDragActive() {
				t.Fatal("header did not begin drag")
			}
			scale := h.ws.view.Scale
			switch over {
			case "textedit":
				tx, _ := h.ws.session.Drawing().Begin()
				target := tx.Add(doc.NewText(geom.V(0, 0), 10, 0, "test"))
				tx.Commit()
				if !h.ws.openTextEditor(h.ws.docTab, target) {
					t.Fatal("editor did not open")
				}
			case "hide":
				h.ws.togglePropsHUD()
				h.hw.Window().Refresh()
			case "tab":
				h.ws.activate(h.ws.newDocTab(doc.New()))
			}
			h.hw.Pixels()
			h.hw.Dispatch(gpui.MouseMoveEvent{Position: to})
			h.hw.Dispatch(gpui.MouseUpEvent{Position: to, Button: gpui.MouseLeft, ClickCount: 1})
			h.hw.Pixels()
			if h.ws.propsDragActive() || h.frameIDs()["props-hud-drag-shield"] {
				t.Fatal("HUD drag or shield survived release/lifecycle change")
			}
			if over != "tab" && h.ws.view.Scale != scale {
				t.Fatal("HUD drag zoomed the drawing")
			}
			if h.ws.session.Selection().Len() != 0 || h.ws.canvas.pressed {
				t.Fatal("HUD drag selected objects")
			}
		})
	}
}

func TestPropertiesHUDReclampsAndKeepsPerDrawingPosition(t *testing.T) {
	h := showDragHUD(t)
	first := h.ws.docTab
	p := h.ws.hudPosition()
	p.corner = 3
	h.ws.activate(h.ws.newDocTab(doc.New()))
	if h.ws.hudPosition().corner != 0 {
		t.Fatal("new drawing inherited another HUD position")
	}
	h.ws.activate(first)
	if h.ws.hudPosition() != p || p.corner != 3 {
		t.Fatal("drawing lost its HUD position")
	}
	for _, rulers := range []bool{false, true} {
		h.ws.SetRulers(rulers)
		for _, size := range []gpui.Bounds{gpui.NewBounds(0, 0, 260, 300), gpui.NewBounds(10, 20, 500, 400)} {
			h.ws.canvas.bounds = size
			p.free, p.x, p.y, p.height = true, 2000, 2000, 180
			a, b := h.ws.hudArea(), h.ws.hudBox(p)
			if b.x < a.x || b.y < a.y || b.x+b.w > a.x+a.w+0.1 || b.y+b.h > a.y+a.h+0.1 {
				t.Fatalf("HUD out of bounds: %+v area %+v", b, a)
			}
		}
	}
}

func TestRenderCompactHUDCorners(t *testing.T) {
	if err := os.MkdirAll("../_tmp/n24", 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light"} {
		h := showDragHUD(t)
		s := h.ws.Settings()
		s.Theme = theme
		h.ws.SetSettings(s, h.hw.Window())
		h.selectAll()
		for corner := 0; corner < 4; corner++ {
			h.ws.hudPosition().corner = corner
			h.hw.Window().Refresh()
			h.hw.Pixels()
			h.hw.Pixels()
			if err := h.hw.SavePNG(fmt.Sprintf("../_tmp/n24/%s-corner-%d.png", theme, corner)); err != nil {
				t.Fatal(err)
			}
		}
	}
	d := doc.New()
	h := openSettingsWS(t, NewWorkspace(d, nil), 640, 600, 1)
	h.ws.togglePropsHUD()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.hw.Pixels()
	if err := h.hw.SavePNG("../_tmp/n24/narrow.png"); err != nil {
		t.Fatal(err)
	}
}

func TestCompactZoomHUDControlsAndTranslucentFills(t *testing.T) {
	h := showDragHUD(t)
	before := h.ws.view.Scale
	h.clickID("canvas-zoom-in")
	h.hw.Pixels()
	if math.Abs(h.ws.view.Scale-before*zoomButtonFactor) > 1e-9 {
		t.Fatal("compact zoom-in changed its behavior")
	}
	h.clickID("canvas-zoom-out")
	h.hw.Pixels()
	if math.Abs(h.ws.view.Scale-before) > 1e-9 {
		t.Fatal("compact zoom-out changed its behavior")
	}
	h.clickID("canvas-zoom-fit")
	h.hw.Pixels()
	if h.ws.canvas.pressed || h.ws.session.Selection().Len() != 0 {
		t.Fatal("zoom controls reached canvas selection")
	}
	fills := 0
	for _, q := range h.hw.Window().Scene().Quads {
		if q.Fill.R == uiPanel.R && q.Fill.G == uiPanel.G && q.Fill.B == uiPanel.B && q.Fill.A == hudAlpha {
			fills++
		}
	}
	if fills < 2 {
		t.Fatalf("expected translucent fills for both HUDs, found %d", fills)
	}
	if hudWidth >= 220 || zoomHUDWidth >= 48 || zoomHUDHeight >= 132 {
		t.Fatal("HUD footprint did not shrink")
	}
}

func TestPropertiesHUDOverflowScrollDoesNotZoomCanvas(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	var hs []doc.Handle
	for _, e := range []doc.Entity{&doc.Line{}, &doc.Circle{Radius: 1}, &doc.Arc{Radius: 1}, &doc.LWPolyline{}, doc.NewText(geom.V(0, 0), 10, 0, "Text"), doc.NewMText(geom.V(0, 0), 10, 20, "Multi")} {
		hs = append(hs, tx.Add(e))
	}
	tx.Commit()
	h := openSettingsWS(t, NewWorkspace(d, nil), 640, 600, 1)
	h.ws.session.Selection().Add(hs...)
	h.ws.togglePropsHUD()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.hw.Pixels()
	p := h.ws.hudPosition()
	if p.scroll.MaxOffset().H <= 0 {
		t.Fatal("fixture did not overflow HUD body")
	}
	scale := h.ws.view.Scale
	b := h.ws.hudBox(p)
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: gpui.Pixels(b.x + b.w/2), Y: gpui.Pixels(b.y + b.h - 10)}, DeltaY: 30})
	h.hw.Pixels()
	if p.scroll.Offset().Y <= 0 || h.ws.view.Scale != scale {
		t.Fatal("HUD wheel must scroll overflowing properties without zooming canvas")
	}
	if err := h.hw.SavePNG("../_tmp/n24/narrow-overflow.png"); err != nil {
		t.Fatal(err)
	}
}

func TestPropertiesHUDHidesWithoutUsableCanvas(t *testing.T) {
	for _, size := range [][2]float32{{640, 400}, {800, 400}, {1280, 400}, {640, 500}, {320, 800}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := openSettingsWS(t, NewWorkspace(doc.New(), nil), size[0], size[1], 1)
			h.ws.togglePropsHUD()
			h.hw.Window().Refresh()
			for i := 0; i < 3; i++ {
				h.hw.Pixels()
			}
			ids := h.frameIDs()
			if ids["props-hud"] || ids["props-hud-handle"] || ids["props-hud-drag-shield"] {
				t.Fatal("HUD retains a hitbox when its header cannot fit")
			}
			for _, q := range h.hw.Window().Scene().Quads {
				if q.Fill.A == hudAlpha && float32(q.Bounds.Size.W) > zoomHUDWidth+1 {
					t.Fatal("Properties background painted without usable canvas space")
				}
			}
			if !h.ws.PropsHUDShown() {
				t.Fatal("temporary lack of space changed the PROPS toggle")
			}
		})
	}
}

func TestPropertiesHUDSpaceRecoveryCancelsDragAndReappears(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	ws.lay.ribbonOff = true
	h := openSettingsWS(t, ws, 1280, 400, 1)
	ws.togglePropsHUD()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.hw.Pixels()
	if !h.frameIDs()["props-hud-handle"] {
		t.Fatal("fixture needs a visible HUD before shrinking canvas")
	}
	p := ws.hudPosition()
	start := h.find("props-hud-handle")
	h.hw.Dispatch(gpui.MouseDownEvent{Position: start, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Pixels()
	if !ws.propsDragActive() {
		t.Fatal("fixture did not begin HUD drag")
	}
	ws.lay.ribbonOff = false
	h.hw.Window().Refresh()
	for i := 0; i < 3; i++ {
		h.hw.Pixels()
	}
	if ws.propsDragActive() || h.frameIDs()["props-hud-handle"] || h.frameIDs()["props-hud-drag-shield"] {
		t.Fatal("shrinking canvas did not hide HUD and cancel drag")
	}
	ws.lay.ribbonOff = true
	h.hw.Window().Refresh()
	for i := 0; i < 3; i++ {
		h.hw.Pixels()
	}
	if !h.frameIDs()["props-hud-handle"] || !ws.PropsHUDShown() || ws.hudPosition() != p || ws.propsDragActive() {
		t.Fatal("HUD did not recover its session position without resuming drag")
	}
}
