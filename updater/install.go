package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const macBundleMarker = ".app/Contents/MacOS/"

// Plan says how to start the installed version: Install fills Exe and App,
// Client.Update adds Args and Version.
type Plan struct {
	// GOOS is the platform the plan was made for.
	GOOS string
	// Exe is the resolved path of the executable that was replaced.
	Exe string
	// App is the .app bundle that was replaced on macOS, else empty.
	App string
	// Args are the arguments the running process was started with.
	Args []string
	// Version is the version installed.
	Version string
}

// Executable returns the running executable's path with symlinks resolved,
// which is the file Install replaces.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Abs(exe)
}

// Update downloads the release's archive for this platform, verifies it,
// installs it over the running installation and returns the plan Relaunch
// needs. It works in a temporary folder that is always removed. On failure
// the installed version is untouched (an unwritable folder returns
// ErrNotWritable). progress may be nil.
func (c *Client) Update(ctx context.Context, rel Release, progress Progress) (Plan, error) {
	if !Supported() {
		return Plan{}, ErrUnsupported
	}
	exe := c.Exe
	if exe == "" {
		var err error
		if exe, err = Executable(); err != nil {
			return Plan{}, err
		}
	}
	tmp, err := os.MkdirTemp("", "cad-lite-update-")
	if err != nil {
		return Plan{}, fmt.Errorf("updater: %w", err)
	}
	defer os.RemoveAll(tmp)
	archive, err := c.Download(ctx, rel, tmp, progress)
	if err != nil {
		return Plan{}, err
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	plan, err := Install(archive, c.GOOS, exe)
	if err != nil {
		return Plan{}, err
	}
	plan.Args = append([]string(nil), os.Args[1:]...)
	plan.Version = rel.Version
	return plan, nil
}

// Install unpacks archive (already verified) and replaces the installation
// that exe belongs to, as goos lays it out:
//
//   - darwin, exe inside X.app: the whole bundle is extracted next to X.app,
//     X.app is renamed to X.app.old, the new bundle renamed into place, and
//     the old one deleted only after that succeeded (restored if it did not);
//   - windows: the running exe is renamed to <exe>.old (allowed while it
//     runs), the new one moved in, and the .old file left for CleanupOld on
//     the next start;
//   - anything else, including a bare macOS binary: the binary is replaced the
//     same way and the old one deleted.
//
// The staging folder is created beside the target so renames stay on one
// file system; if it cannot be created the result wraps ErrNotWritable and
// nothing has changed.
func Install(archive, goos, exe string) (Plan, error) {
	plan := Plan{GOOS: goos, Exe: exe}
	target := exe
	bundle := ""
	if goos == "darwin" {
		if i := strings.Index(filepath.ToSlash(exe), macBundleMarker); i >= 0 {
			bundle = exe[:i+len(".app")]
			target = bundle
			plan.App = bundle
		}
	}
	parent := filepath.Dir(target)
	stage, err := os.MkdirTemp(parent, ".cad-lite-update-")
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %s: %v", ErrNotWritable, parent, err)
	}
	defer os.RemoveAll(stage)
	if err := Extract(archive, stage); err != nil {
		return Plan{}, err
	}
	var fresh string
	if bundle != "" {
		if fresh, err = findApp(stage, filepath.Base(exe), BinaryName, LegacyBinaryName); err != nil {
			return Plan{}, err
		}
		clearQuarantine(fresh)
	} else {
		ext := ""
		if goos == "windows" {
			ext = ".exe"
		}
		name := BinaryName + ext
		p, ok := findFile(stage, name)
		if !ok {
			if p, ok = findFile(stage, LegacyBinaryName+ext); !ok {
				return Plan{}, fmt.Errorf("updater: %s not found in %s", name, filepath.Base(archive))
			}
		}
		if goos != "windows" {
			if err := os.Chmod(p, 0o755); err != nil {
				return Plan{}, fmt.Errorf("updater: %w", err)
			}
		}
		fresh = p
	}
	if err := swap(target, fresh, goos != "windows"); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// BinaryName is the executable's name inside a release archive.
const BinaryName = "cad-lite"

// LegacyBinaryName is the executable's name in archives published while the
// project was called go-draw; Install still accepts it.
const LegacyBinaryName = "go-draw"

// findApp returns the .app directory at the top of dir that holds the main
// executable, under the first of bins that exists.
func findApp(dir string, bins ...string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	for _, e := range ents {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			app := filepath.Join(dir, e.Name())
			for _, bin := range bins {
				if st, err := os.Stat(filepath.Join(app, "Contents", "MacOS", bin)); err == nil && st.Mode().IsRegular() {
					return app, nil
				}
			}
		}
	}
	return "", fmt.Errorf("updater: no app bundle with Contents/MacOS/%s in the archive", bins[0])
}

// swap puts fresh at target, keeping the previous one as target+".old" until
// the new one is in place. With removeOld it then deletes it; a failed
// delete is not an error.
func swap(target, fresh string, removeOld bool) error {
	old := target + ".old"
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("updater: removing stale %s: %w", filepath.Base(old), err)
	}
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("%w: %v", ErrNotWritable, err)
	}
	if err := os.Rename(fresh, target); err != nil {
		if rerr := os.Rename(old, target); rerr != nil {
			return fmt.Errorf("updater: installing failed (%v) and restoring the old version failed too: %w", err, rerr)
		}
		return fmt.Errorf("updater: installing: %w", err)
	}
	if removeOld {
		os.RemoveAll(old)
	}
	return nil
}

// CleanupOld deletes what a previous update left beside the running
// installation (<exe>.old, or <bundle>.app.old). Call it once at startup;
// every failure is ignored.
func CleanupOld() {
	exe, err := Executable()
	if err != nil {
		return
	}
	target := exe
	if i := strings.Index(filepath.ToSlash(exe), macBundleMarker); i >= 0 {
		target = exe[:i+len(".app")]
	}
	os.RemoveAll(target + ".old")
}

// clearQuarantine removes the Gatekeeper quarantine flag from our own
// extracted copy, only when it carries one. Extracting with Go never sets it,
// so this normally does nothing.
func clearQuarantine(app string) {
	if runtime.GOOS != "darwin" {
		return
	}
	if exec.Command("xattr", "-p", "com.apple.quarantine", app).Run() == nil {
		exec.Command("xattr", "-dr", "com.apple.quarantine", app).Run()
	}
}

var errNoExe = errors.New("updater: no executable to start")

// Relaunch starts the installed version with the plan's arguments and returns
// at once; the caller then quits. On macOS it uses `open -n` on the bundle,
// elsewhere it starts the executable detached from this process. Call it
// after the application saved its state and released everything the new
// process needs, as the last thing before exiting.
func Relaunch(p Plan) error {
	switch {
	case p.GOOS == "darwin" && p.App != "":
		args := []string{"-n", p.App}
		if len(p.Args) > 0 {
			args = append(args, "--args")
			args = append(args, p.Args...)
		}
		return exec.Command("open", args...).Start()
	case p.Exe == "":
		return errNoExe
	}
	cmd := exec.Command(p.Exe, p.Args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: starting %s: %w", filepath.Base(p.Exe), err)
	}
	return cmd.Process.Release()
}
