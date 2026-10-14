package shellenv

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMergeLoginFirstDedupe(t *testing.T) {
	got := merge("/usr/bin:/bin:/usr/bin", "/opt/homebrew/bin::/usr/bin:/home/u/.bun/bin", ':', false)
	want := []string{"/opt/homebrew/bin", "/usr/bin", "/home/u/.bun/bin", "/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMergeWindowsFoldsCase(t *testing.T) {
	got := merge(`C:\Windows;c:\tools`, `C:\TOOLS;"C:\Program Files\Go\bin"`, ';', true)
	want := []string{`C:\TOOLS`, `C:\Program Files\Go\bin`, `C:\Windows`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAddedListsOnlyNewEntries(t *testing.T) {
	got := added("/usr/bin:/bin", "/opt/homebrew/bin:/usr/bin:/opt/homebrew/bin:/x", ':', false)
	if want := []string{"/opt/homebrew/bin", "/x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := Added("/a", "/a"); got != nil {
		t.Fatalf("nothing added, got %v", got)
	}
}

func writeExe(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindReturnsFullPathsInOrder(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	claude := writeExe(t, b, "claude", 0o755)
	opencode := writeExe(t, a, "opencode", 0o755)
	writeExe(t, a, "codex", 0o644) // not executable
	if err := os.MkdirAll(filepath.Join(a, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	got := Find([]string{"opencode", "claude", "codex", "dir", "../x"}, a+":"+b)
	want := []Binary{{"opencode", opencode}, {"claude", claude}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLookupWindowsTriesPathext(t *testing.T) {
	dir := t.TempDir()
	cmd := writeExe(t, dir, "claude.cmd", 0o644)
	exe := writeExe(t, dir, "codex.exe", 0o644)
	got, ok := lookup("claude", dir, "windows", ".COM;.EXE;.CMD")
	if !ok || got != cmd {
		t.Fatalf("claude: %q %v", got, ok)
	}
	if got, ok := lookup("codex.exe", dir, "windows", ""); !ok || got != exe {
		t.Fatalf("codex.exe: %q %v", got, ok)
	}
	if got, ok := lookup("codex", dir, "windows", ""); !ok || got != exe {
		t.Fatalf("codex default pathext: %q %v", got, ok)
	}
	if _, ok := lookup("opencode", dir, "windows", ""); ok {
		t.Fatal("opencode must not be found")
	}
}

func TestParseRegValue(t *testing.T) {
	out := "\r\nHKEY_CURRENT_USER\\Environment\r\n    Path    REG_EXPAND_SZ    C:\\Users\\me\\bin;%USERPROFILE%\\go\\bin;C:\\Program Files\\x\r\n\r\n"
	got, ok := parseRegValue(out, "Path")
	if want := `C:\Users\me\bin;%USERPROFILE%\go\bin;C:\Program Files\x`; !ok || got != want {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := parseRegValue(out, "TEMP"); ok {
		t.Fatal("TEMP is not in the output")
	}
	if _, ok := parseRegValue("ERROR: The system was unable to find the specified registry key", "Path"); ok {
		t.Fatal("error text is not a value")
	}
}

func TestExpandVars(t *testing.T) {
	env := map[string]string{"USERPROFILE": `C:\Users\me`}
	look := func(n string) string { return env[n] }
	for in, want := range map[string]string{
		`%USERPROFILE%\bin`:           `C:\Users\me\bin`,
		`%NOPE%\bin`:                  `%NOPE%\bin`,
		`50%`:                         `50%`,
		`a%%b`:                        `a%%b`,
		`%USERPROFILE%;%USERPROFILE%`: `C:\Users\me;C:\Users\me`,
	} {
		if got := expandVars(in, look); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func fakeShell(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	p := filepath.Join(t.TempDir(), "fakesh")
	// The probe calls: shell -l -i -c SCRIPT, so SCRIPT is $4.
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestShellPATHIgnoresBannerNoise(t *testing.T) {
	sh := fakeShell(t, `echo "Welcome to fake shell"; echo "rc noise" >&2
PATH=/opt/fake/bin:/home/u/.bun/bin:/usr/bin
export PATH
[ "$1 $2 $3" = "-l -i -c" ] || exit 9
eval "$4"
echo " trailing banner"`)
	got, err := shellPATH(context.Background(), sh, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/opt/fake/bin:/home/u/.bun/bin:/usr/bin"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestShellPATHStdinIsClosed(t *testing.T) {
	sh := fakeShell(t, `if read x; then echo "stdin had data"; exit 3; fi
PATH=/ok; eval "$4"`)
	got, err := shellPATH(context.Background(), sh, 5*time.Second)
	if err != nil || got != "/ok" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestShellPATHTimesOut(t *testing.T) {
	sh := fakeShell(t, `sleep 30`)
	start := time.Now()
	_, err := shellPATH(context.Background(), sh, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestShellPATHNoMarkersIsAnError(t *testing.T) {
	sh := fakeShell(t, `echo hello; exit 2`)
	if _, err := shellPATH(context.Background(), sh, 5*time.Second); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := shellPATH(context.Background(), filepath.Join(t.TempDir(), "missing"), time.Second); err == nil {
		t.Fatal("expected an error for a missing shell")
	}
}

func TestLoginPATHUsesSHELLAndRunReportsSource(t *testing.T) {
	sh := fakeShell(t, `PATH=/from/login:/usr/bin; eval "$4"`)
	t.Setenv("SHELL", sh)
	p, err := LoginPATH(context.Background())
	if err != nil || p != "/from/login:/usr/bin" {
		t.Fatalf("got %q %v", p, err)
	}
	r := Run(context.Background())
	if r.Err != nil || r.Source != SourceLoginShell || r.Path != p {
		t.Fatalf("run = %+v", r)
	}
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "gone"))
	r = Run(context.Background())
	if r.Err == nil || r.Source != SourceApp || r.Path != "" {
		t.Fatalf("failed run = %+v", r)
	}
}

func TestSetEffectiveEnv(t *testing.T) {
	t.Cleanup(func() { Set("") })
	t.Setenv("PATH", "/usr/bin:/bin")
	Set("")
	if Env() != nil || Effective() != "/usr/bin:/bin" {
		t.Fatalf("before Set: %v %q", Env(), Effective())
	}
	Set("/extra/bin:/usr/bin")
	if runtime.GOOS == "windows" {
		return
	}
	if got := Effective(); got != "/extra/bin:/usr/bin:/bin" {
		t.Fatalf("effective %q", got)
	}
	if got := Env(); len(got) != 1 || got[0] != "PATH=/extra/bin:/usr/bin:/bin" {
		t.Fatalf("env %v", got)
	}
}

func TestResolveUsesLoginPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Cleanup(func() { Set("") })
	dir := t.TempDir()
	p := writeExe(t, dir, "cadlite-fake-agent", 0o755)
	t.Setenv("PATH", "/usr/bin:/bin")
	Set("")
	if _, ok := Resolve("cadlite-fake-agent"); ok {
		t.Fatal("found without the login PATH")
	}
	Set(dir)
	if got, ok := Resolve("cadlite-fake-agent"); !ok || got != p {
		t.Fatalf("got %q %v", got, ok)
	}
}
