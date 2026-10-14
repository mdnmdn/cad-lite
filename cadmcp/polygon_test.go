package cadmcp_test

import (
	"fmt"
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestAddPolygon(t *testing.T) {
	r := newRig(t)
	hex := entityOf[*doc.LWPolyline](t, r, r.must("add_polygon", `{"sides":6,"center":[0,0],"radius":10}`))
	if len(hex.Points) != 6 || !hex.Closed {
		t.Fatalf("hexagon = %+v", hex)
	}
	for _, p := range hex.Points {
		if math.Abs(p.Len()-10) > 1e-9 {
			t.Fatalf("inscribed vertex %v not on the circle", p)
		}
	}
	if b := hex.Bounds(); math.Abs(b.Min.Y+10*math.Sqrt(3)/2) > 1e-9 {
		t.Fatalf("bottom edge not horizontal: %+v", b)
	}
	sq := entityOf[*doc.LWPolyline](t, r, r.must("add_polygon", `{"sides":4,"center":[1,1],"radius":5,"mode":"circumscribed"}`))
	if b := sq.Bounds(); b.Min.Dist(geom.V(-4, -4)) > 1e-9 || b.Max.Dist(geom.V(6, 6)) > 1e-9 {
		t.Fatalf("circumscribed square bounds = %+v", b)
	}
	diamond := entityOf[*doc.LWPolyline](t, r, r.must("add_polygon", `{"sides":4,"center":[0,0],"radius":5,"rotation":45}`))
	if !hasPoint(diamond.Points, geom.V(0, -5)) {
		t.Fatalf("rotated square = %v", diamond.Points)
	}
	tri := entityOf[*doc.LWPolyline](t, r, r.must("add_polygon", `{"sides":3,"edge":[[0,0],[10,0]]}`))
	if !hasPoint(tri.Points, geom.V(5, 10*math.Sqrt(3)/2)) {
		t.Fatalf("edge triangle = %v", tri.Points)
	}
	for args, want := range map[string]string{
		`{"sides":2,"center":[0,0],"radius":1}`:                      "3-1024",
		`{"sides":1025,"center":[0,0],"radius":1}`:                   "3-1024",
		`{"sides":5,"center":[0,0]}`:                                 "needs center and radius",
		`{"sides":5}`:                                                "give center and radius, or edge",
		`{"sides":5,"center":[0,0],"radius":1,"edge":[[0,0],[1,0]]}`: "not both",
		`{"sides":5,"edge":[[0,0],[0,0]]}`:                           "must differ",
		`{"sides":5,"edge":[[0,0]]}`:                                 "two points",
		`{"sides":5,"center":[0,0],"radius":1,"mode":"outside"}`:     "inscribed or circumscribed",
		`{"sides":5,"center":[0,0],"radius":0}`:                      "positive",
		`{"sides":5,"center":[0,0],"radius":1e13}`:                   "1e+12",
	} {
		r.fails("add_polygon", args, want)
	}
	m := r.must("batch", `{"ops":[{"op":"add_polygon","id":"p","args":{"sides":8,"center":[0,0],"radius":2}},{"op":"move","args":{"handles":["$p"],"dx":100,"dy":0}}]}`)
	p := r.s.Drawing().Entity(mustHandle(t, m["handles"].(map[string]any)["p"].(string))).(*doc.LWPolyline)
	if len(p.Points) != 8 || p.Bounds().Min.X < 97 {
		t.Fatalf("batch polygon = %+v", p.Bounds())
	}
	if r.s.Drawing().Len() != 5 {
		t.Fatalf("len = %d", r.s.Drawing().Len())
	}
}

func TestPolygonArgumentsAndBatchAtomicity(t *testing.T) {
	for _, tc := range []struct{ args, want string }{
		{`{"center":[0,0],"radius":1}`, `missing required field "sides"`},
		{`{"sides":3.5,"center":[0,0],"radius":1}`, "cannot unmarshal"},
		{`{"sides":3,"center":[0,0],"radius":1,"unknown":true}`, "unknown field"},
		{`{"sides":3,"center":[1e13,0],"radius":1}`, "1e+12"},
		{`{"sides":3,"edge":[[0,0],[1e13,0]]}`, "1e+12"},
		{`{"sides":3,"center":[0,0],"radius":1,"rotation":1e13}`, "1e+12"},
		{`{"sides":3,"edge":[[0,0],[1,0]],"rotation":0}`, "not both"},
		{`{"sides":3,"edge":[[0,0],[1,0]],"mode":"inscribed"}`, "not both"},
	} {
		t.Run(tc.args, func(t *testing.T) {
			r := newRig(t)
			r.fails("add_polygon", tc.args, tc.want)
			r.fails("batch", fmt.Sprintf(`{"ops":[{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":0}},{"op":"add_polygon","args":%s}]}`, tc.args), "ops[1]")
			if r.s.Drawing().Len() != 0 || r.s.Drawing().UndoDepth() != 0 {
				t.Fatal("rejected polygon changed the drawing or undo history")
			}
		})
	}
}

func TestPolygonUndoAndBatchUndo(t *testing.T) {
	r := newRig(t)
	r.must("add_polygon", `{"sides":3,"edge":[[0,0],[10,0]]}`)
	if r.s.Drawing().UndoDepth() != 1 {
		t.Fatal("standalone polygon did not make one undo step")
	}
	r.must("undo", `{}`)
	if r.s.Drawing().Len() != 0 {
		t.Fatal("undo left the standalone polygon")
	}
	r.must("batch", `{"ops":[{"op":"add_polygon","id":"p","args":{"sides":4,"center":[0,0],"radius":2,"mode":"circumscribed","rotation":30}},{"op":"add_polygon","args":{"sides":3,"edge":[[0,0],[10,0]]}},{"op":"move","args":{"handles":["$p"],"dx":100,"dy":0}}]}`)
	if r.s.Drawing().Len() != 2 || r.s.Drawing().UndoDepth() != 1 {
		t.Fatal("polygon batch did not create two entities in one undo step")
	}
	r.must("undo", `{}`)
	if r.s.Drawing().Len() != 0 {
		t.Fatal("undo left entities from the polygon batch")
	}
}
