package gpui

import "testing"

// pixelAtRGBA reads the premultiplied RGBA bytes at (x, y) from the buffer.
func pixelAtRGBA(t *testing.T, r *Rasterizer, x, y int) [4]uint8 {
	t.Helper()
	w, h := r.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		t.Fatalf("pixelAtRGBA: (%d,%d) out of bounds %dx%d", x, y, w, h)
	}
	pix := r.Pixels()
	i := 4*(y*w) + 4*x
	return [4]uint8{pix[i], pix[i+1], pix[i+2], pix[i+3]}
}

func assertPixelNear(t *testing.T, r *Rasterizer, x, y int, want [4]uint8, tol int, msg string) {
	t.Helper()
	got := pixelAtRGBA(t, r, x, y)
	for c := 0; c < 4; c++ {
		d := int(got[c]) - int(want[c])
		if d < 0 {
			d = -d
		}
		if d > tol {
			t.Fatalf("%s: pixel (%d,%d) = %v, want %v ±%d", msg, x, y, got, want, tol)
		}
	}
}

func TestRasterRenderEmptySceneIsAllZero(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	r.Render(&Scene{})
	for _, b := range r.Pixels() {
		if b != 0 {
			t.Fatalf("expected all-zero buffer, found byte %d", b)
		}
	}
}

func TestRasterSolidOpaqueRedQuad(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertQuad(Quad{
		Bounds:      NewBounds(0, 0, 10, 10),
		ContentMask: InfiniteContentMask(),
		Background:  BackgroundSolid,
		Fill:        Rgba{255, 0, 0, 255},
	})
	r.Render(&s)

	want := [4]uint8{255, 0, 0, 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := pixelAtRGBA(t, r, x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestRasterContentMaskClip(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertQuad(Quad{
		Bounds:      NewBounds(0, 0, 10, 10),
		ContentMask: ContentMask{Bounds: NewBounds(0, 0, 5, 5)},
		Background:  BackgroundSolid,
		Fill:        Rgba{0, 0, 255, 255},
	})
	r.Render(&s)

	blue := [4]uint8{0, 0, 255, 255}
	zero := [4]uint8{0, 0, 0, 0}
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			if got := pixelAtRGBA(t, r, x, y); got != blue {
				t.Fatalf("clipped-in pixel (%d,%d) = %v, want %v", x, y, got, blue)
			}
		}
	}
	for _, p := range [][2]int{{5, 0}, {0, 5}, {9, 9}, {5, 5}, {9, 0}, {0, 9}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got != zero {
			t.Fatalf("clipped-out pixel (%d,%d) = %v, want clear", p[0], p[1], got)
		}
	}
}

func TestRasterAlphaBlendWhiteOverRed(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Rgba{255, 0, 0, 255})
	var s Scene
	s.InsertQuad(Quad{
		Bounds:      NewBounds(0, 0, 5, 10),
		ContentMask: InfiniteContentMask(),
		Background:  BackgroundSolid,
		Fill:        Rgba{255, 255, 255, 128},
	})
	r.Render(&s)

	// premul(white,128) = {128,128,128,128} over premultiplied red.
	pm := Rgba{255, 255, 255, 128}.Premultiply()
	sa := float32(pm.A) / 255
	want := [4]uint8{
		uint8(float32(pm.R) + 255*(1-sa) + 0.5),
		uint8(float32(pm.G) + 0*(1-sa) + 0.5),
		uint8(float32(pm.B) + 0*(1-sa) + 0.5),
		uint8(float32(pm.A) + 255*(1-sa) + 0.5),
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 5; x++ {
			assertPixelNear(t, r, x, y, want, 2, "blended left half")
		}
	}
	red := [4]uint8{255, 0, 0, 255}
	for y := 0; y < 10; y++ {
		for x := 5; x < 10; x++ {
			if got := pixelAtRGBA(t, r, x, y); got != red {
				t.Fatalf("right half pixel (%d,%d) = %v, want %v", x, y, got, red)
			}
		}
	}
}

func TestRasterRoundedCornerAntiAliasing(t *testing.T) {
	r := NewRasterizer(20, 20)
	r.Reset(Transparent)
	var s Scene
	s.InsertQuad(Quad{
		Bounds:      NewBounds(0, 0, 20, 20),
		ContentMask: InfiniteContentMask(),
		Background:  BackgroundSolid,
		Fill:        Rgba{255, 255, 255, 255},
		CornerRadii: Corners{TopLeft: 10, TopRight: 10, BottomLeft: 10, BottomRight: 10},
	})
	r.Render(&s)

	if got := pixelAtRGBA(t, r, 10, 10); got != [4]uint8{255, 255, 255, 255} {
		t.Fatalf("center pixel = %v, want opaque white", got)
	}
	for _, p := range [][2]int{{0, 0}, {19, 0}, {0, 19}, {19, 19}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got[3] != 0 {
			t.Fatalf("corner pixel (%d,%d) alpha = %d, want 0", p[0], p[1], got[3])
		}
	}
	if got := pixelAtRGBA(t, r, 10, 0); got[3] == 0 || got[3] == 255 {
		t.Fatalf("boundary pixel (10,0) alpha = %d, want strictly between 0 and 255", got[3])
	}
}

func TestRasterBorderRing(t *testing.T) {
	r := NewRasterizer(20, 20)
	r.Reset(Transparent)
	var s Scene
	s.InsertQuad(Quad{
		Bounds:       NewBounds(0, 0, 20, 20),
		ContentMask:  InfiniteContentMask(),
		Background:   BackgroundNone,
		BorderColor:  Rgba{0, 0, 0, 255},
		BorderWidths: Edges{Top: 3, Right: 3, Bottom: 3, Left: 3},
	})
	r.Render(&s)

	if got := pixelAtRGBA(t, r, 10, 10); got != [4]uint8{0, 0, 0, 0} {
		t.Fatalf("center pixel = %v, want transparent", got)
	}
	black := [4]uint8{0, 0, 0, 255}
	for _, p := range [][2]int{{1, 10}, {10, 1}, {18, 10}, {10, 18}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got != black {
			t.Fatalf("border pixel (%d,%d) = %v, want opaque black", p[0], p[1], got)
		}
	}
}

func TestRasterGlyphSprite(t *testing.T) {
	mask := &GlyphMask{
		Pix: []uint8{
			0, 0, 0, 0,
			0, 255, 255, 0,
			0, 255, 255, 0,
			0, 0, 0, 0,
		},
		W: 4,
		H: 4,
	}
	r := NewRasterizer(8, 8)
	r.Reset(Transparent)
	var s Scene
	s.InsertSprite(GlyphSprite{
		Bounds:      NewBounds(0, 0, 8, 8),
		ContentMask: InfiniteContentMask(),
		Mask:        mask,
		Color:       Rgba{255, 0, 0, 255},
	})
	r.Render(&s)

	red := [4]uint8{255, 0, 0, 255}
	// The true center of the 8×8 dest samples the heart of the opaque 2×2
	// texel block and comes out fully tinted.
	if got := pixelAtRGBA(t, r, 3, 3); got != red {
		t.Fatalf("center pixel (3,3) = %v, want tinted red", got)
	}
	// Bilinear ramps the mask edges, so neighbouring center pixels are
	// strongly but not fully tinted.
	for _, p := range [][2]int{{3, 4}, {4, 3}, {4, 4}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got[3] < 140 || got[0] == 0 {
			t.Fatalf("center pixel (%d,%d) = %v, want strongly tinted", p[0], p[1], got)
		}
	}
	// Bilinear resampling bleeds at most a fraction of a texel past the
	// opaque block (the (0,0) corner reaches ~16/255), so corners are only
	// weakly tinted — far below the fully tinted center.
	for _, p := range [][2]int{{0, 0}, {7, 0}, {0, 7}, {7, 7}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got[3] > 32 {
			t.Fatalf("corner pixel (%d,%d) = %v, want at most faint bilinear bleed", p[0], p[1], got)
		}
	}
}

func TestRasterGlyphSpriteUpscaleOnePixel(t *testing.T) {
	mask := &GlyphMask{Pix: []uint8{255}, W: 1, H: 1}
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertSprite(GlyphSprite{
		Bounds:      NewBounds(0, 0, 10, 10),
		ContentMask: InfiniteContentMask(),
		Mask:        mask,
		Color:       Rgba{255, 0, 0, 255},
	})
	r.Render(&s)

	want := [4]uint8{255, 0, 0, 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := pixelAtRGBA(t, r, x, y); got != want {
				t.Fatalf("upscaled pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestRasterUnderlineBottomAnchored(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Transparent)
	var s Scene
	s.InsertUnderline(Underline{
		Bounds:      NewBounds(0, 0, 10, 10),
		ContentMask: InfiniteContentMask(),
		Color:       Rgba{255, 0, 0, 255},
		Thickness:   2,
	})
	r.Render(&s)

	red := [4]uint8{255, 0, 0, 255}
	for _, p := range [][2]int{{0, 8}, {5, 9}, {9, 8}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got != red {
			t.Fatalf("underline pixel (%d,%d) = %v, want opaque red", p[0], p[1], got)
		}
	}
	for _, p := range [][2]int{{0, 7}, {5, 0}, {9, 7}} {
		if got := pixelAtRGBA(t, r, p[0], p[1]); got != [4]uint8{0, 0, 0, 0} {
			t.Fatalf("non-underline pixel (%d,%d) = %v, want transparent", p[0], p[1], got)
		}
	}
}

func TestRasterImageUnpremultiplies(t *testing.T) {
	r := NewRasterizer(10, 10)
	r.Reset(Rgba{255, 0, 0, 255})
	img := r.Image()
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Fatalf("Image bounds = %v, want 10×10", img.Bounds())
	}
	c := img.NRGBAAt(2, 2)
	if c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Fatalf("opaque red image pixel = %v, want {255 0 0 255}", c)
	}

	// Straight {255,0,0,128} premultiplies to exactly {128,0,0,128} in the buffer.
	r.Reset(Rgba{255, 0, 0, 128})
	if got := pixelAtRGBA(t, r, 0, 0); got != [4]uint8{128, 0, 0, 128} {
		t.Fatalf("premultiplied buffer = %v, want {128 0 0 128}", got)
	}
	img = r.Image()
	c = img.NRGBAAt(0, 0)
	if diff := int(c.R) - 255; diff > 2 || diff < -2 {
		t.Fatalf("un-premultiplied R = %d, want ~255", c.R)
	}
	if c.G != 0 || c.B != 0 {
		t.Fatalf("un-premultiplied GB = %d,%d, want 0,0", c.G, c.B)
	}
	if diff := int(c.A) - 128; diff > 2 || diff < -2 {
		t.Fatalf("un-premultiplied A = %d, want ~128", c.A)
	}
}

// refFillRounded and refStrokeBorder evaluate the SDF at every pixel of the
// clip window: the straightforward rasterization the span-optimized fast
// paths must reproduce byte for byte.
func refFillRounded(r *Rasterizer, b Bounds, radii Corners, clip Bounds, c Rgba) {
	x0, y0, x1, y1, ok := r.clipWindow(clip)
	if !ok {
		return
	}
	pm := c.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if cov := roundedRectCoverage(b, radii, float32(x)+0.5, float32(y)+0.5); cov > 0 {
				r.blend(x, y, pr*cov, pg*cov, pb*cov, pa*cov)
			}
		}
	}
}

func refStrokeBorder(r *Rasterizer, b Bounds, radii Corners, bw Edges, clip Bounds, c Rgba) {
	inner := b.Inset(bw)
	innerRadii := Corners{
		TopLeft:     maxPixels(0, radii.TopLeft-minPixels(bw.Left, bw.Top)),
		TopRight:    maxPixels(0, radii.TopRight-minPixels(bw.Right, bw.Top)),
		BottomLeft:  maxPixels(0, radii.BottomLeft-minPixels(bw.Left, bw.Bottom)),
		BottomRight: maxPixels(0, radii.BottomRight-minPixels(bw.Right, bw.Bottom)),
	}
	x0, y0, x1, y1, ok := r.clipWindow(clip)
	if !ok {
		return
	}
	pm := c.Premultiply()
	pr, pg, pb, pa := float32(pm.R), float32(pm.G), float32(pm.B), float32(pm.A)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			px, py := float32(x)+0.5, float32(y)+0.5
			cov := roundedRectCoverage(b, radii, px, py)
			if cov <= 0 {
				continue
			}
			if !inner.IsEmpty() {
				cov -= roundedRectCoverage(inner, innerRadii, px, py)
			}
			if cov <= 0 {
				continue
			}
			r.blend(x, y, pr*min(cov, 1), pg*min(cov, 1), pb*min(cov, 1), pa*min(cov, 1))
		}
	}
}

func TestRoundedFastPathsMatchReference(t *testing.T) {
	seed := uint32(12345)
	rnd := func(n float32) float32 {
		seed = seed*1664525 + 1013904223
		return float32(seed>>8) / float32(1<<24) * n
	}
	colors := []Rgba{RgbaOf(30, 120, 220, 255), RgbaOf(200, 40, 40, 128), RgbaOf(10, 200, 90, 77)}
	for i := 0; i < 400; i++ {
		b := NewBounds(rnd(40)-5, rnd(40)-5, rnd(70), rnd(70))
		radii := Corners{TopLeft: Pixels(rnd(30)), TopRight: Pixels(rnd(30)), BottomLeft: Pixels(rnd(30)), BottomRight: Pixels(rnd(30))}
		if i%4 == 0 {
			radii = Corners{}
		}
		bw := Edges{Top: Pixels(rnd(6)), Right: Pixels(rnd(6)), Bottom: Pixels(rnd(6)), Left: Pixels(rnd(6))}
		clip := NewBounds(rnd(20), rnd(20), 30+rnd(70), 30+rnd(70))
		if i%3 == 0 {
			clip = InfiniteContentMask().Bounds
		}
		c := colors[i%len(colors)]
		got, want := NewRasterizer(90, 90), NewRasterizer(90, 90)
		got.Reset(Gray(240))
		want.Reset(Gray(240))
		got.fillRounded(b, radii, clip, c)
		refFillRounded(want, b, radii, clip, c)
		got.strokeBorder(b, radii, bw, clip, colors[(i+1)%len(colors)])
		refStrokeBorder(want, b, radii, bw, clip, colors[(i+1)%len(colors)])
		for j := range got.pix {
			if got.pix[j] != want.pix[j] {
				t.Fatalf("case %d: b=%v radii=%v bw=%v clip=%v: byte %d (px %d,%d) = %d, want %d",
					i, b, radii, bw, clip, j, (j/4)%90, j/4/90, got.pix[j], want.pix[j])
			}
		}
	}
}
