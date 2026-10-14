package harnesses_test

import (
	"context"
	"testing"

	"agentmanager"
	"agentmanager/harnesses"
)

func TestOpenUnknownHarness(t *testing.T) {
	if _, err := harnesses.Open(context.Background(), agentmanager.Config{Harness: "nope"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAvailableMissing(t *testing.T) {
	if p, ok := harnesses.Available("definitely-not-a-real-cli"); ok {
		t.Fatalf("found %s", p)
	}
}
