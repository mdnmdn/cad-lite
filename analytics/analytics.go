// Package analytics records bounded, anonymous application events through interchangeable sinks.
// Its background client keeps analytics failures and slow transports away from application work.
package analytics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"cad-lite/appenv"
)

// Properties contains small scalar metadata values; callers must omit content and credentials.
type Properties map[string]any

// Recorder accepts an event without blocking application work or returning delivery failures.
type Recorder interface {
	// Track registers a named activity and its scalar metadata without waiting for delivery.
	Track(name string, properties Properties)
}

// Event is one anonymous activity record.
type Event struct {
	// Name identifies the activity.
	Name string `json:"event"`
	// DistinctID is a random identifier scoped to this application run.
	DistinctID string `json:"distinct_id"`
	// Timestamp is when the application registered the event.
	Timestamp time.Time `json:"timestamp"`
	// Properties describes the activity and application runtime.
	Properties Properties `json:"properties"`
}

// Sink delivers one event on the client's worker. Implementations must honor context cancellation.
type Sink interface {
	// Send delivers one event within the supplied deadline.
	Send(context.Context, Event) error
}

// Client queues up to 128 events and silently drops overflow, invalid values and failed deliveries.
// It is safe for concurrent use. The zero value, including a nil pointer, disables recording.
type Client struct {
	mu      sync.RWMutex
	queue   chan Event
	done    chan struct{}
	cancel  context.CancelFunc
	closing bool
	id      string
}

// New creates an asynchronous client. A nil sink or unavailable randomness disables recording.
// Application and system metadata are collected on its worker, never on the caller's thread.
func New(sink Sink, version string) *Client {
	if sink == nil {
		return nil
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{queue: make(chan Event, 128), done: make(chan struct{}), cancel: cancel, id: hex.EncodeToString(id[:])}
	go c.run(ctx, sink, version)
	return c
}

// Record safely invokes a recorder. A provider panic never escapes into the application.
// Recorders supplied by hosts must respect the interface's nonblocking contract.
func Record(r Recorder, name string, properties Properties) {
	defer func() { _ = recover() }()
	if r != nil {
		r.Track(name, properties)
	}
}

// Track registers a scalar-only snapshot, dropping the event if the queue is full or closed.
func (c *Client) Track(name string, properties Properties) {
	if c == nil || c.queue == nil || name == "" || len(name) > 100 {
		return
	}
	props := make(Properties)
	count := 0
	for key, value := range properties {
		count++
		if count > 32 {
			break
		}
		if len(key) > 100 {
			continue
		}
		switch v := value.(type) {
		case string:
			if len(v) > 256 {
				v = v[:256]
			}
			props[key] = v
		case bool, int, int32, int64, uint, uint32, uint64, float32, float64:
			props[key] = v
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closing {
		return
	}
	select {
	case c.queue <- Event{Name: name, DistinctID: c.id, Timestamp: time.Now().UTC(), Properties: props}:
	default:
	}
}

// Close flushes queued events until ctx expires, then cancels delivery. Application shutdown stays bounded.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.queue == nil {
		return nil
	}
	c.mu.Lock()
	if !c.closing {
		c.closing = true
		close(c.queue)
	}
	c.mu.Unlock()
	select {
	case <-c.done:
		c.cancel()
		return nil
	case <-ctx.Done():
		c.cancel()
		return ctx.Err()
	}
}

func (c *Client) run(ctx context.Context, sink Sink, version string) {
	defer close(c.done)
	common := appenv.SystemMetadata(ctx)
	common["application"], common["application_version"] = "cad-lite", version
	for event := range c.queue {
		if ctx.Err() != nil {
			return
		}
		for key, value := range common {
			event.Properties[key] = value
		}
		event.Properties["$process_person_profile"] = false
		event.Properties["$lib"] = "cad-lite"
		deliver(ctx, sink, event)
	}
}

func deliver(ctx context.Context, sink Sink, event Event) {
	defer func() { _ = recover() }()
	request, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_ = sink.Send(request, event)
}
