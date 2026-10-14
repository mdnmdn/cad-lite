package acp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agentmanager"
)

func agentText(evs []agentmanager.Event) string {
	var b strings.Builder
	for _, ev := range evs {
		if d, ok := ev.(agentmanager.TextDelta); ok {
			b.WriteString(d.Text)
		}
	}
	return b.String()
}

func TestPromptImagesSendsImageBlocks(t *testing.T) {
	e := builtin(t, "echo-turn")
	s := e.open()
	ip, ok := s.(agentmanager.ImagePrompter)
	if !ok || !ip.AcceptsImages() {
		t.Fatalf("acp session takes no images (ok=%v)", ok)
	}
	imgs := []agentmanager.Image{{MIME: "image/png", Data: []byte("12345")}, {MIME: "image/jpeg", Data: []byte("ab")}}
	if err := ip.PromptImages(context.Background(), "look", imgs); err != nil {
		t.Fatal(err)
	}
	got := agentText(untilTurnEnd(t, s))
	if want := "echo: look [image image/png 5 bytes] [image image/jpeg 2 bytes]"; got != want {
		t.Fatalf("echo %q, want %q", got, want)
	}
	p := e.find("session/prompt")["params"].(map[string]any)["prompt"].([]any)
	if len(p) != 3 || p[1].(map[string]any)["type"] != "image" || p[1].(map[string]any)["data"] != "MTIzNDU=" {
		t.Fatalf("prompt blocks %v", p)
	}
}

func TestPromptImagesRefusedWithoutCapability(t *testing.T) {
	e := builtin(t, "echo-turn,noimage")
	s := e.open()
	ip := s.(agentmanager.ImagePrompter)
	if ip.AcceptsImages() {
		t.Fatal("AcceptsImages with promptCapabilities.image false")
	}
	err := ip.PromptImages(context.Background(), "look", []agentmanager.Image{{MIME: "image/png", Data: []byte("x")}})
	if !errors.Is(err, agentmanager.ErrNoImages) {
		t.Fatalf("err %v", err)
	}
	if e.find("session/prompt") != nil {
		t.Fatal("a refused prompt reached the agent")
	}
	// Text only still works, and the refusal left no turn running.
	if err := ip.PromptImages(context.Background(), "plain", nil); err != nil {
		t.Fatal(err)
	}
	if got := agentText(untilTurnEnd(t, s)); got != "echo: plain" {
		t.Fatalf("echo %q", got)
	}
}
