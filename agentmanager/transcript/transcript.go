// Package transcript folds agentmanager events into a flat list of chat blocks. It is pure: no
// goroutines, no I/O, not goroutine-safe.
package transcript

import "agentmanager"

// Block is one chat item; it is one of the concrete types in this package.
type Block interface{}

// UserMsg is text the user sent.
type UserMsg struct{ Text string }

// AgentMsg is an agent message being streamed.
type AgentMsg struct{ ID, Text string }

// Thought is agent reasoning text.
type Thought struct{ ID, Text string }

// ToolBlock is a tool call with its latest status and output.
type ToolBlock struct {
	Call   agentmanager.ToolCall
	Status agentmanager.ToolStatus
	Output string
}

// PermissionBlock is a permission ask; Answered is set by ResolvePermission.
type PermissionBlock struct {
	RequestID string
	Call      agentmanager.ToolCallPatch
	Options   []agentmanager.PermissionOption
	Answered  bool
}

// Plan is the agent's current plan.
type Plan struct{ Entries []agentmanager.PlanEntry }

// Note is a diagnostic or status line; Level is "debug", "info", "warn" or "error".
type Note struct{ Level, Text string }

// T is a transcript. The zero value is ready to use.
type T struct{ _ struct{} }

// AddUser appends a UserMsg and marks a turn as running.
func (t *T) AddUser(text string) { panic("todo: WP2") }

// Apply folds one event into the transcript.
func (t *T) Apply(ev agentmanager.Event) { panic("todo: WP2") }

// Blocks returns the blocks in order. The slice must not be modified.
func (t *T) Blocks() []Block { panic("todo: WP2") }

// Version increases on every change, for cheap redraw checks.
func (t *T) Version() uint64 { panic("todo: WP2") }

// Running reports whether a turn is in progress.
func (t *T) Running() bool { panic("todo: WP2") }

// ResolvePermission marks the PermissionBlock with this request id as answered.
func (t *T) ResolvePermission(requestID string) { panic("todo: WP2") }
