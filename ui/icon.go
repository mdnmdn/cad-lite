package ui

import (
	"bytes"
	"image"
	"math"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	"go-draw/assets/icons"
	"go-draw/internal/gpui"
)

type iconKey struct {
	name icons.Name
	px   int
}

var (
	iconMu    sync.Mutex
	iconCache = map[iconKey]*gpui.GlyphMask{}
)

// IconMask returns the named icon rasterised to a px by px alpha mask, the
// analogue of GPUI's paint_svg tile. Colour in the SVG is discarded: the
// mask carries coverage only and the caller tints it. Results are cached by
// name and size; an unknown or unparsable icon yields nil.
func IconMask(name icons.Name, px int) *gpui.GlyphMask {
	if px <= 0 {
		return nil
	}
	k := iconKey{name, px}
	iconMu.Lock()
	defer iconMu.Unlock()
	if m, ok := iconCache[k]; ok {
		return m
	}
	m := rasterizeIcon(icons.SVG(name), px)
	iconCache[k] = m
	return m
}

func rasterizeIcon(svg []byte, px int) *gpui.GlyphMask {
	if len(svg) == 0 {
		return nil
	}
	icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(svg), "#000000", oksvg.IgnoreErrorMode)
	if err != nil {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	icon.SetTarget(0, 0, float64(px), float64(px))
	icon.Draw(rasterx.NewDasher(px, px, rasterx.NewScannerGV(px, px, img, img.Bounds())), 1)
	pix := make([]uint8, px*px)
	for i := range pix {
		pix[i] = img.Pix[4*i+3]
	}
	return &gpui.GlyphMask{Pix: pix, W: px, H: px}
}

// Icon is a leaf element that paints one icon as a square of Size logical
// pixels, tinted with a single colour (like GPUI's Icon with a text colour).
type Icon struct {
	gpui.StyledMixin
	name  icons.Name
	size  float32
	color gpui.Rgba
}

// NewIcon returns an icon element of size logical pixels tinted with color.
// It does not shrink in a flex row.
func NewIcon(name icons.Name, size float32, color gpui.Rgba) *Icon {
	i := &Icon{StyledMixin: gpui.NewStyledMixin(), name: name, size: size, color: color}
	i.SizeWH(size, size).FlexShrink(0)
	return i
}

// IntoElement returns the icon's Element adapter.
func (i *Icon) IntoElement() gpui.Element { return iconElement{i} }

type iconElement struct{ i *Icon }

func (e iconElement) ID() (gpui.ElementID, bool) { return gpui.ElementID{}, false }

func (e iconElement) RequestLayout(w *gpui.Window, _ *gpui.App) (gpui.LayoutID, any) {
	return w.RegisterLayoutNode(&gpui.LayoutNode{Style: e.i.ResolvedStyle(gpui.DefaultStyle())}), nil
}

func (e iconElement) Prepaint(*gpui.Window, *gpui.App, gpui.ElementID, gpui.Bounds, any) any {
	return nil
}

func (e iconElement) Paint(w *gpui.Window, _ *gpui.App, _ gpui.ElementID, b gpui.Bounds, _, _ any) {
	scale := w.ScaleFactor()
	if scale <= 0 {
		scale = 1
	}
	px := int(math.Round(float64(e.i.size * scale)))
	m := IconMask(e.i.name, px)
	if m == nil {
		return
	}
	// Snap to the device grid so the mask is blitted 1:1 instead of resampled.
	snap := func(v float32) gpui.Pixels {
		return gpui.Pixels(math.Round(float64(v*scale)) / float64(scale))
	}
	dev := gpui.Pixels(float32(px) / scale)
	w.PaintGlyph(gpui.GlyphSprite{
		Bounds: gpui.Bounds{
			Origin: gpui.Point{
				X: snap(float32(b.Origin.X) + (float32(b.Size.W)-float32(dev))/2),
				Y: snap(float32(b.Origin.Y) + (float32(b.Size.H)-float32(dev))/2),
			},
			Size: gpui.Size{W: dev, H: dev},
		},
		Mask:  m,
		Color: e.i.color,
	})
}
