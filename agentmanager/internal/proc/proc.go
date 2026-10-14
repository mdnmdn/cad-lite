// Package proc spawns agent child processes: env hygiene, stderr ring buffer, graceful close.
package proc

import (
	"io"
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

// Process is a running child with piped stdio.
type Process struct{ _ struct{} }

// Start spawns the child (own process group on unix) with stdin/stdout piped and stderr captured.
func Start(o Options) (*Process, error) { panic("todo: WP1") }

// Stdin is the child's stdin.
func (p *Process) Stdin() io.WriteCloser { panic("todo: WP1") }

// Stdout is the child's stdout.
func (p *Process) Stdout() io.Reader { panic("todo: WP1") }

// StderrTail returns the last captured stderr bytes as text.
func (p *Process) StderrTail() string { panic("todo: WP1") }

// Done is closed when the child has exited.
func (p *Process) Done() <-chan struct{} { panic("todo: WP1") }

// ExitErr is the wait result; valid after Done is closed.
func (p *Process) ExitErr() error { panic("todo: WP1") }

// Close closes stdin, waits up to grace for exit, then kills the process group. Idempotent.
func (p *Process) Close(grace time.Duration) error { panic("todo: WP1") }

// Env returns base with the names in remove deleted and extra appended.
func Env(base, remove, extra []string) []string { panic("todo: WP1") }
