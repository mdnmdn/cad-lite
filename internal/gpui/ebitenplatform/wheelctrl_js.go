//go:build js

package ebitenplatform

import (
	"sync/atomic"
	"syscall/js"
)

var (
	wheelCtrl   atomic.Bool
	wheelCtrlFn js.Func
)

// installWheelCtrlListener records WheelEvent.ctrlKey. A browser reports a
// trackpad pinch as a wheel event with ctrlKey set but no Control keydown, and
// ebiten only derives Control from key events, so the pinch would otherwise
// arrive as a plain precise scroll. The listener is passive and leaves
// ebiten's own preventDefault (which stops the page zoom) in charge.
func installWheelCtrlListener() {
	defer func() { _ = recover() }()
	wheelCtrlFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			wheelCtrl.Store(args[0].Get("ctrlKey").Bool())
		}
		return nil
	})
	opts := js.Global().Get("Object").New()
	opts.Set("capture", true)
	opts.Set("passive", true)
	js.Global().Call("addEventListener", "wheel", wheelCtrlFn, opts)
}

// takeWheelCtrl reports whether the latest wheel event carried ctrlKey.
func takeWheelCtrl() bool { return wheelCtrl.Swap(false) }
