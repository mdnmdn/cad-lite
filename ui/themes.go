package ui

import (
	"weak"

	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Theme is one complete colour scheme: the drawing canvas tokens (render
// colours, because the display list carries them) and the window chrome
// tokens (gpui colours). ApplyTheme makes one current by reassigning the
// package-level colour variables in theme.go, which every view reads.
type Theme struct {
	// Name is the stable key stored in the settings file.
	Name string
	// Label is the name shown in the settings panel.
	Label string

	// Canvas is the model-space background; ACI 7 draws black on a light one
	// and white on a dark one.
	Canvas render.Color
	// Grid and GridMajor are the minor and every-tenth grid lines.
	Grid, GridMajor render.Color
	// Crosshair is the cursor crosshair and pickbox.
	Crosshair render.Color
	// Snap is the object snap marker.
	Snap render.Color
	// Selected and Hover tint selected and hovered entities.
	Selected, Hover render.Color
	// WindowSel and WindowOut are the fill and outline of a window selection
	// box (left to right).
	WindowSel, WindowOut render.Color
	// CrossSel and CrossOut are the fill and outline of a crossing box (right
	// to left).
	CrossSel, CrossOut render.Color
	// AxisX and AxisY are the UCS icon axes.
	AxisX, AxisY render.Color

	// Bg is the window and status bar background.
	Bg gpui.Rgba
	// Panel is the toolbar, side panel and command area.
	Panel gpui.Rgba
	// PanelAlt is hover, button and raised-surface fill.
	PanelAlt gpui.Rgba
	// Border is separators and outlines.
	Border gpui.Rgba
	// Text and Muted are body and secondary text.
	Text, Muted gpui.Rgba
	// Accent fills active buttons and focus rings; Text must read on it.
	Accent gpui.Rgba
	// On is the highlight text colour (titles, enabled flags).
	On gpui.Rgba
	// Field fills text inputs; Select is the text selection highlight.
	Field, Select gpui.Rgba
	// OK, Warn and Err are status colours.
	OK, Warn, Err gpui.Rgba
	// Backdrop dims the window behind a modal panel.
	Backdrop gpui.Rgba
	// Primary fills the main call-to-action button (Send) and OnPrimary is the text on it.
	Primary, OnPrimary gpui.Rgba
}

// Dark reports whether the canvas is dark, which decides how ACI 7 is drawn.
func (t Theme) Dark() bool {
	c := t.Canvas
	return 0.2126*float64(c.R)+0.7152*float64(c.G)+0.0722*float64(c.B) <= 0.5*255
}

// DefaultThemeName is the theme used when the settings name none or an
// unknown one.
const DefaultThemeName = "dark"

var (
	themeDark = Theme{
		Name: "dark", Label: "Dark",
		Canvas:    render.Color{R: 33, G: 40, B: 48, A: 255},
		Grid:      render.Color{R: 255, G: 255, B: 255, A: 18},
		GridMajor: render.Color{R: 255, G: 255, B: 255, A: 36},
		Crosshair: render.Color{R: 220, G: 225, B: 230, A: 200},
		Snap:      render.Color{R: 255, G: 200, B: 40, A: 255},
		Selected:  render.Color{R: 80, G: 160, B: 255, A: 255},
		Hover:     render.Color{R: 160, G: 205, B: 255, A: 255},
		WindowSel: render.Color{R: 60, G: 120, B: 255, A: 40},
		WindowOut: render.Color{R: 90, G: 150, B: 255, A: 220},
		CrossSel:  render.Color{R: 60, G: 200, B: 90, A: 40},
		CrossOut:  render.Color{R: 90, G: 220, B: 120, A: 220},
		AxisX:     render.Color{R: 230, G: 80, B: 80, A: 255},
		AxisY:     render.Color{R: 90, G: 200, B: 90, A: 255},
		Bg:        gpui.RgbaOf(24, 28, 33, 255),
		Panel:     gpui.RgbaOf(36, 41, 48, 255),
		PanelAlt:  gpui.RgbaOf(44, 50, 58, 255),
		Border:    gpui.RgbaOf(58, 65, 75, 255),
		Text:      gpui.RgbaOf(214, 219, 225, 255),
		Muted:     gpui.RgbaOf(140, 150, 162, 255),
		Accent:    gpui.RgbaOf(59, 130, 246, 255),
		On:        gpui.RgbaOf(96, 165, 250, 255),
		Field:     gpui.RgbaOf(24, 28, 33, 255),
		Select:    gpui.RgbaOf(59, 130, 246, 110),
		OK:        gpui.RgbaOf(74, 190, 110, 255),
		Warn:      gpui.RgbaOf(235, 175, 60, 255),
		Err:       gpui.RgbaOf(235, 90, 90, 255),
		Backdrop:  gpui.RgbaOf(8, 10, 14, 150),
		Primary:   gpui.RgbaOf(47, 123, 246, 255),
		OnPrimary: gpui.RgbaOf(255, 255, 255, 255),
	}
	themeLight = Theme{
		Name: "light", Label: "Light",
		Canvas:    render.Color{R: 252, G: 252, B: 253, A: 255},
		Grid:      render.Color{R: 0, G: 0, B: 0, A: 22},
		GridMajor: render.Color{R: 0, G: 0, B: 0, A: 48},
		Crosshair: render.Color{R: 30, G: 36, B: 44, A: 200},
		Snap:      render.Color{R: 196, G: 110, B: 0, A: 255},
		Selected:  render.Color{R: 0, G: 102, B: 214, A: 255},
		Hover:     render.Color{R: 70, G: 140, B: 230, A: 255},
		WindowSel: render.Color{R: 40, G: 100, B: 240, A: 36},
		WindowOut: render.Color{R: 30, G: 90, B: 220, A: 220},
		CrossSel:  render.Color{R: 30, G: 160, B: 70, A: 36},
		CrossOut:  render.Color{R: 20, G: 140, B: 60, A: 220},
		AxisX:     render.Color{R: 205, G: 45, B: 45, A: 255},
		AxisY:     render.Color{R: 30, G: 150, B: 50, A: 255},
		Bg:        gpui.RgbaOf(226, 230, 235, 255),
		Panel:     gpui.RgbaOf(244, 246, 248, 255),
		PanelAlt:  gpui.RgbaOf(230, 234, 240, 255),
		Border:    gpui.RgbaOf(200, 206, 215, 255),
		Text:      gpui.RgbaOf(28, 33, 40, 255),
		Muted:     gpui.RgbaOf(98, 108, 120, 255),
		Accent:    gpui.RgbaOf(96, 165, 250, 255),
		On:        gpui.RgbaOf(29, 78, 216, 255),
		Field:     gpui.RgbaOf(255, 255, 255, 255),
		Select:    gpui.RgbaOf(59, 130, 246, 90),
		OK:        gpui.RgbaOf(22, 135, 68, 255),
		Warn:      gpui.RgbaOf(176, 104, 0, 255),
		Err:       gpui.RgbaOf(200, 48, 48, 255),
		Backdrop:  gpui.RgbaOf(20, 26, 34, 110),
		Primary:   gpui.RgbaOf(47, 123, 246, 255),
		OnPrimary: gpui.RgbaOf(255, 255, 255, 255),
	}
	themeClassic = Theme{
		Name: "classic", Label: "Classic",
		Canvas:    render.Color{R: 6, G: 6, B: 8, A: 255},
		Grid:      render.Color{R: 255, G: 255, B: 255, A: 26},
		GridMajor: render.Color{R: 255, G: 255, B: 255, A: 56},
		Crosshair: render.Color{R: 255, G: 255, B: 255, A: 230},
		Snap:      render.Color{R: 255, G: 230, B: 0, A: 255},
		Selected:  render.Color{R: 0, G: 150, B: 255, A: 255},
		Hover:     render.Color{R: 150, G: 200, B: 255, A: 255},
		WindowSel: render.Color{R: 0, G: 110, B: 255, A: 40},
		WindowOut: render.Color{R: 90, G: 160, B: 255, A: 230},
		CrossSel:  render.Color{R: 0, G: 200, B: 80, A: 40},
		CrossOut:  render.Color{R: 60, G: 220, B: 110, A: 230},
		AxisX:     render.Color{R: 255, G: 60, B: 60, A: 255},
		AxisY:     render.Color{R: 60, G: 220, B: 60, A: 255},
		Bg:        gpui.RgbaOf(58, 58, 60, 255),
		Panel:     gpui.RgbaOf(76, 76, 80, 255),
		PanelAlt:  gpui.RgbaOf(94, 94, 99, 255),
		Border:    gpui.RgbaOf(36, 36, 38, 255),
		Text:      gpui.RgbaOf(238, 238, 238, 255),
		Muted:     gpui.RgbaOf(178, 178, 182, 255),
		Accent:    gpui.RgbaOf(70, 120, 210, 255),
		On:        gpui.RgbaOf(150, 200, 255, 255),
		Field:     gpui.RgbaOf(38, 38, 40, 255),
		Select:    gpui.RgbaOf(70, 120, 210, 130),
		OK:        gpui.RgbaOf(100, 210, 120, 255),
		Warn:      gpui.RgbaOf(240, 190, 70, 255),
		Err:       gpui.RgbaOf(245, 110, 110, 255),
		Backdrop:  gpui.RgbaOf(0, 0, 0, 150),
		Primary:   gpui.RgbaOf(50, 110, 215, 255),
		OnPrimary: gpui.RgbaOf(255, 255, 255, 255),
	}
	themeHighContrast = Theme{
		Name: "high-contrast", Label: "High contrast",
		Canvas:    render.Color{R: 0, G: 0, B: 0, A: 255},
		Grid:      render.Color{R: 255, G: 255, B: 255, A: 44},
		GridMajor: render.Color{R: 255, G: 255, B: 255, A: 96},
		Crosshair: render.Color{R: 255, G: 255, B: 255, A: 255},
		Snap:      render.Color{R: 255, G: 255, B: 0, A: 255},
		Selected:  render.Color{R: 0, G: 255, B: 255, A: 255},
		Hover:     render.Color{R: 255, G: 170, B: 0, A: 255},
		WindowSel: render.Color{R: 0, G: 128, B: 255, A: 64},
		WindowOut: render.Color{R: 0, G: 200, B: 255, A: 255},
		CrossSel:  render.Color{R: 0, G: 255, B: 0, A: 56},
		CrossOut:  render.Color{R: 0, G: 255, B: 0, A: 255},
		AxisX:     render.Color{R: 255, G: 70, B: 70, A: 255},
		AxisY:     render.Color{R: 70, G: 255, B: 70, A: 255},
		Bg:        gpui.RgbaOf(0, 0, 0, 255),
		Panel:     gpui.RgbaOf(10, 10, 10, 255),
		PanelAlt:  gpui.RgbaOf(46, 46, 46, 255),
		Border:    gpui.RgbaOf(255, 255, 255, 255),
		Text:      gpui.RgbaOf(255, 255, 255, 255),
		Muted:     gpui.RgbaOf(214, 214, 214, 255),
		Accent:    gpui.RgbaOf(0, 76, 178, 255),
		On:        gpui.RgbaOf(255, 222, 0, 255),
		Field:     gpui.RgbaOf(0, 0, 0, 255),
		Select:    gpui.RgbaOf(0, 120, 255, 170),
		OK:        gpui.RgbaOf(0, 255, 128, 255),
		Warn:      gpui.RgbaOf(255, 204, 0, 255),
		Err:       gpui.RgbaOf(255, 96, 96, 255),
		Backdrop:  gpui.RgbaOf(0, 0, 0, 190),
		Primary:   gpui.RgbaOf(0, 90, 200, 255),
		OnPrimary: gpui.RgbaOf(255, 255, 255, 255),
	}

	themes  = []Theme{themeDark, themeLight, themeClassic, themeHighContrast}
	current = themeDark
)

// Themes returns the registered themes in display order.
func Themes() []Theme { return append([]Theme(nil), themes...) }

// ThemeByName returns the theme registered under name.
func ThemeByName(name string) (Theme, bool) {
	for _, t := range themes {
		if t.Name == name {
			return t, true
		}
	}
	return Theme{}, false
}

// CurrentTheme returns the theme last applied with ApplyTheme.
func CurrentTheme() Theme { return current }

// ApplyTheme makes t the current theme by reassigning every colour variable.
// Views read the variables at render time, so a redraw shows the new theme;
// text inputs created by themeField are restyled here. It is not safe to call
// from a goroutine other than the UI's.
func ApplyTheme(t Theme) {
	current = t
	colCanvas, colGrid, colGridMajor, colCrosshair = t.Canvas, t.Grid, t.GridMajor, t.Crosshair
	colSnap, colSelected, colHover = t.Snap, t.Selected, t.Hover
	colWindowSel, colWindowOut, colCrossSel, colCrossOut = t.WindowSel, t.WindowOut, t.CrossSel, t.CrossOut
	colAxisX, colAxisY = t.AxisX, t.AxisY
	uiBg, uiPanel, uiPanelAlt, uiBorder = t.Bg, t.Panel, t.PanelAlt, t.Border
	uiText, uiMuted, uiAccent, uiOn = t.Text, t.Muted, t.Accent, t.On
	uiField, uiSelect, uiOK, uiWarn, uiErr, uiBackdrop = t.Field, t.Select, t.OK, t.Warn, t.Err, t.Backdrop
	uiPrimary, uiOnPrimary = t.Primary, t.OnPrimary
	restyleFields()
}

// themedFields tracks the inputs themeField styled, weakly, so ApplyTheme can
// restyle their placeholder and selection colours (which are values, not
// closures).
var themedFields []weak.Pointer[gpui.TextInput]

func trackField(in *gpui.TextInput) {
	live := themedFields[:0]
	for _, w := range themedFields {
		if w.Value() != nil {
			live = append(live, w)
		}
	}
	themedFields = append(live, weak.Make(in))
}

func restyleFields() {
	for _, w := range themedFields {
		if in := w.Value(); in != nil {
			in.PlaceholderColor(uiMuted).SelectionColor(uiSelect)
		}
	}
}
