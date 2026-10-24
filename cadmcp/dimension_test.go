package cadmcp_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cadmcp"
	"cad-lite/doc"
	"cad-lite/geom"
)

// The run_script examples the agent instructions give make the dimensions they describe.
func TestInstructionDimensionExamples(t *testing.T) {
	r := newRig(t)
	out := r.must("run_script", `{"script":"DIMLINEAR 0,0 100,0 50,-10 DIMCONTINUE 180,0 250,0  "}`)
	ds := dims(r)
	if len(ds) != 3 || str(out, "cancelled_command") != "" || r.s.Active() != "" {
		t.Fatalf("%d dimensions, out %v\n%s", len(ds), out, strings.Join(r.s.History(), "\n"))
	}
	for i, want := range []float64{100, 80, 70} {
		if d := ds[i]; d.Kind != doc.DimRotated || d.Rotation != 0 || math.Abs(d.Measurement()-want) > 1e-9 || math.Abs(d.DimLine.Y+10) > 1e-9 {
			t.Fatalf("dimension %d = %+v, measurement %v", i, d, d.Measurement())
		}
	}
	r.must("run_script", `{"script":"DIMALIGNED 0,0 30,40 -20,20"}`)
	r.must("add_arc", `{"center":[0,0],"radius":50,"start_angle":0,"end_angle":90}`)
	r.must("run_script", `{"script":"DIMARC 50,0 50,50"}`)
	ds = dims(r)
	if len(ds) != 5 || ds[3].Kind != doc.DimAligned || ds[3].Measurement() != 50 ||
		ds[4].Kind != doc.DimArcLength || math.Abs(ds[4].Measurement()-25*math.Pi) > 1e-9 {
		t.Fatalf("dimensions %+v\n%s", ds, strings.Join(r.s.History(), "\n"))
	}
	ins := cadmcp.Instructions()
	for _, w := range []string{"DIMLINEAR (DLI)", "DIMALIGNED (DAL)", "DIMCONTINUE (DCO)", "DIMARC (DAR)",
		"DIMLINEAR 0,0 100,0 50,-10 DIMCONTINUE 180,0 250,0  ", "DIMALIGNED 0,0 30,40 -20,20", "DIMARC 50,0 50,50",
		"Enter file format [R12/2000] <2000>:"} {
		if !strings.Contains(ins, w) {
			t.Errorf("the instructions lack %q", w)
		}
	}
}

func dims(r *rig) []*doc.Dimension {
	var out []*doc.Dimension
	for _, h := range r.s.Drawing().Handles() {
		if d, ok := r.s.Drawing().Entity(h).(*doc.Dimension); ok {
			out = append(out, d)
		}
	}
	return out
}

func TestListAndEditDimension(t *testing.T) {
	r := newRig(t)
	r.must("run_script", `{"script":"DIMLINEAR 0,0 100,0 50,-10 LINE 0,50 10,50 "}`)
	m := r.must("list_entities", `{"class":"dimension"}`)
	es, _ := m["entities"].([]any)
	if num(m, "total") != 1 || len(es) != 1 {
		t.Fatalf("list = %v", m)
	}
	e := es[0].(map[string]any)
	st, _ := e["style"].(map[string]any)
	if str(e, "class") != "DIMENSION" || str(e, "kind") != "rotated" || num(e, "measurement") != 100 || str(e, "text") != "100" ||
		num(e, "rotation") != 0 || str(st, "name") != "ISO-25" || num(st, "dimtxt") != 2.5 {
		t.Fatalf("dimension = %v", e)
	}
	if p, _ := e["ext2"].([]any); len(p) != 2 || p[0].(float64) != 100 {
		t.Fatalf("ext2 = %v", e["ext2"])
	}
	if m := r.must("list_entities", `{"text":"100"}`); num(m, "total") != 1 {
		t.Fatalf("text filter = %v", m)
	}
	h := str(e, "handle")
	u := r.must("update_text", `{"handle":"`+h+`","value":"<> mm"}`)
	if str(u, "text") != "100 mm" || str(u, "text_override") != "<> mm" {
		t.Fatalf("updated = %v", u)
	}
	r.fails("update_text", `{"handle":"`+h+`","height":5}`, "only value")
	r.must("move", `{"handles":["`+h+`"],"dx":10,"dy":0}`)
	r.must("rotate", `{"handles":["`+h+`"],"angle":90,"cx":10,"cy":0}`)
	r.must("scale", `{"handles":["`+h+`"],"factor":2,"cx":10,"cy":0}`)
	d := dims(r)[0]
	if !d.Ext2.Near(geom.V(10, 200), 1e-9) || math.Abs(d.Measurement()-200) > 1e-9 || math.Abs(d.Rotation-math.Pi/2) > 1e-9 {
		t.Fatalf("after move, rotate, scale: %+v", d)
	}
}
