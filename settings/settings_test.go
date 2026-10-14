package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Default()
	if d.Theme != "dark" || d.UIScale != 1 || !d.Grid || !d.OSnap || d.Ortho || d.Wheel != WheelAuto {
		t.Fatalf("unexpected defaults: %+v", d)
	}
	if w := d.Normalize(); len(w) != 0 {
		t.Fatalf("defaults need clamping: %v", w)
	}
}

func TestMissingFileYieldsDefaults(t *testing.T) {
	s, w, err := LoadWarn(filepath.Join(t.TempDir(), "nope", "settings.toml"))
	if err != nil || len(w) != 0 || !reflect.DeepEqual(s, Default()) {
		t.Fatalf("got %+v %v %v", s, w, err)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go-draw", "settings.toml")
	want := Settings{
		Theme: "light", Language: LangItalian, UIScale: 1.25, Grid: false, OSnap: true, Ortho: true,
		Wheel: WheelPan, DefaultAgent: "claude", Window: Window{Width: 1000, Height: 700},
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, w, err := LoadWarn(path)
	if err != nil || len(w) != 0 {
		t.Fatalf("load: %v %v", w, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip\n got %+v\nwant %+v", got, want)
	}
}

func TestFileHasHeaderComment(t *testing.T) {
	text := string(Marshal(Default()))
	if !strings.HasPrefix(text, "# go-draw settings") || !strings.Contains(text, "[appearance]") || !strings.Contains(text, "ui_scale = 1.0") {
		t.Fatalf("unexpected file:\n%s", text)
	}
}

func TestClamping(t *testing.T) {
	s, w, err := Parse([]byte(`
[appearance]
theme = ""
ui_scale = 9
language = "fr"
[drawing]
wheel = "sideways"
grid = "yes"
[window]
width = 10
height = 99999
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Language != LangAuto || s.Theme != "dark" || s.UIScale != 2 || s.Wheel != WheelAuto || !s.Grid {
		t.Fatalf("not clamped: %+v", s)
	}
	if s.Window.Width != MinWindowSize || s.Window.Height != MaxWindowSize {
		t.Fatalf("window not clamped: %+v", s.Window)
	}
	if len(w) < 6 {
		t.Fatalf("want a warning per bad value, got %v", w)
	}
}

func TestUIScaleSnapsToStep(t *testing.T) {
	for in, want := range map[float64]float64{0: 0.75, 1.02: 1.0, 1.03: 1.05, 1.5: 1.5, 3: 2, 0.8: 0.8} {
		if got := ClampUIScale(in); got != want {
			t.Errorf("ClampUIScale(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestIntegerUIScaleAccepted(t *testing.T) {
	s, w, err := Parse([]byte("[appearance]\nui_scale = 2\n"))
	if err != nil || len(w) != 0 || s.UIScale != 2 {
		t.Fatalf("%+v %v %v", s, w, err)
	}
}

func TestUnknownKeysPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	src := `
future_flag = true
[appearance]
theme = "classic"
accent = "#ff0000"
[drawing]
snap_aperture = 7
[experimental]
level = 3
names = ["a", "b"]
[experimental.deep]
x = 1.5
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "classic" {
		t.Fatalf("theme = %q", s.Theme)
	}
	s.UIScale = 1.5
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	again, w, err := LoadWarn(path)
	if err != nil || len(w) != 0 {
		t.Fatalf("%v %v", w, err)
	}
	if again.UIScale != 1.5 || !reflect.DeepEqual(again.Extra, s.Extra) {
		t.Fatalf("extra lost\n got %#v\nwant %#v", again.Extra, s.Extra)
	}
	for _, k := range []string{"future_flag", "appearance.accent", "drawing.snap_aperture", "experimental"} {
		if _, ok := again.Extra[k]; !ok {
			t.Errorf("Extra[%q] missing: %#v", k, again.Extra)
		}
	}
}

func TestInvalidTOMLIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	os.WriteFile(path, []byte("[appearance\ntheme ="), 0o600)
	s, err := Load(path)
	if err == nil || !reflect.DeepEqual(s, Default()) {
		t.Fatalf("got %+v, %v", s, err)
	}
}

func TestSaveIsAtomicAndPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	path := filepath.Join(dir, "settings.toml")
	for i := 0; i < 3; i++ {
		s := Default()
		s.Theme = []string{"light", "dark", "classic"}[i]
		if err := Save(path, s); err != nil {
			t.Fatal(err)
		}
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 || ents[0].Name() != FileName {
		t.Fatalf("leftover files: %v", ents)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
			t.Errorf("file mode %v", st.Mode().Perm())
		}
		if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
			t.Errorf("dir mode %v", st.Mode().Perm())
		}
	}
	// A failed save (the target is a directory) leaves no temp file behind.
	bad := filepath.Join(t.TempDir(), "settings.toml")
	os.Mkdir(bad, 0o700)
	os.WriteFile(filepath.Join(bad, "keep"), nil, 0o600)
	if err := Save(bad, Default()); err == nil {
		t.Fatal("saving over a directory succeeded")
	}
	left, _ := os.ReadDir(filepath.Dir(bad))
	if len(left) != 1 {
		t.Fatalf("temp file left behind: %v", left)
	}
}

func TestSaveNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	s := Default()
	s.UIScale = 7
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, w, _ := LoadWarn(path)
	if got.UIScale != 2 || len(w) != 0 {
		t.Fatalf("%v %v", got.UIScale, w)
	}
}

func TestPathEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	p, err := Path()
	if err != nil || p != filepath.Join(dir, "settings.toml") {
		t.Fatalf("Path = %q, %v", p, err)
	}
}

func TestPathDefaultsToUserConfigDir(t *testing.T) {
	t.Setenv(EnvDir, "")
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CONFIG_HOME", "/xdg")
		p, err := Path()
		if err != nil || p != "/xdg/go-draw/settings.toml" {
			t.Fatalf("Path = %q, %v", p, err)
		}
	}
}
