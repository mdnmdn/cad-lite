package ui

import (
	"path/filepath"
	"strings"

	"agentmanager"
	"cad-lite/harnessspike"
	"cad-lite/internal/gpui"
)

type harnessSpikeState struct {
	config     harnessspike.Config
	path       string
	key, model *gpui.TextInput
	message    string
	expanded   bool
}

// EnableHarnessSpike selects the experimental in-process Harness backend and
// opens its cloud provider controls. It never launches an agent executable.
func (p *AgentPanel) EnableHarnessSpike() {
	s := &harnessSpikeState{config: harnessspike.Default(), expanded: true}
	if path := p.ws.SettingsPath(); path != "" {
		s.path = filepath.Join(filepath.Dir(path), "harness-spike.json")
		var err error
		s.config, err = harnessspike.Load(s.path)
		if err != nil {
			s.message = "Could not load Harness settings"
		}
	}
	s.key = themeBareField(gpui.NewTextInput("API key (leave empty to keep saved key)"), fontSmall, false)
	s.model = themeBareField(gpui.NewTextInput("Model ID"), fontSmall, false)
	profile, _, _ := s.config.Selected()
	s.model.SetText(profile.Model)
	p.spike = s
	p.selected = agentmanager.Harness(harnessspike.Harness)
	p.choices = []agentmanager.Harness{p.selected}
	p.explicit, p.probed = true, true
	p.Show()
	if p.cur.rec != nil && p.harnessOf(p.cur) != p.selected {
		t := newAgentTab()
		p.tabs = append(p.tabs, t)
		p.activate(t)
	}
}

func (p *AgentPanel) saveHarnessSpike(w *gpui.Window) {
	s := p.spike
	profile, _, _ := s.config.Selected()
	profile.Model = strings.TrimSpace(s.model.Text())
	if key := strings.TrimSpace(s.key.Text()); key != "" {
		profile.APIKey = key
	}
	if s.config.Provider == "ollama" {
		s.config.Ollama = profile
	} else {
		s.config.OpenCode = profile
	}
	if s.path != "" {
		if err := harnessspike.Save(s.path, s.config); err != nil {
			s.message = "Could not save Harness settings"
			w.Refresh()
			return
		}
		s.message = "Saved. Stop and start the conversation to apply."
	} else {
		s.message = "Kept in memory. Stop and start to apply."
	}
	s.key.SetText("")
	w.Refresh()
}

func (p *AgentPanel) harnessSpikeControls(w *gpui.Window, cx *gpui.App) *gpui.Div {
	s := p.spike
	box := gpui.NewDiv().ID("harness-spike-controls").FlexCol().Gap(6)
	box.Child(p.button("harness-spike-toggle", "Harness cloud settings", "Configure the experimental cloud agent", false, func(w *gpui.Window) { s.expanded = !s.expanded; w.Refresh() }))
	if !s.expanded {
		return box
	}
	profile, url, _ := s.config.Selected()
	box.Child(segmented("harness-provider-", [][2]string{{"ollama", "Ollama Cloud"}, {"opencode-go", "OpenCode Go"}}, "Cloud provider for new sessions", s.config.Provider,
		func(value string, w *gpui.Window) {
			if value == s.config.Provider {
				return
			}
			s.config.Provider = value
			profile, _, _ := s.config.Selected()
			s.key.SetText("")
			s.model.SetText(profile.Model)
			s.message = "Press Save to remember this provider."
			w.Refresh()
		}))
	box.Child(gpui.NewText(url).TextSize(10).TextColor(uiMuted))
	box.Child(gpui.NewDiv().ID("harness-spike-model").SizeH(26).Px_(6).Bg(uiField).Rounded(4).Child(s.model.Render(w, cx)))
	box.Child(gpui.NewDiv().ID("harness-spike-key").SizeH(26).Px_(6).Bg(uiField).Rounded(4).Child(s.key.Render(w, cx)))
	status := "No saved API key"
	if profile.APIKey != "" {
		status = "API key saved"
	}
	box.Child(gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Child(
		p.button("harness-spike-save", "Save", "Save both provider profiles locally (owner-only file)", true, p.saveHarnessSpike),
		gpui.NewText(status).TextSize(10).TextColor(uiMuted)))
	if s.message != "" {
		box.Child(gpui.NewText(s.message).TextSize(10).TextColor(uiMuted))
	}
	return box
}
