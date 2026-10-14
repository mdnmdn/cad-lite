package gpui

// FocusID identifies a focusable element within one App. The zero value
// means "no focus".
type FocusID uint64

// FocusHandle is a refcounted, copyable handle to a focusable element. The
// zero value is invalid; obtain one from App.NewFocusHandle.
//
// Copies of a handle share one FocusID. Entity-style refcounting is handled
// by the App's focusMap; for the POC handles are not released on drop, only
// when explicitly released, so ids stay valid for the App's lifetime.
type FocusHandle struct {
	id       FocusID
	app      *App
	tabIndex int
	tabStop  bool
}

// ID returns the handle's focus id.
func (h *FocusHandle) ID() FocusID {
	if h == nil {
		return 0
	}
	return h.id
}

// IsValid reports whether the handle refers to a live focus id.
func (h *FocusHandle) IsValid() bool {
	return h != nil && h.id != 0 && h.app != nil
}

// Focus moves the window's focus to this handle. It is a no-op on a nil or
// invalid handle, or a nil window.
func (h *FocusHandle) Focus(w *Window) {
	if w == nil || !h.IsValid() {
		return
	}
	w.RequestFocus(h)
}

// Blur removes this handle's focus from the window, if it holds it. It is a
// no-op on a nil handle or a nil window.
func (h *FocusHandle) Blur(w *Window) {
	if w == nil || !h.IsValid() {
		return
	}
	if w.focus == h.id {
		w.Blur()
	}
}

// IsFocused reports whether the window's focus is exactly this handle.
func (h *FocusHandle) IsFocused(w *Window) bool {
	if w == nil || !h.IsValid() {
		return false
	}
	return w.focus == h.id
}

// ContainsFocused reports whether this handle or any descendant is focused.
// The POC has no focus-parent chain, so this is equivalent to IsFocused.
func (h *FocusHandle) ContainsFocused(w *Window) bool { return h.IsFocused(w) }

// SetTabIndex sets the handle's position in tab-order traversal and
// returns h, so it chains like GPUI's .tab_index(i).
func (h *FocusHandle) SetTabIndex(i int) *FocusHandle {
	if h != nil {
		h.tabIndex = i
	}
	return h
}

// SetTabStop sets whether Tab traversal may stop at this handle and returns
// h, so it chains like GPUI's .tab_stop(b).
func (h *FocusHandle) SetTabStop(b bool) *FocusHandle {
	if h != nil {
		h.tabStop = b
	}
	return h
}

// IsFocusVisible reports whether the handle is focused and the window's
// last input was the keyboard (CSS :focus-visible).
func (h *FocusHandle) IsFocusVisible(w *Window) bool {
	return h.IsFocused(w) && w.focusVisible
}

// TabIndex returns the handle's tab-order position.
func (h *FocusHandle) TabIndex() int {
	if h == nil {
		return 0
	}
	return h.tabIndex
}

// TabStop reports whether Tab traversal may stop at this handle.
func (h *FocusHandle) TabStop() bool {
	if h == nil {
		return false
	}
	return h.tabStop
}

// focusMap is the App's FocusID -> refcount table. It is a plain map so the
// App can allocate one directly; ids are handed out by probing upward from 1
// for the first unused slot, which is fine at POC scale.
type focusMap map[FocusID]int

// alloc registers a fresh id with refcount 1 and returns it.
func (m focusMap) alloc() FocusID {
	var id FocusID = 1
	for {
		if _, taken := m[id]; !taken {
			m[id] = 1
			return id
		}
		id++
	}
}

// release drops one refcount, deleting the id when it hits zero and
// reporting whether that happened.
func (m focusMap) release(id FocusID) bool {
	if id == 0 {
		return false
	}
	n, ok := m[id]
	if !ok {
		return false
	}
	if n <= 1 {
		delete(m, id)
		return true
	}
	m[id] = n - 1
	return false
}

// alive reports whether the id currently has any references.
func (m focusMap) alive(id FocusID) bool {
	if id == 0 {
		return false
	}
	_, ok := m[id]
	return ok
}
