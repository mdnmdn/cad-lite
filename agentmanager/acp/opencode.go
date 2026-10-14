package acp

import "agentmanager"

// OpenCode returns the Opener for `opencode acp`.
func OpenCode() agentmanager.Opener { return New(Command{Path: "opencode", Args: []string{"acp"}}) }
