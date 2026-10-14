package gpui_test

import (
	"go-draw/internal/gpui"
	"testing"
)

func activeWindow(t *testing.T, root gpui.EntityRoot) (*gpui.Window, *gpui.HeadlessWindow) {
	t.Helper()
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 120, Height: 120}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
		t.Fatal(err)
	}
	return app.Windows()[0], p.Windows()[0]
}

func activePixel(t *testing.T, hw *gpui.HeadlessWindow, x, y int, want gpui.Rgba) {
	t.Helper()
	c := hw.Image().NRGBAAt(x, y)
	if c.R != want.R || c.G != want.G || c.B != want.B || c.A != want.A {
		t.Fatalf("pixel(%d,%d)=%v want=%v", x, y, c, want)
	}
}

func activeDown(hw *gpui.HeadlessWindow, x, y gpui.Pixels, button gpui.MouseButton) {
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: x, Y: y}, Button: button})
}
func activeUp(hw *gpui.HeadlessWindow, x, y gpui.Pixels, button gpui.MouseButton) {
	hw.Dispatch(gpui.MouseUpEvent{Position: gpui.Point{X: x, Y: y}, Button: button})
}

func TestActivePrimaryPressReleaseAndLeave(t *testing.T) {
	renders := 0
	_, hw := activeWindow(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		renders++
		return gpui.NewDiv().ID("button").SizeWH(60, 60).Bg(gpui.Blue500).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) })
	}))
	activeDown(hw, 10, 10, gpui.MouseRight)
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Red500)
	activeUp(hw, 10, 10, gpui.MouseRight)
	activePixel(t, hw, 10, 10, gpui.Red500)
	activeUp(hw, 100, 100, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 60, Y: 10}})
	activePixel(t, hw, 10, 10, gpui.Blue500)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 10, Y: 10}})
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Red500)
	activeUp(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Blue500)
	n := renders
	hw.Image()
	hw.Image()
	if renders != n {
		t.Fatalf("idle images redrew %d times", renders-n)
	}
}

func TestActiveNestedPathAndStoppedPropagation(t *testing.T) {
	_, hw := activeWindow(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		child := gpui.NewDiv().ID("child").SizeWH(40, 40).Bg(gpui.Blue500).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) }).
			OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
			OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
		return gpui.NewDiv().ID("parent").SizeWH(100, 100).Bg(gpui.Blue500).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Green600) }).Child(child)
	}))
	activeDown(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Red500)
	activePixel(t, hw, 80, 80, gpui.Green600)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 80, Y: 80}})
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activePixel(t, hw, 80, 80, gpui.Green600)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 10, Y: 10}})
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	activeUp(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activePixel(t, hw, 80, 80, gpui.Blue500)
}

func TestActiveRedrawIdentityAndRemoval(t *testing.T) {
	id := "old"
	hidden, enabled, removed := false, true, false
	offset := float32(0)
	renders := 0
	w, hw := activeWindow(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		renders++
		d := gpui.NewDiv().ID(id).Absolute().Left(offset).SizeWH(60, 60).Bg(gpui.Blue500)
		if hidden {
			d.Hidden()
		}
		if enabled {
			d.Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) })
		}
		if removed {
			return gpui.NewDiv()
		}
		return gpui.NewDiv().Child(d)
	}))
	activeDown(hw, 10, 10, gpui.MouseLeft)
	n := renders
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Red500)
	if renders != n+1 {
		t.Fatalf("redraw count=%d want %d", renders, n+1)
	}
	id = "new"
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	id = "old"
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	hidden = true
	w.Refresh()
	hw.Image()
	hidden = false
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	enabled = false
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	enabled = true
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	removed = true
	w.Refresh()
	hw.Image()
	removed = false
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	offset = 70
	w.Refresh()
	hw.Image()
	offset = 0
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
}

func TestActiveOcclusionAndNewFrameCancellation(t *testing.T) {
	cover := false
	w, hw := activeWindow(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		d := gpui.NewDiv().ID("bottom").SizeWH(60, 60).Bg(gpui.Blue500).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) })
		root := gpui.NewDiv().Child(d)
		if cover {
			root.Child(gpui.NewDiv().ID("top").Absolute().SizeWH(60, 60).Bg(gpui.Green600))
		}
		return root
	}))
	cover = true
	w.Refresh()
	hw.Image()
	activeDown(hw, 10, 10, gpui.MouseLeft)
	cover = false
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
	activeDown(hw, 10, 10, gpui.MouseLeft)
	activePixel(t, hw, 10, 10, gpui.Red500)
	cover = true
	w.Refresh()
	hw.Image()
	cover = false
	w.Refresh()
	activePixel(t, hw, 10, 10, gpui.Blue500)
}

func TestActiveFocusHoverPrecedenceAndTextOpacity(t *testing.T) {
	w, hw := activeWindow(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().ID("button").TabIndex(0).SizeWH(100, 60).Bg(gpui.Black).
			Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Yellow400) }).
			FocusVisible(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Green600) }).
			Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Blue500) }).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin {
				return s.Bg(gpui.Red500).TextColor(gpui.Green600).TextStrikethrough().Opacity(0.5).SizeWH(500, 500)
			}).
			Child(gpui.NewText("Pressed"))
	}))
	w.FocusNext()
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "shift"}})
	activePixel(t, hw, 80, 40, gpui.Green600)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 80, Y: 40}})
	activePixel(t, hw, 80, 40, gpui.Blue500)
	activeDown(hw, 80, 40, gpui.MouseLeft)
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "shift"}})
	sc := w.Scene()
	found := false
	for _, q := range sc.Quads {
		if q.Fill.R == gpui.Red500.R && q.Fill.A == gpui.Red500.Opacity(0.5).A {
			wantRect(t, "active keeps layout", q.Bounds, 0, 0, 100, 60)
			found = true
		}
	}
	if !found {
		t.Fatalf("active red/opacity quad absent: %+v", sc.Quads)
	}
	if len(sc.Sprites) > 0 {
		for _, s := range sc.Sprites {
			if s.Color != gpui.Green600.Opacity(0.5) {
				t.Fatalf("active inherited text colour=%v", s.Color)
			}
		}
		if len(sc.Underlines) == 0 {
			t.Fatal("active inherited strikethrough absent")
		}
	}
	activeUp(hw, 80, 40, gpui.MouseLeft)
	activePixel(t, hw, 80, 40, gpui.Blue500)
}
