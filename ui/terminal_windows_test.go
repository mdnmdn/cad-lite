//go:build windows

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWindowsTerminalConsoleHandles(t *testing.T) {
	const resultEnv = "CADLITE_TERMINAL_TEST_RESULT"
	const unicodeEnv = "CADLITE_TERMINAL_TEST_UNICODE"
	if result := os.Getenv(resultEnv); result != "" {
		var failures []string
		for _, stream := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			var mode uint32
			if err := syscall.GetConsoleMode(syscall.Handle(stream.Fd()), &mode); err != nil {
				failures = append(failures, fmt.Sprintf("%s is not a console: %v", stream.Name(), err))
			}
		}
		if got := os.Getenv(unicodeEnv); got != "工具 à" {
			failures = append(failures, "Unicode environment: "+got)
		}
		if dir, err := os.Getwd(); err != nil || !strings.EqualFold(dir, filepath.Dir(result)) {
			failures = append(failures, fmt.Sprintf("working directory: %q %v", dir, err))
		}
		output := "ok"
		if len(failures) > 0 {
			output = strings.Join(failures, "\n")
		}
		if err := os.WriteFile(result+".tmp", []byte(output), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(result+".tmp", result); err != nil {
			t.Fatal(err)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	result := filepath.Join(dir, "console-result.txt")
	cmd := exec.Command(exe, "-test.run=^TestWindowsTerminalConsoleHandles$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), resultEnv+"="+result, unicodeEnv+"=工具 à")
	// Match os/exec's nil-stream redirection even when the parent has a console.
	nul, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer nul.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nul, nul, nul
	newConsole(cmd, "")
	if err := startTerminal(cmd); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(result)
		if err == nil {
			if string(data) != "ok" {
				t.Fatal(string(data))
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("console child did not report its standard handles")
}
