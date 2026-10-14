package gpui_test

import (
	"testing"
	"time"

	"go-draw/internal/gpui"
)

type tipRig struct {
	p   *gpui.HeadlessPlatform
	app *gpui.App
	w   *gpui.Window
	hw  *gpui.HeadlessWindow
}

var tipBg = gpui.RgbaOf(1, 2, 3, 255)

func newTipRig(t *testing.T, ww, wh float32, build func() gpui.IntoElement) *tipRig {
	t.Helper()
	r := &tipRig{p: gpui.NewHeadlessPlatform()}
	r.app = gpui.NewAppWithPlatform(r.p)
	st := gpui.DefaultTooltipStyle()
	st.Background = tipBg
	r.app.SetTooltipStyle(st)
	err := r.app.OpenWindow(gpui.WindowOptions{Width: ww, Height: wh}, func(w *gpui.Window, _ *gpui.App) gpui.EntityRoot {
		r.w = w
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement { return build() })
	})
	if err != nil {
		t.Fatal(err)
	}
	r.hw = r.p.Windows()[0]
	return r
}

func (r *tipRig) move(x, y gpui.Pixels) {
	r.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: x, Y: y}})
}

func (r *tipRig) advance(t *testing.T, d time.Duration) {
	t.Helper()
	if err := r.p.Advance(d); err != nil {
		t.Fatal(err)
	}
}

func (r *tipRig) bubble() (gpui.Bounds, bool) {
	for _, q := range r.w.Scene().Quads {
		if q.Fill == tipBg {
			return q.Bounds, true
		}
	}
	return gpui.Bounds{}, false
}

func tipTree() gpui.IntoElement {
	return gpui.NewDiv().SizeFull().Child(
		gpui.NewDiv().SizeWH(60, 30).Bg(gpui.Blue500).Tooltip("Hello tip"))
}

func TestTooltipAppearsAfterDelayOnly(t *testing.T) {
	r := newTipRig(t, 300, 200, tipTree)
	r.move(10, 10)
	r.advance(t, 499*time.Millisecond)
	if _, ok := r.bubble(); ok {
		t.Fatal("tooltip shown before the delay")
	}
	r.advance(t, 2*time.Millisecond)
	b, ok := r.bubble()
	if !ok {
		t.Fatal("tooltip not shown after the delay")
	}
	if b.Origin.Y < 10 {
		t.Fatalf("bubble overlaps the pointer: %v", b)
	}
	if len(r.w.Scene().Sprites) == 0 {
		t.Fatal("no text glyphs")
	}
}

func TestTooltipMoveRestartsDelay(t *testing.T) {
	r := newTipRig(t, 300, 200, tipTree)
	r.move(10, 10)
	r.advance(t, 400*time.Millisecond)
	r.move(12, 12)
	r.advance(t, 400*time.Millisecond)
	if _, ok := r.bubble(); ok {
		t.Fatal("tooltip shown although the pointer kept moving")
	}
	r.advance(t, 150*time.Millisecond)
	if _, ok := r.bubble(); !ok {
		t.Fatal("tooltip not shown after resting")
	}
}

func TestTooltipHides(t *testing.T) {
	cases := map[string]func(r *tipRig){
		"leave": func(r *tipRig) { r.move(200, 150) },
		"down": func(r *tipRig) {
			r.hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft})
		},
		"scroll": func(r *tipRig) { r.hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: 10, Y: 10}}) },
		"key":    func(r *tipRig) { r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "a"}}) },
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			r := newTipRig(t, 300, 200, tipTree)
			r.move(10, 10)
			r.advance(t, 600*time.Millisecond)
			if _, ok := r.bubble(); !ok {
				t.Fatal("tooltip not shown")
			}
			act(r)
			r.advance(t, time.Millisecond)
			if _, ok := r.bubble(); ok {
				t.Fatal("tooltip still shown")
			}
			if name != "leave" {
				r.advance(t, 2*time.Second)
				if _, ok := r.bubble(); ok {
					t.Fatal("tooltip came back without leaving the element")
				}
				r.move(200, 150)
				r.move(10, 10)
				r.advance(t, 600*time.Millisecond)
				if _, ok := r.bubble(); !ok {
					t.Fatal("tooltip not shown after re-entering")
				}
			}
		})
	}
}

func TestTooltipLeaveBeforeDelayCancels(t *testing.T) {
	r := newTipRig(t, 300, 200, tipTree)
	r.move(10, 10)
	r.advance(t, 300*time.Millisecond)
	r.move(200, 150)
	r.advance(t, time.Second)
	if _, ok := r.bubble(); ok {
		t.Fatal("tooltip shown after leaving early")
	}
}

func TestTooltipClampedInWindow(t *testing.T) {
	build := func() gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Child(
			gpui.NewDiv().SizeFull().Tooltip("A fairly long tooltip text for clamping"))
	}
	r := newTipRig(t, 200, 100, build)
	for _, pt := range []gpui.Point{{X: 195, Y: 50}, {X: 100, Y: 98}, {X: 199, Y: 99}} {
		r.move(pt.X, pt.Y)
		r.advance(t, 600*time.Millisecond)
		b, ok := r.bubble()
		if !ok {
			t.Fatalf("no tooltip at %v", pt)
		}
		if b.Origin.X < 0 || b.Origin.Y < 0 || b.Origin.X+b.Size.W > 200 || b.Origin.Y+b.Size.H > 100 {
			t.Fatalf("bubble %v outside the window at %v", b, pt)
		}
		r.move(pt.X-1, pt.Y-1)
		r.advance(t, time.Millisecond)
	}
}

func TestTooltipDoesNotTakeInput(t *testing.T) {
	r := newTipRig(t, 300, 200, tipTree)
	r.move(10, 10)
	r.advance(t, 600*time.Millisecond)
	b, ok := r.bubble()
	if !ok {
		t.Fatal("tooltip not shown")
	}
	c := gpui.Point{X: b.Origin.X + b.Size.W/2, Y: b.Origin.Y + b.Size.H/2}
	id, ok := r.w.HitTest(c)
	if !ok {
		t.Fatal("nothing under the bubble")
	}
	for _, k := range id.Path {
		if k.Key == "__gpui_tooltip" {
			t.Fatal("tooltip took a hitbox")
		}
	}
}

func TestTooltipStyleDelay(t *testing.T) {
	r := newTipRig(t, 300, 200, tipTree)
	st := r.app.TooltipStyle()
	st.Delay = 100 * time.Millisecond
	r.app.SetTooltipStyle(st)
	r.move(10, 10)
	r.advance(t, 101*time.Millisecond)
	if _, ok := r.bubble(); !ok {
		t.Fatal("custom delay ignored")
	}
}

func TestTooltipWrappedTextBubbleCoversAllLines(t *testing.T) {
	long := "Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron"
	build := func(text string) func() gpui.IntoElement {
		return func() gpui.IntoElement {
			return gpui.NewDiv().SizeFull().Child(gpui.NewDiv().SizeWH(60, 30).Bg(gpui.Blue500).Tooltip(text))
		}
	}
	height := func(text string, maxW float32) gpui.Bounds {
		r := newTipRig(t, 400, 300, build(text))
		st := r.app.TooltipStyle()
		st.MaxWidth = maxW
		r.app.SetTooltipStyle(st)
		r.move(10, 10)
		r.advance(t, 600*time.Millisecond)
		b, ok := r.bubble()
		if !ok {
			t.Fatal("tooltip not shown")
		}
		return b
	}
	one := height("Alpha", 120)
	wrapped := height(long, 120)
	if wrapped.Width() > 121 {
		t.Fatalf("bubble wider than MaxWidth: %v", wrapped)
	}
	if wrapped.Height() < one.Height()*3 {
		t.Fatalf("bubble %v (one line %v) does not cover the wrapped lines", wrapped, one)
	}
	hard := height("one\ntwo", 0)
	if hard.Height() < one.Height()+10 {
		t.Fatalf("newline is not a hard break: %v vs %v", hard, one)
	}
}
