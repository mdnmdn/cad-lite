package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Runner is the framework the protocol files share: it reads NDJSON frames from the client,
// records every received line to FAKEAGENT_LOG, writes frames back, and executes scenario steps.
type Runner struct {
	in   *bufio.Reader
	out  io.Writer
	logf io.Writer
	mu   sync.Mutex
	// Vars holds scenario variables ("$name" substitution in send frames).
	Vars map[string]any
}

// NewRunner wraps in/out. FAKEAGENT_LOG, when set, receives every received line verbatim.
func NewRunner(in io.Reader, out io.Writer) *Runner {
	r := &Runner{in: bufio.NewReaderSize(in, 64<<10), out: out, Vars: map[string]any{}}
	if p := os.Getenv("FAKEAGENT_LOG"); p != "" {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			r.logf = f
		}
	}
	return r
}

// ReadFrame returns the next non-blank line and its parsed JSON object (nil when the line is not a
// JSON object). err is io.EOF when stdin is closed.
func (r *Runner) ReadFrame() (raw []byte, frame map[string]any, err error) {
	for {
		line, rerr := r.in.ReadBytes('\n')
		if t := bytes.TrimSpace(line); len(t) > 0 {
			if r.logf != nil {
				r.logf.Write(append(append([]byte(nil), t...), '\n'))
			}
			var m map[string]any
			if json.Unmarshal(t, &m) != nil {
				m = nil
			}
			return t, m, nil
		}
		if rerr != nil {
			return nil, nil, rerr
		}
	}
}

// WriteFrame writes v as one JSON line after substituting "$name" variables.
func (r *Runner) WriteFrame(v any) error {
	b, err := json.Marshal(r.Subst(v))
	if err != nil {
		return err
	}
	return r.WriteLine(b)
}

// WriteLine writes b verbatim plus a newline.
func (r *Runner) WriteLine(b []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.out.Write(append(append([]byte(nil), b...), '\n'))
	return err
}

// Subst replaces every string equal to "$name" (a known variable) inside v, recursively.
func (r *Runner) Subst(v any) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "$") {
			if val, ok := r.Vars[x[1:]]; ok {
				return val
			}
		}
		return x
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = r.Subst(e)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, e := range x {
			s[i] = r.Subst(e)
		}
		return s
	}
	return v
}

// Match reports whether frame contains want: maps compare as subsets, "*" matches any present
// value, numbers compare by value, everything else by equality.
func Match(want, frame any) bool {
	switch w := want.(type) {
	case map[string]any:
		f, ok := frame.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			fv, present := f[k]
			if !present || !Match(wv, fv) {
				return false
			}
		}
		return true
	case []any:
		f, ok := frame.([]any)
		if !ok || len(f) < len(w) {
			return false
		}
		for i := range w {
			if !Match(w[i], f[i]) {
				return false
			}
		}
		return true
	case string:
		if w == "*" {
			return true
		}
	}
	return fmt.Sprint(want) == fmt.Sprint(frame)
}

// Expect reads frames until one matches (non-matching frames are skipped) and returns it.
func (r *Runner) Expect(match map[string]any) (map[string]any, error) {
	want := r.Subst(match)
	for {
		_, f, err := r.ReadFrame()
		if err != nil {
			return nil, err
		}
		if f != nil && Match(want, f) {
			return f, nil
		}
	}
}

// RunSteps executes steps in order and returns the process exit code: Step exit's code, 0 once the
// steps finish, or 1 when stdin closes while a step still waits for input.
func (r *Runner) RunSteps(steps []Step) int {
	for i, s := range steps {
		switch s.Op {
		case "expect":
			f, err := r.Expect(s.Match)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fakeagent: step %d expect %v: %v\n", i, s.Match, err)
				return 1
			}
			if s.Var != "" {
				r.Vars[s.Var] = f["id"]
			}
		case "send":
			if err := r.WriteFrame(s.Frame); err != nil {
				return 1
			}
		case "wait-for-reply":
			want := r.Vars[s.Var]
			for {
				_, f, err := r.ReadFrame()
				if err != nil {
					fmt.Fprintf(os.Stderr, "fakeagent: step %d wait-for-reply %s: %v\n", i, s.Var, err)
					return 1
				}
				if _, hasMethod := f["method"]; f != nil && !hasMethod && fmt.Sprint(f["id"]) == fmt.Sprint(want) {
					break
				}
			}
		case "sleep":
			time.Sleep(time.Duration(s.MS) * time.Millisecond)
		case "stderr":
			fmt.Fprint(os.Stderr, s.Frame["text"])
		case "hang":
			go io.Copy(io.Discard, r.in)
			select {}
		case "exit":
			return s.Code
		}
	}
	return 0
}
