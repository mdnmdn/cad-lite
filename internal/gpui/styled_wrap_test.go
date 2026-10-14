package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func TestDivChainWrappers(t *testing.T) {
	c := gpui.Red500
	d := gpui.NewDiv().
		Hidden().Visible().SizeFull().SizeWH(1, 2).SizeW(3).SizeH(4).
		FlexGrow(2).FlexShrink(3).Flex1().Flex().FlexRow().FlexCol().Gap(5).
		P(1).Px_(6).Py_(7).Pt(8).Pr(9).Pb(10).Pl(11).M(12).
		Bg(c).TextColor(c).TextSize(20).FontWeight(700).FontFamily("x").
		TextStrikethrough().TextLineHeight(30).Rounded(2).RoundedAll(1, 2, 3, 4).
		Border(1, c).ItemsStart().ItemsCenter().ItemsEnd().ItemsStretch().
		JustifyStart().JustifyCenter().JustifyEnd().JustifyBetween().JustifyAround().
		OverflowHidden().OverflowScroll().MinW(1).MinH(2).MaxW(3).MaxH(4).
		ID("x")
	s := d.StyleRef()
	if *s.Justify != gpui.JustifySpaceAround || *s.AlignItems != gpui.AlignStretch {
		t.Errorf("justify/align not applied: %v %v", *s.Justify, *s.AlignItems)
	}
	if p := *s.Padding; p.Left != 11 || p.Right != 9 || p.Top != 8 || p.Bottom != 10 {
		t.Errorf("padding = %+v", p)
	}
	if m := *s.Margin; m.Left != 12 || m.Top != 12 {
		t.Errorf("margin = %+v", m)
	}
	if s.TextStyle.FontWeight != 700 {
		t.Errorf("FontWeight = %d", s.TextStyle.FontWeight)
	}
	if r := *s.CornerRadii; r.TopLeft != 1 || r.TopRight != 2 || r.BottomRight != 3 || r.BottomLeft != 4 {
		t.Errorf("RoundedAll = %+v", r)
	}

	e := gpui.NewDiv().ItemsEnd().JustifyEnd().JustifyCenter()
	if *e.StyleRef().AlignItems != gpui.AlignEnd || *e.StyleRef().Justify != gpui.JustifyCenter {
		t.Error("ItemsEnd/JustifyCenter chain not applied")
	}
}

func TestTextChainWrappers(t *testing.T) {
	c := gpui.Red500
	x := gpui.NewText("a").
		Hidden().Visible().SizeFull().SizeWH(1, 2).SizeW(3).SizeH(4).
		FlexGrow(2).FlexShrink(3).Flex1().Flex().FlexRow().FlexCol().Gap(5).
		P(1).Px_(6).Py_(7).Pt(8).Pr(9).Pb(10).Pl(11).M(12).
		Bg(c).TextColor(c).TextSize(20).FontWeight(700).FontFamily("x").
		TextStrikethrough().TextLineHeight(30).Rounded(2).RoundedAll(1, 2, 3, 4).
		Border(1, c).ItemsStart().ItemsCenter().ItemsEnd().ItemsStretch().
		JustifyStart().JustifyCenter().JustifyEnd().JustifyBetween().JustifyAround().
		OverflowHidden().OverflowScroll().MinW(1).MinH(2).MaxW(3).MaxH(4).
		Text("b")
	if x.StyleRef().TextStyle.FontWeight != 700 || *x.StyleRef().Justify != gpui.JustifySpaceAround {
		t.Error("Text chain not applied")
	}
}
