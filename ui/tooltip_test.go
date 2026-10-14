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

// bubble reports whether the themed tooltip bubble is in the scene: a
// tooltip-sized quad with the theme's border colour and a width no button has.
func (r *tipRig) bubble() bool {
	th := ui.CurrentTheme()
	for _, q := range r.hw.Window().Scene().Quads {
		if q.BorderColor == th.Border && q.Bounds.Size.W > 150 && q.Bounds.Size.H < 80 {
			return true
		}
	}
	return false
}

func TestToolbarAndStatusTooltipsAppearAfterDelay(t *testing.T) {
	r := openTip(t)
	for _, c := range []struct {
		name string
		x, y gpui.Pixels
	}{
		{"line button", 96, 17},
		{"mirror button", 249, 57},
		{"settings button", 1241, 17},
		{"grid flag", 228, 786},
		{"osnap flag", 300, 786},
	} {
		r.hover(5, 400, 600*time.Millisecond) // park the pointer on the canvas
		r.hover(c.x, c.y, 300*time.Millisecond)
		if r.bubble() {
			t.Errorf("%s: tooltip before the delay", c.name)
		}
		if err := r.p.Advance(300 * time.Millisecond); err != nil { // resting: no new move
			t.Fatal(err)
		}
		if !r.bubble() {
			t.Errorf("%s: no tooltip after 600 ms", c.name)
		}
	}
}

func TestTooltipHidesOnClick(t *testing.T) {
	r := openTip(t)
	r.hover(96, 17, 600*time.Millisecond)
	if !r.bubble() {
		t.Fatal("no tooltip after 600 ms")
	}
	r.hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 96, Y: 17}, Button: gpui.MouseLeft, ClickCount: 1})
	if r.bubble() {
		t.Fatal("tooltip still shown after a click")
	}
}
