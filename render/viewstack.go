package render

// MaxSavedViews is how many previous views ZOOM Previous can step back through, as in AutoCAD.
const MaxSavedViews = 10

// ViewStack is the stack of previous views behind ZOOM Previous. It stores the camera (Center and
// Scale) only: the viewport size is the window's, not the view's.
//
// A command that changes the view calls Save with the view as it is, changes it, then calls
// Settle with the result. Settle remembers what the view looked like when the command left it, so
// that a change made elsewhere since (the mouse wheel, a drag pan, a resize that fits) is noticed
// by the next Save or Pop and saved as a step of its own.
type ViewStack struct {
	views   []View
	last    View
	hasLast bool
}

// History returns v's stack of previous views, creating it on first use. The stack lives in the
// view, so it is shared by every copy of the pointer a session hands its commands.
func (v *View) History() *ViewStack {
	if v.Saved == nil {
		v.Saved = &ViewStack{}
	}
	return v.Saved
}

func sameCamera(a, b View) bool { return a.Center == b.Center && a.Scale == b.Scale }

func (s *ViewStack) push(v View) {
	v.Saved = nil
	if n := len(s.views); n > 0 && sameCamera(s.views[n-1], v) {
		return
	}
	s.views = append(s.views, v)
	if len(s.views) > MaxSavedViews {
		s.views = s.views[len(s.views)-MaxSavedViews:]
	}
}

// reconcile saves the view the last command left when the view has moved on since.
func (s *ViewStack) reconcile(cur View) {
	if s.hasLast && !sameCamera(s.last, cur) {
		s.push(s.last)
	}
}

// Save records cur as a previous view; call it just before the view changes.
func (s *ViewStack) Save(cur View) {
	s.reconcile(cur)
	s.push(cur)
}

// Settle records cur as the view a command left; call it just after the view changed.
func (s *ViewStack) Settle(cur View) { cur.Saved = nil; s.last, s.hasLast = cur, true }

// Pop returns the most recent previous view that differs from cur and removes it, or false when
// there is none.
func (s *ViewStack) Pop(cur View) (View, bool) {
	s.reconcile(cur)
	for len(s.views) > 0 {
		v := s.views[len(s.views)-1]
		s.views = s.views[:len(s.views)-1]
		if !sameCamera(v, cur) {
			return v, true
		}
	}
	return View{}, false
}

// Len returns the number of saved views.
func (s *ViewStack) Len() int { return len(s.views) }

// Restore sets v's camera to the saved view's, keeping v's viewport size.
func (v *View) Restore(saved View) { v.Center, v.Scale = saved.Center, saved.Scale }
