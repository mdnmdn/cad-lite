package proc_test

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"agentmanager/internal/proc"
)

// TestMain doubles as the helper child: with PROC_HELPER set the test binary acts as a tiny program.
func TestMain(m *testing.M) {
	switch os.Getenv("PROC_HELPER") {
	case "":
		os.Exit(m.Run())
	case "echo-env":
		os.Stdout.WriteString(os.Getenv("PROC_PROBE") + "|" + os.Getenv("PROC_EXTRA"))
	case "cat":
		io.Copy(os.Stdout, os.Stdin)
	case "stderr":
		os.Stderr.WriteString(strings.Repeat("a", 20000) + "TAIL")
		os.Exit(3)
	case "ignore-stdin":
		os.Stdout.WriteString("up\n")
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

func start(t *testing.T, helper string, remove, extra []string) *proc.Process {
	t.Helper()
	p, err := proc.Start(proc.Options{
		Path:      os.Args[0],
		Env:       append([]string{"PROC_HELPER=" + helper}, extra...),
		RemoveEnv: remove,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(0) })
	return p
}

func TestEnvFunc(t *testing.T) {
	got := proc.Env([]string{"A=1", "AB=2", "B=3"}, []string{"A", "B"}, []string{"C=4"})
	if strings.Join(got, ",") != "AB=2,C=4" {
		t.Fatalf("got %v", got)
	}
}

func TestEnvRemovalAndExtra(t *testing.T) {
	t.Setenv("PROC_PROBE", "leak")
	p := start(t, "echo-env", []string{"PROC_PROBE"}, []string{"PROC_EXTRA=yes"})
	out, _ := io.ReadAll(p.Stdout())
	<-p.Done()
	if string(out) != "|yes" {
		t.Fatalf("out=%q", out)
	}
}

func TestStdioRoundTripAndGracefulClose(t *testing.T) {
	p := start(t, "cat", nil, nil)
	io.WriteString(p.Stdin(), "hello\n")
	buf := make([]byte, 6)
	if _, err := io.ReadFull(p.Stdout(), buf); err != nil || string(buf) != "hello\n" {
		t.Fatalf("%q %v", buf, err)
	}
	start := time.Now()
	p.Close(5 * time.Second)
	if time.Since(start) > 3*time.Second {
		t.Fatal("graceful close took the grace period")
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("not done")
	}
	if p.ExitErr() != nil {
		t.Fatalf("exit err %v", p.ExitErr())
	}
	p.Close(time.Second) // idempotent
}

func TestStderrTail(t *testing.T) {
	p := start(t, "stderr", nil, nil)
	<-p.Done()
	tail := p.StderrTail()
	if !strings.HasSuffix(tail, "TAIL") || len(tail) > 9000 {
		t.Fatalf("len=%d tail=%q", len(tail), tail[max(0, len(tail)-20):])
	}
	if p.ExitErr() == nil {
		t.Fatal("expected exit error")
	}
}

func TestCloseKillsProcessIgnoringStdin(t *testing.T) {
	p := start(t, "ignore-stdin", nil, nil)
	line := make([]byte, 3)
	io.ReadFull(p.Stdout(), line)
	begin := time.Now()
	p.Close(100 * time.Millisecond)
	if d := time.Since(begin); d > 5*time.Second {
		t.Fatalf("close took %v", d)
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("still running")
	}
	if p.ExitErr() == nil {
		t.Fatal("killed process should report an error")
	}
}

func TestStartMissingBinary(t *testing.T) {
	if _, err := proc.Start(proc.Options{Path: "/nonexistent/binary"}); err == nil {
		t.Fatal("expected error")
	}
}

func openFDs(t *testing.T) int {
	t.Helper()
	es, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("no /proc/self/fd")
	}
	return len(es)
}

func TestCloseReleasesPipeDescriptors(t *testing.T) {
	before := openFDs(t)
	for range 20 {
		p, err := proc.Start(proc.Options{Path: os.Args[0], Env: []string{"PROC_HELPER=ignore-stdin"}})
		if err != nil {
			t.Fatal(err)
		}
		p.Close(10 * time.Millisecond)
	}
	if after := openFDs(t); after > before+4 {
		t.Fatalf("descriptors leaked: %d before, %d after 20 processes", before, after)
	}
}
