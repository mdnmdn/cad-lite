package agentinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cad-lite/updater"
)

// ChecksumFile is the checksum list every mise release carries.
const ChecksumFile = "SHASUMS256.txt"

// maxMiseDownload bounds the mise archive; the real ones are 45 to 70 MB.
const maxMiseDownload = 300 << 20

var (
	// ErrUnsupported is returned for a platform mise publishes no build for.
	ErrUnsupported = errors.New("agentinstall: mise has no build for this platform")
	// ErrChecksum is returned when the downloaded archive does not match the release's list.
	ErrChecksum = errors.New("agentinstall: checksum mismatch")
	// ErrNoChecksum is returned when the release lists no checksum for the archive.
	ErrNoChecksum = errors.New("agentinstall: no checksum for the archive")
)

// AssetSuffix is the end of the mise archive name for goos/goarch, after `mise-v<version>`:
// `-linux-x64-musl.tar.gz`, `-macos-arm64.tar.gz`, `-windows-x64.zip`. Linux takes the static
// musl build, which runs on any distribution.
func AssetSuffix(goos, goarch string) (string, bool) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	switch goos {
	case "linux":
		if goarch == "arm" {
			arch = "armv7"
		}
		if arch == "" {
			return "", false
		}
		return "-linux-" + arch + "-musl.tar.gz", true
	case "darwin":
		if arch == "" {
			return "", false
		}
		return "-macos-" + arch + ".tar.gz", true
	case "windows":
		if arch == "" {
			return "", false
		}
		return "-windows-" + arch + ".zip", true
	}
	return "", false
}

// pickAsset finds the archive for suffix in a checksum list and returns its name, version and
// sum. With several versions listed (never the case in a real release) the highest name wins.
func pickAsset(sums map[string]string, suffix string) (name, version, sum string, ok bool) {
	var names []string
	for n := range sums {
		b := strings.TrimPrefix(n, "./")
		if strings.HasPrefix(b, "mise-v") && strings.HasSuffix(b, suffix) && !strings.ContainsAny(b, "/\\") {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "", "", "", false
	}
	sort.Strings(names)
	n := names[len(names)-1]
	name = strings.TrimPrefix(n, "./")
	version = strings.TrimSuffix(strings.TrimPrefix(name, "mise-v"), suffix)
	if version == "" || strings.ContainsAny(version, "/\\?#") {
		return "", "", "", false
	}
	return name, version, sums[n], true
}

// EnsureMise returns the private mise executable, downloading it first when Dir has none that
// runs. The archive is checked against the release's SHA256SUMS before anything is unpacked; a
// mismatch, a missing entry or a cancelled ctx leaves no file behind.
func (in *Installer) EnsureMise(ctx context.Context) (string, error) {
	target := in.misePath()
	if in.miseRuns(ctx, target) {
		return target, nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	suffix, ok := AssetSuffix(in.goos(), in.goarch())
	if !ok {
		return "", fmt.Errorf("%w (%s/%s)", ErrUnsupported, in.goos(), in.goarch())
	}
	root := filepath.Join(in.Dir, "mise")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	sums, err := in.checksums(ctx)
	if err != nil {
		return "", err
	}
	name, version, want, ok := pickAsset(sums, suffix)
	if !ok {
		return "", fmt.Errorf("%w: %s lists no mise-v*%s", ErrNoChecksum, ChecksumFile, suffix)
	}
	tmp, err := os.MkdirTemp(root, ".download-")
	if err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, name)
	if err := in.download(ctx, in.base()+"/download/v"+version+"/"+name, archive, want); err != nil {
		return "", err
	}
	unpacked := filepath.Join(tmp, "unpacked")
	if err := os.Mkdir(unpacked, 0o700); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	if err := updater.Extract(archive, unpacked); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	exe, ok := findFile(unpacked, filepath.Base(target))
	if !ok {
		return "", fmt.Errorf("agentinstall: %s has no %s", name, filepath.Base(target))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	_ = os.Remove(target)
	if err := os.Rename(exe, target); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	return target, nil
}

// miseRuns reports whether path is a mise that answers --version.
func (in *Installer) miseRuns(ctx context.Context, path string) bool {
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return false
	}
	vctx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	_, err := in.output(vctx, path, in.miseEnv(), "--version")
	return err == nil
}

func (in *Installer) base() string {
	if in.BaseURL != "" {
		return strings.TrimRight(in.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (in *Installer) client() *http.Client {
	if in.HTTP != nil {
		return in.HTTP
	}
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("agentinstall: too many redirects")
		}
		return checkURL(req.URL)
	}}
}

func (in *Installer) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("agentinstall: %w", err)
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cad-lite-agentinstall")
	resp, err := in.client().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("agentinstall: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("agentinstall: downloading %s: %s", filepath.Base(req.URL.Path), resp.Status)
	}
	return resp, nil
}

// checksums reads the latest release's checksum list.
func (in *Installer) checksums(ctx context.Context) (map[string]string, error) {
	resp, err := in.get(ctx, in.base()+"/latest/download/"+ChecksumFile)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	sums, err := updater.ParseChecksums(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("agentinstall: %s: %w", ChecksumFile, err)
	}
	return sums, nil
}

// download fetches rawURL into path, reporting progress, and keeps the file only when its
// sha256 is want.
func (in *Installer) download(ctx context.Context, rawURL, path, want string) error {
	resp, err := in.get(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("agentinstall: %w", err)
	}
	total := max(resp.ContentLength, 0)
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	body := io.LimitReader(resp.Body, maxMiseDownload+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				err = werr
				break
			}
			h.Write(buf[:n])
			done += int64(n)
			in.report(Progress{Step: StepMise, Done: done, Total: total})
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			err = rerr
			break
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && done > maxMiseDownload {
		err = fmt.Errorf("larger than %d MB", maxMiseDownload>>20)
	}
	if err != nil {
		os.Remove(path)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("agentinstall: downloading %s: %w", filepath.Base(path), err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(want) {
		os.Remove(path)
		return fmt.Errorf("%w: %s has sha256 %s, the release lists %s", ErrChecksum, filepath.Base(path), got, want)
	}
	return nil
}

// findFile returns the first regular file under root whose base name is name.
func findFile(root, name string) (string, bool) {
	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && found == "" && d.Type().IsRegular() && d.Name() == name {
			found = p
		}
		return nil
	})
	return found, found != ""
}
