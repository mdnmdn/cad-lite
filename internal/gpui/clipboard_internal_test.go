package gpui

import (
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func toolNames(ts []clipTool) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.read[0])
	}
	return out
}

func TestClipboardToolsLinuxWayland(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := toolNames(clipboardTools("linux", env(nil))); !reflect.DeepEqual(got, []string{"xclip", "xsel"}) {
		t.Errorf("X11 tools = %v", got)
	}
	got := toolNames(clipboardTools("linux", env(map[string]string{"WAYLAND_DISPLAY": "wayland-0"})))
	if !reflect.DeepEqual(got, []string{"wl-paste", "xclip", "xsel"}) {
		t.Errorf("wayland tools = %v", got)
	}
}

func TestClipboardReadFallsThrough(t *testing.T) {
	if _, err := exec.LookPath("printf"); err != nil {
		t.Skip("no printf")
	}
	tools := []clipTool{
		{read: []string{"gpui-no-such-tool"}},
		{read: []string{"false"}},
		{read: []string{"printf", "héllo"}},
	}
	s, err := readWith(tools, time.Second)
	if err != nil || s != "héllo" {
		t.Fatalf("readWith = %q, %v", s, err)
	}
	if _, err := readWith(tools[:2], time.Second); err == nil {
		t.Fatal("expected error when every tool fails")
	}
}

func TestClipboardToolTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep")
	}
	start := time.Now()
	_, err := readWith([]clipTool{{read: []string{"sleep", "5"}}}, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("timeout not enforced: took %v", d)
	}
	start = time.Now()
	if err := writeWith([]clipTool{{write: []string{"sleep", "5"}}}, "x", 100*time.Millisecond); err == nil {
		t.Fatal("expected write timeout error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("write timeout not enforced: took %v", d)
	}
}
