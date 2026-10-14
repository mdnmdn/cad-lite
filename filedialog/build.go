package filedialog

import (
	"fmt"
	"strings"
)

// Backend names a helper program that can show a dialog.
type backend string

const (
	backendOsascript   backend = "osascript"
	backendPowerShell  backend = "powershell"
	backendZenity      backend = "zenity"
	backendKdialog     backend = "kdialog"
	backendUnavailable backend = ""
)

// command is a program and its arguments.
type command struct {
	backend backend
	name    string
	args    []string
}

// chooseBackend picks the backend for goos. have reports whether a program is on the PATH and
// display whether a graphical session exists (only the Linux and BSD backends need one).
func chooseBackend(goos string, have func(string) bool, display bool) backend {
	switch goos {
	case "darwin":
		return backendOsascript
	case "windows":
		return backendPowerShell
	case "js", "wasip1", "plan9", "ios", "android":
		return backendUnavailable
	}
	if !display {
		return backendUnavailable
	}
	switch {
	case have("zenity"):
		return backendZenity
	case have("kdialog"):
		return backendKdialog
	}
	return backendUnavailable
}

// buildCommand builds the program to run for r on backend b.
func buildCommand(b backend, r request) (command, error) {
	switch b {
	case backendOsascript:
		return command{b, "osascript", []string{"-e", osascriptScript(r)}}, nil
	case backendPowerShell:
		return command{b, "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-STA", "-WindowStyle", "Hidden", "-Command", powershellScript(r)}}, nil
	case backendZenity:
		return command{b, "zenity", zenityArgs(r)}, nil
	case backendKdialog:
		return command{b, "kdialog", kdialogArgs(r)}, nil
	}
	return command{}, ErrUnavailable
}

// appleString quotes s as an AppleScript string literal.
func appleString(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(s)
	return `"` + s + `"`
}

// osascriptScript is the AppleScript for r. choose file takes a list of extensions as its type
// restriction; choose file name takes none, so a save dialog is not filtered.
func osascriptScript(r request) string {
	var b strings.Builder
	if r.save {
		b.WriteString("POSIX path of (choose file name")
	} else {
		b.WriteString("POSIX path of (choose file")
		if exts := allExtensions(r.filters); len(exts) > 0 {
			quoted := make([]string, len(exts))
			for i, e := range exts {
				quoted[i] = appleString(e)
			}
			b.WriteString(" of type {" + strings.Join(quoted, ", ") + "}")
		}
	}
	if r.title != "" {
		b.WriteString(" with prompt " + appleString(r.title))
	}
	if r.save && r.defaultName != "" {
		b.WriteString(" default name " + appleString(r.defaultName))
	}
	if r.startDir != "" {
		b.WriteString(" default location (POSIX file " + appleString(r.startDir) + ")")
	}
	b.WriteString(")")
	return b.String()
}

// psString quotes s as a PowerShell single-quoted string.
func psString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// winFilter is the System.Windows.Forms filter string: "Name (*.a;*.b)|*.a;*.b|...".
func winFilter(filters []Filter) string {
	parts := make([]string, 0, len(filters))
	for _, f := range filters {
		pats := "*.*"
		if len(f.Extensions) > 0 {
			p := make([]string, len(f.Extensions))
			for i, e := range f.Extensions {
				p[i] = "*." + e
			}
			pats = strings.Join(p, ";")
		}
		// The pipe separates the filter string's fields; it cannot appear in a name.
		name := strings.ReplaceAll(f.Name, "|", "/")
		parts = append(parts, fmt.Sprintf("%s (%s)|%s", name, pats, pats))
	}
	return strings.Join(parts, "|")
}

// powershellScript is the PowerShell for r: an OpenFileDialog or SaveFileDialog owned by a hidden
// topmost form (so it opens in front), printing the chosen path as UTF-8 and nothing on cancel.
func powershellScript(r request) string {
	kind := "OpenFileDialog"
	if r.save {
		kind = "SaveFileDialog"
	}
	lines := []string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8",
		"[System.Windows.Forms.Application]::EnableVisualStyles()",
		"$d = New-Object System.Windows.Forms." + kind,
	}
	if r.title != "" {
		lines = append(lines, "$d.Title = "+psString(r.title))
	}
	if f := winFilter(r.filters); f != "" {
		lines = append(lines, "$d.Filter = "+psString(f), "$d.FilterIndex = 1")
	}
	if r.startDir != "" {
		lines = append(lines, "$d.InitialDirectory = "+psString(r.startDir))
	}
	if r.save {
		lines = append(lines, "$d.OverwritePrompt = $true", "$d.AddExtension = $true")
		if r.defaultName != "" {
			lines = append(lines, "$d.FileName = "+psString(r.defaultName))
		}
		if e := defaultExtension(r.filters); e != "" {
			lines = append(lines, "$d.DefaultExt = "+psString(e))
		}
	} else {
		lines = append(lines, "$d.CheckFileExists = $true")
	}
	lines = append(lines,
		"$o = New-Object System.Windows.Forms.Form",
		"$o.TopMost = $true",
		"if ($d.ShowDialog($o) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }",
	)
	return strings.Join(lines, "\n")
}

// globPattern is a case-insensitive glob for ext: dxf becomes [dD][xX][fF].
func globPattern(ext string) string {
	var b strings.Builder
	b.WriteString("*.")
	for _, c := range ext {
		lo, up := strings.ToLower(string(c)), strings.ToUpper(string(c))
		if lo == up {
			b.WriteString(lo)
		} else {
			b.WriteString("[" + lo + up + "]")
		}
	}
	return b.String()
}

// zenityArgs is the zenity command line for r. Each filter is "Name | pattern pattern".
func zenityArgs(r request) []string {
	args := []string{"--file-selection"}
	if r.save {
		args = append(args, "--save", "--confirm-overwrite")
	}
	if r.title != "" {
		args = append(args, "--title="+r.title)
	}
	switch {
	case r.startDir != "" && r.defaultName != "":
		args = append(args, "--filename="+strings.TrimRight(r.startDir, "/")+"/"+r.defaultName)
	case r.startDir != "":
		args = append(args, "--filename="+strings.TrimRight(r.startDir, "/")+"/")
	case r.defaultName != "":
		args = append(args, "--filename="+r.defaultName)
	}
	for _, f := range r.filters {
		pats := "*"
		if len(f.Extensions) > 0 {
			p := make([]string, len(f.Extensions))
			for i, e := range f.Extensions {
				p[i] = globPattern(e)
			}
			pats = strings.Join(p, " ")
		}
		args = append(args, "--file-filter="+strings.ReplaceAll(f.Name, "|", "/")+" | "+pats)
	}
	return args
}

// kdialogArgs is the kdialog command line for r: --getopenfilename or --getsavefilename, the start
// location, then one filter string "*.a *.b|Name" per line.
func kdialogArgs(r request) []string {
	var args []string
	if r.title != "" {
		args = append(args, "--title", r.title)
	}
	verb := "--getopenfilename"
	if r.save {
		verb = "--getsavefilename"
	}
	start := "."
	switch {
	case r.startDir != "" && r.defaultName != "":
		start = strings.TrimRight(r.startDir, "/") + "/" + r.defaultName
	case r.startDir != "":
		start = strings.TrimRight(r.startDir, "/") + "/"
	case r.defaultName != "":
		start = r.defaultName
	}
	args = append(args, verb, start)
	lines := make([]string, 0, len(r.filters))
	for _, f := range r.filters {
		pats := "*"
		if len(f.Extensions) > 0 {
			p := make([]string, len(f.Extensions))
			for i, e := range f.Extensions {
				p[i] = "*." + e
			}
			pats = strings.Join(p, " ")
		}
		lines = append(lines, pats+"|"+strings.ReplaceAll(f.Name, "|", "/"))
	}
	if len(lines) > 0 {
		args = append(args, strings.Join(lines, "\n"))
	}
	return args
}

// interpret turns a finished helper into the dialog's result. exit is the exit status (0 when the
// program ran to completion), stdout and stderr what it printed.
func interpret(b backend, stdout, stderr string, exit int) (string, error) {
	path := strings.TrimRight(stdout, "\r\n")
	if b == backendPowerShell {
		// A cancelled dialog prints nothing.
		path = strings.TrimSpace(strings.TrimPrefix(path, "\xef\xbb\xbf"))
	}
	switch {
	case exit == 0 && path != "":
		// kdialog and zenity print one path per line when asked for several; we never ask.
		if i := strings.IndexByte(path, '\n'); i >= 0 {
			path = strings.TrimRight(path[:i], "\r")
		}
		return path, nil
	case exit == 0:
		return "", ErrCancelled
	case b == backendOsascript && exit == 1 && strings.Contains(stderr, "(-128)"):
		// osascript reports "User canceled. (-128)"; any other script error is a failure.
		return "", ErrCancelled
	case b != backendOsascript && exit == 1:
		// zenity and kdialog exit 1 on Cancel.
		return "", ErrCancelled
	}
	return "", fmt.Errorf("filedialog: %s exited with status %d", b, exit)
}
