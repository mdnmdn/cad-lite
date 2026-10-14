package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

func withMargin(n *gpui.LayoutNode, top, right, bottom, left float32) *gpui.LayoutNode {
	n.Style.Margin = gpui.Edges{Top: gpui.Pixels(top), Right: gpui.Pixels(right), Bottom: gpui.Pixels(bottom), Left: gpui.Pixels(left)}
	return n
}

func TestLayoutMarginsRow(t *testing.T) {
	a := withMargin(box(40, 20), 5, 10, 0, 7)
	b := box(30, 20)
	root := node(func(s *gpui.Style) { row(s); s.AlignItems = gpui.AlignStart }, a, b)
	gpui.Layout(root, gpui.Definite(300, 100))
	wantRect(t, "a", a.Rect, 7, 5, 40, 20)
	wantRect(t, "b", b.Rect, 7+40+10, 0, 30, 20)
}

func TestLayoutMarginsContributeToAutoSize(t *testing.T) {
	a := withMargin(box(40, 20), 3, 4, 5, 6)
	inner := node(row, a)
	root := node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, inner)
	gpui.Layout(root, gpui.Definite(300, 100))
	wantRect(t, "inner", inner.Rect, 0, 0, 50, 28)
	wantRect(t, "a", a.Rect, 6, 3, 40, 20)
}

func TestLayoutMarginsShrinkStretchAndFlex(t *testing.T) {
	// Stretch fills the line minus the cross margins.
	a := withMargin(node(nil), 10, 0, 10, 0)
	a.Style.Width = gpui.Px(20)
	// Grow takes the free space left after the main-axis margins.
	g := withMargin(node(func(s *gpui.Style) { s.FlexGrow = 1 }), 0, 15, 0, 5)
	root := node(row, a, g)
	gpui.Layout(root, gpui.Definite(200, 100))
	wantRect(t, "a", a.Rect, 0, 10, 20, 80)
	wantRect(t, "g", g.Rect, 25, 0, 160, 100)
}

func TestLayoutMarginsColumnJustifyEnd(t *testing.T) {
	a := withMargin(box(20, 20), 0, 0, 8, 0)
	root := node(func(s *gpui.Style) { s.Justify = gpui.JustifyEnd; s.AlignItems = gpui.AlignStart }, a)
	gpui.Layout(root, gpui.Definite(100, 100))
	wantRect(t, "a", a.Rect, 0, 72, 20, 20)
}

func TestLayoutAutoMargins(t *testing.T) {
	// ml-auto pushes the last item to the end of a row.
	a, b := box(20, 20), box(30, 20)
	b.Style.MarginAuto.Left = true
	root := node(row, a, b)
	gpui.Layout(root, gpui.Definite(200, 50))
	wantRect(t, "b", b.Rect, 170, 0, 30, 20)

	// mx-auto centers on the main axis, overriding justify.
	c := box(40, 20)
	c.Style.MarginAuto = gpui.AutoEdges{Left: true, Right: true}
	root = node(func(s *gpui.Style) { row(s); s.Justify = gpui.JustifyEnd }, c)
	gpui.Layout(root, gpui.Definite(200, 50))
	wantRect(t, "c", c.Rect, 80, 0, 40, 20)

	// my-auto centers on the cross axis and disables stretch.
	d := node(nil)
	d.Style.Width = gpui.Px(10)
	d.Style.Height = gpui.Px(10)
	d.Style.MarginAuto = gpui.AutoEdges{Top: true, Bottom: true}
	e := node(func(s *gpui.Style) { s.Width = gpui.Px(10); s.MarginAuto.Top = true })
	e.Children = []*gpui.LayoutNode{box(10, 6)}
	root = node(row, d, e)
	gpui.Layout(root, gpui.Definite(100, 50))
	wantRect(t, "d", d.Rect, 0, 20, 10, 10)
	wantRect(t, "e", e.Rect, 10, 44, 10, 6)
}

func TestLayoutFlexBasisZeroGivesEqualColumns(t *testing.T) {
	short, long := textNode("a"), textNode("much longer")
	c1 := node(func(s *gpui.Style) { s.FlexGrow, s.FlexShrink, s.FlexBasis = 1, 1, gpui.Px(0) }, short)
	c2 := node(func(s *gpui.Style) { s.FlexGrow, s.FlexShrink, s.FlexBasis = 1, 1, gpui.Px(0) }, long)
	root := node(row, c1, c2)
	gpui.Layout(root, gpui.Definite(400, 50))
	wantRect(t, "c1", c1.Rect, 0, 0, 200, 50)
	wantRect(t, "c2", c2.Rect, 200, 0, 200, 50)

	// An auto basis (flex: 1 1 auto) distributes on top of the content.
	a1 := node(func(s *gpui.Style) { s.FlexGrow = 1 }, textNode("a"))
	a2 := node(func(s *gpui.Style) { s.FlexGrow = 1 }, textNode("much longer"))
	root = node(row, a1, a2)
	gpui.Layout(root, gpui.Definite(400, 50))
	if a1.Rect.Size.W >= a2.Rect.Size.W {
		t.Errorf("auto basis widths = %v, %v; want content-weighted", a1.Rect.Size.W, a2.Rect.Size.W)
	}

	// A basis-0 item in a content-sized row still contributes its content.
	f := node(func(s *gpui.Style) { s.FlexGrow, s.FlexBasis = 1, gpui.Px(0) }, box(60, 10))
	inner := node(row, f)
	root = node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, inner)
	gpui.Layout(root, gpui.Definite(400, 50))
	wantRect(t, "inner", inner.Rect, 0, 0, 60, 10)
}

func TestFlex1BuilderSetsZeroBasis(t *testing.T) {
	s := gpui.NewDiv().Flex1().ResolvedStyle(gpui.DefaultStyle())
	if s.FlexGrow != 1 || s.FlexShrink != 1 || s.FlexBasis != gpui.Px(0) {
		t.Errorf("Flex1 = grow %v shrink %v basis %v", s.FlexGrow, s.FlexShrink, s.FlexBasis)
	}
	s = gpui.NewDiv().FlexAuto().ResolvedStyle(gpui.DefaultStyle())
	if s.FlexGrow != 1 || s.FlexShrink != 1 || !s.FlexBasis.IsAuto() {
		t.Errorf("FlexAuto = grow %v shrink %v basis %v", s.FlexGrow, s.FlexShrink, s.FlexBasis)
	}
	s = gpui.NewDiv().Mx(3).Mt(4).MlAuto().ResolvedStyle(gpui.DefaultStyle())
	if s.Margin != (gpui.Edges{Top: 4, Right: 3, Left: 3}) || s.MarginAuto != (gpui.AutoEdges{Left: true}) {
		t.Errorf("margins = %+v auto %+v", s.Margin, s.MarginAuto)
	}
	s = gpui.NewText("x").MyAuto().ResolvedStyle(gpui.DefaultStyle())
	if s.MarginAuto != (gpui.AutoEdges{Top: true, Bottom: true}) {
		t.Errorf("text margins auto %+v", s.MarginAuto)
	}
}
