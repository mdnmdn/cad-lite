package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenTerminal opens the user's terminal in dir, so an agent can be
// installed or logged in to by hand, and returns the name of the terminal it
// started. macOS uses Terminal, Windows PowerShell in a new console window,
// elsewhere the first of $TERMINAL, x-terminal-emulator, gnome-terminal,
// konsole and xterm that is installed.
func OpenTerminal(dir string) (string, error) {
	cmd, name, err := terminalCommand(runtime.GOOS, dir, os.Getenv("TERMINAL"), exec.LookPath)
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }()
	return name, nil
}

// terminalCommand builds the command that opens a terminal in dir on goos.
// userTerm is $TERMINAL; look finds a program on PATH.
func terminalCommand(goos, dir, userTerm string, look func(string) (string, error)) (*exec.Cmd, string, error) {
	if dir != "" {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			dir = ""
		}
	}
	switch goos {
	case "darwin":
		args := []string{"-a", "Terminal"}
		if dir != "" {
			args = append(args, dir)
		}
		return exec.Command("open", args...), "Terminal", nil
	case "windows":
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
		if dir != "" {
			switch filepath.Base(f[0]) {
			case "gnome-terminal":
				args = append(args, "--working-directory="+dir)
			case "konsole":
				args = append(args, "--workdir", dir)
			}
		}
		cmd := exec.Command(path, args...)
		cmd.Dir = dir
		return cmd, filepath.Base(f[0]), nil
	}
	return nil, "", errors.New("no terminal found (tried $TERMINAL, x-terminal-emulator, gnome-terminal, konsole, xterm)")
}
