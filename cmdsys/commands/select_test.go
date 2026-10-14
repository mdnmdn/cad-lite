package commands_test

import (
	"cad-lite/cmdsys"
	"strings"
	"testing"

	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// selSession returns a session with horizontal lines at y=0 (x 0..10), y=20 (x 20..30) and a
// vertical one at x=50 (y -5..5), added in that order.
func selSession(t *testing.T) *cmdsys.Session {
	t.Helper()
	d := doc.New()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	tx.Add(&doc.Line{Start: geom.V(20, 20), End: geom.V(30, 20)})
	tx.Add(&doc.Line{Start: geom.V(50, -5), End: geom.V(50, 5)})
	tx.Commit()
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(d, &v, commands.NewRegistry())
	s.SnapOn = false
	return s
}

func feed(s *cmdsys.Session, lines ...string) {
	for _, l := range lines {
		s.Submit(l)
	}
}

func lastLines(s *cmdsys.Session, n int) string {
	h := s.History()
	if len(h) > n {
		h = h[len(h)-n:]
	}
	return strings.Join(h, "\n")
}

func TestSelectOptionsCounts(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  int
	}{
		{"all", []string{"ALL"}, 3},
		{"last", []string{"L"}, 1},
		{"window", []string{"W", "-5,-5", "35,25"}, 2},
		{"window any order", []string{"W", "35,25", "-5,-5"}, 2},
		{"crossing", []string{"C", "5,-5", "25,5"}, 1},
		{"crossing wide", []string{"C", "5,-5", "25,25"}, 2},
		{"wpolygon", []string{"WP", "-5,-5", "35,-5", "35,25", "-5,25", ""}, 2},
		{"cpolygon", []string{"CP", "45,-10", "55,-10", "55,10", "45,10", ""}, 1},
		{"fence", []string{"F", "-5,3", "55,3", ""}, 1},
		{"all then remove", []string{"ALL", "R", "5,0"}, 2},
		{"remove then add", []string{"ALL", "R", "5,0", "A", "5,0"}, 3},
		{"pick", []string{"5,0", "25,20"}, 2},
		{"auto window", []string{"-5,-5", "35,25"}, 0}, // left to right is a window; 2 selected, see below
		{"undo", []string{"L", "W", "-5,-5", "35,25", "U"}, 1},
	}
	for _, c := range cases {
		if c.name == "auto window" {
			continue
		}
		s := selSession(t)
		s.Run("SELECT")
		feed(s, c.input...)
		if got := s.Selection().Len(); got != c.want {
			t.Errorf("%s: %d selected, want %d\n%s", c.name, got, c.want, lastLines(s, 6))
		}
	}
}

func TestSelectAutoBox(t *testing.T) {
	s := selSession(t)
	s.Run("SELECT")
	feed(s, "-5,-5", "35,25") // empty space starts a box; left to right is a window
	if s.Selection().Len() != 2 {
		t.Errorf("window: %d", s.Selection().Len())
	}
	s = selSession(t)
	s.Run("SELECT")
	feed(s, "25,25", "5,-5") // right to left is a crossing
	if s.Selection().Len() != 2 {
		t.Errorf("crossing: %d", s.Selection().Len())
	}
}

func TestSelectMessages(t *testing.T) {
	s := selSession(t)
	s.Run("SELECT")
	feed(s, "L")
	if !strings.Contains(lastLines(s, 1), "1 found") {
		t.Errorf("first: %s", lastLines(s, 3))
	}
	feed(s, "W", "-5,-5", "15,5")
	if !strings.Contains(lastLines(s, 1), "1 found, 2 total") {
		t.Errorf("total: %s", lastLines(s, 3))
	}
	feed(s, "L")
	if !strings.Contains(lastLines(s, 1), "1 found, 1 duplicate, 2 total") {
		t.Errorf("duplicate: %s", lastLines(s, 3))
	}
	feed(s, "R", "ALL")
	if !strings.Contains(lastLines(s, 2), "3 found, 2 removed, 0 total") ||
		!strings.Contains(lastLines(s, 1), "1 was not in the selection set") {
		t.Errorf("remove: %s", lastLines(s, 4))
	}
	if got := s.Prompt(); !strings.HasPrefix(got, "Remove objects") {
		t.Errorf("remove prompt: %q", got)
	}
	feed(s, "xyz")
	if !strings.Contains(lastLines(s, 2), "Invalid selection") {
		t.Errorf("invalid: %s", lastLines(s, 3))
	}
}

func TestSelectPromptsAndPrevious(t *testing.T) {
	s := selSession(t)
	s.Run("SELECT")
	feed(s, "W")
	if got := s.Prompt(); got != "Specify first corner:" {
		t.Errorf("prompt %q", got)
	}
	feed(s, "0,0")
	if got := s.Prompt(); got != "Specify opposite corner:" {
		t.Errorf("prompt %q", got)
	}
	feed(s, "35,25")
	if got := s.Prompt(); !strings.HasPrefix(got, "Select objects") {
		t.Errorf("prompt %q", got)
	}
	feed(s, "") // Enter ends SELECT, the set becomes Previous
	if s.Active() != "" || s.Selection().Len() != 0 {
		t.Fatalf("active %q selected %d", s.Active(), s.Selection().Len())
	}
	s.Run("ERASE")
	feed(s, "P", "")
	if s.Drawing().Len() != 1 {
		t.Errorf("P erased wrong set: %d entities left", s.Drawing().Len())
	}
}

func TestSelectSingleEndsPrompt(t *testing.T) {
	s := selSession(t)
	s.Run("ERASE")
	feed(s, "SI", "5,0")
	if s.Active() != "" || s.Drawing().Len() != 2 {
		t.Errorf("active %q, %d entities", s.Active(), s.Drawing().Len())
	}
}

func TestSelectPolygonUndoAndEscape(t *testing.T) {
	s := selSession(t)
	s.Run("SELECT")
	feed(s, "WP", "0,0", "5,5", "U", "U", "-5,-5", "35,-5", "35,25", "-5,25", "")
	if s.Selection().Len() != 2 {
		t.Errorf("selected %d\n%s", s.Selection().Len(), lastLines(s, 6))
	}
	s.Escape()
	if s.Active() != "" {
		t.Error("escape inside a corner prompt must end the command")
	}
}

func TestSelectClickInsideCorner(t *testing.T) {
	s := selSession(t)
	s.Run("SELECT")
	feed(s, "C")
	s.Click(geom.V(5, -5), false)
	s.Click(geom.V(12, 5), false)
	if s.Selection().Len() != 1 {
		t.Errorf("clicks reach the corner prompt: %d selected", s.Selection().Len())
	}
}

// A crossing window or polygon that lies inside a text's box selects the text, as AutoCAD selects
// text by its box: inside an O, or between two letters, no glyph stroke is touched.
func TestCrossingInsideTextBoxSelectsIt(t *testing.T) {
	s := newSession(t)
	txt := doc.NewText(geom.V(0, 0), 10, 0, "O O")
	addEntities(t, s, txt)
	b := txt.Box()
	mid := b[0].Lerp(b[2], 0.5) // the space between the two Os
	s.SelectBox(mid.Add(geom.V(0.3, 0.3)), mid.Add(geom.V(-0.3, -0.3)), false)
	if s.Selection().Len() != 1 {
		t.Fatal("a crossing window between the letters did not select the text")
	}
	s.Selection().Clear()
	s.Run("SELECT")
	s.Script("CP " + cmdsys.FormatPoint(mid.Add(geom.V(-0.3, -0.3))) + " " + cmdsys.FormatPoint(mid.Add(geom.V(0.3, -0.3))) + " " +
		cmdsys.FormatPoint(mid.Add(geom.V(0, 0.3))) + "  ")
	if !historyHas(s, "1 found") {
		t.Fatalf("a crossing polygon inside the text box did not select it:\n%s", history(s))
	}
}
