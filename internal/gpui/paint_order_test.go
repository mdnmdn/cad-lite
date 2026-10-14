package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func solidMask(w, h int) *gpui.GlyphMask {
	pix := make([]uint8, w*h)
	for i := range pix {
		pix[i] = 255
	}
	return &gpui.GlyphMask{Pix: pix, W: w, H: h}
}

func TestRasterPaintsInSceneOrderAcrossKinds(t *testing.T) {
	red, blue, green := gpui.RgbaOf(255, 0, 0, 255), gpui.RgbaOf(0, 0, 255, 255), gpui.RgbaOf(0, 200, 0, 255)
	var s gpui.Scene
	s.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(0, 0, 10, 10), ContentMask: fullMask(), Mask: solidMask(10, 10), Color: red})
	s.InsertUnderline(gpui.Underline{Bounds: gpui.NewBounds(20, 0, 10, 10), ContentMask: fullMask(), Color: green, Thickness: 10})
	s.PushLayer()
	s.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(0, 0, 40, 10), ContentMask: fullMask(), Background: gpui.BackgroundSolid, Fill: blue})
	r := renderScene(40, 10, &s)
	for _, x := range []int{5, 25} {
		if c := px(r, x, 5); c.B != 255 || c.R != 0 || c.G != 0 {
			t.Errorf("pixel %d = %v, want the later quad (blue) on top", x, c)
		}
	}

	// Without the overlay, text still paints over an earlier background.
	var s2 gpui.Scene
	s2.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(0, 0, 40, 10), ContentMask: fullMask(), Background: gpui.BackgroundSolid, Fill: blue})
	s2.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(0, 0, 10, 10), ContentMask: fullMask(), Mask: solidMask(10, 10), Color: red})
	r = renderScene(40, 10, &s2)
	if c := px(r, 5, 5); c.R != 255 || c.B != 0 {
		t.Errorf("text over background = %v, want red", c)
	}
}

func TestOverlayCoversEarlierText(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Child(
			gpui.NewText("WWWWWWWW").TextSize(30).TextColor(gpui.Black),
			// A later sibling pulled up over the text by a negative margin.
			gpui.NewDiv().SizeWH(200, 140).Mt(-100).Bg(gpui.Blue500),
		)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	img := hw.Image()
	want := gpui.Blue500.Premultiply()
	for y := 0; y < 40; y++ {
		for x := 0; x < 200; x++ {
			c := img.NRGBAAt(x, y)
			if c.R != want.R || c.G != want.G || c.B != want.B {
				t.Fatalf("pixel (%d,%d) = %v, want the overlay color (text must not show through)", x, y, c)
			}
		}
	}
}

func TestRoundedContentMaskClipsChildren(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Child(
			gpui.NewDiv().SizeWH(100, 100).OverflowHidden().Rounded(30).Child(
				gpui.NewDiv().SizeWH(100, 100).Bg(gpui.RgbaOf(255, 0, 0, 255)),
			),
		)
	})
	hw, _ := openHeadless(t, 100, 100, root)
	img := hw.Image()
	if c := img.NRGBAAt(2, 2); c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("corner pixel = %v, want white (clipped by the rounded mask)", c)
	}
	if c := img.NRGBAAt(97, 97); c.G != 255 {
		t.Errorf("bottom-right corner pixel = %v, want white", c)
	}
	if c := img.NRGBAAt(50, 50); c.R != 255 || c.G != 0 {
		t.Errorf("center pixel = %v, want red", c)
	}
	if c := img.NRGBAAt(50, 1); c.R != 255 || c.G != 0 {
		t.Errorf("top edge pixel = %v, want red (straight edge is not rounded)", c)
	}
	// The anti-aliased mask: a pixel on the arc is partially covered.
	c := img.NRGBAAt(8, 8)
	if c.G == 0 || c.G == 255 {
		t.Errorf("arc pixel = %v, want a partial blend", c)
	}

	q := hw.Window().Scene().Quads
	found := false
	for _, qq := range q {
		if qq.Fill == gpui.RgbaOf(255, 0, 0, 255) {
			found = true
			if qq.ContentMask.CornerRadii.TopLeft != 30 {
				t.Errorf("child mask radii = %+v, want 30", qq.ContentMask.CornerRadii)
			}
		}
	}
	if !found {
		t.Fatal("child quad missing")
	}
}

func TestRasterRoundedMaskOnSprites(t *testing.T) {
	var s gpui.Scene
	mask := gpui.ContentMask{Bounds: gpui.NewBounds(0, 0, 40, 40), CornerRadii: gpui.Corners{TopLeft: 20, TopRight: 20, BottomLeft: 20, BottomRight: 20}}
	s.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(0, 0, 40, 40), ContentMask: mask, Mask: solidMask(40, 40), Color: gpui.RgbaOf(0, 0, 0, 255)})
	r := renderScene(40, 40, &s)
	if c := px(r, 1, 1); c.R != 255 {
		t.Errorf("masked corner = %v, want white", c)
	}
	if c := px(r, 20, 20); c.R != 0 {
		t.Errorf("center = %v, want black", c)
	}
}

func TestOverflowDoesNotClipOwnShadow(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().P(20).Child(
			gpui.NewDiv().SizeWH(40, 40).OverflowHidden().Bg(gpui.White).
				Shadow(gpui.BoxShadow{Color: gpui.RgbaOf(0, 0, 0, 255), Offset: gpui.Point{X: 10, Y: 10}}),
		)
	})
	hw, _ := openHeadless(t, 100, 100, root)
	if c := hw.Image().NRGBAAt(65, 65); c.R > 10 {
		t.Errorf("shadow pixel outside the element = %v, want black (own shadow is not clipped by overflow)", c)
	}
}
