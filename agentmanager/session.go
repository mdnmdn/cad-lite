package agentmanager

import (
	"context"
	"errors"
)

// Session is one live agent conversation. Methods are goroutine-safe.
type Session interface {
	// ID is the backend's session/thread id ("" until SessionStarted).
	ID() string
	// Events delivers events in order; it is closed after the final Exited event.
	// The consumer must keep draining; backends buffer (unbounded) and never block the child.
	Events() <-chan Event
	// Prompt starts a turn; returns once the prompt is written. Error if a turn is running.
	Prompt(ctx context.Context, text string) error
	// Cancel aborts the running turn (pending permissions are answered cancelled first); the
	// session stays usable. The turn ends with TurnEnd{Cancelled}. No-op when idle.
	Cancel() error
	// RespondPermission answers a PermissionRequest by option id; Cancelled=true answers "cancelled".
	RespondPermission(requestID string, ans PermissionAnswer) error
	// Close cancels, closes stdin, waits <=10s, kills, joins goroutines. Idempotent.
	Close() error
}

// Opener starts sessions; every backend package exports one.
type Opener interface {
	Open(ctx context.Context, cfg Config) (Session, error)
}

// PermissionAnswer selects an option or cancels.
type PermissionAnswer struct {
	OptionID  string
	Cancelled bool
}

// Queue is an unbounded, ordered event queue shared by every backend: Push never blocks, and
// Events delivers in push order until Close, after which the channel is closed once drained.
type Queue struct{ _ struct{} }

// NewQueue returns an empty Queue and starts its forwarding goroutine.
func NewQueue() *Queue { panic("todo: WP1") }

// Push appends ev; it never blocks. Pushes after Close are dropped.
func (q *Queue) Push(ev Event) { panic("todo: WP1") }

// Events is the consumer side; it is closed after Close once every queued event was delivered.
func (q *Queue) Events() <-chan Event { panic("todo: WP1") }

// Close ends the queue after the already pushed events are delivered. Idempotent.
func (q *Queue) Close() { panic("todo: WP1") }

// ErrClosed is returned by Session methods after the session is closed or
// its agent process has exited.
var ErrClosed = errors.New("agentmanager: session closed")
