package ui_test

import (
	"testing"
	"time"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/ui"
)

// tipRig is a headless window whose clock the test advances.
type tipRig struct {
	t  *testing.T
	p  *gpui.HeadlessPlatform
	hw *gpui.HeadlessWindow
}

func openTip(t *testing.T) *tipRig {
	t.Helper()
	ws := ui.NewWorkspace(doc.New(), nil)
	p := gpui.NewHeadlessPlatform()
	r := &tipRig{t: t, p: p}
	if err := p.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		r.hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	r.hw.Pixels()
	return r
}

func (r *tipRig) hover(x, y gpui.Pixels, wait time.Duration) {
	r.t.Helper()
	r.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: x, Y: y}})
	if err := r.p.Advance(wait); err != nil {
		r.t.Fatal(err)
	}
}

// borders is the set of boxes in the scene drawn with the theme's border colour.
func (r *tipRig) borders() map[gpui.Bounds]bool {
	th := ui.CurrentTheme()
	out := map[gpui.Bounds]bool{}
	for _, q := range r.hw.Window().Scene().Quads {
		if q.BorderColor == th.Border {
			out[q.Bounds] = true
		}
	}
	return out
}

// bubbleSince reports whether a tooltip bubble (a new bordered box, more than 60 px wide) is in
// the scene that base, taken with the pointer parked, did not hold.
func (r *tipRig) bubbleSince(base map[gpui.Bounds]bool) bool {
	for b := range r.borders() {
		if !base[b] && b.Size.W > 60 {
			return true
		}
	}
	return false
}

// centre returns the centre of the element with the given id.
func (r *tipRig) centre(id string) (gpui.Pixels, gpui.Pixels) {
	b, ok := (&tbRig{t: r.t, hw: r.hw}).boundsAnywhere(id)
	if !ok {
		r.t.Fatalf("%s not found", id)
	}
	return gpui.Pixels((b.x0 + b.x1) / 2), gpui.Pixels((b.y0 + b.y1) / 2)
}

func TestControlTooltipsAppearAfterDelay(t *testing.T) {
	r := openTip(t)
	for _, id := range []string{"tool-line", "tool-mirror", "tool-dist", "tool-open", "tool-save", "tool-undo", "tool-agent", "tool-settings",
		"doc-chip", "viewport-label", "canvas-zoom-in", "canvas-zoom-out", "canvas-zoom-fit", "layer-add", "cmd-recent", "cmd-handle",
		"status-tab-model", "status-tab-layout1", "status-coords", "status-grid", "status-snapgrid", "status-ortho", "status-polar",
		"status-snap", "status-lwt", "status-model", "status-units"} {
		x, y := r.centre(id)
		r.hover(5, 400, 600*time.Millisecond) // park the pointer on the canvas
		base := r.borders()
		r.hover(x, y, 300*time.Millisecond)
		if r.bubbleSince(base) {
			t.Errorf("%s: tooltip before the delay", id)
		}
		if err := r.p.Advance(300 * time.Millisecond); err != nil { // resting: no new move
			t.Fatal(err)
		}
		if !r.bubbleSince(base) {
			t.Errorf("%s: no tooltip after 600 ms", id)
		}
	}
}

func TestTooltipHidesOnClick(t *testing.T) {
	r := openTip(t)
	x, y := r.centre("tool-line")
	r.hover(5, 400, 600*time.Millisecond)
	base := r.borders()
	r.hover(x, y, 600*time.Millisecond)
	if !r.bubbleSince(base) {
		t.Fatal("no tooltip after 600 ms")
	}
	r.hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: x, Y: y}, Button: gpui.MouseLeft, ClickCount: 1})
	if r.bubbleSince(base) {
		t.Fatal("tooltip still shown after a click")
	}
}
