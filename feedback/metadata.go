package feedback

import (
	"context"
	"runtime"
	"runtime/debug"
	"time"

	"cad-lite/appenv"
)

// Metadata describes the application and runtime, excluding paths, user names, keys and drawing contents.
func Metadata(ctx context.Context, version, locale, viewport string) map[string]any {
	system := appenv.SystemMetadata(ctx)

	context := map[string]any{"application": "cad-lite", "application_name": "Cad-lite", "application_version": version, "os": runtime.GOOS, "os_version": system["os_version"], "architecture": runtime.GOARCH, "runtime": runtime.Version(), "locale": locale, "viewport": viewport, "client_ts": time.Now().UTC().Format(time.RFC3339)}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				context["build"] = setting.Value
			}
		}
	}
	return context
}
