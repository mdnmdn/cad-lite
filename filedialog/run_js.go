//go:build js

package filedialog

import "context"

// run reports ErrUnavailable: a browser build cannot run programs.
func run(context.Context, request) (string, error) { return "", ErrUnavailable }
