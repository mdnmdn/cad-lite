package ebitenplatform

import (
	"errors"
	"math"
	"sync"

	"cad-lite/internal/gpui"
)

// errPinchUnsupported is returned by installPinchMonitor on platforms with no
// native pinch source; the platform then simply never sends gpui.PinchEvent.
var errPinchUnsupported = errors.New("ebitenplatform: no native pinch gesture source")

// pinchQueueMax bounds the queue so a stalled game loop cannot grow it.
const pinchQueueMax = 256

// Cocoa NSEventModifierFlags bits.
const (
	cocoaShift   = 1 << 17
	cocoaControl = 1 << 18
	cocoaOption  = 1 << 19
	cocoaCommand = 1 << 20
)

// pinchQueue hands pinch events from the thread that produces them (the
// macOS main thread, in an NSEvent monitor) to the ebiten Update loop. It is
// the only state the producer touches.
type pinchQueue struct {
	mu sync.Mutex
	q  []gpui.PinchEvent
}

func newPinchQueue() *pinchQueue { return &pinchQueue{} }

// push queues e; the oldest events are dropped beyond pinchQueueMax.
func (p *pinchQueue) push(e gpui.PinchEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.q) >= pinchQueueMax {
		p.q = p.q[1:]
	}
	p.q = append(p.q, e)
}

// drain returns the queued events in order and empties the queue.
func (p *pinchQueue) drain() []gpui.PinchEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.q
	p.q = nil
	return out
}

// cocoaPoint converts a Cocoa window point (origin bottom-left, in points) to
// gpui window coordinates (origin top-left, logical pixels). contentX,
// contentY and contentH are the content view's frame origin and height in
// window coordinates. Cocoa points are device independent, so no scale
// applies: they already are the logical pixels the backend uses.
func cocoaPoint(x, y, contentX, contentY, contentH float64) gpui.Point {
	return gpui.Point{
		X: gpui.Pixels(x - contentX),
		Y: gpui.Pixels(contentH - (y - contentY)),
	}
}

// cocoaModifiers maps NSEventModifierFlags to gpui modifiers (Command is
// ModPlatform, as for keyboard events).
func cocoaModifiers(flags uint64) gpui.Modifiers {
	return modifiersFor(
		flags&cocoaShift != 0,
		flags&cocoaControl != 0,
		flags&cocoaOption != 0,
		flags&cocoaCommand != 0)
}

// pinchDelta converts NSEvent.magnification to PinchEvent.Delta. It reports
// false for a zero or non-finite value; the result never reaches -1, where
// the scale factor 1+Delta would vanish.
func pinchDelta(m float64) (float32, bool) {
	if m == 0 || math.IsNaN(m) || math.IsInf(m, 0) {
		return 0, false
	}
	return float32(math.Max(m, -0.95)), true
}
