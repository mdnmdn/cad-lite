package ebitenplatform

import (
	"errors"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Graphics selects the graphics device ebiten presents frames with. The
// frames are rasterized on the CPU in every mode; the device only uploads and
// shows the finished buffer.
type Graphics int

const (
	// GraphicsAuto lets ebiten pick the native library (DirectX or OpenGL on
	// Windows, OpenGL on Linux, Metal on macOS). It is the default.
	GraphicsAuto Graphics = iota
	// GraphicsHardware is GraphicsAuto chosen explicitly; the host does not
	// fall back to software when it fails.
	GraphicsHardware
	// GraphicsSoftware presents through a CPU implementation of the graphics
	// API: Direct3D 11 WARP on Windows, Mesa llvmpipe on Linux and the BSDs.
	// macOS and the browser have no such switch and behave as GraphicsAuto.
	GraphicsSoftware
)

// String returns "auto", "gpu" or "software".
func (g Graphics) String() string {
	switch g {
	case GraphicsHardware:
		return "gpu"
	case GraphicsSoftware:
		return "software"
	}
	return "auto"
}

// ErrStartup wraps every error Run returns before the first frame was
// presented: the graphics device or the window could not be created.
var ErrStartup = errors.New("ebitenplatform: graphics startup failed")

// WithGraphics selects the graphics device; the default is GraphicsAuto.
func WithGraphics(g Graphics) Option {
	return func(p *Platform) { p.graphics = g }
}

// WithOnFirstFrame registers f to run once, on the game loop, right after the
// first frame was handed to the graphics device.
func WithOnFirstFrame(f func()) Option {
	return func(p *Platform) { p.onFirstFrame = f }
}

// graphicsConfig maps a mode to the ebiten library and the environment the
// graphics stack reads at startup, for the OS goos. ebiten's DirectX driver
// reads EBITENGINE_DIRECTX ("warp" selects D3D_DRIVER_TYPE_WARP); Mesa reads
// LIBGL_ALWAYS_SOFTWARE and GALLIUM_DRIVER. macOS has no CPU presentation
// ebiten can reach, so software there is OpenGL instead of Metal. Modes without
// a software switch on goos return GraphicsLibraryAuto and no environment.
func graphicsConfig(mode Graphics, goos string) (ebiten.GraphicsLibrary, map[string]string) {
	if mode != GraphicsSoftware {
		return ebiten.GraphicsLibraryAuto, nil
	}
	switch goos {
	case "windows":
		return ebiten.GraphicsLibraryDirectX, map[string]string{"EBITENGINE_DIRECTX": "warp"}
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly":
		return ebiten.GraphicsLibraryOpenGL, map[string]string{"LIBGL_ALWAYS_SOFTWARE": "1", "GALLIUM_DRIVER": "llvmpipe"}
	case "darwin":
		return ebiten.GraphicsLibraryOpenGL, nil
	}
	return ebiten.GraphicsLibraryAuto, nil
}

// setEnv sets env in the process environment and returns a function that
// restores the previous values. A value already in EBITENGINE_DIRECTX is kept
// and "warp" appended, so a user's "debug" token survives.
func setEnv(env map[string]string) (restore func()) {
	type old struct {
		v  string
		ok bool
	}
	prev := make(map[string]old, len(env))
	for k, v := range env {
		o, ok := os.LookupEnv(k)
		prev[k] = old{o, ok}
		if k == "EBITENGINE_DIRECTX" && o != "" && !strings.Contains(o, v) {
			v = o + "," + v
		}
		os.Setenv(k, v)
	}
	return func() {
		for k, o := range prev {
			if o.ok {
				os.Setenv(k, o.v)
			} else {
				os.Unsetenv(k)
			}
		}
	}
}
