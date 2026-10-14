package doc_test

import (
	"testing"

	"cad-lite/doc"
)

func TestACIRGB(t *testing.T) {
	for _, c := range []struct {
		i       int
		r, g, b uint8
	}{
		{1, 255, 0, 0}, {2, 255, 255, 0}, {5, 0, 0, 255}, {7, 255, 255, 255}, {8, 128, 128, 128},
		{250, 51, 51, 51}, {255, 255, 255, 255},
		{10, 255, 0, 0}, // the first generated entry: hue 0, full saturation and value
		{0, 255, 255, 255}, {256, 255, 255, 255}, {-3, 255, 255, 255},
	} {
		if r, g, b := doc.ACIRGB(c.i); r != c.r || g != c.g || b != c.b {
			t.Errorf("ACIRGB(%d) = %d %d %d, want %d %d %d", c.i, r, g, b, c.r, c.g, c.b)
		}
	}
}

func TestNearestACI(t *testing.T) {
	// A colour of the palette maps to an index of the same colour (the lowest, when several share it).
	for i := 1; i <= 255; i++ {
		r, g, b := doc.ACIRGB(i)
		n := doc.NearestACI(r, g, b)
		nr, ng, nb := doc.ACIRGB(n)
		if nr != r || ng != g || nb != b || n > i {
			t.Errorf("NearestACI of index %d (%d %d %d) = %d (%d %d %d)", i, r, g, b, n, nr, ng, nb)
		}
	}
	// True colours AutoCAD converted itself, with the index it wrote to DXF beside each.
	for _, c := range []struct {
		r, g, b uint8
		want    int
	}{{125, 33, 79, 247}, {17, 146, 238, 150}, {252, 193, 135, 31}} {
		if got := doc.NearestACI(c.r, c.g, c.b); got != c.want {
			t.Errorf("NearestACI(%d, %d, %d) = %d, want %d", c.r, c.g, c.b, got, c.want)
		}
	}
}
