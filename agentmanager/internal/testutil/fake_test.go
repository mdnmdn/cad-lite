package testutil_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"agentmanager/internal/testutil"
)

func TestBuildFakeOnceAndRuns(t *testing.T) {
	p := testutil.BuildFake(t)
	if q := testutil.BuildFake(t); q != p {
		t.Fatalf("second call returned %s", q)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(p)
	out, err := cmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 || !strings.Contains(string(out), "no protocol argument") {
		t.Fatalf("err=%v out=%s", err, out)
	}
}
