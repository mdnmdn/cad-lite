// Package ui is the application shell built on the gpui library: toolbar,
// drawing canvas, layer and properties panel, command line and status bar.
// It is the only package besides render/gpuirender and cmd/ that imports
// gpui.
package ui

import (
	"go-draw/internal/gpui"
	"go-draw/render"
)

// Theme colours. The canvas colour matches AutoCAD's dark model space.
var (
	colCanvas    = render.Color{R: 33, G: 40, B: 48, A: 255}
	colGrid      = render.Color{R: 255, G: 255, B: 255, A: 18}
	colGridMajor = render.Color{R: 255, G: 255, B: 255, A: 36}
	colCrosshair = render.Color{R: 220, G: 225, B: 230, A: 200}
	colSnap      = render.Color{R: 255, G: 200, B: 40, A: 255}
	colSelected  = render.Color{R: 80, G: 160, B: 255, A: 255}
	colHover     = render.Color{R: 160, G: 205, B: 255, A: 255}
	colWindowSel = render.Color{R: 60, G: 120, B: 255, A: 40}
	colWindowOut = render.Color{R: 90, G: 150, B: 255, A: 220}
	colCrossSel  = render.Color{R: 60, G: 200, B: 90, A: 40}
	colCrossOut  = render.Color{R: 90, G: 220, B: 120, A: 220}
	colAxisX     = render.Color{R: 230, G: 80, B: 80, A: 255}
	colAxisY     = render.Color{R: 90, G: 200, B: 90, A: 255}

	uiBg       = gpui.RgbaOf(24, 28, 33, 255)
	uiPanel    = gpui.RgbaOf(36, 41, 48, 255)
	uiPanelAlt = gpui.RgbaOf(44, 50, 58, 255)
	uiBorder   = gpui.RgbaOf(58, 65, 75, 255)
	uiText     = gpui.RgbaOf(214, 219, 225, 255)
	uiMuted    = gpui.RgbaOf(140, 150, 162, 255)
	uiAccent   = gpui.RgbaOf(59, 130, 246, 255)
	uiOn       = gpui.RgbaOf(96, 165, 250, 255)
	uiField    = gpui.RgbaOf(24, 28, 33, 255)
	uiSelect   = gpui.RgbaOf(59, 130, 246, 110)
	uiOK       = gpui.RgbaOf(74, 190, 110, 255)
	uiWarn     = gpui.RgbaOf(235, 175, 60, 255)
	uiErr      = gpui.RgbaOf(235, 90, 90, 255)
)

// themeField styles a gpui.TextInput as a dark field: uiField fill, uiBorder
// outline that turns uiAccent while focused, uiText text and caret, uiMuted
// placeholder and a uiSelect selection.
func themeField(in *gpui.TextInput) *gpui.TextInput {
	return in.
		Style(func(d *gpui.Div) *gpui.Div {
			return d.Bg(uiField).Border(1, uiBorder).TextColor(uiText).
				Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(1, uiAccent) })
		}).
		PlaceholderColor(uiMuted).
		SelectionColor(uiSelect)
}
