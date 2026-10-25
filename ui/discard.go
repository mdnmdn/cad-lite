package ui

import (
	"fmt"

	"cad-lite/cmdsys"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// discardDialog is the unsaved-changes dialog that closing a drawing, quitting and the window's
// close button show in place of the command line's "Save changes" question: one drawing's name,
// or, when quitting, every drawing with unsaved changes, and two buttons, Close anyway and Cancel.
// Scripts keep the command-line question (cmdsys.DiscardConfirmer).
type discardDialog struct {
	// quit is set when the whole application closes; items are then the dirty drawings.
	quit  bool
	tab   *docTab
	items []*docTab
	// proceed is what Close anyway does: close the tab, or end the application.
	proceed func()
	// cancelFH and closeFH are the buttons' focus handles; focusWanted moves the focus to Cancel
	// on the first frame.
	cancelFH, closeFH *gpui.FocusHandle
	focusWanted       bool
}

// ConfirmClose is tabHost's cmdsys.DiscardConfirmer side: with a window, CLOSE of a drawing with
// unsaved changes asks in the dialog, and close runs on Close anyway.
func (h tabHost) ConfirmClose(close func()) bool {
	if h.ws.win == nil {
		return false
	}
	h.ws.askClose(h.t, close)
	return true
}

// ConfirmQuit is tabHost's cmdsys.DiscardConfirmer side: with a window and a quit handler, QUIT
// and EXIT show one dialog listing every drawing with unsaved changes, or quit at once when there
// is none. A quit that a script started keeps asking each tab on the command line.
func (h tabHost) ConfirmQuit() bool {
	ws := h.ws
	if ws.win == nil || ws.quit.f == nil || ws.quit.chaining {
		return false
	}
	ws.askQuit(ws.quit.f)
	return true
}

var _ cmdsys.DiscardConfirmer = tabHost{}

// askClose shows the dialog for closing tab t; close runs on Close anyway.
func (ws *Workspace) askClose(t *docTab, close func()) {
	ws.discard = &discardDialog{tab: t, proceed: close, focusWanted: true}
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// askQuit ends the application through quit, at once when no drawing has unsaved changes and
// otherwise after the dialog listing them.
func (ws *Workspace) askQuit(quit func()) {
	var dirty []*docTab
	for _, t := range ws.tabs {
		if t.session.Dirty() {
			dirty = append(dirty, t)
		}
	}
	if len(dirty) == 0 {
		ws.discard = nil
		quit()
		return
	}
	ws.quit.next, ws.quit.asked = nil, nil
	ws.discard = &discardDialog{quit: true, items: dirty, proceed: quit, focusWanted: true}
	if ws.win != nil {
		ws.win.Refresh()
	}
}

// windowClosing is the window's OnShouldClose handler: the close button quits at once when no
// drawing has unsaved changes, and otherwise shows the quit dialog and keeps the window open.
func (ws *Workspace) windowClosing(w *gpui.Window, cx *gpui.App) bool {
	ws.flushPropEdit(w)
	if d := ws.discard; d != nil && d.quit {
		return false
	}
	if !ws.anyDirty() {
		return true
	}
	quit := ws.quit.f
	if quit == nil {
		quit = cx.Quit
	}
	ws.askQuit(quit)
	return false
}

// finishDiscard closes the dialog and, for Close anyway, closes the tab or quits.
func (ws *Workspace) finishDiscard(w *gpui.Window, proceed bool) {
	d := ws.discard
	if d == nil {
		return
	}
	ws.discard = nil
	if w != nil {
		if fh := ws.input.FocusHandle(); fh != nil {
			w.RequestFocus(fh)
		}
		w.Refresh()
	}
	if proceed {
		d.proceed()
	}
}

// DiscardDialog reports what the unsaved-changes dialog shows, for tests and scripting: whether
// it is open, whether it is the quit dialog, and the names of the drawings it lists.
func (ws *Workspace) DiscardDialog() (open, quit bool, names []string) {
	d := ws.discard
	if d == nil {
		return false, false, nil
	}
	if !d.quit {
		return true, false, []string{d.tab.name()}
	}
	for _, t := range d.items {
		names = append(names, t.name())
	}
	return true, true, names
}

// withDiscard lays the dialog over the window while it is open.
func (ws *Workspace) withDiscard(w *gpui.Window, cx *gpui.App, inner gpui.IntoElement) gpui.IntoElement {
	d := ws.discard
	if d == nil {
		return inner
	}
	return gpui.NewDiv().Relative().FlexCol().SizeFull().Child(inner, ws.discardOverlay(w, cx, d))
}

// discardOverlay is the dimmed backdrop and the dialog card. Escape is Cancel, Tab moves between
// the two buttons, Enter and Space press the focused one; Cancel has the focus when it opens.
func (ws *Workspace) discardOverlay(w *gpui.Window, cx *gpui.App, d *discardDialog) gpui.IntoElement {
	if d.cancelFH == nil {
		d.cancelFH = cx.NewFocusHandle().SetTabIndex(1).SetTabStop(true)
		d.closeFH = cx.NewFocusHandle().SetTabIndex(2).SetTabStop(true)
	}
	if d.focusWanted || (!d.cancelFH.IsFocused(w) && !d.closeFH.IsFocused(w)) {
		d.focusWanted = false
		w.RequestFocus(d.cancelFH)
	}
	button := func(id, label, tip string, fh *gpui.FocusHandle, proceed bool) *gpui.Div {
		fire := func(w *gpui.Window) { ws.finishDiscard(w, proceed) }
		b := gpui.NewDiv().ID(id).TrackFocus(fh).Px_(16).Py_(8).Rounded(5).Border(1, uiBorder).Tooltip(tip).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { fire(w); w.StopPropagation() }).
			OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
				if e.Keystroke.Key == "enter" || e.Keystroke.Key == "space" {
					fire(w)
					w.StopPropagation()
				}
			}).
			Focus(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(2, uiOn) }).
			Child(gpui.NewText(label))
		if proceed {
			// The destructive choice is red, as the error colour marks loss elsewhere.
			b.Bg(uiErr).TextColor(uiOnPrimary).FontWeight(700)
		} else {
			b.Bg(uiPanelAlt).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) })
		}
		return b
	}
	card := gpui.NewDiv().ID("discard-dialog").FlexCol().Gap(12).P(20).SizeW(440).MaxW(max(0, float32(w.Bounds().Size.W)-32)).
		Bg(uiPanel).Border(1, uiBorder).Rounded(10).ShadowXl().TextColor(uiText).TextSize(13).
		Child(gpui.NewText(i18n.T("ui.discard.title")).FontWeight(700).TextSize(16))
	if d.quit {
		card.Child(gpui.NewText(i18n.T("ui.discard.quit_body")))
		list := gpui.NewDiv().ID("discard-list").FlexCol().Gap(2).P(8).Rounded(6).Bg(uiPanelAlt).MaxH(180).OverflowYScroll()
		for i, t := range d.items {
			tip := i18n.T("ui.tabs.untitled_tip")
			if p := t.session.FilePath(); p != "" {
				tip = p
			}
			list.Child(gpui.NewDiv().ID(fmt.Sprintf("discard-item-%d", i)).FlexRow().ItemsCenter().Gap(8).Py_(2).Tooltip(tip).
				Child(dot(7, uiOn), gpui.NewText(t.name()).FontWeight(700)))
		}
		card.Child(list, gpui.NewText(i18n.T("ui.discard.quit_question")))
	} else {
		card.Child(gpui.NewText(i18n.T("ui.discard.close_body", d.tab.name())))
	}
	card.Child(
		gpui.NewText(i18n.T("ui.discard.save_hint", ws.keyHint(actionSave, "SAVE"))).TextSize(fontSmall).TextColor(uiMuted),
		gpui.NewDiv().FlexRow().Gap(8).JustifyEnd().Child(
			button("discard-cancel", i18n.T("ui.discard.cancel"), i18n.T("ui.discard.cancel_tip"), d.cancelFH, false),
			button("discard-close", i18n.T("ui.discard.close_anyway"), i18n.T("ui.discard.close_anyway_tip"), d.closeFH, true),
		))
	// Bindings resolve before raw key events; consume the drawing's actions on the dialog's path.
	for _, b := range Bindings() {
		card.OnAction(b.Action, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	card.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		switch e.Keystroke.Key {
		case "escape":
			ws.finishDiscard(w, false)
		case "tab":
			if d.cancelFH.IsFocused(w) {
				w.RequestFocus(d.closeFH)
			} else {
				w.RequestFocus(d.cancelFH)
			}
			w.Refresh()
		}
		w.StopPropagation()
	})
	card.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	return gpui.NewDiv().ID("discard-overlay").Absolute().Inset(0).FlexCol().ItemsCenter().JustifyCenter().Bg(uiBackdrop).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnFileDrop(func(_ *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(card)
}
