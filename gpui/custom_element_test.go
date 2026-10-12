package gpui_test

import (
	"testing"

	"go-gpui/gpui"
)

// frame is a custom container built outside the package: it lays its
// children out in a row with a gap and paints a border quad around them.
type frame struct {
	children []gpui.AnyElement
	clicked  *int
}

func (f *frame) IntoElement() gpui.Element { return frameElement{f} }

type frameElement struct{ f *frame }

func (e frameElement) ID() (gpui.ElementID, bool) { return gpui.ElementID{}, false }

func (e frameElement) RequestLayout(w *gpui.Window, cx *gpui.App) (gpui.LayoutID, any) {
	st := gpui.DefaultStyle()
	st.Direction = gpui.DirectionRow
	st.Gap = 10
	st.Padding = gpui.Edges{}.All(5)
	st.AlignItems = gpui.AlignStart
	return w.RequestLayoutWithChildren(st, e.f.children, cx)
}

func (e frameElement) Prepaint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls any) any {
	if ls.(*gpui.ChildLayout).Node == nil {
		panic("missing node")
	}
	return nil
}

func (e frameElement) Paint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls, ps any) {
	w.PaintQuad(gpui.Quad{Bounds: b, BorderColor: gpui.Black, BorderWidths: gpui.Edges{}.All(1)})
}

func (e frameElement) ElementChildren() []gpui.AnyElement { return e.f.children }

func TestCustomElementWithChildren(t *testing.T) {
	clicks := 0
	red, green := gpui.RgbaOf(255, 0, 0, 255), gpui.RgbaOf(0, 255, 0, 255)
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		f := &frame{children: []gpui.AnyElement{
			gpui.NewAnyElement(gpui.NewDiv().SizeWH(20, 20).Bg(red)),
			gpui.NewAnyElement(gpui.NewDiv().SizeWH(30, 10).Bg(green).
				OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { clicks++ })),
		}}
		return gpui.NewDiv().SizeFull().Child(f)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	var got []gpui.Bounds
	for _, q := range hw.Window().Scene().Quads {
		if q.Fill == red || q.Fill == green {
			got = append(got, q.Bounds)
		}
	}
	if len(got) != 2 {
		t.Fatalf("child quads = %v, want 2 (children must be painted)", got)
	}
	wantRect(t, "red", got[0], 5, 5, 20, 20)
	wantRect(t, "green", got[1], 35, 5, 30, 10)
	click(hw, gpui.Point{X: 40, Y: 8})
	if clicks != 1 {
		t.Fatalf("clicks = %d, want 1 (children must be prepainted)", clicks)
	}
	var _ gpui.ParentElementChildren = frameElement{}
}

func TestRegisterLayoutNodeLeaf(t *testing.T) {
	var seen gpui.Bounds
	leaf := leafElement{onPaint: func(b gpui.Bounds) { seen = b }}
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().P(4).Child(leaf)
	})
	openHeadless(t, 100, 100, root)
	wantRect(t, "leaf", seen, 4, 4, 33, 12)
}

type leafElement struct{ onPaint func(gpui.Bounds) }

func (l leafElement) IntoElement() gpui.Element  { return l }
func (l leafElement) ID() (gpui.ElementID, bool) { return gpui.ElementID{}, false }
func (l leafElement) RequestLayout(w *gpui.Window, _ *gpui.App) (gpui.LayoutID, any) {
	st := gpui.DefaultStyle()
	st.Width, st.Height = gpui.Px(33), gpui.Px(12)
	id := w.RegisterLayoutNode(&gpui.LayoutNode{Style: st})
	if w.LayoutNode(id) == nil {
		panic("LayoutNode lookup failed")
	}
	return id, nil
}
func (l leafElement) Prepaint(*gpui.Window, *gpui.App, gpui.ElementID, gpui.Bounds, any) any {
	return nil
}
func (l leafElement) Paint(_ *gpui.Window, _ *gpui.App, _ gpui.ElementID, b gpui.Bounds, _, _ any) {
	l.onPaint(b)
}
