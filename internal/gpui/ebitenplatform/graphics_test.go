package ebitenplatform

import (
	"maps"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestGraphicsConfig(t *testing.T) {
	soft := map[string]string{"LIBGL_ALWAYS_SOFTWARE": "1", "GALLIUM_DRIVER": "llvmpipe"}
	cases := []struct {
		mode Graphics
		goos string
		lib  ebiten.GraphicsLibrary
		env  map[string]string
	}{
		{GraphicsAuto, "windows", ebiten.GraphicsLibraryAuto, nil},
		{GraphicsHardware, "windows", ebiten.GraphicsLibraryAuto, nil},
		{GraphicsSoftware, "windows", ebiten.GraphicsLibraryDirectX, map[string]string{"EBITENGINE_DIRECTX": "warp"}},
		{GraphicsAuto, "linux", ebiten.GraphicsLibraryAuto, nil},
		{GraphicsHardware, "linux", ebiten.GraphicsLibraryAuto, nil},
		{GraphicsSoftware, "linux", ebiten.GraphicsLibraryOpenGL, soft},
		{GraphicsSoftware, "freebsd", ebiten.GraphicsLibraryOpenGL, soft},
		{GraphicsSoftware, "darwin", ebiten.GraphicsLibraryOpenGL, nil},
		{GraphicsSoftware, "js", ebiten.GraphicsLibraryAuto, nil},
		{GraphicsAuto, "darwin", ebiten.GraphicsLibraryAuto, nil},
	}
	for _, c := range cases {
		lib, env := graphicsConfig(c.mode, c.goos)
		if lib != c.lib || !maps.Equal(env, c.env) {
			t.Errorf("graphicsConfig(%v, %s) = %v, %v; want %v, %v", c.mode, c.goos, lib, env, c.lib, c.env)
		}
	}
}

func TestGraphicsString(t *testing.T) {
	for g, want := range map[Graphics]string{GraphicsAuto: "auto", GraphicsHardware: "gpu", GraphicsSoftware: "software"} {
		if g.String() != want {
			t.Errorf("%d.String() = %q, want %q", g, g.String(), want)
		}
	}
}

func TestSetEnvRestores(t *testing.T) {
	t.Setenv("EBITENGINE_DIRECTX", "debug")
	t.Setenv("GALLIUM_DRIVER", "")
	os.Unsetenv("GALLIUM_DRIVER")
	restore := setEnv(map[string]string{"EBITENGINE_DIRECTX": "warp", "GALLIUM_DRIVER": "llvmpipe"})
	if v := os.Getenv("EBITENGINE_DIRECTX"); v != "debug,warp" {
		t.Errorf("EBITENGINE_DIRECTX = %q, want debug,warp", v)
	}
	if v := os.Getenv("GALLIUM_DRIVER"); v != "llvmpipe" {
		t.Errorf("GALLIUM_DRIVER = %q, want llvmpipe", v)
	}
	restore()
	if v := os.Getenv("EBITENGINE_DIRECTX"); v != "debug" {
		t.Errorf("restored EBITENGINE_DIRECTX = %q, want debug", v)
	}
	if _, ok := os.LookupEnv("GALLIUM_DRIVER"); ok {
		t.Error("GALLIUM_DRIVER still set after restore")
	}
}

func TestOptionsSetGraphicsAndFirstFrame(t *testing.T) {
	calls := 0
	p := New(WithGraphics(GraphicsSoftware), WithOnFirstFrame(func() { calls++ })).(*Platform)
	if p.graphics != GraphicsSoftware || p.onFirstFrame == nil {
		t.Fatalf("options not applied: %+v", p)
	}
	// A quitting platform makes game.Update return at once, without an App.
	p.quit.Store(true)
	ff := &firstFrameGame{game: &game{p: p}, done: p.onFirstFrame}
	_ = ff.Update()
	if calls != 0 {
		t.Fatal("first-frame hook ran before any Draw")
	}
	ff.Draw(nil)
	_ = ff.Update()
	_ = ff.Update()
	if calls != 1 || !ff.fired {
		t.Fatalf("first-frame hook ran %d times, want 1", calls)
	}
}
