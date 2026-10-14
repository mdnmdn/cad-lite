package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// agentRichMarkdown is an agent reply exercising every block the renderer draws.
const agentRichMarkdown = "## Plate with holes\n\nI drew the **outline** on layer `outline`, then the *corner holes*.\n\n" +
	"1. Outline `100 x 60`\n2. Holes, radius 5:\n   - four corners\n   - inset by **10**\n3. Zoom to extents\n\n" +
	"> Holes go on their own layer.\n\n" +
	"```text\nRECTANG 0,0 100,60\nCIRCLE 10,10 5\n```\n\n" +
	"| Layer | Colour | Count |\n|:--|:--|--:|\n| outline | cyan | 1 |\n| holes | red | 4 |\n\n---\n\n" +
	"Details in [the manual](https://example.com/manual)."

// TestAgentPanelRenders writes the panel with rich Markdown, tool cards, queued prompts and the
// working animation to _tmp/agent-ui in the dark and light themes, for a visual check.
func TestAgentPanelRenders(t *testing.T) {
	prev := CurrentTheme()
	t.Cleanup(func() { ApplyTheme(prev) })
	for _, th := range []Theme{themeDark, themeLight} {
		ApplyTheme(th)
		ws := NewWorkspace(doc.New(), nil)
		ws.SetProjectPath(filepath.Join(t.TempDir(), "plate.dxf"))
		p := ws.AgentPanel()
		p.hist.loaded, p.hist.off = true, true
		p.Configure(agentmanager.Claude, "")
		p.Show()
		p.Seed(func(tr *transcript.T) {
			tr.AddUser("Draw a 100 by 60 plate with a 10 mm hole in each corner")
			tr.Apply(agentmanager.TextDelta{MessageID: "m1", Text: agentRichMarkdown})
			tr.Apply(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "1", Title: "cad add_rect", Kind: agentmanager.KindEdit,
				Status: agentmanager.ToolCompleted, RawInput: json.RawMessage(`{"x":0,"y":0,"w":100,"h":60,"layer":"outline"}`)}})
			tr.Apply(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "2", Title: "cad add_circle", Kind: agentmanager.KindEdit,
				Status: agentmanager.ToolInProgress, RawInput: json.RawMessage(`{"cx":10,"cy":10,"r":5}`)}})
			tr.AddUser("Now add a centre line")
		})
		p.cur.expanded[2] = true // the first tool card open, the second collapsed
		p.cur.state = agentReady
		p.cur.harness = agentmanager.Claude
		p.cur.queue = []agentQueued{{text: "Then dimension the plate"}, {text: "And save it as plate-v2"}}
		plat := gpui.NewHeadlessPlatform()
		if err := plat.Run(func(cx *gpui.App) {
			if err := cx.OpenWindow(gpui.WindowOptions{Width: 1600, Height: 1100}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
				t.Fatal(err)
			}
		}); err != nil {
			t.Fatal(err)
		}
		hw := plat.Windows()[0]
		hw.Pixels()
		p.cur.scroll.ScrollToTop()
		hw.Window().Refresh()
		hw.Pixels()
		if err := os.MkdirAll("../_tmp/agent-ui", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := hw.SavePNG(filepath.Join("../_tmp/agent-ui", th.Name+".png")); err != nil {
			t.Fatal(err)
		}
		p.cur.state = agentIdle
		ws.CloseAgent()
	}
}

// TestAgentWorkingPosesRender writes a strip of the working animation's poses, at 4x, to
// _tmp/agent-ui/working.png.
func TestAgentWorkingPosesRender(t *testing.T) {
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		row := gpui.NewDiv().FlexRow().Gap(6).P(4).Bg(uiPanelAlt)
		for ph := 0; ph <= agentWorkingSteps/2; ph += 2 {
			ph := ph
			row.Child(gpui.NewDiv().SizeWH(agentWorkingW, agentWorkingH).Child(gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
				paintWorking(w, b, ph)
			})))
		}
		return row
	})
	plat := gpui.NewHeadlessPlatform()
	plat.SetScaleFactor(4)
	if err := plat.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 200, Height: 20}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	hw := plat.Windows()[0]
	hw.Pixels()
	if err := os.MkdirAll("../_tmp/agent-ui", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := hw.SavePNG("../_tmp/agent-ui/working.png"); err != nil {
		t.Fatal(err)
	}
}
