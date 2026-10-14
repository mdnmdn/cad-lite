package ui

import (
	"strings"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// discoveryAgents are the agents the section lists, with their display names.
var discoveryAgents = []struct {
	h    agentmanager.Harness
	name string
}{
	{agentmanager.OpenCode, "OpenCode"},
	{agentmanager.Claude, "Claude"},
	{agentmanager.Codex, "Codex"},
}

// pathTipMax caps how many folders a PATH tooltip lists.
const pathTipMax = 12

// discoverySection is the "AGENT DISCOVERY" block of the settings panel:
// which agents were found, where, and the buttons that look again or open a
// terminal.
func (ws *Workspace) discoverySection() *gpui.Div {
	d := ws.disc()
	sec := gpui.NewDiv().ID("settings-sec-discovery").FlexCol().Gap(settingsRowGap).Child(
		sectionTitle(i18n.T("ui.settings.section_discovery")),
		gpui.NewText(i18n.T("ui.settings.discovery_hint")).TextSize(11).TextColor(uiMuted),
	)
	switch {
	case d.last != nil:
		for _, a := range discoveryAgents {
			sec.Child(ws.discoveryRow(a.h, a.name))
		}
		sec.Child(ws.discoveryPathLine(d.last))
		if d.showPath && d.last.Err == nil {
			sec.Child(discoveryPathList(d.last))
		}
	case d.running:
		sec.Child(gpui.NewText(i18n.T("ui.settings.discovery_checking")).TextSize(12).TextColor(uiMuted))
	default:
		sec.Child(gpui.NewText(i18n.T("ui.settings.discovery_not_checked")).TextSize(12).TextColor(uiMuted))
	}

	checkLabel := i18n.T("ui.settings.discovery_check")
	if d.running {
		checkLabel = i18n.T("ui.settings.discovery_checking")
	}
	buttons := gpui.NewDiv().FlexRow().FlexWrap().ItemsCenter().Gap(8).Child(
		actionButton("agents-check", checkLabel, i18n.T("ui.settings.discovery_check_tip"), true, !d.running,
			func(w *gpui.Window, _ *gpui.App) { ws.checkAgents(w) }),
		actionButton("agents-terminal", i18n.T("ui.settings.discovery_terminal"), i18n.T("ui.settings.discovery_terminal_tip"), false, true,
			func(w *gpui.Window, _ *gpui.App) { ws.openTerminal(w) }),
	)
	sec.Child(buttons)
	if ws.installWanted(true) {
		sec.Child(ws.agentInstallBlock("settings"))
	}
	if d.term != "" {
		if d.termErr {
			sec.Child(gpui.NewText(i18n.T("ui.settings.discovery_terminal_failed", d.term)).TextSize(11).TextColor(uiErr))
		} else {
			sec.Child(gpui.NewText(i18n.T("ui.settings.discovery_terminal_opened", d.term)).TextSize(11).TextColor(uiMuted))
		}
	}
	return sec
}

// discoveryRow is one agent: its name, and the full path and version found,
// or "not found".
func (ws *Workspace) discoveryRow(h agentmanager.Harness, name string) *gpui.Div {
	f, _ := ws.disc().found(h)
	tip := i18n.T("ui.settings.discovery_row_tip", name)
	val := gpui.NewDiv().ID("agent-found-" + string(h)).FlexCol().Gap(1).Flex1().MinW(0).Tooltip(tip)
	lines := discoveryRowLines(f)
	if f.Path == "" {
		val.Child(gpui.NewText(lines[0]).TextColor(uiMuted))
	} else {
		val.Child(gpui.NewText(lines[0]).TextSize(12))
		for _, l := range lines[1:] {
			val.Child(gpui.NewText(l).TextSize(11).TextColor(uiMuted))
		}
	}
	return settingRow(name, tip, val).ItemsStart()
}

// discoveryRowLines is the text of an agent's row: its full path, when it
// answered its version, and whether the path is the one chosen in the AI
// agent section; or "not found".
func discoveryRowLines(f harnesses.Found) []string {
	if f.Path == "" {
		return []string{i18n.T("ui.settings.discovery_not_found")}
	}
	l := []string{f.Path}
	if f.Version != "" {
		l = append(l, f.Version)
	}
	if f.Override {
		l = append(l, i18n.T("ui.settings.discovery_override"))
	}
	return l
}

// discoveryPathText says where the PATH came from.
func discoveryPathText(r *AgentDiscovery) string {
	switch {
	case r.Err != nil:
		return i18n.T("ui.settings.discovery_path_app", r.Err.Error())
	case r.Source == shellenv.SourceRegistry && len(r.Added) > 0:
		return i18n.T("ui.settings.discovery_path_registry", len(r.Added))
	case r.Source == shellenv.SourceRegistry:
		return i18n.T("ui.settings.discovery_path_registry_same")
	case len(r.Added) > 0:
		return i18n.T("ui.settings.discovery_path_login", len(r.Added))
	}
	return i18n.T("ui.settings.discovery_path_login_same")
}

// discoveryPathLine says where the PATH came from, with the expander that
// lists the folders the login shell added.
func (ws *Workspace) discoveryPathLine(r *AgentDiscovery) *gpui.Div {
	d := ws.disc()
	text := discoveryPathText(r)
	line := gpui.NewDiv().FlexRow().ItemsStart().Gap(8).Child(
		gpui.NewDiv().Flex1().MinW(0).Tooltip(discoveryPathTip(r)).Child(gpui.NewText(text).TextSize(11).TextColor(uiMuted)),
	)
	if r.Err == nil {
		label := i18n.T("ui.settings.discovery_show_path")
		if d.showPath {
			label = i18n.T("ui.settings.discovery_hide_path")
		}
		line.Child(gpui.NewDiv().ID("agents-show-path").Tooltip(i18n.T("ui.settings.discovery_show_path_tip")).FlexShrink(0).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).Rounded(3).Px_(4).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { d.showPath = !d.showPath; w.Refresh() }).
			Child(gpui.NewText(label).TextSize(11).TextColor(uiOn)))
	}
	return line
}

// discoveryPathTip lists the folders the login shell added, for the PATH
// line's tooltip.
func discoveryPathTip(r *AgentDiscovery) string {
	tip := i18n.T("ui.settings.discovery_path_tip")
	if r.Err != nil || len(r.Added) == 0 {
		return tip
	}
	return tip + "\n" + strings.Join(clipList(r.Added, pathTipMax), "\n")
}

// clipList returns at most n entries of l, the last one saying how many more.
func clipList(l []string, n int) []string {
	if len(l) <= n {
		return l
	}
	return append(append([]string(nil), l[:n]...), i18n.T("ui.settings.discovery_path_more", len(l)-n))
}

// discoveryPathList is the expanded PATH: the folders the login shell added.
func discoveryPathList(r *AgentDiscovery) *gpui.Div {
	box := gpui.NewDiv().ID("agents-path-list").FlexCol().Gap(2).P(8).Rounded(4).Bg(uiPanelAlt).
		Child(sectionTitle(i18n.T("ui.settings.discovery_path_added")))
	if len(r.Added) == 0 {
		return box.Child(gpui.NewText(i18n.T("ui.settings.discovery_path_none")).TextSize(11))
	}
	for _, e := range r.Added {
		box.Child(gpui.NewText(e).TextSize(11))
	}
	return box
}
