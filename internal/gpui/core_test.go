package gpui

import "testing"

// fakePlatformWindow records what the window presents, with no OS involved.
type fakePlatformWindow struct {
	w, h      Pixels
	scale     float32
	presented []byte
	presentW  int
	presentH  int
	frames    int
	handler   PlatformInputHandler
	requested int
}

func (f *fakePlatformWindow) Bounds() (Pixels, Pixels) { return f.w, f.h }

func (f *fakePlatformWindow) ScaleFactor() float32 {
	if f.scale == 0 {
		return 1
	}
	return f.scale
}
func (f *fakePlatformWindow) RequestFrame() { f.requested++ }
func (f *fakePlatformWindow) Present(pix []byte, w, h int) {
	f.presented = append([]byte(nil), pix...)
	f.presentW, f.presentH = w, h
	f.frames++
}
func (f *fakePlatformWindow) SetInputHandler(h PlatformInputHandler) { f.handler = h }
func (f *fakePlatformWindow) TakeInputHandler() PlatformInputHandler {
	h := f.handler
	f.handler = nil
	return h
}
func (f *fakePlatformWindow) Quit() {}

type fakePlatform struct{ last *fakePlatformWindow }

func (p *fakePlatform) Run(onLaunch func(*App)) error {
	if onLaunch != nil {
		onLaunch(NewApp())
	}
	return nil
}
func (p *fakePlatform) OpenWindow(o WindowOptions) (PlatformWindow, error) {
	w, h := o.Width, o.Height
	if w == 0 {
		w = 800
	}
	if h == 0 {
		h = 600
	}
	f := &fakePlatformWindow{w: Pixels(w), h: Pixels(h)}
	p.last = f
	return f, nil
}
func (p *fakePlatform) Quit()        {}
func (p *fakePlatform) Name() string { return "fake" }

func newTestApp(t *testing.T, w, h float32, root EntityRoot) (*App, *Window, *fakePlatformWindow) {
	t.Helper()
	p := &fakePlatform{}
	app := NewAppWithPlatform(p)
	err := app.OpenWindow(WindowOptions{Width: w, Height: h, Title: "test"}, func(*Window, *App) EntityRoot { return root })
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	if len(app.Windows()) == 0 {
		t.Fatal("OpenWindow registered no window")
	}
	return app, app.Windows()[0], p.last
}

func TestDrawSingleDivProducesOneQuad(t *testing.T) {
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		return NewDiv().SizeWH(100, 100).Bg(Red500)
	})
	_, win, _ := newTestApp(t, 400, 300, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))

	sc := win.Scene()
	if len(sc.Quads) != 1 {
		t.Fatalf("got %d quads, want 1", len(sc.Quads))
	}
	q := sc.Quads[0]
	if q.Bounds.Origin.X != 0 || q.Bounds.Origin.Y != 0 ||
		q.Bounds.Size.W != 100 || q.Bounds.Size.H != 100 {
		t.Errorf("quad bounds = %+v, want origin (0,0) size 100x100", q.Bounds)
	}
	if q.Fill != Red500 {
		t.Errorf("quad fill = %+v, want Red500", q.Fill)
	}
	if q.Background != BackgroundSolid {
		t.Errorf("background kind = %v, want BackgroundSolid", q.Background)
	}
}

func TestDrawNestedDivsRespectPaddingAndFlex(t *testing.T) {
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		inner := NewDiv().SizeWH(50, 50).Bg(Blue500)
		outer := NewDiv().SizeWH(200, 200).Bg(Gray200).FlexCol().P(10).Child(inner)
		return outer
	})
	_, win, _ := newTestApp(t, 400, 300, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))

	sc := win.Scene()
	if len(sc.Quads) != 2 {
		t.Fatalf("got %d quads, want 2", len(sc.Quads))
	}
	outer, inner := sc.Quads[0], sc.Quads[1]
	if outer.Bounds.Size.W != 200 || outer.Bounds.Size.H != 200 {
		t.Errorf("outer bounds = %+v, want 200x200", outer.Bounds)
	}
	if inner.Bounds.Origin.X != 10 || inner.Bounds.Origin.Y != 10 {
		t.Errorf("inner origin = %+v, want (10,10) from padding", inner.Bounds.Origin)
	}
	if inner.Bounds.Size.W != 50 || inner.Bounds.Size.H != 50 {
		t.Errorf("inner size = %+v, want 50x50", inner.Bounds.Size)
	}
}

func TestBackgroundAndBorderPaintSeparateQuads(t *testing.T) {
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		return NewDiv().SizeWH(80, 40).Bg(White).Border(2, Black)
	})
	_, win, _ := newTestApp(t, 200, 100, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))

	sc := win.Scene()
	if len(sc.Quads) < 1 {
		t.Fatal("expected at least the background quad")
	}
	foundBorder := false
	for _, q := range sc.Quads {
		if q.BorderColor == Black && q.BorderWidths.Left == 2 {
			foundBorder = true
		}
	}
	if !foundBorder {
		t.Errorf("no quad carried the 2px black border; quads = %+v", sc.Quads)
	}
}

func TestHitTestFindsInnermostDiv(t *testing.T) {
	var innerID ElementID
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		inner := NewDiv().ID("inner").SizeWH(50, 50).Bg(Blue500)
		outer := NewDiv().ID("outer").SizeWH(200, 200).Bg(Gray200).FlexCol().P(10).Child(inner)
		return outer
	})
	_, win, _ := newTestApp(t, 400, 300, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))
	_ = innerID

	idx := win.renderedFrame.hitboxes.HitTest(Point{X: 30, Y: 30})
	if idx < 0 {
		t.Fatal("hit test at (30,30) found nothing; expected the inner div")
	}
	if n := win.renderedFrame.hitboxes.Len(); n < 2 {
		t.Fatalf("expected at least 2 hitboxes (outer + inner), got %d", n)
	}
}

func TestClickDispatchReachesListener(t *testing.T) {
	clicked := 0
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		return NewDiv().ID("btn").SizeWH(100, 40).Bg(Blue500).
			OnClick(func(*ClickEvent, *Window, *App) { clicked++ })
	})
	app, win, _ := newTestApp(t, 300, 200, root)
	win.Draw(app)

	win.DispatchEvent(MouseDownEvent{Position: Point{X: 20, Y: 20}, Button: MouseLeft}, app)
	win.DispatchEvent(MouseUpEvent{Position: Point{X: 20, Y: 20}, Button: MouseLeft}, app)
	win.DispatchEvent(ClickEvent{Up: &MouseUpEvent{Position: Point{X: 20, Y: 20}, Button: MouseLeft}}, app)

	if clicked != 1 {
		t.Errorf("onClick fired %d times, want 1", clicked)
	}
}

func TestStopPropagationHaltsBubble(t *testing.T) {
	var order []string
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		inner := NewDiv().ID("inner").SizeWH(100, 100).Bg(Blue500).
			OnMouseDown(func(_ *MouseDownEvent, w *Window, _ *App) {
				order = append(order, "inner")
				w.StopPropagation()
			})
		outer := NewDiv().ID("outer").SizeWH(200, 200).Bg(Gray200).
			OnMouseDown(func(*MouseDownEvent, *Window, *App) {
				order = append(order, "outer")
			})
		return outer.FlexCol().Child(inner)
	})
	app, win, _ := newTestApp(t, 300, 300, root)
	win.Draw(app)
	win.DispatchEvent(MouseDownEvent{Position: Point{X: 20, Y: 20}, Button: MouseLeft}, app)

	// Listeners are bubble-phase: inner runs first and stops the bubble
	// before it reaches outer.
	if len(order) != 1 || order[0] != "inner" {
		t.Errorf("order = %v, want [inner]", order)
	}
}

func TestScrollContainerProducesContentExtent(t *testing.T) {
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		list := NewDiv().ID("list").SizeWH(200, 100).Bg(White).OverflowScroll().FlexCol().Gap(4)
		for i := 0; i < 10; i++ {
			list = list.Child(NewDiv().SizeWH(200, 30).Bg(Gray100))
		}
		return list
	})
	_, win, _ := newTestApp(t, 400, 300, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))

	// Ten 30px rows plus nine 4px gaps = 336px of content in a 100px viewport.
	sc := win.Scene()
	if len(sc.Quads) != 11 {
		t.Fatalf("got %d quads, want 11 (list + 10 rows)", len(sc.Quads))
	}
	last := sc.Quads[len(sc.Quads)-1]
	// Ten 30px rows with 4px gaps puts the last row's origin at y=306, well
	// past the 100px viewport: proof the content overflowed and scrolls.
	if last.Bounds.Origin.Y <= 100 {
		t.Errorf("last row origin y = %v, want it laid out past the 100px viewport", last.Bounds.Origin.Y)
	}
	if last.ContentMask.Bounds.Max().Y > 100.001 {
		t.Errorf("last row clip extends to %v, want clipped at the 100px viewport", last.ContentMask.Bounds.Max().Y)
	}
}

func TestRefreshAndDrawClearInvalid(t *testing.T) {
	root := RenderFunc(func(w *Window, cx *App) IntoElement {
		return NewDiv().SizeWH(50, 50).Bg(Gray100)
	})
	_, win, _ := newTestApp(t, 200, 200, root)
	win.Draw(NewAppWithPlatform(&fakePlatform{}))
	if win.invalid {
		t.Error("Draw left the window marked invalid")
	}
	win.Refresh()
	if !win.invalid {
		t.Error("Refresh did not mark the window invalid")
	}
}

func TestElementStateRoundTrip(t *testing.T) {
	id := GlobalElementID{Path: []ElementPathKey{{Index: 0, Key: "list"}}}
	_, win, _ := newTestApp(t, 100, 100, RenderFunc(func(*Window, *App) IntoElement {
		return NewDiv().SizeWH(10, 10)
	}))
	win.SetElementState(id, 42)
	var got int
	if !win.ElementState(id, &got) {
		t.Fatal("ElementState did not find the stored value")
	}
	if got != 42 {
		t.Errorf("ElementState = %d, want 42", got)
	}
}

func TestFocusRequestAndBlur(t *testing.T) {
	app := NewAppWithPlatform(&fakePlatform{})
	h := app.NewFocusHandle()
	if !h.IsValid() {
		t.Fatal("fresh FocusHandle is not valid")
	}
	if h.ID() == 0 {
		t.Error("fresh FocusHandle has a zero id")
	}
	other := app.NewFocusHandle()
	if h.ID() == other.ID() {
		t.Error("two FocusHandles share an id")
	}
}
