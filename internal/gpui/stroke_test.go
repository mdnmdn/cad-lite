package gpui

import "testing"

func TestStrokeHorizontalLineCoversItsRow(t *testing.T) {
	r := NewRasterizer(20, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertStroke(Stroke{
		ContentMask: InfiniteContentMask(),
		Points:      []Point{{X: 2, Y: 5}, {X: 18, Y: 5}},
		Width:       2,
		Color:       Rgba{255, 0, 0, 255},
	})
	r.Render(&s)
	assertPixelNear(t, r, 10, 4, [4]uint8{255, 0, 0, 255}, 0, "row above the centre line")
	assertPixelNear(t, r, 10, 5, [4]uint8{255, 0, 0, 255}, 0, "row below the centre line")
	assertPixelNear(t, r, 10, 2, [4]uint8{}, 0, "outside the stroke")
	assertPixelNear(t, r, 19, 5, [4]uint8{}, 0, "past the round cap")
}

func TestStrokeJointBlendsOnce(t *testing.T) {
	r := NewRasterizer(20, 20)
	r.Reset(Transparent)
	var s Scene
	s.InsertStroke(Stroke{
		ContentMask: InfiniteContentMask(),
		Points:      []Point{{X: 2, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 2}},
		Width:       2,
		Color:       Rgba{0, 0, 255, 128},
	})
	r.Render(&s)
	joint := pixelAtRGBA(t, r, 9, 9)
	arm := pixelAtRGBA(t, r, 5, 9)
	if joint != arm {
		t.Fatalf("joint pixel %v differs from arm pixel %v: the joint was blended twice", joint, arm)
	}
}

func TestStrokeHairlineAndClip(t *testing.T) {
	r := NewRasterizer(20, 20)
	r.Reset(Transparent)
	var s Scene
	s.InsertStroke(Stroke{
		ContentMask: ContentMask{Bounds: NewBounds(0, 0, 10, 20)},
		Points:      []Point{{X: 0, Y: 10.5}, {X: 20, Y: 10.5}},
		Color:       Rgba{0, 0, 0, 255},
	})
	r.Render(&s)
	assertPixelNear(t, r, 5, 10, [4]uint8{0, 0, 0, 255}, 0, "hairline on its pixel row")
	assertPixelNear(t, r, 5, 9, [4]uint8{}, 0, "hairline is one pixel wide")
	assertPixelNear(t, r, 15, 10, [4]uint8{}, 0, "clipped half")
}

func TestStrokeScalesWithScene(t *testing.T) {
	var s, out Scene
	s.InsertStroke(Stroke{Points: []Point{{X: 1, Y: 2}, {X: 3, Y: 4}}, Width: 1.5})
	got := scaleScene(&s, 2, &out).Strokes[0]
	if got.Points[1] != (Point{X: 6, Y: 8}) || got.Width != 3 {
		t.Fatalf("scaled stroke = %+v", got)
	}
	if s.Strokes[0].Points[1] != (Point{X: 3, Y: 4}) {
		t.Fatal("scaleScene modified the source points")
	}
}

func TestStrokeOrderInterleavesWithQuads(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertStroke(Stroke{ContentMask: InfiniteContentMask(), Points: []Point{{X: 0, Y: 5}, {X: 10, Y: 5}}, Width: 4, Color: Rgba{255, 0, 0, 255}})
	s.InsertQuad(Quad{Bounds: NewBounds(0, 0, 10, 10), ContentMask: InfiniteContentMask(), Background: BackgroundSolid, Fill: Rgba{0, 255, 0, 255}})
	r.Render(&s)
	assertPixelNear(t, r, 5, 5, [4]uint8{0, 255, 0, 255}, 0, "later quad covers the stroke")
}

func TestCanvasPaintsWithItsBounds(t *testing.T) {
	var got Bounds
	root := RenderFunc(func(*Window, *App) IntoElement {
		return NewDiv().SizeFull().P(10).Child(NewCanvas(func(b Bounds, w *Window, _ *App) {
			got = b
			w.PaintStroke(Stroke{Points: []Point{b.Origin, b.Max()}, Color: Black})
		}))
	})
	p := NewHeadlessPlatform()
	var hw *HeadlessWindow
	_ = p.Run(func(cx *App) {
		if err := cx.OpenWindow(WindowOptions{Width: 100, Height: 60}, func(*Window, *App) EntityRoot { return root }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	})
	if got != NewBounds(10, 10, 80, 40) {
		t.Fatalf("canvas bounds = %v, want 10,10 80x40", got)
	}
	if n := len(hw.Window().Scene().Strokes); n != 1 {
		t.Fatalf("strokes = %d, want 1", n)
	}
}
