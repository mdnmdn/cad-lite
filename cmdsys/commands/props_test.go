package commands_test

import (
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

// twoLines draws a line on layer A and one on layer B (colour 1) and returns their handles.
func twoLines(t *testing.T) (s *cmdsys.Session, a, b doc.Handle) {
	t.Helper()
	s = newSession(t)
	d := s.Drawing()
	d.AddLayer("A", 3)
	d.AddLayer("B", 5)
	d.AddLayer("X", 6)
	tx, _ := d.Begin()
	a = tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "A"}, Start: geom.V(0, 0), End: geom.V(10, 0)})
	b = tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "B", Color: 1}, Start: geom.V(0, 5), End: geom.V(10, 5)})
	tx.Commit()
	return s, a, b
}

func TestMatchPropCopiesColorAndLayer(t *testing.T) {
	s, a, b := twoLines(t)
	s.Script("MA\n5,0\n5,5\n\n")
	if s.Active() != "" {
		t.Fatalf("still in %s", s.Active())
	}
	ea, eb := s.Drawing().Entity(a).Common(), s.Drawing().Entity(b).Common()
	if eb.Layer != "A" || eb.Color != doc.ByLayer {
		t.Fatalf("destination = %+v, want layer A and ByLayer", *eb)
	}
	if ea.Layer != "A" {
		t.Fatalf("source changed: %+v", *ea)
	}
	s.Drawing().Undo()
	if got := s.Drawing().Entity(b).Common(); got.Layer != "B" || got.Color != 1 {
		t.Fatalf("one undo step must restore the destination: %+v", *got)
	}
}

func TestMatchPropSettingsLimitWhatIsCopied(t *testing.T) {
	s, _, b := twoLines(t)
	s.Script("MATCHPROP\n5,0\nS\nLA\n\n5,5\n\n")
	eb := s.Drawing().Entity(b).Common()
	if eb.Layer != "B" || eb.Color != doc.ByLayer {
		t.Fatalf("with Layer switched off only the colour is copied: %+v", *eb)
	}
	if !strings.Contains(history(s), "Current active settings = Color") {
		t.Fatalf("settings not reported:\n%s", history(s))
	}
}

func TestMatchPropSkipsLockedDestination(t *testing.T) {
	s, _, b := twoLines(t)
	s.Drawing().Layer("B").Locked = true
	s.Script("MA\n5,0\n5,5\n\n")
	// A locked object cannot be selected, so nothing changes and nothing is half done.
	if got := s.Drawing().Entity(b).Common(); got.Layer != "B" {
		t.Fatalf("locked destination changed: %+v", *got)
	}
}

func TestChpropColorAndLayer(t *testing.T) {
	s, a, _ := twoLines(t)
	s.Script("CHPROP\n5,0\n\nLA\nX\nC\n1\n\n")
	if s.Active() != "" {
		t.Fatalf("still in %s", s.Active())
	}
	if c := s.Drawing().Entity(a).Common(); c.Layer != "X" || c.Color != 1 {
		t.Fatalf("a = %+v", *c)
	}
	s.Script("CHPROP\n5,0\n\nC\nBYLAYER\n\n")
	if c := s.Drawing().Entity(a).Common(); c.Color != doc.ByLayer {
		t.Fatalf("BYLAYER: %+v", *c)
	}
	s.Script("CHPROP\n5,0\n\nLA\nNOSUCH\n\n")
	if c := s.Drawing().Entity(a).Common(); c.Layer != "X" {
		t.Fatalf("a missing layer must be refused: %+v", *c)
	}
}

func TestColorSetsTheColourOfNewObjects(t *testing.T) {
	s := newSession(t)
	s.Script("-COLOR\nred\n")
	s.Script("LINE\n0,0\n10,0\n\n")
	ls := entities[*doc.Line](s)
	if len(ls) != 1 || ls[0].Color != 1 {
		t.Fatalf("new line colour = %v", ls[0].Color)
	}
	if got := s.Vars().Text("CECOLOR"); got != "1" {
		t.Fatalf("CECOLOR = %q", got)
	}
	s.Script("-COLOR\nBYLAYER\n")
	s.Script("LINE\n0,5\n10,5\n\n")
	if ls := entities[*doc.Line](s); ls[1].Color != doc.ByLayer {
		t.Fatalf("BYLAYER line colour = %v", ls[1].Color)
	}
	if err := s.Vars().SetString("CECOLOR", "3"); err != nil || s.Drawing().CurrentColor != 3 {
		t.Fatalf("SETVAR CECOLOR: %v %d", err, s.Drawing().CurrentColor)
	}
}

func TestLtscaleSetsTheVariable(t *testing.T) {
	s := newSession(t)
	s.Script("LTSCALE\n2.5\n")
	if got := s.Vars().Real("LTSCALE"); got != 2.5 {
		t.Fatalf("LTSCALE = %v", got)
	}
}

func TestPurgeLayersAndZeroLength(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	d.AddLayer("USED", 1)
	d.AddLayer("EMPTY1", 2)
	d.AddLayer("EMPTY2", 3)
	tx, _ := d.Begin()
	tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "USED"}, Start: geom.V(0, 0), End: geom.V(10, 0)})
	tx.Add(&doc.Line{Start: geom.V(3, 3), End: geom.V(3, 3)})
	tx.Add(&doc.Circle{Center: geom.V(1, 1), Radius: 0})
	tx.Commit()
	s.Script("-PURGE\nZ\n")
	if n := len(entities[*doc.Line](s)) + len(entities[*doc.Circle](s)); n != 1 {
		t.Fatalf("%d objects left after purging zero-length geometry", n)
	}
	s.Script("-PURGE\nLA\nEMPTY*\nN\n")
	if d.Layer("EMPTY1") != nil || d.Layer("EMPTY2") != nil || d.Layer("USED") == nil || d.Layer("0") == nil {
		t.Fatalf("layers after purge: %v", layerNames(d))
	}
	// With verification each layer is asked about.
	d.AddLayer("E3", 1)
	s.Script("-PURGE\nLA\n*\n\n")
	if !strings.Contains(s.Prompt(), "E3") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("Y")
	if d.Layer("E3") != nil {
		t.Fatal("E3 should be purged")
	}
}

func layerNames(d *doc.Drawing) []string {
	var out []string
	for _, l := range d.Layers() {
		out = append(out, l.Name)
	}
	return out
}
