package ui

import (
	"os"
	"testing"
)

func TestAISettingsRender(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	name := os.Getenv("CADLITE_AI_RENDER")
	if name == "" {
		name = "ai-settings.png"
	}
	h.png(name)
}

func TestAISettingsMissingOpenCodeRender(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h := openSettingsApp(t)
	h.openPanel()
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	h.png("ai-basic-missing.png")
	if !h.has("settings-basic-install-ask") {
		t.Fatal("Basic mode offers no OpenCode installation")
	}
}

func TestAIAdvancedSettingsRender(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	h.clickID("ai-mode-advanced")
	h.png("ai-advanced-after.png")
}
