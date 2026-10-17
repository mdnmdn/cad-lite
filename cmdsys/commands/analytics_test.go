package commands_test

import (
	"encoding/json"
	"strings"
	"testing"

	"cad-lite/analytics"
)

type eventRecord struct {
	name       string
	properties analytics.Properties
}
type eventRecorder struct {
	events []eventRecord
	panics bool
}

func (r *eventRecorder) Track(name string, p analytics.Properties) {
	if r.panics {
		panic("analytics unavailable")
	}
	r.events = append(r.events, eventRecord{name, p})
}

func TestCommandAnalyticsOmitsContentAndPaths(t *testing.T) {
	s, f := fileSession(t)
	r := &eventRecorder{}
	s.SetAnalytics(r)
	s.Script("LINE 0,0 10,0  ")
	s.SetAgentCaller(true)
	s.Run("ID")
	s.Escape()
	s.SetAgentCaller(false)
	s.Script("OPEN /private/customer-secret.dxf ")
	s.Script("SAVEAS /private/customer-secret-copy.dxf ")
	f.open = nil
	s.Script("OPEN /private/missing-secret.dxf ")
	var opened, saved, failed, agent bool
	for _, event := range r.events {
		switch event.name {
		case "drawing_opened":
			opened = true
		case "drawing_saved":
			saved = true
		case "error":
			failed = true
		case "command_used":
			if event.properties["caller"] == "agent" {
				agent = true
			}
		}
		encoded, _ := json.Marshal(event.properties)
		if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "customer-secret") || strings.Contains(string(encoded), "missing-secret") {
			t.Fatalf("event leaks drawing content/path: %s", encoded)
		}
	}
	if !opened || !saved || !failed || !agent {
		t.Fatalf("missing event types: open=%v save=%v error=%v agent=%v", opened, saved, failed, agent)
	}
}

func TestAnalyticsPanicDoesNotInterruptCommand(t *testing.T) {
	s := newSession(t)
	s.SetAnalytics(&eventRecorder{panics: true})
	s.Script("LINE 0,0 10,0  ")
	if lineCount(s) != 1 {
		t.Fatal("analytics provider broke a drawing command")
	}
}
