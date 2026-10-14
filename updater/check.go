// Package updater finds, downloads, verifies and installs Cad-lite releases
// published on GitHub, and restarts the application into the new version.
//
// The package is stdlib only and knows nothing about the UI. Client.Check asks
// the GitHub releases API for the latest release (conditional requests with
// ETag keep it far below the rate limit); Client.Update downloads the asset
// for this platform, verifies it against the release's checksums.txt and
// replaces the running installation in place; Relaunch then starts the new
// process. Asset names follow packaging/README.md. On js every operation
// returns ErrUnsupported.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"cad-lite/appenv"
)

// Defaults of a Client.
const (
	// DefaultBaseURL is the GitHub REST API root.
	DefaultBaseURL = "https://api.github.com"
	// DefaultRepo is the repository releases are read from. It is still the
	// repository's name from before the rename to Cad-lite: change it when the
	// GitHub repository is renamed.
	DefaultRepo = "mdnmdn/go-draw"
	// EnvDisable names the environment variable (read through appenv, so the
	// legacy GODRAW_NO_UPDATE works too) that turns updates off when it is "1".
	EnvDisable = "NO_UPDATE"
)

// Errors returned by the package, to be tested with errors.Is.
var (
	// ErrUnsupported means this platform (the browser) cannot update.
	ErrUnsupported = errors.New("updater: not supported on this platform")
	// ErrNoAsset means the release has no archive for this OS and architecture.
	ErrNoAsset = errors.New("updater: release has no download for this platform")
	// ErrNoChecksum means the release has no checksums file, or none for the asset.
	ErrNoChecksum = errors.New("updater: no checksum for the download")
	// ErrChecksum means the downloaded file does not match its checksum.
	ErrChecksum = errors.New("updater: download does not match its checksum")
	// ErrNotWritable means the installation folder cannot be written to.
	ErrNotWritable = errors.New("updater: installation folder is not writable")
)

// Disabled reports whether the environment turns updates off
// (CADLITE_NO_UPDATE=1, or the legacy GODRAW_NO_UPDATE=1).
func Disabled() bool { return appenv.Get(EnvDisable) == "1" }

// Asset is a file attached to a release.
type Asset struct {
	// Name is the file name, e.g. cad-lite-1.2.3-windows-amd64.zip.
	Name string
	// URL is the public download URL.
	URL string
	// Size is the size in bytes as GitHub reports it.
	Size int64
}

// Release is a published GitHub release.
type Release struct {
	// Version is the tag without its leading "v".
	Version string
	// Tag is the git tag, e.g. v1.2.3.
	Tag string
	// Name is the release title.
	Name string
	// Notes is the release body (Markdown).
	Notes string
	// PageURL is the release's web page, for "open the release page".
	PageURL string
	// Published is when the release was published; zero when unknown.
	Published time.Time
	// Assets are the attached files.
	Assets []Asset
}

// IsZero reports whether r is the empty release (nothing published).
func (r Release) IsZero() bool { return r.Tag == "" }

// Client talks to the GitHub releases API. The zero value is not usable; call
// NewClient and adjust fields before the first call. A Client is safe for
// concurrent use.
type Client struct {
	// BaseURL is the API root; tests point it at an httptest server.
	BaseURL string
	// Repo is "owner/name".
	Repo string
	// HTTP is the client for every request; its redirects are limited to https.
	HTTP *http.Client
	// UserAgent is sent with every request (GitHub requires one).
	UserAgent string
	// GOOS and GOARCH select the asset; they default to the running platform.
	GOOS, GOARCH string
	// Exe overrides the installation Update replaces (tests); empty means
	// the running executable.
	Exe string

	mu     sync.Mutex
	etag   string
	cached Release
}

// NewClient returns a Client for the Cad-lite repository on github.com, with
// the User-Agent "cad-lite-updater/<version>".
func NewClient(version string) *Client {
	c := &Client{
		BaseURL:   DefaultBaseURL,
		Repo:      DefaultRepo,
		UserAgent: "cad-lite-updater/" + version,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
	c.HTTP = &http.Client{
		Timeout: 0, // per-request deadlines come from the context
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("updater: too many redirects")
			}
			return checkURL(req.URL)
		},
	}
	return c
}

// checkURL allows https, and plain http only to a loopback host (tests).
func checkURL(u *url.URL) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return nil
		}
	}
	return fmt.Errorf("updater: refusing non-https URL %s", u.Redacted())
}

type apiRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// Check asks GitHub for the latest release and reports whether it is newer
// than current and installable here. It returns the latest release whenever
// one exists, even when it is not newer (so the UI can show "latest: 1.2.0"),
// and available true only when current is an updatable version (see
// Updatable: not "dev", not a CI build), the release is newer, and it has an
// asset for this platform; a newer release without one returns ErrNoAsset
// with the release filled in. Drafts and pre-releases are skipped. A release
// the server has not changed (HTTP 304 for the remembered ETag) is answered
// from memory; use CheckForce to bypass that.
func (c *Client) Check(ctx context.Context, current string) (Release, bool, error) {
	return c.check(ctx, current, false)
}

// CheckForce is Check without the ETag shortcut: it always fetches the body.
func (c *Client) CheckForce(ctx context.Context, current string) (Release, bool, error) {
	return c.check(ctx, current, true)
}

func (c *Client) check(ctx context.Context, current string, force bool) (Release, bool, error) {
	if !Supported() {
		return Release{}, false, ErrUnsupported
	}
	rel, err := c.latest(ctx, force)
	if err != nil || rel.IsZero() {
		return rel, false, err
	}
	if !Updatable(current) {
		return rel, false, nil
	}
	cur, _ := ParseVersion(current)
	latest, err := ParseVersion(rel.Version)
	if err != nil {
		return rel, false, err
	}
	if latest.Compare(cur) <= 0 {
		return rel, false, nil
	}
	if _, ok := rel.AssetFor(c.GOOS, c.GOARCH); !ok {
		return rel, false, fmt.Errorf("%w (%s/%s) in %s", ErrNoAsset, c.GOOS, c.GOARCH, rel.Tag)
	}
	return rel, true, nil
}

func (c *Client) latest(ctx context.Context, force bool) (Release, error) {
	u := strings.TrimRight(c.BaseURL, "/") + "/repos/" + c.Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Release{}, err
	}
	if err := checkURL(req.URL); err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	c.mu.Lock()
	if !force && c.etag != "" {
		req.Header.Set("If-None-Match", c.etag)
	}
	c.mu.Unlock()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updater: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.cached, nil
	case http.StatusNotFound:
		return Release{}, nil // no release published yet
	case http.StatusOK:
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Release{}, fmt.Errorf("updater: GitHub refused the request (%s), probably the rate limit; try again later", resp.Status)
	default:
		return Release{}, fmt.Errorf("updater: GitHub answered %s", resp.Status)
	}
	var a apiRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&a); err != nil {
		return Release{}, fmt.Errorf("updater: decoding release: %w", err)
	}
	if a.Draft || a.Prerelease || a.TagName == "" {
		return Release{}, nil
	}
	rel := Release{
		Version: strings.TrimPrefix(a.TagName, "v"), Tag: a.TagName, Name: a.Name,
		Notes: a.Body, PageURL: a.HTMLURL, Published: a.PublishedAt,
	}
	for _, x := range a.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: x.Name, URL: x.URL, Size: x.Size})
	}
	c.mu.Lock()
	c.etag, c.cached = resp.Header.Get("ETag"), rel
	c.mu.Unlock()
	return rel, nil
}
