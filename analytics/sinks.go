package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Stdout is the dummy provider: one JSON event per line. Use os.Stdout as Writer in the host.
// New serializes calls on its worker; Write failures never reach application actions.
type Stdout struct {
	// Writer receives events and may be replaced by a buffer in tests.
	Writer io.Writer
}

// Send prints one event without adding network credentials.
func (s Stdout) Send(ctx context.Context, event Event) error {
	if s.Writer == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return json.NewEncoder(s.Writer).Encode(event)
}

// PostHog sends anonymous events to PostHog's public capture endpoint using a project token.
// Configure its fields before handing it to New.
type PostHog struct {
	// URL is the ingestion host, optionally including /i/v0/e/.
	URL string
	// Key is the project capture token, not a personal API key.
	Key string
	// HTTP optionally overrides the transport; redirects are always refused.
	HTTP *http.Client
}

// Configured reports whether the optional build values form an HTTP(S) endpoint and nonempty key.
func (p PostHog) Configured() bool {
	u, err := url.Parse(strings.TrimSpace(p.URL))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.TrimSpace(p.Key) != ""
}

// Send posts one JSON capture request. Response bodies, URLs and keys are never returned in errors.
func (p PostHog) Send(ctx context.Context, event Event) error {
	if !p.Configured() {
		return errors.New("analytics: unconfigured")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(p.URL), "/")
	if !strings.HasSuffix(endpoint, "/i/v0/e") {
		endpoint += "/i/v0/e"
	}
	endpoint += "/"
	body, err := json.Marshal(struct {
		APIKey string `json:"api_key"`
		Event
	}{APIKey: p.Key, Event: event})
	if err != nil {
		return errors.New("analytics: invalid event")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("analytics: invalid request")
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 3 * time.Second}
	if p.HTTP != nil {
		client = *p.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return errors.New("analytics: delivery failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("analytics: delivery rejected")
	}
	return nil
}
