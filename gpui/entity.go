package gpui

import (
	"fmt"
	"reflect"
)

// EntityID mirrors gpui's slotmap key: an index plus a generation. A slot
// whose generation no longer matches the id is dead.
type EntityID struct {
	index      uint32
	generation uint32
}

// IsNil reports whether the id is the zero (unallocated) id.
func (id EntityID) IsNil() bool { return id.index == 0 && id.generation == 0 }

// String renders the id as "entity(index.generation)".
func (id EntityID) String() string {
	return fmt.Sprintf("entity(%d.%d)", id.index, id.generation)
}

// Entity is a cheap, copyable handle to state stored in the App.
//
// Refcounting: NewEntity returns the first strong reference and Clone adds
// one; Release drops one, and when none remain the entity is released at the
// end of the current effect flush. Go has no Drop, so plain copies of an
// Entity (and the handles returned by Context.Entity and WeakEntity.Upgrade)
// are borrowed: they do not count and must not be Released.
type Entity[T any] struct {
	id  EntityID
	app *App
}

// WeakEntity is a non-owning view of an Entity, safe to hold across frames.
type WeakEntity[T any] struct {
	id  EntityID
	app *App
}

// entitySlot is the App-side storage for one entity: the boxed *T plus its
// generation and refcount.
type entitySlot struct {
	value      any // *T
	generation uint32
	refs       int
	typeID     reflect.Type
	// onRelease runs when the entity is released; owned are subscriptions
	// the entity created (Context.Observe) and are closed with it.
	onRelease []*releaseHook
	owned     []*Subscription
}

// releaseHook is one OnRelease callback; closed hooks are skipped.
type releaseHook struct {
	f      func(*App)
	closed bool
}

// ID returns the entity's slot id.
func (e Entity[T]) ID() EntityID { return e.id }

// NewEntity allocates state in cx and returns a handle to it. build runs
// with a Context whose Notify may be called to schedule a re-render.
func NewEntity[T any](cx *App, build func(cx *Context[T]) T) Entity[T] {
	return newEntity(cx, build)
}

// newEntity allocates a slot, runs build, and stores the result as *T. It is
// a package-level function rather than an App method because Go methods
// cannot introduce type parameters of their own.
func newEntity[T any](a *App, build func(cx *Context[T]) T) Entity[T] {
	var idx uint32
	if n := len(a.freeIDs); n > 0 {
		idx = a.freeIDs[n-1]
		a.freeIDs = a.freeIDs[:n-1]
	} else {
		a.nextIndex++
		idx = a.nextIndex
	}
	gen := a.generation[idx] + 1
	a.generation[idx] = gen
	id := EntityID{index: idx, generation: gen}
	// The slot exists before build runs so that Observe and OnRelease called
	// from build attach to it; Read during build sees no value yet.
	s := &entitySlot{
		generation: gen,
		refs:       1,
		typeID:     reflect.TypeOf((*T)(nil)).Elem(),
	}
	a.entities[idx] = s
	v := build(&Context[T]{App: a, entity: id})
	s.value = &v
	return Entity[T]{id: id, app: a}
}

// slot returns the live storage for id, or nil when the id is stale.
func (a *App) slot(id EntityID) *entitySlot {
	if a == nil || id.index == 0 {
		return nil
	}
	s := a.entities[id.index]
	if s == nil || s.generation != id.generation {
		return nil
	}
	return s
}

// Read returns a pointer to the entity's state, or nil when the entity has
// been released. The pointer is only valid until the next call that can
// re-enter the same entity; prefer Context.Listener for callbacks. A read
// during a window's draw makes Notify on the entity redraw that window.
func (e Entity[T]) Read(cx *App) *T {
	app := cx
	if app == nil {
		app = e.app
	}
	s := app.slot(e.id)
	if s == nil {
		return nil
	}
	app.noteAccess(e.id)
	st, ok := s.value.(*T)
	if !ok {
		return nil
	}
	return st
}

// Update mutates the entity's state. A stale or released entity is a no-op.
func (e Entity[T]) Update(cx *App, f func(state *T, cx *Context[T])) {
	if cx == nil || f == nil {
		return
	}
	s := cx.slot(e.id)
	if s == nil {
		return
	}
	st, ok := s.value.(*T)
	if !ok {
		return
	}
	cx.noteAccess(e.id)
	f(st, &Context[T]{App: cx, entity: e.id})
}

// Clone returns a new strong reference to the entity (refcount + 1). Each
// Clone must be balanced by a Release. Cloning a released entity returns
// the (dead) handle unchanged.
func (e Entity[T]) Clone() Entity[T] {
	if s := e.app.slot(e.id); s != nil {
		s.refs++
	}
	return e
}

// Release drops one strong reference. When the last one is gone the entity
// is released at the end of the current effect flush (immediately, via a
// flush, when called outside Update and outside a flush): its slot is freed, its OnRelease
// callbacks run, its observers and event listeners are dropped and the
// subscriptions it created are closed. Releasing a dead entity is a no-op.
func (e Entity[T]) Release(cx *App) {
	app := cx
	if app == nil {
		app = e.app
	}
	s := app.slot(e.id)
	if s == nil || s.refs <= 0 {
		return
	}
	s.refs--
	if s.refs > 0 {
		return
	}
	app.Update(func(a *App) { a.scheduleRelease(e.id) })
}

// Downgrade returns a weak handle that does not keep the entity alive.
func (e Entity[T]) Downgrade() WeakEntity[T] {
	return WeakEntity[T]{id: e.id, app: e.app}
}

// Notify schedules the entity's observers to run during the next effect
// flush.
func (e Entity[T]) Notify(cx *App) {
	if cx != nil {
		cx.Notify(e.id)
	}
}

// SubscribeEvent registers f to run when an E event is emitted on e. It is
// a package-level function because Go methods cannot introduce type
// parameters of their own.
func SubscribeEvent[T any, E any](cx *App, e Entity[T], f func(ev *E)) *Subscription {
	if cx == nil {
		return NewSubscription(nil)
	}
	return Subscribe[T, E](cx, e, f)
}

// Upgrade re-checks the weak handle against the App's storage, returning a
// live Entity when the slot still exists.
func (w WeakEntity[T]) Upgrade() (Entity[T], bool) {
	if w.app == nil || w.app.slot(w.id) == nil {
		return Entity[T]{}, false
	}
	return Entity[T]{id: w.id, app: w.app}, true
}

// releaseEntity frees the slot of an entity with no strong references left
// and bumps its generation so stale ids fail their generation check, then
// tears down what hangs off it: OnRelease callbacks, observers and event
// listeners keyed by it, and the subscriptions it owns. It is a no-op for a
// dead id or one that was re-cloned after its release was scheduled.
func (a *App) releaseEntity(id EntityID) {
	s := a.slot(id)
	if s == nil || s.refs > 0 {
		return
	}
	delete(a.entities, id.index)
	a.generation[id.index]++
	a.freeIDs = append(a.freeIDs, id.index)
	for _, h := range s.onRelease {
		if !h.closed {
			h.closed = true
			h.f(a)
		}
	}
	for _, sub := range s.owned {
		sub.Close()
	}
	a.observers.Remove(id)
	a.eventListeners.RemoveWhere(func(k eventKey) bool { return k.entity == id })
	for _, w := range a.windows {
		delete(w.renderedEntities, id)
	}
}

// noteAccess records id as rendered by the window currently drawing.
func (a *App) noteAccess(id EntityID) {
	if w := a.drawing; w != nil {
		if w.nextEntities == nil {
			w.nextEntities = make(map[EntityID]struct{})
		}
		w.nextEntities[id] = struct{}{}
	}
}

// EntityView adapts an entity whose *T renders into a window root. The view
// holds a strong reference (a Clone), released by Window.Close.
func EntityView[T any, P interface {
	*T
	EntityRoot
}](e Entity[T]) EntityRoot {
	return entityView[T, P]{e: e.Clone()}
}

// entityView is the EntityRoot returned by EntityView.
type entityView[T any, P interface {
	*T
	EntityRoot
}] struct {
	e Entity[T]
}

// Render reads the entity (recording it for Notify-driven redraws) and
// renders it; a released entity renders an empty Div.
func (v entityView[T, P]) Render(w *Window, cx *App) IntoElement {
	st := v.e.Read(cx)
	if st == nil {
		return NewDiv()
	}
	return P(st).Render(w, cx)
}

// releaseView drops the view's strong reference.
func (v entityView[T, P]) releaseView(cx *App) { v.e.Release(cx) }

// viewReleaser is implemented by roots that own an entity reference.
type viewReleaser interface{ releaseView(cx *App) }

// scheduleRelease requests that flushEffects release the entity's slot.
func (a *App) scheduleRelease(id EntityID) {
	a.pendingRelease = append(a.pendingRelease, id)
}
