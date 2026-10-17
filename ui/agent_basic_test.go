package ui

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"agentmanager"
	"cad-lite/settings"
)

func TestBasicOpenCodeProviderConfigurations(t *testing.T) {
	for _, name := range []string{"free", "opencode", "ollama", "both"} {
		t.Run(name, func(t *testing.T) {
			c := settings.Default()
			if name == "opencode" || name == "both" {
				c.OpenCodeKey = "test-opencode-secret"
			}
			if name == "ollama" || name == "both" {
				c.OllamaKey = "test-ollama-secret"
			}
			env := basicOpenCodeEnv(c)
			var content string
			for _, v := range env {
				if s, ok := strings.CutPrefix(v, "OPENCODE_CONFIG_CONTENT="); ok {
					content = s
				}
			}
			if strings.Contains(content, "test-opencode-secret") || strings.Contains(content, "test-ollama-secret") {
				t.Fatal("credential embedded into configuration")
			}
			var conf struct {
				Model    string `json:"model"`
				Provider map[string]struct {
					NPM     string            `json:"npm"`
					Options map[string]string `json:"options"`
					Models  map[string]any    `json:"models"`
				} `json:"provider"`
			}
			if err := json.Unmarshal([]byte(content), &conf); err != nil {
				t.Fatal(err)
			}
			if _, ok := conf.Provider["opencode"]; ok != (c.OpenCodeKey != "") {
				t.Fatal("OpenCode key configuration mismatch")
			}
			if _, ok := conf.Provider["ollama"]; ok != (c.OllamaKey != "") {
				t.Fatal("Ollama key configuration mismatch")
			}
			if c.OllamaKey != "" {
				p := conf.Provider["ollama"]
				if p.NPM != "@ai-sdk/openai-compatible" || p.Options["baseURL"] != "https://ollama.com/v1" || p.Options["apiKey"] != "{env:OLLAMA_KEY}" || p.Models["gpt-oss:120b"] == nil {
					t.Fatal("Ollama Cloud provider is incomplete")
				}
			}
			if name == "ollama" {
				if conf.Model != basicOllamaModel {
					t.Fatal("Ollama-only default is wrong")
				}
			} else if conf.Model != opencodeSuggested {
				t.Fatal("free OpenCode default is wrong")
			}
		})
	}
}

func TestBasicSettingsSaveKeysAndClearDrafts(t *testing.T) {
	h := openSettingsApp(t)
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := h.ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	h.openPanel()
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	fields := h.ws.prefs().aiKeys
	fields.opencode.SetText("opencode-test-key")
	fields.ollama.SetText("ollama-test-key")
	h.clickID("ai-keys-save")
	got, err := settings.Load(path)
	if err != nil || got.OpenCodeKey != "opencode-test-key" || got.OllamaKey != "ollama-test-key" {
		t.Fatal("provider keys were not saved")
	}
	if fields.opencode.Text() != "" || fields.ollama.Text() != "" {
		t.Fatal("saved keys remain in textboxes")
	}
	fields.opencode.SetText("   ")
	h.clickID("ai-keys-save")
	if h.ws.Settings().OpenCodeKey != "opencode-test-key" {
		t.Fatal("blank field erased saved key")
	}
	h.clickID("ai-mode-advanced")
	if h.ws.Settings().AgentMode != settings.AgentAdvanced {
		t.Fatal("Advanced mode not selected")
	}
	h.clickID("ai-mode-basic")
	if h.ws.Settings().AgentMode != settings.AgentBasic || h.ws.Settings().DefaultAgent != "opencode" {
		t.Fatal("Basic mode did not select OpenCode")
	}
	h.clickID("ai-key-opencode-clear")
	if h.ws.Settings().OpenCodeKey != "" || h.ws.Settings().OllamaKey != "ollama-test-key" {
		t.Fatal("removal affected wrong provider")
	}
}

func TestBasicOllamaSuggestion(t *testing.T) {
	h := openSettingsApp(t)
	c := h.ws.Settings()
	c.OllamaKey = "test-key"
	h.ws.SetSettings(c, nil)
	p := h.ws.AgentPanel()
	if m := p.suggestion(p.cur, agentmanager.OpenCode); m != basicOllamaModel {
		t.Fatalf("suggestion %q", m)
	}
	p.cur.mdl.chosen = "opencode/custom"
	if m := p.modelFor(p.cur, agentmanager.OpenCode); m != "opencode/custom" {
		t.Fatal("explicit model was replaced")
	}
}

func TestAISettingsModeVisibility(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	if !h.has("ai-key-opencode") || !h.has("ai-key-ollama") {
		t.Fatal("Basic provider fields missing")
	}
	if h.has("settings-ai-advanced") || h.has("agent-claude") {
		t.Fatal("Basic exposes advanced controls")
	}
	h.clickID("ai-mode-advanced")
	if h.has("ai-key-opencode") || h.has("ai-keys-save") {
		t.Fatal("Advanced exposes Basic keys")
	}
	if !h.has("settings-ai-advanced") {
		t.Fatal("Advanced controls missing")
	}
}

func TestBasicChatHidesHarnessSelector(t *testing.T) {
	h := openSettingsApp(t)
	p := h.ws.AgentPanel()
	p.Configure(agentmanager.OpenCode, "/test/opencode")
	p.Toggle(h.hw.Window())
	h.hw.Pixels()
	if h.has("agent-pick-opencode") {
		t.Fatal("Basic chat exposes harness selector")
	}
	if !h.has("agent-model") {
		t.Fatal("Basic chat hides model selector")
	}
	h.png("ai-basic-chat-after.png")
	c := h.ws.Settings()
	c.AgentMode = settings.AgentAdvanced
	h.ws.SetSettings(c, h.hw.Window())
	p.Configure(agentmanager.OpenCode, "/test/opencode")
	h.hw.Pixels()
	if !h.has("agent-pick-opencode") {
		t.Fatal("Advanced chat hides harness selector")
	}
}
