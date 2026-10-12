package gpui

import (
	"fmt"
	"strconv"
	"strings"
)

// Rgba is a straight (non-premultiplied) color with 8-bit channels.
type Rgba struct {
	R, G, B, A uint8
}

// Hsla is a hue/saturation/lightness color with float channels.
// H is in degrees [0,360); S, L, A are in [0,1].
type Hsla struct {
	H, S, L, A float32
}

// RgbaOf returns the color with the given channels.
func RgbaOf(r, g, b, a uint8) Rgba { return Rgba{R: r, G: g, B: b, A: a} }

// Gray returns an opaque gray with every channel set to v.
func Gray(v uint8) Rgba { return Rgba{R: v, G: v, B: v, A: 255} }

// ParseHex parses "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa".
func ParseHex(s string) (Rgba, error) {
	t := strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(t) {
	case 3, 4:
		vals := make([]uint8, len(t))
		for i := 0; i < len(t); i++ {
			v, err := strconv.ParseUint(string(t[i]), 16, 8)
			if err != nil {
				return Rgba{}, fmt.Errorf("gpui: invalid hex color %q", s)
			}
			vals[i] = uint8(v) * 17
		}
		if len(t) == 3 {
			return Rgba{R: vals[0], G: vals[1], B: vals[2], A: 255}, nil
		}
		return Rgba{R: vals[0], G: vals[1], B: vals[2], A: vals[3]}, nil
	case 6, 8:
		v, err := strconv.ParseUint(t, 16, 64)
		if err != nil {
			return Rgba{}, fmt.Errorf("gpui: invalid hex color %q", s)
		}
		if len(t) == 6 {
			return Rgba{
				R: uint8(v >> 16),
				G: uint8(v >> 8),
				B: uint8(v),
				A: 255,
			}, nil
		}
		return Rgba{
			R: uint8(v >> 24),
			G: uint8(v >> 16),
			B: uint8(v >> 8),
			A: uint8(v),
		}, nil
	default:
		return Rgba{}, fmt.Errorf("gpui: invalid hex color %q", s)
	}
}

// MustParseHex is ParseHex for compile-time-known literals; it panics on error.
func MustParseHex(s string) Rgba {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// Premultiply returns the color with RGB scaled by alpha. The result is still
// stored in an Rgba but is meant for premultiplied upload paths.
func (c Rgba) Premultiply() Rgba {
	if c.A == 255 {
		return c
	}
	if c.A == 0 {
		return Rgba{A: 0}
	}
	a := uint32(c.A)
	return Rgba{
		R: uint8((uint32(c.R)*a + 127) / 255),
		G: uint8((uint32(c.G)*a + 127) / 255),
		B: uint8((uint32(c.B)*a + 127) / 255),
		A: c.A,
	}
}

// Opacity multiplies the alpha channel by f (clamped to [0,1]).
func (c Rgba) Opacity(f float32) Rgba {
	if f >= 1 {
		return c
	}
	if f <= 0 {
		return Rgba{}
	}
	return Rgba{
		R: c.R,
		G: c.G,
		B: c.B,
		A: uint8(float32(c.A)*f + 0.5),
	}
}

// IsTransparent reports whether the alpha channel is zero.
func (c Rgba) IsTransparent() bool { return c.A == 0 }

// ToHsla converts c to hue/saturation/lightness form.
func (c Rgba) ToHsla() Hsla {
	r := float32(c.R) / 255
	g := float32(c.G) / 255
	b := float32(c.B) / 255
	maxc := maxf32(r, maxf32(g, b))
	minc := minf32(r, minf32(g, b))
	l := (maxc + minc) / 2
	var h, s float32
	if maxc == minc {
		h, s = 0, 0
	} else {
		d := maxc - minc
		if l > 0.5 {
			s = d / (2 - maxc - minc)
		} else {
			s = d / (maxc + minc)
		}
		switch maxc {
		case r:
			h = (g - b) / d
			if g < b {
				h += 6
			}
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h *= 60
	}
	return Hsla{H: h, S: s, L: l, A: float32(c.A) / 255}
}

// ToRgba converts c to 8-bit RGBA.
func (c Hsla) ToRgba() Rgba {
	if c.S == 0 {
		v := uint8(clamp32(c.L, 0, 1) * 255)
		return Rgba{R: v, G: v, B: v, A: uint8(clamp32(c.A, 0, 1) * 255)}
	}
	var q float32
	if c.L < 0.5 {
		q = c.L * (1 + c.S)
	} else {
		q = c.L + c.S - c.L*c.S
	}
	p := 2*c.L - q
	return Rgba{
		R: uint8(hueToRGB(p, q, c.H+1.0/3.0) * 255),
		G: uint8(hueToRGB(p, q, c.H) * 255),
		B: uint8(hueToRGB(p, q, c.H-1.0/3.0) * 255),
		A: uint8(clamp32(c.A, 0, 1) * 255),
	}
}

func hueToRGB(p, q, t float32) float32 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}

func maxf32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minf32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// Palette colors. White, Black and Transparent are fixed; GrayN is a neutral
// ramp from light (50) to dark (900); the other names are Material-style
// accents with the shade number as suffix.
var (
	White       = Rgba{R: 255, G: 255, B: 255, A: 255}
	Black       = Rgba{A: 255}
	Transparent = Rgba{}

	Gray50  = MustParseHex("#fafafa")
	Gray100 = MustParseHex("#f5f5f5")
	Gray200 = MustParseHex("#eeeeee")
	Gray300 = MustParseHex("#e0e0e0")
	Gray400 = MustParseHex("#bdbdbd")
	Gray500 = MustParseHex("#9e9e9e")
	Gray600 = MustParseHex("#757575")
	Gray700 = MustParseHex("#616161")
	Gray800 = MustParseHex("#424242")
	Gray900 = MustParseHex("#212121")

	Blue500   = MustParseHex("#2196f3")
	Blue600   = MustParseHex("#1e88e5")
	Red500    = MustParseHex("#f44336")
	Red600    = MustParseHex("#e53935")
	Green600  = MustParseHex("#43a047")
	Yellow400 = MustParseHex("#ffca28")
)
