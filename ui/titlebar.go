package ui

import (
	"path/filepath"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// titleBar is the window's top row: the brand, the document chip, Open and Save, Undo and Redo,
// and at the right end the agent panel toggle and Settings. Every button is icon-only, so each
// carries its tooltip.
func (ws *Workspace) titleBar() *gpui.Div {
	left := gpui.NewDiv().FlexRow().ItemsCenter().Gap(12).MinW(0)
	if b := ws.brand(); b != nil {
		left.Child(b, vDivider(24))
	}
	left.Child(
		ws.docChip(),
		vDivider(24),
		gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).FlexShrink(0).Child(
			ws.fileButton("open", tb("open_tip"), icons.FileOpen, ws.openFile, nil),
			ws.fileButton("save", tb("save_tip"), icons.FileSave, ws.saveFile, ws.saveFileAs),
		),
		vDivider(24),
		gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).FlexShrink(0).Child(
			ws.commandIcon("undo", "U", icons.EditUndo),
			ws.commandIcon("redo", "REDO", icons.EditRedo),
		),
	)
	right := gpui.NewDiv().ID("tbgroup-window").FlexRow().ItemsCenter().Gap(2).FlexShrink(0).MlAuto().Child(
		ws.agentToggle(),
		ws.settingsButton(),
	)
	return gpui.NewDiv().ID("titlebar").FlexRow().ItemsCenter().Gap(12).SizeH(titleBarHeight).FlexShrink(0).Px_(16).Bg(uiBg).Child(left, right)
}

// commandIcon is an icon-only title bar button that runs a command and shows its tooltip.
func (ws *Workspace) commandIcon(id, command string, icon icons.Name) *gpui.Div {
	return iconButton("tool-"+id, i18n.Command(command).Tip, icon, false, func(w *gpui.Window) { ws.run(command, w) })
}

// docName is the drawing's file name, or the translated "Untitled" when it has no file.
func (ws *Workspace) docName() string {
	if p := ws.session.FilePath(); p != "" {
		return filepath.Base(p)
	}
	return i18n.T("ui.title.untitled")
}

// docChip is the rounded document name with a dot in front of it while the drawing has unsaved
// changes.
func (ws *Workspace) docChip() *gpui.Div {
	dirty := ws.session.Dirty()
	tip := i18n.T("ui.title.doc_tip", ws.docName())
	if p := ws.session.FilePath(); p != "" {
		tip = i18n.T("ui.title.doc_tip", p)
	}
	if dirty {
		tip += "\n" + i18n.T("ui.status.modified_tip")
	}
	chip := gpui.NewDiv().ID("doc-chip").FlexRow().ItemsCenter().Gap(8).Px_(12).SizeH(32).Rounded(8).Bg(uiPanel).Border(1, uiBorder).
		MinW(0).OverflowHidden().Tooltip(tip)
	if dirty {
		chip.Child(dot(7, uiOn))
	}
	return chip.Child(gpui.NewText(ws.docName()).TextSize(fontUI + 1).TextColor(uiText).FlexShrink(0))
}
