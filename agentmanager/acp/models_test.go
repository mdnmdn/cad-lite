package acp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agentmanager"
)

func TestModelsListedAndSwitched(t *testing.T) {
	e := builtin(t, "echo-turn,models")
	s := e.open()
	ev, _ := next(t, s)
	if st := ev.(agentmanager.SessionStarted); st.Model != "fake-1" {
		t.Fatalf("model %q", st.Model)
	}
	ev, _ = next(t, s)
	mc := ev.(agentmanager.ModelsChanged)
	want := []agentmanager.ModelInfo{{ID: "fake-1", Name: "Fake One"}, {ID: "fake-2", Name: "Fake Two"}}
	if mc.Current != "fake-1" || len(mc.Available) != 2 || mc.Available[0] != want[0] || mc.Available[1] != want[1] {
		t.Fatalf("%+v", mc)
	}
	ms, ok := s.(agentmanager.ModelSetter)
	if !ok {
		t.Fatal("acp session is not a ModelSetter")
	}
	ctx := context.Background()
	if err := ms.SetModel(ctx, "fake-2"); err != nil {
		t.Fatal(err)
	}
	ev, _ = next(t, s)
	if mc := ev.(agentmanager.ModelsChanged); mc.Current != "fake-2" || mc.Available != nil {
		t.Fatalf("%+v", mc)
	}
	var sets []any
	for _, f := range e.frames() {
		if f["method"] == "session/set_config_option" {
			sets = append(sets, f["params"].(map[string]any)["value"])
		}
	}
	if len(sets) != 1 || sets[0] != "fake-2" {
		t.Fatalf("set_config_option values %v", sets)
	}
	if err := ms.SetModel(ctx, "bad-model"); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("bad model: %v", err)
	}
	if err := ms.SetModel(ctx, ""); !errors.Is(err, agentmanager.ErrNoModel) {
		t.Fatalf("empty model: %v", err)
	}
	s.Close()
	if err := ms.SetModel(ctx, "fake-1"); !errors.Is(err, agentmanager.ErrClosed) {
		t.Fatalf("after close: %v", err)
	}
}

func TestModelsOnlyInModelsField(t *testing.T) {
	e := newEnv(t, `{"steps":[
 {"op":"expect","match":{"method":"initialize"},"var":"i"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$i","result":{"protocolVersion":1}}},
 {"op":"expect","match":{"method":"session/new"},"var":"n"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$n","result":{"sessionId":"s1","models":{"currentModelId":"a","availableModels":[{"modelId":"a","name":"A"},{"modelId":"b"}]}}}},
 {"op":"expect","match":{"method":"session/set_model"},"var":"m"},
 {"op":"send","frame":{"jsonrpc":"2.0","id":"$m","result":{}}},
 {"op":"sleep","ms":2000}]}`)
	s := e.open()
	next(t, s)
	ev, _ := next(t, s)
	if mc := ev.(agentmanager.ModelsChanged); mc.Current != "a" || len(mc.Available) != 2 || mc.Available[1].ID != "b" {
		t.Fatalf("%+v", mc)
	}
	if err := s.(agentmanager.ModelSetter).SetModel(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	if p := e.find("session/set_model")["params"].(map[string]any); p["modelId"] != "b" || p["sessionId"] != "s1" {
		t.Fatalf("%v", p)
	}
}

func TestSetModelWithoutSelection(t *testing.T) {
	e := newEnv(t, `{"steps":[`+handshakeSteps+` {"op":"sleep","ms":2000}]}`)
	s := e.open()
	next(t, s)
	err := s.(agentmanager.ModelSetter).SetModel(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "no model selection") {
		t.Fatalf("%v", err)
	}
}
