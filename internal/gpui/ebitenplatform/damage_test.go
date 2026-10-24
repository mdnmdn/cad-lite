package ebitenplatform

import (
	"bytes"
	"image"
	"testing"
)

func TestDamageStripsMergesTileRuns(t *testing.T) {
	const w, h = 300, 200 // 5x4 tiles, the last column 44 px and the last row 8 px
	cases := []struct {
		name  string
		rects []image.Rectangle
		want  []image.Rectangle
	}{
		{"none", nil, nil},
		{"outside", []image.Rectangle{image.Rect(400, 0, 500, 10)}, nil},
		{"one tile", []image.Rectangle{image.Rect(70, 70, 80, 80)}, []image.Rectangle{image.Rect(64, 64, 128, 128)}},
		{"row run", []image.Rectangle{image.Rect(64, 0, 128, 64), image.Rect(130, 10, 140, 20)}, []image.Rectangle{image.Rect(64, 0, 192, 64)}},
		{"two runs", []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(200, 5, 210, 6)}, []image.Rectangle{image.Rect(0, 0, 64, 64), image.Rect(192, 0, 256, 64)}},
		{"column", []image.Rectangle{image.Rect(70, 10, 80, 150)}, []image.Rectangle{image.Rect(64, 0, 128, 192)}},
		{"edges clipped", []image.Rectangle{image.Rect(290, 195, 310, 210)}, []image.Rectangle{image.Rect(256, 192, 300, 200)}},
		{"overlapping updates", []image.Rectangle{image.Rect(0, 0, 128, 64), image.Rect(64, 0, 192, 64), image.Rect(64, 64, 128, 128)},
			[]image.Rectangle{image.Rect(0, 0, 192, 64), image.Rect(64, 64, 128, 128)}},
		{"block", []image.Rectangle{image.Rect(10, 10, 140, 140)}, []image.Rectangle{image.Rect(0, 0, 192, 192)}},
		{"full", []image.Rectangle{image.Rect(0, 0, w, h)}, []image.Rectangle{image.Rect(0, 0, w, h)}},
	}
	var bits []bool
	for _, c := range cases {
		var got []image.Rectangle
		got, bits = damageStrips(nil, bits, c.rects, w, h)
		if len(got) != len(c.want) {
			t.Fatalf("%s: strips %v, want %v", c.name, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: strips %v, want %v", c.name, got, c.want)
			}
		}
		// Every damaged pixel is covered, by exactly one strip.
		for _, r := range c.rects {
			r = r.Intersect(image.Rect(0, 0, w, h))
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					n := 0
					for _, s := range got {
						if image.Pt(x, y).In(s) {
							n++
						}
					}
					if n != 1 {
						t.Fatalf("%s: pixel (%d,%d) covered by %d strips", c.name, x, y, n)
					}
				}
			}
		}
	}
}

func TestPackRect(t *testing.T) {
	const w, h = 7, 5
	pix := make([]byte, 4*w*h)
	for i := range pix {
		pix[i] = byte(i)
	}
	r := image.Rect(2, 1, 5, 4)
	packed, scratch := packRect(nil, pix, w, r)
	if len(packed) != 4*r.Dx()*r.Dy() {
		t.Fatalf("packed %d bytes", len(packed))
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := packed[4*r.Dx()*(y-r.Min.Y):][:4*r.Dx()]
		if !bytes.Equal(row, pix[4*(y*w+r.Min.X):4*(y*w+r.Max.X)]) {
			t.Fatalf("row %d packed wrong", y)
		}
	}
	full := image.Rect(0, 2, w, 4)
	packed, scratch2 := packRect(scratch, pix, w, full)
	if &packed[0] != &pix[4*2*w] || len(packed) != 4*w*2 {
		t.Fatal("a full-width strip should be a sub-slice of the frame")
	}
	if len(scratch2) > 0 && &scratch2[:1][0] == &pix[0] {
		t.Fatal("scratch must never alias the frame")
	}
	before := append([]byte(nil), pix...)
	packRect(scratch2, pix, w, r)
	if !bytes.Equal(before, pix) {
		t.Fatal("packing modified the frame")
	}
}

func TestPresentDamageAccumulatesUntilDraw(t *testing.T) {
	w := &window{}
	pix := make([]byte, 4*100*80)
	w.PresentDamage(pix, 100, 80, []image.Rectangle{image.Rect(0, 0, 100, 80)})
	if !w.fresh || !w.fullUpload {
		t.Fatal("first frame must be a fresh full upload")
	}
	w.fullUpload, w.fresh, w.damage = false, false, w.damage[:0]
	w.PresentDamage(pix, 100, 80, nil)
	if w.fresh {
		t.Fatal("a frame without damage needs no upload")
	}
	w.PresentDamage(pix, 100, 80, []image.Rectangle{image.Rect(1, 1, 2, 2)})
	w.PresentDamage(pix, 100, 80, []image.Rectangle{image.Rect(70, 70, 71, 71)})
	if !w.fresh || w.fullUpload || len(w.damage) != 2 {
		t.Fatalf("damage of two updates before a Draw: fresh %v full %v %v", w.fresh, w.fullUpload, w.damage)
	}
	w.PresentDamage(make([]byte, 4*120*80), 120, 80, nil)
	if !w.fullUpload || !w.fresh {
		t.Fatal("a size change must upload the whole frame")
	}
}
