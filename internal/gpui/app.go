package gpui

import (
	"reflect"
	"sync/atomic"
	"time"
)

// EntityRoot is the root view a window renders. Any type with a Render
// method satisfies it; use RenderFunc to adapt a plain function.
type EntityRoot interface {
	Render(w *Window, cx *App) IntoElement
}

// RenderFunc adapts a plain function into an EntityRoot.
type RenderFunc func(w *Window, cx *App) IntoElement

// Render satisfies EntityRoot by calling the function.
func (f RenderFunc) Render(w *Window, cx *App) IntoElement { return f(w, cx) }

// effectKind discriminates the queued work flushEffects drains.
type effectKind uint8

const (
	// effectNotify runs an entity's observers.
	effectNotify effectKind = iota
	// effectDefer runs a raw func(*App).
	effectDefer
	// effectEmit delivers an event to an entity's event listeners.
	effectEmit
	// effectRefreshWindows marks every window dirty.
	effectRefreshWindows
)

// effect is one queued unit of work for flushEffects.
type effect struct {
	kind    effectKind
	entity  EntityID
	handler func(*App)
	event   any
	typ     reflect.Type
}

// eventKey identifies one entity's listener set for one event type.
type eventKey struct {
	entity EntityID
	typ    reflect.Type
}

// App owns all entity state, windows, the keymap and the effect queue. It is
// the root context: everything that mutates state does so inside Update.
type App struct {
	platform   Platform
	entities   map[uint32]*entitySlot
	freeIDs    []uint32
	nextIndex  uint32
	generation map[uint32]uint32
	windows    []*Window
	focusMap   focusMap
	keymap     *Keymap

	pendingEffects []effect
	pendingNotify  map[EntityID]bool
	pendingRelease []EntityID

	observers      *SubscriberSet[EntityID, func(EntityID, *App)]
	eventListeners *SubscriberSet[eventKey, func(any)]

	globals        map[reflect.Type]any
	pendingUpdates int
	quitting       atomic.Bool

	shaper     *Shaper
	shaperInit bool

	// drawing is the window whose Draw is running; entity reads during it
	// are recorded on that window for Notify-driven redraws.
	drawing                      *Window
	clock                        Clock
	tooltipStyle                 *TooltipStyle
	timers                       []*Timer
	nextTimerSequence            uint64
	updateSerial, lastTickUpdate uint64
	ticking                      bool
	tickTime                     time.Time
	tickDrawn                    map[*Window]bool

	// onOpenPaths receives OpenPaths requests; openQueue holds the ones that
	// came while it was nil.
	onOpenPaths func(paths []string, cx *App)
	openQueue   [][]string
}

// NewApp returns an App with a no-op stub platform, suitable for headless
// use and tests.
func NewApp() *App { return NewAppWithPlatform(nil) }

// NewAppWithPlatform returns an App driven by p. A nil p selects the stub
// platform, which no-ops every call and hands out in-memory windows.
func NewAppWithPlatform(p Platform) *App { return NewAppWithClock(p, nil) }

// NewAppWithClock uses an explicit clock. Nil uses the platform Clock if
// available, otherwise system time. Call Tick on the UI goroutine.
func NewAppWithClock(p Platform, clock Clock) *App {
	if p == nil {
		p = stubPlatform{}
	}
	if clock == nil {
		if c, ok := p.(Clock); ok {
			clock = c
		} else {
			clock = systemClock{}
		}
	}
	return &App{
		clock:          clock,
		platform:       p,
		entities:       make(map[uint32]*entitySlot),
		generation:     make(map[uint32]uint32),
		focusMap:       make(focusMap),
		keymap:         NewKeymap(),
		pendingNotify:  make(map[EntityID]bool),
		observers:      NewSubscriberSet[EntityID, func(EntityID, *App)](),
		eventListeners: NewSubscriberSet[eventKey, func(any)](),
	}
}

// Update runs f with the app locked for mutation. Nested Update calls on the
// same app are counted; only the outermost call runs flushEffects when it
// returns. This deliberately replaces Rust's RefCell double-borrow panic:
// nesting is allowed and effects flush exactly once, at the end of the
// outermost Update.
func (a *App) Update(f func(*App)) {
	if f == nil {
		return
	}
	if a.pendingUpdates == 0 {
		a.updateSerial++
	}
	a.pendingUpdates++
	defer func() {
		a.pendingUpdates--
		if a.pendingUpdates == 0 {
			a.flushEffects()
		}
	}()
	f(a)
}

// Quit marks the app as quitting and asks the platform to terminate. Unlike
// the rest of App, Quit and IsQuitting are safe to call from any goroutine
// (for example a time.AfterFunc): the flag is atomic and Platform.Quit
// implementations must be goroutine-safe.
func (a *App) Quit() {
	a.quitting.Store(true)
	if a.platform != nil {
		a.platform.Quit()
	}
}

// IsQuitting reports whether Quit has been called; safe from any goroutine.
func (a *App) IsQuitting() bool { return a.quitting.Load() }

// Keymap returns the app's keymap; add bindings with AddBindings.
func (a *App) Keymap() *Keymap { return a.keymap }

// Platform returns the platform driving this app.
func (a *App) Platform() Platform { return a.platform }

// NewFocusHandle allocates a fresh focus id and returns a handle to it.
func (a *App) NewFocusHandle() *FocusHandle {
	return &FocusHandle{id: a.focusMap.alloc(), app: a}
}

// Notify schedules the entity's observers to run during the next effect
// flush and, as in GPUI, marks every window that read the entity during its
// last draw invalid, so it redraws at the end of the flush. Notifications
// for the same entity within one flush are deduplicated.
func (a *App) Notify(id EntityID) {
	if id.IsNil() {
		return
	}
	for _, w := range a.windows {
		if _, ok := w.renderedEntities[id]; ok {
			w.invalid = true
		}
	}
	if a.pendingNotify[id] {
		return
	}
	a.pendingNotify[id] = true
	a.pendingEffects = append(a.pendingEffects, effect{kind: effectNotify, entity: id})
}

// RefreshWindows queues a refresh of every window: each is marked invalid
// during the next effect flush and redrawn.
func (a *App) RefreshWindows() {
	a.pendingEffects = append(a.pendingEffects, effect{kind: effectRefreshWindows})
}

// Defer schedules f to run at the end of the current effect cycle, after the
// enclosing Update body returns.
func (a *App) Defer(f func(*App)) {
	if f == nil {
		return
	}
	a.pendingEffects = append(a.pendingEffects, effect{kind: effectDefer, handler: f})
}

// Observe and Subscribe are package-level functions (ObserveOn and
// Subscribe) rather than App methods, because Go methods cannot introduce
// type parameters of their own. See context.go.

// SetGlobal stores a process-wide value keyed by its concrete type,
// replacing any previous value of that type.
func (a *App) SetGlobal(v any) {
	if v == nil {
		return
	}
	if a.globals == nil {
		a.globals = make(map[reflect.Type]any)
	}
	a.globals[reflect.TypeOf(v)] = v
}

// Global copies the value stored by SetGlobal into out, which must be a
// non-nil pointer to the value's type. It reports whether a value was found.
func (a *App) Global(out any) bool {
	if out == nil || a.globals == nil {
		return false
	}
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return false
	}
	v, ok := a.globals[rv.Type().Elem()]
	if !ok {
		return false
	}
	vv := reflect.ValueOf(v)
	if !vv.Type().AssignableTo(rv.Type().Elem()) {
		return false
	}
	rv.Elem().Set(vv)
	return true
}

// UpdateGlobal calls f once per stored global with the current value. To
// change a global, hold it behind a pointer and mutate in place, or call
// SetGlobal from inside f; f cannot replace a value by itself because the
// callback signature is untyped (func(any)).
func (a *App) UpdateGlobal(f func(any)) {
	if f == nil {
		return
	}
	for _, v := range a.globals {
		f(v)
	}
}

// OpenWindow creates a platform window of size o, builds its root view,
// draws it once immediately (mirroring app.rs's launch path), and appends it
// to Windows.
func (a *App) OpenWindow(o WindowOptions, build func(*Window, *App) EntityRoot) error {
	pw, err := a.platform.OpenWindow(o)
	if err != nil {
		return err
	}
	w := newWindow(pw, o, a)
	w.root = build(w, a)
	a.windows = append(a.windows, w)
	a.Update(func(*App) { w.Draw(a) })
	return nil
}

// defaultShaper returns the App's shared shaper, creating it on first use. It
// uses the bundled fallback face when no system font is available.
func (a *App) defaultShaper() *Shaper {
	if !a.shaperInit {
		a.shaperInit = true
		a.shaper = NewShaper(NewFontManager())
	}
	return a.shaper
}

// Windows returns the open windows in creation order.
func (a *App) Windows() []*Window { return a.windows }

// flushEffects drains the effect queue and releases the entities whose last
// strong reference was dropped (repeating while either produces more work),
// then requests a frame for every window that was marked invalid. Effects queued while draining (by
// observers, defers or emits) are processed in the same flush, so cascading
// notifies terminate only when no new work appears.
func (a *App) flushEffects() {
	// Callbacks run below may call Update (or Release); counting the flush
	// as an Update keeps them from starting a nested flush.
	a.pendingUpdates++
	defer func() { a.pendingUpdates-- }()
	for len(a.pendingEffects) > 0 || len(a.pendingRelease) > 0 {
		if len(a.pendingEffects) == 0 {
			releases := a.pendingRelease
			a.pendingRelease = nil
			for _, id := range releases {
				a.releaseEntity(id)
			}
			continue
		}
		batch := a.pendingEffects
		a.pendingEffects = nil
		for _, e := range batch {
			switch e.kind {
			case effectNotify:
				delete(a.pendingNotify, e.entity)
				id := e.entity
				a.observers.Emit(id, func(cb func(EntityID, *App)) {
					cb(id, a)
				})
			case effectDefer:
				if e.handler != nil {
					e.handler(a)
				}
			case effectEmit:
				key := eventKey{entity: e.entity, typ: e.typ}
				payload := e.event
				a.eventListeners.Emit(key, func(cb func(any)) {
					cb(payload)
				})
			case effectRefreshWindows:
				for _, w := range a.windows {
					w.invalid = true
				}
			}
		}
	}
	for _, w := range a.windows {
		if w.invalid {
			w.scheduleFrame()
		}
	}
}

// OnOpenPaths sets the handler for the OS asking the app to open files with no
// window position (macOS: a file opened from Finder or dropped on the Dock
// icon). It replaces any earlier handler; nil removes it. Requests that came
// while no handler was set are delivered to f at once, in order.
func (a *App) OnOpenPaths(f func(paths []string, cx *App)) {
	a.onOpenPaths = f
	if f == nil {
		return
	}
	q := a.openQueue
	a.openQueue = nil
	for _, paths := range q {
		a.OpenPaths(paths)
	}
}

// OpenPaths hands an open-files request to the OnOpenPaths handler inside
// Update, or queues it until one is set. Platforms call it on the UI
// goroutine; tests call it to inject the request. Empty requests are dropped.
func (a *App) OpenPaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	paths = append([]string(nil), paths...)
	f := a.onOpenPaths
	if f == nil {
		a.openQueue = append(a.openQueue, paths)
		return
	}
	a.Update(func(cx *App) { f(paths, cx) })
}
