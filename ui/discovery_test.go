package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// fakeProbe is a discovery probe that returns r after release is closed.
func fakeProbe(r AgentDiscovery, release chan struct{}) AgentDiscoveryProbe {
	return func(context.Context) AgentDiscovery {
		if release != nil {
			<-release
		}
		return r
	}
}

func sampleDiscovery() AgentDiscovery {
	return AgentDiscovery{
		Source: shellenv.SourceLoginShell, AppPATH: "/usr/bin", LoginPATH: "/opt/homebrew/bin:/home/u/.bun/bin:/usr/bin",
		Added: []string{"/opt/homebrew/bin", "/home/u/.bun/bin"},
		Agents: []harnesses.Found{
			{Harness: agentmanager.OpenCode},
			{Harness: agentmanager.Claude, Path: "/home/u/.local/bin/claude", Version: "2.1.0 (Claude Code)"},
			{Harness: agentmanager.Codex, Path: "/opt/homebrew/bin/codex", Version: "codex-cli 0.9.0"},
		},
	}
}

// has reports whether the element with the given id is in the last frame.
func (h *sHarness) has(id string) bool {
	w := h.hw.Window()
	b := w.Bounds()
	for y := float32(0); y < float32(b.Size.H); y += 2 {
		for x := float32(0); x < float32(b.Size.W); x += 2 {
			gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
			if ok && len(gid.Path) > 0 && gid.Path[len(gid.Path)-1].Key == id {
				return true
			}
		}
	}
	return false
}

func TestSettingsBodyScrollsAndFooterStaysPut(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := openSettingsWS(t, ws, 900, 600, 1)
	h.openPanel()
	sc := ws.prefs().bodyScroll
	if sc.MaxOffset().H <= 0 {
		t.Fatalf("the body must overflow a 600 px window, max offset %v", sc.MaxOffset())
	}
	resetTop, closeTop := h.find("settings-reset"), h.find("settings-close")
	if float32(resetTop.Y) > 600 || float32(closeTop.Y) < 0 {
		t.Fatalf("title bar and footer must be on screen: close %v reset %v", closeTop, resetTop)
	}
	scale0 := ws.View().Scale
	body := h.find("settings-body")
	for i := 0; i < 40 && sc.Offset().Y < sc.MaxOffset().H; i++ {
		h.hw.Dispatch(gpui.ScrollWheelEvent{Position: body, DeltaY: 60})
		h.hw.Pixels()
	}
	if sc.Offset().Y <= 0 || sc.Offset().Y != sc.MaxOffset().H {
		t.Fatalf("wheel did not scroll the body to its end: offset %v of %v", sc.Offset(), sc.MaxOffset())
	}
	if ws.View().Scale != scale0 {
		t.Fatalf("the wheel zoomed the canvas behind the panel: %v -> %v", scale0, ws.View().Scale)
	}
	if got := h.find("settings-reset"); got != resetTop {
		t.Errorf("footer moved while scrolling: %v -> %v", resetTop, got)
	}
	if got := h.find("settings-close"); got != closeTop {
		t.Errorf("title bar moved while scrolling: %v -> %v", closeTop, got)
	}
	// More wheel at the end must still not reach the canvas.
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: body, DeltaY: 60})
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: 20, Y: 300}, DeltaY: 60, Precise: true}) // on the backdrop
	if ws.View().Scale != scale0 {
		t.Fatalf("wheel leaked to the canvas: %v -> %v", scale0, ws.View().Scale)
	}
	h.png("panel-900x600-bottom.png")
	h.clickID("settings-reset")
	if ws.Settings().Theme != "dark" {
		t.Errorf("Reset at the bottom of a short window did not work")
	}
}

func TestSettingsPanelClampsToWindowHeight(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 400, 1)
	h.openPanel()
	b := h.hw.Window().Bounds()
	found := false
	for _, q := range h.hw.Window().Scene().Quads {
		if q.BorderColor == CurrentTheme().Border && q.Bounds.Size.W >= settingsWidth-1 && q.Bounds.Size.W <= settingsWidth+1 {
			found = true
			if q.Bounds.Size.H > b.Size.H-2*settingsMargin+1 {
				t.Errorf("card %v taller than window %v minus margins", q.Bounds.Size.H, b.Size.H)
			}
		}
	}
	if !found {
		t.Fatal("card quad not found")
	}
	h.clickID("settings-reset")
}

func TestSettingsHasTwoAgentSections(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2400, 1)
	h.openPanel()
	for _, id := range []string{"settings-sec-agent", "agent-claude", "settings-sec-discovery", "agents-check", "agents-terminal"} {
		if !h.has(id) {
			t.Errorf("%s missing", id)
		}
	}
	if h.has("agent-found-claude") {
		t.Error("rows must wait for the first check")
	}
	if got := i18n.T("ui.settings.section_agent"); got != "AI AGENT" {
		t.Errorf("section title %q", got)
	}
	if got := i18n.T("ui.settings.section_discovery"); got != "AGENT DISCOVERY" {
		t.Errorf("section title %q", got)
	}
}

func TestCheckButtonShowsFoundAndNotFoundRows(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	release := make(chan struct{})
	ws.SetAgentDiscoveryProbe(fakeProbe(sampleDiscovery(), release))
	h := openSettingsWS(t, ws, 900, 2400, 1)
	h.openPanel()
	h.clickID("agents-check")
	h.hw.Pixels()
	if !ws.disc().running || h.has("agent-found-claude") {
		t.Fatal("the probe should be running and no rows shown yet")
	}
	h.clickID("agents-check") // a second click while running must not start another probe
	close(release)
	h.settle(func() bool { _, ok := ws.AgentDiscovery(); return ok })
	for _, id := range []string{"agent-found-opencode", "agent-found-claude", "agent-found-codex", "agents-show-path"} {
		if !h.has(id) {
			t.Errorf("%s missing after the check", id)
		}
	}
	r, _ := ws.AgentDiscovery()
	if got := discoveryRowLines(r.Agents[0]); len(got) != 1 || got[0] != "not found" {
		t.Errorf("opencode row %q", got)
	}
	if got := discoveryRowLines(r.Agents[1]); len(got) != 2 || got[0] != "/home/u/.local/bin/claude" || got[1] != "2.1.0 (Claude Code)" {
		t.Errorf("claude row %q", got)
	}
	if got := discoveryPathText(&r); got != "PATH: from your login shell, 2 more folders than this app had" {
		t.Errorf("path line %q", got)
	}
	if h.has("agents-path-list") {
		t.Error("the added folders must stay hidden until Show PATH")
	}
	h.clickID("agents-show-path")
	h.hw.Pixels()
	if !h.has("agents-path-list") {
		t.Error("Show PATH should list the folders the login shell added")
	}
	h.png("discovery-expanded-en.png")
	for _, id := range []string{"agent-found-opencode", "agent-found-claude", "agents-show-path", "agents-check", "agents-terminal"} {
		if !h.hoverTip(h.find(id)) {
			t.Errorf("%s: no tooltip", id)
		}
	}
}

func TestCheckFailureShowsAppOnlyPath(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	ws.SetAgentDiscoveryProbe(fakeProbe(AgentDiscovery{
		Source: shellenv.SourceApp, Err: errors.New("shell did not answer"),
		Agents: []harnesses.Found{{Harness: agentmanager.OpenCode}, {Harness: agentmanager.Claude}, {Harness: agentmanager.Codex}},
	}, nil))
	h := openSettingsWS(t, ws, 900, 2400, 1)
	h.openPanel()
	h.clickID("agents-check")
	h.settle(func() bool { _, ok := ws.AgentDiscovery(); return ok })
	r, _ := ws.AgentDiscovery()
	if got := discoveryPathText(&r); !strings.Contains(got, "this app's own only") || !strings.Contains(got, "shell did not answer") {
		t.Errorf("failure text %q", got)
	}
	if !h.has("agent-found-claude") || h.has("agents-show-path") {
		t.Error("rows stay, but there is nothing to expand when the probe failed")
	}
}

func TestDiscoveryResultReachesAgentPanelChoices(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Cleanup(func() { shellenv.Set("") })
	shellenv.Set("")
	p := ws.AgentPanel()
	p.probe()
	if len(p.choices) != 0 {
		t.Skip("real agents installed here")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws.SetAgentDiscoveryProbe(func(context.Context) AgentDiscovery {
		shellenv.Set(dir)
		return sampleDiscovery()
	})
	h := openSettingsWS(t, ws, 900, 1000, 1)
	h.openPanel()
	ws.EnableAgentDiscovery() // the background probe at start
	h.settle(func() bool { _, ok := ws.AgentDiscovery(); return ok })
	if len(p.choices) != 1 || p.choices[0] != agentmanager.Codex || p.selected != agentmanager.Codex {
		t.Fatalf("choices %v selected %q: Auto should pick up an agent the background probe found", p.choices, p.selected)
	}
}

func TestOpenTerminalButtonReportsOutcome(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	var gotDir string
	ws.SetTerminalOpener(func(dir string, _ []string) (string, error) { gotDir = dir; return "xterm", nil })
	h := openSettingsWS(t, ws, 900, 2400, 1)
	h.openPanel()
	h.clickID("agents-terminal")
	h.hw.Pixels()
	if gotDir != ws.projectDir() {
		t.Errorf("terminal opened in %q, want the project folder %q", gotDir, ws.projectDir())
	}
	if d := ws.disc(); d.term != "xterm" || d.termErr {
		t.Errorf("success not recorded: %q %v", d.term, d.termErr)
	}
	ws.SetTerminalOpener(func(string, []string) (string, error) { return "", errors.New("no terminal found") })
	h.clickID("agents-terminal")
	h.hw.Pixels()
	if d := ws.disc(); d.term != "no terminal found" || !d.termErr {
		t.Errorf("failure not recorded: %q %v", d.term, d.termErr)
	}
}

func TestTerminalCommandPerPlatform(t *testing.T) {
	dir := t.TempDir()
	none := func(string) (string, error) { return "", errors.New("missing") }
	only := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, o := range names {
				if o == n {
					return "/bin/" + n, nil
				}
			}
			return "", errors.New("missing")
		}
	}
	cmd, name, err := terminalCommand("darwin", dir, "", none)
	if err != nil || name != "Terminal" || strings.Join(cmd.Args, " ") != "open -a Terminal "+dir {
		t.Errorf("darwin: %v %q %v", cmd, name, err)
	}
	cmd, name, err = terminalCommand("windows", dir, "", none)
	if err != nil || name != "PowerShell" || strings.Join(cmd.Args, " ") != "cmd /c start powershell" || cmd.Dir != dir {
		t.Errorf("windows: %v %q %v", cmd, name, err)
	}
	cmd, name, err = terminalCommand("linux", dir, "", only("konsole", "xterm"))
	if err != nil || name != "konsole" || strings.Join(cmd.Args[1:], " ") != "--workdir "+dir || cmd.Dir != dir {
		t.Errorf("linux konsole: %v %q %v", cmd, name, err)
	}
	cmd, name, err = terminalCommand("linux", dir, "alacritty -e", only("alacritty", "xterm"))
	if err != nil || name != "alacritty" || strings.Join(cmd.Args[1:], " ") != "-e" {
		t.Errorf("linux $TERMINAL: %v %q %v", cmd, name, err)
	}
	cmd, name, err = terminalCommand("linux", dir, "", only("gnome-terminal"))
	if err != nil || name != "gnome-terminal" || !strings.Contains(strings.Join(cmd.Args, " "), "--working-directory="+dir) {
		t.Errorf("linux gnome: %v %q %v", cmd, name, err)
	}
	if _, _, err = terminalCommand("linux", dir, "", none); err == nil || !strings.Contains(err.Error(), "no terminal found") {
		t.Errorf("linux none: %v", err)
	}
	if _, _, err = terminalCommand("js", dir, "", none); err == nil {
		t.Error("the browser has no terminal")
	}
}

// Renders the panel at 1280x600 in both languages, at the top and scrolled
// to the bottom, into _tmp/settings (a look-at-it test, never fails on pixels).
func TestRenderSettingsPanelShortWindow(t *testing.T) {
	for _, lang := range []string{"en", "it"} {
		t.Run(lang, func(t *testing.T) {
			t.Cleanup(func() { i18n.Set(i18n.English) })
			i18n.Set(i18n.FromSetting(lang))
			ws := NewWorkspace(doc.New(), nil)
			ws.SetAgentDiscoveryProbe(fakeProbe(sampleDiscovery(), nil))
			h := openSettingsWS(t, ws, 1280, 600, 1)
			h.openPanel()
			ws.checkAgents(h.hw.Window())
			h.settle(func() bool { _, ok := ws.AgentDiscovery(); return ok })
			h.png("panel-1280x600-top-" + lang + ".png")
			ws.prefs().bodyScroll.ScrollToBottom()
			h.hw.Window().Refresh()
			h.hw.Pixels()
			h.hw.Pixels()
			h.png("panel-1280x600-bottom-" + lang + ".png")
		})
	}
}
