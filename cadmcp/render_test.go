package cadmcp_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"cad-lite/cadmcp"
	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/render"

	"agentmanager/mcp"
)

var fakePNG = []byte("\x89PNG fake")

func renderRig(t *testing.T, withSnapshot bool) (*cmdsys.Session, func(string) mcp.Result, *cadmcp.SnapshotRequest) {
	t.Helper()
	v := render.DefaultView(800, 600)
	v.Center.X, v.Center.Y, v.Scale = 10, 20, 4
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	got := &cadmcp.SnapshotRequest{}
	env := cadmcp.Env{Dispatch: &cadmcp.DirectDispatcher{}, Session: func() *cmdsys.Session { return s }}
	if withSnapshot {
		env.Snapshot = func(_ *cmdsys.Session, r cadmcp.SnapshotRequest) ([]byte, error) { *got = r; return fakePNG, nil }
	}
	var tool mcp.Tool
	for _, tl := range cadmcp.Tools(env) {
		if tl.Name == "render_png" {
			tool = tl
		}
	}
	if tool.Handler == nil {
		t.Fatal("no render_png tool")
	}
	return s, func(args string) mcp.Result {
		res, err := tool.Handler(context.Background(), json.RawMessage(args))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}, got
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6*math.Max(1, math.Abs(b)) }

func TestRenderPNGDefaultsAndResult(t *testing.T) {
	s, call, got := renderRig(t, true)
	s.Script("LINE 0,0 100,0  LINE 0,0 0,50 ")
	res := call(`{}`)
	if res.IsError || len(res.Content) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if c := res.Content[0]; c.Type != "image" || c.MIMEType != "image/png" || c.Data != base64.StdEncoding.EncodeToString(fakePNG) {
		t.Fatalf("image content = %+v", c)
	}
	var info struct {
		Width, Height int
		View          struct {
			Center []float64
			Scale  float64
		}
		BBox         []float64
		PixelToModel string `json:"pixel_to_model"`
		Count        int    `json:"entity_count_visible"`
	}
	if err := json.Unmarshal([]byte(res.Content[1].Text), &info); err != nil {
		t.Fatal(err)
	}
	if info.Width != 1024 || info.Height != 768 || got.Width != 1024 || got.Height != 768 || !got.Grid || !got.Rulers {
		t.Fatalf("defaults: info=%+v req=%+v", info, got)
	}
	if info.Count != 2 || !strings.Contains(info.PixelToModel, "maxy") || len(info.BBox) != 4 {
		t.Fatalf("info = %+v", info)
	}
	if !near(info.View.Center[0], 50) || !near(info.View.Center[1], 25) {
		t.Fatalf("extents centre = %v", info.View.Center)
	}
	vis := got.View.Visible()
	if vis.Min.X > 0 || vis.Max.X < 100 || vis.Min.Y > 0 || vis.Max.Y < 50 {
		t.Fatalf("extents view does not show the drawing: %+v", vis)
	}
	if !near(info.BBox[0], vis.Min.X) || !near(info.BBox[3], vis.Max.Y) {
		t.Fatalf("bbox %v != visible %v", info.BBox, vis)
	}
}

func TestRenderPNGViews(t *testing.T) {
	s, call, got := renderRig(t, true)
	if res := call(`{"extents":true}`); res.IsError || got.View.Center.X != 0 || got.View.Scale != 1 {
		t.Fatalf("empty drawing should give the origin view: %+v %+v", res, got.View)
	}
	s.Script("LINE 0,0 100,0 ")
	call(`{"width":200,"height":100,"bbox":[0,0,200,100]}`)
	if v := got.View; v.Width != 200 || !near(v.Scale, 1) || !near(v.Center.X, 100) || !near(v.Center.Y, 50) {
		t.Fatalf("bbox view = %+v", v)
	}
	call(`{"center":[5,6],"scale":2.5}`)
	if v := got.View; v.Center.X != 5 || v.Center.Y != 6 || v.Scale != 2.5 {
		t.Fatalf("center/scale view = %+v", v)
	}
	call(`{"center":[5,6],"zoom":2}`)
	if v := got.View; v.Scale != 8 {
		t.Fatalf("zoom view = %+v (UI scale is 4)", v)
	}
	call(`{"zoom":0.5}`)
	if v := got.View; v.Center.X != 10 || v.Center.Y != 20 || v.Scale != 2 {
		t.Fatalf("zoom without center = %+v", v)
	}
	call(`{"use_view":true,"width":400,"height":300}`)
	// the UI shows 200x150 model units; 400x300 px is scale 2.
	if v := got.View; !near(v.Scale, 2) || !near(v.Center.X, 10) || !near(v.Center.Y, 20) {
		t.Fatalf("use_view = %+v", v)
	}
	call(`{"grid":false,"rulers":false}`)
	if got.Grid || got.Rulers {
		t.Fatalf("flags not honoured: %+v", got)
	}
	h := s.Drawing().Handles()[0].String()
	call(`{"highlight":["` + h + `"]}`)
	if len(got.Highlight) != 1 || got.Highlight[0].String() != h {
		t.Fatalf("highlight = %v", got.Highlight)
	}
}

func TestRenderPNGErrors(t *testing.T) {
	_, call, _ := renderRig(t, true)
	for args, want := range map[string]string{
		`{"width":10}`:                        "between 64 and 2048",
		`{"height":5000}`:                     "between 64 and 2048",
		`{"extents":true,"use_view":true}`:    "at most one",
		`{"bbox":[0,0,1,1],"center":[0,0]}`:   "at most one",
		`{"bbox":[0,0,1]}`:                    "bbox must",
		`{"bbox":[5,5,1,9]}`:                  "bbox",
		`{"bbox":[1,1,1,1]}`:                  "bbox",
		`{"center":[0,0],"scale":0}`:          "scale",
		`{"center":[0,0],"scale":1,"zoom":1}`: "not both",
		`{"zoom":-1}`:                         "zoom",
		`{"center":[1]}`:                      "center must",
		`{"nope":1}`:                          "invalid arguments",
		`{"highlight":["ZZZZ"]}`:              "invalid handle",
		`{"highlight":["FFFF"]}`:              "no entity",
		`{"extents":false,"use_view":false}`:  "", // valid
	} {
		res := call(args)
		if want == "" {
			if res.IsError {
				t.Errorf("%s: unexpected error %v", args, res.Content[0].Text)
			}
			continue
		}
		if !res.IsError || !strings.Contains(res.Content[0].Text, want) {
			t.Errorf("%s: got error=%v %q, want %q", args, res.IsError, res.Content[0].Text, want)
		}
	}
}

func TestRenderPNGNoSnapshot(t *testing.T) {
	_, call, _ := renderRig(t, false)
	res := call(`{}`)
	if !res.IsError || res.Content[0].Text != "rendering not available" {
		t.Fatalf("result = %+v", res)
	}
}
