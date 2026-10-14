package gpui

// BoxShadow is a CSS-style box shadow (GPUI's BoxShadow).
type BoxShadow struct {
	// Color is the shadow color.
	Color Rgba
	// Offset moves the shadow relative to the element.
	Offset Point
	// BlurRadius is the CSS blur radius; the gaussian sigma is half of it.
	BlurRadius Pixels
	// SpreadRadius grows (or, negative, shrinks) the shadow shape.
	SpreadRadius Pixels
	// Inset draws the shadow inside the element instead of outside.
	Inset bool
}

// NewBoxShadow returns an unblurred outset shadow of color offset by
// (offsetX, offsetY).
func NewBoxShadow(offsetX, offsetY float32, color Rgba) BoxShadow {
	return BoxShadow{Color: color, Offset: Point{X: Pixels(offsetX), Y: Pixels(offsetY)}}
}

// WithBlur returns s with blur radius r.
func (s BoxShadow) WithBlur(r float32) BoxShadow {
	s.BlurRadius = Pixels(r)
	return s
}

// WithSpread returns s with spread radius r.
func (s BoxShadow) WithSpread(r float32) BoxShadow {
	s.SpreadRadius = Pixels(r)
	return s
}

// AsInset returns s as an inner shadow.
func (s BoxShadow) AsInset() BoxShadow {
	s.Inset = true
	return s
}

func presetShadow(y, blur, spread float32, alpha float32) BoxShadow {
	return BoxShadow{
		Color:        Rgba{A: uint8(alpha*255 + 0.5)},
		Offset:       Point{Y: Pixels(y)},
		BlurRadius:   Pixels(blur),
		SpreadRadius: Pixels(spread),
	}
}

// Preset shadows, matching GPUI's shadow_* helpers.
var (
	shadow2xs = []BoxShadow{presetShadow(1, 0, 0, 0.05)}
	shadowXs  = []BoxShadow{presetShadow(1, 2, 0, 0.05)}
	shadowSm  = []BoxShadow{presetShadow(1, 3, 0, 0.1), presetShadow(1, 2, -1, 0.1)}
	shadowMd  = []BoxShadow{presetShadow(4, 6, -1, 0.1), presetShadow(2, 4, -2, 0.1)}
	shadowLg  = []BoxShadow{presetShadow(10, 15, -3, 0.1), presetShadow(4, 6, -4, 0.1)}
	shadowXl  = []BoxShadow{presetShadow(20, 25, -5, 0.1), presetShadow(8, 10, -6, 0.1)}
	shadow2xl = []BoxShadow{presetShadow(25, 50, -12, 0.25)}
)

// shadowPrimitive builds the scene shadow for s on an element with the
// given border box and corner radii.
func shadowPrimitive(s BoxShadow, bounds Bounds, radii Corners) Shadow {
	sp := s.SpreadRadius
	shape := bounds.Translate(s.Offset)
	sign := Pixels(1)
	if s.Inset {
		sign = -1
	}
	shape = shape.Inset(Edges{Top: -sp * sign, Right: -sp * sign, Bottom: -sp * sign, Left: -sp * sign})
	grow := func(r Pixels) Pixels {
		if r <= 0 {
			return 0
		}
		return maxPixels(0, r+sp*sign)
	}
	sh := Shadow{
		Bounds: shape,
		CornerRadii: Corners{
			TopLeft:     grow(radii.TopLeft),
			TopRight:    grow(radii.TopRight),
			BottomLeft:  grow(radii.BottomLeft),
			BottomRight: grow(radii.BottomRight),
		},
		Color:      s.Color,
		BlurRadius: maxPixels(0, s.BlurRadius),
		Inset:      s.Inset,
	}
	if s.Inset {
		sh.Clip, sh.ClipRadii = bounds, radii
	}
	return sh
}

// paintBox paints an element's box: outset shadows, background, inset
// shadows, then the border.
func paintBox(w *Window, bounds Bounds, style Style) {
	mask := w.CurrentContentMask()
	for _, s := range style.BoxShadows {
		if !s.Inset {
			w.PaintShadow(shadowPrimitive(s, bounds, style.CornerRadii))
		}
	}
	switch bg := style.Background; bg.Kind {
	case BackgroundSolid:
		if bg.Solid.A > 0 {
			w.PaintQuad(Quad{
				Bounds:      bounds,
				ContentMask: mask,
				Background:  BackgroundSolid,
				Fill:        bg.Solid,
				CornerRadii: style.CornerRadii,
			})
		}
	case BackgroundLinearGradient:
		if !bg.IsTransparent() {
			w.PaintQuad(Quad{
				Bounds:      bounds,
				ContentMask: mask,
				Background:  BackgroundLinearGradient,
				Gradient:    bg.Gradient,
				CornerRadii: style.CornerRadii,
			})
		}
	}
	for _, s := range style.BoxShadows {
		if s.Inset {
			w.PaintShadow(shadowPrimitive(s, bounds, style.CornerRadii))
		}
	}
	bw := style.BorderWidths
	if bw.Sum() > 0 && style.BorderColor.A > 0 {
		w.PaintQuad(Quad{
			Bounds:       bounds,
			ContentMask:  mask,
			Background:   BackgroundNone,
			BorderColor:  style.BorderColor,
			BorderWidths: bw,
			CornerRadii:  style.CornerRadii,
		})
	}
}
