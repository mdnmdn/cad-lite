package gpui_test

import (
	"fmt"
	"reflect"
	"testing"

	"cad-lite/internal/gpui"
)

func openHeadless(t *testing.T, w, h float32, root gpui.EntityRoot) (*gpui.HeadlessWindow, *gpui.App) {
	t.Helper()
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: w, Height: h}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	hw.Pixels()
	return hw, app
}

func click(hw *gpui.HeadlessWindow, p gpui.Point) {
	hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1}
	hw.Dispatch(up)
	hw.Dispatch(gpui.ClickEvent{Up: &up})
}

func checkLog(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("log = %v, want %v", got, want)
	}
}

// root (0,0 200x100) with children A at x 0..50 and B at x 50..100.
func siblingsRoot(log *[]string) gpui.EntityRoot {
	rec := func(name string) func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) {
		return func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) { *log = append(*log, name) }
	}
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(200, 100).FlexRow().OnMouseDown(rec("root")).
			Child(
				gpui.NewDiv().SizeWH(50, 50).OnMouseDown(rec("A")),
				gpui.NewDiv().SizeWH(50, 50).OnMouseDown(rec("B")),
			)
	})
}

func TestMouseDispatchFollowsHitPathOnce(t *testing.T) {
	var log []string
	hw, _ := openHeadless(t, 200, 100, siblingsRoot(&log))
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 75, Y: 25}, Button: gpui.MouseLeft})
	checkLog(t, log, "B", "root")
	log = nil
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 25, Y: 25}, Button: gpui.MouseLeft})
	checkLog(t, log, "A", "root")
	log = nil
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 150, Y: 75}, Button: gpui.MouseLeft})
	checkLog(t, log, "root")
}

func TestMouseDispatchStopPropagation(t *testing.T) {
	var log []string
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(200, 100).
			OnMouseDown(func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) { log = append(log, "root") }).
			Child(gpui.NewDiv().SizeWH(50, 50).
				OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
					log = append(log, "child")
					w.StopPropagation()
				}))
	})
	hw, _ := openHeadless(t, 200, 100, root)
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft})
	checkLog(t, log, "child")
}

type cInc struct{}

func (cInc) Name() string              { return "inc" }
func (cInc) Clone() gpui.Action        { return cInc{} }
func (cInc) Equals(o gpui.Action) bool { _, ok := o.(cInc); return ok }

type cDec struct{}

func (cDec) Name() string              { return "dec" }
func (cDec) Clone() gpui.Action        { return cDec{} }
func (cDec) Equals(o gpui.Action) bool { _, ok := o.(cDec); return ok }

// counterView mirrors examples/counter.
type counterView struct {
	count   int
	hovered string
}

func (c *counterView) button(id, label string, base gpui.Rgba, delta int) *gpui.Div {
	return gpui.NewDiv().ID(id).SizeWH(56, 56).Bg(base).Rounded(12).ItemsCenter().
		OnMouseMove(func(_ *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) { c.hovered = id }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { c.count += delta; w.Refresh() }).
		Child(gpui.NewText(label).TextSize(28).TextColor(gpui.White))
}

func (c *counterView) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().KeyContext("counter").FlexCol().SizeFull().Gap(20).ItemsCenter().
		OnAction(cInc{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { c.count++; w.Refresh() }).
		OnAction(cDec{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { c.count--; w.Refresh() }).
		Child(
			gpui.NewText("Counter").TextSize(20),
			gpui.NewText(fmt.Sprint(c.count)).TextSize(72),
			gpui.NewDiv().FlexRow().Gap(16).Child(
				c.button("dec", "-", gpui.Red500, -1),
				c.button("inc", "+", gpui.Blue500, 1),
			),
		)
}

func quadCenter(t *testing.T, hw *gpui.HeadlessWindow, fill gpui.Rgba) gpui.Point {
	t.Helper()
	for _, q := range hw.Window().Scene().Quads {
		if q.Background == gpui.BackgroundSolid && q.Fill == fill {
			return gpui.Point{X: q.Bounds.Origin.X + q.Bounds.Size.W/2, Y: q.Bounds.Origin.Y + q.Bounds.Size.H/2}
		}
	}
	t.Fatalf("no quad filled with %v", fill)
	return gpui.Point{}
}

func TestCounterClickPlusIncrementsOnce(t *testing.T) {
	c := &counterView{}
	hw, app := openHeadless(t, 360, 360, c)
	click(hw, quadCenter(t, hw, gpui.Blue500))
	if c.count != 1 {
		t.Fatalf("after clicking +: count = %d, want 1", c.count)
	}
	click(hw, quadCenter(t, hw, gpui.Red500))
	if c.count != 0 {
		t.Fatalf("after clicking -: count = %d, want 0", c.count)
	}

	app.Keymap().AddBindings(
		gpui.NewKeyBinding("up", cInc{}, "counter"),
		gpui.NewKeyBinding("down", cDec{}, "counter"),
	)
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "up"}})
	if c.count != 1 {
		t.Fatalf("after up: count = %d, want 1", c.count)
	}
}

func keyRec(log *[]string, name string) func(*gpui.KeyDownEvent, *gpui.Window, *gpui.App) {
	return func(*gpui.KeyDownEvent, *gpui.Window, *gpui.App) { *log = append(*log, name) }
}

func TestKeyDispatchFocusPath(t *testing.T) {
	var log []string
	var fh *gpui.FocusHandle
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		if fh == nil {
			fh = cx.NewFocusHandle()
		}
		return gpui.NewDiv().SizeWH(200, 100).OnKeyDown(keyRec(&log, "root")).
			Child(
				gpui.NewDiv().SizeWH(50, 50).OnKeyDown(keyRec(&log, "A")),
				gpui.NewDiv().SizeWH(50, 50).TrackFocus(fh).OnKeyDown(keyRec(&log, "B")),
			)
	})
	hw, _ := openHeadless(t, 200, 100, root)

	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "x"}})
	checkLog(t, log, "root")
	log = nil

	fh.Focus(hw.Window())
	hw.Pixels()
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "x"}})
	checkLog(t, log, "B", "root")
}

func TestKeyUpFocusPath(t *testing.T) {
	var log []string
	var fh *gpui.FocusHandle
	rec := func(name string) func(*gpui.KeyUpEvent, *gpui.Window, *gpui.App) {
		return func(*gpui.KeyUpEvent, *gpui.Window, *gpui.App) { log = append(log, name) }
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		if fh == nil {
			fh = cx.NewFocusHandle()
		}
		return gpui.NewDiv().SizeWH(200, 100).OnKeyUp(rec("root")).
			Child(
				gpui.NewDiv().SizeWH(50, 50).OnKeyUp(rec("A")).
					Child(gpui.NewDiv().SizeWH(10, 10).TrackFocus(fh).OnKeyUp(rec("A1"))),
				gpui.NewDiv().SizeWH(50, 50).OnKeyUp(rec("B")),
			)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	hw.Dispatch(gpui.KeyUpEvent{Keystroke: gpui.Keystroke{Key: "x"}})
	checkLog(t, log, "root")
	log = nil
	fh.Focus(hw.Window())
	hw.Pixels()
	hw.Dispatch(gpui.KeyUpEvent{Keystroke: gpui.Keystroke{Key: "x"}})
	checkLog(t, log, "A1", "A", "root")
}

func TestHitTestReturnsStableElementID(t *testing.T) {
	show := false
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		d := gpui.NewDiv().SizeWH(200, 100).FlexRow()
		if show {
			d.Child(gpui.NewDiv().ID("extra").SizeWH(20, 20))
		}
		return d.Child(gpui.NewDiv().ID("btn").SizeWH(50, 50))
	})
	hw, _ := openHeadless(t, 200, 100, root)
	win := hw.Window()
	id1, ok := win.HitTest(gpui.Point{X: 30, Y: 10})
	if !ok {
		t.Fatal("miss")
	}
	win.Refresh()
	hw.Pixels()
	id2, ok := win.HitTest(gpui.Point{X: 30, Y: 10})
	if !ok || !id1.Equal(id2) {
		t.Fatalf("id changed across identical frames: %v vs %v", id1, id2)
	}
	other, _ := win.HitTest(gpui.Point{X: 150, Y: 80})
	if other.Equal(id1) {
		t.Fatal("root and child share an id")
	}
}

func TestHoverEndUsesCurrentFrameCallback(t *testing.T) {
	gen := 0
	var log []string
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		g := gen
		return gpui.NewDiv().SizeWH(200, 100).Child(
			gpui.NewDiv().SizeWH(50, 50).
				OnHoverStart(func(*gpui.Window, *gpui.App) { log = append(log, fmt.Sprintf("start%d", g)) }).
				OnHoverEnd(func(*gpui.Window, *gpui.App) { log = append(log, fmt.Sprintf("end%d", g)) }))
	})
	hw, _ := openHeadless(t, 200, 100, root)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 10, Y: 10}})
	checkLog(t, log, "start0")
	log = nil
	gen = 1
	hw.Window().Refresh()
	hw.Pixels()
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 150, Y: 80}})
	checkLog(t, log, "end1")
}

func TestHoverRecomputedAfterDraw(t *testing.T) {
	show := false
	var log []string
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		d := gpui.NewDiv().SizeWH(200, 100).FlexRow().
			OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { show = !show; w.Refresh() })
		if show {
			d.Child(gpui.NewDiv().SizeWH(50, 50).
				OnHoverStart(func(*gpui.Window, *gpui.App) { log = append(log, "start") }).
				OnHoverEnd(func(*gpui.Window, *gpui.App) { log = append(log, "end") }))
		}
		return d
	})
	hw, _ := openHeadless(t, 200, 100, root)
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft})
	checkLog(t, log, "start")
	log = nil
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft})
	checkLog(t, log)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 150, Y: 80}})
	checkLog(t, log)
}

func TestHoverOrderLeavesFirst(t *testing.T) {
	var log []string
	hov := func(d *gpui.Div, name string) *gpui.Div {
		return d.OnHoverStart(func(*gpui.Window, *gpui.App) { log = append(log, name+"+") }).
			OnHoverEnd(func(*gpui.Window, *gpui.App) { log = append(log, name+"-") })
	}
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		c := hov(gpui.NewDiv().SizeWH(20, 20), "c")
		b := hov(gpui.NewDiv().SizeWH(40, 40), "b").Child(c)
		a := hov(gpui.NewDiv().SizeWH(60, 60), "a").Child(b)
		return gpui.NewDiv().SizeWH(200, 100).Child(a)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	for range 20 {
		log = nil
		hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 5, Y: 5}})
		checkLog(t, log, "a+", "b+", "c+")
		log = nil
		hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 150, Y: 80}})
		checkLog(t, log, "c-", "b-", "a-")
	}
}
