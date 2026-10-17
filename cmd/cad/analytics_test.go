package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cad-lite/analytics"
)

func TestBuildInjectedAnalyticsStartsWithSystemMetadata(t *testing.T) {
	oldURL, oldKey := posthogURLBase64, posthogKeyBase64
	t.Cleanup(func() { posthogURLBase64, posthogKeyBase64 = oldURL, oldKey })
	t.Setenv("CADLITE_ANALYTICS", "")
	var events []analytics.Event
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event analytics.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
		}
		events = append(events, event)
		if event.Name == "app_started" {
			started <- struct{}{}
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	posthogURLBase64 = base64.StdEncoding.EncodeToString([]byte(srv.URL))
	posthogKeyBase64 = base64.StdEncoding.EncodeToString([]byte("test-project-key"))
	c := startAnalytics()
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		t.Fatal("startup event not delivered")
	}
	stopAnalytics(c)
	if len(events) != 2 || events[0].Name != "app_started" || events[1].Name != "app_stopped" {
		t.Fatalf("startup/shutdown events: %+v", events)
	}
	for _, key := range []string{"os", "os_version", "architecture"} {
		if events[0].Properties[key] == nil || events[0].Properties[key] == "" {
			t.Fatalf("startup missing %s", key)
		}
	}
	t.Setenv("CADLITE_ANALYTICS", "off")
	if startAnalytics() != nil {
		t.Fatal("opt-out did not disable analytics")
	}
}
