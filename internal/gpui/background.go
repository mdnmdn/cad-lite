package gpui

import "math"

// Fill is anything usable as a background: Rgba, Hsla or a Background such
// as LinearGradient(...). It mirrors GPUI's `impl Into<Fill>` arguments.
type Fill interface {
	ToBackground() Background
}

// ToBackground makes a solid background of c.
func (c Rgba) ToBackground() Background { return SolidBackground(c) }

// ToBackground makes a solid background of c.
func (c Hsla) ToBackground() Background { return SolidBackground(c.ToRgba()) }

// ColorSpace selects the space gradient colors are interpolated in.
type ColorSpace uint8

const (
	// ColorSpaceSrgb interpolates sRGB-encoded channels (GPUI's default).
	ColorSpaceSrgb ColorSpace = iota
	// ColorSpaceOklab interpolates in the perceptual Oklab space.
	ColorSpaceOklab
)

// String returns "sRGB" or "Oklab", matching GPUI's Display impl.
func (c ColorSpace) String() string {
	if c == ColorSpaceOklab {
		return "Oklab"
	}
	return "sRGB"
}

// ColorStop is one end of a linear gradient: a color at a position along
// the gradient line. It is GPUI's LinearColorStop; the type is renamed
// because Go cannot have a type and a function sharing one name.
type ColorStop struct {
	Color      Rgba
	Percentage float32 // 0..1 along the gradient line
}

// LinearColorStop returns a gradient stop of c at percentage (0..1).
func LinearColorStop(c Rgba, percentage float32) ColorStop {
	return ColorStop{Color: c, Percentage: percentage}
}

// Gradient is a two-stop linear gradient. Angle is in degrees with CSS
// semantics: 0 points towards the top, 90 towards the right.
type Gradient struct {
	Angle float32
	Stops [2]ColorStop
	Space ColorSpace
}

// Background is a quad fill: nothing, a solid color or a linear gradient.
type Background struct {
	Kind     BackgroundKind
	Solid    Rgba
	Gradient Gradient
}

// SolidBackground returns a solid fill of c.
func SolidBackground(c Rgba) Background {
	return Background{Kind: BackgroundSolid, Solid: c}
}

// LinearGradient returns a linear-gradient fill from one stop to the other
// at angle degrees (0 = towards the top, 90 = towards the right).
func LinearGradient(angle float32, from, to ColorStop) Background {
	return Background{Kind: BackgroundLinearGradient, Gradient: Gradient{Angle: angle, Stops: [2]ColorStop{from, to}}}
}

// ToBackground returns b, so a Background is itself a Fill.
func (b Background) ToBackground() Background { return b }

// ColorSpace returns b interpolating in cs (GPUI's .color_space builder).
func (b Background) ColorSpace(cs ColorSpace) Background {
	b.Gradient.Space = cs
	return b
}

// Opacity returns b with every color's alpha multiplied by f.
func (b Background) Opacity(f float32) Background {
	b.Solid = b.Solid.Opacity(f)
	b.Gradient.Stops[0].Color = b.Gradient.Stops[0].Color.Opacity(f)
	b.Gradient.Stops[1].Color = b.Gradient.Stops[1].Color.Opacity(f)
	return b
}

// IsTransparent reports whether painting b would change no pixel.
func (b Background) IsTransparent() bool {
	switch b.Kind {
	case BackgroundSolid:
		return b.Solid.A == 0
	case BackgroundLinearGradient:
		return b.Gradient.Stops[0].Color.A == 0 && b.Gradient.Stops[1].Color.A == 0
	}
	return true
}

// gradientLUTSize is the number of entries in a gradient's color table.
const gradientLUTSize = 256

// gradientLUT is a gradient's premultiplied colors sampled at t = i/255.
type gradientLUT [gradientLUTSize][4]float32

// buildLUT samples g into a premultiplied color table.
func (g Gradient) buildLUT(lut *gradientLUT) {
	c0, c1 := g.Stops[0].Color, g.Stops[1].Color
	var l0, l1 [3]float32
	if g.Space == ColorSpaceOklab {
		l0, l1 = toOklab(c0), toOklab(c1)
	}
	for i := range lut {
		t := float32(i) / (gradientLUTSize - 1)
		a := lerp(float32(c0.A), float32(c1.A), t)
		var r, gg, b float32
		if g.Space == ColorSpaceOklab {
			r, gg, b = fromOklab(lerp(l0[0], l1[0], t), lerp(l0[1], l1[1], t), lerp(l0[2], l1[2], t))
		} else {
			r = lerp(float32(c0.R), float32(c1.R), t)
			gg = lerp(float32(c0.G), float32(c1.G), t)
			b = lerp(float32(c0.B), float32(c1.B), t)
		}
		k := a / 255
		lut[i] = [4]float32{r * k, gg * k, b * k, a}
	}
}

// gradientMapper maps a pixel center to the gradient parameter t in [0,1],
// following GPUI's shader: the direction's short side is expanded to the
// long one and t is remapped by the stop percentages.
type gradientMapper struct {
	cx, cy, dx, dy float32
	half, size     float32
	p0, p1         float32
}

func newGradientMapper(b Bounds, g Gradient) gradientMapper {
	rad := float64(float32(math.Mod(float64(g.Angle), 360))-90) * math.Pi / 180
	dx, dy := float32(math.Cos(rad)), float32(math.Sin(rad))
	w, h := float32(b.Size.W), float32(b.Size.H)
	if w > h {
		dy *= h / w
	} else {
		dx *= w / h
	}
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l > 0 {
		dx, dy = dx/l, dy/l
	}
	m := gradientMapper{
		cx: float32(b.Origin.X) + w/2, cy: float32(b.Origin.Y) + h/2,
		dx: dx, dy: dy,
		p0: g.Stops[0].Percentage, p1: g.Stops[1].Percentage,
	}
	if abs32(dx) > abs32(dy) {
		m.half, m.size = w/2, w
	} else {
		m.half, m.size = h/2, h
	}
	return m
}

// index returns the LUT index for the pixel center (px, py).
func (m *gradientMapper) index(px, py float32) int {
	t := (px-m.cx)*m.dx + (py-m.cy)*m.dy
	if m.size > 0 {
		t = (t + m.half) / m.size
	}
	if d := m.p1 - m.p0; d > 1e-6 {
		t = (t - m.p0) / d
	} else if t < m.p0 {
		t = 0
	} else {
		t = 1
	}
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return gradientLUTSize - 1
	}
	return int(t*(gradientLUTSize-1) + 0.5)
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func srgbToLinear(c float32) float32 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return float32(math.Pow(float64((c+0.055)/1.055), 2.4))
}

func linearToSrgb(c float32) float32 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*float32(math.Pow(float64(c), 1/2.4)) - 0.055
}

// toOklab converts a straight sRGB color to Oklab (L, a, b).
func toOklab(c Rgba) [3]float32 {
	r := srgbToLinear(float32(c.R) / 255)
	g := srgbToLinear(float32(c.G) / 255)
	b := srgbToLinear(float32(c.B) / 255)
	l := cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float32{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// fromOklab converts Oklab back to sRGB channels in 0..255.
func fromOklab(L, a, b float32) (float32, float32, float32) {
	l := L + 0.3963377774*a + 0.2158037573*b
	m := L - 0.1055613458*a - 0.0638541728*b
	s := L - 0.0894841775*a - 1.2914855480*b
	l, m, s = l*l*l, m*m*m, s*s*s
	r := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	bb := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	conv := func(v float32) float32 { return clamp32(linearToSrgb(clamp32(v, 0, 1))*255, 0, 255) }
	return conv(r), conv(g), conv(bb)
}

func cbrt(v float32) float32 { return float32(math.Cbrt(float64(v))) }
