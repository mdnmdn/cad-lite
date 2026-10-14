package claude_test

import (
	"context"
	"strings"
	"testing"

	"agentmanager"
)

func TestPromptImagesSendsImageBlocks(t *testing.T) {
	h := open(t, builtin("echo-turn"))
	ip, ok := h.s.(agentmanager.ImagePrompter)
	if !ok || !ip.AcceptsImages() {
		t.Fatalf("claude session takes no images (ok=%v)", ok)
	}
	if err := ip.PromptImages(context.Background(), "look", []agentmanager.Image{{MIME: "image/webp", Data: []byte("abc")}}); err != nil {
		t.Fatal(err)
	}
	expect[agentmanager.SessionStarted](h)
	var text strings.Builder
	for {
		ev := h.next()
		if d, ok := ev.(agentmanager.TextDelta); ok {
			text.WriteString(d.Text)
		}
		if _, ok := ev.(agentmanager.TurnEnd); ok {
			break
		}
	}
	if want := "echo: look [image image/webp 3 bytes]"; text.String() != want {
		t.Fatalf("echo %q, want %q", text.String(), want)
	}
	found := false
	for _, l := range h.logLines() {
		if strings.Contains(l, `"type":"image"`) && strings.Contains(l, `"source":{"data":"YWJj","media_type":"image/webp","type":"base64"}`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no base64 image block in %v", h.logLines())
	}
}
