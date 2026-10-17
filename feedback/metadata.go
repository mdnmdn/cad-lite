package feedback

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Metadata describes the application and runtime, excluding paths, user names, keys and drawing contents.
func Metadata(ctx context.Context, version, locale, viewport string) map[string]any {
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
		if data, err := exec.CommandContext(query, args[0], args[1:]...).Output(); err == nil {
			osVersion = strings.TrimSpace(string(data))
		}
	}
	context := map[string]any{"application": "cad-lite", "application_name": "Cad-lite", "application_version": version, "os": runtime.GOOS, "os_version": osVersion, "architecture": runtime.GOARCH, "runtime": runtime.Version(), "locale": locale, "viewport": viewport, "client_ts": time.Now().UTC().Format(time.RFC3339)}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				context["build"] = setting.Value
			}
		}
	}
	return context
}
