package cmdsys

import "cad-lite/doc"

// This file shows a command's work while the command still runs. A command's edits stay in its
// open transaction until it ends (one command, one undo step), and the drawing publishes a change
// set only at the commit. AutoCAD shows each LINE segment, OFFSET copy or TRIM cut the moment it
// is made, and snaps and picks see it, so after every Start and Input the session applies the
// transaction's new edits to the spatial index itself and queues them for the host's display
// cache (TakeLiveEdits). Nothing is published or journalled: the commit still reports the whole
// command as one change set, which every listener applies as before.

// syncInFlight brings the spatial index, the selection and the queue of live edits up to date
// with the edits the open transaction made since the last call.
func (s *Session) syncInFlight() {
	t := s.ctx.txn
	if t == nil {
		return
	}
	hs := t.TakeEdits()
	if len(hs) == 0 {
		return
	}
	d := s.ctx.Doc
	seen := make(map[doc.Handle]bool, len(hs))
	var cs doc.ChangeSet
	for _, h := range hs {
		if seen[h] {
			continue
		}
		seen[h] = true
		// The index files a Modified handle it does not hold as an addition.
		if d.Entity(h) != nil {
			cs.Modified = append(cs.Modified, h)
		} else {
			cs.Erased = append(cs.Erased, h)
		}
	}
	s.ctx.Index.Apply(d, cs)
	s.ctx.Selection.Prune(d)
	s.prev.prune(d)
	s.live.Modified = append(s.live.Modified, cs.Modified...)
	s.live.Erased = append(s.live.Erased, cs.Erased...)
}

// commitOpen commits the command's open transaction, if any, after applying its last edits to
// the index: an entity indexed in an earlier step and erased in this one is missing from the
// commit's change set (it was added and erased by the same transaction), so only the sync removes
// it. The commit's change set does not re-apply to the index what the sync just applied, and the
// live-edit queue keeps only what the change set does not report to the drawing's listeners.
// Every commit of a command's transaction goes through here (finish, unwindFrames,
// Context.CommitStep).
func (s *Session) commitOpen() doc.ChangeSet {
	t := s.ctx.txn
	if t == nil {
		return doc.ChangeSet{}
	}
	s.syncInFlight()
	s.committing = true
	cs := t.Commit()
	s.committing = false
	s.ctx.txn = nil
	s.dropLive(cs)
	return cs
}

// abortOpen aborts the command's open transaction, if any, after the same sync, so the restoring
// change set and the index agree.
func (s *Session) abortOpen() {
	if t := s.ctx.txn; t != nil {
		s.syncInFlight()
		t.Abort()
		s.ctx.txn = nil
	}
}

// dropLive forgets the queued live edits that cs reports: the drawing's listeners (the host's
// display cache among them) receive cs itself.
func (s *Session) dropLive(cs doc.ChangeSet) {
	if cs.Empty() || len(s.live.Modified)+len(s.live.Erased) == 0 {
		return
	}
	in := make(map[doc.Handle]bool, len(cs.Added)+len(cs.Modified)+len(cs.Erased))
	for _, hs := range [][]doc.Handle{cs.Added, cs.Modified, cs.Erased} {
		for _, h := range hs {
			in[h] = true
		}
	}
	keep := func(hs []doc.Handle) []doc.Handle {
		out := hs[:0]
		for _, h := range hs {
			if !in[h] {
				out = append(out, h)
			}
		}
		return out
	}
	s.live.Modified, s.live.Erased = keep(s.live.Modified), keep(s.live.Erased)
}

// TakeLiveEdits returns the entities the running command added, changed or erased since the
// previous call, and forgets them. The host drops their cached display items before drawing (a
// render.Builder's Invalidate), so an entity a command edits before it ends draws as it is now.
// Added entities are listed as Modified. The commit's change set reaches the drawing's listeners
// as usual, and the queue drops what it reports; what it cannot report (an entity the command
// added and erased) stays queued until taken.
func (s *Session) TakeLiveEdits() doc.ChangeSet {
	cs := s.live
	s.live = doc.ChangeSet{}
	return cs
}
