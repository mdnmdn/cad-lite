package ebitenplatform

import (
	"errors"
	"sync"
)

// errOpenFilesUnsupported is returned by installOpenFilesHandler where files
// to open arrive as command-line arguments instead (every OS but macOS).
var errOpenFilesUnsupported = errors.New("ebitenplatform: no open-documents event on this OS")

// openQueueMax bounds the open-files queue so a stalled game loop cannot grow it.
const openQueueMax = 64

// openQueue hands open-documents requests from the macOS main thread to the
// ebiten Update loop.
type openQueue struct {
	mu sync.Mutex
	q  [][]string
}

func newOpenQueue() *openQueue { return &openQueue{} }

// push queues a batch of paths; empty batches are dropped, and the oldest
// batches beyond openQueueMax.
func (o *openQueue) push(paths []string) {
	if len(paths) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.q) >= openQueueMax {
		o.q = o.q[1:]
	}
	o.q = append(o.q, paths)
}

// drain returns the queued batches in order and empties the queue.
func (o *openQueue) drain() [][]string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.q
	o.q = nil
	return out
}
