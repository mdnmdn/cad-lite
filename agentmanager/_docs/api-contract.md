# agentmanager API contract

Edit this file before changing any exported signature. Decisions: nested module `agentmanager`
(stdlib only); the host's default policy auto-allows the `cad` MCP server (`AllowMCP("cad")`) and
asks for everything else; `render_png` is deferred; harness shell/file tools get no permissive mode
and opencode/codex run sandboxed read-only where the protocol allows.

**Deviations from the plan's section 4.3 as scaffolded** (the Go source is authoritative):
- `agentmanager.Queue` / `NewQueue` (Push, Events, Close) added: the unbounded event queue WP1 owns.
- `jsonrpc.WithRawHandler(func(line []byte)) Option` and `jsonrpc.Option` signature fixed.
- `jsonrpc.Error` has json tags (`code`, `message`, `data`).
- `internal/fakeagent` entry points are `RunACP`, `RunClaude`, `RunCodex` (three `Run` would clash in one package).
- `transcript` imports `agentmanager` for the event and tool types.
- `internal/proc` API is new (see `internal/proc/proc.go`).

# Frozen API (package `agentmanager`; parallel packages compile against this)

```go
// Package agentmanager launches AI coding agents (opencode, Claude Code, Codex) in structured
// I/O mode and exposes them through one backend-neutral session API.
package agentmanager

import ("context"; "encoding/json")

// Harness names a supported agent CLI.
type Harness string

const (
	OpenCode Harness = "opencode"
	Claude   Harness = "claude"
	Codex    Harness = "codex"
)

// Config is everything needed to start one agent session.
type Config struct {
	Harness      Harness
	Binary       string            // executable; "" = harness default ("opencode","claude","codex")
	Args         []string          // extra argv appended after the managed flags
	Cwd          string            // absolute project dir
	Model        string            // "" = harness default
	ResumeID     string            // backend session/thread id to resume, "" = new
	SystemPrompt string            // claude: --append-system-prompt; codex: -c developer_instructions; acp: prefixed to first prompt
	MCPServers   []MCPServer       // injected per harness (section 3)
	Env          []string          // extra KEY=VALUE for the child
	AutoApprove  PermissionPolicy  // optional; nil = every request goes to the UI
	Logger       Logger            // optional; nil = discard
}

// MCPServer is a streamable-HTTP MCP server to inject into the agent.
type MCPServer struct{ Name, URL, Token string }

// Logger receives trace output (raw frames at Debug; never log tokens).
type Logger interface{ Debugf(format string, args ...any) }

// PermissionPolicy may answer a request itself; ok=false sends it to the UI.
type PermissionPolicy func(PermissionRequest) (optionID string, ok bool)

// Session is one live agent conversation. Methods are goroutine-safe.
type Session interface {
	// ID is the backend's session/thread id ("" until SessionStarted).
	ID() string
	// Events delivers events in order; it is closed after the final Exited event.
	// The consumer must keep draining; backends buffer (unbounded) and never block the child.
	Events() <-chan Event
	// Prompt starts a turn; returns once the prompt is written. Error if a turn is running.
	Prompt(ctx context.Context, text string) error
	// Cancel aborts the running turn (pending permissions are answered cancelled first); the
	// session stays usable. The turn ends with TurnEnd{Cancelled}. No-op when idle.
	Cancel() error
	// RespondPermission answers a PermissionRequest by option id; Cancelled=true answers "cancelled".
	RespondPermission(requestID string, ans PermissionAnswer) error
	// Close cancels, closes stdin, waits <=10s, kills, joins goroutines. Idempotent.
	Close() error
}

// Opener starts sessions; every backend package exports one.
type Opener interface{ Open(ctx context.Context, cfg Config) (Session, error) }

// PermissionAnswer selects an option or cancels.
type PermissionAnswer struct {
	OptionID  string
	Cancelled bool
}

// Event is a closed sum type; consumers type-switch on the concrete types below.
type Event interface{ isEvent() }

type SessionStarted struct{ SessionID, Model string }
type TextDelta struct{ MessageID, Text string }     // append to the message with this id ("" = current)
type ThoughtDelta struct{ MessageID, Text string }
type ToolCallStart struct{ Call ToolCall }
type ToolCallUpdate struct{ Update ToolCallPatch }
type PermissionRequest struct {
	RequestID string
	Call      ToolCallPatch // what is being authorised (ID, Title, Kind, RawInput set where known)
	Options   []PermissionOption
}
type PlanUpdate struct{ Entries []PlanEntry }
type TurnEnd struct {
	Reason StopReason
	Err    string // set when Reason == StopFailed
}
type Log struct{ Level, Message string } // "debug","info","warn","error"
// Exited is always the last event: the process is gone (Err != "" if unexpected; includes stderr tail).
type Exited struct{ Err string }

type ToolKind string
const (KindRead ToolKind = "read"; KindEdit="edit"; KindDelete="delete"; KindMove="move"; KindSearch="search";
	KindExecute="execute"; KindThink="think"; KindFetch="fetch"; KindOther="other")
type ToolStatus string
const (ToolPending ToolStatus = "pending"; ToolInProgress="in_progress"; ToolCompleted="completed"; ToolFailed="failed")

// ToolCall is a tool invocation as announced.
type ToolCall struct {
	ID, Title           string
	Kind                ToolKind
	Status              ToolStatus
	RawInput            json.RawMessage
	MCPServer, MCPTool  string // set when the call targets an MCP server (best effort)
}
// ToolCallPatch is a partial update: nil/empty = unchanged; Content replaces.
type ToolCallPatch struct {
	ID        string
	Title     *string
	Kind      *ToolKind
	Status    *ToolStatus
	Output    *string          // human-readable result text, when known
	RawInput  json.RawMessage
	RawOutput json.RawMessage
}
type PermissionOption struct{ ID, Name string; Kind PermissionKind }
type PermissionKind string
const (AllowOnce PermissionKind = "allow_once"; AllowAlways="allow_always"; RejectOnce="reject_once"; RejectAlways="reject_always")
type PlanEntry struct{ Content, Status string } // status: pending|in_progress|completed
type StopReason string
const (StopEndTurn StopReason = "end_turn"; StopCancelled="cancelled"; StopFailed="failed";
	StopMaxTokens="max_tokens"; StopMaxTurns="max_turn_requests"; StopRefusal="refusal")

// AllowMCP returns a PermissionPolicy that allows (allow_once preferred) requests whose call has MCPServer==server.
func AllowMCP(server string) PermissionPolicy
```
`isEvent()` methods are defined in `types.go` (WP1). Backends fill `MCPServer`/`MCPTool` where they can
(Claude: parse `mcp__srv__tool`; codex: `mcpToolCall.server/tool`; ACP: unverified parse of title/rawInput, best effort).

Package `jsonrpc` (WP1, frozen):
```go
// Message is any JSON-RPC frame: request (ID+Method), notification (Method), response (ID+Result/Error).
type Message struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`     // number or string, echoed verbatim
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}
type Error struct{ Code int; Message string; Data json.RawMessage }
func (e *Error) Error() string
const (CodeParse=-32700; CodeInvalidRequest=-32600; CodeMethodNotFound=-32601; CodeInvalidParams=-32602; CodeInternal=-32603)

// Handler receives inbound traffic. Calls come from the single reader goroutine; HandleRequest MUST
// return promptly or hand the reply to another goroutine via Conn.Reply (permission asks do this).
type Handler interface {
	HandleRequest(c *Conn, id json.RawMessage, method string, params json.RawMessage) (reply bool)
	HandleNotification(c *Conn, method string, params json.RawMessage)
}
// Conn is a newline-delimited JSON-RPC 2.0 peer over a reader/writer pair.
type Conn struct{ /* unexported */ }
func NewConn(r io.Reader, w io.Writer, h Handler, opts ...Option) *Conn // starts reader + writer goroutines
func (c *Conn) Call(ctx context.Context, method string, params, result any) error   // allocates int64 id, registers before write, honours ctx
func (c *Conn) Notify(method string, params any) error
func (c *Conn) CallAsync(method string, params any, done func(result json.RawMessage, err *Error)) (id int64, err error) // for session/prompt: no blocking
func (c *Conn) Reply(id json.RawMessage, result any) error
func (c *Conn) ReplyError(id json.RawMessage, code int, msg string) error
func (c *Conn) SendRaw(v any) error            // arbitrary line (claude control_response etc.)
func (c *Conn) CloseWrite() error              // flush queue then close the writer (stdin EOF)
func (c *Conn) Done() <-chan struct{}          // reader exited (EOF); pending calls fail with ErrClosed
var ErrClosed error
func WithFrameLogger(func(dir string, line []byte)) Option // dir "in"|"out"
```
Writer rule: unbounded queue, one goroutine owns the writer, `SendRaw` never blocks. A `Handler` for
non-JSON-RPC protocols (Claude) may be `nil`-requests plus a `WithRawHandler(func(line []byte))` option
so the Claude backend reuses the framing/writer without the JSON-RPC semantics.

Package `mcp` (WP2, frozen):
```go
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema object
	Handler     func(ctx context.Context, args json.RawMessage) (Result, error) // error -> isError result with err.Error()
}
type Content struct{ Type string; Text string; Data string; MIMEType string } // Type "text" | "image"
type Result struct{ Content []Content; IsError bool; Structured any }
func Text(s string) Result
func JSON(v any) Result                 // one text content holding compact JSON
func Image(png []byte) Content          // base64, image/png
func Errorf(format string, a ...any) Result // IsError=true

type Options struct{ Name, Version, Instructions string; Serialize *bool /* default true */; Logger func(string) }
type Server struct{ /* unexported */ }
func NewServer(o Options) *Server
func (s *Server) Register(t Tool)       // before or after Start; tools/list reflects current set
func (s *Server) Start() (Endpoint, error) // binds 127.0.0.1:0 and generates the token
func (s *Server) Close() error
type Endpoint struct{ URL, Token string }  // URL = http://127.0.0.1:P/mcp
```
(To keep `mcp` independent of the root package, the conversion is `agentmanager.MCPServer{Name:n,URL:e.URL,Token:e.Token}` written by the caller.)

Package `transcript` (WP2): `type Block interface{}`; concrete `UserMsg{Text}`, `AgentMsg{ID,Text}`, `Thought{ID,Text}`,
`ToolBlock{Call ToolCall; Status; Output}`, `PermissionBlock{RequestID; Call; Options; Answered bool}`, `Plan{Entries}`,
`Note{Level,Text}`. `type T struct`; `func (t *T) AddUser(text string)`, `Apply(ev Event)`,
`Blocks() []Block`, `Version() uint64`, `Running() bool`, `ResolvePermission(requestID string)`. Pure, no goroutines.

Package `harnesses` (WP1 stub, finalised after WP3-5): `func Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error)`
(switch on `cfg.Harness`); `func Available(h agentmanager.Harness) (path string, ok bool)` (`exec.LookPath`).

Backend constructors (each owner): `acp.OpenCode() agentmanager.Opener`, `acp.New(Command{Path string; Args, Env []string}) agentmanager.Opener`,
`claude.New() agentmanager.Opener`, `codex.New() agentmanager.Opener`. Each Opener's `Open` spawns,
handshakes (ACP, Codex), returns a Session; MCP injection and Close semantics as in section 2.


## Session errors

`agentmanager.ErrClosed` is returned by every `Session` method after `Close` or after the agent process exits. Backends wrap or return it as is; `jsonrpc.ErrClosed` stays internal to the connection layer.
