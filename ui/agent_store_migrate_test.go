package ui

import (
	"os"
	"path/filepath"
	"testing"

	"cad-lite/doc"
)

func TestPrepareAgentStoreClearsCurrentAndLegacyHistoryOnce(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{agentStoreDir, legacyAgentStoreDir} {
		path := filepath.Join(dir, filepath.FromSlash(sub))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"corrupt.jsonl", "open.json", "open-old.json"} {
			if err := os.WriteFile(filepath.Join(path, name), []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	unrelated := filepath.Join(dir, ".cad-lite", "keep.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewWorkspace(doc.New(), nil).AgentPanel()
	st, err := p.prepareAgentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{agentStoreDir, legacyAgentStoreDir} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sub))); !os.IsNotExist(err) {
			t.Fatal("old store remains")
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("cleanup removed unrelated data")
	}
	if err := os.MkdirAll(st.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(st.Dir(), "current.jsonl")
	if err := os.WriteFile(current, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.prepareAgentStore(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatal("second drawing cleared current-run history")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err == nil {
		if _, err := p.prepareAgentStore(alias); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(current); err != nil {
			t.Fatal("project alias cleared current-run history")
		}
	}
}

func TestPrepareAgentStoreWithoutHistoryCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	p := NewWorkspace(doc.New(), nil).AgentPanel()
	if _, err := p.prepareAgentStore(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cad-lite")); !os.IsNotExist(err) {
		t.Fatal("created a store before first prompt")
	}
}
