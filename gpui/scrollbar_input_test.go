package gpui_test

import (
	"fmt"
	"go-gpui/gpui"
	"math"
	"testing"
)

func scrollbarMove(hw *gpui.HeadlessWindow, x, y gpui.Pixels) {
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: x, Y: y}})
}
func scrollbarWant(t *testing.T, h *gpui.ScrollHandle, x, y gpui.Pixels) {
	t.Helper()
	p := h.Offset()
	if math.Abs(float64(p.X-x)) > .02 || math.Abs(float64(p.Y-y)) > .02 {
		t.Fatalf("offset=%v want (%v,%v)", p, x, y)
	}
}

func TestScrollbarDragBothAxesAndOutsideRelease(t *testing.T) {
	for _, axes := range []gpui.Axes{gpui.AxisX, gpui.AxisY, gpui.AxesBoth} {
		h := gpui.NewScrollHandle()
		root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			cw, ch := float32(100), float32(100)
			if axes.Has(gpui.AxisX) {
				cw = 400
			}
			if axes.Has(gpui.AxisY) {
				ch = 400
			}
			d := gpui.NewDiv().ID("scroll").SizeWH(100, 100).OverflowScroll().TrackScroll(h).ShowScrollbar().Child(gpui.NewDiv().SizeWH(cw, ch).Bg(gpui.Blue500))
			d.StyleRef().ScrollAxes = &axes
			return d
		})
		hw, _ := openHeadless(t, 200, 200, root)
		travel := gpui.Pixels(72)
		if axes == gpui.AxesBoth {
			travel = 66
		}
		if axes.Has(gpui.AxisY) {
			activeDown(hw, 95, 10, gpui.MouseRight)
			scrollbarMove(hw, 95, 46)
			scrollbarWant(t, h, 0, 0)
			activeDown(hw, 95, 10, gpui.MouseLeft)
			activeUp(hw, 95, 10, gpui.MouseRight)
			scrollbarMove(hw, 150, 10+travel/2)
			scrollbarWant(t, h, 0, 150)
			scrollbarMove(hw, 150, 1000)
			scrollbarWant(t, h, 0, 300)
			activeUp(hw, 150, 1000, gpui.MouseLeft)
			scrollbarMove(hw, 95, 5)
			scrollbarWant(t, h, 0, 300)
			h.ScrollToTop()
			hw.Window().Refresh()
			hw.Image()
		}
		if axes.Has(gpui.AxisX) {
			activeDown(hw, 10, 95, gpui.MouseLeft)
			scrollbarMove(hw, 10+travel/2, 150)
			scrollbarWant(t, h, 150, 0)
			scrollbarMove(hw, -1000, 150)
			scrollbarWant(t, h, 0, 0)
			activeUp(hw, -1000, 150, gpui.MouseLeft)
		}
	}
}

func TestScrollbarTrackPagingConsumesGesture(t *testing.T) {
	h := gpui.NewScrollHandle()
	down, up, move, clicks := 0, 0, 0, 0
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		child := gpui.NewDiv().ID("content").TabIndex(0).SizeWH(100, 400).Bg(gpui.Blue500).
			Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) }).
			OnMouseDown(func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) { down++ }).
			OnMouseUp(func(*gpui.MouseUpEvent, *gpui.Window, *gpui.App) { up++ }).
			OnMouseMove(func(*gpui.MouseMoveEvent, *gpui.Window, *gpui.App) { move++ }).
			OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { clicks++ })
		return gpui.NewDiv().ID("scroll").SizeWH(100, 100).OverflowYScroll().TrackScroll(h).ShowScrollbar().Child(child)
	})
	hw, _ := openHeadless(t, 200, 200, root)
	click(hw, gpui.Point{X: 95, Y: 80})
	scrollbarWant(t, h, 0, 100)
	if down+up+move+clicks != 0 || hw.Window().Focus() != nil {
		t.Fatalf("scrollbar leaked handlers/focus: %d,%d,%d,%d focus=%v", down, up, move, clicks, hw.Window().Focus())
	}
	activePixel(t, hw, 20, 20, gpui.Blue500)
	click(hw, gpui.Point{X: 95, Y: 3})
	scrollbarWant(t, h, 0, 0)
	h.ScrollToBottom()
	hw.Window().Refresh()
	hw.Image()
	click(hw, gpui.Point{X: 95, Y: 50})
	scrollbarWant(t, h, 0, 200)
	activeDown(hw, 95, 70, gpui.MouseLeft)
	scrollbarMove(hw, 180, 180)
	activeUp(hw, 180, 180, gpui.MouseLeft)
	// An outside release emits no platform click; the next content click must work.
	click(hw, gpui.Point{X: 20, Y: 20})
	if clicks != 1 || down != 1 || up != 1 {
		t.Fatalf("next content click poisoned: %d,%d,%d", down, up, clicks)
	}
}

func TestScrollbarOverlayOrderingClippingAndNesting(t *testing.T) {
	outer, inner := gpui.NewScrollHandle(), gpui.NewScrollHandle()
	cover := true
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		child := gpui.NewDiv().ID("inner").SizeWH(80, 80).FlexShrink(0).OverflowYScroll().TrackScroll(inner).ShowScrollbar().Child(gpui.NewDiv().SizeWH(80, 300))
		sc := gpui.NewDiv().ID("outer").SizeWH(100, 100).OverflowYScroll().TrackScroll(outer).ShowScrollbar().Child(child, gpui.NewDiv().SizeWH(100, 300))
		r := gpui.NewDiv().Child(sc)
		if cover {
			r.Child(gpui.NewDiv().ID("cover").Absolute().Left(90).SizeWH(10, 100))
		}
		return r
	})
	hw, _ := openHeadless(t, 200, 200, root)
	click(hw, gpui.Point{X: 95, Y: 80})
	scrollbarWant(t, outer, 0, 0)
	cover = false
	hw.Window().Refresh()
	hw.Image()
	click(hw, gpui.Point{X: 75, Y: 65})
	scrollbarWant(t, inner, 0, 80)
	scrollbarWant(t, outer, 0, 0)
	click(hw, gpui.Point{X: 95, Y: 80})
	scrollbarWant(t, outer, 0, 100)
	h := gpui.NewScrollHandle()
	clipped := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(90, 50).OverflowHidden().Child(gpui.NewDiv().ID("clipped").SizeWH(100, 100).OverflowYScroll().ShowScrollbar().TrackScroll(h).Child(gpui.NewDiv().SizeWH(100, 400)))
	})
	hw, _ = openHeadless(t, 200, 200, clipped)
	click(hw, gpui.Point{X: 95, Y: 30})
	scrollbarWant(t, h, 0, 0)
}

func TestScrollbarSmallTracksAndNoOverflow(t *testing.T) {
	for _, height := range []float32{3, 10, 100} {
		h := gpui.NewScrollHandle()
		contentHeight := float32(1000)
		if height == 100 {
			contentHeight = 100
		}
		root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().ID("s").SizeWH(30, height).OverflowYScroll().ShowScrollbar().TrackScroll(h).Child(gpui.NewDiv().SizeWH(30, contentHeight))
		})
		hw, _ := openHeadless(t, 100, 100, root)
		activeDown(hw, 25, 3, gpui.MouseLeft)
		scrollbarMove(hw, 25, 80)
		activeUp(hw, 25, 80, gpui.MouseLeft)
		scrollbarWant(t, h, 0, 0)
	}
	h := gpui.NewScrollHandle()
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().ID("s").SizeWH(100, 100).OverflowYScroll().ShowScrollbar().TrackScroll(h).Child(gpui.NewDiv().SizeWH(100, 10000))
	})
	hw, _ := openHeadless(t, 200, 200, root)
	activeDown(hw, 95, 10, gpui.MouseLeft)
	scrollbarMove(hw, 95, 48)
	scrollbarWant(t, h, 0, 4950)
}

func TestScrollbarCaptureLifecycleAndProgrammaticOffset(t *testing.T) {
	for _, change := range []string{"id", "hidden", "removed", "show", "size", "content", "handle", "window"} {
		t.Run(change, func(t *testing.T) {
			h := gpui.NewScrollHandle()
			id := "scroll"
			hidden, removed, show := false, false, true
			width, content := float32(100), float32(400)
			root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
				d := gpui.NewDiv().ID(id).SizeWH(width, 100).OverflowYScroll().TrackScroll(h).Child(gpui.NewDiv().SizeWH(width, content))
				if hidden {
					d.Hidden()
				}
				if show {
					d.ShowScrollbar()
				}
				r := gpui.NewDiv()
				if !removed {
					r.Child(d)
				}
				return r
			})
			hw, _ := openHeadless(t, 200, 200, root)
			activeDown(hw, 95, 10, gpui.MouseLeft)
			switch change {
			case "id":
				id = "replacement"
			case "hidden":
				hidden = true
			case "removed":
				removed = true
			case "show":
				show = false
			case "size":
				width = 110
			case "content":
				content = 500
			case "handle":
				h.SetOffset(gpui.Point{Y: 40})
			case "window":
				gid, _ := hw.Window().HitTest(gpui.Point{X: 20, Y: 20})
				gid.Path = gid.Path[:len(gid.Path)-1]
				hw.Window().SetScrollOffset(gid, gpui.Point{Y: 40})
			}
			hw.Window().Refresh()
			hw.Image()
			before := h.Offset()
			scrollbarMove(hw, 95, 60)
			if h.Offset() != before {
				t.Fatalf("cancelled capture scrolled: before=%v after=%v", before, h.Offset())
			}
			activeUp(hw, 95, 60, gpui.MouseLeft)
		})
	}
}

func TestScrollbarUniformListDragVirtualizes(t *testing.T) {
	h := gpui.NewScrollHandle()
	var lastStart, lastEnd int
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewUniformList("items", 1000, func(start, end int, _ *gpui.Window, _ *gpui.App) []gpui.IntoElement {
			lastStart, lastEnd = start, end
			out := make([]gpui.IntoElement, 0, end-start)
			for i := start; i < end; i++ {
				out = append(out, gpui.NewDiv().ID(fmt.Sprint(i)).SizeH(20).Bg(gpui.Blue500))
			}
			return out
		}).SizeWH(100, 100).TrackScroll(h).ShowScrollbar()
	})
	hw, _ := openHeadless(t, 200, 200, root)
	activeDown(hw, 95, 10, gpui.MouseLeft)
	scrollbarMove(hw, 95, 48)
	scrollbarWant(t, h, 0, 9950)
	if lastStart != 497 || lastEnd-lastStart > 6 {
		t.Fatalf("virtual range [%d,%d)", lastStart, lastEnd)
	}
	activeUp(hw, 150, 48, gpui.MouseLeft)
	h.ScrollToItem(900)
	hw.Window().Refresh()
	hw.Image()
	if h.Offset().Y < 17900 {
		t.Fatalf("programmatic after drag=%v", h.Offset())
	}
}

func TestScrollbarHorizontalPagingAndReleaseClickMatch(t *testing.T) {
	h := gpui.NewScrollHandle()
	clicks := 0
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().ID("s").SizeWH(100, 100).OverflowXScroll().TrackScroll(h).ShowScrollbar().Child(
			gpui.NewDiv().SizeWH(400, 100).OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { clicks++ }),
		)
	})
	hw, _ := openHeadless(t, 200, 200, root)
	click(hw, gpui.Point{X: 80, Y: 95})
	scrollbarWant(t, h, 100, 0)
	click(hw, gpui.Point{X: 3, Y: 95})
	scrollbarWant(t, h, 0, 0)
	activeDown(hw, 10, 95, gpui.MouseLeft)
	activeUp(hw, 150, 150, gpui.MouseLeft)
	unrelated := gpui.MouseUpEvent{Position: gpui.Point{X: 20, Y: 20}, Button: gpui.MouseLeft}
	hw.Dispatch(gpui.ClickEvent{Up: &unrelated})
	if clicks != 1 {
		t.Fatalf("unrelated click swallowed: clicks=%d", clicks)
	}
}
