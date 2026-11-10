package plot

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoPrinting is returned by Print where the platform has no print spooler Cad-lite can reach.
var ErrNoPrinting = errors.New("plot: printing is not available on this system")

// Print hands the PDF file at path to the system's default printer, under the job name title. It
// runs the tool each platform ships: lp (or lpr) through CUPS on Linux, the BSDs and macOS, and
// PowerShell's print verb on Windows, which goes through the program registered to print PDFs.
func Print(ctx context.Context, path, title string) error {
	name, args, err := printCommand(runtime.GOOS, path, title, func(n string) bool { _, err := exec.LookPath(n); return err == nil })
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %s", name, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// printCommand is the program and arguments that print path on goos; has reports whether a
// program is installed.
func printCommand(goos, path, title string, has func(string) bool) (string, []string, error) {
	switch goos {
	case "windows":
		quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Process -FilePath " + quoted + " -Verb Print"}, nil
	case "js", "wasip1", "ios", "android":
		return "", nil, ErrNoPrinting
	}
	switch {
	case has("lp"):
		return "lp", []string{"-t", title, "--", path}, nil
	case has("lpr"):
		return "lpr", []string{"-T", title, path}, nil
	}
	return "", nil, ErrNoPrinting
}
