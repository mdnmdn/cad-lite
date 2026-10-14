package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateAgentStoreCopiesLegacyFolder(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".go-draw", "agent", "sessions")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "a.json"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateAgentStore(dir)
	got, err := os.ReadFile(filepath.Join(dir, ".cad-lite", "agent", "sessions", "a.json"))
	if err != nil || string(got) != "A" {
		t.Fatalf("copied file = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(old, "a.json")); err != nil {
		t.Fatal("the legacy store must be left in place")
	}
}

func TestMigrateAgentStoreKeepsExistingStore(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".go-draw", "agent"), 0o755)
	os.WriteFile(filepath.Join(dir, ".go-draw", "agent", "old.json"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".cad-lite", "agent"), 0o755)
	migrateAgentStore(dir)
	if _, err := os.Stat(filepath.Join(dir, ".cad-lite", "agent", "old.json")); err == nil {
		t.Fatal("an existing store was merged with the legacy one")
	}
}

func TestMigrateAgentStoreWithoutLegacyDoesNothing(t *testing.T) {
	dir := t.TempDir()
	migrateAgentStore(dir)
	if _, err := os.Stat(filepath.Join(dir, ".cad-lite")); err == nil {
		t.Fatal("created a store folder from nothing")
	}
}
