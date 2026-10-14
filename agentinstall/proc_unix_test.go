//go:build unix

package agentinstall

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// alive reports whether the process whose pid is in file still runs; a zombie (killed, not yet
// reaped by an init that does not reap) counts as gone.
func alive(t *testing.T, file string) bool {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no pid file %s", file)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 0 || syscall.Kill(pid, 0) != nil {
		return false
	}
	if st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if f := strings.Fields(string(st[strings.LastIndexByte(string(st), ')')+1:])); len(f) > 0 && f[0] == "Z" {
			return false
		}
	}
	return true
}
