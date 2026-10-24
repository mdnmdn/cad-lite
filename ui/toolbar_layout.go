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
	ribbonLargeIcon = 22
	ribbonSmallIcon = 16
	// ribbonBodyHeight is the height of a group's buttons: one large button, or two small ones.
	ribbonBodyHeight = 58
	// ribbonSmallHeight is the height of a small button: two of them and the gap between fill the
	// body.
	ribbonSmallHeight = (ribbonBodyHeight - 2) / 2
	// ribbonGroupPad is the space either side of a group's buttons; the group's divider adds 1.
	ribbonGroupPad = 8
	// ribbonLargePad and ribbonSmallPad are the horizontal padding of the two button shapes;
	// ribbonSmallGap is the space between a small button's icon and its label.
	ribbonLargePad = 8
	ribbonSmallPad = 5
	ribbonSmallGap = 5
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

// ribbonSpec is the content of one group: large buttons (icon over label) first, then columns of
// two small ones (icon left of label). Both the layout and the width estimate read it.
type ribbonSpec struct {
	id, nameKey string
	large       []ribbonButton
	small       [][2]ribbonButton
}

// ribbonSpecs lists the ribbon's groups in AutoCAD's Home tab order: Draw, Modify, Annotation and
// Utilities.
func ribbonSpecs() []ribbonSpec {
	return []ribbonSpec{
		{id: "draw", nameKey: "ui.ribbon.draw",
			large: []ribbonButton{
				{"line", "LINE", icons.DrawLine}, {"pline", "PLINE", icons.DrawPolyline},
				{"circle", "CIRCLE", icons.DrawCircle}, {"arc", "ARC", icons.DrawArc},
			},
			small: [][2]ribbonButton{{{"rect", "RECTANG", icons.DrawRectangle}, {"polygon", "POLYGON", icons.DrawPolygon}}}},
		{id: "modify", nameKey: "ui.ribbon.modify", small: [][2]ribbonButton{
			{{"move", "MOVE", icons.ModifyMove}, {"copy", "COPY", icons.ModifyCopy}},
			{{"rotate", "ROTATE", icons.ModifyRotate}, {"scale", "SCALE", icons.ModifyScale}},
			{{"mirror", "MIRROR", icons.ModifyMirror}, {"erase", "ERASE", icons.ModifyErase}},
			{{"offset", "OFFSET", icons.ModifyOffset}, {"fillet", "FILLET", icons.ModifyFillet}},
			{{"chamfer", "CHAMFER", icons.ModifyChamfer}, {"trim", "TRIM", icons.ModifyTrim}},
			{{"extend", "EXTEND", icons.ModifyExtend}, {"break", "BREAK", icons.ModifyBreak}},
			{{"join", "JOIN", icons.ModifyJoin}, {"explode", "EXPLODE", icons.ModifyExplode}},
		}},
		{id: "annotation", nameKey: "ui.ribbon.annotation", small: [][2]ribbonButton{
			{{"dimlinear", "DIMLINEAR", icons.DimLinear}, {"dimaligned", "DIMALIGNED", icons.DimAligned}},
			{{"dimcontinue", "DIMCONTINUE", icons.DimContinue}, {"dimarc", "DIMARC", icons.DimArc}},
		}},
		{id: "utilities", nameKey: "ui.ribbon.utilities", small: [][2]ribbonButton{
			{{"dist", "DIST", icons.MeasureDistance}, {"area", "AREA", icons.MeasureArea}},
			{{"id", "ID", icons.MeasureIdPoint}, {"zoom", "ZOOM", icons.ViewZoomWindow}},
		}},
	}
}

// ribbonGroups returns the ribbon's groups in the order of ribbonSpecs.
func (ws *Workspace) ribbonGroups() []ribbonGroup {
	var out []ribbonGroup
	for _, sp := range ribbonSpecs() {
		body := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2)
		for _, b := range sp.large {
			body.Child(ws.ribbonLarge(b))
		}
		if len(sp.small) > 0 {
			body.Child(ws.smallColumns(sp.small))
		}
		out = append(out, ribbonGroup{sp.id, i18n.T(sp.nameKey), body})
	}
	return out
}

// ribbon lays the groups out in a wrapping flex row: on a wide window they share one row, and as
// the window narrows whole groups drop to the next row. Each group has its name centred under
// its buttons and a vertical divider after it. window is the window's width.
func (ws *Workspace) ribbon(window float32) *gpui.Div {
	row := gpui.NewDiv().ID("ribbon").FlexRow().FlexWrap().ItemsStretch()
	groups := ws.ribbonGroups()
	// When Draw and Modify do not fit one row, the smaller Annotation and Utilities groups move
	// before Modify so they can sit beside Draw, and the ribbon needs two rows rather than three.
	if w := ribbonWidths(); window < w[0]+w[1] {
		groups = append([]ribbonGroup{groups[0]}, append(groups[2:], groups[1])...)
	}
	for _, g := range groups {
		name := gpui.NewDiv().FlexRow().JustifyCenter().Child(gpui.NewText(g.name).TextSize(fontSmall).TextColor(uiMuted))
		row.Child(gpui.NewDiv().ID("tbgroup-"+g.id).FlexRow().FlexShrink(0).Child(
			gpui.NewDiv().FlexCol().Gap(2).Px_(ribbonGroupPad).Pt(6).Pb(4).Child(
				gpui.NewDiv().FlexRow().ItemsCenter().JustifyCenter().SizeH(ribbonBodyHeight).Child(g.body),
				name),
			gpui.NewDiv().SizeW(1).Bg(uiBorder),
		))
	}
	return gpui.NewDiv().FlexCol().FlexShrink(0).Px_(4).Bg(uiPanel).Child(row)
}

// ribbonWidths estimates the width of each group of ribbonSpecs, in order, from the length of the
// translated labels: the layout engine wraps groups but cannot say whether it did. The estimate
// errs wide, so a reorder it triggers never costs a row.
func ribbonWidths() []float32 {
	label := func(command string) float32 {
		return float32(utf8.RuneCountInString(i18n.Command(command).Name)) * ribbonCharWidth
	}
	var out []float32
	for _, sp := range ribbonSpecs() {
		w := float32(2*ribbonGroupPad + 1)
		for _, b := range sp.large {
			w += max(ribbonLargeIcon, label(b.command)) + 2*ribbonLargePad + 2
		}
		for _, c := range sp.small {
			w += max(label(c[0].command), label(c[1].command)) + 2*ribbonSmallPad + ribbonSmallIcon + ribbonSmallGap + 2
		}
		out = append(out, w)
	}
	return out
}

// ribbonButtonBase is what both button shapes share: the id, the command's tooltip, hover, the
// click that runs the command and the soft accent fill while the command is the active one.
func (ws *Workspace) ribbonButtonBase(b ribbonButton) (d *gpui.Div, col gpui.Rgba) {
	col = uiText
	d = gpui.NewDiv().ID("tool-" + b.id).Rounded(6).FlexShrink(0).Tooltip(ws.commandTip(b.command)).
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
	return d.FlexCol().ItemsCenter().JustifyCenter().Gap(4).Px_(ribbonLargePad).SizeH(ribbonBodyHeight-2).
		Child(NewIcon(b.icon, ribbonLargeIcon, col), gpui.NewText(i18n.Command(b.command).Name).TextSize(fontUI).TextColor(col))
}

// ribbonSmall is a ribbon button with its icon left of its label.
func (ws *Workspace) ribbonSmall(b ribbonButton) *gpui.Div {
	d, col := ws.ribbonButtonBase(b)
	return d.FlexRow().ItemsCenter().Gap(ribbonSmallGap).Px_(ribbonSmallPad).SizeH(ribbonSmallHeight).
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
