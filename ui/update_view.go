package ui

import (
	"strings"
	"unicode/utf8"

	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
	"cad-lite/updater"
)

// Release notes excerpt limits.
const (
	notesMaxLines = 7
	notesMaxWidth = 90
)

// notesExcerpt turns a release body (Markdown) into a few plain lines:
// headings, bullets and emphasis marks are stripped, blank lines dropped.
func notesExcerpt(body string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			continue // headings repeat the section title
		}
		l = strings.TrimLeft(l, ">")
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* ") {
			l = "• " + strings.TrimSpace(l[2:])
		}
		l = strings.NewReplacer("**", "", "__", "", "`", "").Replace(l)
		if l == "" {
			continue
		}
		if utf8.RuneCountInString(l) > notesMaxWidth {
			r := []rune(l)
			l = strings.TrimSpace(string(r[:notesMaxWidth-1])) + "…"
		}
		out = append(out, l)
		if len(out) == notesMaxLines {
			break
		}
	}
	return out
}

// updateStatus is the status line's text and colour.
func (ws *Workspace) updateStatus() (string, gpui.Rgba) {
	u := ws.upd()
	dev := !updater.Updatable(u.version)
	switch u.phase {
	case updChecking:
		return i18n.T("ui.update.status_checking"), uiMuted
	case updAvailable:
		if u.err != nil {
			return i18n.T("ui.update.status_error", u.err.Error()), uiErr
		}
		return i18n.T("ui.update.status_available", u.latest.Version), uiOn
	case updDownloading:
		if t := u.size.Load(); t > 0 && u.done.Load() >= t {
			return i18n.T("ui.update.status_installing", u.latest.Version), uiOn
		}
		if t := u.size.Load(); t > 0 {
			return i18n.T("ui.update.status_downloading", u.latest.Version, int(u.done.Load()*100/t)), uiOn
		}
		return i18n.T("ui.update.status_downloading_unknown", u.latest.Version), uiOn
	case updInstalling:
		return i18n.T("ui.update.status_installing", u.latest.Version), uiOn
	case updReady:
		return i18n.T("ui.update.status_ready", u.latest.Version), uiOn
	case updError:
		return i18n.T("ui.update.status_error", u.err.Error()), uiErr
	case updUpToDate:
		if dev {
			if u.latest.IsZero() {
				return i18n.T("ui.update.status_dev_none"), uiMuted
			}
			return i18n.T("ui.update.status_dev", u.latest.Version), uiMuted
		}
		return i18n.T("ui.update.status_current"), uiMuted
	}
	if dev {
		return i18n.T("ui.update.status_dev_none"), uiMuted
	}
	return i18n.T("ui.update.status_idle"), uiMuted
}

// actionButton is a text button; a disabled one is dimmed and inert.
func actionButton(id, label, tip string, primary, enabled bool, f func(*gpui.Window, *gpui.App)) *gpui.Div {
	b := gpui.NewDiv().ID(id).Tooltip(tip).FlexRow().ItemsCenter().JustifyCenter().SizeH(settingsBtnSize).Px_(12).
		FlexShrink(0).Rounded(4).Border(1, uiBorder)
	txt := gpui.NewText(label).TextSize(12)
	switch {
	case !enabled:
		return b.Bg(uiPanelAlt).Child(txt.TextColor(uiMuted))
	case primary:
		b.Bg(uiAccent).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, uiOn) })
		txt.FontWeight(700)
	default:
		b.Bg(uiPanelAlt).Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) })
	}
	return b.OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) { f(w, cx) }).Child(txt)
}

// updateSection is the "About and updates" block of the settings panel.
func (ws *Workspace) updateSection() *gpui.Div {
	u := ws.upd()
	p := ws.prefs()
	ver := u.version
	if ver == "dev" {
		ver = i18n.T("ui.update.version_dev", ver)
	}
	sec := gpui.NewDiv().FlexCol().Gap(settingsRowGap).Child(
		sectionTitle(i18n.T("ui.update.section")),
		settingRow(i18n.T("ui.update.version"), i18n.T("ui.update.version_tip"), gpui.NewText(ver).FontWeight(700)),
	)
	if !updater.Supported() || !u.enabled {
		msg := i18n.T("ui.update.status_unsupported")
		if updater.Supported() && updater.Disabled() {
			msg = i18n.T("ui.update.status_disabled")
		}
		if !updater.Supported() || updater.Disabled() {
			return sec.Child(gpui.NewText(msg).TextSize(11).TextColor(uiMuted))
		}
		return sec
	}
	last := i18n.T("ui.update.never")
	if t := p.cur.LastUpdateCheck; !t.IsZero() {
		last = t.Local().Format("2006-01-02 15:04")
	}
	status, col := ws.updateStatus()
	buttons := gpui.NewDiv().FlexRow().FlexWrap().Gap(8).Child(
		actionButton("update-check", i18n.T("ui.update.check"), i18n.T("ui.update.check_tip"), false, !u.busy() && u.phase != updReady,
			func(w *gpui.Window, _ *gpui.App) { ws.checkForUpdate(w, true) }),
	)
	if u.phase == updAvailable && !u.latest.IsZero() {
		buttons.Child(actionButton("update-install", i18n.T("ui.update.update", u.latest.Version), i18n.T("ui.update.update_tip", u.latest.Version), true, true,
			func(w *gpui.Window, _ *gpui.App) { ws.startUpdate(w) }))
	}
	if u.err != nil && u.latest.PageURL != "" {
		buttons.Child(actionButton("update-page", i18n.T("ui.update.open_release"), i18n.T("ui.update.open_release_tip"), false, true,
			func(w *gpui.Window, _ *gpui.App) { ws.openReleasePage(); w.Refresh() }))
	}
	sec.Child(
		settingRow(i18n.T("ui.update.last_check"), i18n.T("ui.update.last_check_tip"), gpui.NewText(last)),
		settingRow(i18n.T("ui.update.auto"), i18n.T("ui.update.auto_tip"), toggle("set-autoupdate", i18n.T("ui.update.auto_tip"), p.cur.AutoUpdate, func(w *gpui.Window) {
			ws.change(w, func(s *settings.Settings) { s.AutoUpdate = !s.AutoUpdate })
		})),
		gpui.NewText(status).TextColor(col).FontWeight(700),
		buttons,
	)
	if u.phase == updAvailable {
		if lines := notesExcerpt(u.latest.Notes); len(lines) > 0 {
			notes := gpui.NewDiv().FlexCol().Gap(2).P(8).Rounded(4).Bg(uiPanelAlt).Child(sectionTitle(i18n.T("ui.update.notes")))
			for _, l := range lines {
				notes.Child(gpui.NewText(l).TextSize(11))
			}
			sec.Child(notes)
		}
		sec.Child(gpui.NewText(i18n.T("ui.update.restart_hint")).TextSize(11).TextColor(uiMuted))
	}
	return sec
}

// updateBadge is the dot on the toolbar's gear button while a release waits.
func (ws *Workspace) updateBadge() gpui.IntoElement {
	return gpui.NewDiv().ID("update-badge").Absolute().Top(2).Right(4).SizeWH(9, 9).Rounded(5).Bg(uiWarn).Border(1, uiPanel)
}

// settingsTip is the gear button's tooltip, with the update notice appended.
func (ws *Workspace) settingsTip() string {
	tip := i18n.T("ui.toolbar.settings_tip")
	if rel, ok := ws.UpdateAvailable(); ok {
		tip += "\n" + i18n.T("ui.update.badge_tip", rel.Version)
	}
	return tip
}

// updateOverlay is the restart confirmation: nothing unless an install
// finished while the drawing has edits.
func (ws *Workspace) updateOverlay() gpui.IntoElement {
	u := ws.upd()
	if !u.confirm {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	card := gpui.NewDiv().ID("update-confirm").FlexCol().Gap(12).P(20).SizeW(420).
		Bg(uiPanel).Border(1, uiBorder).Rounded(10).ShadowXl().TextColor(uiText).TextSize(13).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(
			gpui.NewText(i18n.T("ui.update.restart_title")).FontWeight(700).TextSize(16),
			gpui.NewText(i18n.T("ui.update.restart_body", u.latest.Version)),
			gpui.NewDiv().FlexRow().Gap(8).JustifyEnd().Child(
				actionButton("update-restart-later", i18n.T("ui.update.restart_later"), i18n.T("ui.update.restart_later_tip"), false, true,
					func(w *gpui.Window, _ *gpui.App) { u.confirm = false; w.Refresh() }),
				actionButton("update-restart-now", i18n.T("ui.update.restart_now"), i18n.T("ui.update.restart_now_tip"), true, true,
					func(w *gpui.Window, cx *gpui.App) { ws.restartNow(w, cx) }),
			),
		)
	return gpui.NewDiv().ID("update-overlay").Absolute().Inset(0).FlexRow().ItemsCenter().JustifyCenter().Bg(uiBackdrop).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(card)
}
