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
// POC refcounting deviation: Entity copies do NOT bump a refcount. Entities
// live until the App is torn down or releaseEntity is called explicitly
// (from flushEffects when a release was requested). WeakEntity.Upgrade
// therefore fails only after such an explicit release, not when the last
// Entity copy is dropped.
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
	cx := &Context[T]{App: a, entity: id}
	v := build(cx)
	a.entities[idx] = &entitySlot{
		value:      &v,
		generation: gen,
		refs:       1,
		typeID:     reflect.TypeOf((*T)(nil)).Elem(),
	}
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
// re-enter the same entity; prefer Context.Listener for callbacks.
func (e Entity[T]) Read(cx *App) *T {
	app := cx
	if app == nil {
		app = e.app
	}
	s := app.slot(e.id)
	if s == nil {
		return nil
	}
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
	f(st, &Context[T]{App: cx, entity: e.id})
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

// releaseEntity frees the slot and bumps its generation so stale ids fail
// their generation check. It is idempotent: releasing an already-dead id is
// a no-op.
func (a *App) releaseEntity(id EntityID) {
	s := a.slot(id)
	if s == nil {
		return
	}
	s.refs--
	if s.refs > 0 {
		return
	}
	delete(a.entities, id.index)
	a.generation[id.index]++
	a.freeIDs = append(a.freeIDs, id.index)
}

// scheduleRelease requests that flushEffects release the entity's slot.
func (a *App) scheduleRelease(id EntityID) {
	a.pendingRelease = append(a.pendingRelease, id)
}
