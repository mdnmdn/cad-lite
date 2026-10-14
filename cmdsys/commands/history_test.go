package commands_test

import (
	"errors"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
)

func lineCount(s *cmdsys.Session) int { return len(entities[*doc.Line](s)) }

// threeLines draws three separate lines, one undo step each.
func threeLines(s *cmdsys.Session) {
	xfeed(s, "LINE", "0,0", "10,0", "", "LINE", "0,10", "10,10", "", "LINE", "0,20", "10,20", "")
}

func TestUndoCountAndRedoReversesTheWholeUndo(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "2")
	if n := lineCount(s); n != 1 {
		t.Fatalf("UNDO 2 left %d lines, want 1", n)
	}
	xfeed(s, "REDO")
	if n := lineCount(s); n != 3 {
		t.Fatalf("REDO after UNDO 2 gave %d lines, want 3 (V19)", n)
	}
	xfeed(s, "REDO")
	if !strings.Contains(lastLines(s, 2), "Nothing to redo") {
		t.Fatalf("second REDO: %s", lastLines(s, 2))
	}
}

func TestUndoPastTheStartSaysEverythingUndone(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "9")
	if lineCount(s) != 0 || !strings.Contains(lastLines(s, 2), "Everything has been undone") {
		t.Fatalf("lines %d, history %s", lineCount(s), lastLines(s, 2))
	}
}

func TestUndoMarkBack(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "", "UNDO", "M", "LINE", "0,10", "10,10", "", "CIRCLE", "5,5", "2", "UNDO", "B")
	if lineCount(s) != 1 || len(entities[*doc.Circle](s)) != 0 {
		t.Fatalf("UNDO Back left %d lines", lineCount(s))
	}
	xfeed(s, "REDO")
	if lineCount(s) != 2 || len(entities[*doc.Circle](s)) != 1 {
		t.Fatalf("REDO after Back: %d lines", lineCount(s))
	}
}

func TestUndoBackWithoutMarkAsksThenUndoesEverything(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "B")
	if !strings.Contains(s.Prompt(), "This will undo everything") {
		t.Fatalf("prompt %q", s.Prompt())
	}
	xfeed(s, "")
	if lineCount(s) != 0 {
		t.Fatalf("%d lines left", lineCount(s))
	}
}

func TestUndoBeginEndGroupsOneStep(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "", "UNDO", "BE", "LINE", "0,10", "10,10", "", "CIRCLE", "5,5", "1", "UNDO", "E")
	if s.Drawing().UndoDepth() != 2 {
		t.Fatalf("depth %d, want 2 (line + group)", s.Drawing().UndoDepth())
	}
	xfeed(s, "U")
	if lineCount(s) != 1 || len(entities[*doc.Circle](s)) != 0 {
		t.Fatalf("U left %d lines", lineCount(s))
	}
	xfeed(s, "UNDO", "E")
	if !strings.Contains(lastLines(s, 2), "no UNDO Begin") {
		t.Fatalf("End without Begin: %s", lastLines(s, 2))
	}
}

func TestUNamesTheUndoneCommand(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "", "U")
	if !strings.Contains(lastLines(s, 1), "LINE") || lineCount(s) != 0 {
		t.Fatalf("U output %q", lastLines(s, 1))
	}
}

func TestUndoControlNoneTurnsUndoOff(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "C", "N", "U")
	if lineCount(s) != 3 || !strings.Contains(lastLines(s, 2), "turned off") {
		t.Fatalf("%d lines, %s", lineCount(s), lastLines(s, 2))
	}
	xfeed(s, "UNDO", "C", "", "U")
	if lineCount(s) != 2 {
		t.Fatalf("after Control All: %d lines", lineCount(s))
	}
}

func TestUndoControlOneLimitsToOneStep(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "C", "O", "UNDO", "3")
	if lineCount(s) != 2 {
		t.Fatalf("%d lines, want 2", lineCount(s))
	}
}

func TestMredo(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "U", "U", "U", "MREDO", "2")
	if lineCount(s) != 2 {
		t.Fatalf("MREDO 2: %d lines", lineCount(s))
	}
	xfeed(s, "MREDO", "ALL")
	if lineCount(s) != 3 {
		t.Fatalf("MREDO All: %d lines", lineCount(s))
	}
	xfeed(s, "U", "U", "MREDO", "L")
	if lineCount(s) != 2 {
		t.Fatalf("MREDO Last: %d lines", lineCount(s))
	}
}

func TestRedoIsDiscardedByANewChange(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "2", "LINE", "0,30", "10,30", "", "REDO")
	if lineCount(s) != 2 || !strings.Contains(lastLines(s, 2), "Nothing to redo") {
		t.Fatalf("%d lines, %s", lineCount(s), lastLines(s, 2))
	}
}

func TestDBMODFollowsTheModifiedCounter(t *testing.T) {
	s := newSession(t)
	if s.Dirty() || s.Vars().Int("DBMOD") != 0 {
		t.Fatal("new drawing is dirty")
	}
	xfeed(s, "LINE", "0,0", "10,0", "")
	if !s.Dirty() || s.Vars().Int("DBMOD") != 1 || s.Modified() != 1 {
		t.Fatalf("after a line: dirty %v DBMOD %d mods %d", s.Dirty(), s.Vars().Int("DBMOD"), s.Modified())
	}
	xfeed(s, "U")
	if s.Modified() != 2 {
		t.Fatalf("undo must count: %d", s.Modified())
	}
	if err := s.Vars().SetString("DBMOD", "0"); !errors.Is(err, cmdsys.ErrVarReadOnly) {
		t.Fatalf("DBMOD is writable: %v", err)
	}
	s.MarkSaved()
	if s.Dirty() {
		t.Fatal("MarkSaved left the drawing dirty")
	}
}

// A change after an UNDO 2 discards the redo steps; a later U and REDO must redo that U alone,
// not the stale two-step batch (UNDO 2, LINE, U, U, REDO leaves one line).
func TestRedoAfterANewChangeAndUndoRedoesOneStep(t *testing.T) {
	s := newSession(t)
	threeLines(s)
	xfeed(s, "UNDO", "2", "LINE", "0,30", "10,30", "", "U", "U", "REDO")
	if lineCount(s) != 1 {
		t.Fatalf("%d lines, want 1", lineCount(s))
	}
}
