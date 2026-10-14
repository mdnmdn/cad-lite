// Package agentmanager launches AI coding agents (opencode, Claude Code, Codex) in structured
// I/O mode and exposes them through one backend-neutral session API.
//
// The package defines the neutral types (Config, Session, Event and friends). Backends live in
// the acp, claude and codex packages; harnesses.Open picks one by Harness id. The module imports
// the standard library only and never imports go-draw.
package agentmanager
