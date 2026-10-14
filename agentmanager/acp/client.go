// Package acp is a generic Agent Client Protocol (v1) client backend; OpenCode is a preset.
package acp

import (
	"context"

	"agentmanager"
)

// Command is the agent process to launch.
type Command struct {
	Path string
	Args []string
	Env  []string
}

type opener struct{ cmd Command }

// New returns an Opener that launches cmd and speaks ACP over its stdio.
func New(cmd Command) agentmanager.Opener { return opener{cmd} }

// Open implements agentmanager.Opener.
func (o opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	panic("todo: WP3")
}
