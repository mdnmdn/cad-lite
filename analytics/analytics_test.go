package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type sinkFunc func(context.Context, Event) error

func (f sinkFunc) Send(ctx context.Context, event Event) error { return f(ctx, event) }

func flush(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStdoutMetadataAndSnapshots(t *testing.T) {
	var output bytes.Buffer
	c := New(Stdout{Writer: &output}, "1.2.3")
	props := Properties{"format": "dxf", "nested": map[string]string{"secret": "not supported"}}
	c.Track("app_started", props)
	props["format"] = "changed"
	c.Track("drawing_opened", nil)
	flush(t, c)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("events: %s", output.String())
	}
	var first, second Event
	if json.Unmarshal([]byte(lines[0]), &first) != nil || json.Unmarshal([]byte(lines[1]), &second) != nil {
		t.Fatal("invalid stdout JSON")
	}
	if first.Properties["format"] != "dxf" || first.Properties["nested"] != nil {
		t.Fatalf("properties were not copied/filtered: %v", first.Properties)
	}
	if first.Properties["os"] != runtime.GOOS || first.Properties["architecture"] != runtime.GOARCH || first.Properties["os_version"] == "" || first.Properties["application_version"] != "1.2.3" {
		t.Fatalf("missing startup metadata: %v", first.Properties)
	}
	if first.DistinctID == "" || first.DistinctID != second.DistinctID || first.Timestamp.IsZero() || first.Properties["$process_person_profile"] != false {
		t.Fatal("events lack anonymous session identity or timestamp")
	}
	c.Track("closed", nil)
}

func TestPostHogCapture(t *testing.T) {
	var captured map[string]any
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/prefix/i/v0/e/" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("invalid capture request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()
	c := New(PostHog{URL: srv.URL + "/prefix", Key: "project-token"}, "dev")
	c.Track("app_started", nil)
	flush(t, c)
	if requests != 1 || captured["api_key"] != "project-token" || captured["event"] != "app_started" || captured["distinct_id"] == "" {
		t.Fatalf("capture body: %v", captured)
	}
	props := captured["properties"].(map[string]any)
	if props["os_version"] == "" || props["$process_person_profile"] != false {
		t.Fatal("missing anonymous runtime properties")
	}
}

func TestFailuresPanicsAndSaturationNeverBlockTrack(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	c := New(sinkFunc(func(ctx context.Context, _ Event) error {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return errors.New("offline")
	}), "dev")
	c.Track("start", nil)
	select {
	case <-entered:
	case <-time.After(4 * time.Second):
		t.Fatal("worker did not start")
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			c.Track("command_used", Properties{"command": "LINE"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full queue blocked caller")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Close(ctx); err == nil {
		t.Fatal("slow sink did not honor bounded shutdown")
	}
	close(release)
	panics := 0
	c = New(sinkFunc(func(context.Context, Event) error {
		panics++
		if panics == 1 {
			panic("provider failed")
		}
		return io.ErrClosedPipe
	}), "dev")
	c.Track("one", nil)
	c.Track("two", nil)
	flush(t, c)
	if panics != 2 {
		t.Fatal("provider panic stopped worker")
	}
}

func TestPostHogRedirectFailureAndCancellation(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	p := PostHog{URL: source.URL, Key: "private-token"}
	err := p.Send(context.Background(), Event{Name: "test"})
	if err == nil || reached || strings.Contains(err.Error(), p.Key) || strings.Contains(err.Error(), source.URL) {
		t.Fatalf("redirect/credential isolation: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.Send(ctx, Event{Name: "test"}) == nil {
		t.Fatal("ignored cancellation")
	}
	for _, p := range []PostHog{{}, {URL: "https://example.test"}, {URL: "file:///tmp/key", Key: "x"}, {URL: "https://example.test?token=x", Key: "x"}} {
		if p.Configured() {
			t.Fatal("accepted missing/invalid build configuration")
		}
	}
}
