package appicon

import "testing"

func TestImages(t *testing.T) {
	imgs := Images()
	want := []int{16, 32, 48, 64, 128, 256}
	if len(imgs) != len(want) {
		t.Fatalf("got %d images, want %d", len(imgs), len(want))
	}
	for i, w := range want {
		if b := imgs[i].Bounds(); b.Dx() != w || b.Dy() != w {
			t.Errorf("image %d is %v, want %dx%d", i, b, w, w)
		}
	}
}
