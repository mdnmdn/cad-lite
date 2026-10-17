package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/agentinstall"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/settings"
)

// fakeInstaller is an AgentInstaller that reports progress, then waits for release (or for its
// context) and returns r, err.
type fakeInstaller struct {
	r       agentinstall.Result
	err     error
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func (f *fakeInstaller) run(ctx context.Context, progress func(agentinstall.Progress)) (agentinstall.Result, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	progress(agentinstall.Progress{Step: agentinstall.StepMise, Done: 21 << 20, Total: 48 << 20})
	select {
	case <-f.release:
	case <-ctx.Done():
		return agentinstall.Result{}, ctx.Err()
	}
	progress(agentinstall.Progress{Step: agentinstall.StepAuth})
	return f.r, f.err
}

func (f *fakeInstaller) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// terminalLog records what the terminal opener was asked to run.
type terminalLog struct {
	mu   sync.Mutex
	argv [][]string
}

func (l *terminalLog) open(_ string, argv []string) (string, error) {
	l.mu.Lock()
	l.argv = append(l.argv, argv)
	l.mu.Unlock()
	return "xterm", nil
}

func (l *terminalLog) runs() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.argv)
}

// installRig is a tall settings panel on a PATH without any agent, a fake installer and a fake
// terminal; the tools folder is a temporary one.
func installRig(t *testing.T, f *fakeInstaller) (*sHarness, *terminalLog) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CADLITE_"+agentinstall.EnvDir, filepath.Join(t.TempDir(), "tools"))
	shellenv.Set("")
	t.Cleanup(func() {
		for _, h := range harnesses.All {
			harnesses.SetBinary(h, "")
		}
	})
	ws := NewWorkspace(doc.New(), nil)
	s := ws.Settings()
	s.AgentMode = settings.AgentAdvanced
	ws.SetSettings(s, nil)
	ws.SetAgentInstaller(f.run)
	term := &terminalLog{}
	ws.SetTerminalOpener(term.open)
	h := openSettingsWS(t, ws, 900, 2400, 1)
	h.openPanel()
	return h, term
}

func TestInstallAsksBeforeInstalling(t *testing.T) {
	f := &fakeInstaller{release: make(chan struct{})}
	h, _ := installRig(t, f)
	if !h.has("settings-install-ask") {
		t.Fatal("no Install OpenCode offer while no agent is found")
	}
	h.clickID("settings-install-ask")
	h.hw.Pixels()
	if !h.has("settings-install-go") || f.count() != 0 {
		t.Fatalf("the offer must ask first: confirm shown %v, installer ran %d times", h.has("settings-install-go"), f.count())
	}
	h.clickID("settings-install-no")
	h.hw.Pixels()
	if h.has("settings-install-go") || f.count() != 0 || h.ws.inst().phase != instIdle {
		t.Fatal("Not now must close the question without installing")
	}
}

func TestInstallStoresTheBinaryAndOpensTheSignIn(t *testing.T) {
	bin := writeExe(t, t.TempDir(), "opencode", `echo "opencode 1.2.3"`)
	f := &fakeInstaller{release: make(chan struct{}), r: agentinstall.Result{Binary: bin}}
	h, term := installRig(t, f)
	h.clickID("settings-install-ask")
	h.hw.Pixels()
	h.clickID("settings-install-go")
	h.settle(func() bool { return h.ws.inst().done.Load() > 0 })
	h.hw.Pixels()
	if !h.ws.AgentInstalling() || !h.has("settings-install-cancel") {
		t.Fatal("no progress while installing")
	}
	h.png("install-progress.png")
	close(f.release)
	h.settle(func() bool { return !h.ws.AgentInstalling() })
	h.hw.Pixels()
	if got := h.ws.Settings().AgentBinaries["opencode"]; got != bin {
		t.Errorf("agent.binaries.opencode = %q, want %q", got, bin)
	}
	if harnesses.Binary(agentmanager.OpenCode) != bin {
		t.Errorf("the harness does not start the installed OpenCode: %q", harnesses.Binary(agentmanager.OpenCode))
	}
	runs := term.runs()
	if len(runs) != 1 || strings.Join(runs[0], " ") != bin+" auth login" {
		t.Fatalf("sign-in terminal runs %v", runs)
	}
	if a := h.ws.inst(); a.phase != instDone || a.login != "xterm" || a.loginErr {
		t.Errorf("result not shown: %+v", a.phase)
	}
	h.png("install-done.png")
	if h.has("settings-install-ask") || h.has("settings-install-login") {
		t.Fatal("installer remains visible after OpenCode is found")
	}
	h.ws.openLogin(h.hw.Window())
	if len(term.runs()) != 2 {
		t.Error("Sign in did not open the terminal again")
	}
	h.ws.dismissInstall(h.hw.Window())
	h.hw.Pixels()
	if h.has("settings-install") {
		t.Error("the card stayed after OK although OpenCode is now found")
	}
}

func TestInstallSignedInOpensNoTerminal(t *testing.T) {
	bin := writeExe(t, t.TempDir(), "opencode", `echo "opencode 1.2.3"`)
	f := &fakeInstaller{release: make(chan struct{}), r: agentinstall.Result{Binary: bin, Authenticated: true}}
	close(f.release)
	h, term := installRig(t, f)
	h.ws.startInstall(h.hw.Window())
	h.settle(func() bool { return !h.ws.AgentInstalling() })
	if len(term.runs()) != 0 {
		t.Errorf("a terminal opened although OpenCode is signed in: %v", term.runs())
	}
}

func TestInstallCancel(t *testing.T) {
	f := &fakeInstaller{release: make(chan struct{})}
	h, term := installRig(t, f)
	h.ws.startInstall(h.hw.Window())
	h.settle(func() bool { return h.ws.inst().done.Load() > 0 })
	h.clickID("settings-install-cancel")
	h.settle(func() bool { return !h.ws.AgentInstalling() })
	h.hw.Pixels()
	a := h.ws.inst()
	if a.phase != instIdle || !a.cancelled || h.ws.Settings().AgentBinaries != nil || len(term.runs()) != 0 {
		t.Fatalf("cancel left phase %v cancelled %v binaries %v", a.phase, a.cancelled, h.ws.Settings().AgentBinaries)
	}
	if !h.has("settings-install-ask") {
		t.Error("the offer is not back after cancelling")
	}
}

func TestQuitCancelsARunningInstall(t *testing.T) {
	cleaned := make(chan struct{})
	h, _ := installRig(t, &fakeInstaller{})
	h.ws.SetAgentInstaller(func(ctx context.Context, progress func(agentinstall.Progress)) (agentinstall.Result, error) {
		progress(agentinstall.Progress{Step: agentinstall.StepTool})
		<-ctx.Done()
		close(cleaned) // the real installer kills mise's process group and removes partial folders here
		return agentinstall.Result{}, ctx.Err()
	})
	h.ws.startInstall(h.hw.Window())
	h.settle(func() bool { return h.ws.inst().step.Load() == int32(agentinstall.StepTool) })
	h.ws.CloseAgent()
	select {
	case <-cleaned:
	default:
		t.Fatal("CloseAgent returned before the installation was cancelled and cleaned up")
	}
	select {
	case <-h.ws.inst().finished:
	default:
		t.Fatal("CloseAgent did not wait for the installer to return")
	}
}

func TestInstallFailureShowsTheErrorAndRetries(t *testing.T) {
	f := &fakeInstaller{release: make(chan struct{}), err: errors.New("agentinstall: checksum mismatch")}
	close(f.release)
	h, _ := installRig(t, f)
	h.ws.startInstall(h.hw.Window())
	h.settle(func() bool { return !h.ws.AgentInstalling() })
	h.hw.Pixels()
	if a := h.ws.inst(); a.phase != instFailed || !strings.Contains(a.err, "checksum mismatch") {
		t.Fatalf("failure not shown: %v %q", a.phase, a.err)
	}
	h.clickID("settings-install-retry")
	h.settle(func() bool { return f.count() == 2 && !h.ws.AgentInstalling() })
	h.clickID("settings-install-close")
	h.hw.Pixels()
	if h.ws.inst().phase != instIdle {
		t.Error("Close did not dismiss the error")
	}
}

func TestInstallOfferInAgentPanel(t *testing.T) {
	f := &fakeInstaller{release: make(chan struct{})}
	h, _ := installRig(t, f)
	h.ws.CloseSettings(h.hw.Window())
	h.ws.AgentPanel().Show()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	if !h.has("agent-install-ask") {
		t.Fatal("the agent panel offers no install when no agent is found")
	}
	h.clickID("agent-install-ask")
	h.hw.Pixels()
	if !h.has("agent-install-go") {
		t.Fatal("the panel's offer does not ask first")
	}
}

// Renders the install card in the agent panel (question and progress) and in the settings panel,
// in both languages, into _tmp/settings (a look-at-it test).
func TestRenderInstallCard(t *testing.T) {
	for _, lang := range []string{"en", "it"} {
		t.Run(lang, func(t *testing.T) {
			t.Cleanup(func() { i18n.Set(i18n.English) })
			i18n.Set(i18n.FromSetting(lang))
			f := &fakeInstaller{release: make(chan struct{})}
			h, _ := installRig(t, f)
			h.ws.CloseSettings(h.hw.Window())
			h.ws.AgentPanel().Show()
			h.ws.askInstall(h.hw.Window())
			h.hw.Pixels()
			h.png("install-panel-confirm-" + lang + ".png")
			h.ws.startInstall(h.hw.Window())
			h.settle(func() bool { return h.ws.inst().done.Load() > 0 })
			h.hw.Pixels()
			h.png("install-panel-progress-" + lang + ".png")
			h.ws.OpenSettings(h.hw.Window())
			h.hw.Pixels()
			h.png("install-settings-progress-" + lang + ".png")
			close(f.release)
			h.settle(func() bool { return !h.ws.AgentInstalling() })
		})
	}
}

func TestTerminalCommandRunsAProgram(t *testing.T) {
	dir := t.TempDir()
	only := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			if slices.Contains(names, n) {
				return "/bin/" + n, nil
			}
			return "", os.ErrNotExist
		}
	}
	argv := []string{"/opt/it's/opencode", "auth", "login"}
	cmd, name, err := terminalCommandArgs("darwin", dir, "", argv, only())
	if err != nil || name != "Terminal" || cmd.Args[0] != "osascript" ||
		!strings.Contains(strings.Join(cmd.Args, " "), `do script "cd '`+dir+`' && '/opt/it'\\''s/opencode' 'auth' 'login'"`) {
		t.Errorf("darwin: %q %q %v", cmd.Args, name, err)
	}
	cmd, name, err = terminalCommandArgs("windows", dir, "", argv, only("powershell"))
	if err != nil || name != "PowerShell" || strings.Join(cmd.Args[1:4], " ") != "-NoProfile -NoExit -EncodedCommand" || cmd.Dir != dir {
		t.Errorf("windows: %q %q %v", cmd.Args, name, err)
	}
	cmd, name, err = terminalCommandArgs("windows", dir, "", argv, only())
	if err != nil || name != "cmd" || cmd.Args[1] != "/k" {
		t.Errorf("windows without PowerShell: %q %q %v", cmd.Args, name, err)
	}
	tail := "sh -c " + keepOpenScript + " " + i18n.T("ui.agent.install.terminal_close") + " " + strings.Join(argv, " ")
	cmd, name, err = terminalCommandArgs("linux", dir, "", argv, only("gnome-terminal"))
	if err != nil || name != "gnome-terminal" || strings.Join(cmd.Args[1:], " ") != "--working-directory="+dir+" -- "+tail {
		t.Errorf("linux gnome: %q %q %v", cmd.Args, name, err)
	}
	cmd, name, err = terminalCommandArgs("linux", dir, "", argv, only("xterm"))
	if err != nil || name != "xterm" || strings.Join(cmd.Args[1:], " ") != "-e "+tail {
		t.Errorf("linux xterm: %q %q %v", cmd.Args, name, err)
	}
	cmd, _, err = terminalCommandArgs("linux", dir, "alacritty -e", argv, only("alacritty"))
	if err != nil || strings.Join(cmd.Args[1:], " ") != "-e "+tail {
		t.Errorf("linux $TERMINAL with -e: %q %v", cmd.Args, err)
	}
}

func TestBasicInstallKeepsSignInInSettings(t *testing.T) {
	bin := writeExe(t, t.TempDir(), "opencode", `echo "opencode 1.2.3"`)
	f := &fakeInstaller{release: make(chan struct{}), r: agentinstall.Result{Binary: bin}}
	h, term := installRig(t, f)
	h.clickID("ai-mode-basic")
	h.clickID("settings-basic-install-ask")
	if f.count() != 0 {
		t.Fatal("installation started before confirmation")
	}
	h.hw.Pixels()
	h.clickID("settings-basic-install-go")
	close(f.release)
	h.settle(func() bool { return !h.ws.AgentInstalling() })
	if h.ws.Settings().AgentBinaries["opencode"] != bin {
		t.Fatal("installed executable was not saved")
	}
	if len(term.runs()) != 0 {
		t.Fatal("Basic mode opened an interactive sign-in terminal")
	}
}
