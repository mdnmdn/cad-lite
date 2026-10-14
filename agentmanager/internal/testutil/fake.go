// Package testutil holds helpers shared by the backend test suites.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	buildPath string
	buildErr  error
)

// BuildFake builds ./internal/fakeagent once per test binary and returns the executable path.
// The binary lives in a per-process temp directory that the OS temp cleaner reclaims.
func BuildFake(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Join(filepath.Dir(file), "..", "..")
		dir, err := os.MkdirTemp("", "agentmanager-fakeagent-")
		if err != nil {
			buildErr = err
			return
		}
		name := "fakeagent"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		buildPath = filepath.Join(dir, name)
		goBin, err := exec.LookPath("go")
		if err != nil {
			goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		}
		cmd := exec.Command(goBin, "build", "-o", buildPath, "./internal/fakeagent")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err: err, out: string(out)}
		}
	})
	if buildErr != nil {
		t.Fatalf("build fakeagent: %v", buildErr)
	}
	return buildPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }
