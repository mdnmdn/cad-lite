package updater

import "testing"

func TestAppBundle(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Cad-lite.app/Contents/MacOS/cad": "/Applications/Cad-lite.app",
		"/usr/local/bin/cad":                            "",
		`C:\Program Files\Cad-lite\cad.exe`:             "",
	} {
		if got := AppBundle(exe); got != want {
			t.Errorf("AppBundle(%q) = %q, want %q", exe, got, want)
		}
	}
}
