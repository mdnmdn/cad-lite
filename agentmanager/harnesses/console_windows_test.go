//go:build windows

package harnesses_test

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"agentmanager/harnesses"
)

func init() {
	if os.Getenv("AGENTMANAGER_VERSION_CONSOLE_HELPER") != "1" {
		return
	}
	hwnd, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	fmt.Fprint(os.Stdout, hwnd)
	os.Exit(0)
}

func TestVersionProbeHasNoConsoleWindow(t *testing.T) {
	t.Setenv("AGENTMANAGER_VERSION_CONSOLE_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	version, err := harnesses.Check(ctx, exe)
	if err != nil || version != "0" {
		t.Fatalf("version probe has a console window: %q, error %v", version, err)
	}
}
