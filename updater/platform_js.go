//go:build js

package updater

// Supported reports whether this platform can check for and install updates;
// the browser build cannot.
func Supported() bool { return false }
