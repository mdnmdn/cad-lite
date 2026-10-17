package ui_test

import (
	"cad-lite/settings"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartClearsSavedAgentHistory(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	r.chat(0, "old prompt", "old-session", "old reply")
	prefs := filepath.Join(dir, ".cad-lite", "preferences.toml")
	config := settings.Default()
	config.AgentFavorites = map[string][]string{"opencode": {"ollama/keep-favorite"}}
	if err := settings.Save(prefs, config); err != nil {
		t.Fatal(err)
	}
	r.ws.CloseAgent()
	legacy := filepath.Join(dir, ".go-draw", "agent")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "old.jsonl"), []byte("old transcript"), 0o600); err != nil {
		t.Fatal(err)
	}
	r2 := newRig(t, dir, nil, true)
	p := r2.ws.AgentPanel()
	if len(p.Sessions()) != 0 || len(p.Tabs()) != 1 || r2.hasText(0, "old reply") {
		t.Fatal("restart restored saved agent history")
	}
	for _, path := range []string{filepath.Join(dir, ".cad-lite", "agent"), legacy} {
		entries, err := os.ReadDir(path)
		if !os.IsNotExist(err) && err != nil {
			t.Fatal(err)
		}
		if len(entries) > 0 {
			t.Fatalf("previous history remains in %s", path)
		}
	}
	saved, err := settings.Load(prefs)
	if err != nil || len(saved.AgentFavorites["opencode"]) != 1 {
		t.Fatal("restart cleared model favorites")
	}
	r2.snap("restart-fresh-chat.png")
	r2.say("fresh prompt", 0)
	if cfg := r2.nextCfg(); cfg.ResumeID != "" {
		t.Fatal("fresh chat resumed old provider session")
	}
}

func TestEveryNewUntitledDrawingStartsWithFreshChat(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	r := newRig(t, dir, nil, true)
	r.chat(0, "plate prompt", "plate-session", "plate reply")
	p := r.ws.AgentPanel()
	r.ws.Session().Run("NEW")
	r.hw.Pixels()
	if len(p.Tabs()) != 1 || p.Tabs()[0].SessionID != "" || len(p.Transcript().Blocks()) != 0 {
		t.Fatal("new drawing inherited a chat")
	}
	r.say("untitled prompt", 1)
	r.nextCfg()
	r.turn(r.sess(1), "untitled-session", "untitled reply")
	r.until("untitled reply", func() bool { return r.hasText(0, "untitled reply") && !p.Transcript().Running() })
	r.ws.Session().Run("NEW")
	r.hw.Pixels()
	if len(p.Tabs()) != 1 || p.Tabs()[0].SessionID != "" || len(p.Transcript().Blocks()) != 0 {
		t.Fatal("second untitled drawing inherited first untitled chat")
	}
	if len(p.Sessions()) != 2 {
		t.Fatal("new drawing deleted current-run project history")
	}
	r.snap("new-drawing-fresh-chat.png")
	r.ws.SwitchDocument(1)
	r.hw.Pixels()
	if !r.hasText(0, "untitled reply") {
		t.Fatal("switching existing drawings lost current chat")
	}
}
