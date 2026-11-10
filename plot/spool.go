package plot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ErrNoPrinting is returned by Print and Printers where the platform has no print spooler
// Cad-lite can reach.
var ErrNoPrinting = errors.New("plot: printing is not available on this system")

// MaxCopies bounds Job.Copies.
const MaxCopies = 99

// Printer is a print queue the system knows.
type Printer struct {
	// Name is the queue name the spooler takes.
	Name string
	// Default marks the system's default printer.
	Default bool
}

// Job is how a PDF is printed.
type Job struct {
	// Title is the job name the spooler shows.
	Title string
	// Printer is the queue to print on; "" is the system default.
	Printer string
	// Copies is the number of copies, 1 to MaxCopies; 0 is one.
	Copies int
}

// runner runs a program and returns its output; tests replace it.
type runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runProgram(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// lpstat's messages are parsed: keep them in English.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return out, fmt.Errorf("%s: %s", name, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func has(name string) bool { _, err := exec.LookPath(name); return err == nil }

// Print hands the PDF file at path to the system's print spooler as job says. It runs the tool
// each platform ships: lp (or lpr) through CUPS on Linux, the BSDs and macOS, and PowerShell's
// PrintTo verb on Windows, which goes through the program registered to print PDFs.
func Print(ctx context.Context, path string, job Job) error {
	name, args, err := printCommand(runtime.GOOS, path, job, has)
	if err != nil {
		return err
	}
	_, err = runProgram(ctx, name, args...)
	return err
}

// copies clamps n to 1..MaxCopies.
func copies(n int) int { return min(max(n, 1), MaxCopies) }

// psQuote is s as a PowerShell single-quoted string.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// printCommand is the program and arguments that print path on goos; has reports whether a
// program is installed.
func printCommand(goos, path string, job Job, has func(string) bool) (string, []string, error) {
	n := copies(job.Copies)
	switch goos {
	case "windows":
		start := "Start-Process -FilePath " + psQuote(path) + " -Verb Print"
		if job.Printer != "" {
			// PrintTo takes the printer as its argument, quoted for the program's command line.
			start = "Start-Process -FilePath " + psQuote(path) + " -Verb PrintTo -ArgumentList " + psQuote(`"`+job.Printer+`"`)
		}
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", "for ($i = 0; $i -lt " + strconv.Itoa(n) + "; $i++) { " + start + " }"}, nil
	case "js", "wasip1", "ios", "android":
		return "", nil, ErrNoPrinting
	}
	switch {
	case has("lp"):
		args := []string{"-t", job.Title, "-n", strconv.Itoa(n)}
		if job.Printer != "" {
			args = append(args, "-d", job.Printer)
		}
		return "lp", append(args, "--", path), nil
	case has("lpr"):
		args := []string{"-T", job.Title, "-#", strconv.Itoa(n)}
		if job.Printer != "" {
			args = append(args, "-P", job.Printer)
		}
		return "lpr", append(args, path), nil
	}
	return "", nil, ErrNoPrinting
}

// Printers lists the system's print queues, the default one marked, in the order the system gives.
func Printers(ctx context.Context) ([]Printer, error) {
	return listPrinters(ctx, runtime.GOOS, has, runProgram)
}

func listPrinters(ctx context.Context, goos string, has func(string) bool, run runner) ([]Printer, error) {
	switch goos {
	case "windows":
		out, err := run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			"Get-CimInstance Win32_Printer | ForEach-Object { $_.Name + \"`t\" + $_.Default }")
		if err != nil {
			return nil, err
		}
		var ps []Printer
		for _, l := range lines(out) {
			name, def, _ := strings.Cut(l, "\t")
			ps = append(ps, Printer{Name: name, Default: strings.EqualFold(strings.TrimSpace(def), "true")})
		}
		return ps, nil
	case "js", "wasip1", "ios", "android":
		return nil, ErrNoPrinting
	}
	if !has("lpstat") {
		if has("lp") || has("lpr") {
			return nil, nil // a spooler without a way to list its queues: the default only
		}
		return nil, ErrNoPrinting
	}
	// -e (CUPS 2) lists the destinations by name; older systems answer -a with "name accepting...".
	var names []string
	if out, err := run(ctx, "lpstat", "-e"); err == nil {
		names = lines(out)
	} else if out, err := run(ctx, "lpstat", "-a"); err == nil {
		for _, l := range lines(out) {
			names = append(names, strings.Fields(l)[0])
		}
	}
	def := ""
	if out, err := run(ctx, "lpstat", "-d"); err == nil {
		if _, after, ok := strings.Cut(string(out), "destination:"); ok {
			def = strings.TrimSpace(after)
		}
	}
	ps := make([]Printer, 0, len(names))
	for _, n := range names {
		ps = append(ps, Printer{Name: n, Default: n == def})
	}
	return ps, nil
}

// lines are the non-blank lines of out, trimmed.
func lines(out []byte) []string {
	var ls []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// OpenViewer opens the PDF at path in the system's PDF viewer, where its print command shows the
// operating system's own print dialog. It returns once the viewer is started.
func OpenViewer(path string) error {
	name, args, err := viewerCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	go cmd.Wait()
	return nil
}

func viewerCommand(goos, path string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{path}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", path}, nil
	case "js", "wasip1", "ios", "android":
		return "", nil, ErrNoPrinting
	}
	return "xdg-open", []string{path}, nil
}
