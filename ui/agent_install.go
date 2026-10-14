package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/agentinstall"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// installTimeout bounds a whole OpenCode installation: the mise download, `mise install` and the
// sign-in check.
const installTimeout = 30 * time.Minute

// AgentInstaller installs OpenCode into Cad-lite's private tools folder, reporting progress, and
// returns where it is and whether it is signed in. The default runs agentinstall.Installer in
// agentinstall.Dir; tests install a fake with SetAgentInstaller. It runs on a goroutine and must
// not touch the UI; a cancelled ctx makes it return ctx's error.
type AgentInstaller func(ctx context.Context, progress func(agentinstall.Progress)) (agentinstall.Result, error)

// defaultAgentInstaller installs into the tools folder with the PATH the login shell reported, so
// mise's npm backend finds node.
func defaultAgentInstaller(ctx context.Context, progress func(agentinstall.Progress)) (agentinstall.Result, error) {
	dir, err := agentinstall.Dir()
	if err != nil {
		return agentinstall.Result{}, err
	}
	in := &agentinstall.Installer{Dir: dir, Env: shellenv.Env(), Progress: progress}
	return in.Install(ctx)
}

// instPhase is where the "Install OpenCode" flow is.
type instPhase int

const (
	instIdle    instPhase = iota // the offer, or nothing when an agent is there
	instConfirm                  // asking before anything is downloaded
	instRunning                  // the installer goroutine runs
	instDone                     // installed; sign-in checked
	instFailed                   // the last attempt failed
)

// agentInstall is the state of the install flow. Everything but mu, res and the atomics belongs
// to the UI goroutine; the installer goroutine writes only those, and the poll timer moves the
// result across.
type agentInstall struct {
	run AgentInstaller

	phase      instPhase
	cancel     context.CancelFunc
	cancelling bool
	// cancelled is set when the last run was cancelled, for a note under the offer.
	cancelled bool
	err       string
	result    agentinstall.Result
	// login is the outcome of opening the sign-in terminal; loginErr is set when it failed.
	login    string
	loginErr bool
	poll     *gpui.Timer

	step        atomic.Int32
	done, total atomic.Int64

	mu  sync.Mutex
	res *instResult
}

// instResult is what the installer goroutine hands to the UI goroutine.
type instResult struct {
	r   agentinstall.Result
	err error
}

func (ws *Workspace) inst() *agentInstall {
	a := &ws.disc().inst
	if a.run == nil {
		a.run = defaultAgentInstaller
	}
	return a
}

// SetAgentInstaller replaces how OpenCode is installed (tests install a fake); nil restores the
// default.
func (ws *Workspace) SetAgentInstaller(f AgentInstaller) {
	ws.disc().inst.run = f
}

// AgentInstalling reports whether an OpenCode installation is running.
func (ws *Workspace) AgentInstalling() bool { return ws.inst().phase == instRunning }

// askInstall is the "Install OpenCode…" button: it shows what will happen and waits for consent.
func (ws *Workspace) askInstall(w *gpui.Window) {
	a := ws.inst()
	if a.phase == instRunning {
		return
	}
	a.phase, a.err, a.cancelled = instConfirm, "", false
	w.Refresh()
}

// startInstall is the confirmation's Install button (and Retry): it runs the installer on a
// goroutine and polls it.
func (ws *Workspace) startInstall(w *gpui.Window) {
	a := ws.inst()
	if a.phase == instRunning {
		return
	}
	a.phase, a.err, a.cancelled, a.cancelling = instRunning, "", false, false
	a.login, a.loginErr = "", false
	a.step.Store(int32(agentinstall.StepMise))
	a.done.Store(0)
	a.total.Store(0)
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	a.cancel = cancel
	run := a.run
	go func() {
		defer cancel()
		r, err := run(ctx, func(p agentinstall.Progress) {
			a.step.Store(int32(p.Step))
			a.done.Store(p.Done)
			a.total.Store(p.Total)
		})
		a.mu.Lock()
		a.res = &instResult{r: r, err: err}
		a.mu.Unlock()
	}()
	ws.armInstallPoll(w)
	w.Refresh()
}

func (ws *Workspace) armInstallPoll(w *gpui.Window) {
	a := ws.inst()
	a.poll.Cancel()
	a.poll = w.After(discoveryPoll, func(w *gpui.Window, _ *gpui.App) { ws.pollInstall(w) })
}

// cancelInstall is the Cancel button while installing: the installer stops its step and the poll
// adopts the cancellation.
func (ws *Workspace) cancelInstall(w *gpui.Window) {
	a := ws.inst()
	if a.phase != instRunning || a.cancel == nil {
		return
	}
	a.cancelling = true
	a.cancel()
	w.Refresh()
}

// dismissInstall closes the confirmation, the result or the error.
func (ws *Workspace) dismissInstall(w *gpui.Window) {
	a := ws.inst()
	if a.phase == instRunning {
		return
	}
	a.phase = instIdle
	w.Refresh()
}

// pollInstall is the poll timer body, on the UI goroutine. A finished installation becomes
// OpenCode's executable (in the settings, so a restart keeps it) and, when OpenCode has no
// credentials, opens a terminal running its sign-in.
func (ws *Workspace) pollInstall(w *gpui.Window) {
	a := ws.inst()
	a.mu.Lock()
	res := a.res
	a.res = nil
	a.mu.Unlock()
	if res == nil {
		ws.armInstallPoll(w)
		w.Refresh()
		return
	}
	a.cancel = nil
	switch {
	case errors.Is(res.err, context.Canceled):
		a.phase, a.cancelled = instIdle, true
	case res.err != nil:
		a.phase, a.err = instFailed, res.err.Error()
	default:
		a.phase, a.result = instDone, res.r
		ws.setAgentBinary(w, agentmanager.OpenCode, res.r.Binary)
		if !res.r.Authenticated {
			ws.openLogin(w)
		}
	}
	a.cancelling = false
	w.Refresh()
}

// openLogin opens a terminal running OpenCode's sign-in.
func (ws *Workspace) openLogin(w *gpui.Window) {
	a := ws.inst()
	if a.result.Binary == "" {
		return
	}
	name, err := ws.disc().open(ws.projectDir(), agentinstall.LoginArgs(a.result.Binary))
	if err != nil {
		a.login, a.loginErr = err.Error(), true
	} else {
		a.login, a.loginErr = name, false
	}
	w.Refresh()
}

// installWanted reports whether the install block belongs on screen: while the flow is under way,
// or when OpenCode cannot be found and missing is true.
func (ws *Workspace) installWanted(missing bool) bool {
	if ws.inst().phase != instIdle {
		return true
	}
	if !missing {
		return false
	}
	_, ok := harnesses.Available(agentmanager.OpenCode)
	return !ok
}

// agentInstallBlock is the "Install OpenCode" card: the offer, the confirmation, the progress, the
// result or the error. id prefixes every element's id, so the agent panel and the settings panel
// can both show it.
func (ws *Workspace) agentInstallBlock(id string) *gpui.Div {
	a := ws.inst()
	card := gpui.NewDiv().ID(id+"-install").FlexCol().Gap(6).P(12).Rounded(8).Bg(uiPanelAlt).Border(1, uiBorder).MinW(0)
	title := func(s string) *gpui.Text { return gpui.NewText(s).TextSize(fontUI).FontWeight(700) }
	line := func(s string) *gpui.Text { return gpui.NewText(s).TextSize(11).TextColor(uiMuted) }
	buttons := gpui.NewDiv().FlexRow().FlexWrap().ItemsCenter().Gap(8).Mt(4)
	switch a.phase {
	case instIdle:
		card.Child(title(i18n.T("ui.agent.install.offer_title")), line(i18n.T("ui.agent.install.offer_hint")))
		if a.cancelled {
			card.Child(line(i18n.T("ui.agent.install.cancelled")))
		}
		buttons.Child(actionButton(id+"-install-ask", i18n.T("ui.agent.install.offer"), i18n.T("ui.agent.install.offer_tip"), true, true,
			func(w *gpui.Window, _ *gpui.App) { ws.askInstall(w) }))
	case instConfirm:
		dir, err := agentinstall.Dir()
		if err != nil {
			dir = err.Error()
		}
		card.Child(title(i18n.T("ui.agent.install.confirm_title")),
			line(i18n.T("ui.agent.install.confirm_what")),
			line(i18n.T("ui.agent.install.confirm_where", dir)),
			line(i18n.T("ui.agent.install.confirm_login")))
		buttons.Child(
			actionButton(id+"-install-go", i18n.T("ui.agent.install.go"), i18n.T("ui.agent.install.go_tip"), true, true,
				func(w *gpui.Window, _ *gpui.App) { ws.startInstall(w) }),
			actionButton(id+"-install-no", i18n.T("ui.agent.install.not_now"), i18n.T("ui.agent.install.not_now_tip"), false, true,
				func(w *gpui.Window, _ *gpui.App) { ws.dismissInstall(w) }))
	case instRunning:
		card.Child(title(i18n.T("ui.agent.install.running_title")))
		cur := agentinstall.Step(a.step.Load())
		for _, s := range []struct {
			step agentinstall.Step
			key  string
		}{{agentinstall.StepMise, "ui.agent.install.step_mise"}, {agentinstall.StepTool, "ui.agent.install.step_tool"}, {agentinstall.StepAuth, "ui.agent.install.step_auth"}} {
			card.Child(installStepRow(s.step, cur, i18n.T(s.key)))
			if s.step == agentinstall.StepMise && cur == agentinstall.StepMise {
				if el := installBar(a.done.Load(), a.total.Load()); el != nil {
					card.Child(el)
				}
			}
		}
		label := i18n.T("ui.agent.install.cancel")
		if a.cancelling {
			label = i18n.T("ui.agent.install.cancelling")
		}
		buttons.Child(actionButton(id+"-install-cancel", label, i18n.T("ui.agent.install.cancel_tip"), false, !a.cancelling,
			func(w *gpui.Window, _ *gpui.App) { ws.cancelInstall(w) }))
	case instDone:
		card.Child(title(i18n.T("ui.agent.install.done_title")), line(a.result.Binary))
		switch {
		case a.result.Authenticated:
			card.Child(gpui.NewText(i18n.T("ui.agent.install.signed_in")).TextSize(11).TextColor(uiOK))
		case a.result.AuthErr != nil:
			card.Child(gpui.NewText(i18n.T("ui.agent.install.auth_failed", a.result.AuthErr.Error())).TextSize(11).TextColor(uiWarn))
		}
		switch {
		case a.login != "" && a.loginErr:
			card.Child(gpui.NewText(i18n.T("ui.agent.install.login_failed", a.login, a.result.Binary)).TextSize(11).TextColor(uiErr))
		case a.login != "":
			card.Child(line(i18n.T("ui.agent.install.login_opened", a.login)))
		}
		buttons.Child(
			actionButton(id+"-install-ok", i18n.T("ui.agent.install.ok"), i18n.T("ui.agent.install.ok_tip"), true, true,
				func(w *gpui.Window, _ *gpui.App) { ws.dismissInstall(w) }),
			actionButton(id+"-install-login", i18n.T("ui.agent.install.login"), i18n.T("ui.agent.install.login_tip"), false, true,
				func(w *gpui.Window, _ *gpui.App) { ws.openLogin(w) }))
	case instFailed:
		card.Child(title(i18n.T("ui.agent.install.failed_title")),
			gpui.NewText(a.err).TextSize(11).TextColor(uiErr))
		buttons.Child(
			actionButton(id+"-install-retry", i18n.T("ui.agent.install.retry"), i18n.T("ui.agent.install.retry_tip"), true, true,
				func(w *gpui.Window, _ *gpui.App) { ws.startInstall(w) }),
			actionButton(id+"-install-close", i18n.T("ui.agent.install.close"), i18n.T("ui.agent.install.close_tip"), false, true,
				func(w *gpui.Window, _ *gpui.App) { ws.dismissInstall(w) }))
	}
	return card.Child(buttons)
}

// installStepRow is one step of the progress list: a dot (done, running or waiting) and its name.
func installStepRow(step, cur agentinstall.Step, label string) *gpui.Div {
	col, txt := uiBorder, uiMuted
	switch {
	case step < cur:
		col, txt = uiOK, uiText
	case step == cur:
		col, txt = uiAccent, uiText
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Child(dot(8, col), gpui.NewText(label).TextSize(12).TextColor(txt))
}

// installBar is the mise download's progress bar with its megabytes, or nil before any byte.
func installBar(done, total int64) *gpui.Div {
	if done <= 0 {
		return nil
	}
	mb := func(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1<<20)) }
	text := i18n.T("ui.agent.install.mb", mb(done))
	row := gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Pl(16)
	if total > 0 {
		frac := min(float32(done)/float32(total), 1)
		text = i18n.T("ui.agent.install.mb_of", mb(done), mb(total))
		const barW = 110
		row.Child(gpui.NewDiv().SizeW(barW).SizeH(4).Rounded(2).Bg(uiBorder).FlexShrink(0).Child(
			gpui.NewDiv().SizeW(barW * frac).SizeH(4).Rounded(2).Bg(uiAccent)))
	}
	return row.Child(gpui.NewText(text).TextSize(11).TextColor(uiMuted))
}
