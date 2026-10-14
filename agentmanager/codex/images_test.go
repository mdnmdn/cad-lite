package codex_test

import (
	"context"
	"strings"
	"testing"

	"agentmanager"
)

func TestPromptImagesSendsImageItems(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	s := e.open(t)
	next(t, s)
	ip, ok := s.(agentmanager.ImagePrompter)
	if !ok || !ip.AcceptsImages() {
		t.Fatalf("codex session takes no images (ok=%v)", ok)
	}
	if err := ip.PromptImages(context.Background(), "look", []agentmanager.Image{{MIME: "image/gif", Data: []byte("GIF8")}}); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, ev := range until(t, s, isTurnEnd) {
		if d, ok := ev.(agentmanager.TextDelta); ok {
			text.WriteString(d.Text)
		}
	}
	if want := "Hello world [image image/gif 4 bytes]"; text.String() != want {
		t.Fatalf("text %q, want %q", text.String(), want)
	}
	if !strings.Contains(logLines(t, e), `{"type":"image","url":"data:image/gif;base64,R0lGOA=="}`) {
		t.Fatalf("no data URL image item in %s", logLines(t, e))
	}
}
