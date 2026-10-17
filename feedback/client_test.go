package feedback

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testReport(t *testing.T) Report {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return Report{ID: id, Title: "Drawing issue", Kind: "bug", Version: "1.2.3", Platform: "test", Context: map[string]any{"application": "cad-lite"}}
}

func TestProtocolAndOutboxRetry(t *testing.T) {
	report := testReport(t)
	var ids []string
	var bodies [][]byte
	out := &Outbox{Dir: filepath.Join(t.TempDir(), "outbox")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing intake authentication or JSON POST")
		}
		if r.URL.Path == "/api/feedback/test" {
			w.Write([]byte(`{"ok":true,"application":{"slug":"cad-lite","name":"Cad-lite"},"kinds":["bug","idea"]}`))
			return
		}
		var raw json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		bodies = append(bodies, raw)
		var got Report
		json.Unmarshal(raw, &got)
		ids = append(ids, got.ID)
		saved, err := out.List()
		if err != nil || len(saved) != 1 || saved[0].ID != report.ID {
			t.Error("report was not saved before delivery")
		}
		if len(ids) < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"reference":"FB-42","url":"https://private/admin"}`))
	}))
	defer srv.Close()
	var waits []time.Duration
	c := &Client{URL: srv.URL, Key: "test-key", Outbox: out, Wait: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
	info, err := c.Test(context.Background())
	if err != nil || !reflect.DeepEqual(info.Kinds, []string{"bug", "idea"}) {
		t.Fatalf("test response: %+v %v", info, err)
	}
	receipt, err := c.Deliver(context.Background(), report)
	if err != nil || receipt.Reference != "FB-42" {
		t.Fatalf("delivery: %+v %v", receipt, err)
	}
	if len(ids) != 3 || ids[0] != ids[1] || ids[1] != ids[2] || !bytes.Equal(bodies[0], bodies[2]) {
		t.Fatal("retry changed report identity/body")
	}
	if !reflect.DeepEqual(waits, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatalf("backoff: %v", waits)
	}
	saved, _ := out.List()
	if len(saved) != 0 {
		t.Fatal("acknowledged report remains saved")
	}
}

func TestPermanentFailureAndStartupRetry(t *testing.T) {
	report := testReport(t)
	status := 401
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if status != 200 {
			w.WriteHeader(status)
			w.Write([]byte("secret-body"))
			return
		}
		w.Write([]byte(`{"reference":"FB-43"}`))
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL + "/api/feedback", Key: "secret-key", Outbox: &Outbox{Dir: t.TempDir()}}
	_, err := c.Deliver(context.Background(), report)
	if err == nil || requests != 1 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("permanent failure: %v / %d", err, requests)
	}
	saved, _ := c.Outbox.List()
	if len(saved) != 1 {
		t.Fatal("failed report missing from outbox")
	}
	data, _ := os.ReadFile(filepath.Join(c.Outbox.Dir, report.ID+".json"))
	if bytes.Contains(data, []byte(c.Key)) {
		t.Fatal("key persisted")
	}
	status = 200
	n, err := c.RetryPending(context.Background())
	if err != nil || n != 1 || requests != 2 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(c.Outbox.Dir)
		if info.Mode().Perm() != 0700 {
			t.Fatal("outbox not private")
		}
	}
}

func TestRedirectNeverReceivesKey(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	c := &Client{URL: source.URL, Key: "private", Attempts: 1}
	if _, err := c.Send(context.Background(), testReport(t)); err == nil || leaked {
		t.Fatalf("redirect leaked credential: %v", err)
	}
}

func TestValidationOutboxAndMetadata(t *testing.T) {
	r := testReport(t)
	out := &Outbox{Dir: t.TempDir()}
	cases := []func(*Report){func(r *Report) { r.ID = "../escape" }, func(r *Report) { r.Title = " " }, func(r *Report) { r.Title = strings.Repeat("x", 301) }, func(r *Report) { r.Description = strings.Repeat("x", 20001) }, func(r *Report) { r.Screenshot = "data:image/png;base64,broken" }, func(r *Report) { r.Context = map[string]any{"large": strings.Repeat("x", 16384)} }}
	for _, change := range cases {
		bad := r
		change(&bad)
		if out.Save(bad) == nil {
			t.Fatalf("accepted invalid report: %+v", bad.ID)
		}
	}
	if err := out.Save(r); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(out.Dir, "junk.json"), []byte("broken"), 0600)
	id, _ := NewID()
	os.WriteFile(filepath.Join(out.Dir, id+".json"), []byte("broken"), 0600)
	reports, err := out.List()
	if err != nil || len(reports) != 1 {
		t.Fatalf("list: %v %v", reports, err)
	}
	meta := Metadata(context.Background(), "1.2.3", "en", "900x700")
	for _, key := range []string{"application", "application_version", "os", "os_version", "architecture"} {
		if meta[key] == nil || meta[key] == "" {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestScreenshotRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.Pix[0] = 255
	img.Pix[3] = 255
	encoded, err := EncodeScreenshot(img)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(encoded)
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	r := testReport(t)
	r.Screenshot = encoded
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeImage(bytes.NewReader(data))
	if err != nil || decoded.Bounds() != img.Bounds() {
		t.Fatalf("decode: %v", err)
	}
}

func TestConnectionFailureDoesNotClaimReportSaved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer srv.Close()
	c := &Client{URL: srv.URL, Key: "test"}
	_, err := c.Test(context.Background())
	if err == nil || strings.Contains(err.Error(), "saved") {
		t.Fatalf("connectivity check claimed it saved a report: %v", err)
	}
}
