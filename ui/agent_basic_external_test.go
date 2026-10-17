package ui_test

import (
	"strings"
	"testing"

	"agentmanager"
	"cad-lite/settings"
)

func TestBasicAgentStartInjectsSavedProviderKeys(t *testing.T) {
	r := openAgent(t)
	c := r.ws.Settings()
	c.OllamaKey = "dummy-ollama-key"
	r.ws.SetSettings(c, r.hw.Window())
	r.ws.AgentPanel().Configure(agentmanager.OpenCode, "/nonexistent/opencode")
	r.start()
	cfg := <-r.cfg
	if cfg.Model != "ollama/gpt-oss:120b" {
		t.Fatal("Ollama-only startup did not choose its cloud model")
	}
	var key, config bool
	for _, v := range cfg.Env {
		key = key || v == "OLLAMA_KEY=dummy-ollama-key"
		config = config || strings.HasPrefix(v, "OPENCODE_CONFIG_CONTENT=") && strings.Contains(v, "{env:OLLAMA_KEY}")
	}
	if !key || !config {
		t.Fatal("saved keys were not injected into the session configuration")
	}
}

func TestAdvancedAgentStartKeepsProviderConfiguration(t *testing.T) {
	r := openAgent(t)
	c := r.ws.Settings()
	c.AgentMode = settings.AgentAdvanced
	c.OllamaKey = "dummy-unused-key"
	r.ws.SetSettings(c, r.hw.Window())
	r.ws.AgentPanel().Configure(agentmanager.OpenCode, "/nonexistent/opencode")
	r.start()
	cfg := <-r.cfg
	for _, v := range cfg.Env {
		if strings.HasPrefix(v, "OPENCODE_CONFIG_CONTENT=") || strings.HasPrefix(v, "OLLAMA_KEY=") {
			t.Fatal("Advanced mode injected managed provider settings")
		}
	}
}
