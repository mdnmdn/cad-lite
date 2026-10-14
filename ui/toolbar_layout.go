package ui

import (
	"cad-lite/assets/icons"
	"cad-lite/internal/gpui"
)

// toolGroup is one run of toolbar controls that always stays on a single
// row: the toolbar wraps between groups, never inside one.
type toolGroup struct {
	id       string
	elements []gpui.IntoElement
	// divider draws a separator after the group.
	divider bool
	// right pushes the group to the right end of its row.
	right bool
}

// toolbarGroups returns the toolbar's groups in order: files (led by the
// brand), drawing, measuring, history and zoom, the three modify groups, and the
// window buttons. Commands without a short caption of their own show their
// display name.
func (ws *Workspace) toolbarGroups() []toolGroup {
	file := []gpui.IntoElement{}
	if b := ws.brand(); b != nil {
		file = append(file, b)
	}
	file = append(file,
		ws.fileButton("open", tb("open"), tb("open_tip"), icons.FileOpen, ws.openFile, nil),
		ws.fileButton("save", tb("save"), tb("save_tip"), icons.FileSave, ws.saveFile, ws.saveFileAs),
	)
	draw := []gpui.IntoElement{
		ws.button("line", tb("line"), "LINE", icons.DrawLine),
		ws.button("pline", tb("pline"), "PLINE", icons.DrawPolyline),
		ws.button("circle", tb("circle"), "CIRCLE", icons.DrawCircle),
		ws.button("arc", tb("arc"), "ARC", icons.DrawArc),
		ws.button("rect", tb("rect"), "RECTANG", icons.DrawRectangle),
	}
	return []toolGroup{
		{id: "file", elements: file, divider: true},
		{id: "draw", elements: draw, divider: true},
		{id: "measure", divider: true, elements: []gpui.IntoElement{
			ws.button("dist", tb("dist"), "DIST", icons.MeasureDistance),
			ws.button("area", tb("area"), "AREA", icons.MeasureArea),
			ws.button("id", "", "ID", icons.MeasureIdPoint),
		}},
		{id: "history", divider: true, elements: []gpui.IntoElement{
			ws.button("undo", tb("undo"), "U", icons.EditUndo),
			ws.button("redo", tb("redo"), "REDO", icons.EditRedo),
			ws.button("zoom", tb("zoom"), "ZOOM", icons.ViewZoomWindow),
		}},
		{id: "modify1", divider: true, elements: []gpui.IntoElement{
			ws.button("move", tb("move"), "MOVE", icons.ModifyMove),
			ws.button("copy", tb("copy"), "COPY", icons.ModifyCopy),
			ws.button("rotate", tb("rotate"), "ROTATE", icons.ModifyRotate),
			ws.button("scale", tb("scale"), "SCALE", icons.ModifyScale),
			ws.button("erase", tb("erase"), "ERASE", icons.ModifyErase),
		}},
		{id: "modify2", divider: true, elements: []gpui.IntoElement{
			ws.button("mirror", "", "MIRROR", icons.ModifyMirror),
			ws.button("offset", "", "OFFSET", icons.ModifyOffset),
			ws.button("fillet", "", "FILLET", icons.ModifyFillet),
			ws.button("chamfer", "", "CHAMFER", icons.ModifyChamfer),
		}},
		{id: "modify3", elements: []gpui.IntoElement{
			ws.button("trim", "", "TRIM", icons.ModifyTrim),
			ws.button("extend", "", "EXTEND", icons.ModifyExtend),
			ws.button("break", "", "BREAK", icons.ModifyBreak),
			ws.button("join", "", "JOIN", icons.ModifyJoin),
			ws.button("explode", "", "EXPLODE", icons.ModifyExplode),
		}},
		{id: "window", right: true, elements: []gpui.IntoElement{
			ws.agentToggle(),
			ws.settingsButton(),
		}},
	}
}

// toolbar lays the groups out in a wrapping flex row: on a wide window they
// share one row with the window buttons at the right end, and as the window
// narrows whole groups drop to the next row.
func (ws *Workspace) toolbar() *gpui.Div {
	row := gpui.NewDiv().ID("toolbar").FlexRow().FlexWrap().ItemsCenter().Gap(2).RowGap(2)
	for _, g := range ws.toolbarGroups() {
		d := gpui.NewDiv().ID("tbgroup-" + g.id).FlexRow().ItemsCenter().Gap(2).FlexShrink(0)
		for _, e := range g.elements {
			d.Child(e)
		}
		if g.divider {
			d.Child(separator())
		}
		if g.right {
			d.MlAuto()
		}
		row.Child(d)
	}
	return gpui.NewDiv().FlexCol().Px_(8).Py_(4).Bg(uiPanel).Child(row)
}
