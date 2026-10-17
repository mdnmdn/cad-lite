package ui

import (
	"encoding/json"
	"strings"

	"agentmanager"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

const basicOllamaModel = "ollama/gpt-oss:120b"

type basicKeyFields struct{ opencode, ollama *gpui.TextInput }

func basicOpenCodeEnv(c settings.Settings, models ...agentmanager.ModelInfo) []string {
	providers := map[string]any{}
	model := opencodeSuggested
	if c.OpenCodeKey != "" {
		providers["opencode"] = map[string]any{"options": map[string]string{"apiKey": "{env:OPENCODE_KEY}"}}
	}
	if c.OllamaKey != "" {
		catalog := map[string]any{}
		if len(models) == 0 {
			models = []agentmanager.ModelInfo{{ID: basicOllamaModel, Name: "gpt-oss 120b (Ollama Cloud)"}}
		}
		for _, m := range models {
			catalog[strings.TrimPrefix(m.ID, "ollama/")] = map[string]string{"name": m.Name}
		}
		providers["ollama"] = map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Ollama Cloud",
			"options": map[string]string{"baseURL": "https://ollama.com/v1", "apiKey": "{env:OLLAMA_KEY}"},
			"models":  catalog}
		if c.OpenCodeKey == "" {
			model = basicOllamaModel
			if _, ok := catalog[strings.TrimPrefix(model, "ollama/")]; !ok && len(models) > 0 {
				model = models[0].ID
			}
		}
	}
	b, _ := json.Marshal(map[string]any{"$schema": "https://opencode.ai/config.json", "provider": providers, "model": model})
	return []string{"OPENCODE_KEY=" + c.OpenCodeKey, "OLLAMA_KEY=" + c.OllamaKey, "OPENCODE_CONFIG_CONTENT=" + string(b)}
}

func (ws *Workspace) basicAISection(w *gpui.Window, cx *gpui.App) *gpui.Div {
	p := ws.prefs()
	if p.aiKeys == nil {
		p.aiKeys = &basicKeyFields{opencode: themeBareField(gpui.NewTextInput(""), fontSmall, false), ollama: themeBareField(gpui.NewTextInput(""), fontSmall, false)}
	}
	fields := p.aiKeys
	c := p.cur
	sec := gpui.NewDiv().ID("settings-ai-basic").FlexCol().Gap(settingsRowGap).P(12).Rounded(8).Border(1, uiBorder).Child(
		sectionTitle(i18n.T("ui.settings.tab_ai")),
		segmented("ai-mode-", [][2]string{{settings.AgentBasic, i18n.T("ui.settings.ai_basic")}, {settings.AgentAdvanced, i18n.T("ui.settings.ai_advanced")}}, i18n.T("ui.settings.ai_mode_tip"), c.AgentMode,
			func(mode string, w *gpui.Window) {
				ws.change(w, func(s *settings.Settings) {
					s.AgentMode = mode
					if mode == settings.AgentBasic {
						s.DefaultAgent = "opencode"
					} else {
						s.DefaultAgent = ""
					}
				})
			}),
	)
	if c.AgentMode == settings.AgentAdvanced {
		return sec
	}
	ws.refreshOllamaModels(w, false)
	sec.Child(gpui.NewText(i18n.T("ui.settings.ai_basic_hint")).TextSize(11).TextColor(uiMuted))
	if path, ok := ws.openCodeAvailable(); ok {
		sec.Child(gpui.NewText(i18n.T("ui.settings.ai_opencode_found", path)).TextSize(11).TextColor(uiMuted))
	} else {
		sec.Child(gpui.NewText(i18n.T("ui.settings.ai_opencode_missing")).TextSize(11).TextColor(uiWarn))
	}
	if c.AgentMode == settings.AgentBasic && ws.installWanted(true) {
		sec.Child(ws.agentInstallBlock("settings-basic"))
	}
	row := func(name, id string, field *gpui.TextInput, saved string, clear func(*settings.Settings)) *gpui.Div {
		placeholder := i18n.T("ui.settings.ai_key_placeholder")
		if saved != "" {
			placeholder = i18n.T("ui.settings.ai_key_saved")
		}
		field.SetPlaceholder(placeholder)
		input := gpui.NewDiv().ID(id).SizeH(28).Px_(6).Rounded(4).Bg(uiField).Border(1, uiBorder).Flex1().MinW(0).Tooltip(i18n.T("ui.settings.ai_key_tip")).Child(field.Render(w, cx))
		controls := gpui.NewDiv().FlexRow().Gap(6).Flex1().MinW(0).Child(input)
		if saved != "" {
			controls.Child(actionButton(id+"-clear", i18n.T("ui.settings.ai_key_remove"), i18n.T("ui.settings.ai_key_remove_tip"), false, true, func(w *gpui.Window, _ *gpui.App) { field.SetText(""); ws.change(w, clear) }))
		}
		return settingRow(name, i18n.T("ui.settings.ai_key_tip"), controls)
	}
	sec.Child(row("OpenCode", "ai-key-opencode", fields.opencode, c.OpenCodeKey, func(s *settings.Settings) { s.OpenCodeKey = "" }),
		row("Ollama Cloud", "ai-key-ollama", fields.ollama, c.OllamaKey, func(s *settings.Settings) { s.OllamaKey = "" }),
		actionButton("ai-keys-save", i18n.T("ui.settings.ai_keys_save"), i18n.T("ui.settings.ai_keys_save_tip"), true, true, func(w *gpui.Window, _ *gpui.App) {
			ws.saveBasicKeys(w)
			w.Refresh()
		}),
		gpui.NewText(i18n.T("ui.settings.ai_keys_hint")).TextSize(11).TextColor(uiMuted))
	sec.Child(ws.basicCheckControls(w))
	return sec
}

func (ws *Workspace) refreshAIMode() {
	if p := ws.agent; p != nil && p.spike == nil {
		p.choices, p.selected = nil, ""
		p.explicit, p.probed = false, false
		p.probe()
	}
}
