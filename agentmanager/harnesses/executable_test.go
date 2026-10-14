package harnesses

import (
	"os"
	"testing"
)

func TestExecutablePerPlatform(t *testing.T) {
	for _, c := range []struct {
		goos, name string
		mode       os.FileMode
		want       bool
	}{
		{"linux", "/usr/bin/claude", 0o755, true},
		{"linux", "/tmp/notes", 0o644, false},
		{"darwin", "/opt/homebrew/bin/codex", 0o700, true},
		{"windows", `C:\tools\opencode.exe`, 0o644, true},
		{"windows", `C:\npm\claude.CMD`, 0o644, true},
		{"windows", `C:\npm\claude.ps1`, 0o777, false},
		{"windows", `C:\npm\claude`, 0o777, false},
	} {
		if got := executable(c.goos, c.name, c.mode); got != c.want {
			t.Errorf("executable(%s, %s, %o) = %v", c.goos, c.name, c.mode, got)
		}
	}
}
