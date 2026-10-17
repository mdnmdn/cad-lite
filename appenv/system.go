package appenv

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"
)

// SystemMetadata returns OS, OS version and architecture without user identity or file paths.
// OS version queries are bounded to two seconds; failure uses "unknown".
func SystemMetadata(ctx context.Context) map[string]any {
	osVersion := "unknown"
	query, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"/usr/bin/sw_vers", "-productVersion"}
	case "windows":
		args = []string{"cmd.exe", "/d", "/c", "ver"}
	case "linux":
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
					osVersion = strings.Trim(value, "\"'")
				}
			}
		}
		if osVersion == "unknown" {
			args = []string{"uname", "-r"}
		}
	}
	if len(args) > 0 {
		if data, err := systemVersionOutput(query, args); err == nil {
			osVersion = strings.TrimSpace(string(data))
		}
	}
	if strings.TrimSpace(osVersion) == "" {
		osVersion = "unknown"
	}
	return map[string]any{"os": runtime.GOOS, "os_version": osVersion, "architecture": runtime.GOARCH}
}
