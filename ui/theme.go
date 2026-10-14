package ui

import "cad-lite/internal/gpui"

// Theme colours. The variables hold the current theme (themes.go): they start
// as the dark theme, whose canvas matches AutoCAD's dark model space, and
// ApplyTheme reassigns them. The per-theme values are the tokens table in
// _docs/tech/ui-framework.md.
var (
	colCanvas    = themeDark.Canvas
	colGrid      = themeDark.Grid
	colGridMajor = themeDark.GridMajor
	colCrosshair = themeDark.Crosshair
	colSnap      = themeDark.Snap
	colSelected  = themeDark.Selected
	colHover     = themeDark.Hover
	colWindowSel = themeDark.WindowSel
	colWindowOut = themeDark.WindowOut
	colCrossSel  = themeDark.CrossSel
	colCrossOut  = themeDark.CrossOut
	colAxisX     = themeDark.AxisX
	colAxisY     = themeDark.AxisY

	uiBg       = themeDark.Bg
	uiPanel    = themeDark.Panel
	uiPanelAlt = themeDark.PanelAlt
	uiBorder   = themeDark.Border
	uiText     = themeDark.Text
	uiMuted    = themeDark.Muted
	uiAccent   = themeDark.Accent
	uiOn       = themeDark.On
	uiField    = themeDark.Field
	uiSelect   = themeDark.Select
	uiOK       = themeDark.OK
	uiWarn     = themeDark.Warn
	uiErr      = themeDark.Err
	uiBackdrop = themeDark.Backdrop

	uiPrimary   = themeDark.Primary
	uiOnPrimary = themeDark.OnPrimary
)

// themeField styles a gpui.TextInput as a themed field: uiField fill, uiBorder
// outline that turns uiAccent while focused, uiText text and caret, uiMuted
// placeholder and a uiSelect selection. ApplyTheme restyles it on a theme
// switch.
func themeField(in *gpui.TextInput) *gpui.TextInput {
	trackField(in)
	return in.
		Style(func(d *gpui.Div) *gpui.Div {
			return d.Bg(uiField).Border(1, uiBorder).TextColor(uiText).
				Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(1, uiAccent) })
		}).
		PlaceholderColor(uiMuted).
		SelectionColor(uiSelect)
}

// themeBareField styles a gpui.TextInput that sits inside a box of its own (the command box, the
// agent composer): no fill or outline of its own, uiText text of size px, in the monospace family
// when mono is set. The box shows the focus.
func themeBareField(in *gpui.TextInput, px float32, mono bool) *gpui.TextInput {
	trackField(in)
	return in.
		Style(func(d *gpui.Div) *gpui.Div {
			d = d.Bg(gpui.RgbaOf(0, 0, 0, 0)).Border(0, uiBorder).P(0).Py_(2).TextColor(uiText).TextSize(px)
			if mono {
				d = d.FontFamily(monoFamily)
			}
			return d
		}).
		PlaceholderColor(uiMuted).
		SelectionColor(uiSelect)
}

// tooltipStyle maps a theme onto the gpui tooltip bubble: the input fill
// (Field, which contrasts with the panels), the theme's border and body text,
// so tooltips follow the window chrome. Size and timing keep gpui's defaults.
func tooltipStyle(t Theme) gpui.TooltipStyle {
	s := gpui.DefaultTooltipStyle()
	s.Background, s.Border, s.Foreground = t.Field, t.Border, t.Text
	s.MaxWidth = 320
	return s
}

// mix returns a blended with b, t (0 to 1) of the way to b: the soft accent fills and borders of
// chips and active buttons, derived from the theme so every theme gets one.
func mix(a, b gpui.Rgba, t float32) gpui.Rgba {
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return gpui.RgbaOf(l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255)
}
