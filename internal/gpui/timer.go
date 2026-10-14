package gpui

import (
	"sort"
	"time"
)

// Clock supplies nondecreasing time on the UI goroutine.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Timer is a window-owned, one-shot UI callback. It must be used on the UI goroutine.
type Timer struct {
	owner    *Window
	deadline time.Time
	sequence uint64
	callback func(*Window, *App)
	done     bool
}

// Done reports whether the timer fired, was cancelled or its window closed.
// A nil timer is done.
func (t *Timer) Done() bool { return t == nil || t.done || t.owner == nil || t.owner.closed }

// Cancel prevents a pending callback and clears its scheduled wakeup. It is nil-safe.
func (t *Timer) Cancel() {
	if t.Done() {
		return
	}
	t.done = true
	t.callback = nil
	a := t.owner.app
	for i, x := range a.timers {
		if x == t {
			a.timers = append(a.timers[:i], a.timers[i+1:]...)
			break
		}
	}
	t.owner.scheduleTimedWake()
}

// After schedules f on the UI goroutine after delay, then redraws this window.
// Nonpositive delays run on a later Tick, never inline. Closing the window cancels it.
func (w *Window) After(delay time.Duration, f func(*Window, *App)) *Timer {
	if w == nil || w.closed || w.app == nil || f == nil {
		return &Timer{done: true}
	}
	if delay < 0 {
		delay = 0
	}
	a := w.app
	a.nextTimerSequence++
	timer := &Timer{owner: w, deadline: a.clock.Now().Add(delay), sequence: a.nextTimerSequence, callback: f}
	a.timers = append(a.timers, timer)
	w.scheduleTimedWake()
	return timer
}

// FrameTime returns this draw's stable timestamp, or the current clock before the first draw.
func (w *Window) FrameTime() time.Time {
	if w.frameValid {
		return w.frameTime
	}
	return w.app.clock.Now()
}

// RequestNextFrame coalesces a frame at a strictly later timestamp. Calls made
// during Render survive Draw; this does not set ordinary dirty invalidation.
func (w *Window) RequestNextFrame() {
	if w == nil || w.closed || w.app == nil || w.nextFrameRequested {
		return
	}
	w.nextFrameRequested = true
	if w.drawing {
		w.nextFrameAt = w.FrameTime()
	} else {
		w.nextFrameAt = w.app.clock.Now()
	}
	if !w.drawing {
		w.scheduleTimedWake()
	}
}

func (w *Window) scheduleTimedWake() {
	if w.platformWindow == nil || w.app == nil {
		return
	}
	var earliest time.Time
	hasDeadline := false
	if !w.closed && w.nextFrameRequested {
		earliest = w.nextFrameAt.Add(time.Nanosecond)
		hasDeadline = true
	}
	if !w.closed {
		for _, t := range w.app.timers {
			if t.owner == w && !t.done && (!hasDeadline || t.deadline.Before(earliest)) {
				earliest = t.deadline
				hasDeadline = true
			}
		}
	}
	if deadline, ok := w.platformWindow.(FrameDeadlineRequester); ok {
		if !hasDeadline {
			deadline.RequestFrameAt(time.Time{})
			return
		}
		if _, headless := w.platformWindow.(*HeadlessWindow); headless {
			deadline.RequestFrameAt(earliest)
			return
		}
		now := w.app.tickTime
		if !w.app.ticking {
			now = w.app.clock.Now()
		}
		if earliest.IsZero() || !now.Before(earliest) {
			deadline.RequestFrameAt(time.Time{})
			w.platformWindow.RequestFrame()
		} else {
			deadline.RequestFrameAt(earliest)
		}
	} else if !w.closed && w.nextFrameRequested && !w.nextFrameWakeIssued {
		w.nextFrameWakeIssued = true
		w.platformWindow.RequestFrame()
	}
}

// Tick runs one bounded UI turn: due timer callbacks, then at most one draw
// per snapshot window. Timers added by callbacks wait for another Tick.
// Custom platforms without timed wakeups call this from their UI loop.
func (a *App) Tick() {
	if a == nil || a.ticking || a.IsQuitting() {
		return
	}
	if a.pendingUpdates > 0 && a.lastTickUpdate == a.updateSerial {
		return
	}
	now := a.clock.Now()
	windows := append([]*Window(nil), a.windows...)
	serials := make(map[*Window]uint64, len(windows))
	for _, w := range windows {
		serials[w] = w.frameSerial
	}
	var due []*Timer
	pending := a.timers[:0]
	for _, t := range a.timers {
		if t.done || t.owner.closed {
			continue
		}
		if !now.Before(t.deadline) {
			due = append(due, t)
		} else {
			pending = append(pending, t)
		}
	}
	a.timers = pending
	sort.SliceStable(due, func(i, j int) bool {
		if due[i].deadline.Equal(due[j].deadline) {
			return due[i].sequence < due[j].sequence
		}
		return due[i].deadline.Before(due[j].deadline)
	})
	a.Update(func(*App) {
		a.lastTickUpdate = a.updateSerial
		a.ticking = true
		a.tickDrawn = make(map[*Window]bool)
		a.tickTime = now
		defer func() { a.ticking = false; clear(a.tickDrawn) }()
		for _, t := range due {
			if t.done || t.owner.closed || a.IsQuitting() {
				t.done = true
				t.callback = nil
				continue
			}
			t.done = true
			f := t.callback
			t.callback = nil
			serial := t.owner.frameSerial
			if f != nil {
				f(t.owner, a)
			}
			if !t.owner.closed && serial == t.owner.frameSerial {
				t.owner.Refresh()
			}
		}
		for _, w := range windows {
			if w.closed || a.IsQuitting() {
				continue
			}
			next := w.nextFrameRequested && now.After(w.nextFrameAt)
			if serials[w] == w.frameSerial && (w.invalid || next) {
				w.Draw(a)
			}
			w.scheduleTimedWake()
		}
	})
}
