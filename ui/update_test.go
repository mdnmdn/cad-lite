package ui

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/settings"
	"cad-lite/updater"
)

type updRig struct {
	srv     *httptest.Server
	hits    atomic.Int32
	sums    string
	archive []byte
	exe     string
	client  *updater.Client
}

func newUpdRig(t *testing.T, tag string) *updRig {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "d/bin/cad-lite", Mode: 0o755, Size: 3, Typeflag: tar.TypeReg})
	tw.Write([]byte("NEW"))
	tw.Close()
	gz.Close()
	r := &updRig{archive: buf.Bytes()}
	name := "cad-lite-" + strings.TrimPrefix(tag, "v") + "-linux-amd64.tar.gz"
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/mdnmdn/go-draw/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		r.hits.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "html_url": r.srv.URL + "/page",
			"body": "## What's new\n- **Faster** zoom\n- Fixed `LINE`\n",
			"assets": []map[string]any{
				{"name": name, "browser_download_url": r.srv.URL + "/dl/" + name},
				{"name": "SHA256SUMS.txt", "browser_download_url": r.srv.URL + "/dl/SHA256SUMS.txt"},
			}})
	})
	mux.HandleFunc("/dl/SHA256SUMS.txt", func(w http.ResponseWriter, _ *http.Request) {
		if r.sums != "" {
			fmt.Fprint(w, r.sums)
			return
		}
		fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(r.archive), name)
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, _ *http.Request) { w.Write(r.archive) })
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	r.exe = filepath.Join(t.TempDir(), "cad-lite")
	os.WriteFile(r.exe, []byte("OLD"), 0o755)
	r.client = updater.NewClient("1.0.0")
	r.client.BaseURL, r.client.GOOS, r.client.GOARCH, r.client.Exe = r.srv.URL, "linux", "amd64", r.exe
	return r
}

// settle advances the virtual clock until cond holds, giving the worker
// goroutine real time to finish.
func (h *sHarness) settle(cond func() bool) {
	h.t.Helper()
	for i := 0; i < 200; i++ {
		h.hp.Advance(300 * time.Millisecond)
		h.hw.Pixels()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatal("condition not reached")
}

func updHarness(t *testing.T, version string, edit bool) (*sHarness, *updRig) {
	rig := newUpdRig(t, "v2.0.0")
	ws := NewWorkspace(doc.New(), nil)
	if edit {
		ws.Session().Script("LINE 0,0 10,10  ")
	}
	ws.SetVersion(version)
	ws.SetUpdateClient(rig.client)
	h := openSettingsWS(t, ws, 900, 2000, 1) // tall enough that the whole panel shows
	return h, rig
}

func TestUpdateButtonChecksAndInstallsThenRestarts(t *testing.T) {
	h, rig := updHarness(t, "1.0.0", false)
	h.openPanel()
	h.clickID("update-check")
	h.settle(func() bool { _, ok := h.ws.UpdateAvailable(); return ok })
	if _, ok := h.ws.UpdateAvailable(); !ok || h.ws.upd().latest.Version != "2.0.0" {
		t.Fatalf("latest %+v", h.ws.upd().latest)
	}
	if h.ws.Settings().LastUpdateCheck.IsZero() {
		t.Error("last check not recorded")
	}
	st, _ := h.ws.updateStatus()
	if !strings.Contains(st, "2.0.0") {
		t.Errorf("status %q", st)
	}
	h.png("update-available-en.png")
	h.clickID("update-install")
	h.settle(func() bool { return h.ws.upd().phase == updReady })
	u := h.ws.upd()
	if !u.confirm || u.restart {
		t.Fatalf("a clean drawing must still ask before restarting: restart=%v confirm=%v", u.restart, u.confirm)
	}
	h.clickID("update-restart-now")
	if !u.restart || u.plan.Exe != rig.exe || u.confirm {
		t.Errorf("Restart and update: restart=%v confirm=%v plan=%+v", u.restart, u.confirm, u.plan)
	}
	if b, _ := os.ReadFile(rig.exe); string(b) != "NEW" {
		t.Errorf("exe %q", b)
	}
}

func TestUpdateAsksBeforeRestartWhenDrawingHasEdits(t *testing.T) {
	h, rig := updHarness(t, "1.0.0", true)
	h.ws.checkForUpdate(h.hw.Window(), true)
	h.settle(func() bool { return h.ws.upd().phase == updAvailable })
	h.ws.startUpdate(h.hw.Window())
	h.settle(func() bool { return h.ws.upd().phase == updReady })
	if !h.ws.upd().confirm || h.ws.upd().restart {
		t.Fatal("expected the confirmation dialog")
	}
	h.png("update-confirm-en.png")
	h.clickID("update-restart-later")
	if h.ws.upd().confirm || h.ws.upd().restart {
		t.Error("Later must keep the app running")
	}
	_ = rig
}

func TestUpdateChecksumMismatchKeepsInstall(t *testing.T) {
	h, rig := updHarness(t, "1.0.0", false)
	rig.sums = strings.Repeat("0", 64) + "  cad-lite-2.0.0-linux-amd64.tar.gz\n"
	h.ws.checkForUpdate(h.hw.Window(), true)
	h.settle(func() bool { return h.ws.upd().phase == updAvailable })
	h.ws.startUpdate(h.hw.Window())
	h.settle(func() bool { return h.ws.upd().err != nil })
	if b, _ := os.ReadFile(rig.exe); string(b) != "OLD" || h.ws.upd().restart {
		t.Errorf("exe %q restart %v", b, h.ws.upd().restart)
	}
	h.openPanel()
	h.hw.Pixels()
	h.find("update-page")
	h.png("update-error-en.png")
}

func TestAutoCheckTimerAndToggle(t *testing.T) {
	h, rig := updHarness(t, "1.0.0", false)
	h.hw.Pixels()
	h.hp.Advance(5 * time.Second)
	if rig.hits.Load() != 0 {
		t.Fatal("checked before the start delay")
	}
	h.settle(func() bool { return rig.hits.Load() >= 1 })
	h.settle(func() bool { return h.ws.upd().phase == updAvailable })
	// Turning the setting off cancels the timer.
	h.ws.change(h.hw.Window(), func(s *settings.Settings) { s.AutoUpdate = false })
	n := rig.hits.Load()
	h.hp.Advance(13 * time.Hour)
	time.Sleep(50 * time.Millisecond)
	if rig.hits.Load() != n {
		t.Error("checked with auto_check off")
	}
}

func TestDevBuildNeverAutoChecksButCanReportLatest(t *testing.T) {
	h, rig := updHarness(t, "dev", false)
	h.hw.Pixels()
	h.hp.Advance(7 * time.Hour)
	time.Sleep(50 * time.Millisecond)
	if rig.hits.Load() != 0 {
		t.Error("dev build checked automatically")
	}
	h.ws.checkForUpdate(h.hw.Window(), true)
	h.settle(func() bool { return h.ws.upd().phase == updUpToDate })
	if _, ok := h.ws.UpdateAvailable(); ok {
		t.Error("dev build offered an update")
	}
	if st, _ := h.ws.updateStatus(); !strings.Contains(st, "2.0.0") {
		t.Errorf("status %q", st)
	}
}

func TestUpdateWithoutEnableShowsNoButtons(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	ws.SetVersion("1.2.3")
	h := openSettingsWS(t, ws, 900, 1000, 1)
	h.openPanel()
	if ws.UpdatesEnabled() || ws.Version() != "1.2.3" {
		t.Fatal("state")
	}
}

func TestUpdatePanelItalian(t *testing.T) {
	i18n.Set(i18n.FromSetting("it"))
	t.Cleanup(func() { i18n.Set(i18n.FromSetting("en")) })
	h, _ := updHarness(t, "1.0.0", false)
	h.openPanel()
	h.ws.checkForUpdate(h.hw.Window(), true)
	h.settle(func() bool { return h.ws.upd().phase == updAvailable })
	h.png("update-available-it.png")
}

func TestNotesExcerpt(t *testing.T) {
	got := notesExcerpt("## What's new\r\n\n- **Faster** zoom\n* Fixed `LINE`\n" + strings.Repeat("x", 200))
	if len(got) != 3 || got[0] != "• Faster zoom" || got[1] != "• Fixed LINE" || len([]rune(got[2])) != notesMaxWidth {
		t.Errorf("%q", got)
	}
	if len(notesExcerpt(strings.Repeat("a\n", 50))) != notesMaxLines {
		t.Error("not limited")
	}
}

func TestRestartDialogWarnsOnlyWhenDirty(t *testing.T) {
	if got := i18n.T(restartBodyKey(true), "2.0.0"); !strings.Contains(got, "unsaved") {
		t.Errorf("dirty body %q", got)
	}
	if got := i18n.T(restartBodyKey(false), "2.0.0"); strings.Contains(got, "unsaved") {
		t.Errorf("clean body %q", got)
	}
}
