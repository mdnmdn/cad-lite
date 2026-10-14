package cadmcp_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestAddText(t *testing.T) {
	r := newRig(t)
	tx := entityOf[*doc.Text](t, r, r.must("add_text", `{"value":"Kitchen","point":[10,20],"height":2.5,"rotation":90,"justify":"MC","style":"standard"}`))
	if tx.Anchor() != geom.V(10, 20) || tx.Justify() != "MC" || tx.Style != "Standard" || math.Abs(tx.Rotation-math.Pi/2) > 1e-12 || tx.Height != 2.5 {
		t.Fatalf("text = %+v", tx)
	}
	if c := tx.Layout().Corners; math.Abs((c[0].Y+c[2].Y)/2-20) > 1e-9 {
		t.Fatalf("MC text not centred on its point: %v", c)
	}
	al := entityOf[*doc.Text](t, r, r.must("add_text", `{"value":"ABC","point":[0,0],"second_point":[30,0],"justify":"Align"}`))
	if al.Justify() != "Align" || math.Abs(al.Layout().Width-30) > 1e-9 || al.Height <= 0 {
		t.Fatalf("aligned = %+v width %v", al, al.Layout().Width)
	}
	fit := entityOf[*doc.Text](t, r, r.must("add_text", `{"value":"ABC","point":[0,0],"second_point":[0,30],"justify":"F","height":2}`))
	if fit.Justify() != "Fit" || math.Abs(fit.Layout().Rotation-math.Pi/2) > 1e-12 || fit.Layout().Height != 2 {
		t.Fatalf("fit = %+v", fit.Layout())
	}
	l := r.must("add_text", `{"value":"%%c50","point":[0,0],"height":1}`)
	if e := entityOf[*doc.Text](t, r, l); e.Justify() != "Left" || e.Insert != geom.V(0, 0) || e.Value != "%%c50" {
		t.Fatalf("left = %+v", e)
	}
	for args, want := range map[string]string{
		`{"value":"a\nb","point":[0,0],"height":1}`:                                                "single line",
		`{"value":"","point":[0,0],"height":1}`:                                                    "empty",
		`{"value":"a","point":[0,0]}`:                                                              "height is required",
		`{"value":"a","point":[0,0],"height":1,"justify":"XX"}`:                                    "not an AutoCAD justification",
		`{"value":"a","point":[0,0],"justify":"Align"}`:                                            "needs second_point",
		`{"value":"a","point":[0,0],"second_point":[1,0],"justify":"Align","height":1}`:            "omit height",
		`{"value":"a","point":[0,0],"second_point":[1,0],"justify":"Fit","height":1,"rotation":5}`: "omit rotation",
		`{"value":"a","point":[0,0],"second_point":[1,0],"height":1}`:                              "only for justify Align and Fit",
		`{"value":"a","point":[0,0],"height":1,"style":"Fancy"}`:                                   `text style "Fancy" does not exist`,
		`{"value":"a","point":[0,0],"height":1e13}`:                                                "1e+12",
		`{"value":"a","point":[0,0],"height":1,"width_factor":0}`:                                  "width_factor",
		`{"value":"a\u0007","point":[0,0],"height":1}`:                                             "control character",
	} {
		r.fails("add_text", args, want)
	}
	if r.s.Drawing().Len() != 4 {
		t.Fatalf("refused calls added entities: %d", r.s.Drawing().Len())
	}
}

func TestAddMTextEscapesUnlessRaw(t *testing.T) {
	r := newRig(t)
	m := entityOf[*doc.MText](t, r, r.must("add_mtext", `{"value":"a\\b {c}\nsecond line","point":[0,10],"height":2,"width":40,"attachment":"BR","rotation":30}`))
	if m.Value != `a\\b \{c\}\Psecond line` || m.Attachment != doc.MTextBottomRight || m.Width != 40 || math.Abs(m.Rotation-math.Pi/6) > 1e-12 {
		t.Fatalf("mtext = %+v", m)
	}
	if got := doc.MTextPlain(m.Value); got != "a\\b {c}\nsecond line" {
		t.Fatalf("plain = %q", got)
	}
	raw := entityOf[*doc.MText](t, r, r.must("add_mtext", `{"value":"{\\H2x;Big}\\Pnext","point":[0,0],"height":1,"raw":true}`))
	if raw.Value != `{\H2x;Big}\Pnext` || doc.MTextPlain(raw.Value) != "Big\nnext" {
		t.Fatalf("raw = %q", raw.Value)
	}
	r.fails("add_mtext", `{"value":"x","point":[0,0],"height":1,"attachment":"Left"}`, "attachment")
	r.fails("add_mtext", `{"value":"x","point":[0,0],"height":1,"width":-1}`, "width")
}

func TestUpdateText(t *testing.T) {
	r := newRig(t)
	h := str(r.must("add_text", `{"value":"Old","point":[5,5],"height":1}`), "handle")
	mh := str(r.must("add_mtext", `{"value":"Notes","point":[0,0],"height":1}`), "handle")
	line := str(r.must("add_line", `{"x1":0,"y1":0,"x2":1,"y2":1}`), "handle")
	u := r.must("update_text", `{"handle":"`+h+`","value":"New","height":3,"justify":"TR"}`)
	if u["value"] != "New" || u["justify"] != "TR" || u["height"] != 3.0 || u["point"].([]any)[0] != 5.0 {
		t.Fatalf("updated = %v", u)
	}
	r.must("update_text", `{"handle":"`+h+`","point":[50,60],"rotation":45}`)
	tx := r.s.Drawing().Entity(mustHandle(t, h)).(*doc.Text)
	if tx.Anchor() != geom.V(50, 60) || math.Abs(tx.Rotation-math.Pi/4) > 1e-12 {
		t.Fatalf("moved = %+v", tx)
	}
	um := r.must("update_text", `{"handle":"`+mh+`","value":"{x}","width":12,"attachment":"MC"}`)
	if um["value"] != "{x}" || um["raw"] != `\{x\}` || um["attachment"] != "MC" || um["width"] != 12.0 {
		t.Fatalf("mtext = %v", um)
	}
	depth := r.s.Drawing().UndoDepth()
	before := snapshot(r)
	r.fails("update_text", `{"handle":"`+h+`","width":3}`, "apply to MTEXT")
	r.fails("update_text", `{"handle":"`+mh+`","justify":"Left"}`, "apply to TEXT")
	r.fails("update_text", `{"handle":"`+line+`","value":"x"}`, "needs TEXT or MTEXT")
	r.fails("update_text", `{"handle":"FFFF","value":"x"}`, "no entity")
	r.fails("update_text", `{"handle":"`+h+`"}`, "at least one field")
	r.fails("update_text", `{"handle":"`+h+`","value":"two\nlines"}`, "single line")
	r.fails("update_text", `{"handle":"`+h+`","second_point":[1,1]}`, "only for justify Align and Fit")
	if snapshot(r) != before || r.s.Drawing().UndoDepth() != depth {
		t.Fatal("a refused update changed the drawing")
	}
	r.must("undo", `{}`)
	if m := r.s.Drawing().Entity(mustHandle(t, mh)).(*doc.MText); m.Value != "Notes" {
		t.Fatalf("undo: %q", m.Value)
	}
}

func TestListEntitiesTextFilter(t *testing.T) {
	r := newRig(t)
	r.must("add_text", `{"value":"Living ROOM","point":[0,0],"height":1}`)
	r.must("add_mtext", `{"value":"Bed{room}\nnorth","point":[0,0],"height":1}`)
	r.must("add_text", `{"value":"Hall","point":[0,0],"height":1}`)
	r.must("add_line", `{"x1":0,"y1":0,"x2":1,"y2":1}`)
	if got := r.must("list_entities", `{"text":"room"}`); got["total"] != 2.0 {
		t.Fatalf("text filter = %v", got)
	}
	l := r.must("list_entities", `{"text":"ROOM","class":"mtext"}`)
	es := l["entities"].([]any)
	if l["total"] != 1.0 || es[0].(map[string]any)["value"] != "Bed{room}\nnorth" {
		t.Fatalf("mtext filter = %v", l)
	}
	e := r.must("list_entities", `{"class":"TEXT","text":"hall"}`)["entities"].([]any)[0].(map[string]any)
	for _, k := range []string{"value", "height", "rotation", "justify", "point", "box", "style", "width_factor"} {
		if _, ok := e[k]; !ok {
			t.Fatalf("TEXT info lacks %s: %v", k, e)
		}
	}
	if len(e["box"].([]any)) != 4 {
		t.Fatalf("box = %v", e["box"])
	}
}

func TestBatchPlacesLabelledRooms(t *testing.T) {
	r := newRig(t)
	r.must("create_layer", `{"name":"labels"}`)
	m := r.must("batch", `{"ops":[
		{"op":"add_rectangle","id":"r1","args":{"origin":[0,0],"width":400,"height":300}},
		{"op":"add_rectangle","id":"r2","args":{"origin":[400,0],"width":300,"height":300}},
		{"op":"add_text","id":"l1","args":{"value":"Living","point":[200,150],"height":20,"justify":"MC","layer":"labels"}},
		{"op":"add_text","id":"l2","args":{"value":"Bed","point":[550,150],"height":20,"justify":"MC","layer":"labels"}},
		{"op":"add_mtext","id":"n","args":{"value":"Area 12 m2\nfloor: oak","point":[0,-20],"height":10,"width":200}},
		{"op":"update_text","args":{"handle":"$l2","value":"Bedroom","height":25}},
		{"op":"move","args":{"handles":["$l1","$l2"],"dx":0,"dy":10}}
	]}`)
	hs := m["handles"].(map[string]any)
	if len(hs) != 5 || r.s.Drawing().UndoDepth() != 1 {
		t.Fatalf("batch = %v", m)
	}
	l2 := r.s.Drawing().Entity(mustHandle(t, hs["l2"].(string))).(*doc.Text)
	if l2.Value != "Bedroom" || l2.Height != 25 || l2.Anchor() != geom.V(550, 160) || l2.Common().Layer != "labels" {
		t.Fatalf("l2 = %+v", l2)
	}
	if got := r.must("list_entities", `{"text":"bed"}`)["total"]; got != 1.0 {
		t.Fatalf("find = %v", got)
	}
	r.must("undo", `{}`)
	if r.s.Drawing().Len() != 0 {
		t.Fatal("one UNDO did not remove the rooms")
	}

	// Rollback: a bad justify, while decoding and while checking against the drawing, changes
	// nothing and leaves the user's command running.
	r.must("redo", `{}`)
	before := snapshot(r)
	r.s.Run("LINE")
	l1 := hs["l1"].(string)
	for body, want := range map[string]string{
		`{"ops":[{"op":"add_text","id":"x","args":{"value":"Hall","point":[0,0],"height":5}},{"op":"update_text","args":{"handle":"$x","justify":"Sideways"}}]}`:          `ops[1] (update_text): justify "Sideways"`,
		`{"ops":[{"op":"add_text","args":{"value":"Hall","point":[0,0],"height":5,"justify":"QQ"}}]}`:                                                                     `ops[0] (add_text): justify "QQ"`,
		`{"ops":[{"op":"add_text","id":"x","args":{"value":"Hall","point":[0,0],"height":5}},{"op":"update_text","args":{"handle":"` + l1 + `","attachment":"TL"}}]}`:     "ops[1] (update_text): " + l1 + ": width and attachment apply to MTEXT",
		`{"ops":[{"op":"update_text","args":{"handle":"$n","justify":"Align","rotation":10}}]}`:                                                                           `"$n" names no earlier`,
		`{"ops":[{"op":"add_text","id":"x","args":{"value":"Hall","point":[0,0],"height":5}},{"op":"update_text","args":{"handle":"$x","justify":"Fit","rotation":10}}]}`: "Align and Fit take the rotation",
		`{"ops":[{"op":"update_text","args":{"handle":"` + l1 + `","style":"Nope"}}]}`:                                                                                    `text style "Nope" does not exist`,
	} {
		r.fails("batch", body, want)
		if snapshot(r) != before {
			t.Fatalf("%s: the drawing changed", want)
		}
	}
	if r.s.Active() != "LINE" {
		t.Fatalf("a refused batch cancelled LINE (active %q)", r.s.Active())
	}
	if !strings.Contains(snapshot(r), "redo=0") {
		t.Fatal("undo state changed")
	}
}
