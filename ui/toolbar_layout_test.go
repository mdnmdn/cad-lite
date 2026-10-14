package ui_test

import (
	"fmt"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/ui"
)

// tbRig is a headless window of a given width.
type tbRig struct {
	t  *testing.T
	ws *ui.Workspace
	hw *gpui.HeadlessWindow
}

func openWidth(t *testing.T, w int) *tbRig {
	t.Helper()
	ws := ui.NewWorkspace(doc.New(), nil)
	p := gpui.NewHeadlessPlatform()
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: float32(w), Height: 700}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	return &tbRig{t: t, ws: ws, hw: hw}
}

type rect struct{ x0, y0, x1, y1 float32 }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

// bounds returns the box of the points whose hit path holds id, scanned over the title bar and
// the ribbon (the top 420 px of the window); ok is false when nothing hit.
func (r *tbRig) bounds(id string) (rect, bool) { return r.boundsIn(id, 0, 420, 1) }

// boundsAnywhere is bounds over the whole window, sampled every 2 px.
func (r *tbRig) boundsAnywhere(id string) (rect, bool) {
	return r.boundsIn(id, 0, float32(r.hw.Window().Bounds().Size.H), 2)
}

func (r *tbRig) boundsIn(id string, y0, y1, step float32) (rect, bool) {
	w := r.hw.Window()
	b := w.Bounds()
	out := rect{1e9, 1e9, -1, -1}
	for y := y0; y < y1; y += step {
		for x := float32(0); x < float32(b.Size.W); x += step {
			gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
			if !ok {
				continue
			}
			for _, k := range gid.Path {
				if k.Key == id {
					out = rect{min(out.x0, x), min(out.y0, y), max(out.x1, x+step), max(out.y1, y+step)}
					break
				}
			}
		}
	}
	return out, out.x1 >= 0
}

// ribbonGroupButtons are the ribbon's groups and the buttons in each, by id.
var ribbonGroupButtons = [][]string{
	{"line", "pline", "circle", "arc", "rect"},
	{"move", "copy", "rotate", "scale", "erase", "mirror", "offset", "fillet", "chamfer", "trim", "extend", "break", "join", "explode"},
	{"dist", "area", "id", "zoom"},
}

// titleButtons are the title bar's buttons from left to right.
var titleButtons = []string{"open", "save", "undo", "redo", "agent", "settings"}

func TestRibbonReflowsWithWidth(t *testing.T) {
	for _, c := range []struct {
		width int
		rows  func(n int) bool
	}{
		{1600, func(n int) bool { return n == 1 }},
		{1100, func(n int) bool { return n == 2 }},
		{800, func(n int) bool { return n >= 2 }},
	} {
		t.Run(fmt.Sprint(c.width), func(t *testing.T) {
			r := openWidth(t, c.width)
			var all []rect
			var tops []float32
			for gi, g := range ribbonGroupButtons {
				var rs []rect
				for _, id := range g {
					b, ok := r.bounds("tool-" + id)
					if !ok {
						t.Fatalf("button %s not visible", id)
					}
					if b.x0 < 0 || b.x1 > float32(c.width) {
						t.Fatalf("button %s clipped: %v in width %d", id, b, c.width)
					}
					for _, o := range all {
						if b.overlaps(o) {
							t.Fatalf("button %s overlaps another: %v %v", id, b, o)
						}
					}
					all = append(all, b)
					rs = append(rs, b)
				}
				// A group is one block: its buttons stay inside the group's own box.
				box, ok := r.bounds([]string{"tbgroup-draw", "tbgroup-modify", "tbgroup-utilities"}[gi])
				if !ok {
					t.Fatalf("group %d has no box", gi)
				}
				for _, b := range rs {
					if b.y0 < box.y0-1 || b.y1 > box.y1+1 {
						t.Fatalf("group %d: button %v leaves its box %v", gi, b, box)
					}
				}
				tops = append(tops, box.y0)
			}
			rows := map[float32]bool{}
			for _, y := range tops {
				rows[float32(int(y/10))] = true
			}
			if !c.rows(len(rows)) {
				t.Fatalf("width %d: %d ribbon rows (%v)", c.width, len(rows), tops)
			}
		})
	}
}

func TestTitleBarButtonsFitAndSitInOrder(t *testing.T) {
	for _, width := range []int{1600, 900, 700} {
		r := openWidth(t, width)
		prev := float32(-1)
		for _, id := range titleButtons {
			b, ok := r.bounds("tool-" + id)
			if !ok {
				t.Fatalf("width %d: title bar button %s not visible", width, id)
			}
			if b.y1 > 52 || b.x1 > float32(width) || b.x0 < prev {
				t.Fatalf("width %d: button %s at %v is out of place (previous ends at %v)", width, id, b, prev)
			}
			if b.x1-b.x0 > 36 {
				t.Fatalf("button %s is %v wide: title bar buttons are icon-only", id, b.x1-b.x0)
			}
			prev = b.x1
		}
		// Agent and Settings sit at the right end.
		settings, _ := r.bounds("tool-settings")
		if settings.x1 < float32(width)-20 {
			t.Fatalf("width %d: settings button not at the right end: %v", width, settings)
		}
	}
}

func TestCanvasIsFramed(t *testing.T) {
	r := openWidth(t, 1000)
	cb, ok := r.boundsAnywhere("canvas")
	if !ok {
		t.Fatal("canvas not found")
	}
	th := ui.CurrentTheme()
	near := func(a, b float32) bool { return a-b < 3 && b-a < 3 }
	var top, bottom, left, right bool
	for _, q := range r.hw.Window().Scene().Quads {
		if q.Fill != th.Border {
			continue
		}
		x, y, w, h := float32(q.Bounds.Origin.X), float32(q.Bounds.Origin.Y), float32(q.Bounds.Size.W), float32(q.Bounds.Size.H)
		wide, tall := near(w, cb.x1-cb.x0) && h == 1, near(h, cb.y1-cb.y0) && w == 1
		top = top || wide && near(y, cb.y0)
		bottom = bottom || wide && near(y+1, cb.y1)
		left = left || tall && near(x, cb.x0)
		right = right || tall && near(x+1, cb.x1)
	}
	if !top || !bottom || !left || !right {
		t.Fatalf("canvas %v: border edges top=%v bottom=%v left=%v right=%v", cb, top, bottom, left, right)
	}
}
