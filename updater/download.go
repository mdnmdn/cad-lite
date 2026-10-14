package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// maxDownload bounds an archive; the real ones are a few tens of MB.
const maxDownload = 500 << 20

// Progress reports bytes received so far and the total (0 when unknown).
type Progress func(done, total int64)

func (c *Client) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("updater: downloading %s: %s", filepath.Base(req.URL.Path), resp.Status)
	}
	return resp, nil
}

// Download fetches the release's archive for this platform into dir, checks
// its sha256 against the release's checksum list and returns the file's path.
// A missing checksum list, a missing entry or a mismatch (ErrNoChecksum,
// ErrChecksum) deletes the file and returns an error: nothing unverified is
// ever handed on. progress may be nil.
func (c *Client) Download(ctx context.Context, rel Release, dir string, progress Progress) (string, error) {
	if !Supported() {
		return "", ErrUnsupported
	}
	asset, ok := rel.AssetFor(c.GOOS, c.GOARCH)
	if !ok {
		return "", fmt.Errorf("%w (%s/%s) in %s", ErrNoAsset, c.GOOS, c.GOARCH, rel.Tag)
	}
	want, err := c.expectedSum(ctx, rel, asset.Name)
	if err != nil {
		return "", err
	}
	resp, err := c.get(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	total := resp.ContentLength
	if total <= 0 {
		total = asset.Size
	}
	path := filepath.Join(dir, asset.Name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), &countReader{r: io.LimitReader(resp.Body, maxDownload+1), total: total, fn: progress})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDownload {
		err = fmt.Errorf("updater: %s is larger than %d MB", asset.Name, maxDownload>>20)
	}
	if err != nil {
		os.Remove(path)
		return "", fmt.Errorf("updater: downloading %s: %w", asset.Name, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(path)
		return "", fmt.Errorf("%w: %s has sha256 %s, the release lists %s", ErrChecksum, asset.Name, got, want)
	}
	return path, nil
}

func (c *Client) expectedSum(ctx context.Context, rel Release, name string) (string, error) {
	ca, ok := rel.ChecksumAsset()
	if !ok {
		return "", fmt.Errorf("%w: %s has no SHA256SUMS.txt", ErrNoChecksum, rel.Tag)
	}
	resp, err := c.get(ctx, ca.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sums, err := ParseChecksums(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	s, ok := sums[name]
	if !ok {
		return "", fmt.Errorf("%w: %s is not listed in %s", ErrNoChecksum, name, ca.Name)
	}
	return s, nil
}

type countReader struct {
	r           io.Reader
	done, total int64
	fn          Progress
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	if c.fn != nil {
		c.fn(c.done, c.total)
	}
	return n, err
}
