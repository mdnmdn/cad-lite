package ui_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/mcp"
	"agentmanager/transcript"
	"go-draw/cadmcp"
	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/internal/gpui"
	"go-draw/render"
	"go-draw/ui"
)

var (
	fakeOnce sync.Once
	fakePath string
	fakeErr  error
)

// fakeAgentBinary builds agentmanager's fake agent (an internal package of
// the nested module) once per test binary.
func fakeAgentBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("no process spawning on js")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}
	fakeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "godraw-fakeagent-")
		if err != nil {
			fakeErr = err
			return
		}
		fakePath = filepath.Join(dir, "fakeagent")
		cmd := exec.Command(goBin, "build", "-o", fakePath, "./internal/fakeagent")
		cmd.Dir = filepath.Join("..", "agentmanager")
		if out, err := cmd.CombinedOutput(); err != nil {
			fakeErr = &buildFailure{err, string(out)}
		}
	})
	if fakeErr != nil {
		t.Fatalf("build fakeagent: %v", fakeErr)
	}
	return fakePath
}

type buildFailure struct {
	err error
	out string
}

func (e *buildFailure) Error() string { return e.err.Error() + "\n" + e.out }

func TestAgentEndToEndAddLine(t *testing.T) {
	bin := fakeAgentBinary(t)
	cases := []struct {
		harness agentmanager.Harness
		builtin string
	}{
		{agentmanager.Claude, "tool-turn"},
		{agentmanager.OpenCode, "tool-turn"},
		{agentmanager.Codex, "mcp-call-turn"},
	}
	for _, tc := range cases {
		t.Run(string(tc.harness), func(t *testing.T) {
			scenario := filepath.Join(t.TempDir(), "scenario.json")
			if err := os.WriteFile(scenario, []byte(`{"builtin":"`+tc.builtin+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FAKEAGENT_SCENARIO", scenario)
			t.Setenv("FAKEAGENT_MCP_TOOL", "add_line")
			t.Setenv("FAKEAGENT_MCP_ARGS", `{"x1":0,"y1":0,"x2":30,"y2":40}`)

			ws := ui.NewWorkspace(doc.New(), nil)
			ws.SetProjectPath(filepath.Join(t.TempDir(), "x.dxf"))
			ws.AgentPanel().Configure(tc.harness, bin)
			ws.AgentPanel().Show()
			p := gpui.NewHeadlessPlatform()
			var hw *gpui.HeadlessWindow
			if err := p.Run(func(cx *gpui.App) {
				cx.Keymap().AddBindings(ui.Bindings()...)
				if err := cx.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
					t.Fatal(err)
				}
				hw = p.Windows()[0]
			}); err != nil {
				t.Fatal(err)
			}
			hw.Pixels()
			t.Cleanup(ws.CloseAgent)

			ws.AgentPanel().Send("draw a line", hw.Window())

			toolDone := func() bool {
				for _, b := range ws.AgentPanel().Transcript().Blocks() {
					if tb, ok := b.(transcript.ToolBlock); ok && tb.Status == agentmanager.ToolCompleted {
						return true
					}
				}
				return false
			}
			deadline := time.Now().Add(30 * time.Second)
			for ws.Session().Drawing().Len() == 0 || !toolDone() || ws.AgentPanel().Transcript().Running() {
				if time.Now().After(deadline) {
					t.Fatalf("timed out: entities=%d blocks=%+v", ws.Session().Drawing().Len(), ws.AgentPanel().Transcript().Blocks())
				}
				if err := p.Advance(16 * time.Millisecond); err != nil {
					t.Fatal(err)
				}
				time.Sleep(time.Millisecond)
			}
			if n := ws.Session().Drawing().Len(); n != 1 {
				t.Fatalf("entities = %d, want 1", n)
			}
			var tool *transcript.ToolBlock
			for _, b := range ws.AgentPanel().Transcript().Blocks() {
				if tb, ok := b.(transcript.ToolBlock); ok {
					tb := tb
					tool = &tb
				}
			}
			if !strings.Contains(tool.Output, `"class":"LINE"`) {
				t.Fatalf("tool output = %q", tool.Output)
			}
		})
	}
}

// TestRenderPNGThroughMCPServer calls render_png over HTTP on the real MCP
// server with the real Snapshot renderer and decodes the image it returns.
func TestRenderPNGThroughMCPServer(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("no sockets on js")
	}
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Script("LINE 0,0 100,0  LINE 0,0 0,50 ")
	srv := mcp.NewServer(mcp.Options{Name: "go-draw", Version: "1"})
	cadmcp.Register(srv, cadmcp.Env{
		Dispatch: &cadmcp.DirectDispatcher{},
		Session:  func() *cmdsys.Session { return s },
		Snapshot: ui.Snapshot,
	})
	ep, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	post := func(body string) []byte {
		req, _ := http.NewRequestWithContext(context.Background(), "POST", ep.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+ep.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			t.Fatalf("http %d: %s", resp.StatusCode, b)
		}
		return b
	}
	post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	out := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"render_png","arguments":{"width":320,"height":240}}}`)
	var rsp struct {
		Result struct {
			Content []struct {
				Type, Data, MIMEType, Text string
			}
			IsError bool
		}
	}
	if err := json.Unmarshal(out, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Result.IsError || len(rsp.Result.Content) != 2 {
		t.Fatalf("result = %s", out)
	}
	img := rsp.Result.Content[0]
	if img.Type != "image" || img.MIMEType != "image/png" {
		t.Fatalf("content[0] = %+v", img)
	}
	raw, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("returned image does not decode: %v", err)
	}
	if b := im.Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("image is %v, want 320x240", b)
	}
	if !json.Valid([]byte(rsp.Result.Content[1].Text)) {
		t.Fatalf("info is not JSON: %q", rsp.Result.Content[1].Text)
	}
}
