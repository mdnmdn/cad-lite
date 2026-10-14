package cadmcp_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"cad-lite/cadmcp"
	"cad-lite/doc"
)

func TestBatchMixedOpsWithClientIDsIsOneUndoStep(t *testing.T) {
	r := newRig(t)
	r.must("create_layer", `{"name":"doors","color":3}`)
	old := str(r.must("add_circle", `{"cx":100,"cy":100,"r":5}`), "handle")
	gone := str(r.must("add_line", `{"x1":0,"y1":-5,"x2":9,"y2":-5}`), "handle")
	depth := r.s.Drawing().UndoDepth()
	changedBefore := r.changed.Load()

	m := r.must("batch", `{"ops":[
		{"op":"add_rectangle","id":"wall","args":{"x1":0,"y1":0,"x2":100,"y2":10}},
		{"op":"add_line","id":"door1","args":{"x1":10,"y1":0,"x2":20,"y2":0}},
		{"op":"add_arc","id":"swing","args":{"p1":[20,0],"p2":[17.07,7.07],"p3":[10,10]}},
		{"op":"add_polyline","args":{"points":[[0,20],[5,25],[10,20]]}},
		{"op":"move","args":{"handles":["$door1","$swing"],"dx":5,"dy":0}},
		{"op":"set_entity_layer","args":{"handles":["$door1","$swing"],"layer":"doors"}},
		{"op":"scale","args":{"handles":["`+old+`"],"cx":100,"cy":100,"factor":2}},
		{"op":"rotate","args":{"handles":["$wall"],"cx":0,"cy":0,"angle":90}},
		{"op":"add_line","id":"tmp","args":{"x1":0,"y1":0,"x2":1,"y2":1}},
		{"op":"erase","args":{"handles":["$tmp","`+gone+`"]}}
	]}`)
	if num(m, "ops") != 10 {
		t.Fatalf("result = %v", m)
	}
	hs := m["handles"].(map[string]any)
	if len(hs) != 3 || hs["tmp"] != nil {
		t.Fatalf("handles = %v (an entity created and erased in the batch has none)", hs)
	}
	door := r.s.Drawing().Entity(mustHandle(t, hs["door1"].(string))).(*doc.Line)
	if door.Start.X != 15 || door.Common().Layer != "doors" {
		t.Fatalf("door = %+v", door)
	}
	wall := r.s.Drawing().Entity(mustHandle(t, hs["wall"].(string))).(*doc.LWPolyline)
	if p := wall.Points[1]; math.Abs(p.X) > 1e-9 || math.Abs(p.Y-100) > 1e-9 {
		t.Fatalf("wall not rotated: %v", wall.Points)
	}
	if c := r.s.Drawing().Entity(mustHandle(t, old)).(*doc.Circle); c.Radius != 10 {
		t.Fatalf("radius = %v", c.Radius)
	}
	created := m["created"].([]any)
	if len(created) != 4 {
		t.Fatalf("created = %v", created)
	}
	for _, c := range created {
		if c := c.(map[string]any); c["id"] == "door1" && (c["class"] != "LINE" || c["layer"] != "doors") {
			t.Fatalf("created door1 = %v", c)
		}
	}
	if fmt.Sprint(m["modified"]) != "["+old+"]" || fmt.Sprint(m["erased"]) != "["+gone+"]" {
		t.Fatalf("modified = %v erased = %v", m["modified"], m["erased"])
	}
	if got := r.s.Drawing().UndoDepth(); got != depth+1 {
		t.Fatalf("undo depth %d, want %d (one step)", got, depth+1)
	}
	if r.changed.Load() != changedBefore+1 {
		t.Fatal("Changed not called once")
	}

	r.must("undo", `{}`)
	d := r.s.Drawing()
	if d.Len() != 2 || d.Entity(mustHandle(t, gone)) == nil || d.Entity(mustHandle(t, old)).(*doc.Circle).Radius != 5 {
		t.Fatalf("one UNDO did not revert the batch: len=%d", d.Len())
	}
	r.must("redo", `{}`)
	if d.Len() != 5 {
		t.Fatalf("redo len = %d", d.Len())
	}
}

// snapshot is the drawing state a failed batch must leave untouched.
func snapshot(r *rig) string {
	d := r.s.Drawing()
	var b strings.Builder
	fmt.Fprintf(&b, "seed=%v depth=%d redo=%d|", d.Header.HandSeed, d.UndoDepth(), d.RedoDepth())
	for _, h := range d.Handles() {
		fmt.Fprintf(&b, "%v;", d.Entity(h))
	}
	return b.String()
}

func TestBatchFailureRollsEverythingBack(t *testing.T) {
	r := newRig(t)
	r.must("create_layer", `{"name":"frozen"}`)
	h := str(r.must("add_line", `{"x1":0,"y1":0,"x2":10,"y2":0}`), "handle")
	onLocked := str(r.must("add_circle", `{"cx":0,"cy":0,"r":1,"layer":"frozen"}`), "handle")
	r.must("create_layer", `{"name":"frozen","locked":true}`)
	r.must("add_line", `{"x1":0,"y1":5,"x2":10,"y2":5}`)
	r.must("undo", `{}`) // a redo step that a failed batch must not clear
	before := snapshot(r)

	good := `{"op":"add_line","id":"a","args":{"x1":0,"y1":0,"x2":1,"y2":1}},` +
		`{"op":"move","args":{"handles":["$a","` + h + `"],"dx":3,"dy":3}},` +
		`{"op":"erase","args":{"handles":["` + h + `"]}}`
	cases := []struct{ bad, want string }{
		// Caught while checking, before anything is applied.
		{`{"op":"add_circle","args":{"cx":0,"cy":0,"r":-1}}`, "ops[3] (add_circle): r must be positive"},
		{`{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":1,"layer":"nope"}}`, "ops[3] (add_line): layer \"nope\" does not exist"},
		{`{"op":"move","args":{"handles":["FFFF"],"dx":1,"dy":1}}`, "ops[3] (move): no entity with handle FFFF"},
		{`{"op":"move","args":{"handles":["$zz"],"dx":1,"dy":1}}`, `"$zz" names no earlier add_* op`},
		{`{"op":"move","args":{"handles":["$late"],"dx":1,"dy":1}},{"op":"add_line","id":"late","args":{"x1":0,"y1":0,"x2":1,"y2":0}}`, "before ops[4] creates it"},
		{`{"op":"rotate","args":{"handles":["` + h + `"],"cx":0,"cy":0,"angle":1}}`, "was erased by ops[2]"},
		{`{"op":"add_line","id":"a","args":{"x1":0,"y1":0,"x2":1,"y2":0}}`, `id "a" is already used by ops[0]`},
		{`{"op":"move","id":"m","args":{"handles":["$a"],"dx":1,"dy":1}}`, "id is only for add_* ops"},
		{`{"op":"run_script","args":{"script":"NEW"}}`, `unknown op "run_script"`},
		{`{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":1,"bogus":1}}`, "invalid arguments"},
		{`{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":1},"extra":1}`, "invalid arguments"},
		{`{"op":"move","args":{"handles":[],"dx":1,"dy":1}}`, "handles must not be empty"},
		// Caught while applying: the transaction is aborted.
		{`{"op":"move","args":{"handles":["` + onLocked + `"],"dx":1,"dy":1}}`, "ops[3] (move): entity " + onLocked + " is on locked layer"},
		{`{"op":"set_entity_layer","args":{"handles":["$a"],"layer":"frozen"}},{"op":"move","args":{"handles":["$a"],"dx":1,"dy":1}}`, "ops[4] (move): entity"},
	}
	for _, c := range cases {
		r.fails("batch", `{"ops":[`+good+`,`+c.bad+`]}`, c.want)
		if got := snapshot(r); got != before {
			t.Fatalf("%s: the drawing changed\nbefore %s\nafter  %s", c.want, before, got)
		}
	}
	if !strings.Contains(snapshot(r), "redo=1") {
		t.Fatal("redo stack lost")
	}
}

func TestBatchCaps(t *testing.T) {
	r := newRig(t)
	r.fails("batch", `{"ops":[]}`, "empty")
	op := `{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":1}}`
	r.fails("batch", `{"ops":[`+strings.Repeat(op+",", cadmcp.MaxBatchOps)+op+`]}`, "too many ops")
	var pts strings.Builder
	for i := range 60000 {
		fmt.Fprintf(&pts, "[%d,0],", i)
	}
	pl := `{"op":"add_polyline","args":{"points":[` + strings.TrimSuffix(pts.String(), ",") + `]}}`
	r.fails("batch", `{"ops":[`+pl+`,`+pl+`,`+pl+`,`+pl+`]}`, "polyline points")
	if r.s.Drawing().Len() != 0 {
		t.Fatal("a refused batch changed the drawing")
	}
	r.must("batch", `{"ops":[`+strings.Repeat(op+",", cadmcp.MaxBatchOps-1)+op+`]}`)
	if r.s.Drawing().Len() != cadmcp.MaxBatchOps || r.s.Drawing().UndoDepth() != 1 {
		t.Fatalf("len = %d depth = %d", r.s.Drawing().Len(), r.s.Drawing().UndoDepth())
	}
}
