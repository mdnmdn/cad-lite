package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRendererDefaultsToAuto(t *testing.T) {
	if d := Default(); d.Renderer != RendererAuto {
		t.Fatalf("default renderer %q", d.Renderer)
	}
	s, w, err := Parse([]byte("[drawing]\ngrid = true\n"))
	if err != nil || len(w) != 0 || s.Renderer != RendererAuto {
		t.Fatalf("file without [display]: %q %v %v", s.Renderer, w, err)
	}
	text := string(Marshal(Default()))
	if !strings.Contains(text, "[display]\n") || !strings.Contains(text, `renderer = "auto"`) {
		t.Fatalf("no display table:\n%s", text)
	}
}

func TestRendererRoundTrips(t *testing.T) {
	for _, r := range Renderers {
		path := filepath.Join(t.TempDir(), "settings.toml")
		s := Default()
		s.Renderer = r
		if err := Save(path, s); err != nil {
			t.Fatal(err)
		}
		got, w, err := LoadWarn(path)
		if err != nil || len(w) != 0 || got.Renderer != r {
			t.Errorf("%s: got %q %v %v", r, got.Renderer, w, err)
		}
	}
}

func TestRendererInvalidIsClamped(t *testing.T) {
	for _, src := range []string{"[display]\nrenderer = \"vulkan\"\n", "[display]\nrenderer = 3\n"} {
		s, w, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if s.Renderer != RendererAuto || len(w) != 1 {
			t.Errorf("%q: renderer %q, warnings %v", src, s.Renderer, w)
		}
	}
}

func TestDisplayTableKeepsUnknownKeys(t *testing.T) {
	s, _, err := Parse([]byte("[display]\nrenderer = \"gpu\"\nvsync = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Renderer != RendererGPU || s.Extra["display.vsync"] != false {
		t.Fatalf("got %q %v", s.Renderer, s.Extra)
	}
	if text := string(Marshal(s)); !strings.Contains(text, "[display]\n# renderer") || !strings.Contains(text, "vsync = false") {
		t.Fatalf("unknown display key not written back:\n%s", text)
	}
}
