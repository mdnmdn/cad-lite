package codex_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"agentmanager"
)

func TestModelsListedAndSwitched(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	e.cfg.Env = append(e.cfg.Env, "FAKEAGENT_MODELS=1")
	s := e.open(t)
	next(t, s)
	mc, ok := next(t, s).(agentmanager.ModelsChanged)
	if !ok || mc.Current != "fake-model" || len(mc.Available) != 2 || mc.Available[1] != (agentmanager.ModelInfo{ID: "fake-mini", Name: "Fake Mini"}) {
		t.Fatalf("%+v", mc)
	}
	ms := s.(agentmanager.ModelSetter)
	if err := ms.SetModel(context.Background(), "fake-mini"); err != nil {
		t.Fatal(err)
	}
	if mc := next(t, s).(agentmanager.ModelsChanged); mc.Current != "fake-mini" || mc.Available != nil {
		t.Fatalf("%+v", mc)
	}
	if err := ms.SetModel(context.Background(), ""); !errors.Is(err, agentmanager.ErrNoModel) {
		t.Fatalf("empty model: %v", err)
	}
	for _, text := range []string{"one", "two"} {
		if err := s.Prompt(context.Background(), text); err != nil {
			t.Fatal(err)
		}
		until(t, s, isTurnEnd)
	}
	n := 0
	for _, l := range strings.Split(logLines(t, e), "\n") {
		var f struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal([]byte(l), &f) == nil && f.Method == "turn/start" {
			n++
			if f.Params["model"] != "fake-mini" {
				t.Errorf("turn/start without the model override: %s", l)
			}
		}
	}
	if n != 2 {
		t.Fatalf("%d turn/start frames", n)
	}
}

func TestModelListUnsupported(t *testing.T) {
	e := setup(t, `{"builtin":"echo-turn"}`)
	s := e.open(t)
	next(t, s)
	if err := s.Prompt(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	for _, ev := range until(t, s, isTurnEnd) {
		if _, ok := ev.(agentmanager.ModelsChanged); ok {
			t.Fatalf("models reported without model/list: %#v", ev)
		}
	}
	if strings.Contains(logLines(t, e), `"model":`) {
		t.Error("turn/start carries a model override nobody asked for")
	}
}
