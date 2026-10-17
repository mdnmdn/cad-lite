package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/filedialog"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// binaryGOOS is the platform the picker's file filters and messages are for; tests change it.
var binaryGOOS = runtime.GOOS

// binaryPicker is the state of the AI agent section's executable picker. picking, errs, vers and
// poll belong to the UI goroutine; the dialog goroutine writes only res, under mu.
type binaryPicker struct {
	// picking is the harness whose dialog or check is running; empty when none is.
	picking agentmanager.Harness
	// errs is the last refusal per harness: why the chosen file cannot be used.
	errs map[agentmanager.Harness]string
	// vers is what the last accepted file printed for --version.
	vers map[agentmanager.Harness]string
	poll *gpui.Timer

	mu  sync.Mutex
	res *binaryPick
}

// binaryPick is one finished Browse: the chosen file and what checking it found.
type binaryPick struct {
	h       agentmanager.Harness
	path    string
	version string
	// dialogErr is the dialog's failure (cancelled, unavailable); checkErr the file's.
	dialogErr, checkErr error
}

// applyAgentBinaries hands the executables chosen in the settings to agentmanager, where they win
// over the PATH lookup for discovery and for every session start, and lets the agent panel offer a
// harness found only that way.
func (ws *Workspace) applyAgentBinaries(m map[string]string) {
	ws.prefs().aiBasic.availableChecked = false
	for _, h := range harnesses.All {
		harnesses.SetBinary(h, m[string(h)])
	}
	ws.agent.reprobe()
}

// browseAgentBinary is a harness's Browse button: it shows the open dialog on a goroutine, checks
// the chosen file there too (its --version call can take seconds), and the poll adopts the answer.
func (ws *Workspace) browseAgentBinary(w *gpui.Window, h agentmanager.Harness) {
	b := &ws.disc().bin
	if b.picking != "" || ws.dlg.busy {
		return
	}
	dialog := ws.dlg.dialog
	if dialog == nil {
		b.setErr(h, i18n.T("ui.settings.agent_bin_err_dialog"))
		w.Refresh()
		return
	}
	b.picking = h
	ws.dlg.busy = true
	title := i18n.T("ui.settings.agent_bin_dialog_title", harnessName(h))
	dir, filters := ws.agentBinaryDir(h), agentBinaryFilters(binaryGOOS)
	go func() {
		ctx := context.Background()
		r := binaryPick{h: h}
		r.path, r.dialogErr = dialog.Open(ctx, title, dir, filters)
		if r.dialogErr == nil {
			r.version, r.checkErr = harnesses.Check(ctx, r.path)
		}
		b.mu.Lock()
		b.res = &r
		b.mu.Unlock()
	}()
	ws.armBinaryPoll(w)
	w.Refresh()
}

func (ws *Workspace) armBinaryPoll(w *gpui.Window) {
	b := &ws.disc().bin
	b.poll.Cancel()
	b.poll = w.After(dialogPoll, func(w *gpui.Window, _ *gpui.App) { ws.pollBinary(w) })
}

// pollBinary is the poll timer body, on the UI goroutine: it stores an accepted file in the
// settings, or the reason it was refused.
func (ws *Workspace) pollBinary(w *gpui.Window) {
	b := &ws.disc().bin
	b.mu.Lock()
	r := b.res
	b.res = nil
	b.mu.Unlock()
	if r == nil {
		ws.armBinaryPoll(w)
		return
	}
	b.picking, ws.dlg.busy = "", false
	switch {
	case errors.Is(r.dialogErr, filedialog.ErrCancelled):
	case r.dialogErr != nil:
		b.setErr(r.h, i18n.T("ui.settings.agent_bin_err_dialog"))
	case r.checkErr != nil:
		b.setErr(r.h, binaryCheckMessage(r.path, r.checkErr))
	default:
		b.setErr(r.h, "")
		if b.vers == nil {
			b.vers = map[agentmanager.Harness]string{}
		}
		b.vers[r.h] = r.version
		ws.setAgentBinary(w, r.h, r.path)
	}
	w.Refresh()
}

// resetAgentBinary is a harness's Reset button: back to the PATH lookup.
func (ws *Workspace) resetAgentBinary(w *gpui.Window, h agentmanager.Harness) {
	b := &ws.disc().bin
	b.setErr(h, "")
	delete(b.vers, h)
	ws.setAgentBinary(w, h, "")
}

// setAgentBinary stores path as h's executable ("" forgets it) and, when the agents were already
// looked for, looks again so the discovery rows show what is used now.
func (ws *Workspace) setAgentBinary(w *gpui.Window, h agentmanager.Harness, path string) {
	ws.change(w, func(s *settings.Settings) {
		if s.AgentBinaries == nil {
			s.AgentBinaries = map[string]string{}
		}
		if path == "" {
			delete(s.AgentBinaries, string(h))
		} else {
			s.AgentBinaries[string(h)] = path
		}
	})
	if d := ws.disc(); d.last != nil {
		ws.checkAgents(w)
	}
}

func (b *binaryPicker) setErr(h agentmanager.Harness, msg string) {
	if msg == "" {
		delete(b.errs, h)
		return
	}
	if b.errs == nil {
		b.errs = map[agentmanager.Harness]string{}
	}
	b.errs[h] = msg
}

// binaryCheckMessage says why path cannot be an agent's executable.
func binaryCheckMessage(path string, err error) string {
	switch {
	case errors.Is(err, harnesses.ErrNotExist):
		return i18n.T("ui.settings.agent_bin_err_missing", path)
	case errors.Is(err, harnesses.ErrIsDir):
		return i18n.T("ui.settings.agent_bin_err_dir", path)
	case binaryGOOS == "windows":
		return i18n.T("ui.settings.agent_bin_err_exec_windows", path)
	}
	return i18n.T("ui.settings.agent_bin_err_exec", path)
}

// agentBinaryFilters is the Browse dialog's file-type list: programs first on Windows, where an
// agent installed with npm is a .cmd; every file elsewhere, where executables have no extension.
func agentBinaryFilters(goos string) []filedialog.Filter {
	all := filedialog.Filter{Name: i18n.T("ui.file.filter_all")}
	if goos == "windows" {
		return []filedialog.Filter{{Name: i18n.T("ui.settings.agent_bin_filter"), Extensions: []string{"exe", "cmd", "bat", "com"}}, all}
	}
	return []filedialog.Filter{all}
}

// agentBinaryDir is where the Browse dialog starts: the folder of the executable in use (chosen
// or found on the PATH), else the home folder.
func (ws *Workspace) agentBinaryDir(h agentmanager.Harness) string {
	if p := ws.prefs().cur.AgentBinaries[string(h)]; p != "" {
		return filepath.Dir(p)
	}
	if p, ok := shellenv.Resolve(string(h)); ok {
		return filepath.Dir(p)
	}
	home, _ := os.UserHomeDir()
	return home
}

// harnessName is a harness's display name.
func harnessName(h agentmanager.Harness) string {
	for _, a := range discoveryAgents {
		if a.h == h {
			return a.name
		}
	}
	return string(h)
}

// agentBinaryRows is the executables block of the AI agent section: per harness the executable
// in use (chosen, found on the PATH, or none), Browse and, for a chosen one, Reset.
func (ws *Workspace) agentBinaryRows() *gpui.Div {
	box := gpui.NewDiv().ID("agent-bins").FlexCol().Gap(settingsRowGap).Child(
		gpui.NewText(i18n.T("ui.settings.agent_bins")).FontWeight(700).TextSize(12),
		gpui.NewText(i18n.T("ui.settings.agent_bins_hint")).TextSize(11).TextColor(uiMuted),
	)
	for _, a := range discoveryAgents {
		box.Child(ws.agentBinaryRow(a.h, a.name))
	}
	return box
}

func (ws *Workspace) agentBinaryRow(h agentmanager.Harness, name string) *gpui.Div {
	b := &ws.disc().bin
	tip := i18n.T("ui.settings.agent_bin_tip", name)
	val := gpui.NewDiv().ID("agent-bin-" + string(h)).FlexCol().Gap(1).Flex1().MinW(0).Tooltip(tip)
	for i, l := range ws.agentBinaryLines(h) {
		t := gpui.NewText(l.text).TextSize(11).TextColor(uiMuted)
		switch {
		case l.bad:
			t.TextColor(uiErr)
		case i == 0 && l.path:
			t.TextSize(12).TextColor(uiText)
		}
		val.Child(t)
	}
	if msg := b.errs[h]; msg != "" {
		val.Child(gpui.NewText(msg).TextSize(11).TextColor(uiErr))
	}
	browse := i18n.T("ui.settings.agent_bin_browse")
	if b.picking == h {
		browse = i18n.T("ui.settings.agent_bin_browsing")
	}
	btns := gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).FlexShrink(0).Child(
		actionButton("agent-bin-browse-"+string(h), browse, i18n.T("ui.settings.agent_bin_browse_tip"), false, b.picking == "",
			func(w *gpui.Window, _ *gpui.App) { ws.browseAgentBinary(w, h) }))
	if ws.prefs().cur.AgentBinaries[string(h)] != "" {
		btns.Child(actionButton("agent-bin-reset-"+string(h), i18n.T("ui.settings.agent_bin_reset"), i18n.T("ui.settings.agent_bin_reset_tip"), false, true,
			func(w *gpui.Window, _ *gpui.App) { ws.resetAgentBinary(w, h) }))
	}
	return settingRow(name, tip, gpui.NewDiv().FlexRow().ItemsStart().Gap(8).Flex1().MinW(0).Child(val, btns)).ItemsStart()
}

// binaryLine is one line of an executable row; path marks the executable's path, bad a problem.
type binaryLine struct {
	text      string
	path, bad bool
}

// agentBinaryLines is the text of h's executable row: the chosen file and whether it is still
// there, else the one on the PATH, else "not found".
func (ws *Workspace) agentBinaryLines(h agentmanager.Harness) []binaryLine {
	if p := ws.prefs().cur.AgentBinaries[string(h)]; p != "" {
		if _, err := os.Stat(p); err != nil {
			return []binaryLine{{text: p, path: true}, {text: i18n.T("ui.settings.agent_bin_gone"), bad: true}}
		}
		if v := ws.disc().bin.vers[h]; v != "" {
			return []binaryLine{{text: p, path: true}, {text: i18n.T("ui.settings.agent_bin_chosen_version", v)}}
		}
		return []binaryLine{{text: p, path: true}, {text: i18n.T("ui.settings.agent_bin_chosen")}}
	}
	if f, ok := ws.disc().found(h); ok && f.Path != "" && !f.Override {
		if f.Version != "" {
			return []binaryLine{{text: f.Path, path: true}, {text: i18n.T("ui.settings.agent_bin_on_path_version", f.Version)}}
		}
		return []binaryLine{{text: f.Path, path: true}, {text: i18n.T("ui.settings.agent_bin_on_path")}}
	}
	if p, ok := shellenv.Resolve(string(h)); ok {
		return []binaryLine{{text: p, path: true}, {text: i18n.T("ui.settings.agent_bin_on_path")}}
	}
	return []binaryLine{{text: i18n.T("ui.settings.agent_bin_not_found")}}
}
