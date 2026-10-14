package cadmcp_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-draw/cadmcp"
	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/render"

	"agentmanager/mcp"
)

type rig struct {
	t       *testing.T
	s       *cmdsys.Session
	d       *cadmcp.Dispatcher
	tools   map[string]mcp.Tool
	changed atomic.Int32
	stop    chan struct{}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	v := render.DefaultView(800, 600)
	r := &rig{t: t, s: cmdsys.NewSession(doc.New(), &v, commands.NewRegistry()), d: cadmcp.NewDispatcher(nil), tools: map[string]mcp.Tool{}, stop: make(chan struct{})}
	go func() {
		for {
			select {
			case <-r.stop:
				return
			case <-time.After(time.Millisecond):
				r.d.Drain()
			}
		}
	}()
	t.Cleanup(func() { close(r.stop); r.d.Close() })
	for _, tl := range cadmcp.Tools(cadmcp.Env{Dispatch: r.d, Session: func() *cmdsys.Session { return r.s }, Changed: func() { r.changed.Add(1) }}) {
		r.tools[tl.Name] = tl
	}
	return r
}

// call runs a tool and returns its decoded JSON result; ok is false for error results.
func (r *rig) call(name, args string) (m map[string]any, text string, ok bool) {
	r.t.Helper()
	tl, found := r.tools[name]
	if !found {
		r.t.Fatalf("no tool %s", name)
	}
	res, err := tl.Handler(context.Background(), json.RawMessage(args))
	if err != nil {
		r.t.Fatalf("%s: handler error %v", name, err)
	}
	text = res.Content[0].Text
	if res.IsError {
		return nil, text, false
	}
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		r.t.Fatalf("%s: bad JSON %q", name, text)
	}
	return m, text, true
}

func (r *rig) must(name, args string) map[string]any {
	r.t.Helper()
	m, text, ok := r.call(name, args)
	if !ok {
		r.t.Fatalf("%s %s: error %s", name, args, text)
	}
	return m
}

func (r *rig) fails(name, args, want string) {
	r.t.Helper()
	_, text, ok := r.call(name, args)
	if ok || !strings.Contains(text, want) {
		r.t.Fatalf("%s %s: ok=%v text=%q, want error containing %q", name, args, ok, text, want)
	}
}

func str(m map[string]any, k string) string  { s, _ := m[k].(string); return s }
func num(m map[string]any, k string) float64 { f, _ := m[k].(float64); return f }

func TestToolListAndSchemas(t *testing.T) {
	r := newRig(t)
	if len(r.tools) < 20 {
		t.Fatalf("only %d tools", len(r.tools))
	}
	for n, tl := range r.tools {
		var sc map[string]any
		if err := json.Unmarshal(tl.InputSchema, &sc); err != nil || sc["type"] != "object" {
			t.Errorf("%s: bad schema %s", n, tl.InputSchema)
		}
		if tl.Description == "" || tl.Handler == nil {
			t.Errorf("%s: missing description or handler", n)
		}
	}
	if cadmcp.Instructions() == "" {
		t.Error("empty instructions")
	}
}

func TestAddShapesOneUndoStepEach(t *testing.T) {
	r := newRig(t)
	line := r.must("add_line", `{"x1":0,"y1":0,"x2":10,"y2":0}`)
	if str(line, "class") != "LINE" || str(line, "layer") != "0" || str(line, "handle") == "" {
		t.Fatalf("line = %v", line)
	}
	r.must("add_circle", `{"cx":5,"cy":5,"r":2}`)
	r.must("add_arc", `{"p1":[0,0],"p2":[1,1],"p3":[2,0]}`)
	r.must("add_polyline", `{"points":[[0,0],[4,0],[4,3]],"closed":true}`)
	r.must("add_rectangle", `{"x1":0,"y1":0,"x2":6,"y2":4}`)
	if r.s.Drawing().Len() != 5 {
		t.Fatalf("len = %d", r.s.Drawing().Len())
	}
	if r.changed.Load() != 5 {
		t.Fatalf("changed called %d times", r.changed.Load())
	}
	u := r.must("undo", `{"steps":2}`)
	if num(u, "steps_done") != 2 || r.s.Drawing().Len() != 3 || u["can_redo"] != true {
		t.Fatalf("undo = %v len=%d", u, r.s.Drawing().Len())
	}
	r.must("redo", `{}`)
	if r.s.Drawing().Len() != 4 {
		t.Fatalf("len after redo = %d", r.s.Drawing().Len())
	}
	r.must("undo", `{"steps":100}`)
	if r.s.Drawing().Len() != 0 {
		t.Fatal("not fully undone")
	}
	if u := r.must("undo", `{}`); num(u, "steps_done") != 0 || u["can_undo"] != false {
		t.Fatalf("undo past start = %v", u)
	}
}

func TestAddValidation(t *testing.T) {
	r := newRig(t)
	r.fails("add_line", `{"x1":1,"y1":1,"x2":1,"y2":1}`, "differ")
	r.fails("add_line", `{"x1":1}`, "missing required")
	r.fails("add_line", `{"x1":0,"y1":0,"x2":1,"y2":1,"bogus":1}`, "invalid arguments")
	r.fails("add_line", `{"x1":"a","y1":0,"x2":1,"y2":1}`, "invalid arguments")
	r.fails("add_line", `{"x1":0,"y1":0,"x2":1,"y2":1,"layer":"nope"}`, "does not exist")
	r.fails("add_circle", `{"cx":0,"cy":0,"r":0}`, "positive")
	r.fails("add_arc", `{"p1":[0,0],"p2":[1,0],"p3":[2,0]}`, "collinear")
	r.fails("add_arc", `{"p1":[0,0],"p2":[1],"p3":[2,0]}`, "invalid arguments")
	r.fails("add_polyline", `{"points":[[0,0]]}`, "at least 2")
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":0,"y2":5}`, "non-zero")
	r.fails("erase", `{"handles":[]}`, "empty")
	r.fails("erase", `{"handles":["zz"]}`, "invalid handle")
	r.fails("erase", `{"handles":["FFFF"]}`, "no entity")
	r.fails("set_layer", `{"name":"ghost"}`, "does not exist")
	r.fails("create_layer", `{"name":"a","color":300}`, "1-255")
	r.fails("create_layer", `{"name":"a=b"}`, "'='")
	r.fails("zoom_extents", `{}`, "empty")
	r.fails("undo", `{"steps":0}`, "1-100")
	r.fails("list_entities", `{"limit":500}`, "limit")
	r.fails("measure", `{}`, "handles and/or points")
	if r.s.Drawing().Len() != 0 {
		t.Fatal("failed calls changed the drawing")
	}
	if _, _, ok := r.call("add_line", ``); ok {
		t.Fatal("empty args should fail validation, not panic")
	}
}

func TestEditByHandle(t *testing.T) {
	r := newRig(t)
	h := str(r.must("add_line", `{"x1":0,"y1":0,"x2":10,"y2":0}`), "handle")
	c := str(r.must("add_circle", `{"cx":0,"cy":0,"r":1}`), "handle")
	r.must("move", `{"handles":["`+h+`","`+c+`"],"dx":5,"dy":2}`)
	ln := r.s.Drawing().Entity(mustHandle(t, h)).(*doc.Line)
	if ln.Start.X != 5 || ln.Start.Y != 2 {
		t.Fatalf("moved line = %+v", ln)
	}
	r.must("rotate", `{"handles":["`+h+`"],"cx":5,"cy":2,"angle":90}`)
	if math.Abs(ln.End.X-5) > 1e-9 || math.Abs(ln.End.Y-12) > 1e-9 {
		t.Fatalf("rotated line end = %+v", ln.End)
	}
	r.must("scale", `{"handles":["`+c+`"],"cx":5,"cy":2,"factor":3}`)
	if ci := r.s.Drawing().Entity(mustHandle(t, c)).(*doc.Circle); math.Abs(ci.Radius-3) > 1e-9 {
		t.Fatalf("scaled radius = %v", ci.Radius)
	}
	r.fails("scale", `{"handles":["`+c+`"],"cx":0,"cy":0,"factor":0}`, "positive")
	r.fails("scale", `{"handles":["`+c+`"],"cx":0,"cy":0,"factor":-1}`, "positive")
	r.fails("move", `{"handles":["`+c+`","FFFF"],"dx":1,"dy":1}`, "no entity")

	r.must("undo", `{}`)
	if ci := r.s.Drawing().Entity(mustHandle(t, c)).(*doc.Circle); ci.Radius != 1 {
		t.Fatalf("undo scale radius = %v", ci.Radius)
	}
	e := r.must("erase", `{"handles":["`+h+`","`+h+`"]}`)
	if num(e, "count") != 1 || r.s.Drawing().Entity(mustHandle(t, h)) != nil {
		t.Fatalf("erase = %v", e)
	}
	r.must("undo", `{}`)
	if r.s.Drawing().Entity(mustHandle(t, h)) == nil {
		t.Fatal("erase not undone")
	}
}

func mustHandle(t *testing.T, s string) doc.Handle {
	t.Helper()
	h, err := doc.ParseHandle(s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestLayers(t *testing.T) {
	r := newRig(t)
	m := r.must("create_layer", `{"name":"walls","color":4}`)
	if m["created"] != true || m["current_layer"] != "0" {
		t.Fatalf("create = %v", m)
	}
	r.must("set_layer", `{"name":"walls"}`)
	h := str(r.must("add_line", `{"x1":0,"y1":0,"x2":1,"y2":1}`), "handle")
	if e := r.s.Drawing().Entity(mustHandle(t, h)); e.Common().Layer != "walls" {
		t.Fatalf("layer = %s", e.Common().Layer)
	}
	r.must("create_layer", `{"name":"aux","color":2,"locked":true}`)
	r.must("set_entity_layer", `{"handles":["`+h+`"],"layer":"aux"}`)
	r.fails("move", `{"handles":["`+h+`"],"dx":1,"dy":1}`, "locked")
	r.fails("set_entity_layer", `{"handles":["`+h+`"],"layer":"zzz"}`, "does not exist")
	r.must("create_layer", `{"name":"aux","locked":false,"make_current":true}`)
	d := r.must("get_drawing", `{}`)
	if d["current_layer"] != "aux" || len(d["layers"].([]any)) != 3 {
		t.Fatalf("drawing = %v", d)
	}
	if l := r.s.Drawing().Layer("aux"); l.Locked || l.Color != 2 {
		t.Fatalf("aux = %+v", l)
	}
	r.must("move", `{"handles":["`+h+`"],"dx":1,"dy":1}`)
}

func TestRunScriptCapturesOutputAndHandles(t *testing.T) {
	r := newRig(t)
	m := r.must("run_script", `{"script":"LAYER walls=4 CIRCLE 10,10 5\nLINE 0,0 10,0 "}`)
	if num(m, "entities_before") != 0 || num(m, "entities_after") != 2 || len(m["new_handles"].([]any)) != 2 {
		t.Fatalf("script result = %v", m)
	}
	if m["cancelled_command"] != "" || m["prompt"] != "Command:" {
		t.Fatalf("script result = %v", m)
	}
	out := m["output"].([]any)
	if len(out) == 0 || !strings.Contains(out[0].(string), "LAYER") {
		t.Fatalf("output = %v", out)
	}
	if r.s.Drawing().Layer("walls") == nil {
		t.Fatal("layer not created")
	}

	m = r.must("run_script", `{"script":"LINE 0,0 5,5"}`)
	if m["cancelled_command"] != "LINE" || num(m, "entities_after") != 3 {
		t.Fatalf("unfinished script = %v", m)
	}
	if r.s.Active() != "" {
		t.Fatal("command left active")
	}

	m = r.must("run_script", `{"script":"NOSUCH"}`)
	if o := m["output"].([]any); !strings.Contains(o[len(o)-1].(string), "Unknown command") {
		t.Fatalf("output = %v", o)
	}
	r.fails("run_script", `{}`, "")
}

func TestRunScriptReturnsExactlyItsOutputPastHistoryCap(t *testing.T) {
	r := newRig(t)
	for range 150 {
		r.s.Submit("NOSUCH")
	}
	var b strings.Builder
	for range 60 {
		b.WriteString("NOSUCH ")
	}
	m := r.must("run_script", `{"script":"`+strings.TrimSpace(b.String())+`"}`)
	out := m["output"].([]any)
	if len(out) != 60 || m["output_truncated"] != false {
		t.Fatalf("got %d lines truncated=%v", len(out), m["output_truncated"])
	}
	if !strings.Contains(out[0].(string), "NOSUCH") {
		t.Fatalf("first = %v", out[0])
	}
}

func TestRunScriptCancelsPreviousCommand(t *testing.T) {
	r := newRig(t)
	r.s.Run("LINE")
	r.must("run_script", `{"script":"CIRCLE 0,0 1"}`)
	if r.s.Drawing().Len() != 1 || r.s.Active() != "" {
		t.Fatalf("len=%d active=%q", r.s.Drawing().Len(), r.s.Active())
	}
}

func TestRunCommandStepwise(t *testing.T) {
	r := newRig(t)
	m := r.must("run_command", `{"input":"CIRCLE"}`)
	if m["active_command"] != "CIRCLE" || !strings.Contains(m["prompt"].(string), "center") && !strings.Contains(m["prompt"].(string), "Specify") {
		t.Fatalf("m = %v", m)
	}
	r.must("run_command", `{"input":"0,0"}`)
	m = r.must("run_command", `{"input":"3"}`)
	if m["active_command"] != "" || r.s.Drawing().Len() != 1 {
		t.Fatalf("m = %v", m)
	}
	r.fails("run_command", `{"input":"a\nb"}`, "single line")
	m = r.must("list_commands", `{}`)
	if !strings.Contains(string(mustJSON(t, m["commands"])), `"LINE"`) {
		t.Fatalf("commands = %v", m)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQueryAndSelection(t *testing.T) {
	r := newRig(t)
	r.must("create_layer", `{"name":"b"}`)
	r.must("add_line", `{"x1":0,"y1":0,"x2":10,"y2":0}`)
	c := str(r.must("add_circle", `{"cx":100,"cy":100,"r":5,"layer":"b"}`), "handle")
	r.must("add_arc", `{"p1":[0,0],"p2":[1,1],"p3":[2,0]}`)
	r.must("add_rectangle", `{"x1":0,"y1":0,"x2":6,"y2":4}`)

	d := r.must("get_drawing", `{}`)
	counts := d["entity_counts"].(map[string]any)
	if d["entity_count"] != 4.0 || counts["LINE"] != 1.0 || counts["LWPOLYLINE"] != 1.0 {
		t.Fatalf("drawing = %v", d)
	}
	if ex := d["extents"].([]any); ex[2] != 105.0 || ex[3] != 105.0 {
		t.Fatalf("extents = %v", ex)
	}

	l := r.must("list_entities", `{"limit":2}`)
	if l["total"] != 4.0 || len(l["entities"].([]any)) != 2 {
		t.Fatalf("list = %v", l)
	}
	if l = r.must("list_entities", `{"offset":3}`); len(l["entities"].([]any)) != 1 {
		t.Fatalf("offset list = %v", l)
	}
	l = r.must("list_entities", `{"layer":"b"}`)
	e := l["entities"].([]any)[0].(map[string]any)
	if l["total"] != 1.0 || e["class"] != "CIRCLE" || e["radius"] != 5.0 || e["handle"] != c {
		t.Fatalf("layer list = %v", l)
	}
	if l = r.must("list_entities", `{"class":"line"}`); l["total"] != 1.0 {
		t.Fatalf("class list = %v", l)
	}
	if l = r.must("list_entities", `{"bbox":[90,90,110,110]}`); l["total"] != 1.0 {
		t.Fatalf("bbox list = %v", l)
	}
	if l = r.must("list_entities", `{"bbox":[-1,-1,1,1]}`); l["total"] != 3.0 {
		t.Fatalf("bbox list = %v", l)
	}
	r.fails("list_entities", `{"layer":"x"}`, "does not exist")
	r.fails("list_entities", `{"bbox":[1,2]}`, "")

	if s := r.must("set_selection", `{"handles":["`+c+`"]}`); len(s["handles"].([]any)) != 1 {
		t.Fatalf("sel = %v", s)
	}
	if r.s.Selection().Len() != 1 {
		t.Fatal("selection not set")
	}
	if g := r.must("get_selection", `{}`); g["handles"].([]any)[0] != c {
		t.Fatalf("get = %v", g)
	}
	r.must("set_selection", `{"handles":[],"add":true}`)
	if r.s.Selection().Len() != 1 {
		t.Fatal("add of nothing cleared")
	}
	r.must("set_selection", `{"handles":[]}`)
	if r.s.Selection().Len() != 0 {
		t.Fatal("not cleared")
	}
	r.fails("set_selection", `{"handles":["FFFF"]}`, "no entity")
}

func TestZoomExtents(t *testing.T) {
	r := newRig(t)
	r.must("add_rectangle", `{"x1":0,"y1":0,"x2":1000,"y2":500}`)
	m := r.must("zoom_extents", `{}`)
	if c := m["center"].([]any); c[0] != 500.0 || c[1] != 250.0 || num(m, "scale") >= 1 {
		t.Fatalf("zoom = %v", m)
	}
	if r.s.View().Center.X != 500 {
		t.Fatal("view not changed")
	}
}

func TestMeasure(t *testing.T) {
	r := newRig(t)
	rect := str(r.must("add_rectangle", `{"x1":0,"y1":0,"x2":6,"y2":4}`), "handle")
	circ := str(r.must("add_circle", `{"cx":0,"cy":0,"r":1}`), "handle")
	ln := str(r.must("add_line", `{"x1":0,"y1":0,"x2":3,"y2":4}`), "handle")
	m := r.must("measure", `{"handles":["`+rect+`","`+circ+`","`+ln+`"],"points":[[0,0],[3,4]]}`)
	if dist := m["distance"].(map[string]any); dist["distance"] != 5.0 {
		t.Fatalf("distance = %v", dist)
	}
	es := m["entities"].([]any)
	if es[0].(map[string]any)["area"] != 24.0 || es[0].(map[string]any)["length"] != 20.0 {
		t.Fatalf("rect = %v", es[0])
	}
	if a := es[1].(map[string]any)["area"].(float64); math.Abs(a-math.Pi) > 1e-9 {
		t.Fatalf("circle area = %v", a)
	}
	if _, has := es[2].(map[string]any)["area"]; has || es[2].(map[string]any)["length"] != 5.0 {
		t.Fatalf("line = %v", es[2])
	}
	if math.Abs(num(m, "total_area")-(24+math.Pi)) > 1e-9 {
		t.Fatalf("total area = %v", m["total_area"])
	}
	r.fails("measure", `{"points":[[0,0]]}`, "exactly two")
}

func TestStructuredToolFinishesRunningCommand(t *testing.T) {
	r := newRig(t)
	r.s.Run("LINE")
	r.s.Submit("0,0")
	r.must("add_circle", `{"cx":0,"cy":0,"r":1}`)
	if r.s.Active() != "" {
		t.Fatal("command still active")
	}
}

func TestDispatcherRunsOnDrainGoroutine(t *testing.T) {
	d := cadmcp.NewDispatcher(nil)
	ran := false
	done := make(chan error, 1)
	go func() { done <- d.Do(context.Background(), func() { ran = true }) }()
	for d.Pending() == 0 {
		time.Sleep(time.Millisecond)
	}
	if ran {
		t.Fatal("ran before Drain")
	}
	d.Drain()
	if err := <-done; err != nil || !ran {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
}

func TestDispatcherCancellationSkipsWork(t *testing.T) {
	d := cadmcp.NewDispatcher(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ran := false
	err := d.Do(ctx, func() { ran = true })
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v", err)
	}
	if !errorsIs(err, cadmcp.ErrUIBusy) {
		t.Fatalf("err does not wrap ErrUIBusy: %v", err)
	}
	d.Drain()
	if ran {
		t.Fatal("cancelled work ran")
	}
	if d.Pending() != 0 {
		t.Fatal("cancelled item still pending")
	}
}

func errorsIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestDispatcherCloseAndPanic(t *testing.T) {
	d := cadmcp.NewDispatcher(nil)
	done := make(chan error, 1)
	go func() { done <- d.Do(context.Background(), func() {}) }()
	for d.Pending() == 0 {
		time.Sleep(time.Millisecond)
	}
	d.Close()
	if err := <-done; !errorsIs(err, cadmcp.ErrUIBusy) {
		t.Fatalf("err = %v", err)
	}
	if err := d.Do(context.Background(), func() {}); !errorsIs(err, cadmcp.ErrUIBusy) {
		t.Fatalf("after close err = %v", err)
	}

	d2 := cadmcp.NewDispatcher(nil)
	go func() { done <- d2.Do(context.Background(), func() { panic("boom") }) }()
	for d2.Pending() == 0 {
		time.Sleep(time.Millisecond)
	}
	d2.Drain()
	if err := <-done; err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("panic err = %v", err)
	}
}

func TestToolReportsBusyUI(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	d := cadmcp.NewDispatcher(nil) // never drained
	tools := cadmcp.Tools(cadmcp.Env{Dispatch: d, Session: func() *cmdsys.Session { return s }})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	for _, tl := range tools {
		if tl.Name != "add_line" {
			continue
		}
		res, err := tl.Handler(ctx, json.RawMessage(`{"x1":0,"y1":0,"x2":1,"y2":1}`))
		if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "busy") {
			t.Fatalf("res = %+v err=%v", res, err)
		}
	}
	if s.Drawing().Len() != 0 {
		t.Fatal("cancelled tool modified the drawing")
	}
}

func TestDirectDispatcher(t *testing.T) {
	var d cadmcp.DirectDispatcher
	n := 0
	if err := d.Do(context.Background(), func() { n++ }); err != nil || n != 1 {
		t.Fatal(err, n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Do(ctx, func() { n++ }); err == nil || n != 1 {
		t.Fatal("cancelled ctx should not run")
	}
}
