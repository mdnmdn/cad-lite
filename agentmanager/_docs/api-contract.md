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
// Without MCPServer only the exact title "mcp__server__tool" matches; Kind must be nil, other or read.
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
var ErrLineTooLong error                       // an inbound line exceeded the cap: the reader stops, Done closes, pending calls fail
func (c *Conn) Err() error                     // why the reader stopped (ErrLineTooLong or a read error); nil on EOF; valid after Done
func WithFrameLogger(func(dir string, line []byte)) Option // dir "in"|"out"
func WithMaxLine(n int) Option                 // inbound line cap in bytes (default 64 MiB; n <= 0 keeps the default)
```
Writer rule: unbounded queue, one goroutine owns the writer, `SendRaw` never blocks. After a write error the writer goroutine exits, closes the writer, drops the queue and fails pending calls with `ErrClosed`; later sends return `ErrClosed`. A `Handler` for
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
(switch on `cfg.Harness`; when `cfg.Binary` is empty it resolves the harness executable on the effective PATH of `shellenv`, and the child's environment gets that PATH);
`func Available(h agentmanager.Harness) (path string, ok bool)` (looks the harness executable up on the effective PATH of `shellenv`);
`type Found struct{ Harness agentmanager.Harness; Path, Version string }`; `func Discover(ctx context.Context, path string) []Found`
(one entry per harness, OpenCode, Claude, Codex in that order, with `Path` empty when not found and `Version` the first line of `--version` when it answers within 3 s).

Package `shellenv` (stdlib only; the PATH a GUI process lacks): `type Source string` (`SourceLoginShell`, `SourceRegistry`, `SourceApp`);
`type Probe struct{ Path string; Source Source; Err error }`; `func LoginPATH(ctx context.Context) (string, error)` (Unix: `$SHELL -l -i -c` printing `$PATH` between unique markers,
5 s timeout, stdin closed; Windows: the Machine and User `Path` from the registry via `reg query`, `%VAR%` expanded);
`func Run(ctx context.Context) Probe` (LoginPATH plus the source; on failure `Path` is empty, `Source` is `SourceApp` and `Err` says why);
`func Merge(appPATH, loginPATH string) string` (login entries first, then the app's, de-duplicated, empty entries dropped);
`func Added(appPATH, loginPATH string) []string` (login entries the app's PATH lacks); `type Binary struct{ Name, Path string }`;
`func Find(names []string, path string) []Binary` (the names found, with full paths, in order; on Windows `PATHEXT` is tried);
`func Lookup(name, path string) (string, bool)`, `func Resolve(name string) (string, bool)` (effective PATH, then `exec.LookPath`);
`func Set(loginPATH string)`, `func Effective() string` (the process PATH merged with what `Set` stored), `func Env() []string` (`["PATH=" + Effective()]`, or nil before `Set`).

Backend constructors (each owner): `acp.OpenCode() agentmanager.Opener`, `acp.New(Command{Path string; Args, Env []string}) agentmanager.Opener`,
`claude.New() agentmanager.Opener`, `codex.New() agentmanager.Opener`. Each Opener's `Open` spawns,
handshakes (ACP, Codex), returns a Session; MCP injection and Close semantics as in section 2.


## Session errors

`agentmanager.ErrClosed` is returned by every `Session` method after `Close` or after the agent process exits. Backends wrap or return it as is; `jsonrpc.ErrClosed` stays internal to the connection layer.

## ToolCallPatch.MCPServer

`ToolCallPatch` carries `MCPServer string`: the owning MCP server when the backend knows it. Backends set it (Claude from `mcp__server__tool`, Codex from MCP tool-call items, ACP from the tool call's MCP metadata or title). `AllowMCP` matches `MCPServer` only when it is set; without it only the exact Claude title `mcp__server__tool` counts. A call whose `Kind` is not `other` or `read` is never auto-allowed. ACP derives `MCPServer` strictly: `mcp__server__tool`, or opencode's `<server>_<tool>` with the tool in `[a-z0-9_]+`, and never for execute/edit/delete/move/fetch kinds.

## Package `store` (session persistence)

A directory-based store of conversations, stdlib plus `agentmanager` and `agentmanager/transcript`. One
session is one file `<dir>/<id>.jsonl` (dir 0700, files 0600), one JSON line per item. A line is the
envelope `{"k":kind,"t":RFC3339Nano time,"d":payload}`; kinds are `meta` (first line: id, harness,
created), `user`, `agent_session`, `permission_resolved` and one per `agentmanager.Event` (`session_started`,
`text`, `thought`, `tool_start`, `tool_update`, `permission`, `plan`, `turn_end`, `log`, `exited`). Event
payloads are the event structs' default JSON (no tags on the frozen types); an empty `json.RawMessage`
reads back as nil. Unknown kinds and unparsable lines are skipped, so a truncated last line costs only that line.

```go
// Meta describes one stored session; List returns it newest first (Updated, then ID).
type Meta struct {
	ID, Harness  string  // Harness is an agentmanager.Harness value
	AgentSessionID string // backend resume id, "" until the backend reported one
	Title        string  // first user prompt, whitespace collapsed, at most 80 runes
	Created, Updated time.Time // Updated is the file's modification time
}
var ErrNotFound error

type Store struct{ /* unexported */ }
func Open(dir string) (*Store, error)     // does not touch the disk; the directory is created by Create
func (s *Store) Dir() string
func (s *Store) List() ([]Meta, error)    // unreadable files are skipped; a missing dir is an empty list
func (s *Store) Create(harness agentmanager.Harness) (*Record, error)
func (s *Store) Load(id string) (*Record, error)   // ErrNotFound when absent
func (s *Store) Latest() (*Record, error)          // ErrNotFound when the store is empty
func (s *Store) Delete(id string) error

// OpenSet is the sessions a host had open (tabs) and the active one. It lives in `<dir>/open.json`
// (0600, written atomically through a temporary file), separate from the session files.
type OpenSet struct {
	IDs    []string // session ids in tab order
	Active string   // one of IDs, or ""
}
func (s *Store) SaveOpen(o OpenSet) error
func (s *Store) LoadOpen() (OpenSet, error) // ErrNotFound when never saved; ids whose session is gone are dropped

// Record is one open session; methods are goroutine-safe. The file opens on the first write, a write
// after a truncated line starts a new line first, and TurnEnd, Exited and Close fsync.
type Record struct{ /* unexported */ }
func (r *Record) Meta() Meta
func (r *Record) Append(ev agentmanager.Event) error  // SessionStarted also sets AgentSessionID
func (r *Record) AddUser(text string) error
func (r *Record) ResolvePermission(requestID string) error
func (r *Record) SetAgentSessionID(id string) error
func (r *Record) Items() []any                        // loaded and appended items: agentmanager.Event, store.User, store.Resolved
func (r *Record) Replay(t *transcript.T)              // rebuilds a transcript; an unfinished turn ends with a cancelled TurnEnd
func (r *Record) Close() error

type User struct{ Text string }
type Resolved struct{ RequestID string }
func MarshalEvent(ev agentmanager.Event) ([]byte, error)    // envelope line without the newline
func UnmarshalEvent(line []byte) (agentmanager.Event, error)
```
