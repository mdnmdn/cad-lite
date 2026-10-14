package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/doc"
	"cad-lite/filedialog"
	"cad-lite/i18n"
	"cad-lite/settings"
)

// binRig is a tall settings panel (every row on screen) with a fake dialog, a PATH that holds
// only a fake opencode, and the overrides cleared afterwards.
func binRig(t *testing.T, fd *fakeDialog) (*sHarness, string) { return binRigSized(t, fd, 900, 2400) }

func binRigSized(t *testing.T, fd *fakeDialog, width, height float32) (*sHarness, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	pathDir := t.TempDir()
	writeExe(t, pathDir, "opencode", `echo "opencode 1.0"`)
	t.Setenv("PATH", pathDir)
	shellenv.Set("")
	t.Cleanup(func() {
		for _, h := range harnesses.All {
			harnesses.SetBinary(h, "")
		}
	})
	ws := NewWorkspace(doc.New(), nil)
	if fd != nil {
		ws.SetFileDialog(fd)
	}
	h := openSettingsWS(t, ws, width, height, 1)
	h.openPanel()
	return h, pathDir
}

func writeExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// pickDone waits for the Browse goroutine's answer to reach the UI.
func (h *sHarness) pickDone(fd *fakeDialog) {
	h.t.Helper()
	h.settle(func() bool { return fd.calls() > 0 && h.ws.disc().bin.picking == "" })
}

func lineTexts(l []binaryLine) string {
	var s []string
	for _, x := range l {
		s = append(s, x.text)
	}
	return strings.Join(s, " | ")
}

func TestBrowseChoosesAnAgentExecutable(t *testing.T) {
	bin := writeExe(t, t.TempDir(), "claude-nightly", `echo "2.5.0 (Claude Code)"`)
	fd := &fakeDialog{path: bin}
	h, _ := binRig(t, fd)
	ws := h.ws
	if got := lineTexts(ws.agentBinaryLines(agentmanager.Claude)); got != i18n.T("ui.settings.agent_bin_not_found") {
		t.Fatalf("claude before: %q", got)
	}
	if h.has("agent-bin-reset-claude") {
		t.Fatal("Reset shows only for a chosen executable")
	}
	h.clickID("agent-bin-browse-claude")
	h.pickDone(fd)

	if fd.title != i18n.T("ui.settings.agent_bin_dialog_title", "Claude") {
		t.Errorf("dialog title %q", fd.title)
	}
	if home, _ := os.UserHomeDir(); fd.startDir != home {
		t.Errorf("start dir %q, want the home folder when nothing is found", fd.startDir)
	}
	if got := ws.Settings().AgentBinaries["claude"]; got != bin {
		t.Fatalf("setting = %q, want %q", got, bin)
	}
	if got := harnesses.Binary(agentmanager.Claude); got != bin {
		t.Fatalf("agentmanager override = %q", got)
	}
	if p, ok := harnesses.Available(agentmanager.Claude); !ok || p != bin {
		t.Errorf("Available = %q %v", p, ok)
	}
	want := bin + " | " + i18n.T("ui.settings.agent_bin_chosen_version", "2.5.0 (Claude Code)")
	if got := lineTexts(ws.agentBinaryLines(agentmanager.Claude)); got != want {
		t.Errorf("row %q, want %q", got, want)
	}
	h.hw.Pixels()
	if !h.has("agent-bin-reset-claude") {
		t.Fatal("Reset missing after a choice")
	}

	// A second Browse starts where the chosen file is.
	h.clickID("agent-bin-browse-claude")
	h.settle(func() bool { return fd.calls() > 1 && ws.disc().bin.picking == "" })
	if fd.startDir != filepath.Dir(bin) {
		t.Errorf("start dir %q, want %q", fd.startDir, filepath.Dir(bin))
	}

	h.clickID("agent-bin-reset-claude")
	h.hw.Pixels()
	if _, ok := ws.Settings().AgentBinaries["claude"]; ok || harnesses.Binary(agentmanager.Claude) != "" {
		t.Fatalf("Reset kept the override: %v %q", ws.Settings().AgentBinaries, harnesses.Binary(agentmanager.Claude))
	}
	if h.has("agent-bin-reset-claude") {
		t.Error("Reset still shown")
	}
}

func TestBrowseRefusesAFileThatCannotRun(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path, key string
	}{
		{notes, "ui.settings.agent_bin_err_exec"},
		{dir, "ui.settings.agent_bin_err_dir"},
		{filepath.Join(dir, "gone"), "ui.settings.agent_bin_err_missing"},
	} {
		fd := &fakeDialog{path: c.path}
		h, _ := binRig(t, fd)
		h.clickID("agent-bin-browse-codex")
		h.pickDone(fd)
		if got, want := h.ws.disc().bin.errs[agentmanager.Codex], i18n.T(c.key, c.path); got != want {
			t.Errorf("%s: error %q, want %q", c.path, got, want)
		}
		if s := h.ws.Settings(); s.AgentBinaries != nil || harnesses.Binary(agentmanager.Codex) != "" {
			t.Errorf("%s: a refused file was stored: %v", c.path, s.AgentBinaries)
		}
	}
}

func TestBrowseCancelledOrWithoutDialog(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h, _ := binRig(t, fd)
	h.clickID("agent-bin-browse-opencode")
	h.pickDone(fd)
	if b := &h.ws.disc().bin; len(b.errs) != 0 || h.ws.Settings().AgentBinaries != nil || h.ws.dlg.busy {
		t.Errorf("cancel changed something: %+v %v", b.errs, h.ws.Settings().AgentBinaries)
	}

	fd = &fakeDialog{err: filedialog.ErrUnavailable}
	h, _ = binRig(t, fd)
	h.clickID("agent-bin-browse-opencode")
	h.pickDone(fd)
	if got := h.ws.disc().bin.errs[agentmanager.OpenCode]; got != i18n.T("ui.settings.agent_bin_err_dialog") {
		t.Errorf("no backend: %q", got)
	}

	h, _ = binRig(t, nil)
	h.clickID("agent-bin-browse-opencode")
	if got := h.ws.disc().bin.errs[agentmanager.OpenCode]; got != i18n.T("ui.settings.agent_bin_err_dialog") {
		t.Errorf("no dialog installed: %q", got)
	}
}

func TestAgentBinaryRowShowsThePATHOne(t *testing.T) {
	fd := &fakeDialog{err: filedialog.ErrCancelled}
	h, pathDir := binRig(t, fd)
	oc := filepath.Join(pathDir, "opencode")
	if got, want := lineTexts(h.ws.agentBinaryLines(agentmanager.OpenCode)), oc+" | "+i18n.T("ui.settings.agent_bin_on_path"); got != want {
		t.Errorf("row %q, want %q", got, want)
	}
	h.clickID("agent-bin-browse-opencode")
	h.pickDone(fd)
	if fd.startDir != pathDir {
		t.Errorf("start dir %q, want the PATH one's folder %q", fd.startDir, pathDir)
	}
}

func TestAgentBinariesLoadFromTheSettingsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	t.Cleanup(func() { harnesses.SetBinary(agentmanager.Codex, "") })
	bin := writeExe(t, t.TempDir(), "codex", `echo codex`)
	path := filepath.Join(t.TempDir(), "settings.toml")
	s := settings.Default()
	s.AgentBinaries = map[string]string{"codex": bin}
	if err := settings.Save(path, s); err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	if got := harnesses.Binary(agentmanager.Codex); got != bin {
		t.Fatalf("override after load = %q", got)
	}
	if got := lineTexts(ws.agentBinaryLines(agentmanager.Codex)); got != bin+" | "+i18n.T("ui.settings.agent_bin_chosen") {
		t.Errorf("row %q", got)
	}
	os.Remove(bin)
	if l := ws.agentBinaryLines(agentmanager.Codex); len(l) != 2 || !l[1].bad {
		t.Errorf("a vanished file must be flagged: %+v", l)
	}
	ws.SetSettings(settings.Default(), nil)
	if harnesses.Binary(agentmanager.Codex) != "" {
		t.Error("defaults must clear the override")
	}
}

func TestAgentBinaryFilters(t *testing.T) {
	if f := agentBinaryFilters("linux"); len(f) != 1 || len(f[0].Extensions) != 0 {
		t.Errorf("linux: %+v", f)
	}
	f := agentBinaryFilters("windows")
	if len(f) != 2 || strings.Join(f[0].Extensions, ",") != "exe,cmd,bat,com" || len(f[1].Extensions) != 0 {
		t.Errorf("windows: %+v", f)
	}
	defer func(g string) { binaryGOOS = g }(binaryGOOS)
	binaryGOOS = "windows"
	if got := binaryCheckMessage(`C:\x\claude.ps1`, harnesses.ErrNotExecutable); got != i18n.T("ui.settings.agent_bin_err_exec_windows", `C:\x\claude.ps1`) {
		t.Errorf("windows message %q", got)
	}
}

func TestDiscoveryRowMarksTheOverride(t *testing.T) {
	got := discoveryRowLines(harnesses.Found{Harness: agentmanager.Claude, Path: "/x/claude", Version: "1.0", Override: true})
	if len(got) != 3 || got[2] != i18n.T("ui.settings.discovery_override") {
		t.Errorf("%q", got)
	}
}

// Renders the AI agent section with one chosen, one found and one refused executable, into
// _tmp/settings/agent-binaries.png (a look-at-it test, never fails on pixels).
func TestRenderAgentBinaryRows(t *testing.T) {
	dir, err := os.MkdirTemp("", "cl") // short, so the row shows a path the way a user's looks
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	bin := writeExe(t, dir, "claude", `echo "2.5.0 (Claude Code)"`)
	fd := &fakeDialog{path: bin}
	h, _ := binRigSized(t, fd, 1000, 1250)
	h.clickID("agent-bin-browse-claude")
	h.pickDone(fd)
	h.ws.disc().bin.setErr(agentmanager.Codex, i18n.T("ui.settings.agent_bin_err_exec", "/home/u/Downloads/codex.tar.gz"))
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.png("agent-binaries.png")
}
