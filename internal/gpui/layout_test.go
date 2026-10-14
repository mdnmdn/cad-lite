package gpui_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/internal/gpui"
)

// fakeMeasure lays text out with 10px per character (spaces included) and a
// 20px line height, wrapping greedily at spaces. maxWidth <= 0 asks for the
// min-content size: the longest word.
func fakeMeasure(text string, _ gpui.TextStyle, maxWidth float32) (float32, float32) {
	const cw, lh = 10, 20
	words := strings.Fields(text)
	if maxWidth <= 0 {
		longest := 0
		for _, w := range words {
			longest = max(longest, len(w))
		}
		maxWidth = float32(longest * cw)
	}
	var lines []int
	cur := -1
	for _, w := range words {
		switch {
		case cur < 0:
			cur = len(w)
		case float32((cur+1+len(w))*cw) <= maxWidth:
			cur += 1 + len(w)
		default:
			lines = append(lines, cur)
			cur = len(w)
		}
	}
	if cur >= 0 {
		lines = append(lines, cur)
	}
	widest := 0
	for _, l := range lines {
		widest = max(widest, l)
	}
	return float32(widest * cw), float32(len(lines) * lh)
}

func box(w, h float32) *gpui.LayoutNode {
	s := gpui.DefaultStyle()
	s.Width, s.Height = gpui.Px(w), gpui.Px(h)
	return &gpui.LayoutNode{Style: s}
}

func node(mod func(*gpui.Style), kids ...*gpui.LayoutNode) *gpui.LayoutNode {
	s := gpui.DefaultStyle()
	if mod != nil {
		mod(&s)
	}
	return &gpui.LayoutNode{Style: s, Children: kids}
}

func textNode(s string) *gpui.LayoutNode {
	return &gpui.LayoutNode{Style: gpui.DefaultStyle(), Text: s, Measure: fakeMeasure}
}

func row(s *gpui.Style) { s.Direction = gpui.DirectionRow }

func wantRect(t *testing.T, name string, got gpui.Bounds, x, y, w, h float32) {
	t.Helper()
	near := func(a gpui.Pixels, b float32) bool { return math.Abs(float64(a)-float64(b)) < 0.01 }
	if !near(got.Origin.X, x) || !near(got.Origin.Y, y) || !near(got.Size.W, w) || !near(got.Size.H, h) {
		t.Errorf("%s = (%v,%v %vx%v), want (%v,%v %vx%v)", name,
			got.Origin.X, got.Origin.Y, got.Size.W, got.Size.H, x, y, w, h)
	}
}

func TestLayoutChildRectsAreParentRelative(t *testing.T) {
	leaf := box(20, 20)
	mid := node(func(s *gpui.Style) {
		s.Width, s.Height = gpui.Px(100), gpui.Px(100)
		s.Padding = gpui.Edges{Top: 5, Right: 5, Bottom: 5, Left: 5}
	}, leaf)
	root := node(func(s *gpui.Style) {
		s.Padding = gpui.Edges{Top: 10, Right: 10, Bottom: 10, Left: 10}
	}, mid)
	gpui.Layout(root, gpui.Definite(300, 300))

	wantRect(t, "root", root.Rect, 0, 0, 300, 300)
	wantRect(t, "mid", mid.Rect, 10, 10, 100, 100)
	wantRect(t, "leaf", leaf.Rect, 5, 5, 20, 20)
}

func TestNestedDivPaintsAtAbsoluteOffset(t *testing.T) {
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(200, 200).P(10).Child(
			gpui.NewDiv().P(20).Child(
				gpui.NewDiv().SizeWH(10, 10).Bg(gpui.Red500),
			),
		)
	})
	_, win := drawTextRoot(t, 200, 200, root, false)
	for _, q := range win.Scene().Quads {
		if q.Fill == gpui.Red500 {
			wantRect(t, "red quad", q.Bounds, 30, 30, 10, 10)
			return
		}
	}
	t.Fatal("red quad not painted")
}

func TestLayoutTextWrapsInFixedWidthColumn(t *testing.T) {
	txt := textNode("aaaa bbbb cccc")
	col := node(func(s *gpui.Style) { s.Width = gpui.Px(100) }, txt)
	root := node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, col)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "text", txt.Rect, 0, 0, 100, 40)
	wantRect(t, "col", col.Rect, 0, 0, 100, 40)
}

func TestLayoutRowTextShrinksToMinContent(t *testing.T) {
	txt := textNode("aaaa bbbb cccc dddd")
	fixed := box(100, 10)
	r := node(func(s *gpui.Style) {
		row(s)
		s.Width = gpui.Px(200)
		s.AlignItems = gpui.AlignStart
	}, txt, fixed)
	root := node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, r)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "text", txt.Rect, 0, 0, 100, 40)
	wantRect(t, "fixed", fixed.Rect, 100, 0, 100, 10)
	wantRect(t, "row", r.Rect, 0, 0, 200, 40)
}

func TestLayoutCardTextWrapsWithoutMinW(t *testing.T) {
	// A row of two auto-width cards, each holding a long paragraph, must fit
	// the row: the cards shrink to (at least) their longest word.
	t1 := textNode("one two three four five six seven")
	t2 := textNode("eight nine ten eleven twelve")
	c1 := node(nil, t1)
	c2 := node(nil, t2)
	r := node(func(s *gpui.Style) { row(s); s.Gap = 10 }, c1, c2)
	root := node(func(s *gpui.Style) { s.Width = gpui.Px(310) }, r)
	gpui.Layout(root, gpui.Definite(310, 400))

	if end := c2.Rect.Max().X; end > 310.01 {
		t.Errorf("second card ends at %v, overflowing the 310px row", end)
	}
	if c1.Rect.Size.H <= 20 || t1.Rect.Size.H <= 20 {
		t.Errorf("first card text did not wrap: card %v, text %v", c1.Rect.Size, t1.Rect.Size)
	}
	if t1.Rect.Size.W > c1.Rect.Size.W+0.01 {
		t.Errorf("text %v wider than its card %v", t1.Rect.Size.W, c1.Rect.Size.W)
	}
}

func TestLayoutRowAutoHeightIsContentHeight(t *testing.T) {
	// Mirrors the todo header: a text-input-like box (padded, holding text)
	// next to a button in an ItemsCenter row inside a tall column.
	input := node(func(s *gpui.Style) {
		row(s)
		s.AlignItems = gpui.AlignCenter
		s.Padding = gpui.Edges{Top: 8, Right: 8, Bottom: 8, Left: 8}
		s.FlexGrow = 1
	}, textNode("placeholder"))
	btn := node(func(s *gpui.Style) {
		s.Padding = gpui.Edges{Top: 8, Right: 8, Bottom: 8, Left: 8}
	}, textNode("Add"))
	r := node(func(s *gpui.Style) { row(s); s.Gap = 8; s.AlignItems = gpui.AlignCenter }, input, btn)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(400, 600))

	wantRect(t, "row", r.Rect, 0, 0, 400, 36)
	wantRect(t, "input", input.Rect, 0, 0, 400-8-46, 36)
	wantRect(t, "button", btn.Rect, 400-46, 0, 46, 36)
}

func TestLayoutColumnOfAutoHeightBoxes(t *testing.T) {
	a := node(nil, box(50, 50))
	b := node(nil, box(50, 30))
	root := node(nil, a, b)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "a", a.Rect, 0, 0, 400, 50)
	wantRect(t, "b", b.Rect, 0, 50, 400, 30)
}

func TestLayoutStretchOnlyOnCrossAxis(t *testing.T) {
	// In a definite-height row, auto-height children stretch to the row's
	// height but keep their content width.
	a := node(nil, box(30, 10))
	r := node(func(s *gpui.Style) { row(s); s.Height = gpui.Px(80) }, a)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "row", r.Rect, 0, 0, 400, 80)
	wantRect(t, "a", a.Rect, 0, 0, 30, 80)
}

func TestLayoutRowWithoutHeightStretchesToTallestItem(t *testing.T) {
	a := node(nil, box(30, 10))
	b := box(30, 40)
	r := node(row, a, b)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "row", r.Rect, 0, 0, 400, 40)
	wantRect(t, "a", a.Rect, 0, 0, 30, 40)
}

func justifyRow(j gpui.Justify) (*gpui.LayoutNode, []*gpui.LayoutNode) {
	kids := []*gpui.LayoutNode{box(50, 10), box(50, 10), box(50, 10)}
	r := node(func(s *gpui.Style) {
		row(s)
		s.Width, s.Height = gpui.Px(300), gpui.Px(50)
		s.Justify = j
	}, kids...)
	return r, kids
}

func TestLayoutJustify(t *testing.T) {
	cases := []struct {
		name string
		j    gpui.Justify
		xs   [3]float32
	}{
		{"start", gpui.JustifyStart, [3]float32{0, 50, 100}},
		{"center", gpui.JustifyCenter, [3]float32{75, 125, 175}},
		{"end", gpui.JustifyEnd, [3]float32{150, 200, 250}},
		{"between", gpui.JustifySpaceBetween, [3]float32{0, 125, 250}},
		{"around", gpui.JustifySpaceAround, [3]float32{25, 125, 225}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, kids := justifyRow(c.j)
			gpui.Layout(r, gpui.Definite(300, 50))
			for i, k := range kids {
				wantRect(t, "child", k.Rect, c.xs[i], 0, 50, 10)
			}
		})
	}
}

func TestLayoutJustifyBetweenWithGapAndPadding(t *testing.T) {
	kids := []*gpui.LayoutNode{box(40, 24), box(40, 40), box(40, 24)}
	r := node(func(s *gpui.Style) {
		row(s)
		s.Padding = gpui.Edges{Top: 8, Right: 8, Bottom: 8, Left: 8}
		s.Justify = gpui.JustifySpaceBetween
		s.AlignItems = gpui.AlignCenter
		s.Gap = 4
	}, kids...)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(216, 400))

	wantRect(t, "row", r.Rect, 0, 0, 216, 56)
	wantRect(t, "a", kids[0].Rect, 8, 16, 40, 24)
	wantRect(t, "b", kids[1].Rect, 88, 8, 40, 40)
	wantRect(t, "c", kids[2].Rect, 168, 16, 40, 24)
}

func TestLayoutItemsCenter(t *testing.T) {
	a := box(20, 20)
	b := node(nil, box(40, 10))
	r := node(func(s *gpui.Style) {
		row(s)
		s.Width, s.Height = gpui.Px(200), gpui.Px(50)
		s.AlignItems = gpui.AlignCenter
	}, a, b)
	gpui.Layout(r, gpui.Definite(200, 50))

	wantRect(t, "a", a.Rect, 0, 15, 20, 20)
	wantRect(t, "b", b.Rect, 20, 20, 40, 10)
}

func TestLayoutColumnCentered(t *testing.T) {
	// The counter layout: a full-size column, JustifyCenter + ItemsCenter.
	a := box(100, 40)
	b := textNode("hello")
	root := node(func(s *gpui.Style) {
		s.Width, s.Height = gpui.Flex(1), gpui.Flex(1)
		s.Justify = gpui.JustifyCenter
		s.AlignItems = gpui.AlignCenter
		s.Gap = 20
	}, a, b)
	gpui.Layout(root, gpui.Definite(300, 300))

	wantRect(t, "root", root.Rect, 0, 0, 300, 300)
	wantRect(t, "a", a.Rect, 100, 110, 100, 40)
	wantRect(t, "b", b.Rect, 125, 170, 50, 20)
}

func TestLayoutFlexGrowFillsRow(t *testing.T) {
	fixed := box(80, 10)
	g1 := node(func(s *gpui.Style) { s.FlexGrow = 1 })
	g2 := node(func(s *gpui.Style) { s.FlexGrow = 1 })
	r := node(func(s *gpui.Style) { row(s); s.Gap = 10 }, fixed, g1, g2)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(300, 300))

	wantRect(t, "g1", g1.Rect, 90, 0, 100, 10)
	wantRect(t, "g2", g2.Rect, 200, 0, 100, 10)
}

func TestLayoutScrollContainerContentSize(t *testing.T) {
	kids := []*gpui.LayoutNode{node(nil, box(10, 50)), node(nil, box(10, 50)), node(nil, box(10, 50))}
	list := node(func(s *gpui.Style) {
		s.Height = gpui.Flex(1)
		s.Overflow = gpui.OverflowScroll
	}, kids...)
	header := box(10, 20)
	root := node(nil, header, list)
	gpui.Layout(root, gpui.Definite(200, 120))

	wantRect(t, "list", list.Rect, 0, 20, 200, 100)
	wantRect(t, "last", kids[2].Rect, 0, 100, 200, 50)
	if w, h := list.ScrollMax(); w != 0 || h != 50 {
		t.Errorf("ScrollMax = (%v, %v), want (0, 50)", w, h)
	}
}

func TestLayoutHiddenChildTakesNoSpace(t *testing.T) {
	a := box(10, 10)
	hidden := box(10, 10)
	hidden.Style.Display = false
	b := box(10, 10)
	root := node(func(s *gpui.Style) { s.Gap = 5 }, a, hidden, b)
	gpui.Layout(root, gpui.Definite(100, 100))

	wantRect(t, "b", b.Rect, 0, 15, 10, 10)
}

func TestTextElementWrapsAtLayoutWidth(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).ItemsStart().Child(
			gpui.NewDiv().SizeW(120).Child(
				gpui.NewText("the quick brown fox jumps over the lazy dog").TextSize(14),
			),
		)
	})
	_, win := drawTextRoot(t, 400, 300, root, true)

	sc := win.Scene()
	if len(sc.Sprites) == 0 {
		t.Fatal("no glyph sprites")
	}
	ys := map[gpui.Pixels]bool{}
	minY, maxY := gpui.Pixels(math.MaxFloat32), gpui.Pixels(0)
	for _, s := range sc.Sprites {
		if s.Bounds.Max().X > 125 {
			t.Errorf("glyph at x=%v extends past the 120px box", s.Bounds.Max().X)
		}
		ys[s.Bounds.Origin.Y] = true
		minY = min(minY, s.Bounds.Origin.Y)
		maxY = max(maxY, s.Bounds.Origin.Y)
	}
	if maxY-minY < 14 {
		t.Errorf("text did not wrap onto several lines (glyph y range %v..%v)", minY, maxY)
	}
}

func TestLayoutFlexGrowContentSizedRowKeepsBases(t *testing.T) {
	a := textNode("hello world foo")
	b := textNode("bar baz")
	for _, n := range []*gpui.LayoutNode{a, b} {
		n.Style.FlexGrow = 1
	}
	r := node(func(s *gpui.Style) { row(s); s.AlignItems = gpui.AlignStart }, a, b)
	root := node(func(s *gpui.Style) { s.AlignItems = gpui.AlignStart }, r)
	gpui.Layout(root, gpui.Definite(400, 400))

	wantRect(t, "a", a.Rect, 0, 0, 150, 20)
	wantRect(t, "b", b.Rect, 150, 0, 70, 20)
}

func TestLayoutFlexMaxWidthViolationRedistributes(t *testing.T) {
	g1 := node(func(s *gpui.Style) { s.Width = gpui.Flex(1); s.MaxWidth = gpui.Px(50); s.Height = gpui.Px(10) })
	g2 := node(func(s *gpui.Style) { s.Width = gpui.Flex(1); s.Height = gpui.Px(10) })
	r := node(func(s *gpui.Style) { row(s) }, g1, g2)
	root := node(nil, r)
	gpui.Layout(root, gpui.Definite(300, 300))

	wantRect(t, "g1", g1.Rect, 0, 0, 50, 10)
	wantRect(t, "g2", g2.Rect, 50, 0, 250, 10)
}

func TestLayoutAbsoluteDoesNotAffectFlow(t *testing.T) {
	for _, direction := range []gpui.Direction{gpui.DirectionRow, gpui.DirectionColumn} {
		a, b, overlay := box(20, 10), box(20, 10), box(900, 800)
		overlay.Style.Position = gpui.PositionAbsolute
		overlay.Style.Margin = gpui.Edges{Top: 100, Left: 100}
		overlay.Style.FlexGrow = 10
		hidden := box(999, 999)
		hidden.Style.Position, hidden.Style.Display = gpui.PositionAbsolute, false
		r := node(func(s *gpui.Style) { s.Direction = direction; s.Gap = 5 }, a, overlay, nil, hidden, b)
		gpui.Layout(r, gpui.Available{})
		if direction == gpui.DirectionRow {
			wantRect(t, "parent", r.Rect, 0, 0, 45, 10)
			wantRect(t, "second", b.Rect, 25, 0, 20, 10)
		} else {
			wantRect(t, "parent", r.Rect, 0, 0, 20, 25)
			wantRect(t, "second", b.Rect, 0, 15, 20, 10)
		}
		wantRect(t, "hidden", hidden.Rect, 0, 0, 0, 0)
	}
	abs := box(1000, 1000)
	abs.Style.Position = gpui.PositionAbsolute
	container := node(nil, abs)
	other := box(20, 20)
	r := node(row, container, other)
	gpui.Layout(r, gpui.Definite(100, 100))
	wantRect(t, "empty container", container.Rect, 0, 0, 0, 100)
	wantRect(t, "other", other.Rect, 0, 0, 20, 20)
}

func TestLayoutAbsoluteOffsetsAndSizing(t *testing.T) {
	cases := []struct {
		name       string
		style      func(*gpui.Style)
		x, y, w, h float32
	}{
		{"unset", func(s *gpui.Style) {}, 0, 0, 40, 20},
		{"leading", func(s *gpui.Style) { s.Left, s.Top = gpui.Px(12), gpui.Px(8) }, 12, 8, 40, 20},
		{"trailing", func(s *gpui.Style) { s.Right, s.Bottom = gpui.Px(12), gpui.Px(8) }, 148, 72, 40, 20},
		{"stretch", func(s *gpui.Style) {
			s.Width, s.Height = gpui.Auto(), gpui.Auto()
			s.Left, s.Right, s.Top, s.Bottom = gpui.Px(10), gpui.Px(20), gpui.Px(5), gpui.Px(15)
		}, 10, 5, 170, 80},
		{"clamped stretch", func(s *gpui.Style) {
			s.Width, s.Height = gpui.Auto(), gpui.Auto()
			s.Left, s.Right, s.Top, s.Bottom = gpui.Px(10), gpui.Px(20), gpui.Px(5), gpui.Px(15)
			s.MaxWidth, s.MinHeight = gpui.Px(50), gpui.Px(90)
		}, 10, 5, 50, 90},
		{"explicit wins", func(s *gpui.Style) {
			s.Left, s.Right, s.Top, s.Bottom = gpui.Px(10), gpui.Px(20), gpui.Px(5), gpui.Px(15)
		}, 10, 5, 40, 20},
		{"negative", func(s *gpui.Style) { s.Left, s.Top = gpui.Px(-10), gpui.Px(-5) }, -10, -5, 40, 20},
		{"fixed margins", func(s *gpui.Style) { s.Left, s.Top = gpui.Px(10), gpui.Px(5); s.Margin = gpui.Edges{Left: 3, Top: 4} }, 13, 9, 40, 20},
		{"auto margins", func(s *gpui.Style) {
			s.Margin = gpui.Edges{Left: 30, Top: 40}
			s.MarginAuto = gpui.AutoEdges{Left: true, Top: true}
		}, 0, 0, 40, 20},
		{"nonnegative stretch", func(s *gpui.Style) { s.Width = gpui.Auto(); s.Left, s.Right = gpui.Px(120), gpui.Px(120) }, 120, 0, 0, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := box(40, 20)
			c.Style.Position = gpui.PositionAbsolute
			tc.style(&c.Style)
			r := node(func(s *gpui.Style) {
				s.Padding = gpui.Edges{Top: 25, Right: 25, Bottom: 25, Left: 25}
				s.Justify = gpui.JustifyEnd
				s.AlignItems = gpui.AlignCenter
			}, c)
			gpui.Layout(r, gpui.Definite(200, 100))
			wantRect(t, "absolute", c.Rect, tc.x, tc.y, tc.w, tc.h)
		})
	}
}

func TestLayoutAbsoluteContentSizingNestingAndOverflow(t *testing.T) {
	text := textNode("aaaa bbbb cccc")
	text.Style.Position, text.Style.Left = gpui.PositionAbsolute, gpui.Px(100)
	inner := box(30, 20)
	inner.Style.Position, inner.Style.Right, inner.Style.Bottom = gpui.PositionAbsolute, gpui.Px(-20), gpui.Px(-10)
	middle := node(func(s *gpui.Style) {
		s.Position = gpui.PositionAbsolute
		s.Width, s.Height = gpui.Px(80), gpui.Px(60)
		s.Left, s.Top = gpui.Px(30), gpui.Px(15)
	}, inner)
	root := node(nil, text, middle)
	gpui.Layout(root, gpui.Definite(200, 100))
	wantRect(t, "text", text.Rect, 100, 0, 90, 40)
	wantRect(t, "middle", middle.Rect, 30, 15, 80, 60)
	wantRect(t, "inner", inner.Rect, 70, 50, 30, 20)
	if middle.ContentSize != (gpui.Size{W: 100, H: 70}) {
		t.Errorf("overflow = %v", middle.ContentSize)
	}
	outside := box(50, 40)
	outside.Style.Position, outside.Style.Left, outside.Style.Top = gpui.PositionAbsolute, gpui.Px(180), gpui.Px(90)
	root.Children = append(root.Children, outside)
	gpui.Layout(root, gpui.Definite(200, 100))
	if w, h := root.ScrollMax(); w != 30 || h != 30 {
		t.Errorf("ScrollMax = %v,%v", w, h)
	}
}

func TestAbsoluteOverlapPaintOrder(t *testing.T) {
	for _, absoluteFirst := range []bool{false, true} {
		root := gpui.RenderFunc(func(_ *gpui.Window, _ *gpui.App) gpui.IntoElement {
			a := gpui.NewDiv().Absolute().Inset(10).Bg(gpui.Red500)
			b := gpui.NewDiv().SizeWH(50, 50).Bg(gpui.Blue500)
			if absoluteFirst {
				return gpui.NewDiv().SizeWH(60, 60).Child(a, b)
			}
			return gpui.NewDiv().SizeWH(60, 60).Child(b, a)
		})
		img, err := gpui.RenderToImage(60, 60, 1, root)
		if err != nil {
			t.Fatal(err)
		}
		want := gpui.Red500
		if absoluteFirst {
			want = gpui.Blue500
		}
		r, g, b, _ := img.At(20, 20).RGBA()
		if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B {
			t.Errorf("absoluteFirst=%v: pixel=%v want=%v", absoluteFirst, img.At(20, 20), want)
		}
	}
}

func TestAbsoluteBuilders(t *testing.T) {
	s := gpui.NewDiv().Absolute().Inset(5).InsetX(10).InsetY(20).Top(1).Right(2).Bottom(3).Left(4).ResolvedStyle(gpui.DefaultStyle())
	if s.Position != gpui.PositionAbsolute || s.Top != gpui.Px(1) || s.Right != gpui.Px(2) || s.Bottom != gpui.Px(3) || s.Left != gpui.Px(4) {
		t.Fatalf("style = %+v", s)
	}
	s = gpui.NewText("x").Absolute().Inset(5).Relative().ResolvedStyle(gpui.DefaultStyle())
	if s.Position != gpui.PositionRelative {
		t.Fatal("Relative did not reset Position")
	}
}
