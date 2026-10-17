package ui

import (
	"fmt"
	"testing"

	"agentmanager"
	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

func TestAgentModelMenuLongListKeepsComposerVisible(t *testing.T) {
	for _, height := range []float32{700, 500} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, height, 1)
			p := h.ws.AgentPanel()
			p.Configure(agentmanager.OpenCode, "/test/opencode")
			for i := 0; i < 80; i++ {
				p.cur.mdl.offered = append(p.cur.mdl.offered, agentmanager.ModelInfo{ID: fmt.Sprintf("ollama/model-%02d", i)})
			}
			p.Toggle(h.hw.Window())
			h.hw.Pixels()
			before := h.find("agent-input")
			p.modelMenu = true
			h.hw.Window().Refresh()
			h.hw.Pixels()
			h.png(fmt.Sprintf("model-menu-after-%d.png", int(height)))
			if p.modelPanel.Max().Y > h.hw.Window().Bounds().Size.H {
				t.Fatal("panel overflows window")
			}
			bottom := p.modelPanel.Max().Y - 6
			for x := p.modelPanel.Origin.X + 20; x < p.modelPanel.Max().X-20; x += 5 {
				id, _ := h.hw.Window().HitTest(gpui.Point{X: x, Y: bottom})
				for _, part := range id.Path {
					if part.Key == "agent-model-menu" {
						t.Fatal("model menu crosses panel bottom margin")
					}
				}
			}
			if !h.has("agent-model-default") {
				t.Fatal("model list is not reachable")
			}
			point := h.find("agent-model-default")
			h.hw.Dispatch(gpui.ScrollWheelEvent{Position: point, DeltaY: 10000})
			h.hw.Pixels()
			if !h.has("agent-model-search") || !h.has("agent-model-field") || !h.has("agent-model-ollama/model-79") {
				t.Fatal("last models and custom field are unreachable")
			}
			p.modelMenu = false
			h.hw.Window().Refresh()
			h.hw.Pixels()
			after := h.find("agent-input")
			if before != after {
				t.Fatal("opening menu moves composer")
			}
		})
	}
}
