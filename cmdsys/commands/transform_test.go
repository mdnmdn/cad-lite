package commands_test

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

// feed sends each line as one response; an empty line is Enter.
func xfeed(s *cmdsys.Session, lines ...string) { s.Script(strings.Join(lines, "\n") + "\n") }

// lineSet lists the drawing's lines as "x1,y1 x2,y2" rounded to 3 decimals, sorted.
func lineSet(s *cmdsys.Session) []string {
	r := func(v float64) string {
		x := math.Round(v*1000) / 1000
		if x == 0 {
			x = 0
		}
		return fmt.Sprint(x)
	}
	var out []string
	for _, l := range entities[*doc.Line](s) {
		out = append(out, fmt.Sprintf("%s,%s %s,%s", r(l.Start.X), r(l.Start.Y), r(l.End.X), r(l.End.Y)))
	}
	sort.Strings(out)
	return out
}

func wantLines(t *testing.T, s *cmdsys.Session, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := lineSet(s); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines\n got %v\nwant %v\n%s", got, want, history(s))
	}
	if s.Active() != "" {
		t.Fatalf("command %s still running: %s", s.Active(), s.Prompt())
	}
}

func TestSelectionCountPrintedOnce(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "ERASE", "L", "")
	if n := strings.Count(history(s), "1 found"); n != 1 {
		t.Fatalf("%q printed %d times\n%s", "1 found", n, history(s))
	}
}

func TestMoveDisplacement(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "MOVE", "L", "")
	if p := s.Prompt(); p != "Specify base point or [Displacement] <Displacement>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "3,4")
	if p := s.Prompt(); p != "Specify second point or <use first point as displacement>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "") // the base point is the displacement
	wantLines(t, s, "3,4 13,4")

	xfeed(s, "MOVE", "L", "", "D")
	if p := s.Prompt(); p != "Specify displacement <0.0000, 0.0000, 0.0000>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,-10")
	wantLines(t, s, "3,-6 13,-6")

	// Enter at the base point selects Displacement; Enter again is the zero vector.
	xfeed(s, "MOVE", "L", "", "")
	xfeed(s, "")
	wantLines(t, s, "3,-6 13,-6")
}

func TestMoveIsOneUndoStep(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "MOVE", "L", "", "0,0", "5,5")
	xfeed(s, "U")
	wantLines(t, s, "0,0 10,0")
}

func TestCopyDisplacementAndMultiple(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "")
	if p := s.Prompt(); p != "Specify base point or [Displacement/mOde] <Displacement>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "D", "0,10")
	wantLines(t, s, "0,0 10,0", "0,10 10,10")

	xfeed(s, "COPY", "L", "", "0,0")
	if p := s.Prompt(); p != "Specify second point or [Array] <use first point as displacement>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,20")
	if p := s.Prompt(); p != "Specify second point or [Array/Exit/Undo] <Exit>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,30", "")
	if n := len(entities[*doc.Line](s)); n != 4 {
		t.Fatalf("%d lines, want 4\n%s", n, history(s))
	}
}

func TestCopyEnterIsDisplacement(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "", "2,3", "")
	wantLines(t, s, "0,0 10,0", "2,3 12,3")
}

func TestCopyKeepsProperties(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("walls", 3)
	s.Drawing().CurrentLayer = "walls"
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "", "0,0", "0,5", "")
	for _, l := range entities[*doc.Line](s) {
		if l.Layer != "walls" {
			t.Fatalf("copy on layer %q", l.Layer)
		}
	}
}

func TestCopyModeSingle(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "", "O")
	if p := s.Prompt(); p != "Enter a copy mode option or [Single/Multiple] <Multiple>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "S")
	if v := s.Vars().Int("COPYMODE"); v != 1 {
		t.Fatalf("COPYMODE = %d, want 1", v)
	}
	xfeed(s, "0,0", "0,10")
	wantLines(t, s, "0,0 10,0", "0,10 10,10")

	// The mode persists: the next COPY also ends after one copy; M turns it back.
	xfeed(s, "COPY", "L", "", "O", "M", "0,0", "0,20", "0,30", "")
	wantLines(t, s, "0,0 10,0", "0,10 10,10", "0,30 10,30", "0,40 10,40")
	if v := s.Vars().Int("COPYMODE"); v != 0 {
		t.Fatalf("COPYMODE = %d, want 0", v)
	}
}

func TestCopyUndoOption(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "", "0,0", "0,10", "0,20", "U")
	wantLinesRunning := lineSet(s)
	if len(wantLinesRunning) != 2 {
		t.Fatalf("after Undo: %v\n%s", wantLinesRunning, history(s))
	}
	xfeed(s, "0,30", "E")
	wantLines(t, s, "0,0 10,0", "0,10 10,10", "0,30 10,30")
	// One undo step for the whole command.
	xfeed(s, "U")
	wantLines(t, s, "0,0 10,0")
}

func TestCopyArray(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "", "0,0", "A")
	if p := s.Prompt(); p != "Enter number of items to array <2>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "1")
	if !strings.Contains(history(s), "Requires") {
		t.Fatalf("no message for 1 item\n%s", history(s))
	}
	xfeed(s, "3")
	if p := s.Prompt(); p != "Specify second point or [Fit]:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,10")
	wantLines(t, s, "0,0 10,0", "0,10 10,10", "0,20 10,20")

	xfeed(s, "COPY", "L", "", "0,0", "A", "4", "F")
	if p := s.Prompt(); p != "Specify second point or [Array]:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,9")
	if n := len(entities[*doc.Line](s)); n != 3+3 {
		t.Fatalf("%d lines, want 6\n%s", n, history(s))
	}
	// The latest line is the original one at 0,20 (Last); the fitted row spans 0,20..0,29.
	got := strings.Join(lineSet(s), "|")
	for _, w := range []string{"0,23 10,23", "0,26 10,26", "0,29 10,29"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing %s in %s", w, got)
		}
	}
}

func TestRotate(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "ROTATE")
	if h := history(s); !strings.Contains(h, "ANGDIR=counterclockwise  ANGBASE=0") {
		t.Fatalf("no header\n%s", h)
	}
	xfeed(s, "L", "", "0,0")
	if p := s.Prompt(); p != "Specify rotation angle or [Copy/Reference] <0>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "90")
	wantLines(t, s, "0,0 0,10")
}

func TestRotateCopy(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "ROTATE", "L", "", "0,0", "C")
	if !strings.Contains(history(s), "Rotating a copy of the selected objects.") {
		t.Fatalf("no copy message\n%s", history(s))
	}
	xfeed(s, "90")
	wantLines(t, s, "0,0 10,0", "0,0 0,10")
}

func TestRotateReference(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,10", "")
	xfeed(s, "ROTATE", "L", "", "0,0", "R")
	if p := s.Prompt(); p != "Specify the reference angle <0>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "45")
	if p := s.Prompt(); p != "Specify the new angle or [Points] <0>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "90")
	wantLines(t, s, "0,0 0,14.142")

	// Two points give the reference angle; the new angle may be two points too.
	s = newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "ROTATE", "L", "", "0,0", "R", "5,5", "5,15") // reference 90 degrees
	xfeed(s, "P", "0,0", "-10,0")                          // new 180 degrees
	wantLines(t, s, "0,0 0,10")
}

func TestRotateReferenceThenCopy(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "ROTATE", "L", "", "0,0", "C", "R", "0", "30")
	wantLines(t, s, "0,0 10,0", "0,0 8.66,5")
}

func TestScale(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "SCALE", "L", "", "0,0")
	if p := s.Prompt(); p != "Specify scale factor or [Copy/Reference] <1>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0")
	if !strings.Contains(history(s), "Requires a positive and nonzero value.") || s.Active() != "SCALE" {
		t.Fatalf("zero factor accepted\n%s", history(s))
	}
	xfeed(s, "2")
	wantLines(t, s, "0,0 20,0")
}

func TestScaleCopyAndReference(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "SCALE", "L", "", "0,0", "C")
	if !strings.Contains(history(s), "Scaling a copy of the selected objects.") {
		t.Fatalf("no copy message\n%s", history(s))
	}
	xfeed(s, "3")
	wantLines(t, s, "0,0 10,0", "0,0 30,0")

	s = newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "SCALE", "L", "", "0,0", "R")
	if p := s.Prompt(); p != "Specify reference length <1>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "10")
	if p := s.Prompt(); p != "Specify new length or [Points] <1>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "25")
	wantLines(t, s, "0,0 25,0")

	// Reference by two points, new length by two points; a zero reference is refused.
	s = newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "SCALE", "L", "", "0,0", "R", "0")
	if !strings.Contains(history(s), "Requires a positive and nonzero value.") {
		t.Fatalf("zero reference accepted\n%s", history(s))
	}
	xfeed(s, "1,1", "1,6")       // reference length 5
	xfeed(s, "P", "0,0", "0,20") // new length 20
	wantLines(t, s, "0,0 40,0")
}

func TestScaleClickSetsLastPoint(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "SCALE", "L", "", "0,0")
	s.Click(geom.V(3, 4), false)
	wantLines(t, s, "0,0 50,0")
	if p := s.Vars().Point("LASTPOINT"); !p.Near(geom.V(3, 4), 1e-9) {
		t.Fatalf("LASTPOINT = %v", p)
	}
}

func TestArrayRectangular(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "1,0", "")
	xfeed(s, "-ARRAY", "L", "")
	if p := s.Prompt(); p != "Enter the type of array or [Rectangular/Polar] <R>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "R")
	if p := s.Prompt(); p != "Enter the number of rows (---) <1>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "2", "3")
	if p := s.Prompt(); p != "Enter the distance between rows or specify unit cell (---):" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "10")
	if p := s.Prompt(); p != "Specify the distance between columns (|||):" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "-20")
	wantLines(t, s, "0,0 1,0", "-20,0 -19,0", "-40,0 -39,0", "0,10 1,10", "-20,10 -19,10", "-40,10 -39,10")
	xfeed(s, "U")
	wantLines(t, s, "0,0 1,0")
}

func TestArrayUnitCellAndOneRow(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "1,0", "")
	xfeed(s, "-ARRAY", "L", "", "", "2", "2", "0,0")
	if p := s.Prompt(); p != "Specify opposite corner of unit cell:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "5,7")
	wantLines(t, s, "0,0 1,0", "5,0 6,0", "0,7 1,7", "5,7 6,7")

	// One row: only the column distance is asked.
	s = newSession(t)
	xfeed(s, "LINE", "0,0", "1,0", "")
	xfeed(s, "-ARRAY", "L", "", "R", "1", "3", "4")
	wantLines(t, s, "0,0 1,0", "4,0 5,0", "8,0 9,0")
}

func TestArrayPolar(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "10,100", "11,100", "")
	xfeed(s, "-ARRAY", "L", "", "P")
	if p := s.Prompt(); p != "Specify center point of array or [Base]:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,100")
	if p := s.Prompt(); p != "Enter the number of items in the array:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "4")
	if p := s.Prompt(); p != "Specify the angle to fill (+=ccw, -=cw) <360>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "")
	if p := s.Prompt(); !strings.HasPrefix(p, "Rotate arrayed objects as they are copied? or [Yes/No] <Y>") {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "")
	wantLines(t, s, "10,100 11,100", "0,110 0,111", "-10,100 -11,100", "0,90 0,89")
}

func TestArrayPolarPartialFillNoRotate(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "10,0", "12,0", "")
	// 3 items over 180 degrees about the origin, objects keep their orientation; the base point
	// is the selection's centre (11,0).
	xfeed(s, "-ARRAY", "L", "", "P", "0,0", "3", "180", "N")
	wantLines(t, s, "10,0 12,0", "-1,11 1,11", "-12,0 -10,0")
	// A negative fill goes clockwise.
	s = newSession(t)
	xfeed(s, "LINE", "10,0", "11,0", "")
	xfeed(s, "-ARRAY", "L", "", "P", "0,0", "2", "-90", "Y")
	wantLines(t, s, "10,0 11,0", "0,-10 0,-11")
}

func TestArrayCommands(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "1,0", "")
	xfeed(s, "ARRAYRECT", "L", "")
	if !strings.Contains(history(s), "Type = Rectangular  Associative = No") {
		t.Fatalf("no type line\n%s", history(s))
	}
	xfeed(s, "1", "2", "5")
	wantLines(t, s, "0,0 1,0", "5,0 6,0")

	s = newSession(t)
	xfeed(s, "LINE", "0,0", "1,0", "")
	xfeed(s, "AR", "L", "")
	if p := s.Prompt(); p != "Enter array type or [Rectangular/PAth/Polar] <Rectangular>:" {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "PA")
	if !strings.Contains(history(s), "ARRAYPATH is not supported") || s.Active() != "ARRAY" {
		t.Fatalf("path accepted\n%s", history(s))
	}
	xfeed(s, "P", "0,5", "2", "180", "Y")
	wantLines(t, s, "0,0 1,0", "0,10 -1,10")
}

func TestOops(t *testing.T) {
	s := newSession(t)
	xfeed(s, "OOPS")
	if !strings.Contains(history(s), "Nothing to restore.") {
		t.Fatalf("no message\n%s", history(s))
	}
	s.Drawing().AddLayer("walls", 3)
	s.Drawing().CurrentLayer = "walls"
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "CIRCLE", "20,0", "5")
	xfeed(s, "ERASE", "ALL", "")
	if s.Drawing().Len() != 0 {
		t.Fatalf("%d entities left", s.Drawing().Len())
	}
	xfeed(s, "LINE", "0,50", "10,50", "") // other commands in between do not matter
	xfeed(s, "OOPS")
	if s.Drawing().Len() != 3 {
		t.Fatalf("%d entities after OOPS, want 3", s.Drawing().Len())
	}
	if got := len(entities[*doc.Circle](s)); got != 1 {
		t.Fatalf("%d circles restored", got)
	}
	for _, l := range entities[*doc.Line](s) {
		if l.Layer != "walls" {
			t.Fatalf("restored line on %q", l.Layer)
		}
	}
	// OOPS restores once; it is one undo step.
	xfeed(s, "OOPS")
	if s.Drawing().Len() != 3 {
		t.Fatalf("second OOPS changed the drawing: %d", s.Drawing().Len())
	}
	xfeed(s, "U")
	if s.Drawing().Len() != 1 {
		t.Fatalf("undo of OOPS left %d entities, want 1", s.Drawing().Len())
	}
}

func TestTransformPromptsInItalian(t *testing.T) {
	italian(t)
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "COPY", "L", "")
	if p := s.Prompt(); !strings.Contains(p, "Specificare punto base o [Spostamento/mOdalità] <Spostamento>") {
		t.Fatalf("prompt %q", p)
	}
	xfeed(s, "0,0", "0,5", "")
	if len(lineSet(s)) != 2 {
		t.Fatalf("copy failed\n%s", history(s))
	}
	xfeed(s, "RO", "L", "", "0,0", "Riferimento")
	if p := s.Prompt(); !strings.Contains(p, "angolo di riferimento") {
		t.Fatalf("prompt %q", p)
	}
	s.Escape()
	xfeed(s, "-ARRAY", "L", "", "Polare")
	if p := s.Prompt(); !strings.Contains(p, "centro della serie") {
		t.Fatalf("prompt %q", p)
	}
	s.Escape()
}
