// Package claude is the Claude Code stream-json backend.
package claude

import (
	"context"

	"agentmanager"
)

type opener struct{}

// New returns the Opener for `claude -p` in stream-json mode.
func New() agentmanager.Opener { return opener{} }

// Open implements agentmanager.Opener.
func (opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	panic("todo: WP4")
}
