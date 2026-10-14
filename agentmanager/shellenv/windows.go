package shellenv

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Registry keys that hold the Windows PATH.
const (
	regMachine = `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	regUser    = `HKCU\Environment`
)

// windowsPATH reads the Machine then the User Path (the order Windows builds
// a new process's PATH in) with reg.exe and expands %VARS%.
func windowsPATH(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	var parts []string
	var firstErr error
	for _, key := range []string{regMachine, regUser} {
		cmd := exec.CommandContext(ctx, "reg", "query", key, "/v", "Path")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if v, ok := parseRegValue(string(out), "Path"); ok {
			parts = append(parts, expandVars(v, os.Getenv))
		}
	}
	if len(parts) == 0 {
		if firstErr == nil {
			firstErr = errors.New("no Path value")
		}
		return "", errors.New("shellenv: registry: " + firstErr.Error())
	}
	return strings.Join(parts, ";"), nil
}

// parseRegValue extracts the data of the named value from `reg query` output,
// whose lines look like "    Path    REG_EXPAND_SZ    C:\a;%USERPROFILE%\b".
func parseRegValue(out, name string) (string, bool) {
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !strings.EqualFold(f[0], name) || !strings.HasPrefix(f[1], "REG_") {
			continue
		}
		i := strings.Index(line, f[1])
		return strings.TrimSpace(line[i+len(f[1]):]), true
	}
	return "", false
}

// expandVars replaces %NAME% with lookup(NAME); an unknown or empty variable
// stays as written.
func expandVars(s string, lookup func(string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		b.WriteString(s[:i])
		if v := lookup(name); v != "" && name != "" {
			b.WriteString(v)
		} else {
			b.WriteString(s[i : i+j+2])
		}
		s = s[i+j+2:]
	}
	b.WriteString(s)
	return b.String()
}
