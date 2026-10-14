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

// bounds returns the box of the points whose hit path holds id, scanned over
// the top 140 px of the window; ok is false when nothing hit.
func (r *tbRig) bounds(id string) (rect, bool) {
	w := r.hw.Window()
	b := w.Bounds()
	out := rect{1e9, 1e9, -1, -1}
	for y := float32(0); y < 140; y++ {
		for x := float32(0); x < float32(b.Size.W); x++ {
			gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
			if !ok {
				continue
			}
			for _, k := range gid.Path {
				if k.Key == id {
					out = rect{min(out.x0, x), min(out.y0, y), max(out.x1, x+1), max(out.y1, y+1)}
					break
				}
			}
		}
	}
	return out, out.x1 >= 0
}

var toolbarGroupButtons = [][]string{
	{"line", "pline", "circle", "arc", "rect"},
	{"dist", "area", "id"},
	{"undo", "redo", "zoom"},
	{"move", "copy", "rotate", "scale", "erase"},
	{"mirror", "offset", "fillet", "chamfer"},
	{"trim", "extend", "break", "join", "explode"},
	{"agent", "settings"},
}

func TestToolbarReflowsWithWidth(t *testing.T) {
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
			for gi, g := range toolbarGroupButtons {
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
				for _, b := range rs[1:] {
					if b.y0 > rs[0].y0+3 || b.y0 < rs[0].y0-3 {
						t.Fatalf("group %d split across rows: %v vs %v", gi, rs[0], b)
					}
				}
				tops = append(tops, rs[0].y0)
			}
			rows := map[float32]bool{}
			for _, y := range tops {
				rows[float32(int(y/10))] = true
			}
			if !c.rows(len(rows)) {
				t.Fatalf("width %d: %d toolbar rows (%v)", c.width, len(rows), tops)
			}
			// The window buttons sit at the right end of the last row they are on.
			agent, _ := r.bounds("tool-agent")
			settings, _ := r.bounds("tool-settings")
			if agent.x1 > settings.x0 || settings.x1 < float32(c.width)-20 {
				t.Fatalf("window buttons not at the right end: %v %v", agent, settings)
			}
		})
	}
}

func TestCanvasIsFramed(t *testing.T) {
	r := openWidth(t, 1000)
	th := ui.CurrentTheme()
	n := 0
	for _, q := range r.hw.Window().Scene().Quads {
		if q.Fill == th.Border && (q.Bounds.Size.W > 300 && q.Bounds.Size.H == 1 || q.Bounds.Size.H > 300 && q.Bounds.Size.W == 1) {
			n++
		}
	}
	if n < 4 {
		t.Fatalf("want 4 border edges around the canvas, found %d", n)
	}
}
