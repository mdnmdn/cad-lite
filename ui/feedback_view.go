package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"cad-lite/feedback"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

func feedbackTheme() Theme {
	if CurrentTheme().Dark() {
		return themeLight
	}
	return themeDark
}

func (ws *Workspace) feedbackAction(cx *gpui.App, id, label string, enabled bool, action func(*gpui.Window)) *gpui.Div {
	f := ws.feedbackState()
	theme := feedbackTheme()
	fh := f.buttons[id]
	if fh == nil {
		fh = cx.NewFocusHandle().SetTabStop(true)
		f.buttons[id] = fh
	}
	b := gpui.NewDiv().ID(id).TrackFocus(fh).Px_(12).Py_(7).Rounded(5).Bg(theme.PanelAlt).TextColor(theme.Text).Border(1, theme.Border).Child(gpui.NewText(label))
	if !enabled {
		return b.TextColor(theme.Muted)
	}
	f.focusOrder = append(f.focusOrder, fh)
	if id == "feedback-send" || id == "feedback-edit-save" {
		b.Bg(theme.Primary).TextColor(theme.OnPrimary)
	}
	return b.Focus(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, theme.Accent) }).OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { action(w); w.StopPropagation() }).OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		if e.Keystroke.Key == "enter" || e.Keystroke.Key == "space" {
			action(w)
			w.StopPropagation()
		}
	})
}

func feedbackFit(b gpui.Bounds, img image.Image) gpui.Bounds {
	if img == nil || img.Bounds().Empty() {
		return b
	}
	scale := math.Min(float64(b.Size.W)/float64(img.Bounds().Dx()), float64(b.Size.H)/float64(img.Bounds().Dy()))
	width, height := float32(float64(img.Bounds().Dx())*scale), float32(float64(img.Bounds().Dy())*scale)
	return gpui.NewBounds(float32(b.Origin.X)+(float32(b.Size.W)-width)/2, float32(b.Origin.Y)+(float32(b.Size.H)-height)/2, width, height)
}

func (ws *Workspace) renderFeedback(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	f := ws.feedbackState()
	theme := feedbackTheme()
	f.focusOrder = nil
	if f.focus == nil {
		f.focus = cx.NewFocusHandle()
	}
	root := gpui.NewDiv().ID("feedback-root").KeyContext("feedback").TrackFocus(f.focus).FlexCol().SizeFull().Bg(theme.Bg).TextColor(theme.Text).TextSize(fontUI)
	if f.focusWanted {
		w.RequestFocus(f.focus)
		f.focusWanted = false
	}
	// A separate root excludes drawing actions and underlying pointer handlers from the modal path.
	for _, binding := range Bindings() {
		root.OnAction(binding.Action, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	root.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		if e.Keystroke.Key == "escape" {
			if f.editing {
				f.editing = false
				f.editor = nil
				f.dragging = false
				w.RequestFocus(f.focus)
			} else {
				ws.closeFeedback(w)
			}
			w.Refresh()
		}
		if e.Keystroke.Key == "tab" && len(f.focusOrder) > 0 {
			index := -1
			for i, h := range f.focusOrder {
				if h.IsFocused(w) {
					index = i
					break
				}
			}
			delta := 1
			if e.Keystroke.Modifiers&gpui.Shift != 0 {
				delta = -1
				if index < 0 {
					index = 0
				}
			}
			w.RequestFocus(f.focusOrder[(index+delta+len(f.focusOrder))%len(f.focusOrder)])
		}
		if f.editing && e.Keystroke.Key == "z" && e.Keystroke.Modifiers&(gpui.Control|gpui.ModPlatform) != 0 {
			f.editor.Undo()
			w.Refresh()
		}
		w.StopPropagation()
	})
	if f.editing {
		return ws.renderFeedbackEditor(root, w, cx)
	}
	if f.background != nil {
		background := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) { w.PaintImage(b, f.background) })
		background.Absolute().Inset(0)
		root.Child(background)
	}
	for _, input := range []*gpui.TextInput{f.title, f.description} {
		input.Style(func(d *gpui.Div) *gpui.Div {
			return d.Bg(theme.Field).Border(1, theme.Border).TextColor(theme.Text).
				Focus(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, theme.Accent) })
		}).PlaceholderColor(theme.Muted).SelectionColor(theme.Select)
	}
	body := gpui.NewDiv().FlexCol().Gap(10).FlexShrink(0)
	if f.shot != nil {
		preview := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) { w.PaintImage(feedbackFit(b, f.shot), f.shot) })
		body.Child(gpui.NewDiv().ID("feedback-preview").FlexCol().SizeH(160).FlexShrink(0).Bg(theme.Bg).OverflowHidden().Child(preview).OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			if !f.busy {
				f.editor = feedback.NewEditor(f.shot)
				f.editing = true
				w.RequestFocus(f.focus)
				w.Refresh()
			}
		}))
	}
	body.Child(gpui.NewDiv().FlexRow().FlexWrap().Gap(6).FlexShrink(0).Child(
		ws.feedbackAction(cx, "feedback-capture", i18n.T("ui.feedback.capture"), !f.busy && f.background != nil, func(w *gpui.Window) { f.shot = feedback.PrepareImage(f.background); f.revision++; w.Refresh() }),
		ws.feedbackAction(cx, "feedback-attach", i18n.T("ui.feedback.attach"), !f.busy && ws.dlg.dialog != nil, ws.attachFeedback),
		ws.feedbackAction(cx, "feedback-remove", i18n.T("ui.feedback.remove"), !f.busy && f.shot != nil, func(w *gpui.Window) { f.shot = nil; f.revision++; w.Refresh() })))
	if f.shot != nil {
		body.Child(gpui.NewText(i18n.T("ui.feedback.preview_hint")).TextSize(fontSmall).TextColor(theme.Muted))
	}
	if !f.sending {
		body.Child(gpui.NewText(i18n.T("ui.feedback.title")).TextColor(theme.Muted), f.title.Render(w, cx), gpui.NewText(i18n.T("ui.feedback.kind")).TextColor(theme.Muted))
		f.focusOrder = append(f.focusOrder, f.title.FocusHandle())
		kinds := gpui.NewDiv().FlexRow().FlexWrap().Gap(6).FlexShrink(0)
		for _, kind := range f.kinds {
			label := kind
			if kind == "bug" || kind == "improvement" || kind == "note" {
				label = i18n.T("ui.feedback." + kind)
			}
			b := ws.feedbackAction(cx, "feedback-kind-"+kind, label, true, func(w *gpui.Window) { f.kind = kind; w.Refresh() })
			if f.kind == kind {
				b.Border(1, theme.Accent)
			}
			kinds.Child(b)
		}
		body.Child(kinds, gpui.NewText(i18n.T("ui.feedback.description")).TextColor(theme.Muted), f.description.Render(w, cx))
		f.focusOrder = append(f.focusOrder, f.description.FocusHandle())
	} else {
		body.Child(gpui.NewText(i18n.T("ui.feedback.working")))
	}
	body.Child(gpui.NewText(i18n.T("ui.feedback.metadata")).TextSize(fontSmall).TextColor(theme.Muted))
	if !f.client.Configured() {
		body.Child(gpui.NewText(i18n.T("ui.feedback.unconfigured")).TextColor(theme.Muted))
	}
	footer := gpui.NewDiv().FlexCol().Gap(8).P(16).FlexShrink(0)
	if f.message != "" {
		footer.Child(gpui.NewText(f.message).TextSize(fontSmall))
	}
	footer.Child(gpui.NewDiv().FlexRow().FlexWrap().Gap(6).Child(
		ws.feedbackAction(cx, "feedback-test", i18n.T("ui.feedback.test"), !f.busy && f.client.Configured(), ws.testFeedback),
		ws.feedbackAction(cx, "feedback-retry", i18n.T("ui.feedback.retry"), !f.busy && f.client.Configured(), ws.retryFeedback),
		ws.feedbackAction(cx, "feedback-clear", i18n.T("ui.feedback.clear"), !f.busy, ws.clearFeedback),
		ws.feedbackAction(cx, "feedback-send", i18n.T("ui.feedback.send"), !f.busy && f.client.Configured(), ws.sendFeedback)))
	card := gpui.NewDiv().ID("feedback-dialog").FlexCol().SizeW(510).MaxW(max(0, ws.winW-24)).MaxH(max(0, ws.winH-24)).Rounded(10).Bg(theme.Panel).Border(1, theme.Border).Child(
		gpui.NewDiv().FlexRow().ItemsCenter().JustifyBetween().Px_(18).Py_(12).FlexShrink(0).Child(gpui.NewText(i18n.T("ui.feedback.heading")).TextSize(18).FontWeight(700), ws.feedbackAction(cx, "feedback-close", i18n.T("ui.feedback.close"), true, ws.closeFeedback)), gpui.NewDiv().FlexCol().P(18).Flex1().MinH(0).OverflowYScroll().ShowScrollbar().TrackScroll(f.scroll).Child(body), footer)
	root.Child(gpui.NewDiv().Absolute().Inset(0).FlexCol().ItemsCenter().JustifyCenter().Bg(theme.Backdrop).Child(card))
	return root
}

func (ws *Workspace) feedbackPoint(p gpui.Point) feedback.Point {
	f := ws.feedbackState()
	b := f.imageBounds
	img := f.editor.Image()
	return feedback.Point{X: float64(p.X-b.Origin.X) / math.Max(1, float64(b.Size.W)) * float64(img.Bounds().Dx()), Y: float64(p.Y-b.Origin.Y) / math.Max(1, float64(b.Size.H)) * float64(img.Bounds().Dy())}
}

func (ws *Workspace) renderFeedbackEditor(root *gpui.Div, w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	f := ws.feedbackState()
	theme := feedbackTheme()
	root.Bg(theme.Bg).TextColor(theme.Text)
	done := func(save bool, w *gpui.Window) {
		if save {
			f.shot = f.editor.Snapshot()
			f.revision++
		}
		f.editing = false
		f.dragging = false
		f.editor = nil
		w.Refresh()
	}
	toolbar := gpui.NewDiv().ID("feedback-editor-toolbar").FlexRow().FlexWrap().ItemsCenter().Gap(6).P(10).FlexShrink(0).Bg(theme.Panel).Border(1, theme.Border).Child(
		ws.feedbackAction(cx, "feedback-edit-cancel", i18n.T("ui.feedback.cancel"), true, func(w *gpui.Window) { done(false, w) }),
		ws.feedbackAction(cx, "feedback-pen", i18n.T("ui.feedback.pen"), true, func(w *gpui.Window) { f.erase = false; w.Refresh() }),
		ws.feedbackAction(cx, "feedback-eraser", i18n.T("ui.feedback.eraser"), true, func(w *gpui.Window) { f.erase = true; w.Refresh() }),
		ws.feedbackAction(cx, "feedback-undo", i18n.T("ui.feedback.undo"), f.editor.CanUndo(), func(w *gpui.Window) { f.editor.Undo(); w.Refresh() }),
		gpui.NewText(fmt.Sprintf("%s · %.0f px", map[bool]string{false: i18n.T("ui.feedback.pen"), true: i18n.T("ui.feedback.eraser")}[f.erase], f.width)),
		newSliderWithTheme("feedback-width", &f.slider, SliderOptions{Value: f.width, Min: 1, Max: 50, Step: 1, OnChange: func(v float64, w *gpui.Window) { f.width = v; w.Refresh() }}, theme).SizeW(100))
	palette := []color.NRGBA{{0, 0, 0, 255}, {255, 255, 255, 255}, {235, 45, 65, 255}, {255, 125, 25, 255}, {245, 210, 40, 255}, {40, 180, 90, 255}, {20, 200, 210, 255}, {35, 100, 235, 255}, {160, 70, 225, 255}}
	for i, c := range palette {
		chip := ws.feedbackAction(cx, fmt.Sprintf("feedback-color-%d", i), " ", true, func(w *gpui.Window) { f.ink = c; f.erase = false; w.Refresh() })
		chip.SizeW(24).Bg(gpui.Rgba{R: c.R, G: c.G, B: c.B, A: 255})
		if f.ink == c && !f.erase {
			chip.Border(2, theme.Accent)
		}
		toolbar.Child(chip)
	}
	toolbar.Child(ws.feedbackAction(cx, "feedback-edit-save", i18n.T("ui.feedback.save"), true, func(w *gpui.Window) { done(true, w) }))
	screenshot := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		f.imageBounds = feedbackFit(b, f.editor.Image())
		w.PaintImage(f.imageBounds, f.editor.Image())
	})
	area := gpui.NewDiv().ID("feedback-editor-image").FlexCol().Flex1().MinH(0).Child(screenshot).OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
		if e.IsLeft() && f.imageBounds.Contains(e.Position) {
			f.dragging = true
			f.editor.Begin(ws.feedbackPoint(e.Position), f.width, f.ink, f.erase)
			w.RequestFocus(f.focus)
			w.Refresh()
		}
		w.StopPropagation()
	})
	root.OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
		f.slider.Move(e.Position, w)
		if f.dragging {
			f.editor.Move(ws.feedbackPoint(e.Position))
			w.Refresh()
		}
		w.StopPropagation()
	}).OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
		f.slider.End(w)
		f.dragging = false
		f.editor.End()
		w.Refresh()
		w.StopPropagation()
	})
	return root.Child(toolbar, area)
}
