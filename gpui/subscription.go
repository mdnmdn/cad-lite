package gpui

import "sync"

// Subscription is a handle that cancels a registration when closed. Close is
// idempotent and safe on a nil receiver, so `defer sub.Close()` is always
// correct even when the subscription may not exist.
type Subscription struct {
	close  func()
	closed bool
}

// NewSubscription returns a Subscription that runs close the first time it is
// closed.
func NewSubscription(close func()) *Subscription {
	return &Subscription{close: close}
}

// Close runs the subscription's close function exactly once. Calling Close on
// a nil *Subscription is a no-op.
func (s *Subscription) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	fn := s.close
	s.close = nil
	if fn != nil {
		fn()
	}
}

// IsClosed reports whether Close has been called. A nil *Subscription counts
// as closed.
func (s *Subscription) IsClosed() bool {
	if s == nil {
		return true
	}
	return s.closed
}

// SubscriberSet stores callbacks keyed by an arbitrary comparable emitter key.
//
// It is safe against re-entrant modification: Emit snapshots the callbacks for
// a key before iterating, so callbacks inserted during an Emit run only on the
// next Emit (and only for keys emitted after the insert). Deactivated
// callbacks (whose Subscription was closed, or whose key was Removed) are
// skipped during Emit and pruned lazily.
//
// The set is guarded by a mutex so a later App that touches it from multiple
// goroutines cannot corrupt it, but callbacks themselves run synchronously on
// the Emit caller's goroutine; there is no cross-goroutine ordering guarantee
// beyond the mutex.
type SubscriberSet[K comparable, T any] struct {
	mu      sync.Mutex
	entries map[K][]*subscriberEntry[K, T]
	dirty   bool
}

type subscriberEntry[K comparable, T any] struct {
	cb     T
	closed bool
}

// NewSubscriberSet returns an empty set.
func NewSubscriberSet[K comparable, T any]() *SubscriberSet[K, T] {
	return &SubscriberSet[K, T]{}
}

// Insert registers cb for key and returns a Subscription whose Close
// deactivates that callback.
func (s *SubscriberSet[K, T]) Insert(key K, cb T) *Subscription {
	s.mu.Lock()
	e := &subscriberEntry[K, T]{cb: cb}
	if s.entries == nil {
		s.entries = make(map[K][]*subscriberEntry[K, T])
	}
	s.entries[key] = append(s.entries[key], e)
	s.mu.Unlock()
	return NewSubscription(func() {
		s.mu.Lock()
		e.closed = true
		s.dirty = true
		s.mu.Unlock()
	})
}

// Emit calls emit for every live callback registered for key, in insertion
// order. Callbacks inserted during this Emit are not invoked until a later
// Emit. Callbacks closed during the Emit are not invoked again.
func (s *SubscriberSet[K, T]) Emit(key K, emit func(cb T)) {
	s.mu.Lock()
	if s.dirty {
		s.pruneLocked()
	}
	snapshot := append([]*subscriberEntry[K, T](nil), s.entries[key]...)
	s.mu.Unlock()
	for _, e := range snapshot {
		s.mu.Lock()
		closed := e.closed
		s.mu.Unlock()
		if !closed {
			emit(e.cb)
		}
	}
}

// Remove deactivates and drops every callback registered for key.
func (s *SubscriberSet[K, T]) Remove(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries[key] {
		e.closed = true
	}
	delete(s.entries, key)
}

// Len returns the number of live callbacks across all keys, pruning
// deactivated ones first.
func (s *SubscriberSet[K, T]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dirty {
		s.pruneLocked()
	}
	n := 0
	for _, list := range s.entries {
		n += len(list)
	}
	return n
}

func (s *SubscriberSet[K, T]) pruneLocked() {
	for key, list := range s.entries {
		kept := list[:0]
		for _, e := range list {
			if !e.closed {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(s.entries, key)
		} else {
			s.entries[key] = kept
		}
	}
	s.dirty = false
}
