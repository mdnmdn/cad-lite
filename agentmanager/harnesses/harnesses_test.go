package harnesses_test

import (
	"agentmanager/shellenv"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func fakeCLI(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscoverReportsPathsAndVersions(t *testing.T) {
	dir := t.TempDir()
	oc := fakeCLI(t, dir, "opencode", `echo; echo "opencode 1.2.3"; echo second`)
	fakeCLI(t, dir, "codex", `sleep 30`)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got := harnesses.Discover(ctx, dir)
	if len(got) != 3 || got[0].Harness != agentmanager.OpenCode || got[1].Harness != agentmanager.Claude || got[2].Harness != agentmanager.Codex {
		t.Fatalf("order: %+v", got)
	}
	if got[0].Path != oc || got[0].Version != "opencode 1.2.3" {
		t.Errorf("opencode: %+v", got[0])
	}
	if got[1].Path != "" || got[1].Version != "" {
		t.Errorf("claude should be missing: %+v", got[1])
	}
	if got[2].Path == "" || got[2].Version != "" {
		t.Errorf("codex is found but silent: %+v", got[2])
	}
}

func TestAvailableUsesLoginPATH(t *testing.T) {
	dir := t.TempDir()
	p := fakeCLI(t, dir, "claude", `exit 0`)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Cleanup(func() { shellenv.Set("") })
	shellenv.Set("")
	if _, ok := harnesses.Available(agentmanager.Claude); ok {
		t.Skip("a real claude is installed here")
	}
	shellenv.Set(dir)
	if got, ok := harnesses.Available(agentmanager.Claude); !ok || got != p {
		t.Fatalf("got %q %v", got, ok)
	}
}

func TestOpenLaunchesFromLoginPATHWithMergedEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "path.txt")
	// Not a real agent: it records its PATH and exits, so Open fails, but it must have been started.
	fakeCLI(t, dir, "claude", `echo "$PATH" > `+out+`; exit 1`)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Cleanup(func() { shellenv.Set("") })
	shellenv.Set(dir)
	s, err := harnesses.Open(context.Background(), agentmanager.Config{Harness: agentmanager.Claude, Cwd: dir})
	if s != nil {
		s.Close()
	}
	_ = err
	var b []byte
	for i := 0; i < 100 && len(b) == 0; i++ {
		b, _ = os.ReadFile(out)
		time.Sleep(20 * time.Millisecond)
	}
	if got := strings.TrimSpace(string(b)); !strings.HasPrefix(got, dir+":") {
		t.Fatalf("child PATH = %q, want it to start with %s", got, dir)
	}
}
