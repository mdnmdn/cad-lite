package gpui_test

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"cad-lite/internal/gpui"
)

// listRig is a List of items whose heights come from heights[ix]; every
// render call is counted per frame.
type listRig struct {
	state    *gpui.ListState
	handle   *gpui.ScrollHandle
	heights  []float32
	keys     []uint64
	calls    map[int]int
	clicked  []int
	width    float32
	overdraw float32
	gap, pad float32
}

func newListRig(n int, h func(ix int) float32) *listRig {
	r := &listRig{state: gpui.NewListState(), handle: gpui.NewScrollHandle(), heights: make([]float32, n), keys: make([]uint64, n),
		calls: map[int]int{}, width: 200}
	for i := range r.heights {
		r.heights[i] = h(i)
	}
	return r
}

func (r *listRig) root() gpui.EntityRoot {
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		l := gpui.NewList("items", r.state, len(r.heights), func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
			r.calls[ix]++
			ix0 := ix
			return gpui.NewDiv().ID(fmt.Sprint("item-", ix)).SizeH(r.heights[ix]).Bg(gpui.Gray200).
				OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { r.clicked = append(r.clicked, ix0) })
		}).ItemKey(func(ix int) uint64 { return r.keys[ix] }).Overdraw(r.overdraw).TrackScroll(r.handle).SizeWH(r.width, 300)
		if r.gap > 0 {
			l.Gap(r.gap)
		}
		if r.pad > 0 {
			l.P(r.pad)
		}
		return gpui.NewDiv().SizeFull().FlexCol().ItemsStart().Child(l)
	})
}

// frame redraws and returns the items rendered in that frame.
func (r *listRig) frame(hw *gpui.HeadlessWindow) map[int]int {
	r.calls = map[int]int{}
	hw.Window().Refresh()
	_ = hw.Pixels()
	return r.calls
}

// itemQuad returns the painted bounds of item ix (its background quad),
// matched by height and order; items paint in index order after the list.
func (r *listRig) itemBounds(t *testing.T, hw *gpui.HeadlessWindow, ix int) gpui.Bounds {
	t.Helper()
	start, end := r.state.VisibleRange()
	if ix < start || ix >= end {
		t.Fatalf("item %d not built (range [%d,%d))", ix, start, end)
	}
	q := hw.Window().Scene().Quads
	return q[ix-start].Bounds
}

func sumHeights(h []float32, gap float32) float32 {
	var s float32
	for i, v := range h {
		if i > 0 {
			s += gap
		}
		s += v
	}
	return s
}

func mixed(ix int) float32 { return float32(20 + (ix%5)*15) } // 20, 35, 50, 65, 80

func TestListBuildsOnlyVisibleItems(t *testing.T) {
	r := newListRig(2000, mixed)
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	// Top: 20+35+50+65+80 = 250, then 20+35 reaches 305 >= 300.
	if s, e := r.state.VisibleRange(); s != 0 || e != 7 {
		t.Fatalf("top range = [%d,%d), want [0,7)", s, e)
	}
	// The first frame starts from the estimate (24px), so it may build the
	// items 300/24 would show; never more.
	for ix := range r.calls {
		if ix >= 13 {
			t.Fatalf("item %d rendered at the top: not virtualized (calls %v)", ix, r.calls)
		}
	}
	for ix := range r.frame(hw) {
		if ix >= 7 {
			t.Fatalf("item %d rendered in a steady frame (calls %v)", ix, r.calls)
		}
	}
	if n := len(hw.Window().Scene().Quads); n != 7 {
		t.Fatalf("painted %d items, want 7", n)
	}
	if b := r.itemBounds(t, hw, 3); b.Origin.Y != 105 || b.Size.H != 65 {
		t.Fatalf("item 3 = %v, want y=105 h=65", b)
	}

	// Middle: every frame builds only the items around the viewport.
	r.calls = map[int]int{}
	hw.Dispatch(wheel(50, 50, 0, 25000))
	s, e := r.state.VisibleRange()
	if e-s > 20 || len(r.calls) > 2*20 {
		t.Fatalf("after scrolling range [%d,%d), %d items rendered: not virtualized", s, e, len(r.calls))
	}
	off := r.handle.Offset().Y
	first := r.itemBounds(t, hw, s)
	if first.Origin.Y > 0 || first.Max().Y <= 0 {
		t.Fatalf("first built item %d at %v does not cover the viewport top (offset %v)", s, first, off)
	}
	last := r.itemBounds(t, hw, e-1)
	if last.Origin.Y >= 300 || last.Max().Y < 300 {
		t.Fatalf("last built item %d at %v does not reach the viewport bottom", e-1, last)
	}

	// Bottom.
	r.handle.ScrollToBottom()
	r.frame(hw)
	if _, e := r.state.VisibleRange(); e != 2000 {
		t.Fatalf("bottom range ends at %d, want 2000", e)
	}
	if b := r.itemBounds(t, hw, 1999); b.Max().Y != 300 {
		t.Fatalf("last item at %v, want its bottom at 300", b)
	}
	if o, m := r.handle.Offset().Y, r.handle.MaxOffset().H; o != m {
		t.Fatalf("offset %v != max %v at the bottom", o, m)
	}
}

func TestListContentHeightIsSumOfCachedHeights(t *testing.T) {
	r := newListRig(2000, mixed)
	r.gap, r.pad = 4, 6
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	// Unmeasured items count as the (lower) median of the measured ones.
	var measured float32
	var hs []float32
	for i := 0; i < 2000; i++ {
		if h, ok := r.state.MeasuredHeight(i); ok {
			measured += float32(h)
			hs = append(hs, float32(h))
		}
	}
	n := len(hs)
	slices.Sort(hs)
	median := hs[(n-1)/2]
	want := measured + median*float32(2000-n) + 4*1999 + 12
	// float32 sums over 2,000 items: allow rounding.
	if got := float32(r.state.ContentHeight()); math.Abs(float64(got-want)) > 4 {
		t.Fatalf("content height = %v, want %v (measured %d)", got, want, n)
	}
	if m := float32(r.handle.MaxOffset().H); math.Abs(float64(m-(want-300))) > 4 {
		t.Fatalf("max offset = %v, want %v", m, want-300)
	}
	// Scrolling through everything measures every item: the content height
	// is then exactly the items plus gaps plus padding.
	for i := 0; i < 2000; i += 5 {
		r.handle.ScrollToItem(i)
		r.frame(hw)
	}
	r.handle.ScrollToBottom()
	r.frame(hw)
	exact := sumHeights(r.heights, 4) + 12
	if got := float32(r.state.ContentHeight()); got != exact {
		t.Fatalf("content height after measuring all = %v, want %v", got, exact)
	}
	if m := float32(r.handle.MaxOffset().H); m != exact-300 {
		t.Fatalf("max offset = %v, want %v", m, exact-300)
	}
}

func TestListScrollToBottomThroughUnmeasuredItems(t *testing.T) {
	// The estimate (24) is far from the real heights (100): resolving the
	// bottom must still land exactly on the last item's end.
	r := newListRig(500, func(int) float32 { return 100 })
	r.pad = 8
	r.handle.ScrollToBottom()
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	b := r.itemBounds(t, hw, 499)
	if b.Max().Y != 300-8 {
		t.Fatalf("last item at %v, want bottom at 292 (padding)", b)
	}
	if o, m := r.handle.Offset().Y, r.handle.MaxOffset().H; o != m {
		t.Fatalf("offset %v != max %v", o, m)
	}
	if s, _ := r.state.VisibleRange(); s < 490 {
		t.Fatalf("range starts at %d: built far too much", s)
	}
	if n := len(r.calls); n > 20 {
		t.Fatalf("%d items rendered for a bottom jump", n)
	}
}

func TestListWidthChangeDropsHeights(t *testing.T) {
	// Each item is a wrapping row of 40px chips: narrower means taller.
	state := gpui.NewListState()
	width := float32(200)
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		l := gpui.NewList("wrap", state, 200, func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
			row := gpui.NewDiv().FlexRow().FlexWrap()
			for k := 0; k < 4; k++ {
				row.Child(gpui.NewDiv().SizeWH(40, 10).Bg(gpui.Gray300))
			}
			return row
		}).Overdraw(0).SizeWH(width, 300)
		return gpui.NewDiv().SizeFull().FlexCol().ItemsStart().Child(l)
	})
	hw, _ := openBench(t, 400, 400, 1, root)
	_ = hw.Pixels()
	if h, ok := state.MeasuredHeight(0); !ok || h != 10 {
		t.Fatalf("item 0 at 200px = %v %v, want 10", h, ok)
	}
	// Scroll down so item 0 is no longer built, then narrow the list.
	hw.Dispatch(wheel(50, 50, 0, 1000))
	if s, _ := state.VisibleRange(); s == 0 {
		t.Fatal("item 0 still built after scrolling")
	}
	width = 100
	hw.Window().Refresh()
	_ = hw.Pixels()
	if _, ok := state.MeasuredHeight(0); ok {
		t.Fatal("item 0 kept its height across a width change")
	}
	s, _ := state.VisibleRange()
	if h, ok := state.MeasuredHeight(s); !ok || h != 20 {
		t.Fatalf("visible item %d at 100px = %v %v, want re-measured 20", s, h, ok)
	}
}

func TestListItemChangeInvalidates(t *testing.T) {
	r := newListRig(300, func(int) float32 { return 30 })
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	r.handle.ScrollToItem(100)
	r.frame(hw)
	r.handle.ScrollToTop()
	r.frame(hw)
	if h, ok := r.state.MeasuredHeight(100); !ok || h != 30 {
		t.Fatalf("item 100 = %v %v, want measured 30", h, ok)
	}
	// Off-screen content change: a new key drops the stale height.
	r.heights[100] = 90
	r.keys[100]++
	r.frame(hw)
	if _, ok := r.state.MeasuredHeight(100); ok {
		t.Fatal("item 100 kept its height after its key changed")
	}
	// Explicit invalidation.
	r.state.Invalidate(5)
	if _, ok := r.state.MeasuredHeight(5); ok {
		t.Fatal("Invalidate(5) kept the height")
	}
	// A visible item whose content changed corrects itself even without a
	// key change, in the same frame.
	r.heights[2] = 70
	r.frame(hw)
	if h, _ := r.state.MeasuredHeight(2); h != 70 {
		t.Fatalf("visible item 2 = %v, want re-measured 70", h)
	}
	if b := r.itemBounds(t, hw, 3); b.Origin.Y != 130 {
		t.Fatalf("item 3 at %v, want y=130 after item 2 grew", b)
	}
	r.handle.ScrollToItem(100)
	r.frame(hw)
	if h, ok := r.state.MeasuredHeight(100); !ok || h != 90 {
		t.Fatalf("item 100 = %v %v, want re-measured 90", h, ok)
	}
}

func TestListAnchorsWhenItemsAboveChange(t *testing.T) {
	r := newListRig(400, func(int) float32 { return 30 })
	r.overdraw = 100
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	hw.Dispatch(wheel(50, 50, 0, 3000)) // item 100 at the top
	before := r.itemBounds(t, hw, 100)
	off := r.handle.Offset().Y
	if before.Origin.Y != 0 || off != 3000 {
		t.Fatalf("item 100 at %v, offset %v; want at the top, 3000", before, off)
	}
	// An item in the overdraw band above the viewport grows.
	r.heights[98] = 130
	r.frame(hw)
	if b := r.itemBounds(t, hw, 100); b.Origin.Y != 0 {
		t.Fatalf("item 100 moved to %v when item 98 grew", b)
	}
	// The offset moves by the growth (and by the estimate of the
	// unmeasured items above, which follows the median).
	if o := r.handle.Offset().Y; o < 3100 {
		t.Fatalf("offset = %v, want at least 3100 (compensated)", o)
	}
	// An item far above changes its key and falls back to the estimate.
	r.heights[10] = 500
	r.keys[10]++
	r.frame(hw)
	if b := r.itemBounds(t, hw, 100); b.Origin.Y != 0 {
		t.Fatalf("item 100 moved to %v when item 10 was invalidated", b)
	}
	// Wheel deltas still apply on top of the anchor.
	hw.Dispatch(wheel(50, 50, 0, 15))
	if b := r.itemBounds(t, hw, 100); b.Origin.Y != -15 {
		t.Fatalf("item 100 at %v after a 15px wheel, want y=-15", b)
	}
}

func TestListClicksReachVisibleItems(t *testing.T) {
	r := newListRig(1000, mixed)
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	hw.Dispatch(wheel(50, 50, 0, 10000))
	s, e := r.state.VisibleRange()
	for ix := s; ix < e; ix++ {
		b := r.itemBounds(t, hw, ix)
		y := b.Origin.Y + b.Size.H/2
		if y < 0 || y >= 300 {
			continue
		}
		r.clicked = nil
		click(hw, gpui.Point{X: 50, Y: y})
		if len(r.clicked) != 1 || r.clicked[0] != ix {
			t.Fatalf("click at y=%v hit %v, want item %d", y, r.clicked, ix)
		}
		id, ok := hw.Window().HitTest(gpui.Point{X: 50, Y: y})
		if !ok || id.Path[len(id.Path)-1].Key != fmt.Sprint("item-", ix) {
			t.Fatalf("hit test at y=%v = %v, want item-%d", y, id, ix)
		}
	}
	// Below the list nothing is hit.
	r.clicked = nil
	click(hw, gpui.Point{X: 50, Y: 350})
	if len(r.clicked) != 0 {
		t.Fatalf("click below the list hit %v", r.clicked)
	}
}

func TestListAutoHeightAndScrollbarDrag(t *testing.T) {
	state := gpui.NewListState()
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		l := gpui.NewList("l", state, 5, func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeH(mixed(ix)).Bg(gpui.Gray300)
		}).Gap(2).P(4).EstimatedItemHeight(50)
		return gpui.NewDiv().SizeFull().FlexCol().ItemsStretch().Child(l)
	})
	hw, _ := openBench(t, 200, 600, 1, root)
	_ = hw.Pixels()
	// First frame lays out at the estimate; the next uses the measurements.
	hw.Window().Refresh()
	_ = hw.Pixels()
	if h := state.ContentHeight(); h != 250+8+8 {
		t.Fatalf("content height = %v, want 266", h)
	}

	// A thumb drag across unmeasured items survives the extent changing.
	h := gpui.NewScrollHandle()
	ls := gpui.NewListState()
	drag := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewList("d", ls, 1000, func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeH(mixed(ix)).Bg(gpui.Blue500)
		}).Overdraw(0).SizeWH(100, 100).TrackScroll(h).ShowScrollbar()
	})
	dw, _ := openHeadless(t, 200, 200, drag)
	activeDown(dw, 95, 5, gpui.MouseLeft)
	prev := h.Offset().Y
	for y := gpui.Pixels(10); y <= 60; y += 10 {
		scrollbarMove(dw, 95, y)
		if o := h.Offset().Y; o <= prev {
			t.Fatalf("drag stalled at pointer y=%v: offset %v after %v", y, o, prev)
		}
		prev = h.Offset().Y
	}
	activeUp(dw, 95, 60, gpui.MouseLeft)
	if s, e := ls.VisibleRange(); e-s > 6 {
		t.Fatalf("drag built [%d,%d)", s, e)
	}
}

// BenchmarkList2000 draws a 2,000-item list scrolled to the middle; the
// cost tracks the visible items, not the count.
func BenchmarkList2000(b *testing.B) {
	r := newListRig(2000, mixed)
	hw, _ := openBench(b, 300, 400, 1, r.root())
	r.handle.ScrollToItem(1000)
	_ = hw.Pixels()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.calls = map[int]int{}
		hw.Window().Refresh()
		_ = hw.Pixels()
	}
	b.ReportMetric(float64(len(r.calls)), "items/frame")
}

func TestListEmptyFrameKeepsOffsetAtTop(t *testing.T) {
	// Regression: an empty frame stored the anchor as an absolute offset, which
	// the next non-empty frame read relative to item 0 (at the padding).
	r := newListRig(500, func(int) float32 { return 30 })
	r.pad = 12
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	r.heights = nil
	r.keys = nil
	r.frame(hw)
	r.heights = make([]float32, 300)
	r.keys = make([]uint64, 300)
	for i := range r.heights {
		r.heights[i] = 300
	}
	r.frame(hw)
	if o := r.handle.Offset().Y; o != 0 {
		t.Fatalf("offset after empty frame = %v, want 0", o)
	}
}

func TestListEstimateIgnoresOutliers(t *testing.T) {
	// One 5,000px item among 10px ones must not inflate the estimate of the
	// unmeasured items after a jump to the bottom.
	r := newListRig(1000, func(ix int) float32 {
		if ix == 999 {
			return 5000
		}
		return 10
	})
	r.gap, r.pad = 8, 12
	r.handle.ScrollToBottom()
	hw, _ := openBench(t, 300, 400, 1, r.root())
	_ = hw.Pixels()
	want := float32(999*10 + 5000 + 999*8 + 24) // 23,006
	if got := float32(r.state.ContentHeight()); math.Abs(float64(got-want)) > float64(want)*0.05 {
		t.Fatalf("content height = %v, want about %v", got, want)
	}
	if b := r.itemBounds(t, hw, 999); b.Max().Y != 300-12 {
		t.Fatalf("last item at %v, want its bottom at 288", b)
	}
}
