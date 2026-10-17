// Package feedback sends application feedback to the intake service, keeps a private retry outbox,
// and edits screenshot annotations without depending on the UI framework.
package feedback

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Report is the service's JSON intake body. Credentials never belong in a report or its context.
type Report struct {
	// ID is the stable ULID shared by all delivery attempts.
	ID string `json:"id"`
	// Title is the short report summary.
	Title string `json:"title"`
	// Kind is a category accepted by the configured application.
	Kind string `json:"kind"`
	// Description contains reproduction steps or a suggestion.
	Description string `json:"description,omitempty"`
	// Version is the application release version.
	Version string `json:"version,omitempty"`
	// Platform is the operating system identifier.
	Platform string `json:"platform,omitempty"`
	// Context holds non-sensitive application and runtime metadata.
	Context map[string]any `json:"context,omitempty"`
	// Screenshot is a raw base64 PNG, without a data URL prefix.
	Screenshot string `json:"screenshot_png_base64,omitempty"`
}

// Receipt identifies a successfully accepted report. The administrative URL is deliberately omitted.
type Receipt struct {
	// Reference is the public receipt shown to the user.
	Reference string `json:"reference"`
}

// Info describes the application and categories accepted by the configured intake key.
type Info struct {
	// OK indicates successful service authentication.
	OK bool `json:"ok"`
	// Application identifies the service application associated with the key.
	Application struct {
		// Slug is the service application identifier.
		Slug string `json:"slug"`
		// Name is the service application display name.
		Name string `json:"name"`
	} `json:"application"`
	// Kinds lists categories currently accepted by the service.
	Kinds []string `json:"kinds"`
}

// Error describes an intake failure without echoing response bodies, URLs or credentials.
type Error struct {
	// Status is the HTTP status, or zero for a transport failure.
	Status int
	// Retryable permits bounded automatic delivery retries.
	Retryable bool
}

// Error returns a credential-free description of the failure.
func (e *Error) Error() string {
	switch e.Status {
	case 0:
		return "Cannot reach the feedback service."
	case 400:
		return "The feedback service rejected the report. Check its fields and category."
	case 401, 403:
		return "Feedback is unavailable in this build. Its service key needs updating."
	case 409:
		return "The report ID conflicts with another report. Clear the form and submit again."
	case 413:
		return "The report or screenshot is too large. Remove the screenshot and try again."
	case 429:
		return "The feedback service is busy. Try again later."
	default:
		return fmt.Sprintf("Feedback service returned HTTP %d.", e.Status)
	}
}

// Client is the desktop intake configuration. URL and Key are injected by the host, never persisted.
type Client struct {
	// URL is the service base URL or intake endpoint.
	URL string
	// Key authenticates this desktop application.
	Key string
	// HTTP optionally supplies the transport and timeout. Redirects are always refused.
	HTTP *http.Client
	// Outbox stores private pending reports before delivery.
	Outbox *Outbox
	// Attempts bounds one delivery, including the initial attempt; zero means three.
	Attempts int
	// Wait replaces cancellable backoff in deterministic tests; nil uses a timer.
	Wait func(context.Context, time.Duration) error
}

// Configured reports whether both intake configuration values and an HTTP(S) service URL are valid.
func (c *Client) Configured() bool {
	if c == nil || strings.TrimSpace(c.Key) == "" {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if !strings.HasSuffix(base, "/api/feedback") {
		base += "/api/feedback"
	}
	return base
}

func (c *Client) request(ctx context.Context, endpoint string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.Key)
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	// Intake keys must not follow an HTTP redirect to another service.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, &Error{Retryable: true}
	}
	return res, nil
}

// Test checks connectivity without creating a report and retrieves the application's accepted kinds.
func (c *Client) Test(ctx context.Context) (Info, error) {
	var info Info
	if !c.Configured() {
		return info, errors.New("Feedback is not configured in this build.")
	}
	res, err := c.request(ctx, c.endpoint()+"/test", []byte("{}"))
	if err != nil {
		return info, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return info, &Error{Status: res.StatusCode, Retryable: retryStatus(res.StatusCode)}
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&info) != nil || !info.OK || len(info.Kinds) == 0 {
		return Info{}, errors.New("Invalid feedback service response.")
	}
	kinds := info.Kinds[:0]
	for _, kind := range info.Kinds {
		if strings.TrimSpace(kind) != "" {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) == 0 {
		return Info{}, errors.New("The feedback service has no report categories.")
	}
	info.Kinds = kinds
	return info, nil
}

// NewID creates a ULID for one report. Reuse it for every retry of that report.
func NewID() (string, error) {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var id [26]byte
	stamp := uint64(time.Now().UnixMilli())
	for i := 9; i >= 0; i-- {
		id[i] = alphabet[stamp&31]
		stamp >>= 5
	}
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", errors.New("Cannot create a feedback reference.")
	}
	copy(id[10:], base32.NewEncoding(alphabet).WithPadding(base32.NoPadding).EncodeToString(entropy[:]))
	return string(id[:]), nil
}

// Validate checks the service's field and body limits before any outbox write or network request.
func (r Report) Validate() error {
	if !validID(r.ID) {
		return errors.New("Invalid feedback report ID.")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(r.Title)); n < 1 || n > 300 {
		return errors.New("Enter a title of 1–300 characters.")
	}
	if strings.TrimSpace(r.Kind) == "" {
		return errors.New("Choose a feedback category.")
	}
	if utf8.RuneCountInString(r.Description) > 20000 {
		return errors.New("Description exceeds 20,000 characters.")
	}
	if utf8.RuneCountInString(r.Version) > 100 || utf8.RuneCountInString(r.Platform) > 100 {
		return errors.New("Feedback version or platform is too long.")
	}
	context, err := json.Marshal(r.Context)
	if err != nil || len(context) > 16384 {
		return errors.New("Feedback metadata is too large.")
	}
	if len(r.Screenshot) > 14000000 {
		return errors.New("Screenshot is too large. Remove it and try again.")
	}
	if r.Screenshot != "" {
		data, decodeErr := base64.StdEncoding.DecodeString(r.Screenshot)
		if decodeErr != nil {
			return errors.New("Screenshot must be PNG base64.")
		}
		cfg, pngErr := png.DecodeConfig(bytes.NewReader(data))
		if pngErr != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32000000 {
			return errors.New("Screenshot must be a PNG under 32 megapixels.")
		}
	}
	body, err := json.Marshal(r)
	if err != nil || len(body) > 15000000 {
		return errors.New("Feedback report is too large.")
	}
	return nil
}

func validID(id string) bool {
	if len(id) != 26 || id[0] > '7' {
		return false
	}
	for _, ch := range id {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", ch) {
			return false
		}
	}
	return true
}

func retryStatus(status int) bool {
	return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
}

// Send delivers a report using bounded backoff and the same ID on each attempt. It does not save an outbox.
func (c *Client) Send(ctx context.Context, r Report) (Receipt, error) {
	var receipt Receipt
	if !c.Configured() {
		return receipt, errors.New("Feedback is not configured in this build.")
	}
	if err := r.Validate(); err != nil {
		return receipt, err
	}
	body, _ := json.Marshal(r)
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	attempts = min(attempts, 8)
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return Receipt{}, err
		}
		res, err := c.request(ctx, c.endpoint(), body)
		delay := time.Second * time.Duration(1<<i)
		if err == nil {
			status := res.StatusCode
			if status == http.StatusOK || status == http.StatusCreated {
				receipt = Receipt{}
				err = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&receipt)
				res.Body.Close()
				if err != nil || strings.TrimSpace(receipt.Reference) == "" {
					err = &Error{Retryable: true}
				} else {
					return receipt, nil
				}
			} else {
				if status == 429 {
					delay = max(delay, 5*time.Second)
				}
				if seconds, parseErr := strconv.Atoi(res.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
					delay = max(delay, time.Duration(min(seconds, 60))*time.Second)
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
				res.Body.Close()
				err = &Error{Status: status, Retryable: retryStatus(status)}
			}
		}
		var failure *Error
		if !errors.As(err, &failure) || !failure.Retryable || i == attempts-1 {
			return Receipt{}, err
		}
		wait := c.Wait
		if wait == nil {
			wait = waitContext
		}
		if err := wait(ctx, min(delay, time.Minute)); err != nil {
			return Receipt{}, err
		}
	}
	return Receipt{}, errors.New("Feedback delivery failed.")
}

func waitContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Deliver saves a private outbox copy before sending and removes it only after acknowledgement.
func (c *Client) Deliver(ctx context.Context, r Report) (Receipt, error) {
	if c.Outbox == nil {
		return Receipt{}, errors.New("Feedback outbox is unavailable.")
	}
	if !c.Configured() {
		return Receipt{}, errors.New("Feedback is not configured in this build.")
	}
	if err := c.Outbox.Save(r); err != nil {
		return Receipt{}, err
	}
	receipt, err := c.Send(ctx, r)
	if err == nil {
		if removeErr := c.Outbox.Remove(r.ID); removeErr != nil {
			return receipt, errors.New("Feedback was sent, but its retry copy could not be removed.")
		}
	}
	return receipt, err
}

// RetryPending retries a bounded batch of saved reports. Permanent rejections remain for manual correction.
func (c *Client) RetryPending(ctx context.Context) (int, error) {
	if !c.Configured() || c.Outbox == nil {
		return 0, nil
	}
	reports, err := c.Outbox.List()
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, report := range reports[:min(len(reports), 3)] {
		if err := ctx.Err(); err != nil {
			return sent, err
		}
		_, err := c.Deliver(ctx, report)
		if err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
