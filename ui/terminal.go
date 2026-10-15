package ui

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"agentmanager/shellenv"
	"cad-lite/i18n"
)

// OpenTerminal opens the user's terminal in dir, so an agent can be
// installed or logged in to by hand, and returns the name of the terminal it
// started. macOS uses Terminal, Windows PowerShell in a new console window,
// elsewhere the first of $TERMINAL, x-terminal-emulator, gnome-terminal,
// konsole and xterm that is installed.
func OpenTerminal(dir string) (string, error) { return OpenTerminalCommand(dir, nil) }

// OpenTerminalCommand opens the user's terminal in dir running argv (the
// program and its arguments) and returns the terminal's name; with no argv it
// opens a plain shell, as OpenTerminal does. The window stays open when the
// program ends, so its last words can be read: macOS runs it with Terminal's
// "do script", Windows in PowerShell -NoExit (cmd /k when PowerShell is
// missing), elsewhere through sh, which waits for Enter.
func OpenTerminalCommand(dir string, argv []string) (string, error) {
	cmd, name, err := terminalCommandArgs(runtime.GOOS, dir, os.Getenv("TERMINAL"), argv, exec.LookPath)
	if err != nil {
		return "", err
	}
	if len(argv) > 0 {
		cmd.Env = append(os.Environ(), shellenv.Env()...)
	}
	if err := startTerminal(cmd); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return name, nil
}

// TerminalOpener opens a terminal in dir running argv (a plain shell when argv
// is empty) and returns the name of what it opened; OpenTerminalCommand is the
// real one.
type TerminalOpener func(dir string, argv []string) (string, error)

// terminalCommand builds the command that opens a plain terminal in dir.
func terminalCommand(goos, dir, userTerm string, look func(string) (string, error)) (*exec.Cmd, string, error) {
	return terminalCommandArgs(goos, dir, userTerm, nil, look)
}

// terminalCommandArgs builds the command that opens a terminal in dir on goos,
// running argv when it is not empty. userTerm is $TERMINAL; look finds a
// program on PATH.
func terminalCommandArgs(goos, dir, userTerm string, argv []string, look func(string) (string, error)) (*exec.Cmd, string, error) {
	if dir != "" {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			dir = ""
		}
	}
	switch goos {
	case "darwin":
		if len(argv) > 0 {
			script := posixCommand(argv)
			if dir != "" {
				script = "cd " + posixQuote(dir) + " && " + script
			}
			return exec.Command("osascript", "-e", `tell application "Terminal"`, "-e", "activate",
				"-e", "do script "+appleScriptString(script), "-e", "end tell"), "Terminal", nil
		}
		args := []string{"-a", "Terminal"}
		if dir != "" {
			args = append(args, dir)
		}
		return exec.Command("open", args...), "Terminal", nil
	case "windows":
		if len(argv) > 0 {
			return windowsTerminal(dir, argv, look)
		}
		cmd := exec.Command("cmd", "/c", "start", "powershell")
		cmd.Dir = dir
		return cmd, "PowerShell", nil
	case "js":
		return nil, "", errors.New("not available in the browser")
	}
	var cands []string
	if f := strings.Fields(userTerm); len(f) > 0 {
		cands = append(cands, userTerm)
	}
	cands = append(cands, "x-terminal-emulator", "gnome-terminal", "konsole", "xterm")
	for _, c := range cands {
		f := strings.Fields(c)
		path, err := look(f[0])
		if err != nil {
			continue
		}
		args := append([]string(nil), f[1:]...)
		base := filepath.Base(f[0])
		if dir != "" {
			switch base {
			case "gnome-terminal":
				args = append(args, "--working-directory="+dir)
			case "konsole":
				args = append(args, "--workdir", dir)
			}
		}
		if len(argv) > 0 {
			switch last := f[len(f)-1]; {
			case base == "gnome-terminal":
				args = append(args, "--")
			case len(f) > 1 && (last == "-e" || last == "--" || last == "-x"):
			default:
				args = append(args, "-e")
			}
			args = append(args, "sh", "-c", keepOpenScript, i18n.T("ui.agent.install.terminal_close"))
			args = append(args, argv...)
		}
		cmd := exec.Command(path, args...)
		cmd.Dir = dir
		return cmd, base, nil
	}
	return nil, "", errors.New("no terminal found (tried $TERMINAL, x-terminal-emulator, gnome-terminal, konsole, xterm)")
}

// keepOpenScript runs its arguments, then prints $0 (the "press Enter"
// message) and waits, so the terminal does not vanish with the program.
const keepOpenScript = `"$@"; echo; printf '%s\n' "$0"; read _`

// windowsTerminal runs argv in a new PowerShell console that stays open, or in
// cmd /k when PowerShell is missing. PowerShell receives a UTF-16LE encoded
// script, so Windows argument escaping cannot alter its shell literals.
// The cmd fallback uses a verbatim command line for its special quoting rules.
func windowsTerminal(dir string, argv []string, look func(string) (string, error)) (*exec.Cmd, string, error) {
	var cmd *exec.Cmd
	name := "PowerShell"
	if ps, err := look("powershell"); err == nil {
		parts := make([]string, len(argv))
		for i, a := range argv {
			parts[i] = "'" + strings.ReplaceAll(a, "'", "''") + "'"
		}
		script := "& " + strings.Join(parts, " ")
		units := utf16.Encode([]rune(script))
		encoded := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(encoded[i*2:], unit)
		}
		cmd = exec.Command(ps, "-NoProfile", "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
		newConsole(cmd, "")
	} else {
		name = "cmd"
		parts := make([]string, len(argv))
		for i, a := range argv {
			parts[i] = `"` + strings.ReplaceAll(a, `"`, "") + `"`
		}
		line := strings.Join(parts, " ")
		cmd = exec.Command("cmd", "/k", line)
		newConsole(cmd, `cmd.exe /k "`+line+`"`)
	}
	cmd.Dir = dir
	return cmd, name, nil
}

// posixQuote quotes s for a POSIX shell.
func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// posixCommand is argv as one POSIX shell command line.
func posixCommand(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = posixQuote(a)
	}
	return strings.Join(parts, " ")
}

// appleScriptString is s as an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
