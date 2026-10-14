package main

import (
	"path/filepath"
	"testing"

	"cad-lite/appenv"
)

// TestScenarios runs every scenarios/*.gui against the real window under
// Xvfb. It is gated: set CADLITE_GUI_TEST=1 (just gui-test does). Output goes
// to CADLITE_GUI_OUT, default _tmp/gui/out under the repository root.
func TestScenarios(t *testing.T) {
	if appenv.Get("GUI_TEST") != "1" {
		t.Skip("set CADLITE_GUI_TEST=1 to run the real-window scenarios")
	}
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := buildApp(root)
	if err != nil {
		t.Fatal(err)
	}
	out := appenv.Get("GUI_OUT")
	if out == "" {
		out = filepath.Join(root, "_tmp", "gui", "out")
	}
	files, _ := filepath.Glob("scenarios/*.gui")
	if len(files) == 0 {
		t.Fatal("no scenarios")
	}
	for _, f := range files {
		name := filepath.Base(f[:len(f)-len(".gui")])
		t.Run(name, func(t *testing.T) {
			if err := RunFile(f, exe, filepath.Join(out, name), false); err != nil {
				t.Fatal(err)
			}
		})
	}
}
