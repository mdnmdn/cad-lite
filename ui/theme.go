package ui

import "go-draw/internal/gpui"

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
