package commands_test

import (
	"errors"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

func TestChpropLinetypeScaleAndLineweight(t *testing.T) {
	s, a, _ := twoLines(t)
	s.Script("CHPROP\n5,0\n\nLT\nDASHED\n\n")
	if c := s.Drawing().Entity(a).Common(); c.Linetype != "" {
		t.Fatalf("an unloaded linetype was set: %+v", *c)
	}
	if !strings.Contains(history(s), `Linetype "DASHED" is not loaded.`) {
		t.Fatalf("no message:\n%s", history(s))
	}
	s.Script("-LINETYPE\nL\nDASHED\n\n")
	depth := s.Drawing().UndoDepth()
	s.Script("CHPROP\n5,0\n\nLT\ndashed\nS\n2\nLW\n0,30\nC\nBYBLOCK\n\n")
	c := s.Drawing().Entity(a).Common()
	if c.Linetype != "DASHED" || c.LinetypeScale != 2 || c.Lineweight != 30 || c.Color != doc.ByBlock {
		t.Fatalf("CHPROP: %+v", *c)
	}
	if s.Drawing().UndoDepth() != depth+1 {
		t.Fatal("CHPROP is not one undo step")
	}
	s.Script("CHPROP\n5,0\n\nLT\nBYLAYER\nLW\nBYLAYER\n\n")
	if c := s.Drawing().Entity(a).Common(); c.Linetype != "" || c.Lineweight != doc.LineweightByLayer {
		t.Fatalf("BYLAYER: %+v", *c)
	}
	s.Drawing().Undo()
	s.Drawing().Undo()
	if c := s.Drawing().Entity(a).Common(); c.Linetype != "" || c.LinetypeScale != 0 || c.Color != doc.ByLayer {
		t.Fatalf("undo: %+v", *c)
	}
}

func TestLinetypeCommandListLoadSet(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	s.Script("-LINETYPE\nL\ndashed,hidden*\n\n")
	for _, n := range []string{"DASHED", "HIDDEN", "HIDDEN2", "HIDDENX2"} {
		if _, ok := d.Linetype(n); !ok {
			t.Fatalf("%s not loaded", n)
		}
	}
	if !strings.Contains(history(s), "Linetype DASHED loaded.") {
		t.Fatalf("no load message:\n%s", history(s))
	}
	s.Script("LT\nL\nDASHED,NOSUCH\n\n")
	if h := history(s); !strings.Contains(h, "Linetype DASHED is already loaded.") || !strings.Contains(h, "Linetype NOSUCH not found in acadiso.lin.") {
		t.Fatalf("messages:\n%s", h)
	}
	s.Script("-LINETYPE\nS\nborder\n\n")
	if !strings.Contains(history(s), `Linetype "border" is not loaded.`) {
		t.Fatalf("unloaded Set:\n%s", history(s))
	}
	s.Script("-LINETYPE\nS\ndashed\n\n")
	if s.Vars().Text("CELTYPE") != "DASHED" || d.CurrentLinetype != "DASHED" {
		t.Fatalf("CELTYPE %q", s.Vars().Text("CELTYPE"))
	}
	s.Script("-LINETYPE\n?\n*\n\n")
	if !strings.Contains(history(s), "HIDDENX2") {
		t.Fatalf("? lists the loaded linetypes:\n%s", history(s))
	}
	s.Script("LINE\n0,0\n1,0\n\n")
	if l := entities[*doc.Line](s); len(l) != 1 || l[0].Linetype != "DASHED" {
		t.Fatal("a new line does not get CELTYPE")
	}
	s.Script("U\nU\n")
	if s.Vars().Text("CELTYPE") != "BYLAYER" {
		t.Fatalf("undo of Set: %q", s.Vars().Text("CELTYPE"))
	}
	s.Script("U\nU\n")
	if _, ok := d.Linetype("DASHED"); ok {
		t.Fatal("undo of Load")
	}
}

func TestLweightCommand(t *testing.T) {
	s := newSession(t)
	s.Script("LWEIGHT\n0,30\n")
	if s.Vars().Int("CELWEIGHT") != 30 || s.Drawing().CurrentLineweight != 30 {
		t.Fatalf("CELWEIGHT %d", s.Vars().Int("CELWEIGHT"))
	}
	s.Script("-LW\nBYBLOCK\n")
	if s.Vars().Int("CELWEIGHT") != -2 {
		t.Fatal("BYBLOCK")
	}
	s.Script("-LWEIGHT\n0.31\n\n")
	if !strings.Contains(history(s), "Invalid lineweight.") || s.Vars().Int("CELWEIGHT") != -2 {
		t.Fatalf("bad value:\n%s", history(s))
	}
	s.Script("LW\n?\n0\n")
	h := history(s)
	if !strings.Contains(h, "Valid lineweights: BYLAYER BYBLOCK DEFAULT 0.00 0.05") || s.Vars().Int("CELWEIGHT") != 0 {
		t.Fatalf("? and 0.00 mm:\n%s", h)
	}
	if !strings.Contains(h, "Current lineweight: BYLAYER") {
		t.Fatalf("current:\n%s", h)
	}
}

func TestMatchPropCopiesLinetypeScaleAndLineweight(t *testing.T) {
	s, a, b := twoLines(t)
	d := s.Drawing()
	s.Script("-LINETYPE\nL\nDASHED\n\n")
	s.Script("CHPROP\n5,0\n\nLT\nDASHED\nS\n3\nLW\n0.5\n\n")
	s.Script("MA\n5,0\n5,5\n\n")
	if c := d.Entity(b).Common(); c.Linetype != "DASHED" || c.LinetypeScale != 3 || c.Lineweight != 50 || c.Layer != "A" {
		t.Fatalf("destination %+v", *c)
	}
	if !strings.Contains(history(s), "Current active settings = Color Layer Ltype Ltscale Lineweight") {
		t.Fatalf("settings line:\n%s", history(s))
	}
	_ = a
	// Switch Ltype off: the linetype is not copied.
	s.Script("CHPROP\n5,5\n\nLT\nBYLAYER\n\n")
	s.Script("MA\n5,0\nS\nLT\n\n5,5\n\n")
	if c := d.Entity(b).Common(); c.Linetype != "" {
		t.Fatalf("Ltype switched off: %+v", *c)
	}
}

func TestPropertySystemVariables(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	v := s.Vars()
	for name, want := range map[string]string{"CECOLOR": "BYLAYER", "CELTYPE": "BYLAYER", "CELWEIGHT": "-1", "CELTSCALE": "1", "LTSCALE": "1", "DIMSTYLE": "ISO-25", "DIMTAD": "1", "DIMBLK": "", "DIMCLRD": "0", "DIMATFIT": "3", "DIMTOFL": "1", "DIMTXSTY": "Standard"} {
		if got := v.String(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	s.Script("CECOLOR BYBLOCK\nCELTSCALE 2\nLTSCALE 4\nDIMBLK _archtick\nDIMTAD 4\nDIMCLRD 256\nDIMJUST 2\nDIMSAH 1\nDIMBLK1 dot\n")
	cur := d.Current()
	if cur.Color != doc.ByBlock || cur.LinetypeScale != 2 || cur.LTScale != 4 {
		t.Fatalf("current %+v", cur)
	}
	st := d.ActiveDimStyle()
	if st.ArrowBlock != "_ArchTick" || st.TextVertical != 4 || st.LineColor != 256 || st.TextJust != 2 || !st.SeparateArrows || st.ArrowBlock1 != "_Dot" {
		t.Fatalf("style %+v", st)
	}
	for name, val := range map[string]string{"CELTYPE": "NOSUCH", "CELTSCALE": "0", "CELWEIGHT": "7", "DIMBLK": "NOBLOCK", "DIMTXSTY": "NOSTYLE", "DIMSTYLE": "NOSTYLE", "CLAYER": "NOLAYER"} {
		before := v.String(name)
		if err := v.SetString(name, val); !errors.Is(err, cmdsys.ErrVarInvalid) {
			t.Errorf("%s %s: %v", name, val, err)
		}
		if v.String(name) != before {
			t.Errorf("%s changed to %q", name, v.String(name))
		}
	}
	d.AddLayer("F", 1).Frozen = true
	if err := v.Set("CLAYER", "F"); !errors.Is(err, cmdsys.ErrVarInvalid) || d.CurrentLayer != "0" {
		t.Fatalf("a frozen layer made current: %v", err)
	}
	// Each variable typed at Command: is its own undo step.
	s.Script("U\n")
	if d.ActiveDimStyle().ArrowBlock1 != "" {
		t.Fatal("U did not undo DIMBLK1")
	}
	// Outside a command a Set is its own named step.
	n := d.UndoDepth()
	if err := v.Set("CELWEIGHT", 50); err != nil || d.UndoDepth() != n+1 || d.UndoName() != "CELWEIGHT" {
		t.Fatalf("Set outside a command: %v depth %d name %q", err, d.UndoDepth(), d.UndoName())
	}
	// -COLOR writes CECOLOR.
	s.Script("-COLOR\nred\n")
	if d.CurrentColor != 1 {
		t.Fatal("-COLOR")
	}
}

func TestDimensionFollowsStyleVariables(t *testing.T) {
	s := newSession(t)
	s.Script("DLI 0,0 10,0 5,5")
	dm := oneDim(t, s)
	s.Script("DIMASZ 7\n")
	if dm := oneDim(t, s); dm.Style().ArrowSize != 7 || dm.Overrides.Set != 0 {
		t.Fatalf("existing dimension does not follow the style: %v", dm.Style().ArrowSize)
	}
	_ = dm
}

// G86: copies keep ByBlock and ByLayer values and the linetype, scale and lineweight, whatever
// the current properties.
func TestCopiesKeepTheirProperties(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	s.Script("-LINETYPE\nL\nDASHED\n\n")
	l := &doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByBlock, Linetype: "DASHED", LinetypeScale: 2, Lineweight: 50}, Start: geom.V(0, 0), End: geom.V(10, 0)}
	tx, _ := d.Begin()
	h := tx.AddCopy(l)
	tx.Commit()
	s.Script("CECOLOR 1\nCELTYPE BYBLOCK\nCELWEIGHT 30\nCELTSCALE 5\n")
	want := *d.Entity(h).Common()
	check := func(cmd string) {
		t.Helper()
		for _, x := range d.Handles() {
			if x == h {
				continue
			}
			c := *d.Entity(x).Common()
			c.H, c.Owner = 0, 0
			w := want
			w.H, w.Owner = 0, 0
			if c != w {
				t.Fatalf("%s: copy %+v, want %+v", cmd, c, w)
			}
		}
	}
	s.Selection().Add(h)
	s.Script("COPY\n0,0\n0,5\n\n")
	check("COPY")
	s.Selection().Add(h)
	s.Script("MIRROR\n0,10\n10,10\nN\n")
	check("MIRROR")
	s.Script("OFFSET\n1\n5,0\n5,-5\n\n")
	check("OFFSET")
	s.Script("BREAK\n2,0\n4,0\n")
	check("BREAK")
}

func TestExplodePiecesKeepProperties(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	s.Script("-LINETYPE\nL\nHIDDEN\n\n")
	p := &doc.LWPolyline{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByBlock, Linetype: "HIDDEN", Lineweight: 40}, Points: []geom.Vec2{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
	tx, _ := d.Begin()
	tx.AddCopy(p)
	tx.Commit()
	s.Script("CECOLOR 2\n")
	s.Selection().Add(d.Handles()...)
	s.Script("EXPLODE\n")
	ls := entities[*doc.Line](s)
	if len(ls) != 2 {
		t.Fatalf("%d lines", len(ls))
	}
	for _, l := range ls {
		if l.Color != doc.ByBlock || l.Linetype != "HIDDEN" || l.Lineweight != 40 {
			t.Fatalf("piece %+v", l.EntityCommon)
		}
	}
}

// G88: -LAYER and LAYER edit layers in the command's undo step.
func TestLayerCommandsAreUndoable(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	s.Script("-LAYER\nN\nA,B\nC\n1\nA\nLO\nA\n\n")
	if l := d.Layer("A"); l == nil || l.Color != 1 || !l.Locked || d.Layer("B") == nil {
		t.Fatal("-LAYER edits")
	}
	s.Script("U\n")
	if d.Layer("A") != nil || d.Layer("B") != nil {
		t.Fatal("U did not undo -LAYER")
	}
	s.Script("LAYER\nW=3\n")
	if d.CurrentLayer != "W" || d.Layer("W").Color != 3 {
		t.Fatal("LAYER")
	}
	s.Script("U\n")
	if d.CurrentLayer != "0" || d.Layer("W") != nil {
		t.Fatal("U did not undo LAYER")
	}
	// Transparently inside LINE, the layer edit joins LINE's step.
	s.Script("LINE\n0,0\n10,0\n'LAYER\nT=4\n10,10\n\n")
	if d.CurrentLayer != "T" || len(entities[*doc.Line](s)) != 2 {
		t.Fatalf("transparent LAYER: current %q", d.CurrentLayer)
	}
	s.Script("U\n")
	if d.Layer("T") != nil || len(entities[*doc.Line](s)) != 0 {
		t.Fatal("U did not undo LINE with its transparent LAYER")
	}
	s.Script("-LAYER\nM\nX\n\n-LAYER\nF\n0\n\n")
	if d.CurrentLayer != "X" || !d.Layer("0").Frozen {
		t.Fatal("Make and Freeze")
	}
	s.Script("U\nU\n")
	if d.CurrentLayer != "0" || d.Layer("0").Frozen || d.Layer("X") != nil {
		t.Fatal("U did not undo Make and Freeze")
	}
}

// TestLWDEFAULTVariable checks LWDEFAULT reads and writes Drawing.LWDefault in hundredths of a
// millimetre (AutoCAD's 25 by default), takes only standard widths, 0 being 0.00 mm, and is undoable.
func TestLWDEFAULTVariable(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	v := s.Vars()
	if got := v.Int("LWDEFAULT"); got != 25 {
		t.Fatalf("LWDEFAULT = %d, want 25", got)
	}
	s.Script("LWDEFAULT 50\n")
	if d.LWDefaultValue() != 50 || v.Int("LWDEFAULT") != 50 {
		t.Fatalf("LWDEFAULT 50: drawing %d\n%s", d.LWDefault, history(s))
	}
	if err := v.Set("LWDEFAULT", 0); err != nil || d.LWDefault != doc.LineweightZero || v.Int("LWDEFAULT") != 0 {
		t.Fatalf("LWDEFAULT 0: %v, drawing %d", err, d.LWDefault)
	}
	for _, bad := range []int{7, -1, -3, 212} {
		if err := v.Set("LWDEFAULT", bad); err == nil || v.Int("LWDEFAULT") != 0 {
			t.Errorf("LWDEFAULT %d accepted: %v", bad, err)
		}
	}
	d.Undo()
	if v.Int("LWDEFAULT") != 50 {
		t.Fatalf("undo: LWDEFAULT %d", v.Int("LWDEFAULT"))
	}
}

// TestDimVariableRanges checks DIMSCALE 0 is taken (it reads as 1, AutoCAD's value with no paper
// space), a negative DIMGAP (AutoCAD's boxed text, not drawn here) and a DIMDSEP longer than one
// character are refused with a message.
func TestDimVariableRanges(t *testing.T) {
	s := newSession(t)
	v := s.Vars()
	if err := v.Set("DIMSCALE", 0.0); err != nil || v.Real("DIMSCALE") != 0 {
		t.Fatalf("DIMSCALE 0: %v", err)
	}
	if err := v.Set("DIMGAP", -1.0); err == nil || v.Real("DIMGAP") != 0.625 {
		t.Fatalf("DIMGAP -1: %v, now %v", err, v.Real("DIMGAP"))
	}
	if err := v.Set("DIMDSEP", ";;"); err == nil || v.Text("DIMDSEP") != "," {
		t.Fatalf("DIMDSEP ;;: %v, now %q", err, v.Text("DIMDSEP"))
	}
	s.Script("DIMDSEP\n\"ab\"\n")
	if !strings.Contains(history(s), "Invalid value.") || v.Text("DIMDSEP") != "," {
		t.Fatalf("no message, DIMDSEP %q\n%s", v.Text("DIMDSEP"), history(s))
	}
}
