package ui_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/settings"
)

// modelSession is a fakeSession that switches models like a real backend: SetModel records the
// model and reports it with ModelsChanged, or fails for refuse.
type modelSession struct {
	*fakeSession
	refuse string
	mu     sync.Mutex
	sets   []string
}

func (m *modelSession) SetModel(_ context.Context, model string) error {
	m.mu.Lock()
	m.sets = append(m.sets, model)
	m.mu.Unlock()
	if model == m.refuse {
		return errors.New("unknown model")
	}
	m.events <- agentmanager.ModelsChanged{Current: model}
	return nil
}

func (m *modelSession) setList() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.sets...)
}

// withModelSessions makes every session the rig starts a modelSession and returns them in order.
func withModelSessions(r *agentRig) func(i int) *modelSession {
	var mu sync.Mutex
	var all []*modelSession
	r.ws.AgentPanel().SetOpener(func(_ context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
		r.cfg <- cfg
		s := &modelSession{fakeSession: newFakeSession(), refuse: "bad-model"}
		mu.Lock()
		all = append(all, s)
		mu.Unlock()
		return s, nil
	})
	return func(i int) *modelSession {
		mu.Lock()
		defer mu.Unlock()
		if i < len(all) {
			return all[i]
		}
		return nil
	}
}

func (r *agentRig) note(i int, text string) bool {
	for _, b := range r.ws.AgentPanel().TabTranscript(i).Blocks() {
		if n, ok := b.(transcript.Note); ok && strings.Contains(n.Text, text) {
			return true
		}
	}
	return false
}

func TestModelChosenBeforeStartAndSwitchedLive(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, nil, true)
	sess := withModelSessions(r)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	if m := p.Tabs()[0].Model; m != "default" {
		t.Fatalf("label before any choice %q", m)
	}
	p.SetModel(" fake-fast ", w)
	p.Send("hi", w)
	if cfg := r.nextCfg(); cfg.Model != "fake-fast" {
		t.Fatalf("Config.Model %q", cfg.Model)
	}
	r.until("prompt", func() bool { return sess(0) != nil && len(sess(0).promptList()) == 1 })
	s := sess(0)
	s.events <- agentmanager.SessionStarted{SessionID: "S1", Model: "fake-fast-2026"}
	s.events <- agentmanager.ModelsChanged{Current: "fake-fast", Available: []agentmanager.ModelInfo{
		{ID: "fake-fast", Name: "Fake Fast"}, {ID: "fake-deep", Name: "Fake Deep"}}}
	s.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("models", func() bool { return len(p.Models()) == 2 && !p.Transcript().Running() })
	if m := p.Tabs()[0].Model; m != "Fake Fast" {
		t.Fatalf("label %q", m)
	}

	p.SetModel("fake-deep", w)
	r.until("switch", func() bool { return len(s.setList()) == 1 && p.Tabs()[0].Model == "Fake Deep" })
	if s.isClosed() {
		t.Fatal("a live switch restarted the session")
	}
	p.SetModel("bad-model", w)
	r.until("refusal", func() bool { return r.note(0, "cannot switch to model bad-model") })
	if m := p.Tabs()[0].Model; m != "Fake Deep" {
		t.Fatalf("refused model kept: %q", m)
	}
	id := p.Tabs()[0].SessionID
	p.CloseTab(0, w)
	p.OpenSession(id, w)
	if m := p.Tabs()[0].Model; m != "Fake Deep" {
		t.Fatalf("reopened model %q", m)
	}
	if len(p.Models()) != 2 {
		t.Fatalf("reopened models %v", p.Models())
	}
	p.Send("again", w)
	if cfg := r.nextCfg(); cfg.Model != "fake-deep" || cfg.ResumeID != "S1" {
		t.Fatalf("resumed with %q / %q", cfg.Model, cfg.ResumeID)
	}
}

func TestModelSwitchWithoutSetterRestartsAfterTurn(t *testing.T) {
	r := openAgent(t)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	r.say("hi", 0)
	r.nextCfg()
	r.fs.events <- agentmanager.SessionStarted{SessionID: "R1"}
	r.until("started", func() bool { return p.Transcript().Running() })
	p.SetModel("sonnet", w)
	if r.fs.isClosed() {
		t.Fatal("restarted during the turn")
	}
	r.fs.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("restart", r.fs.isClosed)
	if !r.note(0, "Model sonnet applies from the next prompt") {
		t.Fatal("no note about the restart")
	}
	r.say("next", 1)
	if cfg := r.nextCfg(); cfg.Model != "sonnet" || cfg.ResumeID != "R1" {
		t.Fatalf("restart config %q / %q", cfg.Model, cfg.ResumeID)
	}
	// Back to the default: the session ends at once when idle.
	r.sess(1).events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	r.until("idle", func() bool { return !p.Transcript().Running() })
	p.SetModel("", w)
	r.until("closed", r.sess(1).isClosed)
	r.say("third", 2)
	if cfg := r.nextCfg(); cfg.Model != "" {
		t.Fatalf("default model %q", cfg.Model)
	}
}

func TestDefaultModelPerHarness(t *testing.T) {
	r := openAgent(t)
	p, w := r.ws.AgentPanel(), r.hw.Window()
	s := settings.Default()
	s.AgentModels = map[string]string{"claude": "haiku", "codex": "gpt-x"}
	r.ws.SetSettings(s, w)
	if m := p.Tabs()[0].Model; m != "haiku" {
		t.Fatalf("label %q", m)
	}
	r.say("hi", 0)
	if cfg := r.nextCfg(); cfg.Model != "haiku" {
		t.Fatalf("Config.Model %q", cfg.Model)
	}
	p.SetModel("opus", w)
	p.SetDefaultModel(agentmanager.Claude, "opus", w)
	if got := r.ws.Settings().AgentModels; got["claude"] != "opus" || got["codex"] != "gpt-x" {
		t.Fatalf("settings %v", got)
	}
	p.SetDefaultModel(agentmanager.Claude, "", w)
	if _, ok := r.ws.Settings().AgentModels["claude"]; ok {
		t.Fatal("empty default not removed")
	}
}

func TestModelMenuRenders(t *testing.T) {
	r := openAgent(t)
	r.click(r.pos("agent-model"))
	r.tick()
	r.pos("agent-model-menu")
	r.pos("agent-model-default")
	r.click(r.pos("agent-model-field"))
	r.typ("my-model")
	r.enter()
	r.tick()
	if m := r.ws.AgentPanel().Tabs()[0].Model; m != "my-model" {
		t.Fatalf("typed model %q", m)
	}
}
