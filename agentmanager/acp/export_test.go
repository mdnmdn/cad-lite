package acp

import "time"

// SetHandshakeTimeout shortens the handshake wait for a test and returns the restore func.
func SetHandshakeTimeout(d time.Duration) func() {
	old := handshakeTimeout
	handshakeTimeout = d
	return func() { handshakeTimeout = old }
}

// SetCloseGrace shortens the wait for the agent to exit after stdin closes.
func SetCloseGrace(d time.Duration) func() {
	old := closeGrace
	closeGrace = d
	return func() { closeGrace = old }
}
