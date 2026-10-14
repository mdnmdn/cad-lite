package commands_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/doc"
	"go-draw/geom"
)

func TestOffsetSettingsLineAndEraseOption(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Submit("OFFSET")
	if !strings.Contains(history(s), "Current settings: Erase source=No  Layer=Source  OFFSETGAPTYPE=0") {
		t.Fatalf("settings line missing\n%s", history(s))
	}
	s.Submit("E")
	if got := s.Prompt(); got != "Erase source object after offsetting? [Yes/No] <No>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("Y")
	if !strings.HasPrefix(s.Prompt(), "Specify offset distance") {
		t.Fatalf("Erase did not return to the distance prompt: %q", s.Prompt())
	}
	s.Script("10 50,0 50,5 ")
	ls := entities[*doc.Line](s)
	if len(ls) != 1 {
		t.Fatalf("lines = %d, want only the offset\n%s", len(ls), history(s))
	}
	near(t, "offset", ls[0].Start, geom.V(0, 10))
	if got := s.Vars().Int("OFFSETERASE"); got != 1 {
		t.Fatalf("OFFSETERASE = %d", got)
	}
	s.Submit("U")
	if l := entities[*doc.Line](s); len(l) != 1 || l[0].Start != geom.V(0, 0) {
		t.Fatalf("undo did not restore the source: %+v", l)
	}
}

func TestOffsetLayerOption(t *testing.T) {
	s := newSession(t)
	s.Drawing().CurrentLayer = "cur"
	addEntities(t, s, &doc.Line{EntityCommon: doc.EntityCommon{Layer: "src"}, Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Submit("OFFSET")
	s.Submit("L")
	if got := s.Prompt(); got != "Enter layer option for offset objects [Current/Source] <Source>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("C")
	s.Script("10 50,0 50,5 ")
	if ls := entities[*doc.Line](s); ls[1].Layer != "cur" {
		t.Fatalf("offset layer = %q, want the current layer", ls[1].Layer)
	}
	s.Script("OFFSET L S 10 50,0 50,5 ")
	if ls := entities[*doc.Line](s); ls[len(ls)-1].Layer != "src" {
		t.Fatalf("offset layer = %q, want the source layer", ls[len(ls)-1].Layer)
	}
}

func TestOffsetMultipleAndUndo(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("OFFSET 10 50,0 M")
	if got := s.Prompt(); got != "Specify point on side to offset or [Exit/Undo] <next object>:" {
		t.Fatalf("prompt = %q", got)
	}
	// Each new offset is taken from the previous one.
	s.Script("50,5 50,15 50,25")
	ls := entities[*doc.Line](s)
	if len(ls) != 4 {
		t.Fatalf("lines = %d\n%s", len(ls), history(s))
	}
	for i, y := range []float64{10, 20, 30} {
		near(t, "multiple", ls[i+1].Start, geom.V(0, y))
	}
	s.Submit("U")
	if len(entities[*doc.Line](s)) != 3 || !strings.HasPrefix(s.Prompt(), "Specify point on side") {
		t.Fatalf("Undo in Multiple: %d lines, prompt %q", len(entities[*doc.Line](s)), s.Prompt())
	}
	s.Submit("")
	if !strings.HasPrefix(s.Prompt(), "Select object to offset") {
		t.Fatalf("Enter in Multiple did not return to the select prompt: %q", s.Prompt())
	}
	s.Submit("U")
	if len(entities[*doc.Line](s)) != 2 {
		t.Fatalf("Undo at the select prompt: %d lines", len(entities[*doc.Line](s)))
	}
	s.Submit("")
	if s.Active() != "" {
		t.Fatalf("still active %s", s.Active())
	}
	s.Submit("U")
	if s.Drawing().Len() != 1 {
		t.Fatalf("OFFSET is not one undo step: %d entities", s.Drawing().Len())
	}
}

func TestOffsetThroughResetsOffsetDist(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("OFFSET 10 ")
	if got := s.Vars().Real("OFFSETDIST"); got != 10 {
		t.Fatalf("OFFSETDIST = %v", got)
	}
	s.Script("OFFSET T ")
	if got := s.Vars().Real("OFFSETDIST"); got != -1 {
		t.Fatalf("OFFSETDIST after Through = %v, want -1", got)
	}
	s.Submit("OFFSET")
	if got := s.Prompt(); got != "Specify offset distance or [Through/Erase/Layer] <Through>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Escape()
	s.Script("OFFSET 0")
	if !strings.Contains(history(s), "Requires a positive and nonzero value.") {
		t.Fatalf("zero distance wording\n%s", history(s))
	}
	s.Escape()
}

func TestOffsetKeepsPolylineWidths(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0)}, Widths: [][2]float64{{2, 4}}})
	s.Script("OFFSET 10 50,0 50,5 ")
	ps := entities[*doc.LWPolyline](s)
	if len(ps) != 2 || len(ps[1].Widths) != 1 || ps[1].Widths[0] != [2]float64{2, 4} {
		t.Fatalf("offset polyline widths = %+v", ps)
	}
}

func TestFilletPolylineRoundsEveryVertex(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{
		Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100), geom.V(0, 100)}, Closed: true,
		Widths: [][2]float64{{2, 2}, {4, 4}, {2, 2}, {4, 4}},
	})
	s.Script("FILLET R 10 P 50,0")
	if !strings.Contains(history(s), "4 lines were filleted") || s.Active() != "" {
		t.Fatalf("active %q\n%s", s.Active(), history(s))
	}
	p := entities[*doc.LWPolyline](s)[0]
	if len(p.Points) != 8 || !p.Closed {
		t.Fatalf("polyline = %+v", p)
	}
	near(t, "first", p.Points[0], geom.V(10, 0))
	near(t, "arc start", p.Points[1], geom.V(90, 0))
	near(t, "arc end", p.Points[2], geom.V(100, 10))
	if math.Abs(p.Bulges[1]-math.Tan(math.Pi/8)) > 1e-9 || p.Bulges[0] != 0 {
		t.Fatalf("bulges = %v", p.Bulges)
	}
	// The arc piece blends the widths of the two segments it joins.
	if p.Widths[1] != [2]float64{2, 4} || p.Widths[0] != [2]float64{2, 2} {
		t.Fatalf("widths = %v", p.Widths)
	}
	s.Submit("U")
	if q := entities[*doc.LWPolyline](s)[0]; len(q.Points) != 4 {
		t.Fatalf("undo did not restore the polyline: %+v", q)
	}
}

func TestFilletPolylineOpenAndTooShort(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100)}})
	s.Script("FILLET P R 10 50,0")
	p := entities[*doc.LWPolyline](s)[0]
	if len(p.Points) != 4 || p.Closed {
		t.Fatalf("polyline = %+v", p)
	}
	near(t, "end", p.Points[3], geom.V(100, 100))
	s = newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100)}})
	s.Script("FILLET P R 150 50,0")
	if !strings.Contains(history(s), "0 lines were filleted") || !strings.Contains(history(s), "1 was too short") {
		t.Fatalf("too-large radius\n%s", history(s))
	}
}

func TestFilletNoTrimMultipleAndUndo(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
		&doc.Line{Start: geom.V(200, 0), End: geom.V(300, 0)},
		&doc.Line{Start: geom.V(200, 0), End: geom.V(200, 100)},
	)
	s.Submit("FILLET")
	s.Submit("T")
	if got := s.Prompt(); got != "Enter Trim mode option [Trim/No trim] <Trim>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("N")
	if got := s.Vars().Int("TRIMMODE"); got != 0 {
		t.Fatalf("TRIMMODE = %d", got)
	}
	s.Script("R 10 M 50,0 0,50")
	if s.Active() != "FILLET" {
		t.Fatalf("Multiple ended the command")
	}
	if got := len(entities[*doc.Arc](s)); got != 1 {
		t.Fatalf("arcs = %d", got)
	}
	if ls := entities[*doc.Line](s); ls[0].End != geom.V(100, 0) || ls[1].End != geom.V(0, 100) {
		t.Fatalf("No trim changed the lines: %+v", ls)
	}
	s.Submit("U")
	if len(entities[*doc.Arc](s)) != 0 {
		t.Fatal("Undo did not remove the arc")
	}
	s.Script("T T 250,0 200,50 ")
	if s.Active() != "" || len(entities[*doc.Arc](s)) != 1 {
		t.Fatalf("active %q arcs %d\n%s", s.Active(), len(entities[*doc.Arc](s)), history(s))
	}
	near(t, "trimmed", entities[*doc.Line](s)[2].Start, geom.V(210, 0))
}

func TestChamferAngleMethodAndPolyline(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Submit("CHAMFER")
	s.Submit("A")
	if got := s.Prompt(); got != "Specify chamfer length on the first line <0>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("10")
	if got := s.Prompt(); got != "Specify chamfer angle from the first line <0>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("30")
	if s.Vars().Int("CHAMMODE") != 1 || s.Vars().Real("CHAMFERC") != 10 || math.Abs(s.Vars().Real("CHAMFERD")-30) > 1e-9 {
		t.Fatalf("variables: %d %v %v", s.Vars().Int("CHAMMODE"), s.Vars().Real("CHAMFERC"), s.Vars().Real("CHAMFERD"))
	}
	s.Script("50,0 0,50")
	b := entities[*doc.Line](s)[2]
	near(t, "bevel a", b.Start, geom.V(10, 0))
	near(t, "bevel b", b.End, geom.V(0, 10*math.Sin(math.Pi/6)/math.Sin(math.Pi*2/3)))
	// Method back to Distance, then a whole polyline.
	s.Submit("CHAMFER")
	s.Submit("E")
	if got := s.Prompt(); got != "Enter trim method [Distance/Angle] <Angle>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("D")
	if s.Vars().Int("CHAMMODE") != 0 {
		t.Fatal("Method Distance did not set CHAMMODE 0")
	}
	s.Escape()
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(500, 0), geom.V(600, 0), geom.V(600, 100)}})
	s.Script("CHAMFER D 10 20 P 550,0")
	if !strings.Contains(history(s), "1 line was chamfered") {
		t.Fatalf("history\n%s", history(s))
	}
	ps := entities[*doc.LWPolyline](s)
	p := ps[len(ps)-1]
	if len(p.Points) != 4 {
		t.Fatalf("polyline = %+v", p)
	}
	near(t, "cut a", p.Points[1], geom.V(590, 0))
	near(t, "cut b", p.Points[2], geom.V(600, 20))
}

func TestChamferNoTrimKeepsLines(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("CHAMFER T N D 10 10 50,0 0,50")
	ls := entities[*doc.Line](s)
	if len(ls) != 3 || ls[0].Start != geom.V(0, 0) {
		t.Fatalf("lines = %+v", ls)
	}
	s.Submit("CHAMFER")
	if !strings.Contains(history(s), "(NOTRIM mode) Current chamfer Dist1 = 10, Dist2 = 10") {
		t.Fatalf("settings line\n%s", history(s))
	}
	s.Escape()
}
