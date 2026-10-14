package agentmanager

import "encoding/json"

// Event is a closed sum type; consumers type-switch on the concrete types below.
type Event interface{ isEvent() }

// SessionStarted reports the backend session/thread id once it is known.
type SessionStarted struct{ SessionID, Model string }

// TextDelta appends to the message with this id ("" = current).
type TextDelta struct{ MessageID, Text string }

// ThoughtDelta appends to the reasoning block with this id ("" = current).
type ThoughtDelta struct{ MessageID, Text string }

// ToolCallStart announces a tool invocation.
type ToolCallStart struct{ Call ToolCall }

// ToolCallUpdate patches a previously announced tool call.
type ToolCallUpdate struct{ Update ToolCallPatch }

// PermissionRequest asks the user to authorise a tool call; answer with Session.RespondPermission.
type PermissionRequest struct {
	RequestID string
	Call      ToolCallPatch // what is being authorised (ID, Title, Kind, RawInput set where known)
	Options   []PermissionOption
}

// PlanUpdate replaces the agent's plan.
type PlanUpdate struct{ Entries []PlanEntry }

// TurnEnd ends the running turn.
type TurnEnd struct {
	Reason StopReason
	Err    string // set when Reason == StopFailed
}

// Log is a diagnostic line; Level is "debug", "info", "warn" or "error".
type Log struct{ Level, Message string }

// Exited is always the last event: the process is gone (Err != "" if unexpected; includes stderr tail).
type Exited struct{ Err string }

func (SessionStarted) isEvent()    {}
func (TextDelta) isEvent()         {}
func (ThoughtDelta) isEvent()      {}
func (ToolCallStart) isEvent()     {}
func (ToolCallUpdate) isEvent()    {}
func (PermissionRequest) isEvent() {}
func (PlanUpdate) isEvent()        {}
func (TurnEnd) isEvent()           {}
func (Log) isEvent()               {}
func (Exited) isEvent()            {}

// ToolKind classifies a tool call.
type ToolKind string

// Tool kinds.
const (
	KindRead    ToolKind = "read"
	KindEdit    ToolKind = "edit"
	KindDelete  ToolKind = "delete"
	KindMove    ToolKind = "move"
	KindSearch  ToolKind = "search"
	KindExecute ToolKind = "execute"
	KindThink   ToolKind = "think"
	KindFetch   ToolKind = "fetch"
	KindOther   ToolKind = "other"
)

// ToolStatus is the lifecycle state of a tool call.
type ToolStatus string

// Tool statuses.
const (
	ToolPending    ToolStatus = "pending"
	ToolInProgress ToolStatus = "in_progress"
	ToolCompleted  ToolStatus = "completed"
	ToolFailed     ToolStatus = "failed"
)

// ToolCall is a tool invocation as announced.
type ToolCall struct {
	ID, Title          string
	Kind               ToolKind
	Status             ToolStatus
	RawInput           json.RawMessage
	MCPServer, MCPTool string // set when the call targets an MCP server (best effort)
}

// ToolCallPatch is a partial update: nil/empty = unchanged; Content replaces.
type ToolCallPatch struct {
	ID        string
	Title     *string
	Kind      *ToolKind
	Status    *ToolStatus
	Output    *string // human-readable result text, when known
	RawInput  json.RawMessage
	RawOutput json.RawMessage
}

// PermissionOption is one answer the user can pick; ID is opaque and sent back verbatim.
type PermissionOption struct {
	ID, Name string
	Kind     PermissionKind
}

// PermissionKind is a hint about what a PermissionOption does.
type PermissionKind string

// Permission option kinds.
const (
	AllowOnce    PermissionKind = "allow_once"
	AllowAlways  PermissionKind = "allow_always"
	RejectOnce   PermissionKind = "reject_once"
	RejectAlways PermissionKind = "reject_always"
)

// PlanEntry is one step of the agent's plan; Status is pending, in_progress or completed.
type PlanEntry struct{ Content, Status string }

// StopReason says why a turn ended.
type StopReason string

// Stop reasons.
const (
	StopEndTurn   StopReason = "end_turn"
	StopCancelled StopReason = "cancelled"
	StopFailed    StopReason = "failed"
	StopMaxTokens StopReason = "max_tokens"
	StopMaxTurns  StopReason = "max_turn_requests"
	StopRefusal   StopReason = "refusal"
)
