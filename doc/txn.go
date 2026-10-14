package doc

import (
	"errors"
	"slices"
)

// ChangeSet lists the handles a commit, undo or redo touched.
type ChangeSet struct {
	Added, Modified, Erased []Handle
}

// Empty reports whether the change set touches nothing.
func (c ChangeSet) Empty() bool { return len(c.Added)+len(c.Modified)+len(c.Erased) == 0 }

// ErrTxnOpen is returned by Begin while another transaction is open.
var ErrTxnOpen = errors.New("doc: a transaction is already open")

// Txn is an open transaction. Every modification of a drawing happens inside
// one; Commit records it in the journal as a single undo step.
type Txn struct {
	d *Drawing
	// before holds each touched entity's state at its first touch; nil for
	// an entity the transaction added.
	before map[Handle]Entity
	order  []Handle
	done   bool
}

// Begin opens a transaction. Only one may be open at a time.
func (d *Drawing) Begin() (*Txn, error) {
	if d.open != nil {
		return nil, ErrTxnOpen
	}
	t := &Txn{d: d, before: map[Handle]Entity{}}
	d.open = t
	return t, nil
}

func (t *Txn) touch(h Handle) {
	if _, ok := t.before[h]; ok {
		return
	}
	t.order = append(t.order, h)
	if e := t.d.entities[h]; e != nil {
		t.before[h] = e.Clone().(Entity)
	} else {
		t.before[h] = nil
	}
}

// Add inserts e into model space on its layer (the current layer when
// unset) and returns its new handle. It adds nothing and returns 0 for an
// *Unknown, which only a file reader may create.
func (t *Txn) Add(e Entity) Handle {
	if _, ok := e.(*Unknown); ok {
		return 0
	}
	c := e.Common()
	c.H = t.d.allocHandle()
	c.Owner = ModelSpace
	if c.Layer == "" {
		c.Layer = t.d.CurrentLayer
	}
	if c.Color == 0 {
		c.Color = ByLayer
		if t.d.CurrentColor >= 1 && t.d.CurrentColor <= 255 {
			c.Color = t.d.CurrentColor
		}
	}
	t.touch(c.H)
	t.d.insert(e)
	return c.H
}

// Modify snapshots the entity h, then calls fn to change it in place.
// It does nothing when h does not exist or is an *Unknown, which is never
// editable.
func (t *Txn) Modify(h Handle, fn func(Entity)) {
	e := t.d.entities[h]
	if _, unknown := e.(*Unknown); e == nil || unknown {
		return
	}
	t.touch(h)
	fn(e)
}

// Erase removes the entity h. It does nothing for an *Unknown.
func (t *Txn) Erase(h Handle) {
	e := t.d.entities[h]
	if _, unknown := e.(*Unknown); e == nil || unknown {
		return
	}
	t.touch(h)
	t.d.remove(h)
}

// Commit closes the transaction, records it in the journal and publishes
// its change set. A transaction that changed nothing records nothing.
func (t *Txn) Commit() ChangeSet {
	if t.done {
		return ChangeSet{}
	}
	t.done = true
	t.d.open = nil
	step := JournalStep{Before: make(map[Handle]Entity, len(t.order)), After: make(map[Handle]Entity, len(t.order))}
	var cs ChangeSet
	for _, h := range t.order {
		b, a := t.before[h], t.d.entities[h]
		switch {
		case b == nil && a == nil:
			continue
		case b == nil:
			cs.Added = append(cs.Added, h)
		case a == nil:
			cs.Erased = append(cs.Erased, h)
		default:
			cs.Modified = append(cs.Modified, h)
		}
		step.Before[h] = b
		if a != nil {
			step.After[h] = a.Clone().(Entity)
		} else {
			step.After[h] = nil
		}
	}
	if !cs.Empty() {
		t.d.journal.record(step)
	}
	t.d.publish(cs)
	return cs
}

// Abort closes the transaction and restores every touched entity.
func (t *Txn) Abort() {
	if t.done {
		return
	}
	t.done = true
	t.d.open = nil
	var cs ChangeSet
	for _, h := range slices.Backward(t.order) {
		cs = t.d.restore(h, t.before[h], cs)
	}
	t.d.publish(cs)
}

// AbortOpen aborts the open transaction, if any, restoring the entities it touched. A host
// uses it to recover after a panic left a transaction open.
func (d *Drawing) AbortOpen() {
	if d.open != nil {
		d.open.Abort()
	}
}

// Touched reports whether the transaction has changed anything yet.
func (t *Txn) Touched() bool { return len(t.order) > 0 }

// restore puts state (nil = absent) back for h and records the kind of
// change in cs.
func (d *Drawing) restore(h Handle, state Entity, cs ChangeSet) ChangeSet {
	_, had := d.entities[h]
	switch {
	case state == nil && had:
		d.remove(h)
		cs.Erased = append(cs.Erased, h)
	case state != nil && !had:
		d.insert(state.Clone().(Entity))
		cs.Added = append(cs.Added, h)
	case state != nil:
		d.insert(state.Clone().(Entity))
		cs.Modified = append(cs.Modified, h)
	}
	return cs
}

// JournalStep is one undoable unit: each touched handle's state before and
// after (nil = absent).
type JournalStep struct {
	Before, After map[Handle]Entity
	// Name is the command that made the step, when the session recorded it (NameLastStep).
	Name string
}

// Journal is the undo/redo history of committed transactions.
type Journal struct {
	undo, redo []JournalStep
	// marks are undo depths recorded by Mark, oldest first.
	marks []int
	// groups is the BeginGroup nesting depth; groupStart is the undo depth the outermost group
	// began at.
	groups, groupStart int
}

func (j *Journal) record(s JournalStep) {
	j.undo = append(j.undo, s)
	j.redo = j.redo[:0]
}

// CanUndo reports whether there is a step to undo.
func (j *Journal) CanUndo() bool { return len(j.undo) > 0 }

// CanRedo reports whether there is a step to redo.
func (j *Journal) CanRedo() bool { return len(j.redo) > 0 }

// Undo reverts the latest committed step and reports whether there was one.
func (d *Drawing) Undo() bool {
	j := &d.journal
	if d.open != nil || len(j.undo) == 0 {
		return false
	}
	s := j.undo[len(j.undo)-1]
	j.undo = j.undo[:len(j.undo)-1]
	j.redo = append(j.redo, s)
	j.dropMarks()
	d.apply(s.Before)
	return true
}

// Redo re-applies the latest undone step and reports whether there was one.
func (d *Drawing) Redo() bool {
	j := &d.journal
	if d.open != nil || len(j.redo) == 0 {
		return false
	}
	s := j.redo[len(j.redo)-1]
	j.redo = j.redo[:len(j.redo)-1]
	j.undo = append(j.undo, s)
	d.apply(s.After)
	return true
}

func (d *Drawing) apply(states map[Handle]Entity) {
	hs := make([]Handle, 0, len(states))
	for h := range states {
		hs = append(hs, h)
	}
	slices.Sort(hs)
	var cs ChangeSet
	for _, h := range hs {
		cs = d.restore(h, states[h], cs)
	}
	d.publish(cs)
}
