package doc

import (
	"errors"
	"slices"
	"strings"
)

// Group associates independent model-space entities for collective selection.
type Group struct {
	// H is the persistent group object handle.
	H Handle
	// Name is the unique drawing-local name; Description is user text.
	Name, Description string
	// Unnamed records an automatic name; Selectable enables selection expansion.
	Unnamed, Selectable bool
	// Members contains independent model-space entity handles.
	Members []Handle
}

func (g *Group) clone() *Group { c := *g; c.Members = slices.Clone(g.Members); return &c }
func cloneGroups(gs map[Handle]*Group) map[Handle]*Group {
	out := map[Handle]*Group{}
	for h, g := range gs {
		out[h] = g.clone()
	}
	return out
}

// Groups returns copies of groups in handle order.
func (d *Drawing) Groups() []*Group {
	var hs []Handle
	for h := range d.groups {
		hs = append(hs, h)
	}
	slices.Sort(hs)
	var out []*Group
	for _, h := range hs {
		out = append(out, d.groups[h].clone())
	}
	return out
}

// Group returns a copy of a group by name, case-insensitively.
func (d *Drawing) Group(name string) *Group {
	for _, g := range d.groups {
		if strings.EqualFold(g.Name, name) {
			return g.clone()
		}
	}
	return nil
}

// GroupsFor returns copies of groups containing h.
func (d *Drawing) GroupsFor(h Handle) []*Group {
	var out []*Group
	for _, g := range d.Groups() {
		if slices.Contains(g.Members, h) {
			out = append(out, g)
		}
	}
	return out
}

// ExpandGroups includes every member connected through selectable groups.
func (d *Drawing) ExpandGroups(hs []Handle) []Handle {
	out := slices.Clone(hs)
	seen := map[Handle]bool{}
	for _, h := range out {
		seen[h] = true
	}
	for i := 0; i < len(out); i++ {
		for _, g := range d.GroupsFor(out[i]) {
			if !g.Selectable {
				continue
			}
			for _, h := range g.Members {
				if !seen[h] {
					seen[h] = true
					out = append(out, h)
				}
			}
		}
	}
	return out
}

// AddGroup creates an undoable named or unnamed group.
func (t *Txn) AddGroup(g *Group) (Handle, error) {
	c := g.clone()
	if c.Name == "" {
		c.Unnamed = true
		c.Name = "*A" + t.d.Header.HandSeed.String()
	}
	if t.d.Group(c.Name) != nil {
		return 0, errors.New("group name already exists")
	}
	if err := t.validateGroup(c); err != nil {
		return 0, err
	}
	t.touchGroups()
	c.H = t.d.allocHandle()
	t.d.groups[c.H] = c
	return c.H, nil
}
func (t *Txn) validateGroup(g *Group) error {
	if strings.TrimSpace(g.Name) == "" {
		return errors.New("group name is empty")
	}
	seen := map[Handle]bool{}
	var hs []Handle
	for _, h := range g.Members {
		e := t.d.Entity(h)
		if e == nil {
			return errors.New("group member does not exist")
		}
		if _, ok := e.(*Unknown); ok {
			return errors.New("unsupported group member")
		}
		if !seen[h] {
			hs = append(hs, h)
			seen[h] = true
		}
	}
	g.Members = hs
	return nil
}

// ModifyGroup validates and changes a copy of the group atomically.
func (t *Txn) ModifyGroup(h Handle, fn func(*Group)) error {
	g := t.d.groups[h]
	if g == nil {
		return errors.New("group does not exist")
	}
	c := g.clone()
	fn(c)
	c.H = h
	if err := t.validateGroup(c); err != nil {
		return err
	}
	for oh, o := range t.d.groups {
		if oh != h && strings.EqualFold(o.Name, c.Name) {
			return errors.New("group name already exists")
		}
	}
	t.touchGroups()
	t.d.groups[h] = c
	return nil
}

// EraseGroup dissolves a group without erasing its members.
func (t *Txn) EraseGroup(h Handle) {
	if t.d.groups[h] != nil {
		t.touchGroups()
		delete(t.d.groups, h)
	}
}

// LoadGroup installs a group from a file, outside the undo journal.
func (d *Drawing) LoadGroup(g *Group) {
	c := g.clone()
	if c.H == 0 {
		c.H = d.allocHandle()
	}
	if c.H >= d.Header.HandSeed {
		d.Header.HandSeed = c.H + 1
	}
	d.groups[c.H] = c
}
func (t *Txn) touchGroups() {
	if t.groupsBefore == nil {
		t.groupsBefore = cloneGroups(t.d.groups)
	}
}
