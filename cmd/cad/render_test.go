package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cad-lite/internal/gpui/ebitenplatform"
	"cad-lite/settings"
	"cad-lite/ui"
)

func TestResolveRenderMode(t *testing.T) {
	cases := []struct {
		flag, env, setting, goos string
		mode                     string
		forced, warn             bool
	}{
		{"", "", "", "windows", settings.RendererAuto, false, false},
		{"", "", "software", "windows", settings.RendererSoftware, false, false},
		{"", "", "bogus", "linux", settings.RendererAuto, false, false},
		{"", "gpu", "software", "linux", settings.RendererGPU, true, false},
		{"", "CPU", "auto", "linux", settings.RendererSoftware, true, false},
		{"", "vulkan", "software", "linux", settings.RendererSoftware, false, true},
		{"software", "gpu", "auto", "windows", settings.RendererSoftware, true, false},
		{"cpu", "", "", "windows", settings.RendererSoftware, true, false},
		{"Auto", "software", "software", "windows", settings.RendererAuto, true, false},
		{"software", "", "", "darwin", settings.RendererSoftware, true, false},
		{"", "", "software", "js", settings.RendererAuto, false, true},
		{"", "", "auto", "darwin", settings.RendererAuto, false, false},
	}
	for _, c := range cases {
		mode, forced, warn, err := resolveRenderMode(c.flag, c.env, c.setting, c.goos)
		if err != nil || mode != c.mode || forced != c.forced || (warn != "") != c.warn {
			t.Errorf("resolveRenderMode(%q, %q, %q, %s) = %q, %v, %q, %v; want %q, %v, warn %v",
				c.flag, c.env, c.setting, c.goos, mode, forced, warn, err, c.mode, c.forced, c.warn)
		}
	}
	if _, _, _, err := resolveRenderMode("vulkan", "", "", "linux"); err == nil {
		t.Error("a bad -render flag should be an error")
	}
}

func TestDecideRenderStart(t *testing.T) {
	soft, hw, auto := ebitenplatform.GraphicsSoftware, ebitenplatform.GraphicsHardware, ebitenplatform.GraphicsAuto
	cases := []struct {
		name                 string
		mode                 string
		forced, child, stale bool
		goos                 string
		graphics             ebitenplatform.Graphics
		probe                bool
		info                 ui.RenderInfo
	}{
		{"auto probes", settings.RendererAuto, false, false, false, "windows", auto, true, ui.RenderInfo{Active: "gpu"}},
		{"stale marker starts software", settings.RendererAuto, false, false, true, "linux", soft, false, ui.RenderInfo{Active: "software", Fallback: true}},
		{"fallback child", settings.RendererSoftware, true, true, false, "windows", soft, false, ui.RenderInfo{Active: "software", Fallback: true}},
		{"software setting", settings.RendererSoftware, false, false, true, "windows", soft, false, ui.RenderInfo{Active: "software"}},
		{"gpu flag ignores marker", settings.RendererGPU, true, false, true, "linux", hw, false, ui.RenderInfo{Active: "gpu", Forced: true}},
		{"macOS stale marker starts OpenGL", settings.RendererAuto, false, false, true, "darwin", soft, false, ui.RenderInfo{Active: "software", Fallback: true}},
		{"macOS auto probes", settings.RendererAuto, false, false, false, "darwin", auto, true, ui.RenderInfo{Active: "gpu"}},
		{"browser never probes", settings.RendererAuto, false, false, true, "js", auto, false, ui.RenderInfo{}},
	}
	for _, c := range cases {
		got := decideRenderStart(c.mode, c.forced, c.child, c.stale, c.goos)
		if got.graphics != c.graphics || got.probe != c.probe || got.info != c.info {
			t.Errorf("%s: got %+v, want graphics %v probe %v info %+v", c.name, got, c.graphics, c.probe, c.info)
		}
	}
}

func TestShouldFallback(t *testing.T) {
	startup := fmt.Errorf("%w: %w", ebitenplatform.ErrStartup, errors.New("no OpenGL 3.2"))
	probing := renderStart{probe: true}
	if !shouldFallback(probing, startup) {
		t.Error("a startup failure while probing should fall back")
	}
	if shouldFallback(probing, errors.New("later failure")) {
		t.Error("an error after the first frame must not fall back")
	}
	if shouldFallback(renderStart{graphics: ebitenplatform.GraphicsHardware}, startup) {
		t.Error("an explicit gpu start must not fall back")
	}
	if shouldFallback(probing, nil) {
		t.Error("a clean exit must not fall back")
	}
}

func TestProbeMarkerLivesBesideSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CADLITE_CONFIG_DIR", dir)
	p := probePath()
	if p != filepath.Join(dir, probeName) {
		t.Fatalf("probePath = %q", p)
	}
	if probeExists(p) {
		t.Fatal("marker before writing")
	}
	writeProbe(p)
	if !probeExists(p) {
		t.Fatal("marker not written")
	}
	// A marker left behind by a start that died sends the next auto start to software.
	if s := decideRenderStart(settings.RendererAuto, false, false, probeExists(p), "windows"); s.graphics != ebitenplatform.GraphicsSoftware || !s.info.Fallback {
		t.Fatalf("stale marker ignored: %+v", s)
	}
	removeProbe(p)
	if probeExists(p) {
		t.Fatal("marker not removed")
	}
	removeProbe("")
	writeProbe("")
	if probeExists("") {
		t.Fatal("empty path exists")
	}
}

func TestTakeFallbackEnv(t *testing.T) {
	t.Setenv("CADLITE_RENDER", "software")
	t.Setenv("CADLITE_RENDER_FALLBACK", "1")
	if !takeFallbackEnv() {
		t.Fatal("fallback child not recognized")
	}
	for _, k := range []string{"CADLITE_RENDER", "CADLITE_RENDER_FALLBACK"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Errorf("%s still set: a restart would inherit it", k)
		}
	}
	if takeFallbackEnv() {
		t.Fatal("second call should see no fallback")
	}
	t.Setenv("CADLITE_RENDER", "software")
	if takeFallbackEnv() || os.Getenv("CADLITE_RENDER") != "software" {
		t.Fatal("a user's CADLITE_RENDER must be kept")
	}
}

// TestMain runs main itself when the test binary is started as the application
// (TestPNGIgnoresRender does), so the flags are exercised end to end.
func TestMain(m *testing.M) {
	if os.Getenv("CADLITE_TEST_RUN_MAIN") == "1" {
		os.Args = append([]string{"cad"}, os.Args[1:]...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runMain starts this test binary as the application with args.
func runMain(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "CADLITE_TEST_RUN_MAIN=1", "CADLITE_CONFIG_DIR="+t.TempDir(),
		"CADLITE_ANALYTICS=off", "CADLITE_NO_UPDATE=1", "GPUI_NO_SYSTEM_FONTS=1", "CADLITE_RENDER=gpu")
	return cmd.CombinedOutput()
}

// A -png render is headless and CPU-only: -render (and CADLITE_RENDER) change nothing in it,
// and a bad -render value is still refused.
func TestPNGIgnoresRender(t *testing.T) {
	if testing.Short() {
		t.Skip("renders the demo plate three times")
	}
	dir := t.TempDir()
	plain, soft := filepath.Join(dir, "plain.png"), filepath.Join(dir, "soft.png")
	if out, err := runMain(t, "-width", "640", "-height", "400", "-png", plain); err != nil {
		t.Fatalf("plain -png: %v\n%s", err, out)
	}
	if out, err := runMain(t, "-render=software", "-width", "640", "-height", "400", "-png", soft); err != nil {
		t.Fatalf("-render=software -png: %v\n%s", err, out)
	}
	a, errA := os.ReadFile(plain)
	b, errB := os.ReadFile(soft)
	if errA != nil || errB != nil || len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatalf("renders differ or are missing: %v %v, %d and %d bytes", errA, errB, len(a), len(b))
	}
	if out, err := runMain(t, "-render=vulkan", "-png", filepath.Join(dir, "bad.png")); err == nil {
		t.Fatalf("a bad -render value was accepted:\n%s", out)
	}
}
