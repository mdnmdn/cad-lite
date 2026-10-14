package ui_test

import (
	"testing"

	"agentmanager"
	"cad-lite/internal/gpui"
)

// composerHeight is the agent composer's on-screen height.
func (r *agentRig) composerHeight() float64 {
	r.t.Helper()
	b, ok := (&tbRig{t: r.t, hw: r.hw}).boundsAnywhere("agent-composer")
	if !ok {
		r.t.Fatal("agent composer not on screen")
	}
	return float64(b.y1 - b.y0)
}

func TestAgentPromptShiftEnterAddsLinesAndEnterSends(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.click(r.inputPos())
	one := r.composerHeight()
	r.typ("draw a circle")
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter", Modifiers: gpui.Shift}})
	r.typ("then a square")
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter", Modifiers: gpui.Shift}})
	r.typ("and hatch both")
	three := r.composerHeight()
	r.fs.mu.Lock()
	sent := len(r.fs.prompts)
	r.fs.mu.Unlock()
	if sent != 0 {
		t.Fatal("shift-enter sent the prompt")
	}
	if three <= one+10 {
		t.Fatalf("composer did not grow with a 3-line prompt: %v -> %v", one, three)
	}
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}})
	r.fs.mu.Lock()
	got := append([]string(nil), r.fs.prompts...)
	r.fs.mu.Unlock()
	if want := "draw a circle\nthen a square\nand hatch both"; len(got) != 1 || got[0] != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	if r.ws.Session().Active() != "" {
		t.Fatal("text leaked into the command line")
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("turn end", func() bool { return !r.ws.AgentPanel().Transcript().Running() })
	r.hw.Pixels()
	if h := r.composerHeight(); h != one {
		t.Fatalf("composer height after send = %v, want %v", h, one)
	}
}

func TestAgentPromptPasteKeepsNewlines(t *testing.T) {
	r := openAgent(t)
	r.start()
	r.click(r.inputPos())
	text := "first\r\nsecond"
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: text, KeyChar: &text}, PreferCharacterInput: true})
	r.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}})
	r.fs.mu.Lock()
	got := append([]string(nil), r.fs.prompts...)
	r.fs.mu.Unlock()
	if len(got) != 1 || got[0] != "first\nsecond" {
		t.Fatalf("prompts = %q", got)
	}
	if r.ws.Session().Active() != "" {
		t.Fatal("pasted text reached the command line")
	}
}
