package gpui_test

import (
	"math"
	"testing"

	"go-draw/internal/gpui"
)

func TestBuildTimeObserveAndOnReleaseSurviveConstruction(t *testing.T) {
	app := gpui.NewApp()
	other := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	var fired, released int
	ent := gpui.NewEntity(app, func(cx *gpui.Context[relCounter]) relCounter {
		cx.Observe(other, func(gpui.Entity[relCounter], *gpui.App) { fired++ })
		cx.OnRelease(func(*gpui.App) { released++ })
		return relCounter{N: 7}
	})
	if st := ent.Read(app); st == nil || st.N != 7 {
		t.Fatalf("Read after build = %v, want N=7", st)
	}
	app.Update(func(a *gpui.App) { a.Notify(other.ID()) })
	if fired != 1 {
		t.Fatalf("build-time observer fired %d times, want 1", fired)
	}
	ent.Release(app)
	if released != 1 {
		t.Fatalf("build-time OnRelease ran %d times, want 1", released)
	}
	app.Update(func(a *gpui.App) { a.Notify(other.ID()) })
	if fired != 1 {
		t.Fatalf("observer owned by a released entity kept firing (%d)", fired)
	}
}

func TestReleaseInsideObserverWaitsForFlushEnd(t *testing.T) {
	app := gpui.NewApp()
	ent := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	other := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	aliveInside := false
	gpui.ObserveOn(app, other, func(gpui.Entity[relCounter], *gpui.App) {
		ent.Release(app)
		aliveInside = ent.Read(app) != nil
	})
	app.Update(func(a *gpui.App) { a.Notify(other.ID()) })
	if !aliveInside {
		t.Fatal("Release inside an observer freed the entity before the flush ended")
	}
	if ent.Read(app) != nil {
		t.Fatal("entity still alive after the flush")
	}
}

func TestReleaseInsideEventHandlerWaitsForDispatchEnd(t *testing.T) {
	var app *gpui.App
	var ent gpui.Entity[relCounter]
	aliveInside := false
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().OnMouseDown(func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) {
			ent.Release(app)
			aliveInside = ent.Read(app) != nil
		})
	})
	hw, a := openHeadless(t, 100, 100, root)
	app = a
	ent = gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	hw.Window().DispatchEvent(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft, ClickCount: 1}, app)
	if !aliveInside {
		t.Fatal("Release inside an event handler freed the entity mid-dispatch")
	}
	if ent.Read(app) != nil {
		t.Fatal("entity still alive after dispatch")
	}
}

func TestMouseDownHandlerFocusBeatsAutoFocus(t *testing.T) {
	var hA, hB *gpui.FocusHandle
	root := gpui.RenderFunc(func(_ *gpui.Window, cx *gpui.App) gpui.IntoElement {
		if hA == nil {
			hA, hB = cx.NewFocusHandle(), cx.NewFocusHandle()
		}
		return gpui.NewDiv().SizeFull().TrackFocus(hA).Child(
			gpui.NewDiv().SizeWH(50, 50).OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { hB.Focus(w) }),
			gpui.NewDiv().SizeWH(10, 10).TrackFocus(hB),
		)
	})
	hw, _ := openHeadless(t, 100, 100, root)
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft, ClickCount: 1})
	if f := hw.Window().Focus(); f == nil || f.ID() != hB.ID() {
		t.Fatalf("focus = %v, want the handle focused by OnMouseDown (%v)", f.ID(), hB.ID())
	}
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 80, Y: 80}, Button: gpui.MouseLeft, ClickCount: 1})
	if f := hw.Window().Focus(); f == nil || f.ID() != hA.ID() {
		t.Fatal("auto-focus no longer works without a handler")
	}
}

func TestUniformListInheritsTextStyle(t *testing.T) {
	rows := func(start, end int, _ *gpui.Window, _ *gpui.App) []gpui.IntoElement {
		var out []gpui.IntoElement
		for i := start; i < end; i++ {
			out = append(out, gpui.NewText("row"))
		}
		return out
	}
	listHeight := func(root gpui.EntityRoot) gpui.Pixels {
		t.Helper()
		_, win := drawTextRoot(t, 400, 300, root, false)
		for _, q := range win.Scene().Quads {
			if q.Fill == gpui.Red500 {
				return q.Bounds.Size.H
			}
		}
		t.Fatal("list quad not found")
		return 0
	}
	inherited := listHeight(gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).TextSize(30).Child(gpui.NewUniformList("l", 3, rows).Bg(gpui.Red500))
	}))
	if math.Abs(float64(inherited-108)) > 0.01 {
		t.Errorf("list in a TextSize(30) div: height %v, want 108 (3 rows of 36)", inherited)
	}
	own := listHeight(gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		u := gpui.NewUniformList("l", 3, rows).Bg(gpui.Red500)
		u.StyledMixin.TextSize(30)
		return gpui.NewDiv().SizeWH(400, 300).Child(u)
	}))
	if math.Abs(float64(own-108)) > 0.01 {
		t.Errorf("list with TextSize(30): height %v, want 108", own)
	}

	if !requireFontForShaper(t) {
		return
	}
	sp := spritesOf(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(gpui.NewUniformList("l", 3, rows).TextColor(gpui.Red500))
	}))
	if len(sp) == 0 {
		t.Fatal("rows painted no glyphs")
	}
	for i, s := range sp {
		if s.Color != gpui.Red500 {
			t.Fatalf("row sprite %d color %v, want the list's Red500", i, s.Color)
		}
	}
}

// tsFrame is a custom container whose style carries a text style.
type tsFrame struct {
	style    gpui.Style
	children []gpui.AnyElement
}

func (f *tsFrame) IntoElement() gpui.Element { return tsFrameElement{f} }

type tsFrameElement struct{ f *tsFrame }

func (e tsFrameElement) ID() (gpui.ElementID, bool) { return gpui.ElementID{}, false }
func (e tsFrameElement) RequestLayout(w *gpui.Window, cx *gpui.App) (gpui.LayoutID, any) {
	return w.RequestLayoutWithChildren(e.f.style, e.f.children, cx)
}
func (e tsFrameElement) Prepaint(*gpui.Window, *gpui.App, gpui.ElementID, gpui.Bounds, any) any {
	return nil
}
func (e tsFrameElement) Paint(*gpui.Window, *gpui.App, gpui.ElementID, gpui.Bounds, any, any) {}
func (e tsFrameElement) ElementChildren() []gpui.AnyElement                                   { return e.f.children }

func TestRequestLayoutWithChildrenPushesTextStyle(t *testing.T) {
	boxHeight := func(outer *gpui.Div, style gpui.Style) gpui.Pixels {
		t.Helper()
		f := &tsFrame{style: style, children: []gpui.AnyElement{
			gpui.NewAnyElement(gpui.NewDiv().Bg(gpui.Red500).Child(gpui.NewText("x"))),
		}}
		_, win := drawTextRoot(t, 400, 300, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			return outer.Child(f)
		}), false)
		for _, q := range win.Scene().Quads {
			if q.Fill == gpui.Red500 {
				return q.Bounds.Size.H
			}
		}
		t.Fatal("box not found")
		return 0
	}
	big := gpui.DefaultStyle()
	big.TextStyle.FontSize = 30
	if h := boxHeight(gpui.NewDiv().SizeWH(400, 300), big); math.Abs(float64(h-36)) > 0.01 {
		t.Errorf("text under a FontSize-30 custom container: height %v, want 36", h)
	}
	if h := boxHeight(gpui.NewDiv().SizeWH(400, 300).TextSize(30), gpui.DefaultStyle()); math.Abs(float64(h-36)) > 0.01 {
		t.Errorf("default-styled custom container broke inheritance: height %v, want 36", h)
	}
}

func TestHoverTextColorReachesChildText(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(200, 100).Child(
			gpui.NewDiv().ID("h").SizeWH(150, 60).TextColor(gpui.Green600).
				Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.TextColor(gpui.Red500).TextStrikethrough() }).
				Child(gpui.NewText("hi"), gpui.NewText("own").TextColor(gpui.Blue500)),
		)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	count := func() map[gpui.Rgba]int {
		m := map[gpui.Rgba]int{}
		for _, s := range hw.Window().Scene().Sprites {
			m[s.Color]++
		}
		return m
	}
	before := count()
	if before[gpui.Green600] == 0 || before[gpui.Blue500] == 0 || before[gpui.Red500] != 0 {
		t.Fatalf("unhovered colors = %v", before)
	}
	if n := len(hw.Window().Scene().Underlines); n != 0 {
		t.Fatalf("unhovered: %d strikethrough rules", n)
	}
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 10, Y: 10}})
	hw.Pixels()
	after := count()
	if after[gpui.Red500] != before[gpui.Green600] || after[gpui.Green600] != 0 {
		t.Fatalf("hovered colors = %v, want inherited text recoloured Red500", after)
	}
	if after[gpui.Blue500] != before[gpui.Blue500] {
		t.Fatalf("text with its own color was overridden: %v", after)
	}
	if n := len(hw.Window().Scene().Underlines); n == 0 {
		t.Fatal("hover strikethrough not painted")
	}
}

func TestTextOpacity(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	sp := spritesOf(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		tx := gpui.NewText("ab")
		tx.StyledMixin.Opacity(0.5)
		st := gpui.NewStyledText(gpui.TextRun{Text: "cd"})
		st.StyledMixin.Opacity(0.5)
		return gpui.NewDiv().SizeWH(400, 300).Child(tx, st)
	}))
	if len(sp) == 0 {
		t.Fatal("no sprites")
	}
	for i, s := range sp {
		if s.Color.A > 130 {
			t.Fatalf("sprite %d alpha %d, want ~128 (Opacity 0.5)", i, s.Color.A)
		}
	}
}

func TestScrollNonScrollAxisStaysPut(t *testing.T) {
	h := gpui.NewScrollHandle()
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(100, 100).OverflowYScroll().TrackScroll(h).
			Child(gpui.NewDiv().SizeWH(300, 300).FlexShrink(0))
	})
	hw, _ := openHeadless(t, 200, 200, root)
	h.SetOffset(gpui.Point{X: 50, Y: 50})
	hw.Window().Refresh()
	hw.Pixels()
	if off := h.Offset(); off.X != 0 || off.Y != 50 {
		t.Fatalf("offset = %v, want (0,50) for a Y-only container", off)
	}
	if m := h.MaxOffset(); m.W != 0 || m.H != 200 {
		t.Fatalf("max = %v, want (0,200)", m)
	}
	h.ScrollToItem(0)
	hw.Window().Refresh()
	hw.Pixels()
	if off := h.Offset(); off.X != 0 {
		t.Fatalf("ScrollToItem moved the X axis: %v", off)
	}
	child, ok := hw.Window().HitTest(gpui.Point{X: 10, Y: 10})
	if !ok || len(child.Path) < 1 {
		t.Fatal("no hit")
	}
	gid := gpui.GlobalElementID{Path: child.Path[:len(child.Path)-1]}
	hw.Window().SetScrollOffset(gid, gpui.Point{X: 40, Y: 10})
	if off := hw.Window().ScrollOffset(gid); off.X != 0 || off.Y != 10 {
		t.Fatalf("SetScrollOffset = %v, want (0,10)", off)
	}
}

func TestTextBaselineUsesHalfLeading(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	top := func(lh float32) gpui.Pixels {
		sp := spritesOf(t, gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			tx := gpui.NewText("x").TextSize(14)
			if lh > 0 {
				tx.TextLineHeight(lh)
			}
			return gpui.NewDiv().SizeWH(400, 300).Child(tx)
		}))
		return sp[0].Bounds.Origin.Y
	}
	got := float64(top(40) - top(0))
	want := (40 - 14*1.2) / 2
	if math.Abs(got-want) > 0.6 {
		t.Fatalf("glyph shift for line height 40 = %v, want %v (half-leading)", got, want)
	}
}
