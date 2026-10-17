package settings

import (
	"reflect"
	"testing"
)

func TestAgentFavoritesRoundTripNormalizeAndClone(t *testing.T) {
	s := Default()
	s.AgentFavorites = map[string][]string{"opencode": {" ollama/model-a ", "ollama/model-a", "", "opencode/model-b"}, "claude": {"model-a"}}
	s.Normalize()
	want := map[string][]string{"opencode": {"ollama/model-a", "opencode/model-b"}, "claude": {"model-a"}}
	if !reflect.DeepEqual(s.AgentFavorites, want) {
		t.Fatalf("favorites: %v", s.AgentFavorites)
	}
	got, warnings, err := Parse(Marshal(s))
	if err != nil || len(warnings) > 0 || !reflect.DeepEqual(got.AgentFavorites, want) {
		t.Fatalf("round trip: %v %v %v", got.AgentFavorites, warnings, err)
	}
	cloned := s.Clone()
	cloned.AgentFavorites["opencode"][0] = "changed"
	delete(cloned.AgentFavorites, "claude")
	if !reflect.DeepEqual(s.AgentFavorites, want) {
		t.Fatal("clone shares favorite state")
	}
}

func TestAgentFavoritesInvalidEntries(t *testing.T) {
	s, warnings, err := Parse([]byte("[agent]\nfavorites_opencode = ['good',42,'good','  ']\nfavorites_claude = 42\n"))
	if err != nil || len(warnings) != 2 || !reflect.DeepEqual(s.AgentFavorites, map[string][]string{"opencode": {"good"}}) {
		t.Fatalf("parse: %v %v %v", s.AgentFavorites, warnings, err)
	}
}
