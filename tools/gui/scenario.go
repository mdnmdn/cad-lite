package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Step is one parsed scenario line.
type Step struct {
	// Line is the 1-based source line, for error messages.
	Line int
	// Op is the verb: move, click, dclick, drag, key, type, wheel, wait,
	// shot, expect-count, expect-diff, expect-same or quit.
	Op string
	// Args are the verb's arguments, with quotes removed.
	Args []string
}

// Scenario is a parsed .gui file.
type Scenario struct {
	// AppFlags are passed to cmd/cad (from the "app" line).
	AppFlags []string
	// Width and Height are the window size in logical pixels (from "size").
	Width, Height int
	// Steps run in order after the window is mapped.
	Steps []Step
}

// ParseScenario reads the scenario DSL: one verb per line, "#" comments,
// double quotes group words. See README.md for the verbs.
func ParseScenario(r io.Reader) (*Scenario, error) {
	sc := &Scenario{Width: 1280, Height: 800}
	in := bufio.NewScanner(r)
	for n := 1; in.Scan(); n++ {
		toks, err := tokenize(in.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if len(toks) == 0 {
			continue
		}
		op, args := toks[0], toks[1:]
		switch op {
		case "app":
			sc.AppFlags = append(sc.AppFlags, args...)
		case "size":
			if len(args) != 2 {
				return nil, fmt.Errorf("line %d: size W H", n)
			}
			w, e1 := strconv.Atoi(args[0])
			h, e2 := strconv.Atoi(args[1])
			if e1 != nil || e2 != nil || w <= 0 || h <= 0 {
				return nil, fmt.Errorf("line %d: size W H", n)
			}
			sc.Width, sc.Height = w, h
			sc.AppFlags = append(sc.AppFlags, "-width", args[0], "-height", args[1])
		default:
			if err := checkStep(op, args); err != nil {
				return nil, fmt.Errorf("line %d: %w", n, err)
			}
			sc.Steps = append(sc.Steps, Step{Line: n, Op: op, Args: args})
		}
	}
	return sc, in.Err()
}

// arity is the allowed argument count range per verb.
var arity = map[string][2]int{
	"move": {2, 2}, "click": {2, 3}, "dclick": {2, 3}, "drag": {4, 5},
	"key": {1, 99}, "type": {1, 1}, "wheel": {1, 3}, "hwheel": {1, 3}, "hold": {1, 1}, "release": {1, 1}, "wait": {1, 1},
	"shot": {1, 1}, "expect-count": {2, 2}, "expect-diff": {2, 2},
	"expect-same": {2, 2}, "quit": {0, 0},
}

func checkStep(op string, args []string) error {
	a, ok := arity[op]
	if !ok {
		return fmt.Errorf("unknown verb %q", op)
	}
	if len(args) < a[0] || len(args) > a[1] {
		return fmt.Errorf("%s: wrong number of arguments", op)
	}
	return nil
}

// tokenize splits a line into words; double quotes group, "#" starts a
// comment outside quotes, and inside quotes \n, \t, \" and \\ are escapes.
func tokenize(s string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	inTok, inQ := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQ && c == '\\' && i+1 < len(s):
			i++
			switch s[i] {
			case 'n':
				cur.WriteByte('\n')
			case 't':
				cur.WriteByte('\t')
			default:
				cur.WriteByte(s[i])
			}
		case c == '"':
			inQ, inTok = !inQ, true
		case inQ:
			cur.WriteByte(c)
		case c == '#':
			i = len(s)
		case c == ' ' || c == '\t':
			if inTok {
				toks = append(toks, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	if inQ {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inTok {
		toks = append(toks, cur.String())
	}
	return toks, nil
}

// CountDXF counts entities by type in the ENTITIES section of DXF text.
func CountDXF(text string) map[string]int {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	counts := map[string]int{}
	inSec, inEnt := false, false
	for i := 0; i+1 < len(lines); i += 2 {
		code, val := strings.TrimSpace(lines[i]), strings.TrimSpace(lines[i+1])
		switch {
		case code == "0" && val == "SECTION":
			inSec = true
		case code == "2" && inSec && !inEnt:
			inEnt = val == "ENTITIES"
			inSec = false
		case code == "0" && val == "ENDSEC":
			inEnt, inSec = false, false
		case code == "0" && inEnt:
			counts[val]++
		}
	}
	return counts
}
