package ui

import (
	"fmt"
	"strconv"

	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/plot"
)

// Plot dialog geometry in logical pixels.
const (
	plotWidth   = 780
	plotPreview = 300
	plotLabelW  = 110
)

// withPlot lays the plot dialog over the window while it is open.
func (ws *Workspace) withPlot(w *gpui.Window, cx *gpui.App, inner gpui.IntoElement) gpui.IntoElement {
	if ws.plt == nil || !ws.plt.open {
		return inner
	}
	return gpui.NewDiv().Relative().FlexCol().SizeFull().Child(inner, ws.plotOverlay(w, cx))
}

// plotChoice is one button of a row of mutually exclusive options; a disabled one is muted and
// does nothing.
func plotChoice(id, label, tip string, on, enabled bool, f func(*gpui.Window)) *gpui.Div {
	b := gpui.NewDiv().ID(id).Px_(12).Py_(5).Rounded(4).Border(1, uiBorder).Tooltip(tip)
	switch {
	case !enabled:
		return b.Bg(uiPanel).Child(gpui.NewText(label).TextSize(fontSmall).TextColor(uiMuted))
	case on:
		b.Bg(uiAccent).Child(gpui.NewText(label).TextSize(fontSmall).FontWeight(700))
	default:
		b.Bg(uiPanelAlt).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			Child(gpui.NewText(label).TextSize(fontSmall))
	}
	return b.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w); w.StopPropagation() })
}

// plotRow is a labelled row of the options column.
func plotRow(label string, controls ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(6).Child(
		gpui.NewText(label).TextSize(fontSmall).TextColor(uiMuted),
		gpui.NewDiv().FlexRow().FlexWrap().Gap(4).Child(controls...))
}

// mm formats a length in millimetres without trailing zeros.
func mm(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// plotButton is a footer button; primary is the filled one.
func plotButton(id, label, tip string, primary, enabled bool, f func(*gpui.Window)) *gpui.Div {
	b := gpui.NewDiv().ID(id).Px_(16).Py_(8).Rounded(5).Border(1, uiBorder).Tooltip(tip)
	switch {
	case !enabled:
		return b.Bg(uiPanel).Child(gpui.NewText(label).TextColor(uiMuted))
	case primary:
		b.Bg(uiPrimary).TextColor(uiOnPrimary).FontWeight(700)
	default:
		b.Bg(uiPanelAlt).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) })
	}
	return b.Child(gpui.NewText(label)).OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w); w.StopPropagation() })
}

// plotOverlay is the dimmed backdrop and the dialog card: what and where to plot on the left; the
// preview, the plot style, the printer and the copies on the right; Cancel, Save PDF, Open in PDF
// viewer and Print under them. Escape is Cancel and Enter is Print.
func (ws *Workspace) plotOverlay(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	p := ws.plotState()
	if p.focus == nil {
		p.focus = cx.NewFocusHandle()
	}
	if p.focusWanted {
		p.focusWanted = false
		w.RequestFocus(p.focus)
	}
	ws.refreshPlot(float64(plotPreview * w.ScaleFactor() * w.UIScale()))
	o := p.opt
	set := func(f func(o *plot.Options)) func(*gpui.Window) { return func(w *gpui.Window) { ws.setPlot(w, f) } }

	nsel := len(o.Selection)
	selTip := i18n.T("ui.plot.what_selection_tip")
	if nsel == 0 {
		selTip = i18n.T("ui.plot.what_selection_none_tip")
	}
	what := plotRow(i18n.T("ui.plot.what"),
		plotChoice("plot-what-all", i18n.T("ui.plot.what_all"), i18n.T("ui.plot.what_all_tip"), o.What == plot.WhatAll, true,
			set(func(o *plot.Options) { o.What = plot.WhatAll })),
		plotChoice("plot-what-selection", i18n.T("ui.plot.what_selection", nsel), selTip, o.What == plot.WhatSelection, nsel > 0,
			set(func(o *plot.Options) { o.What = plot.WhatSelection })))

	hasWindow := !p.window.IsEmpty()
	area := plotRow(i18n.T("ui.plot.area"),
		plotChoice("plot-area-extents", i18n.T("ui.plot.area_extents"), i18n.T("ui.plot.area_extents_tip"), o.Area == plot.AreaExtents, true,
			set(func(o *plot.Options) { o.Area = plot.AreaExtents })),
		plotChoice("plot-area-display", i18n.T("ui.plot.area_display"), i18n.T("ui.plot.area_display_tip"), o.Area == plot.AreaDisplay, true,
			set(func(o *plot.Options) { o.Area = plot.AreaDisplay })),
		plotChoice("plot-area-window", i18n.T("ui.plot.area_window"), i18n.T("ui.plot.area_window_tip"), o.Area == plot.AreaWindow, true,
			func(w *gpui.Window) {
				if hasWindow {
					ws.setPlot(w, func(o *plot.Options) { o.Area = plot.AreaWindow })
				} else {
					ws.pickPlotWindow(w)
				}
			}),
		plotChoice("plot-pick", i18n.T("ui.plot.pick"), i18n.T("ui.plot.pick_tip"), false, !p.busy, ws.pickPlotWindow))

	var papers []gpui.IntoElement
	for _, pp := range plot.Papers {
		papers = append(papers, plotChoice("plot-paper-"+pp.Name, pp.Name, i18n.T("ui.plot.paper_tip", pp.Name, mm(pp.Width), mm(pp.Height)),
			o.Paper.Name == pp.Name, true, set(func(o *plot.Options) { o.Paper = pp })))
	}
	orient := plotRow(i18n.T("ui.plot.orientation"),
		plotChoice("plot-portrait", i18n.T("ui.plot.portrait"), i18n.T("ui.plot.portrait_tip"), !o.Landscape, true,
			set(func(o *plot.Options) { o.Landscape = false })),
		plotChoice("plot-landscape", i18n.T("ui.plot.landscape"), i18n.T("ui.plot.landscape_tip"), o.Landscape, true,
			set(func(o *plot.Options) { o.Landscape = true })))

	scales := []gpui.IntoElement{plotChoice("plot-scale-fit", i18n.T("ui.plot.scale_fit"), i18n.T("ui.plot.scale_fit_tip"), !o.Scale.Fixed(), true,
		func(w *gpui.Window) { ws.setScale(w, plot.Fit) })}
	for _, sc := range plot.Scales {
		scales = append(scales, plotChoice("plot-scale-"+sc.String(), sc.String(), i18n.T("ui.plot.scale_tip", sc.String()), o.Scale == sc, true,
			func(w *gpui.Window) { ws.setScale(w, sc) }))
	}
	scales = append(scales, gpui.NewDiv().ID("plot-scale-custom").SizeW(90).Tooltip(i18n.T("ui.plot.scale_custom_tip")).Child(p.scaleIn.Render(w, cx)))
	scale := plotRow(i18n.T("ui.plot.scale_label"), scales...)

	offField := func(id, label string, in *gpui.TextInput) *gpui.Div {
		row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(4).Child(gpui.NewText(label).TextSize(fontSmall).TextColor(uiMuted))
		if !o.NoCenter {
			return row.Child(gpui.NewText(i18n.T("ui.plot.offset_centered")).TextSize(fontSmall).TextColor(uiMuted))
		}
		return row.Child(gpui.NewDiv().ID(id).SizeW(70).Child(in.Render(w, cx)), gpui.NewText("mm").TextSize(fontSmall).TextColor(uiMuted))
	}
	place := plotRow(i18n.T("ui.plot.placement"),
		plotChoice("plot-center", i18n.T("ui.plot.center"), i18n.T("ui.plot.center_tip"), !o.NoCenter, true,
			set(func(o *plot.Options) { o.NoCenter = false })),
		plotChoice("plot-offset", i18n.T("ui.plot.offset"), i18n.T("ui.plot.offset_tip"), o.NoCenter, true,
			set(func(o *plot.Options) { o.NoCenter = true })),
		offField("plot-offset-x", "X", p.offX), offField("plot-offset-y", "Y", p.offY))

	style := plotRow(i18n.T("ui.plot.ink"),
		plotChoice("plot-color", i18n.T("ui.plot.color"), i18n.T("ui.plot.color_tip"), o.Style == plot.StyleColor, true,
			set(func(o *plot.Options) { o.Style = plot.StyleColor })),
		plotChoice("plot-gray", i18n.T("ui.plot.gray"), i18n.T("ui.plot.gray_tip"), o.Style == plot.StyleGray, true,
			set(func(o *plot.Options) { o.Style = plot.StyleGray })),
		plotChoice("plot-mono", i18n.T("ui.plot.mono"), i18n.T("ui.plot.mono_tip"), o.Style == plot.StyleMono, true,
			set(func(o *plot.Options) { o.Style = plot.StyleMono })),
		plotChoice("plot-lineweights", i18n.T("ui.plot.lineweights"), i18n.T("ui.plot.lineweights_tip"), !o.NoLineweights, true,
			set(func(o *plot.Options) { o.NoLineweights = !o.NoLineweights })))

	printers := []gpui.IntoElement{}
	switch {
	case !canPrint() || p.noPrinting:
		printers = append(printers, gpui.NewText(i18n.T("ui.plot.no_printers")).TextSize(fontSmall).TextColor(uiMuted))
	default:
		def := i18n.T("ui.plot.printer_default")
		for _, q := range p.printers {
			if q.Default {
				def = i18n.T("ui.plot.printer_default_named", q.Name)
			}
		}
		printers = append(printers, plotChoice("plot-printer-default", def, i18n.T("ui.plot.printer_default_tip"), p.printer == "", !p.busy,
			func(w *gpui.Window) { ws.setJob(w, "", p.copies) }))
		for i, q := range p.printers {
			if q.Default && p.printer != q.Name {
				continue // the Default button stands for it
			}
			printers = append(printers, plotChoice(fmt.Sprintf("plot-printer-%d", i), q.Name, i18n.T("ui.plot.printer_tip", q.Name), p.printer == q.Name, !p.busy,
				func(w *gpui.Window) { ws.setJob(w, q.Name, p.copies) }))
		}
		if p.listing {
			printers = append(printers, gpui.NewText(i18n.T("ui.plot.printers_looking")).TextSize(fontSmall).TextColor(uiMuted))
		}
	}
	copies := plotRow(i18n.T("ui.plot.copies"),
		plotChoice("plot-copies-less", "−", i18n.T("ui.plot.copies_less_tip"), false, !p.busy && p.copies > 1,
			func(w *gpui.Window) { ws.setJob(w, p.printer, p.copies-1) }),
		gpui.NewDiv().ID("plot-copies").SizeW(32).FlexRow().ItemsCenter().JustifyCenter().Child(gpui.NewText(strconv.Itoa(p.copies)).FontWeight(700)),
		plotChoice("plot-copies-more", "+", i18n.T("ui.plot.copies_more_tip"), false, !p.busy && p.copies < plot.MaxCopies,
			func(w *gpui.Window) { ws.setJob(w, p.printer, p.copies+1) }))

	options := gpui.NewDiv().ID("plot-options").FlexCol().Gap(14).Flex1().MinW(260).Child(
		what, area, plotRow(i18n.T("ui.plot.paper"), papers...), orient, scale, place)

	img, version := p.preview, p.version
	paper := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		if img != nil {
			w.PaintImageVersion(feedbackFit(b, img), img, version)
		}
	})
	pw, ph := o.Paper.Size(o.Landscape)
	orientation := i18n.T("ui.plot.portrait")
	if o.Landscape {
		orientation = i18n.T("ui.plot.landscape")
	}
	info := gpui.NewDiv().FlexCol().Gap(2).Child(
		gpui.NewText(i18n.T("ui.plot.sheet", o.Paper.Name, orientation, mm(pw), mm(ph))).TextSize(fontSmall))
	switch {
	case p.err != nil:
		info.Child(gpui.NewText(i18n.T("ui.plot.nothing")).TextSize(fontSmall).TextColor(uiErr))
	case p.sheet != nil:
		units := strconv.FormatFloat(1/p.sheet.Scale, 'g', 4, 64)
		if o.Scale.Fixed() {
			info.Child(gpui.NewText(i18n.T("ui.plot.scale_fixed", o.Scale.String(), units)).TextSize(fontSmall).TextColor(uiMuted))
		} else {
			info.Child(gpui.NewText(i18n.T("ui.plot.scale", units)).TextSize(fontSmall).TextColor(uiMuted))
		}
		if p.sheet.Overflow {
			info.Child(gpui.NewText(i18n.T("ui.plot.overflow")).TextSize(fontSmall).TextColor(uiErr))
		}
	}
	preview := gpui.NewDiv().FlexCol().Gap(14).FlexShrink(0).SizeW(plotPreview).Child(
		gpui.NewDiv().ID("plot-preview").FlexCol().SizeWH(plotPreview, plotPreview).P(10).Rounded(6).Bg(uiBg).Border(1, uiBorder).Child(paper),
		info, style, plotRow(i18n.T("ui.plot.printer"), printers...), copies)

	footer := gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Child(gpui.NewDiv().Flex1().MinW(0).Child(
		gpui.NewText(p.message).TextSize(fontSmall).TextColor(map[bool]gpui.Rgba{true: uiMuted, false: uiErr}[p.busy])))
	footer.Child(
		plotButton("plot-cancel", i18n.T("ui.plot.cancel"), i18n.T("ui.plot.cancel_tip"), false, !p.busy, ws.closePlot),
		plotButton("plot-save", i18n.T("ui.plot.save"), i18n.T("ui.plot.save_tip"), false, !p.busy && p.sheet != nil && ws.dlg.dialog != nil, ws.savePlot),
		plotButton("plot-view", i18n.T("ui.plot.view"), i18n.T("ui.plot.view_tip"), false, !p.busy && p.sheet != nil && canPrint(), ws.viewPlot),
		plotButton("plot-print", i18n.T("ui.plot.print"), i18n.T("ui.plot.print_tip"), true, !p.busy && p.sheet != nil && canPrint() && !p.noPrinting, ws.printPlot))

	// The heading and the buttons keep their place; only the options scroll in a short window.
	body := gpui.NewDiv().ID("plot-body").FlexRow().FlexWrap().Gap(20).Px_(20).Py_(4).Flex1().MinH(0).
		OverflowYScroll().ShowScrollbar().TrackScroll(p.scroll).Child(options, preview)
	card := gpui.NewDiv().ID("plot-dialog").TrackFocus(p.focus).FlexCol().Gap(12).Py_(20).SizeW(plotWidth).
		MaxW(max(0, float32(w.Bounds().Size.W)-32)).MaxH(max(0, float32(w.Bounds().Size.H)-32)).
		Bg(uiPanel).Border(1, uiBorder).Rounded(10).ShadowXl().TextColor(uiText).TextSize(fontUI).
		Child(
			gpui.NewDiv().Px_(20).FlexShrink(0).Child(gpui.NewText(i18n.T("ui.plot.heading")).FontWeight(700).TextSize(16)),
			body,
			footer.Px_(20).FlexShrink(0))
	// Bindings resolve before raw key events; consume the drawing's actions on the dialog's path.
	for _, b := range Bindings() {
		card.OnAction(b.Action, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	}
	card.OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
		switch e.Keystroke.Key {
		case "escape":
			if !p.busy {
				ws.closePlot(w)
			}
		case "enter":
			// A text field keeps its own Enter, so this is Enter on the dialog itself.
			ws.printPlot(w)
		}
		w.StopPropagation()
	})
	card.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() })
	return gpui.NewDiv().ID("plot-overlay").Absolute().Inset(0).FlexCol().ItemsCenter().JustifyCenter().Bg(uiBackdrop).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnFileDrop(func(_ *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(e *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) {
			// The wheel scrolls the dialog's body wherever the pointer is, and never the canvas.
			off := p.scroll.Offset()
			off.Y = min(max(off.Y+e.DeltaY, 0), p.scroll.MaxOffset().H)
			if off != p.scroll.Offset() {
				p.scroll.SetOffset(off)
				w.Refresh()
			}
			w.StopPropagation()
		}).
		Child(card)
}
