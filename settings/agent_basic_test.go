package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBasicKeysPersistPrivately(t *testing.T) {
	s := Default()
	s.OpenCodeKey, s.OllamaKey = "opencode-test-key", "ollama-test-key"
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.AgentMode != AgentBasic || got.OpenCodeKey != s.OpenCodeKey || got.OllamaKey != s.OllamaKey {
		t.Fatal("Basic settings did not round trip")
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && stat.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions: %v", stat.Mode())
	}
}

func TestAdvancedAgentMigration(t *testing.T) {
	s, _, err := Parse([]byte("[agent]\ndefault = 'claude'\n"))
	if err != nil || s.AgentMode != AgentAdvanced {
		t.Fatal("existing custom agent must retain Advanced mode")
	}
	s, _, err = Parse([]byte("[agent]\nmode = 'advanced'\n"))
	if err != nil || s.AgentMode != AgentAdvanced {
		t.Fatal("Advanced auto was not retained")
	}
}

func TestMalformedAIKeyDoesNotEnterWarning(t *testing.T) {
	_, warnings, err := Parse([]byte("[agent]\nopencode_key = ['private-test-value']\n"))
	if err != nil || len(warnings) != 1 {
		t.Fatal("invalid key type was not reported")
	}
	for _, w := range warnings {
		if strings.Contains(w, "private-test-value") {
			t.Fatal("private key entered warning")
		}
	}
}
