package ui

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"go-draw/internal/gpui"
	"go-draw/updater"
)

// Timing of the update checks.
const (
	// updateFirstDelay is how long after start the first automatic check runs.
	updateFirstDelay = 10 * time.Second
	// updateInterval separates automatic checks.
	updateInterval = 6 * time.Hour
	// updatePoll is how often the UI looks at the worker while it runs.
	updatePoll = 250 * time.Millisecond
	// updateCheckTimeout bounds one release lookup.
	updateCheckTimeout = 30 * time.Second
	// updateInstallTimeout bounds download, verification and installation.
	updateInstallTimeout = 20 * time.Minute
)

// updPhase is where the update flow is.
type updPhase int

const (
	updIdle        updPhase = iota // nothing checked yet
	updChecking                    // a release lookup is running
	updUpToDate                    // the last check found nothing newer
	updAvailable                   // a newer release can be installed
	updDownloading                 // the archive is downloading
	updInstalling                  // verifying and replacing the installation
	updReady                       // installed; waiting for the restart
	updError                       // the last check or update failed
)

// updResult is what the worker goroutine hands to the UI goroutine.
type updResult struct {
	install   bool
	release   updater.Release
	available bool
	plan      updater.Plan
	err       error
}

// updateState is a workspace's update state. Everything but mu, res and the
// progress counters belongs to the UI goroutine; the worker goroutine writes
// only those, and the poll timer moves the result across.
type updateState struct {
	enabled bool
	started bool
	version string
	client  *updater.Client

	phase   updPhase
	latest  updater.Release
	err     error
	confirm bool
	plan    updater.Plan
	restart bool

	auto, poll *gpui.Timer

	mu   sync.Mutex
	res  *updResult
	done atomic.Int64
	size atomic.Int64
}

var updTable = struct {
	sync.Mutex
	m map[weak.Pointer[Workspace]]*updateState
}{m: map[weak.Pointer[Workspace]]*updateState{}}

// upd returns ws's update state, creating it on first use. Like prefs, it
// lives beside the Workspace so the Workspace stays collectable.
func (ws *Workspace) upd() *updateState {
	key := weak.Make(ws)
	updTable.Lock()
	defer updTable.Unlock()
	if u, ok := updTable.m[key]; ok {
		return u
	}
	u := &updateState{version: "dev"}
	updTable.m[key] = u
	runtime.AddCleanup(ws, func(k weak.Pointer[Workspace]) {
		updTable.Lock()
		delete(updTable.m, k)
		updTable.Unlock()
	}, key)
	return u
}

// SetVersion tells the workspace which release it is, for the settings panel
// and for deciding whether a release is newer. Hosts pass the value stamped
// into the binary ("dev" when none was).
func (ws *Workspace) SetVersion(v string) {
	if v == "" {
		v = "dev"
	}
	u := ws.upd()
	u.version = v
	u.client = nil
}

// Version returns the version set with SetVersion, "dev" by default.
func (ws *Workspace) Version() string { return ws.upd().version }

// SetUpdateClient replaces the client that looks for releases (tests point it
// at a local server). It implies EnableUpdates.
func (ws *Workspace) SetUpdateClient(c *updater.Client) {
	u := ws.upd()
	u.client = c
	u.enabled = true
}

// EnableUpdates turns on the update machinery: the automatic checks (when the
// setting allows them) and the panel's buttons. Hosts that must not touch the
// network (-png renders, tests) do not call it. It does nothing where
// updating is unsupported (the browser) or when GODRAW_NO_UPDATE=1.
func (ws *Workspace) EnableUpdates() {
	if !updater.Supported() || updater.Disabled() {
		return
	}
	u := ws.upd()
	u.enabled = true
	go updater.CleanupOld()
}

// UpdatesEnabled reports whether update checks can run in this workspace.
func (ws *Workspace) UpdatesEnabled() bool { return ws.upd().enabled }

// UpdateAvailable returns the release waiting to be installed, if any.
func (ws *Workspace) UpdateAvailable() (updater.Release, bool) {
	u := ws.upd()
	if u.phase == updAvailable || u.phase == updReady {
		return u.latest, true
	}
	return updater.Release{}, false
}

// RelaunchIfPending starts the newly installed version when an update asked
// for a restart. The host calls it as the very last step of quitting, after
// settings are saved and the agent is closed.
func (ws *Workspace) RelaunchIfPending() error {
	u := ws.upd()
	if !u.restart {
		return nil
	}
	u.restart = false
	return updater.Relaunch(u.plan)
}

func (u *updateState) busy() bool {
	return u.phase == updChecking || u.phase == updDownloading || u.phase == updInstalling
}

func (u *updateState) getClient() *updater.Client {
	if u.client == nil {
		u.client = updater.NewClient(u.version)
	}
	return u.client
}

// startUpdates arms the automatic checks once the window exists; Render calls
// it every frame through mountSettings.
func (ws *Workspace) startUpdates(w *gpui.Window) {
	u := ws.upd()
	if !u.enabled || u.started || w == nil {
		return
	}
	u.started = true
	ws.armAutoCheck(w, updateFirstDelay)
}

// armAutoCheck schedules the next automatic check after d, or nothing when
// automatic checks are off.
func (ws *Workspace) armAutoCheck(w *gpui.Window, d time.Duration) {
	u := ws.upd()
	u.auto.Cancel()
	u.auto = nil
	if !u.enabled || !ws.prefs().cur.AutoUpdate || !updater.Updatable(u.version) {
		return
	}
	u.auto = w.After(d, func(w *gpui.Window, _ *gpui.App) {
		if ws.prefs().cur.AutoUpdate && (u.phase == updIdle || u.phase == updUpToDate || u.phase == updError) {
			ws.checkForUpdate(w, false)
		}
		ws.armAutoCheck(w, updateInterval)
	})
}

// armPoll keeps looking at the worker until it is done.
func (ws *Workspace) armPoll(w *gpui.Window) {
	u := ws.upd()
	u.poll.Cancel()
	u.poll = w.After(updatePoll, func(w *gpui.Window, cx *gpui.App) { ws.pollUpdate(w, cx) })
}

// checkForUpdate starts a release lookup on a goroutine. force bypasses the
// ETag cache (the button); the automatic check keeps it.
func (ws *Workspace) checkForUpdate(w *gpui.Window, force bool) {
	u := ws.upd()
	if !u.enabled || u.busy() {
		return
	}
	u.phase, u.err = updChecking, nil
	c, version := u.getClient(), u.version
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		check := c.Check
		if force {
			check = c.CheckForce
		}
		rel, ok, err := check(ctx, version)
		u.mu.Lock()
		u.res = &updResult{release: rel, available: ok, err: err}
		u.mu.Unlock()
	}()
	ws.armPoll(w)
	w.Refresh()
}

// startUpdate downloads, verifies and installs the available release on a
// goroutine; when it succeeds the restart follows (see finishInstall).
func (ws *Workspace) startUpdate(w *gpui.Window) {
	u := ws.upd()
	if !u.enabled || u.phase != updAvailable {
		return
	}
	u.phase, u.err = updDownloading, nil
	u.done.Store(0)
	u.size.Store(0)
	c, rel := u.getClient(), u.latest
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateInstallTimeout)
		defer cancel()
		plan, err := c.Update(ctx, rel, func(done, total int64) {
			u.done.Store(done)
			u.size.Store(total)
		})
		u.mu.Lock()
		u.res = &updResult{install: true, release: rel, plan: plan, err: err}
		u.mu.Unlock()
	}()
	ws.armPoll(w)
	w.Refresh()
}

// pollUpdate is the poll timer body, on the UI goroutine: it adopts the
// worker's result and keeps polling while the worker runs.
func (ws *Workspace) pollUpdate(w *gpui.Window, cx *gpui.App) {
	u := ws.upd()
	u.mu.Lock()
	res := u.res
	u.res = nil
	u.mu.Unlock()
	if res != nil {
		ws.adoptUpdate(w, cx, res)
	}
	if u.busy() {
		ws.armPoll(w)
	}
	w.Refresh()
}

func (ws *Workspace) adoptUpdate(w *gpui.Window, cx *gpui.App, r *updResult) {
	u := ws.upd()
	if r.install {
		if r.err != nil {
			u.phase, u.err = updAvailable, r.err
			return
		}
		u.phase, u.plan = updReady, r.plan
		ws.finishInstall(w, cx)
		return
	}
	u.err = r.err
	switch {
	case r.err != nil:
		u.phase = updError
		if !r.release.IsZero() {
			u.latest = r.release
		}
	case r.available:
		u.phase, u.latest = updAvailable, r.release
	default:
		u.phase, u.latest = updUpToDate, r.release
	}
	if r.err == nil || !r.release.IsZero() {
		p := ws.prefs()
		p.cur.LastUpdateCheck = time.Now()
		ws.scheduleSave(w)
	}
}

// finishInstall runs once the new version is on disk. A drawing without
// edits restarts at once, so one press of Update does everything; one with
// unsaved changes asks first (Session.Dirty: the drawing changed since it was created, opened or
// saved).
func (ws *Workspace) finishInstall(w *gpui.Window, cx *gpui.App) {
	if ws.session.Dirty() {
		ws.upd().confirm = true
		return
	}
	ws.restartNow(w, cx)
}

// restartNow quits the application and asks the host to start the new
// version once it is closed (RelaunchIfPending).
func (ws *Workspace) restartNow(w *gpui.Window, cx *gpui.App) {
	u := ws.upd()
	u.confirm = false
	u.restart = true
	ws.SaveSettings()
	cx.Quit()
}

// openReleasePage opens the release on GitHub in the browser.
func (ws *Workspace) openReleasePage() {
	u := ws.upd()
	if u.latest.PageURL != "" {
		if err := updater.OpenURL(u.latest.PageURL); err != nil {
			u.err = err
		}
	}
}
