package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScenario(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "sc.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadScenario(t *testing.T) {
	sc, err := LoadScenario("")
	if err != nil || sc.Builtin != "" || len(sc.Steps) != 0 {
		t.Fatalf("empty: %+v %v", sc, err)
	}
	p := writeScenario(t, `{"builtin":"echo-turn","steps":[{"op":"expect","match":{"method":"initialize"},"var":"i"},{"op":"send","frame":{"id":"$i"}},{"op":"sleep","ms":5},{"op":"exit","code":3}]}`)
	sc, err = LoadScenario(p)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Builtin != "echo-turn" || len(sc.Steps) != 4 || sc.Steps[0].Var != "i" || sc.Steps[3].Code != 3 || sc.Steps[2].MS != 5 {
		t.Fatalf("%+v", sc)
	}
}

func TestLoadScenarioErrors(t *testing.T) {
	if _, err := LoadScenario(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file")
	}
	if _, err := LoadScenario(writeScenario(t, `{not json`)); err == nil {
		t.Fatal("bad json")
	}
	if _, err := LoadScenario(writeScenario(t, `{"steps":[{"op":"bogus"}]}`)); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("bad op: %v", err)
	}
}

func TestMatch(t *testing.T) {
	frame := map[string]any{"method": "x", "id": float64(3), "params": map[string]any{"a": "b", "list": []any{1.0, 2.0}}}
	cases := []struct {
		want map[string]any
		ok   bool
	}{
		{map[string]any{"method": "x"}, true},
		{map[string]any{"id": 3}, true},
		{map[string]any{"params": map[string]any{"a": "*"}}, true},
		{map[string]any{"params": map[string]any{"list": []any{1}}}, true},
		{map[string]any{"method": "y"}, false},
		{map[string]any{"missing": "*"}, false},
		{map[string]any{"params": map[string]any{"a": "c"}}, false},
	}
	for i, c := range cases {
		if got := Match(c.want, frame); got != c.ok {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestRunStepsVarsAndLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log")
	t.Setenv("FAKEAGENT_LOG", logPath)
	in := strings.NewReader("\nnoise\n{\"id\":7,\"method\":\"hello\"}\n{\"id\":9,\"result\":{}}\n{\"id\":1,\"result\":{}}\n")
	var out bytes.Buffer
	r := NewRunner(in, &out)
	code := r.RunSteps([]Step{
		{Op: "expect", Match: map[string]any{"method": "hello"}, Var: "rid"},
		{Op: "send", Frame: map[string]any{"jsonrpc": "2.0", "id": "$rid", "result": map[string]any{"echo": []any{"$rid"}}}},
		{Op: "send", Frame: map[string]any{"id": 9, "method": "ask"}},
		{Op: "wait-for-reply", Var: "rid"},
		{Op: "exit", Code: 4},
	})
	// wait-for-reply looks for id 7 reply; only id 9 and 1 are queued, so stdin ends first.
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), `"id":7`) || !strings.Contains(out.String(), `"echo":[7]`) {
		t.Fatalf("out=%s", out.String())
	}
	logged, _ := os.ReadFile(logPath)
	if strings.Count(string(logged), "\n") != 4 || !strings.Contains(string(logged), "noise") {
		t.Fatalf("log=%q", logged)
	}
}

func TestRunStepsWaitForReplyAndExit(t *testing.T) {
	in := strings.NewReader("{\"id\":5,\"method\":\"m\"}\n{\"id\":5,\"method\":\"req-not-reply\"}\n{\"id\":5,\"result\":1}\n")
	r := NewRunner(in, io.Discard)
	code := r.RunSteps([]Step{
		{Op: "expect", Match: map[string]any{"method": "m"}, Var: "x"},
		{Op: "wait-for-reply", Var: "x"},
		{Op: "exit", Code: 4},
	})
	if code != 4 {
		t.Fatalf("code=%d", code)
	}
	if code := NewRunner(strings.NewReader(""), io.Discard).RunSteps(nil); code != 0 {
		t.Fatalf("empty steps code=%d", code)
	}
}

func TestRunNoProtocol(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestDumpArgv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "argv")
	t.Setenv("FAKEAGENT_ARGV", p)
	dumpArgv([]string{"-p", "--flag"})
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "ARGV\n-p\n--flag\nENV\n") {
		t.Fatalf("%q", b[:min(len(b), 40)])
	}
}
