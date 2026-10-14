package doc

import "math"

// ACIRGB returns the RGB of an AutoCAD Color Index. 1–9 and the grays 250–255 are the standard
// values; 10–249 are generated from the palette's hue/shade layout and match AutoCAD closely but not
// exactly. An index outside 1–255 gives white.
func ACIRGB(i int) (r, g, b uint8) {
	switch {
	case i >= 1 && i <= 9:
		c := aciBase[i]
		return c[0], c[1], c[2]
	case i >= 250 && i <= 255:
		g := [6]uint8{51, 91, 132, 173, 214, 255}[i-250]
		return g, g, g
	case i >= 10 && i <= 249:
		hue := float64((i-10)/10) * 15
		shade := (i - 10) % 10
		v := [5]float64{1, 0.8, 0.6, 0.5, 0.3}[shade/2]
		s := 1.0
		if shade%2 == 1 {
			s = 0.5
		}
		return hsv(hue, s, v)
	}
	return 255, 255, 255
}

// NearestACI returns the AutoCAD Color Index (1–255) whose RGB, by ACIRGB, is closest to (r, g, b)
// in squared RGB distance, the lowest index winning a tie. It is how a true colour becomes an index
// where only indices exist; for the true colours AutoCAD itself converts, it picks the index
// AutoCAD wrote beside them.
func NearestACI(r, g, b uint8) int {
	best, bestD := 7, math.MaxInt
	for i := 1; i <= 255; i++ {
		cr, cg, cb := ACIRGB(i)
		dr, dg, db := int(cr)-int(r), int(cg)-int(g), int(cb)-int(b)
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

var aciBase = [10][3]uint8{
	{0, 0, 0},
	{255, 0, 0},
	{255, 255, 0},
	{0, 255, 0},
	{0, 255, 255},
	{0, 0, 255},
	{255, 0, 255},
	{255, 255, 255},
	{128, 128, 128},
	{192, 192, 192},
}

func hsv(h, s, v float64) (r8, g8, b8 uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	f := func(u float64) uint8 { return uint8(math.Round((u + m) * 255)) }
	return f(r), f(g), f(b)
}
