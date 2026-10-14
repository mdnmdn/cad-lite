package agentmanager

import (
	"context"
	"errors"
	"sync"
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

// ModelSetter is implemented by sessions that can switch model while live. SetModel blocks until
// the agent accepted or refused model (a non-empty ModelInfo.ID or any id the harness takes) or ctx
// ends; the switch applies from the next turn. On success the session emits ModelsChanged with
// Current set to model. An empty model is an error: to return to the harness default, start a new
// session without Config.Model.
type ModelSetter interface {
	SetModel(ctx context.Context, model string) error
}

// Image is a picture attached to a prompt. MIME is image/png, image/jpeg, image/gif or image/webp;
// Data is the encoded file, sent as is (base64 on the wire). Size limits are the host's.
type Image struct {
	MIME string
	Data []byte
}

// ImagePrompter is implemented by sessions that can attach images to a prompt (all three backends).
type ImagePrompter interface {
	// AcceptsImages reports whether the agent takes images at all: ACP's initialize
	// agentCapabilities.promptCapabilities.image; always true for Claude and Codex.
	AcceptsImages() bool
	// PromptImages is Prompt with images after the text; with no images it is Prompt. It returns
	// ErrNoImages, writing nothing, when images are given and AcceptsImages is false.
	PromptImages(ctx context.Context, text string, images []Image) error
}

// ErrNoImages is returned by PromptImages when the agent does not accept images.
var ErrNoImages = errors.New("agentmanager: the agent does not accept images")

// ErrNoModel is returned by SetModel for an empty model.
var ErrNoModel = errors.New("agentmanager: empty model")

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
type Queue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []Event
	closed bool
	out    chan Event
}

// NewQueue returns an empty Queue and starts its forwarding goroutine.
func NewQueue() *Queue {
	q := &Queue{out: make(chan Event)}
	q.cond = sync.NewCond(&q.mu)
	go q.run()
	return q
}

func (q *Queue) run() {
	defer close(q.out)
	for {
		q.mu.Lock()
		for len(q.buf) == 0 && !q.closed {
			q.cond.Wait()
		}
		if len(q.buf) == 0 {
			q.mu.Unlock()
			return
		}
		ev := q.buf[0]
		q.buf[0] = nil
		q.buf = q.buf[1:]
		q.mu.Unlock()
		q.out <- ev
	}
}

// Push appends ev; it never blocks. Pushes after Close are dropped.
func (q *Queue) Push(ev Event) {
	q.mu.Lock()
	if !q.closed {
		q.buf = append(q.buf, ev)
		q.cond.Signal()
	}
	q.mu.Unlock()
}

// Events is the consumer side; it is closed after Close once every queued event was delivered.
func (q *Queue) Events() <-chan Event { return q.out }

// Close ends the queue after the already pushed events are delivered. Idempotent.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// ErrClosed is returned by Session methods after the session is closed or
// its agent process has exited.
var ErrClosed = errors.New("agentmanager: session closed")
