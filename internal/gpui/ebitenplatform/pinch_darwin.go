//go:build darwin

package ebitenplatform

import (
	"fmt"
	"log"
	"structs"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"cad-lite/internal/gpui"
)

// cocoaEventMaskMagnify is NSEventMaskMagnify (1 << NSEventTypeMagnify, 30).
const cocoaEventMaskMagnify = 1 << 30

// The geometry structs mirror NSPoint/NSSize/NSRect as ebiten's own darwin
// code declares them for objc.Send struct returns.
type nsPoint struct {
	_    structs.HostLayout
	X, Y float64
}
type nsSize struct {
	_    structs.HostLayout
	W, H float64
}
type nsRect struct {
	_      structs.HostLayout
	Origin nsPoint
	Size   nsSize
}

var (
	pinchOnce sync.Once
	pinchErr  error
	// pinchInstallCB is kept in a variable so the callback is never collected.
	pinchInstallCB uintptr
)

// installPinchMonitor schedules the monitor install on the main queue, where
// AppKit calls belong, and returns once it is scheduled. It is idempotent.
// Failures to schedule are returned; failures inside the main-queue callback
// are logged once there, and either way pinch is simply unsupported.
func installPinchMonitor(q *pinchQueue) error {
	pinchOnce.Do(func() { pinchErr = scheduleOnMainQueue(q) })
	return pinchErr
}

func scheduleOnMainQueue(q *pinchQueue) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dispatch: %v", r)
		}
	}()
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return err
	}
	mainQ, err := purego.Dlsym(lib, "_dispatch_main_q")
	if err != nil {
		return err
	}
	async, err := purego.Dlsym(lib, "dispatch_async_f")
	if err != nil {
		return err
	}
	pinchInstallCB = purego.NewCallback(func(uintptr) {
		if err := addPinchMonitor(q); err != nil {
			log.Printf("gpui: pinch gestures unavailable: %v", err)
		}
	})
	purego.SyscallN(async, mainQ, 0, pinchInstallCB)
	return nil
}

// addPinchMonitor registers an NSEvent local monitor for magnify events.
// GLFW (so ebiten) drops them. It runs on the main thread, as does the monitor
// block, which only pushes to q; every Objective-C call is guarded so a
// failure returns an error and leaves pinch unsupported.
func addPinchMonitor(q *pinchQueue) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("objc: %v", r)
		}
	}()
	nsEvent := objc.GetClass("NSEvent")
	if nsEvent == 0 {
		return fmt.Errorf("NSEvent class not found")
	}
	var (
		selAdd      = objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:")
		selMagnify  = objc.RegisterName("magnification")
		selLocation = objc.RegisterName("locationInWindow")
		selFlags    = objc.RegisterName("modifierFlags")
		selWindow   = objc.RegisterName("window")
		selContent  = objc.RegisterName("contentView")
		selFrame    = objc.RegisterName("frame")
	)
	handle := func(ev objc.ID) {
		defer func() { _ = recover() }()
		delta, ok := pinchDelta(objc.Send[float64](ev, selMagnify))
		if !ok {
			return
		}
		win := objc.Send[objc.ID](ev, selWindow)
		if win == 0 {
			return
		}
		view := objc.Send[objc.ID](win, selContent)
		if view == 0 {
			return
		}
		frame := objc.Send[nsRect](view, selFrame)
		loc := objc.Send[nsPoint](ev, selLocation)
		flags := objc.Send[uint64](ev, selFlags)
		q.push(gpui.PinchEvent{
			Position:  cocoaPoint(loc.X, loc.Y, frame.Origin.X, frame.Origin.Y, frame.Size.H),
			Delta:     delta,
			Modifiers: cocoaModifiers(flags),
		})
	}
	// The block lives for the whole process, so it is never released.
	block := objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
		handle(ev)
		return ev
	})
	if mon := objc.ID(nsEvent).Send(selAdd, uint64(cocoaEventMaskMagnify), block); mon == 0 {
		return fmt.Errorf("addLocalMonitorForEventsMatchingMask returned nil")
	}
	return nil
}
