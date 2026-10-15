package ui

import (
	"strings"

	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

func (ws *Workspace) withTextEditor(w *gpui.Window, cx *gpui.App, root *gpui.Div) *gpui.Div {
	s := ws.textEditor
	if s == nil {
		return root
	}
	_, multi := s.original.(*doc.MText)
	body := gpui.NewDiv().ID("textedit-body").FlexCol().Gap(12).P(18).Flex1().MinH(0).
		OverflowYScroll().ShowScrollbar().TrackScroll(s.scroll)
	names := []string{"content", "height", "rotation", "justify", "style"}
	if multi {
		names = append(names, "width")
	}
	for i, name := range names {
		f := s.fields[name]
		field := f.Render(w, cx)
		if fh := f.FocusHandle(); fh != nil {
			fh.SetTabIndex(i + 1).SetTabStop(true)
			if name == "content" && s.focusWanted {
				s.focusWanted = false
				w.RequestFocus(fh)
			}
		}
		row := gpui.NewDiv().FlexCol().Gap(4).FlexShrink(0).Child(
			gpui.NewText(i18n.T("ui.textedit."+name)).TextColor(uiMuted),
			gpui.NewDiv().ID("textedit-"+name).FlexCol().Child(field))
		switch name {
		case "content":
			if multi {
				row.Child(gpui.NewText(i18n.T("ui.textedit.plain_hint")).TextSize(fontSmall).TextColor(uiMuted))
			}
		case "justify":
			opts := "Left, Center, Right, Align, Middle, Fit, TL, TC, TR, ML, MC, MR, BL, BC, BR"
			if multi {
				opts = "TL, TC, TR, ML, MC, MR, BL, BC, BR"
			}
			row.Child(gpui.NewText(opts).TextSize(fontSmall).TextColor(uiMuted))
		case "style":
			row.Child(gpui.NewText(strings.Join(doc.TextStyles(s.drawing), ", ")).TextSize(fontSmall).TextColor(uiMuted))
		}
		body.Child(row)
	}
	button := func(id, label, tip string, tab int, apply bool) *gpui.Div {
		fh := s.buttons[id]
		if fh == nil {
			fh = cx.NewFocusHandle().SetTabIndex(tab).SetTabStop(true)
			s.buttons[id] = fh
		}
		fire := func(w *gpui.Window) {
			if apply {
				ws.applyTextEditor(w)
			} else {
				ws.closeTextEditor(w)
			}
		}
		b := gpui.NewDiv().ID(id).TrackFocus(fh).Px_(16).Py_(8).Rounded(5).Bg(uiPanelAlt).
			Border(1, uiBorder).Tooltip(tip).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { fire(w) }).
			OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
				if e.Keystroke.Key == "enter" || e.Keystroke.Key == "space" {
					fire(w)
					w.StopPropagation()
				}
			}).
			Focus(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, uiAccent) }).
			Child(gpui.NewText(label))
		if apply {
			b.Bg(uiPrimary).TextColor(uiOnPrimary)
		}
		return b
	}
	footer := gpui.NewDiv().FlexCol().Gap(8).P(16).FlexShrink(0)
	if s.err != "" {
		footer.Child(gpui.NewText(s.err).TextColor(uiErr))
	}
	footer.Child(gpui.NewDiv().FlexRow().JustifyEnd().Gap(8).Child(
		button("textedit-cancel", i18n.T("ui.textedit.cancel"), i18n.T("ui.textedit.cancel_tip"), len(names)+1, false),
		button("textedit-apply", i18n.T("ui.textedit.apply"), i18n.T("ui.textedit.apply_tip"), len(names)+2, true)))
	card := gpui.NewDiv().ID("textedit-card").FlexCol().SizeW(560).MaxW(max(0, float32(w.Bounds().Size.W)-32)).
		MaxH(max(0, float32(w.Bounds().Size.H)-32)).Bg(uiPanel).Border(1, uiBorder).Rounded(8).
		Child(gpui.NewDiv().Px_(18).Py_(14).FlexShrink(0).Child(
			gpui.NewText(i18n.T("ui.textedit.title", s.original.Class(), s.original.Handle().String())).FontWeight(700).TextSize(16)), body, footer)
	// Bindings resolve before raw key events; consume drawing actions on the modal's focus path.
	for _, b := range Bindings() {
		card.OnAction(b.Action, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	card.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		if e.Keystroke.Key == "escape" {
			ws.closeTextEditor(w)
		}
		if e.Keystroke.Key == "tab" && e.Keystroke.Modifiers&^gpui.Shift == 0 {
			var handles []*gpui.FocusHandle
			for _, name := range names {
				handles = append(handles, s.fields[name].FocusHandle())
			}
			handles = append(handles, s.buttons["textedit-cancel"], s.buttons["textedit-apply"])
			current := 0
			for i, h := range handles {
				if h != nil && h.IsFocused(w) {
					current = i
				}
			}
			delta := 1
			if e.Keystroke.Modifiers&gpui.Shift != 0 {
				delta = -1
			}
			next := (current + delta + len(handles)) % len(handles)
			w.RequestFocus(handles[next])
			if next < len(names) {
				s.scroll.ScrollToItem(next)
			}
		}
		w.StopPropagation()
	})
	hasFocus := false
	for _, name := range names {
		if fh := s.fields[name].FocusHandle(); fh != nil && fh.IsFocused(w) {
			hasFocus = true
		}
	}
	for _, fh := range s.buttons {
		hasFocus = hasFocus || fh.IsFocused(w)
	}
	if !hasFocus {
		w.RequestFocus(s.fields["content"].FocusHandle())
	}
	overlay := gpui.NewDiv().ID("textedit-overlay").Absolute().Inset(0).FlexCol().ItemsCenter().JustifyCenter().Bg(uiBackdrop).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnFileDrop(func(_ *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(e *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) {
			off := s.scroll.Offset()
			off.Y = min(max(off.Y+e.DeltaY, 0), s.scroll.MaxOffset().H)
			s.scroll.SetOffset(off)
			w.StopPropagation()
			w.Refresh()
		}).Child(card)
	return root.Child(overlay)
}
