package ui

import (
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/filedialog"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Save as file format: the native save dialogs cannot report a chosen file type (osascript has
// none), so Save as first shows a small menu of the formats under the Save button, the drawing's
// current one ticked, and then the system save dialog listing that format alone. The answer runs
// SAVEAS with the format keyword and the path, as typing them would.

// saveFormatMenuWidth is the format menu's width.
const saveFormatMenuWidth = 300

// formatLister is what a file handler offers when its writer writes several versions
// (fileio.Registry does).
type formatLister interface {
	SaveVersions(path string) []string
	SaveVersion(path string, d *doc.Drawing, version string) string
}

// saveFormats returns the versions Save as offers, newest first as AutoCAD's dialog lists them,
// and the one the drawing saves in now. It returns nil when there is no choice to make.
func (ws *Workspace) saveFormats() (versions []string, current string) {
	l, ok := ws.files.(formatLister)
	if !ok {
		return nil, ""
	}
	path := ""
	if p := ws.session.FilePath(); p != "" {
		path = ws.writablePath(p)
	}
	for _, v := range l.SaveVersions(path) {
		if commands.FormatKeyword(v) != "" {
			versions = append([]string{v}, versions...)
		}
	}
	if len(versions) < 2 {
		return nil, ""
	}
	d := ws.session.Drawing()
	return versions, l.SaveVersion(path, d, d.Header.Version)
}

// saveExt is the extension the save dialog's format entry names: the first a writer writes.
func (ws *Workspace) saveExt() string {
	if l, ok := ws.files.(extensionLister); ok {
		if exts := l.WriteExtensions(); len(exts) > 0 {
			return exts[0]
		}
	}
	return ".dxf"
}

// formatLabel is a format's entry in the menu and the save dialog: "AutoCAD 2000/LT2000 DXF (*.dxf)".
func (ws *Workspace) formatLabel(version string) string {
	return i18n.T("ui.file.format_row", commands.FormatName(i18n.For(i18n.Current()), version), ws.saveExt())
}

// formatFilters is the save dialog's type list for version: that format alone.
func (ws *Workspace) formatFilters(version string) []filedialog.Filter {
	return []filedialog.Filter{{Name: ws.formatLabel(version), Extensions: []string{strings.TrimPrefix(ws.saveExt(), ".")}}}
}

// SaveFormatMenuShown reports whether the Save as format menu is showing.
func (ws *Workspace) SaveFormatMenuShown() bool { return ws.dlg.formatMenu }

// chooseSaveFormat is a click on a format row: the menu closes and the save dialog for that format
// opens.
func (ws *Workspace) chooseSaveFormat(version string, w *gpui.Window) {
	ws.dlg.formatMenu = false
	ws.startSaveDialog(w, dialogSaveAs, "", version)
	w.Refresh()
}

// runSaveAs runs SAVEAS and answers its format prompt with version (when it asks one and version is
// set) and its path prompt with path, as a typed SAVEAS would be answered.
func (ws *Workspace) runSaveAs(version, path string, w *gpui.Window) {
	ws.run("SAVEAS", w)
	if kw := commands.FormatKeyword(version); kw != "" {
		if p := ws.session.CurrentPrompt(); ws.session.Active() == "SAVEAS" && p != nil && p.Accepts&cmdsys.InputKeyword != 0 {
			ws.submit("_"+kw, w)
		}
	}
	ws.submit(path, w)
}

// saveButton is the title bar's Save button: QSAVE on a click, Save as on a right click. A marker
// in front of it records its position while painting, so the format menu can hang under it.
func (ws *Workspace) saveButton() *gpui.Div {
	mark := gpui.NewDiv().SizeWH(0, iconButtonSize).FlexShrink(0).Child(gpui.NewCanvas(func(b gpui.Bounds, _ *gpui.Window, _ *gpui.App) {
		ws.dlg.saveAnchor = b
	}))
	return gpui.NewDiv().FlexRow().ItemsCenter().FlexShrink(0).Child(mark,
		ws.fileButton("save", ws.saveTip(), icons.FileSave, ws.saveFile, ws.saveFileAs))
}

// saveFormatOverlay is the format menu over the whole window: a transparent backdrop that closes it
// on any press outside, and the menu under the Save button. Closed, it is an empty absolute box.
func (ws *Workspace) saveFormatOverlay() *gpui.Div {
	versions, current := ws.saveFormats()
	if !ws.dlg.formatMenu || versions == nil {
		ws.dlg.formatMenu = false
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	a := ws.dlg.saveAnchor
	menu := gpui.NewDiv().ID("save-format-menu").Absolute().Left(float32(a.Origin.X)).Top(float32(a.Origin.Y+a.Size.H)+6).
		SizeW(saveFormatMenuWidth).FlexCol().Gap(1).P(6).Rounded(8).Bg(uiPanelAlt).Border(1, uiBorder).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(menuHeading(i18n.T("ui.file.format_title")))
	for _, v := range versions {
		v := v
		tick := gpui.NewDiv().SizeW(16).FlexShrink(0)
		if v == current {
			tick.Child(NewIcon(icons.AppCheck, 14, uiOn))
		}
		name := commands.FormatName(i18n.For(i18n.Current()), v)
		menu.Child(menuRow("save-format-"+strings.ToLower(v), i18n.T("ui.file.format_tip", name), func(w *gpui.Window) { ws.chooseSaveFormat(v, w) }).
			Child(tick, gpui.NewText(ws.formatLabel(v)).TextSize(fontUI).TextColor(uiText)))
	}
	return gpui.NewDiv().ID("save-format-backdrop").Absolute().Inset(0).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			w.StopPropagation()
			ws.dlg.formatMenu = false
			w.Refresh()
		}).
		Child(menu)
}
