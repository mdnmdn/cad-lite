package gpui_test

import (
	"go-gpui/gpui"
	"testing"
)

func TestFlexWrapLinesAndGaps(t *testing.T) {
	for _, dir := range []gpui.Direction{gpui.DirectionRow, gpui.DirectionColumn} {
		for _, width := range []float32{100, 99} {
			a, b, c := box(45, 20), box(45, 20), box(45, 20)
			if dir == gpui.DirectionColumn {
				a, b, c = box(20, 45), box(20, 45), box(20, 45)
			}
			hidden, abs := box(900, 900), box(900, 900)
			hidden.Style.Display = false
			abs.Style.Position = gpui.PositionAbsolute
			r := node(func(s *gpui.Style) {
				s.Wrap = true
				s.Direction = dir
				s.Gap = 10
				s.AlignContent = gpui.AlignContentStart
			}, a, hidden, nil, abs, b, c)
			if dir == gpui.DirectionRow {
				gpui.Layout(r, gpui.Definite(width, 100))
				if width == 100 {
					wantRect(t, "b", b.Rect, 55, 0, 45, 20)
					wantRect(t, "c", c.Rect, 0, 30, 45, 20)
				} else {
					wantRect(t, "b", b.Rect, 0, 30, 45, 20)
					wantRect(t, "c", c.Rect, 0, 60, 45, 20)
				}
			} else {
				gpui.Layout(r, gpui.Definite(100, width))
				if width == 100 {
					wantRect(t, "b", b.Rect, 0, 55, 20, 45)
					wantRect(t, "c", c.Rect, 30, 0, 20, 45)
				} else {
					wantRect(t, "b", b.Rect, 30, 0, 20, 45)
					wantRect(t, "c", c.Rect, 60, 0, 20, 45)
				}
			}
		}
	}
	a, b := box(40, 20), box(40, 20)
	a.Style.Margin = gpui.Edges{Left: 5, Right: 5}
	b.Style.Margin = gpui.Edges{Left: 5, Right: 5}
	r := node(func(s *gpui.Style) {
		s.Wrap = true
		s.Direction = gpui.DirectionRow
		s.RowGap = gpui.Px(7)
		s.ColumnGap = gpui.Px(1)
		s.AlignContent = gpui.AlignContentStart
	}, a, b)
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "margin wrap", b.Rect, 5, 27, 40, 20)
}

func TestFlexWrapFlexesPerLine(t *testing.T) {
	a, b, c := box(40, 20), box(40, 20), box(40, 20)
	for _, n := range []*gpui.LayoutNode{a, b, c} {
		n.Style.FlexGrow = 1
	}
	r := node(func(s *gpui.Style) {
		s.Wrap = true
		s.Direction = gpui.DirectionRow
		s.Gap = 10
		s.AlignContent = gpui.AlignContentStart
	}, a, b, c)
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "a", a.Rect, 0, 0, 45, 20)
	wantRect(t, "b", b.Rect, 55, 0, 45, 20)
	wantRect(t, "c", c.Rect, 0, 30, 100, 20)
	c.Style.MaxWidth = gpui.Px(70)
	r.Style.Justify = gpui.JustifyEnd
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "limited singleton", c.Rect, 30, 30, 70, 20)
	oversize := box(130, 20)
	oversize.Style.MinWidth = gpui.Px(0)
	r.Children = []*gpui.LayoutNode{oversize, box(20, 20)}
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "shrunk oversized", oversize.Rect, 0, 0, 100, 20)
}

func TestFlexWrapContentAlignment(t *testing.T) {
	for _, tc := range []struct {
		mode      gpui.AlignContent
		y1, y2, h float32
	}{
		{gpui.AlignContentStart, 0, 30, 20}, {gpui.AlignContentCenter, 25, 55, 20}, {gpui.AlignContentEnd, 50, 80, 20},
		{gpui.AlignContentSpaceBetween, 0, 80, 20}, {gpui.AlignContentSpaceAround, 12.5, 67.5, 20}, {gpui.AlignContentStretch, 0, 55, 45},
	} {
		a, b := node(nil, box(60, 20)), node(nil, box(60, 20))
		r := node(func(s *gpui.Style) {
			s.Wrap = true
			s.Direction = gpui.DirectionRow
			s.Gap = 10
			s.AlignContent = tc.mode
		}, a, b)
		gpui.Layout(r, gpui.Definite(100, 100))
		wantRect(t, "line1", a.Rect, 0, tc.y1, 60, tc.h)
		wantRect(t, "line2", b.Rect, 0, tc.y2, 60, tc.h)
	}
	a := box(40, 20)
	r := node(func(s *gpui.Style) {
		s.Wrap = true
		s.Direction = gpui.DirectionRow
		s.AlignContent = gpui.AlignContentEnd
	}, a)
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "single line end", a.Rect, 0, 80, 40, 20)
	r.Style.Wrap = false
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "nowrap ignores content end", a.Rect, 0, 0, 40, 20)
}

func TestFlexWrapAutoSizingAndMaximum(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		a, b, c := box(60, 20), box(60, 30), box(60, 10)
		for _, n := range []*gpui.LayoutNode{a, b, c} {
			n.Style.FlexGrow = 1
		}
		r := node(func(s *gpui.Style) {
			s.Wrap = true
			s.Direction = gpui.DirectionRow
			s.Gap = 5
			s.MaxWidth = gpui.Px(100)
		}, a, b, c)
		if bounded {
			outer := node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, r)
			gpui.Layout(outer, gpui.Available{W: 200, BoundedW: true})
		} else {
			gpui.Layout(r, gpui.Available{})
		}
		wantRect(t, "parent", r.Rect, 0, 0, 60, 70)
		wantRect(t, "a", a.Rect, 0, 0, 60, 20)
		wantRect(t, "c", c.Rect, 0, 60, 60, 10)
	}
	a, b := box(60, 20), box(60, 20)
	r := node(func(s *gpui.Style) { s.Wrap = true; s.Direction = gpui.DirectionRow; s.Gap = 5 }, a, b)
	gpui.Layout(r, gpui.Available{})
	wantRect(t, "unbounded line", r.Rect, 0, 0, 125, 20)
	r.Style.MaxWidth = gpui.Px(100)
	r.Style.MinWidth = gpui.Px(80)
	r.Style.MinHeight = gpui.Px(100)
	gpui.Layout(r, gpui.Available{})
	wantRect(t, "min parent", r.Rect, 0, 0, 80, 100)
	wantRect(t, "second", b.Rect, 0, 52.5, 60, 20)
}

func TestFlexWrapTextAndAbsolute(t *testing.T) {
	txt := textNode("aaaa bbbb cccc")
	txt.Style.MinWidth = gpui.Px(0)
	abs := box(20, 10)
	abs.Style.Position = gpui.PositionAbsolute
	abs.Style.Right = gpui.Px(5)
	abs.Style.Bottom = gpui.Px(5)
	r := node(func(s *gpui.Style) { s.Wrap = true; s.Direction = gpui.DirectionRow; s.Width = gpui.Px(100); s.Gap = 5 }, txt, box(50, 20), abs)
	gpui.Layout(r, gpui.Available{})
	wantRect(t, "wrapped text", txt.Rect, 0, 0, 100, 40)
	wantRect(t, "parent", r.Rect, 0, 0, 100, 65)
	wantRect(t, "absolute", abs.Rect, 75, 50, 20, 10)
}

func TestFlexWrapBuildersAndAxisGapPrecedence(t *testing.T) {
	s := gpui.NewDiv().FlexWrap().Gap(5).RowGap(0).ColumnGap(10).ContentAround().ResolvedStyle(gpui.DefaultStyle())
	if !s.Wrap || s.RowGap != gpui.Px(0) || s.ColumnGap != gpui.Px(10) || s.AlignContent != gpui.AlignContentSpaceAround {
		t.Fatalf("style=%+v", s)
	}
	s = gpui.NewText("x").FlexWrap().RowGap(9).ColumnGap(9).Gap(4).FlexNoWrap().ResolvedStyle(gpui.DefaultStyle())
	if s.Wrap || !s.RowGap.IsAuto() || !s.ColumnGap.IsAuto() {
		t.Fatalf("reset=%+v", s)
	}
	a, b := box(20, 20), box(20, 20)
	r := node(func(s *gpui.Style) {
		s.Direction = gpui.DirectionRow
		s.Gap = 8
		s.ColumnGap = gpui.Px(3)
		s.RowGap = gpui.Px(99)
	}, a, b)
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "nowrap column gap", b.Rect, 23, 0, 20, 20)
}
