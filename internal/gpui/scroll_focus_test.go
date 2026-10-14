package gpui_test

import (
	"fmt"
	"testing"

	"go-draw/internal/gpui"
)

func wheel(x, y, dx, dy gpui.Pixels) gpui.ScrollWheelEvent {
	return gpui.ScrollWheelEvent{Position: gpui.Point{X: x, Y: y}, DeltaX: dx, DeltaY: dy}
}

func TestScrollAxesAndBubbling(t *testing.T) {
	outerH, innerH := gpui.NewScrollHandle(), gpui.NewScrollHandle()
	var clicked []string
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		row := gpui.NewDiv().ID("horizontal").SizeWH(100, 50).FlexShrink(0).OverflowXScroll().TrackScroll(innerH).FlexRow().
			Child(gpui.NewDiv().SizeWH(300, 50).Bg(gpui.Green600))
		col := gpui.NewDiv().ID("vertical").SizeWH(100, 100).OverflowYScroll().TrackScroll(outerH).FlexCol().
			Child(row, gpui.NewDiv().ID("tall").SizeWH(100, 400).Bg(gpui.Gray200).
				OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { clicked = append(clicked, "tall") }))
		return gpui.NewDiv().SizeFull().Child(col, gpui.NewDiv().ID("below").SizeWH(100, 100).
			OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { clicked = append(clicked, "below") }))
	})
	hw, _ := openBench(t, 200, 300, 1, root)
	_ = hw.Pixels()

	// Over the X-only row a vertical wheel scrolls horizontally.
	hw.Dispatch(wheel(50, 20, 0, 30))
	if o := innerH.Offset(); o.X != 30 || o.Y != 0 {
		t.Fatalf("inner offset = %v, want (30,0)", o)
	}
	if outerH.Offset() != (gpui.Point{}) {
		t.Fatalf("outer offset = %v, want unchanged", outerH.Offset())
	}
	// At the row's end the delta bubbles to the Y container.
	hw.Dispatch(wheel(50, 20, 0, 500))
	if o := innerH.Offset(); o.X != 200 {
		t.Fatalf("inner offset = %v, want clamped to 200", o)
	}
	hw.Dispatch(wheel(50, 20, 0, 40))
	if o := outerH.Offset(); o.Y != 40 || o.X != 0 {
		t.Fatalf("outer offset = %v, want (0,40)", o)
	}
	// A Y-only container ignores pure horizontal deltas.
	hw.Dispatch(wheel(50, 80, 25, 0))
	if o := outerH.Offset(); o.X != 0 || o.Y != 40 {
		t.Fatalf("outer offset after DeltaX = %v, want (0,40)", o)
	}
	if m := outerH.MaxOffset(); m.H != 350 || m.W != 0 {
		t.Fatalf("outer max = %v, want (0,350)", m)
	}

	// The tall child now extends below the container; its clipped-off part
	// must not be hit, the sibling below must.
	hw.Dispatch(gpui.ClickEvent{Up: &gpui.MouseUpEvent{Position: gpui.Point{X: 50, Y: 150}}})
	if len(clicked) != 1 || clicked[0] != "below" {
		t.Fatalf("clicked = %v, want [below]", clicked)
	}

	outerH.ScrollToBottom()
	hw.Window().Refresh()
	_ = hw.Pixels()
	if o := outerH.Offset(); o.Y != 350 {
		t.Fatalf("ScrollToBottom offset = %v, want 350", o)
	}
	outerH.ScrollToItem(0)
	hw.Window().Refresh()
	_ = hw.Pixels()
	if o := outerH.Offset(); o.Y != 0 {
		t.Fatalf("ScrollToItem(0) offset = %v, want 0", o)
	}
	outerH.SetOffset(gpui.Point{Y: 1e6})
	hw.Window().Refresh()
	_ = hw.Pixels()
	if o := outerH.Offset(); o.Y != 350 {
		t.Fatalf("SetOffset clamp = %v, want 350", o)
	}
}

func TestScrollbarOptIn(t *testing.T) {
	build := func(show bool) int {
		root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			d := gpui.NewDiv().ID("s").SizeWH(100, 100).OverflowYScroll().
				Child(gpui.NewDiv().SizeWH(100, 400).Bg(gpui.Gray200))
			if show {
				d.ShowScrollbar()
			}
			return d
		})
		hw, _ := openBench(t, 100, 100, 1, root)
		_ = hw.Pixels()
		return len(hw.Window().Scene().Quads)
	}
	if a, b := build(false), build(true); b != a+1 {
		t.Fatalf("quads without/with scrollbar = %d/%d, want one thumb more", a, b)
	}
}

func TestUniformListVirtualizes(t *testing.T) {
	var ranges [][2]int
	handle := gpui.NewScrollHandle()
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		list := gpui.NewUniformList("items", 10000, func(start, end int, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
			ranges = append(ranges, [2]int{start, end})
			out := make([]gpui.IntoElement, 0, end-start)
			for i := start; i < end; i++ {
				out = append(out, gpui.NewDiv().ID(fmt.Sprint(i)).SizeH(24).Bg(gpui.Gray100))
			}
			return out
		}).SizeH(300).TrackScroll(handle)
		return gpui.NewDiv().SizeFull().FlexCol().Child(list)
	})
	hw, _ := openBench(t, 200, 400, 1, root)
	_ = hw.Pixels()
	last := ranges[len(ranges)-1]
	if last != [2]int{0, 13} {
		t.Fatalf("visible range = %v (all calls %v), want [0,13)", last, ranges)
	}
	for _, r := range ranges {
		if r[1]-r[0] > 13 {
			t.Fatalf("render asked for %v: not virtualized", r)
		}
	}
	if m := handle.MaxOffset(); m.H != 24*10000-300 {
		t.Fatalf("max offset = %v, want %v", m.H, 24*10000-300)
	}
	if n := len(hw.Window().Scene().Quads); n != 13 {
		t.Fatalf("painted %d rows, want 13", n)
	}

	ranges = nil
	hw.Dispatch(wheel(50, 50, 0, 2400))
	if last := ranges[len(ranges)-1]; last != [2]int{100, 113} {
		t.Fatalf("after scrolling 2400px range = %v, want [100,113)", last)
	}
	q := hw.Window().Scene().Quads[0]
	if q.Bounds.Origin.Y != 0 || q.Bounds.Size.H != 24 {
		t.Fatalf("first row bounds = %v, want at y=0, 24 tall", q.Bounds)
	}

	handle.ScrollToItem(5000)
	hw.Window().Refresh()
	_ = hw.Pixels()
	if o := handle.Offset().Y; o != 5001*24-300 {
		t.Fatalf("ScrollToItem(5000) offset = %v, want %v", o, 5001*24-300)
	}
}

func TestUniformListAutoHeight(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		list := gpui.NewUniformList("l", 5, func(start, end int, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
			var out []gpui.IntoElement
			for i := start; i < end; i++ {
				out = append(out, gpui.NewDiv().SizeH(20).Bg(gpui.Gray300))
			}
			return out
		}).P(4).Bg(gpui.White)
		return gpui.NewDiv().SizeFull().FlexCol().ItemsStart().Child(list)
	})
	hw, _ := openBench(t, 200, 400, 1, root)
	_ = hw.Pixels()
	sc := hw.Window().Scene()
	if len(sc.Quads) != 6 {
		t.Fatalf("quads = %d, want list bg + 5 rows", len(sc.Quads))
	}
	if h := sc.Quads[0].Bounds.Size.H; h != 108 {
		t.Fatalf("auto list height = %v, want 5*20+8", h)
	}
	if r := sc.Quads[5].Bounds; r.Origin.Y != 84 || r.Origin.X != 4 {
		t.Fatalf("last row = %v, want at (4,84)", r)
	}
}

type tabNext struct{ gpui.Action }
type tabPrev struct{ gpui.Action }

func TestTabTraversalAndFocusVisible(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	app.Keymap().AddBindings(
		gpui.NewKeyBinding("tab", tabNext{gpui.NewAction("tab")}, ""),
		gpui.NewKeyBinding("shift-tab", tabPrev{gpui.NewAction("tab-prev")}, ""),
	)
	rootFocus := app.NewFocusHandle()
	items := []*gpui.FocusHandle{
		app.NewFocusHandle().SetTabIndex(1).SetTabStop(true),
		app.NewFocusHandle().SetTabIndex(3).SetTabStop(true),
		app.NewFocusHandle(), // not a tab stop
		app.NewFocusHandle().SetTabIndex(2).SetTabStop(true),
	}
	green := gpui.RgbaOf(16, 185, 129, 255)
	yellow := gpui.RgbaOf(251, 191, 36, 255)
	err := app.OpenWindow(gpui.WindowOptions{Width: 300, Height: 300}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			root := gpui.NewDiv().TrackFocus(rootFocus).SizeFull().FlexCol().Gap(4).Bg(gpui.White).
				OnAction(tabNext{}, func(gpui.Action, *gpui.Window, *gpui.App) { w.FocusNext() }).
				OnAction(tabPrev{}, func(gpui.Action, *gpui.Window, *gpui.App) { w.FocusPrev() })
			for _, h := range items {
				root.Child(gpui.NewDiv().TrackFocus(h).SizeH(20).Bg(gpui.Gray200).
					Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(4, yellow) }).
					FocusVisible(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(4, green) }))
			}
			// Implicit focus handle from TabIndex alone.
			root.Child(gpui.NewDiv().ID("btn").TabIndex(4).SizeH(20).Bg(gpui.Gray200))
			return root
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	win := hw.Window()
	rootFocus.Focus(win)
	_ = hw.Pixels()

	key := func(s string) {
		ks, err := gpui.ParseKeystroke(s)
		if err != nil {
			t.Fatal(err)
		}
		hw.Dispatch(gpui.KeyDownEvent{Keystroke: ks})
	}
	focusedIndex := func() int {
		for i, h := range items {
			if h.IsFocused(win) {
				return i
			}
		}
		return -1
	}
	borderOf := func(i int) gpui.Rgba {
		c := hw.Image().NRGBAAt(1, i*24+10)
		return gpui.RgbaOf(c.R, c.G, c.B, c.A)
	}

	key("tab")
	if focusedIndex() != 0 {
		t.Fatalf("after tab focused = %d, want item 0 (tab index 1)", focusedIndex())
	}
	if !items[0].IsFocusVisible(win) || borderOf(0) != green {
		t.Fatalf("keyboard focus: visible=%v border=%v, want focus-visible green", items[0].IsFocusVisible(win), borderOf(0))
	}
	key("tab")
	if focusedIndex() != 3 {
		t.Fatalf("second tab focused = %d, want item 3 (tab index 2)", focusedIndex())
	}
	key("tab")
	if focusedIndex() != 1 {
		t.Fatalf("third tab focused = %d, want item 1 (tab index 3)", focusedIndex())
	}
	key("tab")
	if focusedIndex() != -1 || win.Focus() == nil || win.Focus().ID() == rootFocus.ID() {
		t.Fatalf("fourth tab should focus the implicit TabIndex(4) handle")
	}
	key("tab")
	if focusedIndex() != 0 {
		t.Fatalf("tab should wrap to item 0, got %d", focusedIndex())
	}
	key("shift-tab")
	if win.Focus() == nil || focusedIndex() != -1 {
		t.Fatalf("shift-tab should wrap back to the implicit handle")
	}

	// A click focuses the innermost focusable element without focus-visible.
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 50, Y: 2*24 + 10}, Button: gpui.MouseLeft})
	if focusedIndex() != 2 {
		t.Fatalf("click focused = %d, want item 2", focusedIndex())
	}
	if win.FocusVisible() || items[2].IsFocusVisible(win) {
		t.Fatal("mouse focus must not be focus-visible")
	}
	if borderOf(2) != yellow {
		t.Fatalf("mouse-focused border = %v, want the Focus (yellow) style", borderOf(2))
	}
}
