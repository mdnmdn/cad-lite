package commands_test

import (
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func TestLayerCLIOptions(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nWALLS,DIMS,TEXT\nC\n1\nWALLS\nC\nred\nDIMS,TEXT\nM\nWALLS\nOFF\nTEXT\nLO\nDIMS\nF\nDIMS\nD\nWALLS\nExterior\n\n")
	d := s.Drawing()
	if s.Active() != "" {
		t.Fatalf("still in %s", s.Active())
	}
	w, di, tx := d.Layer("WALLS"), d.Layer("DIMS"), d.Layer("TEXT")
	if w == nil || di == nil || tx == nil {
		t.Fatal("layers missing")
	}
	if w.Color != 1 || di.Color != 1 || tx.Color != 1 {
		t.Fatalf("colours %d %d %d", w.Color, di.Color, tx.Color)
	}
	if !tx.Off || tx.Locked || !di.Locked || !di.Frozen || w.Off {
		t.Fatalf("states: %+v %+v %+v", w, di, tx)
	}
	if d.CurrentLayer != "WALLS" || w.Description != "Exterior" {
		t.Fatalf("current %q desc %q", d.CurrentLayer, w.Description)
	}
}

func TestLayerCLIWildcardsAndFreezeCurrent(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nA1,A2,B1\nM\nA1\nF\nA*\nLO\n?2\n\n")
	d := s.Drawing()
	if d.Layer("A1").Frozen {
		t.Fatal("the current layer must not freeze")
	}
	if !d.Layer("A2").Frozen || d.Layer("B1").Frozen {
		t.Fatalf("wildcard freeze: A2 %v B1 %v", d.Layer("A2").Frozen, d.Layer("B1").Frozen)
	}
	if !d.Layer("A2").Locked || d.Layer("A1").Locked {
		t.Fatal("? wildcard must match one character: A2 only")
	}
	if !strings.Contains(strings.ToLower(history(s)), "current layer") {
		t.Fatalf("no refusal message:\n%s", history(s))
	}
}

func TestLayerCLIOffCurrentAsks(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nOFF\n0\n")
	if s.Prompt() == "" || !strings.Contains(s.Prompt(), "CURRENT") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("")
	if s.Drawing().Layer("0").Off {
		t.Fatal("default answer must be No")
	}
	s.Script("OFF\n0\nY\n\n")
	if !s.Drawing().Layer("0").Off {
		t.Fatal("answering Yes must turn the current layer off")
	}
}

func TestLayerCLISetRefusesFrozenAndMissing(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nA\nF\nA\nS\nA\nS\nNOPE\n\n")
	if s.Drawing().CurrentLayer != "0" {
		t.Fatalf("current = %q", s.Drawing().CurrentLayer)
	}
}

func TestLayerCLILtypeLweightPlot(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nA\nLW\n0.25\nA\nP\nN\nA\nLT\nCONTINUOUS\nA\nLT\nHIDDEN\nA\n\n")
	l := s.Drawing().Layer("A")
	if l.Lineweight != 25 || !l.NoPlot || l.Linetype != "" {
		t.Fatalf("A = %+v", l)
	}
	s.Script("-LAYER\nLW\n0.27\nA\n\n")
	if s.Drawing().Layer("A").Lineweight != 25 {
		t.Fatal("a non-standard lineweight must be refused")
	}
}

func TestLayerCLIListsLayers(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nWALLS\nF\nWALLS\n?\n\n\n")
	h := history(s)
	if !strings.Contains(h, "WALLS") || !strings.Contains(h, "Frozen") {
		t.Fatalf("listing missing:\n%s", h)
	}
}

func TestFrozenLayerIsNotDisplayedPickedOrSnapped(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	d.AddLayer("F", 1)
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "F"}, Start: geom.V(0, 0), End: geom.V(10, 0)})
	tx.Commit()
	b := render.NewBuilder(d, render.Color{})
	e := d.Entity(h)
	if !b.Visible(e) {
		t.Fatal("thawed layer must be visible")
	}
	d.Layer("F").Frozen = true
	if b.Visible(e) {
		t.Fatal("frozen layer must not be drawn")
	}
	if _, ok := cmdsys.Pick(d, s.Index(), geom.V(5, 0), 2); ok {
		t.Fatal("frozen layer must not be pickable")
	}
	if _, ok := cmdsys.FindSnap(d, s.Index(), geom.V(0.5, 0.2), 2, cmdsys.SnapEndpoint); ok {
		t.Fatal("frozen layer must not be snappable")
	}
	d.Layer("F").Frozen = false
	if _, ok := cmdsys.Pick(d, s.Index(), geom.V(5, 0), 2); !ok {
		t.Fatal("thawed layer must be pickable")
	}
}

// A bare number at a layer-name prompt is a name, even when a last point exists (a number would
// otherwise read as a distance from it).
func TestLayerCLISetNumericNameAfterAPoint(t *testing.T) {
	s := newSession(t)
	s.Script("-LAYER\nN\nW\nM\nW\n\nLINE\n0,0\n10,0\n\n-LAYER\nS\n0\n\n")
	if got := s.Drawing().CurrentLayer; got != "0" {
		t.Fatalf("current layer = %q, want 0", got)
	}
}

func TestLayerCLIPickObjectForItsLayer(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	d.AddLayer("PICKED", 4)
	tx, _ := d.Begin()
	tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "PICKED"}, Start: geom.V(0, 0), End: geom.V(10, 0)})
	tx.Commit()
	s.Script("-LAYER\nS\n\n5,0\n\n")
	if d.CurrentLayer != "PICKED" {
		t.Fatalf("current layer = %q", d.CurrentLayer)
	}
	s.Script("-LAYER\nLO\n\n5,0\n\n")
	if !d.Layer("PICKED").Locked {
		t.Fatal("LOck on a picked object must lock its layer")
	}
}

func TestLayerCLIItalian(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Script("-LAYER\nNuovo\nA\nCongela\nA\nBL\nA\n\n")
	l := s.Drawing().Layer("A")
	if l == nil || !l.Frozen || !l.Locked {
		t.Fatalf("A = %+v\n%s", l, history(s))
	}
	if !strings.Contains(history(s), "Layer corrente") {
		t.Fatalf("history is not Italian:\n%s", history(s))
	}
}
