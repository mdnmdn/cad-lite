package gpui

import (
	"bytes"
	"image"
	"testing"
)

func TestTranslucentFillMatchesPerPixelBlend(t *testing.T) {
	for _, alpha := range []uint8{0, 1, 64, 127, 128, 220, 254} {
		for _, radii := range []Corners{{}, {TopLeft: 9, TopRight: 4, BottomLeft: 7, BottomRight: 12}} {
			r := NewRasterizer(80, 64)
			for i := range r.pix {
				r.pix[i] = uint8(i*73 + i/11)
			}
			want := NewRasterizer(80, 64)
			copy(want.pix, r.pix)
			b := Bounds{Origin: Point{X: 1.25, Y: 2.75}, Size: Size{W: 75.5, H: 57.5}}
			clip := Bounds{Origin: Point{X: 3, Y: 4}, Size: Size{W: 70, H: 53}}
			color := RgbaOf(43, 182, 251, alpha)
			pm := color.Premultiply()
			x0, y0, x1, y1, _ := want.coverWindow(b, clip)
			rr := newRRect(b, radii)
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					cov := rr.coverage(float32(x)+0.5, float32(y)+0.5)
					if cov > 0 {
						want.blend(x, y, float32(pm.R)*cov, float32(pm.G)*cov, float32(pm.B)*cov, float32(pm.A)*cov)
					}
				}
			}
			r.fillRounded(b, radii, clip, color)
			if !bytes.Equal(r.pix, want.pix) {
				t.Fatalf("translucent fill changed pixels at alpha %d, radii %v", alpha, radii)
			}
		}
	}
}

func TestUnscaledImageMatchesBilinearPath(t *testing.T) {
	for _, offset := range []Point{{}, {X: -3, Y: 2}, {X: 4, Y: -5}} {
		img := image.NewNRGBA(image.Rect(7, 9, 47, 41))
		for i := range img.Pix {
			img.Pix[i] = uint8(i*61 + i/5)
		}
		for i := 3; i < len(img.Pix); i += 4 {
			if i%7 != 0 {
				img.Pix[i] = 255
			}
		}
		fast, slow := NewRasterizer(48, 40), NewRasterizer(48, 40)
		fast.Reset(RgbaOf(12, 34, 56, 255))
		slow.Reset(RgbaOf(12, 34, 56, 255))
		g := GlyphSprite{Image: img, Bounds: Bounds{Origin: offset, Size: Size{W: 40, H: 32}}, Color: RgbaOf(255, 255, 255, 255), ContentMask: InfiniteContentMask()}
		fast.drawImage(g)
		// A rectangular coverage mask selects the general sampling path without changing pixels.
		slow.masked = true
		slow.mask = newRRect(NewBounds(-100, -100, 300, 300), Corners{})
		slow.band = maskBand{top: -100, bottom: 200, left: -100, right: 200}
		slow.drawImage(g)
		if !bytes.Equal(fast.pix, slow.pix) {
			t.Fatalf("image fast path changed pixels at %v", offset)
		}
	}
}
