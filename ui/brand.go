package ui

import (
	"image/color"
	"math"

	"cad-lite/assets/appicon"
	"cad-lite/internal/gpui"
)

// Brand selects what stands at the left end of the title bar.
type Brand int

const (
	// BrandText shows the application icon and the "Cad-lite" wordmark; the zero value, used on
	// Linux and the web.
	BrandText Brand = iota
	// BrandIcon shows the application icon alone, in place of the wordmark (macOS).
	BrandIcon
	// BrandNone shows nothing and leaves no gap (Windows, whose window frame carries icon and title).
	BrandNone
)

// brandIconSize is the logo edge in logical pixels; it fits the title bar, and the 32 and
// 64 px window icons are drawn 1:1 at UI scale 1 and 2.
const brandIconSize = 32

// BrandForOS returns the brand for a runtime.GOOS value: the icon on "darwin", nothing on
// "windows", the wordmark everywhere else.
func BrandForOS(goos string) Brand {
	switch goos {
	case "darwin":
		return BrandIcon
	case "windows":
		return BrandNone
	}
	return BrandText
}

// ParseBrand maps the names "mac", "win" and "text" to a Brand; ok is false for anything else.
func ParseBrand(s string) (b Brand, ok bool) {
	switch s {
	case "mac":
		return BrandIcon, true
	case "win":
		return BrandNone, true
	case "text":
		return BrandText, true
	}
	return BrandText, false
}

// brand returns the title bar's leading element, or nil for BrandNone.
func (ws *Workspace) brand() gpui.IntoElement {
	switch ws.Brand {
	case BrandNone:
		return nil
	case BrandIcon:
		return newBrandIcon(brandIconSize)
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(10).FlexShrink(0).Child(
		newBrandIcon(brandIconSize),
		gpui.NewText("Cad-lite").FontWeight(700).TextSize(fontTitle+1).TextColor(uiText))
}

// brandIcon paints the full-colour application icon. gpui has no colour image primitive, so the
// bitmap is drawn as runs of same-coloured device-pixel quads.
type brandIcon struct {
	gpui.StyledMixin
	size float32
}

func newBrandIcon(size float32) *brandIcon {
	b := &brandIcon{StyledMixin: gpui.NewStyledMixin(), size: size}
	b.SizeWH(size, size).FlexShrink(0)
	return b
}

// IntoElement returns the icon's Element adapter.
func (b *brandIcon) IntoElement() gpui.Element { return brandElement{b} }

type brandElement struct{ b *brandIcon }

func (e brandElement) ID() (gpui.ElementID, bool) { return gpui.ElementID{}, false }

func (e brandElement) RequestLayout(w *gpui.Window, _ *gpui.App) (gpui.LayoutID, any) {
	return w.RegisterLayoutNode(&gpui.LayoutNode{Style: e.b.ResolvedStyle(gpui.DefaultStyle())}), nil
}

func (e brandElement) Prepaint(*gpui.Window, *gpui.App, gpui.ElementID, gpui.Bounds, any) any {
	return nil
}

func (e brandElement) Paint(w *gpui.Window, _ *gpui.App, _ gpui.ElementID, b gpui.Bounds, _, _ any) {
	scale := w.ScaleFactor()
	if scale <= 0 {
		scale = 1
	}
	px := int(math.Round(float64(e.b.size * scale)))
	img := appicon.Image(px)
	if img == nil {
		return
	}
	r := img.Bounds()
	ox := math.Round(float64(b.Origin.X)*float64(scale)) + math.Floor(float64(float32(b.Size.W)*scale-float32(r.Dx()))/2)
	oy := math.Round(float64(b.Origin.Y)*float64(scale)) + math.Floor(float64(float32(b.Size.H)*scale-float32(r.Dy()))/2)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			x2 := x + 1
			for x2 < r.Max.X && color.NRGBAModel.Convert(img.At(x2, y)).(color.NRGBA) == c {
				x2++
			}
			if c.A > 0 {
				sc := float64(scale)
				w.PaintQuad(gpui.Quad{
					Bounds: gpui.NewBounds(float32((ox+float64(x-r.Min.X))/sc), float32((oy+float64(y-r.Min.Y))/sc),
						float32(float64(x2-x)/sc), float32(1/sc)),
					Background: gpui.BackgroundSolid,
					Fill:       gpui.RgbaOf(c.R, c.G, c.B, c.A),
				})
			}
			x = x2
		}
	}
}
