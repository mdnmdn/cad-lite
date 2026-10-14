// Package agentinstall installs an AI agent CLI for Cad-lite without touching the system: it
// downloads mise (github.com/jdx/mise) into Cad-lite's private tools folder, verifies it against
// the release's SHA256 list, has that mise install OpenCode inside the same folder, and asks
// OpenCode whether it has credentials. Every mise directory (data, config, cache, state) and the
// working directory live in the tools folder, so nothing is written to the user's own mise or
// shell configuration.
package agentinstall

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cad-lite/appenv"
)

// EnvDir names the environment variable (CADLITE_TOOLS_DIR) that replaces the tools folder, for
// tests and portable setups.
const EnvDir = "TOOLS_DIR"

// DefaultTool is the mise tool spec installed for OpenCode. mise's registry maps `opencode` to
// its npm package and, failing that, to the release binaries (aqua:anomalyco/opencode).
const DefaultTool = "opencode@latest"

// DefaultBaseURL is mise's GitHub releases page; `/latest/download/<file>` and
// `/download/<tag>/<file>` are appended to it.
const DefaultBaseURL = "https://github.com/jdx/mise/releases"

// Binary is the executable name mise installs for OpenCode.
const Binary = "opencode"

// Timeouts of the steps that run programs.
const (
	// ToolTimeout bounds `mise install`, which downloads OpenCode.
	ToolTimeout = 20 * time.Minute
	// AuthTimeout bounds `opencode auth list`; its first run may migrate a database.
	AuthTimeout = 30 * time.Second
	// shortTimeout bounds quick queries (`mise --version`, `mise which`).
	shortTimeout = 30 * time.Second
)

// Dir returns the private tools folder: $CADLITE_TOOLS_DIR when set, else `cad-lite/tools` in
// the per-user data folder (%LocalAppData% on Windows, ~/Library/Application Support on macOS,
// $XDG_DATA_HOME or ~/.local/share elsewhere). It does not create the folder.
func Dir() (string, error) {
	if d := appenv.Get(EnvDir); d != "" {
		return d, nil
	}
	return defaultDir(runtime.GOOS, os.Getenv, os.UserHomeDir)
}

func defaultDir(goos string, env func(string) string, home func() (string, error)) (string, error) {
	var base string
	switch goos {
	case "js":
		return "", errors.New("agentinstall: no tools folder in the browser")
	case "windows":
		base = env("LocalAppData")
	case "darwin":
		if h, err := home(); err == nil {
			base = filepath.Join(h, "Library", "Application Support")
		}
	default:
		base = env("XDG_DATA_HOME")
		if base == "" || !filepath.IsAbs(base) {
			base = ""
			if h, err := home(); err == nil {
				base = filepath.Join(h, ".local", "share")
			}
		}
	}
	if base == "" {
		return "", errors.New("agentinstall: no user data folder")
	}
	return filepath.Join(base, "cad-lite", "tools"), nil
}

// Step is the part of an installation that is running.
type Step int

// The steps, in the order Install runs them.
const (
	StepMise Step = iota // downloading and verifying mise
	StepTool             // mise installs OpenCode
	StepAuth             // asking OpenCode whether it has credentials
)

// Progress reports where an installation is: the step and, while mise downloads, the bytes
// received and the total (0 when unknown).
type Progress struct {
	Step        Step
	Done, Total int64
}

// Result is a finished installation.
type Result struct {
	// Mise is the mise executable in the tools folder.
	Mise string
	// Binary is OpenCode's executable, as `mise which` reports it.
	Binary string
	// Authenticated reports whether `opencode auth list` listed at least one credential.
	Authenticated bool
	// AuthErr is why the credential check could not run; Authenticated is false then.
	AuthErr error
}

// Installer installs OpenCode through a private mise. Set Dir; every other field has a default.
// An Installer runs one installation at a time.
type Installer struct {
	// Dir is the private tools folder (see Dir); it is created as needed.
	Dir string
	// BaseURL is mise's releases page (DefaultBaseURL); tests use a local server.
	BaseURL string
	// HTTP fetches the release files; nil uses a client that follows only https redirects.
	HTTP *http.Client
	// GOOS and GOARCH pick the mise build; they default to the running platform.
	GOOS, GOARCH string
	// Tool is the mise tool spec (DefaultTool).
	Tool string
	// Env is added to the environment of every program run, after the process's own (later
	// entries win); hosts pass the PATH the user's login shell reports, so mise finds node.
	Env []string
	// Progress, when set, is called from the installing goroutine.
	Progress func(Progress)
}

// Install makes OpenCode available: it reuses the mise already in Dir or downloads it, runs
// `mise install <Tool>`, finds the executable with `mise which` and checks the credentials. A
// cancelled ctx stops the running step and returns ctx's error; nothing half-downloaded is
// kept.
func (in *Installer) Install(ctx context.Context) (Result, error) {
	if in.Dir == "" {
		return Result{}, errors.New("agentinstall: no tools folder")
	}
	if !filepath.IsAbs(in.Dir) {
		return Result{}, fmt.Errorf("agentinstall: tools folder %q is not absolute", in.Dir)
	}
	in.report(Progress{Step: StepMise})
	mise, err := in.EnsureMise(ctx)
	if err != nil {
		return Result{}, err
	}
	in.report(Progress{Step: StepTool})
	bin, err := in.InstallTool(ctx, mise)
	if err != nil {
		return Result{}, err
	}
	in.report(Progress{Step: StepAuth})
	r := Result{Mise: mise, Binary: bin}
	r.Authenticated, r.AuthErr = in.CheckAuth(ctx, bin)
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	return r, nil
}

func (in *Installer) report(p Progress) {
	if in.Progress != nil {
		in.Progress(p)
	}
}

func (in *Installer) goos() string {
	if in.GOOS != "" {
		return in.GOOS
	}
	return runtime.GOOS
}

func (in *Installer) goarch() string {
	if in.GOARCH != "" {
		return in.GOARCH
	}
	return runtime.GOARCH
}

func (in *Installer) tool() string {
	if in.Tool != "" {
		return in.Tool
	}
	return DefaultTool
}

// toolName is the tool spec without its version: what `mise which --tool` and the
// executable's name are about.
func (in *Installer) toolName() string {
	name, _, _ := strings.Cut(in.tool(), "@")
	return name
}

// misePath is where the private mise executable lives.
func (in *Installer) misePath() string {
	name := "mise"
	if in.goos() == "windows" {
		name += ".exe"
	}
	return filepath.Join(in.Dir, "mise", "bin", name)
}

// workDir is the working directory of every program run: an empty folder inside Dir.
func (in *Installer) workDir() string { return filepath.Join(in.Dir, "work") }

// miseEnv is the environment that confines mise to Dir: its four directories, a global config
// file of its own, no walking above the tools folder for project files, and no prompts.
func (in *Installer) miseEnv() []string {
	root := filepath.Join(in.Dir, "mise")
	return []string{
		"MISE_DATA_DIR=" + filepath.Join(root, "data"),
		"MISE_CONFIG_DIR=" + filepath.Join(root, "config"),
		"MISE_CACHE_DIR=" + filepath.Join(root, "cache"),
		"MISE_STATE_DIR=" + filepath.Join(root, "state"),
		"MISE_GLOBAL_CONFIG_FILE=" + filepath.Join(root, "config", "config.toml"),
		"MISE_SYSTEM_CONFIG_FILE=" + filepath.Join(root, "config", "system.toml"),
		"MISE_CEILING_PATHS=" + in.Dir,
		"MISE_YES=1",
		"MISE_COLOR=0",
		"NO_COLOR=1",
		"npm_config_cache=" + filepath.Join(root, "cache", "npm"),
		"npm_config_update_notifier=false",
	}
}

// command builds a program run in the work folder with the installer's environment.
func (in *Installer) command(ctx context.Context, bin string, env []string, args ...string) (*exec.Cmd, error) {
	if err := os.MkdirAll(in.workDir(), 0o700); err != nil {
		return nil, fmt.Errorf("agentinstall: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = in.workDir()
	cmd.Env = append(append(os.Environ(), in.Env...), env...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdin = nil
	hideWindow(cmd)
	return cmd, nil
}

// output runs a program and returns what it printed on stdout; a failure carries the last lines
// of its stderr and stdout.
func (in *Installer) output(ctx context.Context, bin string, env []string, args ...string) (string, error) {
	cmd, err := in.command(ctx, bin, env, args...)
	if err != nil {
		return "", err
	}
	var out, errOut tailBuffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		msg := strings.TrimSpace(errOut.String() + "\n" + out.String())
		if msg == "" {
			return "", fmt.Errorf("agentinstall: %s %s: %w", filepath.Base(bin), strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("agentinstall: %s %s: %w: %s", filepath.Base(bin), strings.Join(args, " "), err, lastLines(msg, 4))
	}
	return out.String(), nil
}

// InstallTool runs `mise install <Tool>` and returns the executable `mise which` reports.
func (in *Installer) InstallTool(ctx context.Context, mise string) (string, error) {
	ictx, cancel := context.WithTimeout(ctx, ToolTimeout)
	defer cancel()
	if _, err := in.output(ictx, mise, in.miseEnv(), "install", in.tool()); err != nil {
		return "", err
	}
	wctx, wcancel := context.WithTimeout(ctx, shortTimeout)
	defer wcancel()
	out, err := in.output(wctx, mise, in.miseEnv(), "which", "--tool", in.tool(), in.toolName())
	if err != nil {
		return "", err
	}
	bin := lastLine(out)
	if bin == "" || !filepath.IsAbs(bin) {
		return "", fmt.Errorf("agentinstall: mise did not report where %s is (%q)", in.toolName(), bin)
	}
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		return "", fmt.Errorf("agentinstall: mise reported %s, which is not a file", bin)
	}
	in.pruneCache(ctx, mise)
	return bin, nil
}

// pruneCache drops mise's download cache, which holds a second copy of the package; failures are
// ignored because the installation is complete without it.
func (in *Installer) pruneCache(ctx context.Context, mise string) {
	cctx, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	_, _ = in.output(cctx, mise, in.miseEnv(), "cache", "clear")
}

// CheckAuth runs `<bin> auth list` and reports whether it lists a credential (see
// HasCredentials). err is set when the command could not run or failed.
func (in *Installer) CheckAuth(ctx context.Context, bin string) (bool, error) {
	actx, cancel := context.WithTimeout(ctx, AuthTimeout)
	defer cancel()
	out, err := in.output(actx, bin, []string{"NO_COLOR=1"}, "auth", "list")
	if err != nil {
		return false, err
	}
	return HasCredentials(out), nil
}

// LoginArgs is the command line that signs in to OpenCode at bin, for a terminal.
func LoginArgs(bin string) []string { return []string{bin, "auth", "login"} }

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
	return fmt.Errorf("agentinstall: refusing non-https URL %s", u.Redacted())
}

// tailBuffer keeps the last 64 KiB written to it.
type tailBuffer struct{ b []byte }

const tailMax = 64 << 10

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > tailMax {
		t.b = append(t.b[:0], t.b[len(t.b)-tailMax:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.b) }

// lastLine is the last non-empty line of s, trimmed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// lastLines is the last n non-empty lines of s, joined with " / ".
func lastLines(s string, n int) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(stripANSI(l)); l != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, " / ")
}
