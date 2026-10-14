//go:build !unix

package agentinstall

import "testing"

// alive is never asked outside Unix: the tests that use it skip there (the fake mise is a shell
// script).
func alive(*testing.T, string) bool { return false }
