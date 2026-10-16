package ui_test

import (
	"path/filepath"
	"testing"

	"cad-lite/harnessspike"
)

func TestHarnessSpikeControlsSaveBothProviders(t *testing.T) {
	r := openAgent(t)
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := r.ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	r.ws.AgentPanel().EnableHarnessSpike()
	r.hw.Pixels()
	r.click(r.pos("harness-spike-key"))
	r.typ("ollama-test-secret")
	r.click(r.pos("harness-spike-save"))
	configPath := filepath.Join(filepath.Dir(path), "harness-spike.json")
	c, err := harnessspike.Load(configPath)
	if err != nil || c.Ollama.APIKey != "ollama-test-secret" {
		t.Fatalf("save Ollama: %v", err)
	}
	r.hw.Pixels()
	r.click(r.pos("harness-provider-opencode-go"))
	r.hw.Pixels()
	r.click(r.pos("harness-spike-key"))
	r.typ("go-test-secret")
	r.click(r.pos("harness-spike-save"))
	c, err = harnessspike.Load(configPath)
	if err != nil || c.Provider != "opencode-go" || c.OpenCode.APIKey != "go-test-secret" || c.Ollama.APIKey != "ollama-test-secret" {
		t.Fatalf("save Go profile or retain Ollama: %v", err)
	}
	r.click(r.pos("harness-spike-toggle"))
	r.hw.Pixels()
	if _, ok := (&tbRig{t: t, hw: r.hw}).boundsAnywhere("harness-spike-key"); ok {
		t.Fatal("collapsed credentials still visible")
	}
}
