package ui

import (
	"unicode/utf8"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Ribbon metrics, in logical pixels.
const (
	// ribbonLargeIcon and ribbonSmallIcon are the icon edges of the two button shapes.
	ribbonLargeIcon = 24
	ribbonSmallIcon = 18
	// ribbonBodyHeight is the height of a group's buttons: one large button, or two small ones.
	ribbonBodyHeight = 72
	// ribbonSmallHeight is the height of a small button.
	ribbonSmallHeight = 34
	// ribbonCharWidth is the average width of a ribbon label's character, which sizes the estimate
	// of the groups' widths.
	ribbonCharWidth = 7.3
)

// ribbonGroup is one named panel of the ribbon. The ribbon wraps between groups, never inside one.
type ribbonGroup struct {
	id   string
	name string
	body gpui.IntoElement
}

// ribbonButton is one command button of the ribbon: the translated command name, its icon and
// the command it runs.
type ribbonButton struct {
	id, command string
	icon        icons.Name
}

// ribbonGroups returns the ribbon's groups in order: Draw, Modify and Utilities.
func (ws *Workspace) ribbonGroups() []ribbonGroup {
	draw := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Child(
		ws.ribbonLarge(ribbonButton{"line", "LINE", icons.DrawLine}),
		ws.ribbonLarge(ribbonButton{"pline", "PLINE", icons.DrawPolyline}),
		ws.ribbonLarge(ribbonButton{"circle", "CIRCLE", icons.DrawCircle}),
		ws.ribbonLarge(ribbonButton{"arc", "ARC", icons.DrawArc}),
		ws.ribbonSmall(ribbonButton{"rect", "RECTANG", icons.DrawRectangle}),
	)
	return []ribbonGroup{
		{"draw", i18n.T("ui.ribbon.draw"), draw},
		{"modify", i18n.T("ui.ribbon.modify"), ws.smallColumns([][2]ribbonButton{
			{{"move", "MOVE", icons.ModifyMove}, {"copy", "COPY", icons.ModifyCopy}},
			{{"rotate", "ROTATE", icons.ModifyRotate}, {"scale", "SCALE", icons.ModifyScale}},
			{{"mirror", "MIRROR", icons.ModifyMirror}, {"erase", "ERASE", icons.ModifyErase}},
			{{"offset", "OFFSET", icons.ModifyOffset}, {"fillet", "FILLET", icons.ModifyFillet}},
			{{"chamfer", "CHAMFER", icons.ModifyChamfer}, {"trim", "TRIM", icons.ModifyTrim}},
			{{"extend", "EXTEND", icons.ModifyExtend}, {"break", "BREAK", icons.ModifyBreak}},
			{{"join", "JOIN", icons.ModifyJoin}, {"explode", "EXPLODE", icons.ModifyExplode}},
		})},
		{"utilities", i18n.T("ui.ribbon.utilities"), ws.smallColumns([][2]ribbonButton{
			{{"dist", "DIST", icons.MeasureDistance}, {"area", "AREA", icons.MeasureArea}},
			{{"id", "ID", icons.MeasureIdPoint}, {"zoom", "ZOOM", icons.ViewZoomWindow}},
		})},
	}
}

// ribbon lays the groups out in a wrapping flex row: on a wide window they share one row, and as
// the window narrows whole groups drop to the next row. Each group has its name centred under
// its buttons and a vertical divider after it. window is the window's width.
func (ws *Workspace) ribbon(window float32) *gpui.Div {
	row := gpui.NewDiv().ID("ribbon").FlexRow().FlexWrap().ItemsStretch()
	groups := ws.ribbonGroups()
	// When the three groups do not fit one row, the small Utilities group moves before Modify so
	// it can sit beside Draw, and the ribbon needs two rows rather than three.
	if window < ribbonWidth() {
		groups[1], groups[2] = groups[2], groups[1]
	}
	for _, g := range groups {
		name := gpui.NewDiv().FlexRow().JustifyCenter().Child(gpui.NewText(g.name).TextSize(fontSmall).TextColor(uiMuted))
		row.Child(gpui.NewDiv().ID("tbgroup-"+g.id).FlexRow().FlexShrink(0).Child(
			gpui.NewDiv().FlexCol().Gap(4).Px_(10).Pt(10).Pb(6).Child(
				gpui.NewDiv().FlexRow().ItemsCenter().JustifyCenter().SizeH(ribbonBodyHeight).Child(g.body),
				name),
			gpui.NewDiv().SizeW(1).Bg(uiBorder),
		))
	}
	return gpui.NewDiv().FlexCol().FlexShrink(0).Px_(6).Bg(uiPanel).Child(row)
}

// ribbonWidth estimates the width of the ribbon with its groups on one row, from the length of
// the translated labels: the layout engine wraps groups but cannot say whether it did.
func ribbonWidth() float32 {
	label := func(command string) float32 {
		return float32(utf8.RuneCountInString(i18n.Command(command).Name)) * ribbonCharWidth
	}
	large := func(command string) float32 { return max(ribbonLargeIcon, label(command)) + 22 }
	small := func(a, b string) float32 { return max(label(a), label(b)) + 8 + ribbonSmallIcon + 8 + 8 + 2 }
	draw := large("LINE") + large("PLINE") + large("CIRCLE") + large("ARC") + small("RECTANG", "RECTANG") + 21
	modify := small("MOVE", "COPY") + small("ROTATE", "SCALE") + small("MIRROR", "ERASE") + small("OFFSET", "FILLET") +
		small("CHAMFER", "TRIM") + small("EXTEND", "BREAK") + small("JOIN", "EXPLODE") + 21
	utilities := small("DIST", "AREA") + small("ID", "ZOOM") + 21
	return draw + modify + utilities + 14
}

// ribbonButtonBase is what both button shapes share: the id, the command's tooltip, hover, the
// click that runs the command and the soft accent fill while the command is the active one.
func (ws *Workspace) ribbonButtonBase(b ribbonButton) (d *gpui.Div, col gpui.Rgba) {
	col = uiText
	d = gpui.NewDiv().ID("tool-" + b.id).Rounded(6).FlexShrink(0).Tooltip(i18n.Command(b.command).Tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.run(b.command, w) })
	if ws.session.Active() == b.command {
		d.Bg(activeFill())
		col = uiOn
	}
	return d, col
}

// ribbonLarge is a ribbon button with its icon above its label.
func (ws *Workspace) ribbonLarge(b ribbonButton) *gpui.Div {
	d, col := ws.ribbonButtonBase(b)
	return d.FlexCol().ItemsCenter().JustifyCenter().Gap(6).Px_(10).SizeH(ribbonBodyHeight-6).
		Child(NewIcon(b.icon, ribbonLargeIcon, col), gpui.NewText(i18n.Command(b.command).Name).TextSize(fontUI).TextColor(col))
}

// ribbonSmall is a ribbon button with its icon left of its label.
func (ws *Workspace) ribbonSmall(b ribbonButton) *gpui.Div {
	d, col := ws.ribbonButtonBase(b)
	return d.FlexRow().ItemsCenter().Gap(8).Px_(8).SizeH(ribbonSmallHeight).
		Child(NewIcon(b.icon, ribbonSmallIcon, col), gpui.NewText(i18n.Command(b.command).Name).TextSize(fontUI).TextColor(col))
}

// smallColumns lays small buttons out as columns of two, so the second row lines up under the
// first and both share the column's width.
func (ws *Workspace) smallColumns(cols [][2]ribbonButton) *gpui.Div {
	row := gpui.NewDiv().FlexRow().ItemsStretch().Gap(2)
	for _, c := range cols {
		row.Child(gpui.NewDiv().FlexCol().ItemsStretch().Gap(2).Child(ws.ribbonSmall(c[0]), ws.ribbonSmall(c[1])))
	}
	return row
}
