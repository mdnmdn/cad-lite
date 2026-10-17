package cmdsys

import (
	"path/filepath"
	"strings"

	"cad-lite/analytics"
)

// SetAnalytics installs a nonblocking event recorder; nil disables session analytics.
// Events contain command names, format categories and error codes, never input text or paths.
func (s *Session) SetAnalytics(recorder analytics.Recorder) { s.analytics = recorder }

func (s *Session) track(name string, properties analytics.Properties) {
	analytics.Record(s.analytics, name, properties)
}

func (s *Session) trackCommand(name string) {
	caller := "user"
	if s.agentCaller {
		caller = "agent"
	}
	s.track("command_used", analytics.Properties{"command": name, "caller": caller})
}

func (s *Session) trackMessage(key string, args []any) {
	switch key {
	case "cmd.file.msg.opened", "cmd.file.msg.saved":
		format := "other"
		if len(args) > 0 {
			if path, ok := args[0].(string); ok {
				switch ext := strings.ToLower(filepath.Ext(path)); ext {
				case ".dxf", ".dwg":
					format = strings.TrimPrefix(ext, ".")
				}
			}
		}
		name := "drawing_opened"
		if key == "cmd.file.msg.saved" {
			name = "drawing_saved"
		}
		s.track(name, analytics.Properties{"format": format})
	default:
		if strings.Contains(key, "_failed") || strings.Contains(key, ".err.") || key == "ui.file.unsupported" {
			s.track("error", analytics.Properties{"source": "command", "code": key})
		}
	}
}
