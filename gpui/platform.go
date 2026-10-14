package gpui

import "time"

// Platform is the OS/backend abstraction. Implemented by
// gpui/ebitenplatform and by the headless test platform.
type Platform interface {
	// Run blocks until Quit. onLaunch is called once on the main thread
	// before the first frame.
	Run(onLaunch func(*App)) error
	// OpenWindow creates a new OS window.
	OpenWindow(o WindowOptions) (PlatformWindow, error)
	// Quit asks the platform to terminate its run loop.
	Quit()
	// Name is a short backend name: "macos", "windows", "linux", "headless".
	Name() string
}

// PlatformWindow is one OS window. All methods are called on the main thread.
type PlatformWindow interface {
	// Bounds returns the window size in logical pixels.
	Bounds() (w, h Pixels)
	// ScaleFactor is the device pixel ratio (logical px -> device px).
	ScaleFactor() float32
	// RequestFrame asks the platform to draw soon.
	RequestFrame()
	// Present hands the rasterized frame (premultiplied RGBA, 4*w*h bytes)
	// to the platform for display.
	Present(pix []byte, w, h int)
	// SetInputHandler installs or removes the focused text field's editing
	// surface.
	SetInputHandler(h PlatformInputHandler)
	// TakeInputHandler removes and returns the installed input handler.
	TakeInputHandler() PlatformInputHandler
	// Quit closes this window.
	Quit()
}

// FrameRequester is implemented by a PlatformWindow that wants a callback
// invoked when the window should draw. The Window registers its draw routine
// here so the platform can drive frames without holding a *Window.
type FrameRequester interface {
	// OnRequestFrame registers the bounded UI pump. RequestFrame must queue it,
	// never invoke it inline.
	OnRequestFrame(f func())
}

// FrameDeadlineRequester optionally schedules a timed UI wakeup. A call
// replaces the prior timed deadline; zero clears it. RequestFrame is separate.
// Wakeup callbacks must be queued on the UI goroutine, never called inline.
type FrameDeadlineRequester interface{ RequestFrameAt(deadline time.Time) }

// PlatformInputHandler is the focused text field's editing surface. The
// textbox implementation in input.go satisfies it. It is declared here so
// platform.go compiles before input.go lands.
type PlatformInputHandler interface {
	// TextForRange returns the text in the given UTF-16 range.
	TextForRange(rng IntRange) (string, bool)
	// SelectedTextRange returns the current selection.
	SelectedTextRange() (UTF16Selection, bool)
	// MarkedTextRange returns the in-progress composing (marked) range.
	MarkedTextRange() (IntRange, bool)
	// UnmarkText commits any marked text.
	UnmarkText()
	// ReplaceTextInRange replaces the given range (nil range = insert at
	// caret) with text and moves the caret to the end of the inserted text.
	ReplaceTextInRange(rng *IntRange, text string)
	// BoundsForRange returns the on-screen rect of a text range within the
	// element's bounds.
	BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool)
	// CharacterIndexForPoint maps a point within the element's bounds to a
	// UTF-16 code unit offset.
	CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool)
	// SetSelectedTextRange moves the selection to the given range.
	SetSelectedTextRange(rng IntRange)
	// TextLengthUTF16 returns the document length in UTF-16 code units.
	TextLengthUTF16() (int, bool)
	// AcceptsTextInput reports whether this handler wants printable key
	// characters routed to it.
	AcceptsTextInput() bool
}

// IntRange is a half-open [Start, End) range of UTF-16 code unit offsets.
type IntRange struct {
	// Start is the inclusive start offset.
	Start int
	// End is the exclusive end offset.
	End int
}

// UTF16Selection is a selected range of UTF-16 code units plus its caret
// orientation. IsNil marks "no selection" (as opposed to an empty one).
type UTF16Selection struct {
	// Range is the selected span.
	Range IntRange
	// Reversed is true when the caret is at the start of the range.
	Reversed bool
	// IsNil is true when there is no selection at all.
	IsNil bool
}

// WindowOptions describes a window to be opened.
type WindowOptions struct {
	// Width is the initial width in logical pixels.
	Width float32
	// Height is the initial height in logical pixels.
	Height float32
	// Title is the window title.
	Title string
	// Resizable reports whether the user may resize the window.
	Resizable bool
}

// DefaultWindowOptions returns an 800x600 resizable window titled "gpui".
func DefaultWindowOptions() WindowOptions {
	return WindowOptions{Width: 800, Height: 600, Title: "gpui", Resizable: true}
}

// stubPlatform is the Platform used by NewApp when none is supplied. Every
// call is a no-op and OpenWindow hands back a memory-only PlatformWindow, so
// headless tests can still open windows and draw.
type stubPlatform struct{}

// Run calls onLaunch with a freshly built App and returns immediately.
func (stubPlatform) Run(onLaunch func(*App)) error {
	if onLaunch != nil {
		onLaunch(NewAppWithPlatform(stubPlatform{}))
	}
	return nil
}

// OpenWindow returns an in-memory PlatformWindow sized by o.
func (stubPlatform) OpenWindow(o WindowOptions) (PlatformWindow, error) {
	return &stubPlatformWindow{w: Pixels(o.Width), h: Pixels(o.Height)}, nil
}

// Quit is a no-op.
func (stubPlatform) Quit() {}

// Name identifies the stub backend.
func (stubPlatform) Name() string { return "headless" }

// stubPlatformWindow is a PlatformWindow that keeps no OS state. Tests embed
// their own recording platforms when they need Present/RequestFrame data.
type stubPlatformWindow struct {
	w, h    Pixels
	scale   float32
	handler PlatformInputHandler
}

func (s *stubPlatformWindow) Bounds() (Pixels, Pixels) { return s.w, s.h }

func (s *stubPlatformWindow) ScaleFactor() float32 {
	if s.scale <= 0 {
		return 1
	}
	return s.scale
}

func (s *stubPlatformWindow) RequestFrame() {}

func (s *stubPlatformWindow) Present(pix []byte, w, h int) {}

func (s *stubPlatformWindow) SetInputHandler(h PlatformInputHandler) { s.handler = h }

func (s *stubPlatformWindow) TakeInputHandler() PlatformInputHandler {
	h := s.handler
	s.handler = nil
	return h
}

func (s *stubPlatformWindow) Quit() {}
