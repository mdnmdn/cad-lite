package gpui_test

import (
	"image/color"
	"testing"

	"cad-lite/internal/gpui"
)

func renderScene(w, h int, s *gpui.Scene) *gpui.Rasterizer {
	r := gpui.NewRasterizer(w, h)
	r.Reset(gpui.White)
	r.Render(s)
	return r
}

func px(r *gpui.Rasterizer, x, y int) color.NRGBA { return r.Image().NRGBAAt(x, y) }

func near(a, b uint8, tol int) bool {
	d := int(a) - int(b)
	return d >= -tol && d <= tol
}

func fullMask() gpui.ContentMask { return gpui.InfiniteContentMask() }

func TestLinearGradientAngles(t *testing.T) {
	red, blue := gpui.RgbaOf(255, 0, 0, 255), gpui.RgbaOf(0, 0, 255, 255)
	paint := func(bg gpui.Background) *gpui.Rasterizer {
		var s gpui.Scene
		s.InsertQuad(gpui.Quad{
			Bounds: gpui.NewBounds(0, 0, 100, 100), ContentMask: fullMask(),
			Background: gpui.BackgroundLinearGradient, Gradient: bg.Gradient,
		})
		return renderScene(100, 100, &s)
	}
	// 90deg runs left (from) to right (to).
	r := paint(gpui.LinearGradient(90, gpui.LinearColorStop(red, 0), gpui.LinearColorStop(blue, 1)))
	if c := px(r, 0, 50); !(c.R > 245 && c.B < 10) {
		t.Errorf("90deg left = %v, want red", c)
	}
	if c := px(r, 99, 50); !(c.B > 245 && c.R < 10) {
		t.Errorf("90deg right = %v, want blue", c)
	}
	if c := px(r, 50, 50); !near(c.R, 127, 4) || !near(c.B, 127, 4) {
		t.Errorf("90deg middle = %v, want an even mix", c)
	}
	// 0deg runs bottom (from) to top (to).
	r = paint(gpui.LinearGradient(0, gpui.LinearColorStop(red, 0), gpui.LinearColorStop(blue, 1)))
	if c := px(r, 50, 99); !(c.R > 245 && c.B < 10) {
		t.Errorf("0deg bottom = %v, want red", c)
	}
	if c := px(r, 50, 0); !(c.B > 245 && c.R < 10) {
		t.Errorf("0deg top = %v, want blue", c)
	}
	// Coincident stops give a hard edge.
	r = paint(gpui.LinearGradient(90, gpui.LinearColorStop(blue, 0.5), gpui.LinearColorStop(red, 0.5)))
	if c := px(r, 48, 50); c.B != 255 || c.R != 0 {
		t.Errorf("hard stop left = %v, want pure blue", c)
	}
	if c := px(r, 52, 50); c.R != 255 || c.B != 0 {
		t.Errorf("hard stop right = %v, want pure red", c)
	}
	// Oklab interpolates differently from sRGB.
	g := gpui.LinearGradient(90, gpui.LinearColorStop(red, 0), gpui.LinearColorStop(blue, 1))
	srgb, oklab := px(paint(g), 50, 50), px(paint(g.ColorSpace(gpui.ColorSpaceOklab)), 50, 50)
	if srgb == oklab {
		t.Errorf("oklab midpoint %v equals srgb midpoint", oklab)
	}
	if gpui.ColorSpaceOklab.String() != "Oklab" || gpui.ColorSpaceSrgb.String() != "sRGB" {
		t.Error("ColorSpace.String")
	}
}

func TestShadowRaster(t *testing.T) {
	black := gpui.RgbaOf(0, 0, 0, 255)
	var s gpui.Scene
	s.InsertShadow(gpui.Shadow{
		Bounds: gpui.NewBounds(40, 40, 40, 40), ContentMask: fullMask(),
		Color: black, BlurRadius: 10,
	})
	r := renderScene(120, 120, &s)
	center := px(r, 60, 60)
	if center.R > 5 {
		t.Errorf("shadow center = %v, want ~black", center)
	}
	edge := px(r, 40, 60)
	if !near(edge.R, 128, 12) {
		t.Errorf("shadow edge = %v, want ~50%% (gaussian edge)", edge)
	}
	if c := px(r, 40-16, 60); c.R != 255 {
		t.Errorf("shadow beyond 3 sigma = %v, want untouched", c)
	}
	prev := uint8(0)
	for x := 40; x > 25; x-- {
		c := px(r, x, 60).R
		if c < prev {
			t.Fatalf("shadow not monotonic at x=%d: %d < %d", x, c, prev)
		}
		prev = c
	}

	// Inset: dark at the element edge, clear in the middle, nothing outside.
	s = gpui.Scene{}
	s.InsertShadow(gpui.Shadow{
		Bounds: gpui.NewBounds(20, 20, 60, 60), ContentMask: fullMask(),
		Color: black, BlurRadius: 8, Inset: true,
		Clip: gpui.NewBounds(10, 10, 80, 80),
	})
	r = renderScene(100, 100, &s)
	if c := px(r, 11, 50); c.R > 30 {
		t.Errorf("inset edge = %v, want dark", c)
	}
	if c := px(r, 50, 50); c.R < 250 {
		t.Errorf("inset middle = %v, want clear", c)
	}
	if c := px(r, 5, 50); c.R != 255 {
		t.Errorf("outside inset element = %v, want untouched", c)
	}
}

func TestDivShadowAndGradientPaint(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		card := gpui.NewDiv().SizeWH(60, 40).Bg(gpui.White).Rounded(8).ShadowLg()
		grad := gpui.NewDiv().SizeWH(60, 40).Bg(gpui.LinearGradient(90,
			gpui.LinearColorStop(gpui.Black, 0), gpui.LinearColorStop(gpui.White, 1)))
		return gpui.NewDiv().SizeFull().Bg(gpui.White).P(30).Gap(40).Child(card, grad)
	})
	hw, _ := openBench(t, 200, 220, 1, root)
	img := hw.Image()
	below, above := img.NRGBAAt(60, 30+40+6), img.NRGBAAt(60, 30-3)
	if below.R >= 250 {
		t.Errorf("below card = %v, want shadowed", below)
	}
	if above.R < below.R {
		t.Errorf("shadow above (%v) darker than below (%v); lg offset is downward", above, below)
	}
	sc := hw.Window().Scene()
	if len(sc.Shadows) != 2 {
		t.Errorf("ShadowLg emitted %d shadows, want 2", len(sc.Shadows))
	}
	if l, r := img.NRGBAAt(31, 130), img.NRGBAAt(88, 130); !(l.R < 20 && r.R > 230) {
		t.Errorf("gradient div left %v right %v, want black to white", l, r)
	}
}

func TestOpacitySubtree(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		inner := gpui.NewDiv().SizeWH(20, 20).Bg(gpui.Black).Opacity(0.5)
		outer := gpui.NewDiv().SizeWH(60, 60).Opacity(0.5).Border(2, gpui.Black).Child(inner)
		return gpui.NewDiv().SizeFull().Bg(gpui.White).Child(outer, gpui.NewDiv().SizeWH(10, 10).Bg(gpui.Black))
	})
	hw, _ := openBench(t, 100, 100, 1, root)
	sc := hw.Window().Scene()
	var alphas []uint8
	for _, q := range sc.Quads {
		if q.Background == gpui.BackgroundSolid {
			alphas = append(alphas, q.Fill.A)
		} else {
			alphas = append(alphas, q.BorderColor.A)
		}
	}
	// root bg 255, outer border 128, inner bg 0.25*255, sibling bg 255.
	want := []uint8{255, 128, 64, 255}
	if len(alphas) != len(want) {
		t.Fatalf("alphas = %v, want %v", alphas, want)
	}
	for i := range want {
		if !near(alphas[i], want[i], 1) {
			t.Fatalf("alphas = %v, want %v", alphas, want)
		}
	}
	if c := hw.Image().NRGBAAt(10, 10); !near(c.R, 191, 2) {
		t.Errorf("25%% black on white = %v, want ~191", c)
	}
}

func TestHoverStyleRedraws(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		box := gpui.NewDiv().SizeWH(40, 40).Bg(gpui.Blue500).
			Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) })
		return gpui.NewDiv().SizeFull().Bg(gpui.White).Child(box)
	})
	hw, _ := openBench(t, 100, 100, 1, root)
	at := func() gpui.Rgba {
		c := hw.Image().NRGBAAt(20, 20)
		return gpui.RgbaOf(c.R, c.G, c.B, c.A)
	}
	if at() != gpui.Blue500 {
		t.Fatalf("initial = %v, want blue", at())
	}
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 20, Y: 20}})
	if at() != gpui.Red500 {
		t.Fatalf("hovered = %v, want red", at())
	}
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 80, Y: 80}})
	if at() != gpui.Blue500 {
		t.Fatalf("unhovered = %v, want blue", at())
	}
}
