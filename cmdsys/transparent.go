package cmdsys

// Transparent is implemented by commands that may run inside another command, typed with a
// leading ' at a prompt (ZOOM, PAN, LAYER, DIST, ID, SETVAR and the other view and settings
// commands). A transparent command must not open a transaction. The marker is all that exists
// today: the session does not yet push a frame, print ">>" and resume the interrupted prompt, so
// a ' typed at a prompt is still an invalid response. At Command: the prefix is stripped and any
// command runs.
type Transparent interface {
	// Transparent reports true for a command that may run at another command's prompt.
	Transparent() bool
}

// IsTransparent reports whether c may run transparently.
func IsTransparent(c Command) bool {
	t, ok := c.(Transparent)
	return ok && t.Transparent()
}
