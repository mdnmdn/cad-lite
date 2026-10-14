package ui_test

import (
	"testing"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/internal/gpui"
	"go-draw/ui"
)

type harness struct {
	t  *testing.T
	ws *ui.Workspace
	hw *gpui.HeadlessWindow
}

func open(t *testing.T) *harness {
	t.Helper()
	ws := ui.NewWorkspace(doc.New(), nil)
	p := gpui.NewHeadlessPlatform()
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(ui.Bindings()...)
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 900, Height: 560}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	return &harness{t: t, ws: ws, hw: hw}
}

func (h *harness) typ(s string) {
	for _, r := range s {
		c := string(r)
		key := c
		if c == " " {
			key = "space"
		}
		h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: key, KeyChar: &c}, PreferCharacterInput: true})
	}
}

func (h *harness) key(k string) { h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: k}}) }

func (h *harness) at(p geom.Vec2) gpui.Point { return h.ws.ScreenPoint(p) }

func (h *harness) press(p gpui.Point, b gpui.MouseButton) {
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: b, ClickCount: 1})
}

func (h *harness) release(p gpui.Point, b gpui.MouseButton) {
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: b, ClickCount: 1})
}

func (h *harness) click(p gpui.Point, b gpui.MouseButton) {
	h.press(p, b)
	h.release(p, b)
}

func (h *harness) lines() []*doc.Line {
	var out []*doc.Line
	d := h.ws.Session().Drawing()
	for _, hd := range d.Handles() {
		if l, ok := d.Entity(hd).(*doc.Line); ok {
			out = append(out, l)
		}
	}
	return out
}

func TestTypedCommandClicksAndRightClickEnter(t *testing.T) {
	h := open(t)
	h.typ("line")
	h.key("enter")
	if h.ws.Session().Active() != "LINE" {
		t.Fatalf("typing LINE + Enter: active = %q, history %q", h.ws.Session().Active(), h.ws.Session().History())
	}
	h.click(h.at(geom.V(0, 0)), gpui.MouseLeft)
	h.click(h.at(geom.V(100, 0)), gpui.MouseLeft)
	h.typ("@0,50")
	h.key("enter")
	h.click(h.at(geom.V(0, 0)), gpui.MouseRight)
	if h.ws.Session().Active() != "" {
		t.Fatalf("right click did not end LINE (active %q)", h.ws.Session().Active())
	}
	ls := h.lines()
	if len(ls) != 2 {
		t.Fatalf("lines = %d, want 2", len(ls))
	}
	if !ls[0].End.Near(geom.V(100, 0), 0.5) || !ls[1].End.Sub(ls[1].Start).Near(geom.V(0, 50), 1e-9) {
		t.Fatalf("line ends %v %v", ls[0].End, ls[1].End)
	}
}

func TestSpaceIsEnterAndEscapeCancels(t *testing.T) {
	h := open(t)
	h.typ("c ")
	if h.ws.Session().Active() != "CIRCLE" {
		t.Fatalf("\"c\" + space: active = %q", h.ws.Session().Active())
	}
	h.key("escape")
	if h.ws.Session().Active() != "" {
		t.Fatal("escape did not cancel the command")
	}
	h.typ("la ")
	h.typ("my layer")
	h.key("enter")
	if got := h.ws.Session().Drawing().CurrentLayer; got != "my layer" {
		t.Fatalf("text prompt kept spaces? current layer = %q", got)
	}
}

func TestBoxDragSelectsAndCtrlZUndoes(t *testing.T) {
	h := open(t)
	h.ws.Session().Script("LINE 0,0 50,0 ")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: h.at(geom.V(0, 0))})
	h.press(h.at(geom.V(-10, -10)), gpui.MouseLeft)
	h.release(h.at(geom.V(60, 10)), gpui.MouseLeft)
	if n := h.ws.Session().Selection().Len(); n != 1 {
		t.Fatalf("window drag selected %d, want 1", n)
	}
	h.typ("e ")
	if h.ws.Session().Drawing().Len() != 0 {
		t.Fatal("ERASE with a pre-selection did not erase")
	}
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "z", Modifiers: gpui.Control}})
	if h.ws.Session().Drawing().Len() != 1 {
		t.Fatal("ctrl-z did not undo the erase")
	}
}

func TestPanAndZoom(t *testing.T) {
	h := open(t)
	v := h.ws.View()
	c0, s0 := v.Center, v.Scale
	start := gpui.Point{X: 300, Y: 300}
	h.press(start, gpui.MouseMiddle)
	h.release(gpui.Point{X: 350, Y: 300}, gpui.MouseMiddle)
	if !v.Center.Near(c0.Add(geom.V(-50/s0, 0)), 1e-9) {
		t.Fatalf("middle drag: center %v, want %v", v.Center, c0.Add(geom.V(-50/s0, 0)))
	}
	anchor := h.ws.View().ToModel(200, 200)
	origin := h.at(anchor)
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: origin, DeltaY: -120})
	if v.Scale <= s0 {
		t.Fatalf("wheel up did not zoom in: %v -> %v", s0, v.Scale)
	}
	if got := h.at(anchor); abs(got.X-origin.X) > 0.01 || abs(got.Y-origin.Y) > 0.01 {
		t.Fatalf("zoom moved the point under the cursor: %v -> %v", origin, got)
	}
}

func TestSelectionHighlightIsPainted(t *testing.T) {
	h := open(t)
	h.ws.Session().Script("CIRCLE 0,0 40")
	before := len(h.hw.Window().Scene().Strokes)
	h.click(h.at(geom.V(40, 0)), gpui.MouseLeft)
	if h.ws.Session().Selection().Len() != 1 {
		t.Fatal("click on the circle did not select it")
	}
	if after := len(h.hw.Window().Scene().Strokes); after <= before {
		t.Fatalf("strokes %d -> %d: no highlight painted", before, after)
	}
}

func abs(v gpui.Pixels) gpui.Pixels {
	if v < 0 {
		return -v
	}
	return v
}
