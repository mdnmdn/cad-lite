//go:build windows

package proc_test

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"agentmanager/internal/proc"
)

func init() {
	if os.Getenv("PROC_CONSOLE_HELPER") != "1" {
		return
	}
	hwnd, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	fmt.Fprint(os.Stdout, hwnd)
	os.Exit(0)
}

func TestAgentLaunchHasNoConsoleWindow(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := proc.Start(proc.Options{Path: exe, Env: []string{"PROC_CONSOLE_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(0)
	timer := time.AfterFunc(10*time.Second, func() { p.Close(0) })
	defer timer.Stop()
	output, err := io.ReadAll(p.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	<-p.Done()
	if p.ExitErr() != nil || string(output) != "0" {
		t.Fatalf("agent has a console window: %q, exit %v", output, p.ExitErr())
	}
}
