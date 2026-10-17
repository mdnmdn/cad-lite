package ui

import (
	"path/filepath"
	"testing"

	"agentmanager"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

func favoriteMenuRig(t *testing.T) *sHarness {
	t.Helper()
	h := openSettingsApp(t)
	if err := h.ws.LoadSettingsFrom(filepath.Join(t.TempDir(), "settings.toml")); err != nil {
		t.Fatal(err)
	}
	p := h.ws.AgentPanel()
	p.Configure(agentmanager.OpenCode, "/test/opencode")
	p.cur.mdl.offered = []agentmanager.ModelInfo{{ID: "opencode/alpha", Name: "Alpha"}, {ID: "ollama/beta", Name: "Beta Cloud"}, {ID: "ollama/gamma", Name: "Gamma"}}
	p.Toggle(h.hw.Window())
	h.hw.Pixels()
	h.clickID("agent-model")
	return h
}

func TestModelFavoritesTogglePersistsWithoutSelecting(t *testing.T) {
	h := favoriteMenuRig(t)
	p := h.ws.AgentPanel()
	before := p.cur.mdl.chosen
	h.clickID("agent-model-ollama/beta-star")
	if !p.modelMenu || p.cur.mdl.chosen != before {
		t.Fatal("star selected a model or closed the menu")
	}
	if !p.isFavorite(agentmanager.OpenCode, "ollama/beta") || p.isFavorite(agentmanager.Claude, "ollama/beta") {
		t.Fatal("favorite harness scope is incorrect")
	}
	if !h.has("agent-model-favorite-ollama/beta-star") {
		t.Fatal("favorite section missing")
	}
	first, other := h.find("agent-model-favorite-ollama/beta"), h.find("agent-model-opencode/alpha")
	if first.Y >= other.Y {
		t.Fatal("favorites are not above the catalog")
	}
	saved, err := settings.Load(h.ws.prefs().path)
	if err != nil || len(saved.AgentFavorites["opencode"]) != 1 {
		t.Fatalf("favorite not persisted: %v", err)
	}
	h.png("model-favorites.png")
	h.clickID("agent-model-favorite-ollama/beta-star")
	saved, err = settings.Load(h.ws.prefs().path)
	if err != nil || len(saved.AgentFavorites["opencode"]) != 0 || !p.modelMenu {
		t.Fatal("unfavorite did not persist or closed menu")
	}
}

func TestModelSearchFiltersNamesAndIDs(t *testing.T) {
	h := favoriteMenuRig(t)
	p := h.ws.AgentPanel()
	h.clickID("agent-model-ollama/beta-star")
	h.clickID("agent-model-search")
	for _, char := range "CLOUD" {
		key := string(char)
		h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: key, KeyChar: &key}, PreferCharacterInput: true})
	}
	h.hw.Pixels()
	if !h.has("agent-model-favorite-ollama/beta") || !h.has("agent-model-ollama/beta") || h.has("agent-model-opencode/alpha") {
		t.Fatal("search does not filter favorites and catalog by name")
	}
	p.modelSearch.SetText("  OLLAMA/GAMMA  ")
	h.hw.Pixels()
	if !h.has("agent-model-ollama/gamma") || h.has("agent-model-ollama/beta") {
		t.Fatal("search does not match model ID")
	}
	p.modelSearch.SetText("no-such-model")
	h.hw.Pixels()
	if h.has("agent-model-ollama/gamma") || !h.has("agent-model-field") {
		t.Fatal("empty results hide custom model input")
	}
	h.png("model-search-no-results.png")
	p.modelSearch.SetText("Gamma")
	h.hw.Pixels()
	h.clickID("agent-model-ollama/gamma")
	if p.cur.mdl.chosen != "ollama/gamma" || p.modelMenu {
		t.Fatal("filtered result could not be selected")
	}
	h.clickID("agent-model")
	if p.modelSearch.Text() != "" || !h.has("agent-model-opencode/alpha") {
		t.Fatal("reopened menu retained stale search")
	}
}

func TestModelFavoritesAvailableBeforeAgentCatalog(t *testing.T) {
	h := favoriteMenuRig(t)
	p := h.ws.AgentPanel()
	p.toggleFavorite(agentmanager.OpenCode, "ollama/remembered", h.hw.Window())
	p.cur.mdl.offered = nil
	h.hw.Window().Refresh()
	h.hw.Pixels()
	if !h.has("agent-model-favorite-ollama/remembered-star") {
		t.Fatal("saved favorite requires a live catalog")
	}
	h.clickID("agent-model-favorite-ollama/remembered")
	if p.cur.mdl.chosen != "ollama/remembered" {
		t.Fatal("saved favorite cannot be selected")
	}
}
