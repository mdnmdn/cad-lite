// Package codex is the Codex app-server (JSON-RPC) backend.
package codex

import (
	"context"

	"agentmanager"
)

type opener struct{}

// New returns the Opener for `codex app-server --listen stdio://`.
func New() agentmanager.Opener { return opener{} }

// Open implements agentmanager.Opener.
func (opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	panic("todo: WP5")
}
