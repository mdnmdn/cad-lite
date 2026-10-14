package cadmcp_test

import (
	"math"
	"os"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dwg"
)

// TestAgentToolsSeeADrawingOpenedFromDWG checks that the agent tools work on a drawing the DWG
// reader produced: the agent cannot open files itself, but whatever the user opened is queryable
// and editable like any other drawing.
func TestAgentToolsSeeADrawingOpenedFromDWG(t *testing.T) {
	f, err := os.Open("../fileio/dwg/testdata/sample_AC1018.dwg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, _, err := dwg.Codec{}.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	r.s.SetDrawing(d)

	l := r.must("list_entities", `{"class":"line"}`)
	if l["total"] != 26.0 {
		t.Fatalf("%v LINEs, want 26", l["total"])
	}
	if got := r.must("list_entities", `{}`)["total"]; got != 56.0 {
		t.Fatalf("%v entities, want 56", got)
	}
	// The layers arrive with their properties.
	dr := r.must("get_drawing", `{}`)
	layers := dr["layers"].([]any)
	if len(layers) != 21 {
		t.Fatalf("%d layers, want 21", len(layers))
	}
	if l := layers[2].(map[string]any); l["name"] != "Layer_Off" || l["off"] != true {
		t.Fatalf("layer 2 = %v, want Layer_Off, off", l)
	}
	// Edit by handle: move a line the DWG defined.
	r.must("move", `{"handles":["2C7"],"dx":10,"dy":0}`)
	ln := r.s.Drawing().Entity(0x2C7).(*doc.Line)
	if math.Abs(ln.Start.X-(3.592533998909388+10)) > 1e-9 || ln.Start.Y != 1.477241896180196 {
		t.Fatalf("moved line = %+v", ln)
	}
	r.must("undo", `{}`)
	if ln = r.s.Drawing().Entity(0x2C7).(*doc.Line); ln.Start.X > 4 {
		t.Fatalf("undo left the line at %v", ln.Start)
	}
}
