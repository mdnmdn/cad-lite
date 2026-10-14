package ui

import (
	"cad-lite/assets/icons"
	"cad-lite/internal/gpui"
)

// Type scale of the window chrome, in logical pixels.
const (
	// fontUI is the size of ribbon labels, rows and body text.
	fontUI = 13
	// fontSmall is the size of group names, chips and hints.
	fontSmall = 12
	// fontTitle is the size of the brand and panel headings.
	fontTitle = 14
	// titleBarHeight is the title bar's height.
	titleBarHeight = 52
	// ribbonHeight is the ribbon's height when its groups share one row.
	ribbonHeight = 108
	// iconButtonSize is the edge of an icon-only button.
	iconButtonSize = 32
	// chromeIconSize is the icon edge of ribbon, title bar and panel buttons.
	chromeIconSize = 18
)

// monoFamily is the font family of the viewport label, the command line, the coordinates and
// the status toggles: the system monospace font, else the bundled Go Mono.
const monoFamily = "monospace"

// monoText returns a monospace text of size px.
func monoText(s string, px float32) *gpui.Text {
	return gpui.NewText(s).FontFamily(monoFamily).TextSize(px)
}

// vDivider is a 1 px vertical separator of height h.
func vDivider(h float32) *gpui.Div { return gpui.NewDiv().SizeWH(1, h).Bg(uiBorder).FlexShrink(0) }

// hDivider is a full-width 1 px horizontal separator.
func hDivider() *gpui.Div { return gpui.NewDiv().SizeH(1).Bg(uiBorder).FlexShrink(0) }

// dot is a filled circle of diameter d.
func dot(d float32, c gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().SizeWH(d, d).Rounded(d / 2).Bg(c).FlexShrink(0)
}

// activeFill and activeBorder colour a switched-on button or chip: a soft accent over the panel.
func activeFill() gpui.Rgba   { return mix(uiPanel, uiAccent, 0.28) }
func activeBorder() gpui.Rgba { return mix(uiPanel, uiAccent, 0.75) }

// iconButton is an icon-only button of iconButtonSize with a tooltip. active paints the soft
// accent fill; act runs on a click.
func iconButton(id, tip string, icon icons.Name, active bool, act func(*gpui.Window)) *gpui.Div {
	col := uiText
	b := gpui.NewDiv().ID(id).FlexRow().ItemsCenter().JustifyCenter().SizeWH(iconButtonSize, iconButtonSize).Rounded(6).FlexShrink(0).
		Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { act(w) })
	if active {
		col = uiOn
		b.Bg(activeFill())
	}
	return b.Child(NewIcon(icon, chromeIconSize, col))
}
