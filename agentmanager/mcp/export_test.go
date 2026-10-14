package mcp

import "time"

// SetWriteTimeout shortens the server's write timeout for a test and returns the restore function.
// Servers started afterwards use it.
func SetWriteTimeout(d time.Duration) func() {
	old := writeTimeout.Swap(int64(d))
	return func() { writeTimeout.Store(old) }
}

// ReplyMargin exposes replyMargin to tests.
func ReplyMargin(wt time.Duration) time.Duration { return replyMargin(wt) }
