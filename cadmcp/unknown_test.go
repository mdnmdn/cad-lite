package cadmcp_test

import (
	"strings"
	"testing"

	"cad-lite/doc"
)

func TestListEntitiesFlagsUnknown(t *testing.T) {
	r := newRig(t)
	r.s.Drawing().Load(&doc.Unknown{
		EntityCommon: doc.EntityCommon{H: 0x40, Layer: "0", Color: doc.ByLayer},
		Type:         "SPLINE",
		Raw:          []doc.Pair{{Code: 10, Value: "5"}, {Code: 20, Value: "5"}},
	})
	r.must("add_line", `{"x1":0,"y1":0,"x2":10,"y2":0}`)
	l := r.must("list_entities", `{"class":"spline"}`)
	es := l["entities"].([]any)
	if l["total"] != 1.0 || len(es) != 1 {
		t.Fatalf("list = %v", l)
	}
	e := es[0].(map[string]any)
	if e["class"] != "SPLINE" || e["unknown"] != true || e["handle"] != "40" {
		t.Fatalf("entity = %v", e)
	}
	for _, e := range r.must("list_entities", `{}`)["entities"].([]any) {
		if m := e.(map[string]any); m["class"] == "LINE" && m["unknown"] != nil {
			t.Fatalf("a LINE is flagged unknown: %v", m)
		}
	}
}

// G32: a handle-taking tool given an entity it cannot act on (a class the app does not model)
// used to report success while the transaction silently skipped it.
func TestHandleToolsRejectUnknownEntities(t *testing.T) {
	r := newRig(t)
	r.s.Drawing().Load(&doc.Unknown{
		EntityCommon: doc.EntityCommon{H: 0x40, Layer: "0", Color: doc.ByLayer},
		Type:         "SPLINE",
		Raw:          []doc.Pair{{Code: 10, Value: "5"}, {Code: 20, Value: "5"}},
	})
	cases := []struct{ tool, args string }{
		{"erase", `{"handles":["40"]}`},
		{"move", `{"handles":["40"],"dx":1,"dy":1}`},
		{"rotate", `{"handles":["40"],"cx":0,"cy":0,"angle":90}`},
		{"scale", `{"handles":["40"],"cx":0,"cy":0,"factor":2}`},
		{"set_entity_layer", `{"handles":["40"],"layer":"0"}`},
		{"set_selection", `{"handles":["40"]}`},
		{"measure", `{"handles":["40"]}`},
	}
	for _, c := range cases {
		_, text, ok := r.call(c.tool, c.args)
		if ok || !strings.Contains(text, "SPLINE") {
			t.Errorf("%s: ok=%v text=%q, want an error naming the SPLINE", c.tool, ok, text)
		}
	}
	if r.s.Drawing().Journal().CanUndo() {
		t.Fatal("a rejected call recorded an undo step")
	}

	s, render, _ := renderRig(t, true)
	s.Drawing().Load(&doc.Unknown{EntityCommon: doc.EntityCommon{H: 0x40, Layer: "0", Color: doc.ByLayer}, Type: "SPLINE"})
	if res := render(`{"highlight":["40"]}`); !res.IsError || !strings.Contains(res.Content[0].Text, "SPLINE") {
		t.Errorf("render_png highlight: error=%v, want an error naming the SPLINE", res.IsError)
	}
}
