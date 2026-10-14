package codex

import "time"

// SetHandshakeTimeout shortens the handshake timeout for a test and returns a restore func.
func SetHandshakeTimeout(d time.Duration) func() {
	old := handshakeTimeout
	handshakeTimeout = d
	return func() { handshakeTimeout = old }
}
