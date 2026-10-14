// Package cadmcp exposes the Cad-lite drawing to AI agents as MCP tools.
//
// gpui is single threaded while MCP handlers run on HTTP goroutines, so every
// tool body runs through a Runner: the UI's Dispatcher is a mutex-guarded
// mailbox that the UI goroutine drains on a timer. Coordinates are model
// units, Y up; angles are degrees; handles are hexadecimal strings.
package cadmcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrUIBusy is returned by Do when the UI goroutine did not run the work in
// time, or the dispatcher is closed.
var ErrUIBusy = errors.New("cadmcp: the drawing window is busy or closed")

// DefaultTimeout bounds Do when the context has no deadline of its own.
const DefaultTimeout = 15 * time.Second

// Runner runs f on the goroutine that owns the drawing and waits for it.
type Runner interface {
	Do(ctx context.Context, f func()) error
}

type itemState int

const (
	pending itemState = iota
	running
	cancelled
)

type item struct {
	f     func()
	done  chan struct{}
	state itemState
	err   error
}

// Dispatcher is a Runner backed by a mailbox: Do enqueues from any
// goroutine, and the UI goroutine calls Drain to run the queued work.
type Dispatcher struct {
	mu     sync.Mutex
	queue  []*item
	closed bool
	wake   func()
}

// NewDispatcher returns an empty dispatcher. wake, when non-nil, is called
// (without locks held, from the enqueuing goroutine) after each enqueue so
// the UI can arm its drain timer; it must be goroutine-safe.
func NewDispatcher(wake func()) *Dispatcher { return &Dispatcher{wake: wake} }

// Do queues f for the UI goroutine and waits until it has run. It returns
// ErrUIBusy, wrapping the context error when there is one, if ctx ends or the
// dispatcher closes before f started; f then never runs. If f was already
// running when ctx ended, Do waits for it and reports its outcome. A panic in
// f is returned as an error.
func (d *Dispatcher) Do(ctx context.Context, f func()) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	it := &item{f: f, done: make(chan struct{})}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrUIBusy
	}
	d.queue = append(d.queue, it)
	wake := d.wake
	d.mu.Unlock()
	if wake != nil {
		wake()
	}
	select {
	case <-it.done:
		return it.err
	case <-ctx.Done():
	}
	d.mu.Lock()
	if it.state == pending {
		it.state = cancelled
		d.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrUIBusy, ctx.Err())
	}
	d.mu.Unlock()
	<-it.done
	return it.err
}

// Drain runs every queued function on the calling goroutine, which must be
// the UI goroutine. Work cancelled before it started is skipped.
func (d *Dispatcher) Drain() {
	d.mu.Lock()
	q := d.queue
	d.queue = nil
	d.mu.Unlock()
	for _, it := range q {
		d.mu.Lock()
		if it.state != pending {
			d.mu.Unlock()
			continue
		}
		it.state = running
		d.mu.Unlock()
		runItem(it)
	}
}

func runItem(it *item) {
	defer close(it.done)
	defer func() {
		if r := recover(); r != nil {
			it.err = fmt.Errorf("cadmcp: tool panicked: %v", r)
		}
	}()
	it.f()
}

// Pending returns the number of queued items not yet started.
func (d *Dispatcher) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, it := range d.queue {
		if it.state == pending {
			n++
		}
	}
	return n
}

// Close makes every queued and future Do return ErrUIBusy. It is idempotent.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	d.closed = true
	q := d.queue
	d.queue = nil
	for _, it := range q {
		if it.state == pending {
			it.state = cancelled
			it.err = ErrUIBusy
			close(it.done)
		}
	}
	d.mu.Unlock()
}

// DirectDispatcher is a Runner for tests and single-goroutine use: it runs f
// inline under a mutex.
type DirectDispatcher struct{ mu sync.Mutex }

// Do runs f immediately unless ctx is already done.
func (d *DirectDispatcher) Do(ctx context.Context, f func()) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrUIBusy, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cadmcp: tool panicked: %v", r)
		}
	}()
	f()
	return nil
}
