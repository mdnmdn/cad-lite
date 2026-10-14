package agentinstall

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	ansiRe  = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	countRe = regexp.MustCompile(`^(\d+)\s+(credentials?|environment variables?|integrations?|accounts?)$`)
	// frameRe matches the frame and bullet glyphs opencode's prompt library draws in front of a
	// line, in their Unicode form (│ ┌ └ ● ◇) and their ASCII fallbacks (| T — o), which stand
	// alone, followed by spaces.
	frameRe = regexp.MustCompile(`^(?:[\s│┌└├─●○◇◆■▪•*|—–+-]|[To](?:\s{2,}|$))*`)
)

// HasCredentials reads the output of `opencode auth list` and reports whether it lists at least
// one credential. Two layouts are understood: the framed one, whose sections end with a count
// (`└  1 credentials`, `—  0 environment variables`), where any non-zero count is a credential;
// and the plain one, one line per credential (`Anthropic  oauth  ~/.local/...`), where any line
// that is not a heading or a "no ..." notice is one. Colours are ignored.
func HasCredentials(out string) bool {
	counted, nonZero, entries := false, false, 0
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimSpace(frameRe.ReplaceAllString(stripANSI(raw), ""))
		if l == "" {
			continue
		}
		low := strings.ToLower(l)
		if m := countRe.FindStringSubmatch(low); m != nil {
			counted = true
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				nonZero = true
			}
			continue
		}
		switch {
		case strings.HasPrefix(low, "credentials"), strings.HasPrefix(low, "environment"),
			strings.HasPrefix(low, "no "), strings.Contains(low, "not logged in"),
			strings.Contains(low, "not authenticated"), strings.HasPrefix(low, "warn"),
			strings.HasPrefix(low, "provider") && strings.Contains(low, "type"):
			continue
		}
		entries++
	}
	if counted {
		return nonZero
	}
	return entries > 0
}

// stripANSI removes terminal colour sequences.
func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }
