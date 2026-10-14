// Package proc spawns agent child processes: env hygiene, stderr ring buffer, graceful close.
package proc

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Options describes the child to start.
type Options struct {
	Path      string
	Args      []string
	Dir       string
	Env       []string // extra KEY=VALUE appended to the inherited environment
	RemoveEnv []string // variable names removed from the inherited environment
}

const stderrCap = 8 << 10

// Process is a running child with piped stdio.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *os.File
	stderr *ring
	done   chan struct{}
	exit   error
	once   sync.Once
}

// Start spawns the child (own process group on unix) with stdin/stdout piped and stderr captured.
func Start(o Options) (*Process, error) {
	cmd := exec.Command(o.Path, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = Env(os.Environ(), o.RemoveEnv, o.Env)
	setGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	// An os.Pipe (rather than StdoutPipe) lets the reader drain output after the child exits;
	// cmd.Wait would otherwise close the read side under the reader.
	cmd.Stdout = pw
	p := &Process{cmd: cmd, stdin: stdin, stdout: pr, stderr: &ring{}, done: make(chan struct{})}
	cmd.Stderr = p.stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		stdin.Close()
		return nil, err
	}
	pw.Close()
	go func() {
		p.exit = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// Stdin is the child's stdin.
func (p *Process) Stdin() io.WriteCloser { return p.stdin }

// Stdout is the child's stdout. Its read end is closed when the reader reaches EOF or an error,
// and by Close.
func (p *Process) Stdout() io.Reader { return eofCloser{p.stdout} }

// eofCloser closes the pipe's read end once the stream ends.
type eofCloser struct{ f *os.File }

func (e eofCloser) Read(b []byte) (int, error) {
	n, err := e.f.Read(b)
	if err != nil {
		e.f.Close()
	}
	return n, err
}

// StderrTail returns the last captured stderr bytes as text.
func (p *Process) StderrTail() string { return p.stderr.String() }

// Done is closed when the child has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitErr is the wait result; valid after Done is closed.
func (p *Process) ExitErr() error { return p.exit }

// Close closes stdin, waits up to grace for exit, then kills the process group. Idempotent.
func (p *Process) Close(grace time.Duration) error {
	p.once.Do(func() {
		_ = p.stdin.Close()
		t := time.NewTimer(grace)
		defer t.Stop()
		select {
		case <-p.done:
		case <-t.C:
			killGroup(p.cmd)
		}
	})
	<-p.done
	_ = p.stdout.Close()
	return nil
}

// Env returns base with the names in remove deleted and extra appended.
func Env(base, remove, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
outer:
	for _, kv := range base {
		for _, name := range remove {
			if strings.HasPrefix(kv, name+"=") {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

// ring keeps the last stderrCap bytes written to it.
type ring struct {
	mu  sync.Mutex
	buf []byte
}

func (r *ring) Write(b []byte) (int, error) {
	r.mu.Lock()
	r.buf = append(r.buf, b...)
	if len(r.buf) > stderrCap {
		r.buf = append([]byte(nil), r.buf[len(r.buf)-stderrCap:]...)
	}
	r.mu.Unlock()
	return len(b), nil
}

func (r *ring) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.TrimSpace(string(r.buf))
}
