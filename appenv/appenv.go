// Package appenv reads Cad-lite's environment variables. Every variable is named
// CADLITE_<NAME>; the name it had while the project was called go-draw,
// GODRAW_<NAME>, is still read when the new one is not set. It is a leaf
// package: pure Go, no imports from the rest of Cad-lite.
package appenv

import "os"

const (
	prefix       = "CADLITE_"
	legacyPrefix = "GODRAW_"
)

// Lookup returns the value of CADLITE_<name>, else of GODRAW_<name>, and
// whether either is set. A variable set to the empty string counts as set.
func Lookup(name string) (string, bool) {
	if v, ok := os.LookupEnv(prefix + name); ok {
		return v, true
	}
	return os.LookupEnv(legacyPrefix + name)
}

// Get returns Lookup's value, or "" when neither variable is set. A variable
// that is set but empty shadows the legacy name, so CADLITE_X= switches an
// inherited GODRAW_X off.
func Get(name string) string {
	v, _ := Lookup(name)
	return v
}
