package cadmcp_test

import (
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
