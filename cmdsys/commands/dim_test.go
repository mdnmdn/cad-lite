package commands_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

func dims(s *cmdsys.Session) []*doc.Dimension { return entities[*doc.Dimension](s) }

// submitAll answers the prompts one by one; unlike Script, an answer may hold spaces.
func submitAll(s *cmdsys.Session, answers ...string) {
	for _, a := range answers {
		s.Submit(a)
	}
}

func oneDim(t *testing.T, s *cmdsys.Session) *doc.Dimension {
	t.Helper()
	ds := dims(s)
	if len(ds) != 1 {
		t.Fatalf("dimensions = %d, want 1; history:\n%s", len(ds), history(s))
	}
	if s.Active() != "" {
		t.Fatalf("command still active at %q", s.Prompt())
	}
	return ds[0]
}

func closeTo(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestDimLinearPromptsAndHorizontal(t *testing.T) {
	s := newSession(t)
	s.Run("DIMLINEAR")
	if want := "Specify first extension line origin or <select object>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q, want %q", s.Prompt(), want)
	}
	s.Submit("0,0")
	if want := "Specify second extension line origin:"; s.Prompt() != want {
		t.Fatalf("prompt = %q, want %q", s.Prompt(), want)
	}
	s.Submit("120,30")
	if want := "Specify dimension line location or [Mtext/Text/Angle/Horizontal/Vertical/Rotated]:"; s.Prompt() != want {
		t.Fatalf("prompt = %q, want %q", s.Prompt(), want)
	}
	if n := previewItems(s, geom.V(60, 50)); n == 0 {
		t.Fatal("no preview of the dimension")
	}
	s.Submit("60,50") // above the box of the origins: horizontal
	d := oneDim(t, s)
	if d.Kind != doc.DimRotated || d.Rotation != 0 {
		t.Fatalf("kind %v rotation %v, want rotated 0", d.Kind, d.Rotation)
	}
	near(t, "Ext1", d.Ext1, geom.V(0, 0))
	near(t, "Ext2", d.Ext2, geom.V(120, 30))
	closeTo(t, "dimension line y", d.DimLine.Y, 50)
	if d.MeasurementText() != "120" {
		t.Fatalf("text = %q", d.MeasurementText())
	}
	if !strings.Contains(history(s), "Dimension text = 120") {
		t.Fatalf("no echo:\n%s", history(s))
	}
	if d.Style() != doc.DefaultDimStyle() {
		t.Fatalf("style = %+v, want the ISO-25 defaults", d.Style())
	}
	s.Submit("U")
	if len(dims(s)) != 0 {
		t.Fatal("U did not remove the dimension")
	}
}

func TestDimLinearAutoVerticalAndLocks(t *testing.T) {
	s := newSession(t)
	s.Script("DLI 0,0 120,30 150,10")
	d := oneDim(t, s)
	closeTo(t, "rotation", d.Rotation, math.Pi/2) // right of the box: vertical
	if d.MeasurementText() != "30" {
		t.Fatalf("text = %q", d.MeasurementText())
	}

	s = newSession(t)
	s.Script("DLI 0,0 120,30 V 60,80")
	d = oneDim(t, s)
	closeTo(t, "locked vertical", d.Rotation, math.Pi/2)

	s = newSession(t)
	s.Script("DLI 0,0 120,30 H 150,10")
	d = oneDim(t, s)
	closeTo(t, "locked horizontal", d.Rotation, 0)
	closeTo(t, "measure", d.Measurement(), 120)
}

func TestDimLinearRotatedTextAngle(t *testing.T) {
	s := newSession(t)
	s.Run("DIMLINEAR")
	submitAll(s, "0,0", "10,10", "R")
	if want := "Specify angle of dimension line <0>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q, want %q", s.Prompt(), want)
	}
	s.Submit("45")
	s.Submit("T")
	if want := "Enter dimension text <"; !strings.HasPrefix(s.Prompt(), want) {
		t.Fatalf("prompt = %q, want prefix %q", s.Prompt(), want)
	}
	s.Submit("L = <> mm")
	s.Submit("A")
	if want := "Specify angle of dimension text:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "30", "0,20")
	d := oneDim(t, s)
	closeTo(t, "rotation", d.Rotation, math.Pi/4)
	closeTo(t, "text angle", d.TextAngle, math.Pi/6)
	if d.Text != "L = <> mm" || d.MeasurementText() != "L = 14,14 mm" {
		t.Fatalf("text %q -> %q", d.Text, d.MeasurementText())
	}
	if !strings.Contains(history(s), "Dimension text = 14,14") {
		t.Fatalf("echo:\n%s", history(s))
	}
}

func TestDimTextEnterKeepsMeasurementAndMtext(t *testing.T) {
	s := newSession(t)
	s.Run("DAL")
	submitAll(s, "0,0", "30,40", "M")
	if want := "Enter dimension text <50>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q, want %q", s.Prompt(), want)
	}
	submitAll(s, "", "0,100")
	if d := oneDim(t, s); d.Text != "" {
		t.Fatalf("Enter set text %q", d.Text)
	}
}

func TestDimStyleFromVariables(t *testing.T) {
	s := newSession(t)
	s.Script("DIMSCALE 10 DIMASZ 3 DIMTXT 3.5 DIMEXO 1 DIMEXE 2 DIMGAP 0.5 DIMDEC 3 DIMDSEP . DIMZIN 0 DIMTAD 0")
	s.Script("DLI 0,0 10,0 5,5")
	d := oneDim(t, s)
	want := doc.DefaultDimStyle()
	want.Scale, want.ArrowSize, want.TextHeight, want.ExtOffset, want.ExtExtend, want.Gap = 10, 3, 3.5, 1, 2, 0.5
	want.Decimals, want.DecSep, want.ZeroSupp, want.TextVertical = 3, ".", 0, 0
	if d.Style() != want {
		t.Fatalf("style = %+v, want %+v", d.Style(), want)
	}
	if d.MeasurementText() != "10.000" {
		t.Fatalf("text = %q", d.MeasurementText())
	}
	if got := s.Vars().Text("DIMSTYLE"); got != "ISO-25" {
		t.Fatalf("DIMSTYLE = %q", got)
	}
	if d.Overrides.Set != 0 {
		t.Fatalf("the DIM* variables are the current style's record: overrides %b", d.Overrides.Set)
	}
	if st, _ := s.Drawing().DimStyle("ISO-25"); st != want {
		t.Fatalf("record = %+v", st)
	}
	if err := s.Vars().Set("DIMSTYLE", "X"); !errors.Is(err, cmdsys.ErrVarInvalid) {
		t.Fatalf("DIMSTYLE set to a missing style: err = %v", err)
	}
	if err := s.Vars().Set("DIMDEC", 9); !errors.Is(err, cmdsys.ErrVarRange) {
		t.Fatalf("DIMDEC 9 err = %v", err)
	}
}

func TestDimAligned(t *testing.T) {
	s := newSession(t)
	s.Run("DIMALIGNED")
	submitAll(s, "0,0", "30,40")
	if want := "Specify dimension line location or [Mtext/Text/Angle]:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("-8,6")
	d := oneDim(t, s)
	if d.Kind != doc.DimAligned || d.MeasurementText() != "50" {
		t.Fatalf("kind %v text %q", d.Kind, d.MeasurementText())
	}
	// The dimension line passes through the picked location, parallel to the origins.
	if dist := d.DimLine.Sub(geom.V(30, 40)).Len(); math.Abs(dist-10) > 1e-9 {
		t.Fatalf("offset = %v, want 10", dist)
	}
}

func TestDimSelectObject(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(40, 0)},
		&doc.LWPolyline{Points: []geom.Vec2{{X: 0, Y: 100}, {X: 50, Y: 100}, {X: 50, Y: 130}}},
		&doc.Circle{Center: geom.V(200, 0), Radius: 10},
		&doc.Arc{Center: geom.V(300, 0), Radius: 20, StartAngle: 0, EndAngle: math.Pi},
	)
	s.Run("DLI")
	s.Submit("")
	if want := "Select object to dimension:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "20,0", "20,-10")
	d := dims(s)[0]
	near(t, "line Ext1", d.Ext1, geom.V(0, 0))
	near(t, "line Ext2", d.Ext2, geom.V(40, 0))

	submitAll(s, "DLI", "", "50,120", "70,115") // the second polyline segment, vertical
	d = dims(s)[1]
	near(t, "segment Ext1", d.Ext1, geom.V(50, 100))
	near(t, "segment Ext2", d.Ext2, geom.V(50, 130))
	if d.MeasurementText() != "30" {
		t.Fatalf("segment text %q", d.MeasurementText())
	}

	submitAll(s, "DLI", "", "210,0", "220,0") // circle, vertical diameter
	d = dims(s)[2]
	near(t, "circle Ext1", d.Ext1, geom.V(200, -10))
	near(t, "circle Ext2", d.Ext2, geom.V(200, 10))
	if d.MeasurementText() != "20" {
		t.Fatalf("circle text %q", d.MeasurementText())
	}

	submitAll(s, "DAL", "", "300,20", "300,40") // arc ends
	d = dims(s)[3]
	if d.Kind != doc.DimAligned {
		t.Fatalf("kind %v", d.Kind)
	}
	near(t, "arc Ext1", d.Ext1, geom.V(320, 0))
	near(t, "arc Ext2", d.Ext2, geom.V(280, 0))

	// Nothing there, then a dimension: both re-prompt.
	submitAll(s, "DLI", "", "500,500")
	if !strings.Contains(history(s), "No object found.") || s.Active() != "DIMLINEAR" {
		t.Fatalf("empty pick: active %q\n%s", s.Active(), history(s))
	}
	s.Submit(cmdsys.FormatPoint(dims(s)[0].Layout().TextBox[0]))
	if !strings.Contains(history(s), "Object selected is not a line, arc, circle or polyline segment.") {
		t.Fatalf("dimension pick:\n%s", history(s))
	}
	s.Escape()
}

func TestDimContinueChain(t *testing.T) {
	s := newSession(t)
	s.Script("DLI 0,0 10,0 5,20")
	s.Run("DCO")
	if want := "Specify a second extension line origin or [Select/Undo] <Select>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	if previewItems(s, geom.V(30, 5)) == 0 {
		t.Fatal("no preview")
	}
	submitAll(s, "30,5", "45,-3", "U", "60,0")
	ds := dims(s)
	if len(ds) != 3 {
		t.Fatalf("dimensions = %d, want 3", len(ds))
	}
	for i, d := range ds[1:] {
		if d.Kind != doc.DimRotated || d.Rotation != 0 {
			t.Fatalf("dim %d: kind %v rotation %v", i, d.Kind, d.Rotation)
		}
		closeTo(t, "dimension line", d.DimLine.Y, 20)
	}
	near(t, "first continued Ext1", ds[1].Ext1, geom.V(10, 0))
	near(t, "after Undo Ext1", ds[2].Ext1, geom.V(30, 5))
	if ds[2].MeasurementText() != "30" {
		t.Fatalf("text %q", ds[2].MeasurementText())
	}
	if c := strings.Count(history(s), "Dimension text = "); c != 4 {
		t.Fatalf("echoes = %d, want 4\n%s", c, history(s))
	}
	s.Submit("") // <Select>
	if want := "Select continued dimension:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("")
	if s.Active() != "" {
		t.Fatalf("Enter did not end; prompt %q", s.Prompt())
	}
	s.Submit("U")
	if n := len(dims(s)); n != 1 {
		t.Fatalf("after U: %d dimensions, want 1 (DIMCONTINUE is one undo step)", n)
	}
}

func TestDimContinueSelectAndAligned(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Dimension{Kind: doc.DimAligned, Ext1: geom.V(0, 0), Ext2: geom.V(30, 40), DimLine: geom.V(-8, 6)})
	dims(s)[0].Recompute()
	s.Run("DIMCONTINUE")
	if want := "Select continued dimension:"; s.Prompt() != want {
		t.Fatalf("no last dimension: prompt = %q", s.Prompt())
	}
	// Pick the first extension line: the chain continues from Ext1, the other way.
	base := dims(s)[0]
	s.Submit(cmdsys.FormatPoint(base.Layout().ExtLines[0].Mid()))
	s.Submit("-30,-40")
	ds := dims(s)
	if len(ds) != 2 {
		t.Fatalf("dimensions = %d\n%s", len(ds), history(s))
	}
	d := ds[1]
	if d.Kind != doc.DimRotated {
		t.Fatalf("kind %v, want rotated", d.Kind)
	}
	closeTo(t, "rotation", d.Rotation, math.Atan2(40, 30))
	near(t, "Ext1", d.Ext1, geom.V(0, 0))
	if d.MeasurementText() != "50" {
		t.Fatalf("text %q", d.MeasurementText())
	}
	// Same dimension line as the base.
	if off := d.DimLine.Sub(base.DimLine).Cross(base.Direction()); math.Abs(off) > 1e-9 {
		t.Fatalf("dimension line offset %v", off)
	}
	s.Submit("S")
	s.Submit("100,100") // nothing
	if !strings.Contains(history(s), "No object found.") {
		t.Fatalf("history:\n%s", history(s))
	}
	s.Escape()
}

func TestDimContinueUndoWithNothing(t *testing.T) {
	s := newSession(t)
	s.Script("DLI 0,0 10,0 5,20")
	s.Script("DCO U")
	if !strings.Contains(history(s), "Nothing to undo.") {
		t.Fatalf("history:\n%s", history(s))
	}
	s.Escape()
	if n := len(dims(s)); n != 1 {
		t.Fatalf("dimensions = %d", n)
	}
}

func TestDimArcWholeAndPartial(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi / 2},
		&doc.LWPolyline{Points: []geom.Vec2{{X: 100, Y: 0}, {X: 120, Y: 0}}, Bulges: []float64{1}},
		&doc.Line{Start: geom.V(0, 50), End: geom.V(10, 50)},
	)
	s.Run("DIMARC")
	if want := "Select arc or polyline arc segment:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit(cmdsys.FormatPoint(geom.V(10*math.Cos(0.7), 10*math.Sin(0.7))))
	if want := "Specify arc length dimension location or [Mtext/Text/Angle/Partial/Leader]:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	if previewItems(s, geom.V(12, 12)) == 0 {
		t.Fatal("no preview")
	}
	s.Submit("L")
	if !strings.Contains(history(s), "Leader is not supported") {
		t.Fatalf("history:\n%s", history(s))
	}
	s.Submit("12,12")
	d := oneDim(t, s)
	if d.Kind != doc.DimArcLength || d.Partial || d.Radius != 10 {
		t.Fatalf("dim = %+v", d)
	}
	if d.MeasurementText() != "15,71" || !strings.Contains(history(s), "Dimension text = 15,71") {
		t.Fatalf("text %q\n%s", d.MeasurementText(), history(s))
	}
	if d.ArcSymbol != doc.DimArcSymBefore {
		t.Fatalf("arc symbol %d", d.ArcSymbol)
	}

	// Partial, on the bulged polyline segment (the lower half circle of radius 10 centred at
	// 110,0), from the projection of 120,-5 to its bottom.
	s.Script("DIMARCSYM 2")
	s.Run("ARCOQUOTA")
	submitAll(s, "110,-10", "P")
	if want := "Specify first point for arc length dimension:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "120,-5", "110,-30", "110,-20")
	ds := dims(s)
	if len(ds) != 2 {
		t.Fatalf("dimensions = %d\n%s", len(ds), history(s))
	}
	d = ds[1]
	if !d.Partial || d.ArcSymbol != doc.DimArcSymNone {
		t.Fatalf("partial %v symbol %d", d.Partial, d.ArcSymbol)
	}
	near(t, "partial Ext2", d.Ext2, geom.V(110, -10))
	closeTo(t, "partial length", d.Measurement(), 10*(math.Pi/2-math.Atan(0.5)))

	// A line is refused.
	submitAll(s, "DAR", "5,50")
	if !strings.Contains(history(s), "Object selected is not an arc or polyline arc segment.") {
		t.Fatalf("history:\n%s", history(s))
	}
	s.Escape()
}

func TestDimCommandsInItalian(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Run("DIMLINEARE")
	if want := "Specificare origine prima linea di estensione o <seleziona oggetto>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "0,0", "10,0")
	if want := "Specificare posizione della linea di quota o [Mtesto/Testo/Angolo/Orizzontale/Verticale/Ruotato]:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "O", "5,-10")
	if !strings.Contains(history(s), "Testo di quota = 10") {
		t.Fatalf("history:\n%s", history(s))
	}
	s.Run("DIMCONTINUA")
	if want := "Specificare origine seconda linea di estensione o [Seleziona/Annulla] <Seleziona>:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "25,0", "")
	s.Escape()
	s.Run("DIMALLINEATA")
	submitAll(s, "0,0", "0,10", "-5,5")
	addEntities(t, s, &doc.Arc{Center: geom.V(100, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi})
	s.Run("ARCOQUOTA")
	if want := "Selezionare arco o segmento di arco di polilinea:"; s.Prompt() != want {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	submitAll(s, "100,10", "100,20")
	if n := len(dims(s)); n != 4 {
		t.Fatalf("dimensions = %d\n%s", n, history(s))
	}
}
