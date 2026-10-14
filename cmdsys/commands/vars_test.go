package commands_test

import (
	"testing"
)

// The construct commands' remembered defaults are per session (G6-1): they used to be package
// variables shared by every session in the process.
func TestConstructDefaultsArePerSession(t *testing.T) {
	a, b := newSession(t), newSession(t)
	a.Script("FILLET R 7 ")
	a.Escape()
	if got := a.Vars().Real("FILLETRAD"); got != 7 {
		t.Fatalf("session a FILLETRAD = %v, want 7", got)
	}
	if got := b.Vars().Real("FILLETRAD"); got != 0 {
		t.Fatalf("session b FILLETRAD = %v: defaults must not leak between sessions", got)
	}
	a.Script("CHAMFER D 2 3 ")
	a.Escape()
	if a.Vars().Real("CHAMFERA") != 2 || a.Vars().Real("CHAMFERB") != 3 || b.Vars().Real("CHAMFERA") != 0 {
		t.Fatalf("CHAMFERA/B = %v/%v, other session %v", a.Vars().Real("CHAMFERA"), a.Vars().Real("CHAMFERB"), b.Vars().Real("CHAMFERA"))
	}
}

func TestOffsetDistanceVariable(t *testing.T) {
	s := newSession(t)
	if s.Vars().Real("OFFSETDIST") != -1 {
		t.Fatalf("OFFSETDIST default = %v, want -1 (Through)", s.Vars().Real("OFFSETDIST"))
	}
	s.Script("OFFSET 5 ")
	s.Escape()
	if s.Vars().Real("OFFSETDIST") != 5 {
		t.Fatalf("OFFSETDIST = %v after OFFSET 5", s.Vars().Real("OFFSETDIST"))
	}
	// A variable set through SETVAR-style access is what the next OFFSET proposes.
	_ = s.Vars().Set("OFFSETDIST", 9)
	s.Run("OFFSET")
	if p := s.CurrentPrompt(); p == nil || p.Default == nil || p.Default.Value != 9 {
		t.Fatalf("OFFSET default = %+v, want 9", p)
	}
}

func TestEveryCommandFileRegistersOnce(t *testing.T) {
	s := newSession(t)
	for _, n := range []string{"LINE", "ERASE", "MIRROR", "TRIM", "AREA", "DIST", "ZOOM", "LAYER", "UNDO", "SAVE", "AI", "OPTIONS"} {
		if _, ok := s.Registry().Lookup(n); !ok {
			t.Errorf("%s is not registered", n)
		}
	}
}
