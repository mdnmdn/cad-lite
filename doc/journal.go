package doc

// This file adds the undo-journal hooks the UNDO command family builds on: marks (UNDO Mark and
// Back), groups (UNDO Begin and End, and any command that wants several transactions to be one
// step) and counted undo and redo. They only reorganise steps the journal already records.

// Mark records the current undo depth as a marker UndoToMark can return to. Markers are a stack:
// each Mark pushes one. A marker the history has since undone past is dropped.
func (d *Drawing) Mark() {
	j := &d.journal
	j.marks = append(j.marks, len(j.undo))
}

// HasMark reports whether a marker is set.
func (d *Drawing) HasMark() bool { return len(d.journal.marks) > 0 }

// UndoToMark undoes every step committed since the latest marker, removes that marker and
// returns how many steps it undid. It returns 0 and does nothing when no marker is set or a
// transaction or group is open.
func (d *Drawing) UndoToMark() int {
	j := &d.journal
	if len(j.marks) == 0 || d.open != nil || j.groups > 0 {
		return 0
	}
	m := j.marks[len(j.marks)-1]
	n := 0
	for len(j.undo) > m && d.Undo() {
		n++
	}
	j.marks = j.marks[:len(j.marks)-1]
	return n
}

// BeginGroup starts a group: the transactions committed until the matching EndGroup become one
// undo step. Groups nest; only the outermost EndGroup merges.
func (d *Drawing) BeginGroup() {
	j := &d.journal
	if j.groups == 0 {
		j.groupStart = len(j.undo)
	}
	j.groups++
}

// EndGroup closes the innermost group; closing the outermost merges the steps committed since
// BeginGroup into one. An unmatched call does nothing.
func (d *Drawing) EndGroup() {
	j := &d.journal
	if j.groups == 0 {
		return
	}
	j.groups--
	if j.groups > 0 || j.groupStart >= len(j.undo) {
		return
	}
	steps := j.undo[j.groupStart:]
	if len(steps) > 1 {
		merged := JournalStep{Before: map[Handle]Entity{}, After: map[Handle]Entity{}}
		for _, s := range steps {
			for h, b := range s.Before {
				if _, ok := merged.Before[h]; !ok {
					merged.Before[h] = b
				}
			}
			for h, a := range s.After {
				merged.After[h] = a
			}
		}
		j.undo = append(j.undo[:j.groupStart], merged)
	}
	j.dropMarks()
}

// InGroup reports whether a group is open.
func (d *Drawing) InGroup() bool { return d.journal.groups > 0 }

// UndoN undoes up to n steps and returns how many it undid.
func (d *Drawing) UndoN(n int) int {
	done := 0
	for done < n && d.Undo() {
		done++
	}
	return done
}

// RedoN redoes up to n steps and returns how many it redid.
func (d *Drawing) RedoN(n int) int {
	done := 0
	for done < n && d.Redo() {
		done++
	}
	return done
}

// UndoDepth returns the number of steps that can be undone.
func (d *Drawing) UndoDepth() int { return len(d.journal.undo) }

// dropMarks forgets markers deeper than the current undo depth.
func (j *Journal) dropMarks() {
	for len(j.marks) > 0 && j.marks[len(j.marks)-1] > len(j.undo) {
		j.marks = j.marks[:len(j.marks)-1]
	}
}
