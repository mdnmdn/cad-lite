package gpui

import "reflect"

// Context is the per-entity view handed to Render and Update. It carries the
// App plus the id of the entity it was created for, so callbacks can reach
// state without capturing an Entity directly.
type Context[T any] struct {
	App    *App
	entity EntityID
}

// Notify schedules this entity's observers to run during the next effect
// flush, which is how a view asks its window to redraw.
func (c *Context[T]) Notify() {
	if c == nil || c.App == nil {
		return
	}
	c.App.Notify(c.entity)
}

// Entity returns a handle to the entity this context belongs to.
func (c *Context[T]) Entity() Entity[T] {
	return Entity[T]{id: c.entity, app: c.App}
}

// WeakEntity returns a non-owning handle to this entity.
func (c *Context[T]) WeakEntity() WeakEntity[T] {
	return WeakEntity[T]{id: c.entity, app: c.App}
}

// Defer schedules f to run at the end of the current effect cycle, after the
// enclosing App.Update body has returned.
func (c *Context[T]) Defer(f func(*App)) {
	if c == nil || c.App == nil {
		return
	}
	c.App.Defer(f)
}

// ObserveOn registers f to run when e is notified. Activation is immediate
// for the POC: the subscription is live as soon as ObserveOn returns, rather
// than being deferred to the next effect flush as in GPUI. It is a
// package-level function because Go methods cannot introduce type
// parameters of their own.
func ObserveOn[T any](a *App, e Entity[T], f func(Entity[T], *App)) *Subscription {
	if a == nil {
		return NewSubscription(nil)
	}
	if f == nil {
		return NewSubscription(nil)
	}
	id := e.id
	return a.observers.Insert(id, func(notified EntityID, app *App) {
		f(Entity[T]{id: id, app: app}, app)
	})
}

// Emit queues an event for delivery to the entity's event listeners during
// the next effect flush. It is a package-level function because Go methods
// cannot introduce type parameters of their own.
func Emit[T, E any](c *Context[T], event *E) {
	if c == nil || c.App == nil || event == nil {
		return
	}
	c.App.pendingEffects = append(c.App.pendingEffects, effect{
		kind:   effectEmit,
		entity: c.entity,
		event:  event,
		typ:    reflect.TypeOf((*E)(nil)).Elem(),
	})
}

// Observe registers f to run when other is notified. Activation is immediate
// for the POC: the subscription is live as soon as Observe returns, rather
// than being deferred to the next effect flush as in GPUI.
func (c *Context[T]) Observe(other Entity[T], f func(Entity[T], *App)) *Subscription {
	if c == nil || c.App == nil {
		return NewSubscription(nil)
	}
	return ObserveOn[T](c.App, other, f)
}

// Subscribe registers f to run when an E event is emitted on e, using the
// given App. It is a package-level function because Go methods cannot
// introduce type parameters of their own.
func Subscribe[T, E any](cx *App, e Entity[T], f func(*E)) *Subscription {
	if cx == nil || f == nil {
		return NewSubscription(nil)
	}
	key := eventKey{entity: e.id, typ: reflect.TypeOf((*E)(nil)).Elem()}
	return cx.eventListeners.Insert(key, func(payload any) {
		ev, ok := payload.(*E)
		if !ok {
			return
		}
		f(ev)
	})
}

// Task is a handle to work scheduled with Spawn. For the POC the work is not
// run on a separate goroutine: Spawn defers it to the end of the current
// effect cycle, and Cancel simply prevents it from running. Real background
// execution is out of scope.
type Task struct {
	cancel func()
	done   bool
}

// Cancel prevents the task's work from running if it has not already run or
// been cancelled. It is a no-op on a nil or finished Task.
func (t *Task) Cancel() {
	if t == nil || t.done {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.done = true
}

// Detach abandons the handle: the work still runs (unless cancelled before
// Detach), and later Cancel calls are no-ops.
func (t *Task) Detach() {
	if t == nil {
		return
	}
	t.cancel = nil
	t.done = true
}

// Spawn schedules f to run via Defer at the end of the current effect cycle
// and returns a Task that can cancel it. See Task for the POC's simplifying
// deviations.
func (c *Context[T]) Spawn(f func()) *Task {
	t := &Task{}
	cancelled := false
	t.cancel = func() { cancelled = true }
	c.Defer(func(*App) {
		if t.done {
			return
		}
		t.done = true
		if !cancelled && f != nil {
			f()
		}
	})
	return t
}

// Listener adapts a state-owning callback into the plain callback shape used
// by window dispatch. The returned function upgrades the weak entity handle
// at invoke time and no-ops when the entity is gone. This is the Go
// replacement for Rust's borrow-checker guarantee that closures capture a
// live view: never hold a *T across a call that can re-enter the entity.
// It is a package-level function because Go methods cannot introduce type
// parameters of their own.
func Listener[T, E any](c *Context[T], f func(state *T, ev *E, w *Window, cx *App)) func(*E, *Window, *App) {
	if c == nil {
		return func(*E, *Window, *App) {}
	}
	weak := c.WeakEntity()
	return func(ev *E, w *Window, cx *App) {
		if f == nil {
			return
		}
		ent, ok := weak.Upgrade()
		if !ok {
			return
		}
		app := cx
		if app == nil {
			app = ent.app
		}
		st := ent.Read(app)
		if st == nil {
			return
		}
		f(st, ev, w, app)
	}
}
