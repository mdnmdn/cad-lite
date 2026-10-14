package agentinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The mise tool specs InstallTool tries, in order.
const (
	// BinarySpec is OpenCode's release binary through mise's aqua backend. mise looks the release
	// up with the GitHub API (unauthenticated: 60 requests an hour per address).
	BinarySpec = "aqua:anomalyco/opencode@latest"
	// NpmSpec is mise's registry entry for OpenCode, which installs its npm package when Node.js
	// is there: the last resort, needing node at run time and about 1 GB of disk.
	NpmSpec = "opencode@latest"
)

// DefaultOpenCodeURL is OpenCode's releases page (github.com/sst/opencode redirects there).
const DefaultOpenCodeURL = "https://github.com/anomalyco/opencode/releases"

var (
	// ErrNoNode is part of the error when no release binary could be installed and the npm
	// fallback is not possible because Node.js is not installed.
	ErrNoNode = errors.New("Node.js is not installed, so OpenCode's npm package cannot be used either")
	// ErrRateLimited is part of the error when GitHub refused a release lookup.
	ErrRateLimited = errors.New("GitHub refused the release lookup (rate limit or access); try again later")
)

// ReleaseAsset is the name of OpenCode's release archive for goos/goarch:
// `opencode-linux-x64.tar.gz`, `opencode-darwin-arm64.zip`, `opencode-windows-x64.zip`.
func ReleaseAsset(goos, goarch string) (string, bool) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" {
		return "", false
	}
	switch goos {
	case "linux":
		return "opencode-linux-" + arch + ".tar.gz", true
	case "darwin":
		return "opencode-darwin-" + arch + ".zip", true
	case "windows":
		return "opencode-windows-" + arch + ".zip", true
	}
	return "", false
}

// InstallTool installs OpenCode with mise and returns the executable `mise which` reports. With
// Tool set it installs only that spec. Otherwise it tries, in order: the release binary through
// the aqua backend (BinarySpec); the same binary through mise's http backend from a direct
// download URL, whose version comes from the `latest/download` redirect, so no GitHub API call is
// made; and, only when Node.js is on the PATH, the npm package (NpmSpec). The error says why each
// failed. What a failed or cancelled attempt left in mise's install folder is removed.
func (in *Installer) InstallTool(ctx context.Context, mise string) (string, error) {
	if in.Tool != "" {
		return in.installSpec(ctx, mise, in.Tool)
	}
	var errs []error
	bin, err := in.installSpec(ctx, mise, BinarySpec)
	if err == nil || ctx.Err() != nil {
		return bin, err
	}
	errs = append(errs, fmt.Errorf("release binary (mise aqua): %w", rateLimited(err)))
	spec, err := in.directSpec(ctx)
	if err == nil {
		bin, err = in.installSpec(ctx, mise, spec)
		if err == nil || ctx.Err() != nil {
			return bin, err
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	errs = append(errs, fmt.Errorf("release binary (direct download): %w", rateLimited(err)))
	if !in.hasNode() {
		errs = append(errs, ErrNoNode)
		return "", fmt.Errorf("agentinstall: OpenCode could not be installed: %w", errors.Join(errs...))
	}
	bin, err = in.installSpec(ctx, mise, NpmSpec)
	if err == nil || ctx.Err() != nil {
		return bin, err
	}
	errs = append(errs, fmt.Errorf("npm package: %w", err))
	return "", fmt.Errorf("agentinstall: OpenCode could not be installed: %w", errors.Join(errs...))
}

// rateLimited marks an error whose text shows that GitHub refused a request.
func rateLimited(err error) error {
	if err == nil {
		return nil
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "rate limit") || strings.Contains(low, "403") || strings.Contains(low, "429") {
		return fmt.Errorf("%w: %w", ErrRateLimited, err)
	}
	return err
}

// installSpec runs `mise install <spec>` and `mise which --tool <spec> opencode`, then drops
// mise's download cache. A failure or cancellation removes the install folders the attempt
// created.
func (in *Installer) installSpec(ctx context.Context, mise, spec string) (string, error) {
	before := in.installs()
	ictx, cancel := context.WithTimeout(ctx, ToolTimeout)
	defer cancel()
	if _, err := in.output(ictx, mise, in.miseEnv(), "install", spec); err != nil {
		in.removeNewInstalls(before)
		if ctx.Err() != nil {
			os.RemoveAll(filepath.Join(in.Dir, "mise", "cache"))
			return "", ctx.Err()
		}
		return "", err
	}
	wctx, wcancel := context.WithTimeout(ctx, shortTimeout)
	defer wcancel()
	out, err := in.output(wctx, mise, in.miseEnv(), "which", "--tool", spec, Binary)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	bin := lastLine(out)
	if bin == "" || !filepath.IsAbs(bin) {
		return "", fmt.Errorf("agentinstall: mise did not report where %s is (%q)", Binary, bin)
	}
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		return "", fmt.Errorf("agentinstall: mise reported %s, which is not a file", bin)
	}
	in.pruneCache(ctx, mise)
	return bin, nil
}

// installs lists mise's install folders, `<tool>/<version>`, as they are now.
func (in *Installer) installs() map[string]bool {
	root := filepath.Join(in.Dir, "mise", "data", "installs")
	out := map[string]bool{}
	tools, _ := os.ReadDir(root)
	for _, t := range tools {
		out[t.Name()] = true
		vers, _ := os.ReadDir(filepath.Join(root, t.Name()))
		for _, v := range vers {
			out[filepath.Join(t.Name(), v.Name())] = true
		}
	}
	return out
}

// removeNewInstalls deletes the install folders that are not in before: what an interrupted or
// failed `mise install` left half-written.
func (in *Installer) removeNewInstalls(before map[string]bool) {
	root := filepath.Join(in.Dir, "mise", "data", "installs")
	for p := range in.installs() {
		if before[p] {
			continue
		}
		if tool := filepath.Dir(p); tool != "." && !before[tool] {
			continue // the whole tool folder is new and goes below
		}
		os.RemoveAll(filepath.Join(root, p))
	}
}

// pruneCache drops mise's download cache, which holds a second copy of the package; failures are
// ignored because the installation is complete without it.
func (in *Installer) pruneCache(ctx context.Context, mise string) {
	cctx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	_, _ = in.output(cctx, mise, in.miseEnv(), "cache", "clear")
}

// directSpec is the mise http-backend spec for the latest OpenCode release archive of this
// platform, e.g. `http:opencode[url=…/download/v{{version}}/opencode-linux-x64.tar.gz]@1.19.0`.
func (in *Installer) directSpec(ctx context.Context) (string, error) {
	asset, ok := ReleaseAsset(in.goos(), in.goarch())
	if !ok {
		return "", fmt.Errorf("agentinstall: OpenCode publishes no binary for %s/%s", in.goos(), in.goarch())
	}
	ver, err := in.latestOpenCode(ctx, asset)
	if err != nil {
		return "", err
	}
	return "http:" + Binary + "[url=" + in.openCodeBase() + "/download/v{{version}}/" + asset + "]@" + ver, nil
}

func (in *Installer) openCodeBase() string {
	if in.OpenCodeURL != "" {
		return strings.TrimRight(in.OpenCodeURL, "/")
	}
	return DefaultOpenCodeURL
}

// latestOpenCode reads the latest release's version from where `latest/download/<asset>`
// redirects (`…/download/v1.19.0/<asset>`), without the GitHub API.
func (in *Installer) latestOpenCode(ctx context.Context, asset string) (string, error) {
	var target string
	c := *in.client()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkURL(req.URL); err != nil {
			return err
		}
		if len(via) >= 10 {
			return errors.New("agentinstall: too many redirects")
		}
		if d := path.Dir(req.URL.Path); path.Base(req.URL.Path) == asset && path.Base(path.Dir(d)) == "download" {
			target = path.Base(d)
			return http.ErrUseLastResponse
		}
		return nil
	}
	hctx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodHead, in.openCodeBase()+"/latest/download/"+asset, nil)
	if err != nil {
		return "", fmt.Errorf("agentinstall: %w", err)
	}
	if err := checkURL(req.URL); err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "cad-lite-agentinstall")
	resp, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("agentinstall: finding the latest OpenCode: %w", err)
	}
	resp.Body.Close()
	ver := strings.TrimPrefix(target, "v")
	if ver == "" || strings.ContainsAny(ver, "[]@{} ") {
		return "", fmt.Errorf("agentinstall: finding the latest OpenCode: %s did not redirect to a release (%s)", asset, resp.Status)
	}
	return ver, nil
}

// hasNode reports whether the npm fallback can run.
func (in *Installer) hasNode() bool {
	if in.HasNode != nil {
		return in.HasNode()
	}
	p := os.Getenv("PATH")
	for _, e := range in.Env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			p = v
		}
	}
	names := []string{"node"}
	if in.goos() == "windows" {
		names = []string{"node.exe", "node.cmd"}
	}
	for _, d := range filepath.SplitList(p) {
		for _, n := range names {
			if fi, err := os.Stat(filepath.Join(d, n)); err == nil && !fi.IsDir() {
				return true
			}
		}
	}
	return false
}

// CheckAuth runs `<bin> auth list` and reports whether it lists a credential (see
// HasCredentials); err is set when the command could not run or failed. OpenCode 2.x starts a
// detached background service on that call and records it in `<StateDir>/service.json`; when
// that file appears only during the call, the service is ours and is stopped again (`opencode
// service stop`, else the recorded process is killed). A service that was already running is
// left alone.
func (in *Installer) CheckAuth(ctx context.Context, bin string) (bool, error) {
	svc := ""
	had := true
	if d := in.stateDir(); d != "" {
		svc = filepath.Join(d, "service.json")
		_, statErr := os.Stat(svc)
		had = statErr == nil
	}
	actx, cancel := context.WithTimeout(ctx, AuthTimeout)
	defer cancel()
	out, err := in.output(actx, bin, []string{"NO_COLOR=1"}, "auth", "list")
	if !had {
		in.stopService(bin, svc)
	}
	if err != nil {
		return false, err
	}
	return HasCredentials(out), nil
}

// stopService stops the OpenCode service recorded in svc, if there is one.
func (in *Installer) stopService(bin, svc string) {
	if _, err := os.Stat(svc); err != nil {
		return
	}
	pid := servicePID(svc)
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = in.output(sctx, bin, []string{"NO_COLOR=1"}, "service", "stop")
	if _, err := os.Stat(svc); err != nil || pid <= 0 {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
	os.Remove(svc)
}

// servicePID is the process id service.json records, or 0.
func servicePID(svc string) int {
	b, err := os.ReadFile(svc)
	if err != nil {
		return 0
	}
	var s struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(b, &s) != nil {
		return 0
	}
	return s.PID
}

func (in *Installer) stateDir() string {
	if in.StateDir != "" {
		return in.StateDir
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" && filepath.IsAbs(d) {
		return filepath.Join(d, "opencode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "opencode")
}
