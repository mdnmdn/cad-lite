package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"cad-lite/appenv"
	"cad-lite/internal/gpui/ebitenplatform"
	"cad-lite/settings"
	"cad-lite/ui"
	"cad-lite/updater"
)

// Render mode environment: CADLITE_RENDER picks the renderer like -render;
// CADLITE_RENDER_FALLBACK=1 marks the process the automatic fallback started.
const (
	envRender         = "RENDER"
	envRenderFallback = "RENDER_FALLBACK"
	// probeName is the marker file written beside the settings file while a
	// graphics-card start is in progress; one left behind means the last
	// start died before its first frame.
	probeName = "render-probe"
)

// parseRenderMode accepts auto, gpu, software and its alias cpu, in any case.
func parseRenderMode(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto":
		return settings.RendererAuto, true
	case "gpu":
		return settings.RendererGPU, true
	case "software", "cpu":
		return settings.RendererSoftware, true
	}
	return "", false
}

// softwareAvailable reports whether goos has a software renderer to switch to.
func softwareAvailable(goos string) bool { return goos != "darwin" && goos != "js" }

// resolveRenderMode picks the renderer: the -render flag, else CADLITE_RENDER,
// else the setting, else auto. forced reports that the flag or the variable
// decided. A bad flag is an error; a bad variable or setting is skipped with a
// warning. Where goos has no software renderer the result is always auto.
func resolveRenderMode(flagV, env, setting, goos string) (mode string, forced bool, warn string, err error) {
	mode = settings.RendererAuto
	switch {
	case flagV != "":
		m, ok := parseRenderMode(flagV)
		if !ok {
			return "", false, "", fmt.Errorf("-render %q: want auto, gpu or software (cpu)", flagV)
		}
		mode, forced = m, true
	case env != "":
		if m, ok := parseRenderMode(env); ok {
			mode, forced = m, true
		} else {
			warn = fmt.Sprintf("CADLITE_RENDER=%q: want auto, gpu or software (cpu); ignored", env)
			if m, ok := parseRenderMode(setting); ok {
				mode = m
			}
		}
	default:
		if m, ok := parseRenderMode(setting); ok {
			mode = m
		}
	}
	if !softwareAvailable(goos) {
		if mode != settings.RendererAuto && warn == "" {
			warn = fmt.Sprintf("renderer %q has no effect on %s", mode, goos)
		}
		return settings.RendererAuto, forced, warn, nil
	}
	return mode, forced, warn, nil
}

// renderStart is how this process starts the window.
type renderStart struct {
	// graphics is the device the platform uses.
	graphics ebitenplatform.Graphics
	// info is what the workspace is told.
	info ui.RenderInfo
	// probe is set when the start must write the marker and may relaunch
	// itself in software mode if the graphics card fails.
	probe bool
}

// decideRenderStart turns the resolved mode into a start. fallbackChild is
// set in the process the fallback relaunched; staleProbe when a marker from a
// start that died is present.
func decideRenderStart(mode string, forced, fallbackChild, staleProbe bool, goos string) renderStart {
	switch {
	case fallbackChild:
		return renderStart{graphics: ebitenplatform.GraphicsSoftware,
			info: ui.RenderInfo{Active: settings.RendererSoftware, Fallback: true}}
	case mode == settings.RendererSoftware:
		return renderStart{graphics: ebitenplatform.GraphicsSoftware,
			info: ui.RenderInfo{Active: settings.RendererSoftware, Forced: forced}}
	case mode == settings.RendererGPU:
		return renderStart{graphics: ebitenplatform.GraphicsHardware,
			info: ui.RenderInfo{Active: settings.RendererGPU, Forced: forced}}
	case !softwareAvailable(goos):
		return renderStart{graphics: ebitenplatform.GraphicsAuto}
	case staleProbe:
		return renderStart{graphics: ebitenplatform.GraphicsSoftware,
			info: ui.RenderInfo{Active: settings.RendererSoftware, Fallback: true, Forced: forced}}
	}
	return renderStart{graphics: ebitenplatform.GraphicsAuto, probe: true,
		info: ui.RenderInfo{Active: settings.RendererGPU, Forced: forced}}
}

// probePath is the marker file beside the settings file, "" without a
// settings directory (the browser).
func probePath() string {
	p, err := settings.Path()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(p), probeName)
}

// writeProbe creates the marker; failures are logged and ignored.
func writeProbe(path string) {
	if path == "" {
		return
	}
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err == nil {
		err = os.WriteFile(path, []byte("graphics start in progress\n"), 0o600)
	}
	if err != nil {
		log.Printf("render: probe: %v", err)
	}
}

// probeExists reports whether a marker is present.
func probeExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// removeProbe deletes the marker, if any.
func removeProbe(path string) {
	if path != "" {
		os.Remove(path)
	}
}

// takeFallbackEnv reports whether this process is the automatic fallback and
// removes the variables the fallback set, so a process this one starts later
// (a restart, an update) obeys the setting again.
func takeFallbackEnv() bool {
	if appenv.Get(envRenderFallback) != "1" {
		return false
	}
	for _, p := range []string{"CADLITE_", "GODRAW_"} {
		os.Unsetenv(p + envRender)
		os.Unsetenv(p + envRenderFallback)
	}
	return true
}

// shouldFallback reports whether a failed Run should be retried in software
// mode: the start probed the graphics card and failed before the first frame.
func shouldFallback(start renderStart, err error) bool {
	return start.probe && errors.Is(err, ebitenplatform.ErrStartup)
}

// relaunchSoftware starts this executable again, with the same arguments, in
// software mode and marked as the fallback; the caller then exits.
func relaunchSoftware() error {
	exe, err := updater.Executable()
	if err != nil {
		return err
	}
	os.Setenv("CADLITE_"+envRender, settings.RendererSoftware)
	os.Setenv("CADLITE_"+envRenderFallback, "1")
	return updater.Relaunch(updater.Plan{GOOS: runtime.GOOS, Exe: exe, Args: append([]string(nil), os.Args[1:]...)})
}
