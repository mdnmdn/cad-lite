package ui

import (
	"context"
	"slices"
	"strings"
	"time"

	"agentmanager"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// agentModelTimeout bounds a live model switch.
const agentModelTimeout = 15 * time.Second

// tabModel is a tab's model: what the user chose and what the agent reported. UI goroutine only.
type tabModel struct {
	// chosen is the user's pick for this tab; "" means the default for the harness (settings),
	// else the harness's own.
	chosen string
	// offered is the last model list the agent reported; current the model it last said is in effect.
	offered []agentmanager.ModelInfo
	current string
	// setting is true while a live switch runs; pending applies chosen once the starting session
	// is ready; restart ends the session after the running turn, so the next prompt starts it again
	// on chosen.
	setting, pending, restart bool
	// prev is the choice a switch in flight replaces, restored when the agent refuses.
	prev string
	// picked is set once the user chose a model (Default included) or the tab came from the store;
	// until then an opencode tab is offered a free model (suggestedModel).
	picked bool
}

// opencodeSuggested is the free opencode model a fresh opencode tab starts on when the agent has
// not listed its models yet. opencode names models "<provider>/<model>"; big-pickle is served free
// by its own "opencode" provider (OpenCode Zen).
const opencodeSuggested = "opencode/big-pickle"

// freeModel reports whether id is one of opencode's free models: big-pickle or an id ending in
// "-free", with or without the provider prefix.
func freeModel(id string) bool {
	base := id
	if i := strings.LastIndexByte(id, '/'); i >= 0 {
		base = id[i+1:]
	}
	return base == "big-pickle" || strings.HasSuffix(base, "-free")
}

// orderModels lists models for the menu: on opencode the free ones first (big-pickle before the
// others, otherwise in the agent's order), then the rest; other harnesses keep the agent's order.
func orderModels(h agentmanager.Harness, ms []agentmanager.ModelInfo) []agentmanager.ModelInfo {
	if h != agentmanager.OpenCode {
		return ms
	}
	rank := func(m agentmanager.ModelInfo) int {
		switch {
		case !freeModel(m.ID):
			return 2
		case strings.HasSuffix(m.ID, "big-pickle"):
			return 0
		}
		return 1
	}
	out := slices.Clone(ms)
	slices.SortStableFunc(out, func(a, b agentmanager.ModelInfo) int { return rank(a) - rank(b) })
	return out
}

// suggestedModel is the free model offered to tab t on harness h before the user picked one: on
// opencode big-pickle, else the first free model the agent listed, else (no list yet)
// opencodeSuggested; "" on other harnesses or when the list has no free model.
func suggestedModel(h agentmanager.Harness, m *tabModel) string {
	if h != agentmanager.OpenCode {
		return ""
	}
	if len(m.offered) == 0 {
		return opencodeSuggested
	}
	if l := orderModels(h, m.offered); freeModel(l[0].ID) {
		return l[0].ID
	}
	return ""
}

// observe folds what an agent event says about models into m.
func (m *tabModel) observe(ev agentmanager.Event) {
	switch e := ev.(type) {
	case agentmanager.SessionStarted:
		if e.Model != "" {
			m.current = e.Model
		}
	case agentmanager.ModelsChanged:
		if e.Available != nil {
			m.offered = e.Available
		}
		if e.Current != "" {
			m.current = e.Current
		}
	}
}

// name is the label of model id: its name in the offered list, else the id.
func (m *tabModel) name(id string) string {
	for _, o := range m.offered {
		if o.ID == id && o.Name != "" {
			return o.Name
		}
	}
	return id
}

// modelResult is the outcome of ModelSetter.SetModel, handed from its goroutine to the UI.
type modelResult struct {
	gen   int
	model string
	// prev is the choice to go back to when the switch failed.
	prev string
	err  error
}

// defaultModel is the settings' model for new tabs on harness h, "" for the harness's own.
func (p *AgentPanel) defaultModel(h agentmanager.Harness) string {
	return p.ws.prefs().cur.AgentModels[string(h)]
}

// modelFor is the model tab t starts with on harness h: its own choice, else the default, else
// (an opencode tab the user has not picked for, with no stored conversation) a free model.
func (p *AgentPanel) modelFor(t *agentTab, h agentmanager.Harness) string {
	if t.mdl.chosen != "" {
		return t.mdl.chosen
	}
	if d := p.defaultModel(h); d != "" {
		return d
	}
	return p.suggestion(t, h)
}

// suggestion is the free model tab t is offered on harness h, "" once the user picked a model, the
// tab has a stored conversation or the settings name a default.
func (p *AgentPanel) suggestion(t *agentTab, h agentmanager.Harness) string {
	if t.mdl.picked || t.rec != nil || t.mdl.chosen != "" || p.defaultModel(h) != "" {
		return ""
	}
	c := p.ws.prefs().cur
	if h == agentmanager.OpenCode && c.AgentMode == settings.AgentBasic && c.OllamaKey != "" && c.OpenCodeKey == "" {
		return p.ws.basicOllamaDefault()
	}
	return suggestedModel(h, &t.mdl)
}

// commitSuggestion turns the free model tab t is offered into its choice, so the conversation is
// stored on it and keeps it when restored.
func (p *AgentPanel) commitSuggestion(t *agentTab) {
	if s := p.suggestion(t, p.harnessOf(t)); s != "" {
		t.mdl.chosen = s
	}
	t.mdl.picked = true
}

// modelLabel is what the panel shows as tab t's model: the model's name, else its id without the
// "<provider>/" prefix opencode ids carry.
func (p *AgentPanel) modelLabel(t *agentTab) string {
	id := p.modelFor(t, p.harnessOf(t))
	if id == "" {
		id = t.mdl.current
	}
	if id == "" {
		return i18n.T("ui.agent.model.default")
	}
	if n := t.mdl.name(id); n != id {
		return n
	}
	if i := strings.LastIndexByte(id, '/'); i >= 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}

// Models returns the models the active tab's agent reported, for the model menu and tests.
func (p *AgentPanel) Models() []agentmanager.ModelInfo {
	return append([]agentmanager.ModelInfo(nil), p.cur.mdl.offered...)
}

// SetModel chooses model for the active tab ("" for the default) and stores the choice with the
// conversation. A tab without a session starts on it. A live session switches when it can (from
// the next turn); otherwise, and to go back to the default, the session ends, after the running
// turn if there is one, and the next prompt starts it again, resuming the conversation, on the new
// model.
func (p *AgentPanel) SetModel(model string, w *gpui.Window) {
	p.ensureLoaded()
	p.win = w
	model = strings.TrimSpace(model)
	t := p.cur
	p.modelMenu = false
	if p.modelInput != nil {
		p.modelInput.SetText("")
	}
	t.mdl.picked = true
	if model == t.mdl.chosen {
		w.Refresh()
		return
	}
	prev := t.mdl.chosen
	t.mdl.chosen = model
	if t.rec != nil {
		p.check(t.rec.SetModel(model))
	}
	t.mdl.prev = prev
	switch t.state {
	case agentStarting:
		t.mdl.pending = true
	case agentReady:
		p.applyModel(t)
	}
	w.Refresh()
}

// applyModel brings the ready session of tab t onto its chosen model: through ModelSetter on a
// goroutine, or by ending the session (after the running turn) so the next prompt restarts it.
func (p *AgentPanel) applyModel(t *agentTab) {
	model := p.modelFor(t, t.harness)
	if ms, ok := t.sess.(agentmanager.ModelSetter); ok && model != "" {
		t.mdl.setting = true
		gen, prev := t.gen, t.mdl.prev
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), agentModelTimeout)
			defer cancel()
			err := ms.SetModel(ctx, model)
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.gen == gen {
				t.modelRes = &modelResult{gen: gen, model: model, prev: prev, err: err}
			}
		}()
		if p.win != nil {
			p.arm(p.win, agentBusyPoll)
		}
		return
	}
	label := model
	if label == "" {
		label = i18n.T("ui.agent.model.default")
	}
	p.apply(t, agentmanager.Log{Level: "info", Message: i18n.T("ui.agent.model.restarts", label)})
	if t.tr.Running() {
		t.mdl.restart = true
		return
	}
	p.stopTab(t, false)
}

// adoptModel takes the result of a finished model switch of tab t; a refused model is reported
// and the previous choice restored.
func (p *AgentPanel) adoptModel(t *agentTab) {
	t.mu.Lock()
	res := t.modelRes
	t.modelRes = nil
	t.mu.Unlock()
	if res == nil || res.gen != t.gen {
		return
	}
	t.mdl.setting = false
	if res.err == nil {
		return
	}
	p.apply(t, agentmanager.Log{Level: "warn", Message: i18n.T("ui.agent.model.failed", res.model, res.err)})
	if t.mdl.chosen == res.model {
		t.mdl.chosen = res.prev
		if t.rec != nil {
			p.check(t.rec.SetModel(res.prev))
		}
	}
}

// SetDefaultModel makes model the settings' default for new tabs on harness h ("" for the
// harness's own) and saves the settings.
func (p *AgentPanel) SetDefaultModel(h agentmanager.Harness, model string, w *gpui.Window) {
	model = strings.TrimSpace(model)
	p.ws.change(w, func(s *settings.Settings) {
		if s.AgentModels == nil {
			s.AgentModels = map[string]string{}
		}
		if model == "" {
			delete(s.AgentModels, string(h))
		} else {
			s.AgentModels[string(h)] = model
		}
	})
}

// modelChip is the header button that shows the active tab's model and opens the model menu.
func (p *AgentPanel) modelChip() *gpui.Div {
	chip := gpui.NewDiv().ID("agent-model").Relative().FlexRow().ItemsCenter().Gap(4).Px_(8).Py_(2).Rounded(10).Bg(uiPanelAlt).Border(1, uiBorder).FlexShrink(1).MinW(0).
		Tooltip(i18n.T("ui.agent.model.tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			p.ensureLoaded()
			p.modelMenu = !p.modelMenu
			if p.modelMenu {
				if p.modelSearch != nil {
					p.modelSearch.SetText("")
				}
				if p.modelScroll != nil {
					p.modelScroll.ScrollToTop()
				}
			}
			p.hist.menu = false
			w.Refresh()
		}).
		Child(gpui.NewText(clip(p.modelLabel(p.cur), 22)).TextSize(fontSmall), gpui.NewText("v").TextSize(10).TextColor(uiMuted))
	chip.Child(gpui.NewDiv().Absolute().Left(0).Top(0).SizeWH(0, 0).Child(gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		if p.modelAnchor != b.Origin {
			p.modelAnchor = b.Origin
			if p.modelMenu {
				w.Refresh()
			}
		}
	})))
	if p.modelMenu {
		chip.Bg(activeFill())
	}
	return chip
}

// modelRow is one choice of the model menu; on marks the active tab's choice and free adds a
// "free" badge.
func (p *AgentPanel) modelRow(id, label, detail, model string, on, free bool, f func(*gpui.Window)) *gpui.Div {
	mark := gpui.NewText("").TextSize(12)
	if on {
		mark = gpui.NewText("*").TextSize(12).TextColor(uiOK)
	}
	row := gpui.NewDiv().ID(id).FlexRow().ItemsCenter().Gap(6).Px_(6).Py_(3).Rounded(4).Tooltip(i18n.T("ui.agent.model.pick_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(w) }).
		Child(gpui.NewDiv().SizeW(10).FlexShrink(0).Child(mark), gpui.NewText(clip(label, 30)).TextSize(12).Flex1().MinW(0))
	if free {
		row.Child(gpui.NewDiv().ID(id + "-free").Px_(5).Rounded(6).Bg(mix(uiPanelAlt, uiOK, 0.25)).FlexShrink(0).
			Tooltip(i18n.T("ui.agent.model.free_tip")).
			Child(gpui.NewText(i18n.T("ui.agent.model.free")).TextSize(10).TextColor(uiOK)))
	}
	if detail != "" {
		row.Child(gpui.NewText(clip(detail, 24)).TextSize(10).TextColor(uiMuted).FlexShrink(0))
	}
	if model != "" {
		favorite := p.isFavorite(p.harnessOf(p.cur), model)
		glyph, tip, color := "☆", "ui.agent.model.favorite_add", uiMuted
		if favorite {
			glyph, tip, color = "★", "ui.agent.model.favorite_remove", uiWarn
		}
		row.Child(gpui.NewDiv().ID(id+"-star").SizeWH(22, 22).FlexShrink(0).FlexRow().ItemsCenter().JustifyCenter().Rounded(4).Tooltip(i18n.T(tip)).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiField) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				w.StopPropagation()
				p.toggleFavorite(p.harnessOf(p.cur), model, w)
			}).
			Child(gpui.NewText(glyph).TextSize(16).TextColor(color)))
	}
	return row
}

func (p *AgentPanel) isFavorite(h agentmanager.Harness, id string) bool {
	return slices.Contains(p.ws.prefs().cur.AgentFavorites[string(h)], id)
}

func (p *AgentPanel) toggleFavorite(h agentmanager.Harness, id string, w *gpui.Window) {
	p.ws.change(w, func(s *settings.Settings) {
		if s.AgentFavorites == nil {
			s.AgentFavorites = map[string][]string{}
		}
		ids := s.AgentFavorites[string(h)]
		if i := slices.Index(ids, id); i >= 0 {
			ids = slices.Delete(ids, i, i+1)
		} else {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			delete(s.AgentFavorites, string(h))
		} else {
			s.AgentFavorites[string(h)] = ids
		}
	})
	p.ws.SaveSettings()
}

func modelMatches(m agentmanager.ModelInfo, query string) bool {
	return strings.Contains(strings.ToLower(m.ID), query) || strings.Contains(strings.ToLower(m.Name), query)
}

func (p *AgentPanel) menuModels(h agentmanager.Harness) []agentmanager.ModelInfo {
	offered := slices.Clone(orderModels(h, p.cur.mdl.offered))
	if h == agentmanager.OpenCode && p.ws.prefs().cur.AgentMode == settings.AgentBasic && p.ws.prefs().cur.OllamaKey != "" {
		for _, m := range p.ws.prefs().aiBasic.models {
			if !slices.ContainsFunc(offered, func(o agentmanager.ModelInfo) bool { return o.ID == m.ID }) {
				offered = append(offered, m)
			}
		}
	}
	if len(offered) == 0 && h == agentmanager.OpenCode {
		offered = []agentmanager.ModelInfo{{ID: opencodeSuggested}}
	}
	return offered
}

func (p *AgentPanel) modelMenuView(w *gpui.Window, cx *gpui.App) *gpui.Div {
	close := func(w *gpui.Window, _ *gpui.App) { p.modelMenu = false; p.returnFocus(w); w.Refresh() }
	if p.modelInput == nil {
		p.modelInput = themeBareField(gpui.NewTextInput(i18n.T("ui.agent.model.input_placeholder")), fontSmall, false)
		p.modelInput.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { p.SetModel(text, w) })
		p.modelInput.OnEscape(close)
	}
	if p.modelSearch == nil {
		p.modelSearch = themeBareField(gpui.NewTextInput(i18n.T("ui.agent.model.search")), fontSmall, false)
		p.modelScroll = gpui.NewScrollHandle()
		p.modelSearch.OnChange(func(_ string, w *gpui.Window, _ *gpui.App) { p.modelScroll.ScrollToTop(); w.Refresh() })
		p.modelSearch.OnEscape(close)
	}
	p.modelSearch.SetPlaceholder(i18n.T("ui.agent.model.search"))
	t, h := p.cur, p.harnessOf(p.cur)
	query := strings.ToLower(strings.TrimSpace(p.modelSearch.Text()))
	menu := gpui.NewDiv().ID("agent-model-menu").FlexCol().Gap(4).MinH(0).MaxH(320).OverflowHidden().P(4).Rounded(6).Bg(uiPanelAlt).Border(1, uiBorder)
	menu.Child(gpui.NewDiv().ID("agent-model-search").SizeH(28).FlexShrink(0).Px_(6).Rounded(4).Bg(uiField).Border(1, uiBorder).Child(p.modelSearch.Render(w, cx)))
	content := gpui.NewDiv().FlexCol().Gap(2).FlexShrink(0)
	title := func(label string) *gpui.Text {
		return gpui.NewText(label).TextSize(11).FontWeight(700).TextColor(uiMuted).Px_(6)
	}
	eff := t.mdl.chosen
	if eff == "" {
		eff = p.suggestion(t, h)
	}
	offered := p.menuModels(h)
	row := func(id string, m agentmanager.ModelInfo) *gpui.Div {
		label, detail := m.Name, ""
		if label == "" {
			label = m.ID
		} else if label != m.ID {
			detail = m.ID
		}
		return p.modelRow(id, label, detail, m.ID, m.ID == eff, h == agentmanager.OpenCode && freeModel(m.ID), func(w *gpui.Window) { p.SetModel(m.ID, w) })
	}
	var favorites []agentmanager.ModelInfo
	for _, id := range p.ws.prefs().cur.AgentFavorites[string(h)] {
		m := agentmanager.ModelInfo{ID: id}
		if i := slices.IndexFunc(offered, func(o agentmanager.ModelInfo) bool { return o.ID == id }); i >= 0 {
			m = offered[i]
		}
		if modelMatches(m, query) {
			favorites = append(favorites, m)
		}
	}
	if len(favorites) > 0 {
		content.Child(title(i18n.T("ui.agent.model.favorites")))
		for _, m := range favorites {
			content.Child(row("agent-model-favorite-"+m.ID, m))
		}
	}
	content.Child(title(i18n.T("ui.agent.model.menu_title", string(h))))
	def := i18n.T("ui.agent.model.default")
	if d := p.defaultModel(h); d != "" {
		def = i18n.T("ui.agent.model.default_named", t.mdl.name(d))
	}
	if query == "" || strings.Contains(strings.ToLower(def), query) {
		content.Child(p.modelRow("agent-model-default", upperFirst(def), "", "", t.mdl.chosen == "" && p.suggestion(t, h) == "", false, func(w *gpui.Window) { p.SetModel("", w) }))
	}
	matches := 0
	listed := eff == ""
	for _, m := range offered {
		listed = listed || m.ID == eff
		if !modelMatches(m, query) {
			continue
		}
		matches++
		content.Child(row("agent-model-"+m.ID, m))
	}
	if !listed {
		m := agentmanager.ModelInfo{ID: eff}
		if modelMatches(m, query) {
			matches++
			content.Child(row("agent-model-chosen", m))
		}
	}
	if query != "" && matches == 0 && len(favorites) == 0 {
		content.Child(gpui.NewText(i18n.T("ui.agent.model.no_matches")).TextSize(11).TextColor(uiMuted).Px_(6).Py_(2))
	}
	if query == "" && len(offered) == 0 {
		content.Child(gpui.NewText(i18n.T("ui.agent.model.none_listed")).TextSize(11).TextColor(uiMuted).Px_(6).Py_(2))
	}
	content.Child(gpui.NewDiv().ID("agent-model-field").FlexCol().SizeH(26).Mx(4).Px_(6).Rounded(4).Bg(uiField).Border(1, uiBorder).Child(p.modelInput.Render(w, cx)))
	if h != "" && eff != p.defaultModel(h) {
		chosen := eff
		content.Child(gpui.NewDiv().FlexRow().Px_(2).Pt(4).Child(p.button("agent-model-make-default", i18n.T("ui.agent.model.make_default", string(h)), i18n.T("ui.agent.model.make_default_tip", string(h)), false, func(w *gpui.Window) { p.SetDefaultModel(h, chosen, w); p.modelMenu = false })))
	}
	return menu.Child(gpui.NewDiv().ID("agent-model-results").FlexCol().MinH(0).OverflowYScroll().ShowScrollbar().TrackScroll(p.modelScroll).Child(content))
}
