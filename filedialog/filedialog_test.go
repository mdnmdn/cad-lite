package filedialog

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var dxfDwg = []Filter{
	{Name: "All supported files", Extensions: []string{"dxf", "dwg"}},
	{Name: "DXF drawing", Extensions: []string{".DXF"}},
	{Name: "All files"},
}

func TestNormalize(t *testing.T) {
	got := normalize([]Filter{{Name: "x", Extensions: []string{".DXF", "*.Dwg", " ", "dxf"}}})
	if want := []string{"dxf", "dwg", "dxf"}; !slices.Equal(got[0].Extensions, want) {
		t.Fatalf("got %v want %v", got[0].Extensions, want)
	}
}

func TestChooseBackend(t *testing.T) {
	has := func(names ...string) func(string) bool {
		return func(n string) bool { return slices.Contains(names, n) }
	}
	for _, c := range []struct {
		goos    string
		have    func(string) bool
		display bool
		want    backend
	}{
		{"darwin", has(), false, backendOsascript},
		{"windows", has(), false, backendPowerShell},
		{"linux", has("zenity", "kdialog"), true, backendZenity},
		{"freebsd", has("kdialog"), true, backendKdialog},
		{"linux", has(), true, backendUnavailable},
		{"linux", has("zenity"), false, backendUnavailable},
		{"js", has("zenity"), true, backendUnavailable},
	} {
		if got := chooseBackend(c.goos, c.have, c.display); got != c.want {
			t.Errorf("%s: got %q want %q", c.goos, got, c.want)
		}
	}
	if _, err := buildCommand(backendUnavailable, request{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no backend: %v", err)
	}
}

func TestOsascriptOpen(t *testing.T) {
	got := osascriptScript(request{title: `Open "x"`, startDir: "/Users/a b", filters: normalize(dxfDwg[:2])})
	want := `POSIX path of (choose file of type {"dxf", "dwg"} with prompt "Open \"x\"" default location (POSIX file "/Users/a b"))`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// An "All files" entry lifts the type restriction.
	if got := osascriptScript(request{title: "T", filters: normalize(dxfDwg)}); strings.Contains(got, "of type") {
		t.Fatalf("restricted despite All files: %s", got)
	}
}

func TestOsascriptSave(t *testing.T) {
	got := osascriptScript(request{save: true, title: "Save", defaultName: "a.dxf", startDir: "/tmp", filters: normalize(dxfDwg[1:2])})
	want := `POSIX path of (choose file name with prompt "Save" default name "a.dxf" default location (POSIX file "/tmp"))`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestWinFilter(t *testing.T) {
	got := winFilter(normalize(dxfDwg))
	want := "All supported files (*.dxf;*.dwg)|*.dxf;*.dwg|DXF drawing (*.dxf)|*.dxf|All files (*.*)|*.*"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestPowershellScript(t *testing.T) {
	open := powershellScript(request{title: "It's", startDir: `C:\Users\a`, filters: normalize(dxfDwg)})
	for _, w := range []string{
		"New-Object System.Windows.Forms.OpenFileDialog",
		"$d.Title = 'It''s'",
		`$d.InitialDirectory = 'C:\Users\a'`,
		"$d.Filter = 'All supported files (*.dxf;*.dwg)|*.dxf;*.dwg|",
		"$d.ShowDialog($o)",
	} {
		if !strings.Contains(open, w) {
			t.Errorf("open script lacks %q:\n%s", w, open)
		}
	}
	save := powershellScript(request{save: true, defaultName: "x.dxf", filters: normalize(dxfDwg[1:2])})
	for _, w := range []string{"SaveFileDialog", "$d.FileName = 'x.dxf'", "$d.OverwritePrompt = $true", "$d.DefaultExt = 'dxf'"} {
		if !strings.Contains(save, w) {
			t.Errorf("save script lacks %q:\n%s", w, save)
		}
	}
	c, _ := buildCommand(backendPowerShell, request{})
	if !slices.Contains(c.args, "-STA") || c.name != "powershell.exe" {
		t.Fatalf("powershell invocation: %v", c)
	}
}

func TestZenityArgs(t *testing.T) {
	got := zenityArgs(request{title: "Open", startDir: "/home/a/", filters: normalize(dxfDwg)})
	want := []string{
		"--file-selection", "--title=Open", "--filename=/home/a/",
		"--file-filter=All supported files | *.[dD][xX][fF] *.[dD][wW][gG]",
		"--file-filter=DXF drawing | *.[dD][xX][fF]",
		"--file-filter=All files | *",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	save := zenityArgs(request{save: true, title: "Save", startDir: "/home/a", defaultName: "n.dxf", filters: normalize(dxfDwg[1:2])})
	if save[1] != "--save" || save[2] != "--confirm-overwrite" || !slices.Contains(save, "--filename=/home/a/n.dxf") {
		t.Fatalf("save args %q", save)
	}
}

func TestKdialogArgs(t *testing.T) {
	got := kdialogArgs(request{title: "Open", filters: normalize(dxfDwg)})
	want := []string{"--title", "Open", "--getopenfilename", ".", "*.dxf *.dwg|All supported files\n*.dxf|DXF drawing\n*|All files"}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	save := kdialogArgs(request{save: true, startDir: "/d", defaultName: "n.dxf", filters: normalize(dxfDwg[1:2])})
	if want := []string{"--getsavefilename", "/d/n.dxf", "*.dxf|DXF drawing"}; !slices.Equal(save, want) {
		t.Fatalf("got %q want %q", save, want)
	}
}

func TestInterpret(t *testing.T) {
	for _, c := range []struct {
		b      backend
		out    string
		errOut string
		exit   int
		path   string
		err    error
	}{
		{backendZenity, "/a/b.dxf\n", "", 0, "/a/b.dxf", nil},
		{backendZenity, "", "", 1, "", ErrCancelled},
		{backendKdialog, "", "", 1, "", ErrCancelled},
		{backendOsascript, "/Users/a/x.dxf\n", "", 0, "/Users/a/x.dxf", nil},
		{backendOsascript, "", "execution error: User canceled. (-128)", 1, "", ErrCancelled},
		{backendPowerShell, "", "", 0, "", ErrCancelled},
		{backendPowerShell, "\xef\xbb\xbfC:\\a\\b.dxf", "", 0, `C:\a\b.dxf`, nil},
	} {
		path, err := interpret(c.b, c.out, c.errOut, c.exit)
		if path != c.path || !errors.Is(err, c.err) {
			t.Errorf("%s exit %d: got %q, %v", c.b, c.exit, path, err)
		}
	}
	if _, err := interpret(backendZenity, "", "boom", 5); err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("exit 5 is a failure, got %v", err)
	}
	if _, err := interpret(backendOsascript, "", "syntax error", 1); err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("osascript script error is a failure, got %v", err)
	}
}

func TestDefaultExtension(t *testing.T) {
	f := normalize(dxfDwg)
	if got := withDefaultExt("/a/drawing", defaultExtension(f)); got != "/a/drawing.dxf" {
		t.Fatalf("got %q", got)
	}
	if got := withDefaultExt("/a/drawing.dwg", "dxf"); got != "/a/drawing.dwg" {
		t.Fatalf("an existing extension is kept, got %q", got)
	}
	if got := withDefaultExt("/a/drawing", defaultExtension([]Filter{{Name: "All"}})); got != "/a/drawing" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenWithoutBackendIsUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("these systems always have a backend")
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("PATH", t.TempDir())
	if _, err := Open(context.Background(), "t", "", nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
