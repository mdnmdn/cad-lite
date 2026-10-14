package ui

import (
	"fmt"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

// hudRig opens a workspace with two lines on layer Walls, zoomed to them.
func hudRig(t *testing.T) *sHarness {
	t.Helper()
	d := doc.New()
	d.AddLayer("Walls", 1)
	d.CurrentLayer = "Walls"
	h := openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, 1)
	h.ws.Session().Script("LINE 0,0 100,0  LINE 0,50 100,50  ZOOM E")
	h.ws.session.Selection().Clear()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	return h
}

// selectAll selects every object, as a window around the drawing would.
func (h *sHarness) selectAll() {
	h.ws.session.SelectBox(geom.Vec2{X: -1000, Y: -1000}, geom.Vec2{X: 1000, Y: 1000}, false)
	h.hw.Window().Refresh()
	h.hw.Pixels()
}

func TestPropsHUDTogglesAndFollowsSelection(t *testing.T) {
	h := hudRig(t)
	if h.ws.PropsHUDShown() || h.inFrame("props-hud") {
		t.Fatal("the HUD must be off by default")
	}
	h.clickID("status-props")
	h.hw.Pixels()
	if !h.ws.PropsHUDShown() {
		t.Fatal("PROPS did not turn the HUD on")
	}
	hud := h.find("props-hud")
	_, empty, ok := h.spanAt("props-hud", float32(hud.X))
	if !ok {
		t.Fatal("no HUD on screen")
	}
	// It sits inside the canvas, left of the zoom buttons.
	cb := h.ws.canvas.bounds
	zoom := h.find("canvas-zoom-in")
	if float32(hud.X) >= float32(zoom.X) || float32(hud.Y) < float32(cb.Origin.Y) || float32(hud.X) < float32(cb.Origin.X) {
		t.Errorf("HUD at %v: canvas %v, zoom buttons at %v", hud, cb, zoom)
	}
	h.selectAll()
	if h.ws.session.Selection().Len() != 2 {
		t.Fatalf("selected %d", h.ws.session.Selection().Len())
	}
	y0, full, _ := h.spanAt("props-hud", float32(hud.X))
	if full-y0 <= empty-y0 {
		t.Errorf("the HUD did not grow for a selection: %v -> %v", empty, full)
	}
	h.png("hud-on.png")
	h.clickID("status-props")
	h.hw.Pixels()
	if h.ws.PropsHUDShown() || h.inFrame("props-hud") {
		t.Fatal("PROPS did not turn the HUD off")
	}
}

func TestPropsHUDIsClickThroughExceptItself(t *testing.T) {
	h := hudRig(t)
	h.ws.togglePropsHUD()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	cb := h.ws.canvas.bounds
	// A line through the HUD's middle: a click on the HUD over it picks nothing.
	hud := h.find("props-hud")
	m := h.ws.view.ToModel(float64(hud.X-cb.Origin.X), float64(hud.Y-cb.Origin.Y))
	h.ws.Session().Script(fmt.Sprintf("LINE %g,%g %g,%g ", m.X-500, m.Y, m.X+500, m.Y))
	h.ws.session.Selection().Clear()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.click(hud)
	h.hw.Pixels()
	if n := h.ws.session.Selection().Len(); n != 0 {
		t.Fatalf("a click on the HUD reached the canvas: %d selected", n)
	}
	if _, ok := h.locate("props-hud"); !ok {
		t.Fatal("the HUD went away")
	}
	// Beside the HUD the canvas takes the click: the same line is picked.
	h.click(gpui.Point{X: hud.X - 300, Y: hud.Y})
	h.hw.Pixels()
	if n := h.ws.session.Selection().Len(); n != 1 {
		t.Fatalf("a pick beside the HUD selected %d", n)
	}
}

// Renders the layer popover and the HUD with a selection, dark and light, into _tmp/settings.
func TestRenderLayerPopoverAndHUD(t *testing.T) {
	for _, th := range []string{"dark", "light"} {
		h := hudRig(t)
		s := h.ws.Settings()
		s.Theme = th
		h.ws.SetSettings(s, h.hw.Window())
		for _, n := range []string{"Doors", "Windows", "wall-hatch", "Furniture"} {
			h.ws.session.Drawing().AddLayer(n, 3)
		}
		h.ws.session.Drawing().Layer("Doors").Off = true
		h.ws.session.Drawing().Layer("Windows").Locked = true
		h.ws.togglePropsHUD()
		h.selectAll()
		h.png("hud-" + th + ".png")
		h.openLayerPop()
		h.png("layerpop-" + th + ".png")
		h.typeText("wa")
		h.png("layerpop-filter-" + th + ".png")
	}
}

func TestRulerChipTogglesRulers(t *testing.T) {
	h := hudRig(t)
	on := h.ws.Rulers()
	if !h.hoverTip(h.find("status-ruler")) {
		t.Error("RULER: no tooltip after 600 ms")
	}
	h.clickID("status-ruler")
	h.hw.Pixels()
	if h.ws.Rulers() == on {
		t.Fatal("RULER did not toggle the rulers")
	}
	h.clickID("status-ruler")
	h.hw.Pixels()
	if h.ws.Rulers() != on {
		t.Fatal("a second click on RULER did not toggle them back")
	}
}

// The HUD's top edge is overlayInset below the top ruler when the rulers show, and below the
// canvas's top edge when they do not, level with the zoom buttons either way.
func TestPropsHUDClearsTheRuler(t *testing.T) {
	h := hudRig(t)
	h.ws.togglePropsHUD()
	for _, rulers := range []bool{true, false} {
		h.ws.SetRulers(rulers)
		h.hw.Window().Refresh()
		h.hw.Pixels()
		h.hw.Pixels() // the HUD follows the canvas a frame later
		hud := h.find("props-hud")
		top, _, ok := h.spanAt("props-hud", float32(hud.X))
		zoomTop, _, zok := h.spanAt("canvas-zoom", float32(h.find("canvas-zoom-in").X))
		if !ok || !zok {
			t.Fatal("HUD or zoom buttons not on screen")
		}
		_, ruler := h.ws.rulerInsets()
		want := float32(h.ws.canvas.bounds.Origin.Y) + ruler + overlayInset
		if top < want-1 || top > want+1 || top != zoomTop {
			t.Errorf("rulers %v: HUD top %v, want %v (zoom buttons at %v)", rulers, top, want, zoomTop)
		}
		if rulers && ruler == 0 {
			t.Error("rulers on but no top inset")
		}
	}
}

// A splitter drag let go over the HUD or the zoom buttons ends there: the release reaches the
// root, and later pointer moves no longer resize the column.
func TestDragReleasedOverHUDOrZoomEnds(t *testing.T) {
	for _, over := range []string{"props-hud", "canvas-zoom-in"} {
		h := hudRig(t)
		h.ws.togglePropsHUD()
		h.hw.Window().Refresh()
		h.hw.Pixels()
		start := h.find("split-side")
		to := h.find(over)
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: start})
		h.hw.Dispatch(gpui.MouseDownEvent{Position: start, Button: gpui.MouseLeft, ClickCount: 1})
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: to})
		h.hw.Dispatch(gpui.MouseUpEvent{Position: to, Button: gpui.MouseLeft, ClickCount: 1})
		h.hw.Pixels()
		if h.ws.lay.drag.kind != splitNone {
			t.Fatalf("%s: the drag is still held after the release", over)
		}
		w := h.ws.lay.sideW
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 400}})
		h.hw.Pixels()
		if h.ws.lay.sideW != w {
			t.Fatalf("%s: the column follows the pointer after the release: %v -> %v", over, w, h.ws.lay.sideW)
		}
	}
}

// The Properties lines' summary is worked out once per selection and drawing state, however many
// views (side panel, HUD) ask for it in a frame; a new selection or an edit refreshes it.
func TestSelectionSummaryIsCached(t *testing.T) {
	h := hudRig(t)
	h.selectAll()
	a := h.ws.selectionSummary()
	classes := a.classes
	if b := h.ws.selectionSummary(); &b.classes[0] != &classes[0] {
		t.Fatal("the summary was worked out again for the same selection")
	}
	if a.n != 2 || len(a.classes) != 1 || a.counts[0] != 2 {
		t.Fatalf("summary %+v", *a)
	}
	h.ws.session.Selection().Remove(h.ws.session.Selection().Handles()[0])
	c := h.ws.selectionSummary()
	if c.n != 1 || c.handle == "" || c.layer != "Walls" {
		t.Fatalf("after deselecting one: %+v", *c)
	}
	d := h.ws.session.Drawing()
	tx, _ := d.Begin()
	tx.Modify(c.handles[0], func(e doc.Entity) { e.Common().Layer = "0" })
	tx.Commit()
	if e := h.ws.selectionSummary(); e.layer != "0" {
		t.Fatalf("an edit did not refresh the summary: layer %q", e.layer)
	}
}

func BenchmarkSelectionSummary(b *testing.B) {
	h := hudRig(&testing.T{})
	h.selectAll()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ws.selectionSummary()
	}
}
