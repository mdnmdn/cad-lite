# Agent manager port: plan for parallel implementation

Raw planning material (inbox). Source: Rust crate `ubiq/crates/agent-manager` (read-only clone at
`/home/user/mdnmdn/ubiq`). Goal from the user: add an AI agent to go-draw using ACP (adapted for
opencode), inject an MCP server that controls the drawing app, and also support Claude Code and
Codex through their own protocols. New requirement: the whole agent manager is an isolated,
reusable tree (`agentmanager/`) that imports nothing from go-draw; go-draw glue lives outside it.

Facts marked **(crate)** were read from the Rust source or its docs. Facts marked **(unverified)**
come from the CLIs' public behaviour or memory and are not exercised by the crate; the fake agents
must encode them and a gated live test must confirm them. The crate itself says only Grok was ever
pinned against a live ACP agent; opencode over ACP was captured in a `_docs/wip` file that is not
in the clone.

Machine check (`which`): only `claude` exists (`/opt/node22/bin/claude`, 2.1.299, newer than the
2.1.258-2.1.283 the crate verified). No `opencode`, no `codex`. Everything must be testable with fakes.

---------------------------------------------------------------------------------------------------

## 1. What the crate does, what we take, what we drop

The crate is a harness manager: resolve a `RunSpec` (profile, account, skills, MCPs, policy) ->
provision an ephemeral config dir -> launch the harness either on a PTY (passthrough) or on piped
stdio with a protocol bridge (`IoBridge`) -> normalise every harness to one neutral
`AgentEvent`/`AgentInput` model that is ACP's `session/update` vocabulary. Per-harness bridges:
`io/jsonl.rs` (Claude stream-json), `io/codex.rs` (codex app-server JSON-RPC),
`io/acp_client.rs` (generic ACP client; opencode, grok, copilot). `mcp/server.rs` is a tiny
loopback HTTP MCP server. Ubiq's `ubiq-host/src/conversation.rs` pumps a bridge on a thread, maps
events onto a UI bus, answers permissions through a detached input sink.

**Take (the structured I/O path only):**
- neutral event model + session handle (events out, prompt/cancel/answer in);
- three bridges: ACP client, Claude stream-json, Codex app-server;
- in-process MCP server on loopback, injected per harness;
- the lifecycle rules: reader started before the handshake, every blocking wait has a timeout, a
  dedicated writer so nothing blocks on stdin, every inbound request gets exactly one reply,
  cancel answers pending permissions first, stdin-close then bounded wait then kill.

**Leave out, explicitly:** profiles and accounts, credentials/keyring/`SecretStore`, registry and
catalogs, skills and MCP-as-skill, hooks, `RunSpec` resolve/provision/overlay/isolate/confinement,
quotas and rate-limit gauges, usage/cost/context-ring accounting, PTY passthrough and the TUI/CLI,
session recording and GC, model/effort discovery (`opencode models`, `codex debug models`), the
other harnesses (copilot, grok, gemini, cursor, kiro, ...), subagent/delegate attribution
(`Origin`), AG-UI output adapter, ACP *server* direction, `terminal/*` and `elicitation/*` client
capabilities, login flows (we assume the CLI is already authenticated), Windows ConPTY.

**Deliberate deviations from the crate:**
1. MCP server requires a bearer token and checks Host/Origin (crate: none, any local process can call it).
2. MCP `initialize` negotiates the protocol version (crate: hard-coded `2024-11-05`).
3. We advertise no ACP `fs` capability (crate advertises `fs.readTextFile/writeTextFile` and
   serves them confined to cwd). The agents have their own file tools and a CAD app needs none.
4. Streaming deltas for Claude via `--include-partial-messages` (crate maps whole messages only).
5. Go idiom: `Events() <-chan Event` + context, not blocking `next_event` on a pump thread.

---------------------------------------------------------------------------------------------------

## 2. The three wire protocols

All three are newline-delimited JSON, one compact object per line, UTF-8, no `Content-Length`.
Readers must skip blank and non-JSON lines (stderr noise and banners happen) and must tolerate
objects that lack `"jsonrpc":"2.0"` (codex responses omit it). Lines can be large (tool output):
use a `bufio.Reader` with `ReadBytes('\n')`, not a `Scanner` with the default 64 KiB limit.

### 2.1 ACP (Agent Client Protocol v1, JSON-RPC 2.0 over stdio) - used for opencode (crate: `io/acp_client.rs`)

Both peers are client and server with independent id spaces. Demultiplex each line by shape, in
this order: (1) has `id` and (`result` or `error`) -> response to our request; (2) has `id` and
`method` -> inbound request, we MUST reply exactly once; (3) `method`, no `id` -> notification.
The `result`/`error` test comes first because the agent may send a request whose id collides with
one of ours.

Launch: `opencode acp` (crate: argv is exactly `["acp", ...passthrough]`; no `--model`, `--session`,
prompt or permission-skip flag). cwd = project dir. Stdin/stdout piped, stderr captured to a ring
buffer for error messages. Env hygiene: none required. Optional `OPENCODE_DISABLE_CLAUDE_CODE=1`
to stop opencode reading `~/.claude` (crate docs list it; unverified need).

Handshake (reader goroutine already running; each wait is `HANDSHAKE_TIMEOUT` = 30 s):

```
-> {"jsonrpc":"2.0","id":1,"method":"initialize","params":{
     "protocolVersion":1,                       # integer, not string
     "clientInfo":{"name":"go-draw","version":"0"},
     "clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false}}}
<- {"id":1,"result":{"protocolVersion":1,
     "agentCapabilities":{"loadSession":true,
        "promptCapabilities":{"image":true,"audio":false,"embeddedContext":true},
        "mcpCapabilities":{"http":true,"sse":true}},
     "authMethods":[...]}}
```
Abort with a clear error if `result.protocolVersion != 1`. Record `loadSession`, `promptCapabilities`
(text and resource_link are always accepted; down-convert image/audio/resource to text when not
advertised) and `mcpCapabilities`. `authenticate {"methodId":...}` is sent only when
`result._meta.defaultAuthMethodId` exists (crate); otherwise skip. If `session/new` then fails with
code `-32000` ("auth required"), report "authenticate the CLI first (`opencode auth login`)" naming
the advertised `authMethods` ids.

```
-> {"id":2,"method":"session/new","params":{
     "cwd":"/abs/project",                      # absolute
     "mcpServers":[{"type":"http","name":"cad","url":"http://127.0.0.1:PORT/mcp",
                    "headers":[{"name":"Authorization","value":"Bearer TOKEN"}]}]}}
<- {"id":2,"result":{"sessionId":"sess_abc", "models":{...}?, "modes":{...}?, "configOptions":[...]?}}
```
`mcpServers` is always an array, never null. A remote server entry is sent only when
`mcpCapabilities.http` (or `sse`) is true; stdio entries are `{"name","command","args","env":[{name,value}]}`
with no `type`. Resume: `session/load {"sessionId","cwd","mcpServers"}` (only if `loadSession`); the
agent replays history as `session/update` notifications before responding, and the result has no
sessionId (reuse ours).

Prompt: `session/prompt` is a long-lived request that returns when the whole turn ends. Do NOT
block on it: allocate id, store it in a single "turn slot" BEFORE writing, write, return. One turn at
a time: a second prompt while the slot is set is an error. The response with that id ends the turn.

```
-> {"id":7,"method":"session/prompt","params":{"sessionId":"sess_abc","prompt":[{"type":"text","text":"draw a 50x30 rectangle"}]}}
<- {"id":7,"result":{"stopReason":"end_turn"}}   # end_turn|max_tokens|max_turn_requests|refusal|cancelled
<- {"id":7,"error":{"code":-32603,"message":"..."}}   # kills the TURN only: emit TurnEnd{Failed, err}; session stays usable
```
Live prompts are never echoed back (opencode sends `user_message_chunk` only while replaying a
loaded session): the UI draws the user's text itself and the client drops `user_message_chunk`.

Streaming: notification `session/update`, payload under `params.update` (fall back to `params`
itself if `update` is absent), discriminator `sessionUpdate` (snake_case values, camelCase keys):

| `sessionUpdate` | fields | maps to |
|---|---|---|
| `agent_message_chunk` | `content:{type:"text",text}`, `messageId?` | `TextDelta` |
| `agent_thought_chunk` | same | `ThoughtDelta` |
| `user_message_chunk` | same | drop (replay only) |
| `tool_call` | `toolCallId,title,kind,status,content[],locations[],rawInput` | `ToolCallStart` |
| `tool_call_update` | `toolCallId` + any of `title,kind,status,content,locations,rawOutput` | `ToolCallUpdate` (absent field = unchanged; `content`/`locations` REPLACE) |
| `plan` | `entries[{content,priority,status}]` | `PlanUpdate` (optional) |
| `available_commands_update`, `current_mode_update`, `config_option_update`, `session_info_update`, `usage_update` | - | ignore (first release) |

`kind`: read, edit, delete, move, search, execute, think, fetch, switch_mode, other (unknown -> other).
`status`: pending, in_progress, completed, failed. Tool content items: `{"type":"content","content":{ContentBlock}}`,
`{"type":"diff","path","oldText","newText"}`, `{"type":"terminal","terminalId"}`.
opencode specifics (**unverified**): its `task` tool arrives as kind `think`; `todowrite` completion
carries the todo list in `rawInput` (opencode never sends ACP `plan`); MCP tools are named
`<server>_<tool>` in `title`/`rawInput` (e.g. `cad_add_line`).

Permissions: inbound request from the agent, which blocks until we reply.

```
<- {"id":"p1","method":"session/request_permission","params":{"sessionId":"sess_abc",
     "toolCall":{"toolCallId":"call_1","title":"...","kind":"edit","status":"pending","rawInput":{...}},
     "options":[{"optionId":"once","name":"Allow once","kind":"allow_once"},
                {"optionId":"always","name":"Always allow","kind":"allow_always"},
                {"optionId":"reject","name":"Reject","kind":"reject_once"}]}}   # ids above: unverified, treat as opaque
-> {"jsonrpc":"2.0","id":"p1","result":{"outcome":{"outcome":"selected","optionId":"once"}}}
-> {"jsonrpc":"2.0","id":"p1","result":{"outcome":{"outcome":"cancelled"}}}
```
`outcome` appears twice on purpose. Echo the raw `id` (number or string). Key the outstanding table
by `n:<num>`/`s:<str>` so `7` and `"7"` differ. Kinds: allow_once, allow_always, reject_once,
reject_always; `kind` is a hint, `optionId` is what is sent. If the request cannot be parked
(table error, no event consumer) reply `cancelled` immediately. A turn end drops (without replying)
whatever is still outstanding: the agent already retired those ids.

Cancel (spec order): reply `cancelled` to every outstanding permission FIRST, then send the
notification `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"sess_abc"}}` (no id,
no response). Keep reading: the agent sends more updates and MUST end the turn with
`stopReason:"cancelled"`. Close = close stdin (after cancelling pending permissions), wait <= 10 s
for exit, then kill; join reader and writer.

Every other inbound request (`terminal/*`, `elicitation/create`, `fs/*` since we do not advertise
it, `_vendor/...`) -> `{"id":..,"error":{"code":-32601,"message":"method not found"}}`.
Optional later: `session/set_config_option {sessionId,configId,value}`, `session/set_model {sessionId,modelId}`,
`session/set_mode {sessionId,modeId}` for the model picker.

Concurrency rules (crate, all six guards): all waits use timeouts (handshake 30 s, mid-session
request 10 s); `session/prompt` never blocks the caller; a single writer goroutine owns stdin
(unbounded queue) so the reader, which answers inbound requests itself, can never block in a write;
every inbound request gets exactly one reply on every path; when the reader exits it fails all
pending calls and emits the final `Exited` event; teardown takes no mutex.

**opencode adaptation = configuration, not a different protocol:** launch `opencode acp`, inject
MCP through the `session/new` `mcpServers` entry (preferred; crate's `AcpBridge::with_mcp_servers`
exists for adapters whose only per-run route is the wire). Fallback when the agent does not advertise
`mcpCapabilities.http`: env `OPENCODE_CONFIG_CONTENT={"mcp":{"cad":{"type":"remote","url":"...","headers":{"Authorization":"Bearer T"},"enabled":true}}}`
(crate: opencode config shape; merged over user/project config; no file written) with `mcpServers:[]`.
Permissions are real round trips (no `--dangerously-skip-permissions`). Model selection by
`session/set_config_option`, never argv. The same client with a different `Command` serves any other
ACP agent, so the package is `acp` with an `OpenCode` preset, not `opencode`.

### 2.2 Claude Code (stream-json NDJSON; crate: `io/jsonl.rs`, `harness/claude.rs`, `_docs/harness/claude-code.md`)

Launch (crate argv, structured mode):

```
claude -p --output-format stream-json --input-format stream-json --verbose \
  --permission-prompt-tool stdio --disallowedTools AskUserQuestion \
  --mcp-config <path-or-json> --strict-mcp-config \
  [--model M] [--resume SESSION_ID] [--permission-mode MODE] [--include-partial-messages] \
  [--append-system-prompt TEXT]
```
- `--permission-prompt-tool stdio` is what makes Claude ASK: without it `-p` auto-denies gated tools
  and emits no `control_request`. `--permission-mode` is ignored in `-p` (reports `default`).
  2.1.299 adds `--permission-prompts host|none`; "host" should be the default, (unverified) confirm live.
- `--strict-mcp-config` = only our MCP file's servers. Write `{"mcpServers":{...}}` to a 0600 temp
  file (a token in argv shows in `ps`); delete on close.
- Env: remove `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`, `CLAUDE_CODE_SESSION_ID`,
  `CLAUDE_CODE_SSE_PORT` so a launch from inside Claude does not inherit the parent.
- MCP config entry for our server: `{"mcpServers":{"cad":{"type":"http","url":"http://127.0.0.1:PORT/mcp","headers":{"Authorization":"Bearer T"}}}}`.
  Claude names the tools `mcp__cad__<tool>`. Optionally pre-allow with `--allowedTools mcp__cad`
  (unverified interplay with the prompt tool); the neutral `AutoApprove` hook covers it either way.

There is NO handshake. Claude emits `system/init` after it receives the first user line (the crate's
fake reads the prompt first). So `Open` returns immediately after spawn; `SessionStarted` is emitted
when init arrives. `--resume` takes the id from a previous init's `session_id`.

stdin lines:
```
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<prompt>"}]}}
{"type":"control_response","response":{"subtype":"success","request_id":"<id>","response":{"behavior":"allow","updatedInput":{}}}}
{"type":"control_response",...,"response":{"behavior":"deny","updatedInput":{}}}          # (unverified) add "message":"Denied by user"
{"type":"control_request","request_id":"gd-int-N","request":{"subtype":"interrupt","reason":"interrupt"}}
```
For an "always" choice, `response` also carries `"updatedPermissions":[<the suggestion object>]`.
The prompt line is never echoed; a prompt may be sent while a turn runs (queued by Claude), but we
enforce one turn at a time like ACP.

stdout lines (`type`):

| line | meaning -> neutral event |
|---|---|
| `{"type":"system","subtype":"init","session_id","model","tools":[..]}` | `SessionStarted` (may arrive twice per session; emit once) |
| `{"type":"system","subtype":"compact_boundary",...}` | ignore |
| `{"type":"assistant","message":{"id","content":[blocks]}}` | per block: `text` -> `TextDelta` (whole text), `thinking` -> `ThoughtDelta` (skip when empty: a bare `signature` means withheld), `tool_use{id,name,input}` -> `ToolCallStart` (status in_progress) |
| `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id","is_error","content":string\|[{type:"text",text}]}]}}` | `ToolCallUpdate` (failed if `is_error`; text = joined text blocks; raw output kept). A `tool_result` with `status:"async_launched"` stays in_progress |
| `{"type":"control_request","request_id","request":{"subtype":"can_use_tool","tool_name","display_name","input","description","permission_suggestions":[{"type":"setMode","mode":"acceptEdits","destination":"session"}],"tool_use_id"}}` | `PermissionRequest` joined to the earlier `tool_use` by `tool_use_id`; options: `allow`, one `allow_always:<n>` per setMode suggestion, `deny`. Hold outstanding until answered |
| any other `control_request` subtype | reply `{"type":"control_response","response":{"subtype":"error","request_id":..,"error":"unsupported"}}` (shape unverified) or the turn stalls |
| `{"type":"control_response",...}` | answer to our interrupt: ignore |
| `{"type":"result","subtype":"success"\|"error_max_turns"\|"error_during_execution"\|...,"is_error":bool,"result":"text","usage":{},"modelUsage":{}}` | THE turn end: `TurnEnd`: `error_max_turns` -> MaxTurnRequests; `is_error` -> Failed with `result`; else EndTurn; if we sent an interrupt for this turn -> Cancelled |
| `{"type":"log","log":{"level","message"}}` | `Log` |

Tool kind map (crate `tool_kind`): Read/NotebookRead/Glob/LS -> read; Edit/Write/NotebookEdit/MultiEdit ->
edit; Bash/BashOutput/KillShell -> execute; Grep/WebSearch -> search; WebFetch -> fetch;
Task/Agent -> other (delegate); TodoWrite/ExitPlanMode -> think; `mcp__<srv>__<tool>` -> other with
`MCPServer`/`MCPTool` filled. Title = `"<Name> <file_path|path|command|pattern|url>"`.

Partial streaming (not in the crate; flag exists in the CLI): with `--include-partial-messages` the
CLI also emits `{"type":"stream_event","event":{...Anthropic stream events...}}`: `message_start`
(`message.id`), `content_block_delta` with `delta:{type:"text_delta","text"}` or
`{type:"thinking_delta","thinking"}`, `message_stop`. Map text/thinking deltas to `TextDelta`/`ThoughtDelta`
and remember the message ids that were streamed; the complete `assistant` line follows and its
text/thinking blocks for an already-streamed id MUST be skipped (tool_use blocks are not). Ignore
`input_json_delta`. This is unverified live; the fake agent must exercise both with and without.

Cancel: deny every outstanding permission FIRST (an interrupt with an unanswered ask makes Claude fail
the tool with "Tool permission stream closed before response received"), then write the interrupt
`control_request`; stdin stays OPEN and the next prompt reaches the same process. Close: close stdin,
wait <= 10 s, kill. An unanswered permission stalls the turn forever, so Close also denies first.

### 2.3 Codex (`codex app-server --listen stdio://`, JSON-RPC; crate: `io/codex.rs`, `harness/codex.rs`, `_docs/harness/codex.md`)

Requires codex >= 0.100 (crate verified up to 0.161). No jsonrpc member on responses. Launch:

```
codex [-c key=value ...] app-server --listen stdio://      # -c overrides go BEFORE the subcommand
```
`-c key=value` takes TOML on the right (dot notation for nesting). Our injection:
```
-c 'mcp_servers.cad={url="http://127.0.0.1:PORT/mcp", bearer_token_env_var="GODRAW_MCP_TOKEN", default_tools_approval_mode="approve", tool_timeout_sec=120}'
env GODRAW_MCP_TOKEN=<token>
```
The crate writes catalog servers this way (`mcp_servers.<id>` inline table, `url` + `http_headers`)
or into `$CODEX_HOME/config.toml` as `[mcp_servers.<id>]`; `bearer_token_env_var`,
`default_tools_approval_mode`, `tool_timeout_sec` (default 60) and `startup_timeout_sec` (default 10)
are documented in the crate's codex notes but never exercised: unverified. Other useful overrides:
`model="..."`, `model_reasoning_effort="low|medium|high"`, `approval_policy="on-request"`,
`sandbox_mode="read-only"|"workspace-write"|"danger-full-access"`. Optional `CODEX_HOME` to relocate
config/auth (leave unset: use the user's login).

Handshake (timeout 10 s each; reader running first):
```
-> {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"go-draw","version":"0"},"capabilities":{"experimentalApi":true}}}
<- {"id":1,"result":{...}}
-> {"jsonrpc":"2.0","method":"initialized","params":{}}                       # notification
-> {"id":2,"method":"thread/start","params":{"cwd":"/abs/project"}}           # resume: thread/resume {"threadId","cwd","excludeTurns":true}
<- {"id":2,"result":{"thread":{"id":"t-1"},"model":"gpt-5.5","sandbox":{"type":"workspaceWrite"},...}}  # -> SessionStarted{t-1}
```
Prompt (ack returns at once; the turn runs asynchronously):
```
-> {"id":3,"method":"turn/start","params":{"threadId":"t-1","input":[{"type":"text","text":"...","text_elements":[]}]}}
<- {"id":3,"result":{"turn":{"id":"turn-1"}}}     # remember turn.id: it is the ONLY place it is stated
```
`turn/start` accepts overrides `model`, `effort`, `sandboxPolicy`, `approvalPolicy` (hold for
subsequent turns). A prompt during a live turn may be `turn/steer {threadId,input,expectedTurnId}`;
we skip steering (one turn at a time).

Notifications (every thread-scoped one has `threadId`; handle only our root thread's, drop others,
they are subagents):

| method | params | maps to |
|---|---|---|
| `item/agentMessage/delta` | `{threadId,turnId,itemId,delta}` | `TextDelta{MessageID:itemId}` |
| `item/reasoning/summaryTextDelta`, `item/reasoning/textDelta` | `{itemId,delta}` | `ThoughtDelta` |
| `item/started` / `item/completed` | `{item,threadId,turnId}`, `item.type`: | |
| - `agentMessage {id,text}` | completed | `TextDelta` only if no delta streamed for that id |
| - `reasoning {summary[],content[]}` | completed | `ThoughtDelta` likewise |
| - `commandExecution {command,cwd,status,aggregatedOutput,exitCode}` | started/completed | `ToolCallStart` (execute) / `ToolCallUpdate` (failed if exitCode!=0) |
| - `fileChange {changes:[{path,kind,diff}],status}` | | edit call with diff content |
| - `mcpToolCall {server,tool,arguments,result,error,status}` | | `ToolCallStart` title `"server: tool"`, raw input=arguments; update on completed |
| - `webSearch {query}`, `plan`, `userMessage` (drop; we draw our own), `contextCompaction`, `collabAgentToolCall`, `subAgentActivity` | | ignore (first release) |
| `turn/plan/updated` | `{plan:[{step,status}]}` | `PlanUpdate` |
| `turn/completed` | `{threadId,turn:{id,status:"completed"\|"interrupted"\|"failed",error:{message,codexErrorInfo}}}` | THE turn end: interrupted -> Cancelled; failed -> Failed(error message) |
| `error` `{error,willRetry}` | | `Log` (never the turn end; a final one is followed by turn/completed failed) |
| `turn/started`, `thread/status/changed`, `thread/tokenUsage/updated`, `account/*`, `model/rerouted` | | ignore |

`item.status` is `inProgress|completed|failed|declined`.

Approvals are server->client REQUESTS (`id` + `method`); park each for the user and reply with the
method's own shape:

| request method | reply `result` |
|---|---|
| `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` | `{"decision":"accept"\|"acceptForSession"\|"decline"\|"cancel"}` |
| `execCommandApproval`, `applyPatchApproval` (legacy) | `{"decision":"approved"\|"approved_for_session"\|{"denied":{"rejection":"..."}}\|"abort"}` |
| `item/permissions/requestApproval` | allow: `{"permissions":<params.permissions.network/fileSystem>,"scope":"turn"\|"session"}`; refuse: `{"permissions":{},"scope":"turn"}` |
| `mcpServer/elicitation/request` | `{"action":"accept"\|"decline"\|"cancel","content":null,"_meta":null}`; offer only url elicitations or forms needing no content, `cancel` the rest at once |
| `item/tool/requestUserInput` | `{"answers":{}}` immediately (not offered) |
| anything else (`item/tool/call`, `account/chatgptAuthTokens/refresh`, ...) | error `-32601` |

Option ids exposed to the UI: `accept`/`acceptForSession`/`decline` (kinds allow_once/allow_always/reject_once).
`serverRequest/resolved {requestId}` means the server withdrew the ask: drop it from the table.
Whether codex asks at all is `approval_policy`: `never` asks nothing, `on-request` asks.

Cancel: answer every parked ask `cancel`, then request `turn/interrupt {"threadId","turnId"}` (both
required; reply `{}`; the end arrives as `turn/completed` status `interrupted`; error "no active turn to
interrupt" is a race, log only). If `turn/start`'s ack with the turn id has not arrived yet, set a
cancel-pending flag and interrupt when it does. The thread and process stay alive. Close: close stdin,
wait <= 10 s, kill (process group on unix: set `Setpgid`, kill `-pid`).

---------------------------------------------------------------------------------------------------

## 3. The in-process MCP server

Crate (`mcp/server.rs`, feature `inproc-mcp`): `tiny_http` on `127.0.0.1:0`, URL
`http://127.0.0.1:<port>/mcp`, one request at a time, 50 ms stop-poll, **no auth, no sessions, no SSE**.
JSON-RPC 2.0 in the POST body, plain `application/json` response:
- `initialize` -> `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"agent-manager-inproc","version":"0.1.0"}}`
- notification (no `id`) -> bare `202` (covers `notifications/initialized`)
- `tools/list` -> `{"tools":[{"name","description","inputSchema"}]}`
- `tools/call {name,arguments}` -> `{"content":[{"type":"text","text":<JSON-serialised value>}],"isError":false}`; handler error or
  unknown tool -> same shape with `isError:true` and the message as text (in-band, NOT a JSON-RPC error)
- anything else -> JSON-RPC error `-32601`; bad JSON -> `-32700` with `id:null`
- `Drop` stops and joins the serving thread. Handler trait: `tools() []ToolDef`, `call(name,args) -> Result<Value>`.

Our server (Go `net/http`, `agentmanager/mcp`), same behaviour plus:
1. **Bearer token**: 32 random bytes hex (`crypto/rand`), `Authorization: Bearer <t>` compared with
   `subtle.ConstantTimeCompare`; mismatch -> `401` plain (no OAuth discovery metadata).
2. **DNS-rebinding defence**: reject `Host` not in {`127.0.0.1:<port>`, `localhost:<port>`} and any
   `Origin` header that is not loopback -> `403`.
3. **Version negotiation**: reply with the client's `protocolVersion` when in {`2025-06-18`,
   `2025-03-26`, `2024-11-05`}, else `2025-06-18`. `capabilities:{"tools":{"listChanged":false}}`,
   `serverInfo{name,version}`, optional `instructions` string (tells the model how to use the tools).
4. Methods: `initialize`, `notifications/*` (202), `ping` -> `{}`, `tools/list`, `tools/call`. `GET /mcp` -> `405`
   (clients treat it as "no server-initiated stream"), `DELETE` -> `200`/`405`, no `Mcp-Session-Id`.
5. Tool results: `content:[{type:"text",text}]`, optional `{type:"image",data:<base64>,mimeType}`,
   `isError`, optional `structuredContent`. Unknown tool and handler errors in-band (crate); malformed
   params `-32602`.
6. Handlers run on the request goroutine with `r.Context()` (cancelled when the client disconnects);
   `Options.Serialize` (default true) serialises handler calls with a mutex (go-draw marshals to one UI
   goroutine anyway). Body limit 4 MiB.
7. Lifecycle: `Start()` binds before returning (so the URL is valid at once), `Close()` shuts down with a
   2 s grace and waits. One server per app, token per server, shared by every agent session; started
   lazily on first agent open.

Injection per harness (all HTTP + bearer, via `MCPServer{Name,URL,Token}` in the launch config):

| harness | injection |
|---|---|
| ACP/opencode | `session/new.mcpServers:[{"type":"http","name":"cad","url":URL,"headers":[{"name":"Authorization","value":"Bearer T"}]}]` if `mcpCapabilities.http`; else env `OPENCODE_CONFIG_CONTENT` (see 2.1) |
| Claude | `--mcp-config <0600 tmp file> --strict-mcp-config`, entry `{"type":"http","url","headers":{"Authorization":"Bearer T"}}` |
| Codex | `-c 'mcp_servers.cad={url=..., bearer_token_env_var="GODRAW_MCP_TOKEN", ...}'` + env var |

---------------------------------------------------------------------------------------------------

## 4. Layout and isolation

### 4.1 Nested module or plain package tree?

Recommendation: **nested Go module** `agentmanager/go.mod` (`module agentmanager`, `go 1.26.1`, no
requirements: stdlib only), with `require agentmanager v0.0.0` and `replace agentmanager => ./agentmanager`
in the root `go.mod` (same pattern AGENTS.md of gpui uses for `go-gpui => ../..` doc checks).

| | nested module | plain packages in root module |
|---|---|---|
| Isolation | enforced by the compiler: cannot import `go-draw/...` | by convention plus a test (`go list -deps`) |
| Lift to own repo | copy the folder, nothing to edit | needs a go.mod, import-path rewrite |
| `go test ./...` / `go vet ./...` from root | does NOT include it; need `cd agentmanager && go test ./...` (add to `justfile`, AGENTS.md verification rule) | included |
| `gofmt -l .` | still covers it (walks dirs) | covers |
| Tooling (`tools/docs`, `go mod tidy`) | tidy per module; docs lint must be told to skip `agentmanager/_docs` or learn it | one |
| Dependency drift | own go.sum, can pin separately | shared |

Because the manager needs zero third-party deps the nested module costs little; the one real cost is
the second verification command, which WP6 adds to `AGENTS.md`, `justfile` and `_docs/tech/operations.md`.
If the coordinator prefers one-command verification, the fallback is the plain tree plus a test
`agentmanager/isolation_test.go` that runs `go list -deps ./agentmanager/...` and fails on any `go-draw/` import.
Import paths below assume the nested module (`agentmanager/...`); with the plain tree they become `go-draw/agentmanager/...`.

### 4.2 Tree

```
agentmanager/                 module agentmanager; imports stdlib only
  go.mod  AGENTS.md  README.md
  _docs/  index.md api-contract.md protocols.md (the section-2 wire details) testing.md roadmap.md
  types.go session.go config.go options.go doc.go        package agentmanager (neutral API, 4.3)
  jsonrpc/        conn.go message.go error.go             package jsonrpc
  mcp/            server.go tool.go result.go             package mcp
  acp/            client.go update.go permission.go opencode.go session.go   package acp
  claude/         launch.go session.go events.go mcpconfig.go               package claude
  codex/          launch.go session.go events.go approvals.go               package codex
  harnesses/      harnesses.go                            package harnesses (Open by Harness id; imports all backends)
  transcript/     transcript.go                           package transcript (Event -> chat blocks, pure)
  internal/proc/      proc.go proc_unix.go proc_other.go  spawn, env, stderr ring, graceful close
  internal/fakeagent/ main.go scenario.go acp.go claude.go codex.go   fake CLIs (one binary, subcommands)
  internal/testutil/  fake.go                             BuildFake(t) path, scenario helpers
cadmcp/                       go-draw glue: drawing tools over cmdsys.Session (imports agentmanager/mcp)
ui/agentpanel.go agentpanel_test.go agentmailbox.go       go-draw chat panel (imports agentmanager, agentmanager/transcript)
```
Import rules: `agentmanager` (root) imports nothing but stdlib. `jsonrpc`, `mcp`, `transcript`,
`internal/proc` import nothing from the tree except `jsonrpc` (mcp may use `jsonrpc.Message` types).
`acp`, `claude`, `codex` import `agentmanager`, `jsonrpc`, `internal/proc`. `harnesses` imports the
backends. `internal/fakeagent` imports `jsonrpc` only. Backends never import each other or `mcp`.
go-draw side: `cadmcp` -> `agentmanager/mcp`, `cmdsys`, `doc`, `render`, `geom`. `ui` -> `agentmanager`,
`agentmanager/harnesses`, `agentmanager/transcript`, `cadmcp`. Nothing in `agentmanager/` ever imports go-draw.

### 4.3 FROZEN shared API (package `agentmanager`; parallel packages compile against this)

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

---------------------------------------------------------------------------------------------------

## 5. go-draw glue

### 5.1 `cadmcp` (WP6) - drawing tools, UI-goroutine safe

gpui is single-threaded and has no cross-goroutine post (only `App.Quit` is goroutine-safe;
`Window.After` timers run on the UI goroutine). MCP handlers run on HTTP goroutines. Therefore:
```go
// Dispatcher runs f on the UI goroutine and waits for it.
type Dispatcher interface{ Do(ctx context.Context, f func()) error }  // ErrUIBusy on ctx timeout / closed
type Env struct {
	Dispatch Dispatcher
	Session  func() *cmdsys.Session   // current session (it is replaced by OPEN/NEW)
	Snapshot func(w, h int) ([]byte, error) // optional PNG render; nil disables render_png
	Changed  func()                   // UI refresh hook (builder.InvalidateAll + window refresh), called inside Do
}
func Tools(e Env) []mcp.Tool
func Instructions() string            // text for mcp.Options.Instructions: coordinate system, units, script syntax
type DirectDispatcher struct{ mu sync.Mutex } // tests: runs f inline under a mutex
```
The UI implements `Dispatcher` with a mailbox: `Do` pushes `{f, done chan}` onto a mutex-guarded queue and
waits on `done` or ctx (default 15 s); the agent panel drains the queue on the UI goroutine from a
`Window.After(16ms)` timer that is re-armed only while an agent turn is active (idle app stays at ~0% CPU).
Tool calls only occur during a turn, so no extra wake-up is needed. A dead window (timer stopped) makes
`Do` return `ErrUIBusy` and the tool reports it.

Tools (all inputs JSON objects; coordinates are model units, Y up; every result is compact JSON text):

| tool | input | effect / result |
|---|---|---|
| `run_script` | `{script}` | If a command is active, Escape it. `Session.Script(script)` (single spaces / newlines separate tokens; double space = Enter). After the script, if a command is still active, Escape it and say so. Returns `{output:[history lines added], entities_before, entities_after, new_handles:[..], cancelled_command:""}` |
| `add_line` | `{x1,y1,x2,y2}` | generates `LINE x1,y1 x2,y2  ` |
| `add_circle` | `{cx,cy,r}` | `CIRCLE cx,cy r` |
| `add_rect` | `{x1,y1,x2,y2}` | `RECTANG x1,y1 x2,y2` |
| `add_polyline` | `{points:[[x,y]..],closed}` | `PLINE ... [c]` |
| `add_arc` | `{p1,p2,p3}` | `ARC p1 p2 p3` (3-point) |
| `set_layer` | `{name,color?}` | `LAYER name[=color]` (as the demo script) |
| `get_drawing` | `{}` | layers (name,color,off,locked), current layer, entity count per class, extents, can_undo/can_redo |
| `list_entities` | `{layer?,class?,offset?,limit?<=200}` | `[{handle,class,layer,bounds,geometry-summary}]` (handles as hex strings) |
| `get_selection` / `select` | `{}` / `{handles:[..]}` | selection handles; select replaces selection |
| `undo` / `redo` | `{steps?}` | `Drawing.Undo/Redo`; returns remaining flags |
| `zoom_extents` | `{}` | `Session.Run("ZOOM")` + `E` token or `View.Fit`; UI refresh |
| `list_commands` | `{}` | `Registry.Names()` |
| `render_png` | `{width?,height?}` | image content via `Env.Snapshot` (optional, WP6b) |

Rules: each tool body is one `Dispatcher.Do`; call `Env.Changed` before returning; one command = one undo
step (Session already guarantees it); a tool never leaves a command half-open; errors become
`mcp.Errorf` with the session's last history lines. One small additive core change is allowed in WP6:
`cmdsys.Session.HistoryTotal() int` (monotonic count of printed lines; `History()` is capped at 200 and
trims) so `run_script` can return exactly the lines it produced. It requires editing
`_docs/tech/command-system.md` in the same change (repo rule).

### 5.2 `ui` agent panel (WP6)

`ui/agentpanel.go`: a docked right panel (toggle button + `F2`) with a harness selector (only harnesses where
`harnesses.Available` is true, plus a "fake" entry when built with a test tag), transcript list rendered from
`transcript.T`, a multi-line prompt `gpui.TextInput`, Send/Stop buttons, pending-permission cards with one
button per option (and a plain "allow all MCP for this chat" toggle wired to `AllowMCP("cad")`).
Threading: one pump goroutine per session reads `Session.Events()` into a mutex-guarded slice (`ui/agentmailbox.go`);
the UI timer drains it, calls `transcript.Apply`, and `w.Refresh()`. Starting the session
(`harnesses.Open`, which spawns a process and can take seconds) runs on a goroutine and reports back via the mailbox.
`Workspace` gets `AgentPanel()` and `cmd/cad` flags `-agent opencode|claude|codex` and `-agent-bin PATH`.
Visual check: `-png` render with a canned transcript (a seeded `transcript.T`), compared per AGENTS.md rule 8.

---------------------------------------------------------------------------------------------------

## 6. Test strategy (no real CLIs)

- **Fake agents** (`agentmanager/internal/fakeagent`): one `main` with subcommands `acp`, `claude`, `codex`;
  behaviour comes from a JSON **scenario** (`FAKEAGENT_SCENARIO=<file>` or `--scenario`): ordered steps such as
  `expect {method/line match}`, `send {frame}`, `wait-for-reply {id var}`, `sleep`, `exit {code}`, plus builtins
  `echo-turn` (init, one text turn, end) and `tool-turn` (tool call + permission + result + end). It also
  records every received line to `FAKEAGENT_LOG` so tests assert what the client wrote (as the crate's
  `AM_FAKE_STDIN`). WP1 owns `main.go`/`scenario.go` and creates stub `acp.go/claude.go/codex.go` (one
  `func Run(sc *Scenario, in io.Reader, out io.Writer) int` each) which WP3/4/5 then own.
- `internal/testutil.BuildFake(t) string` runs `go build -o <cache>/fakeagent ./internal/fakeagent` once per test
  binary (`sync.Once`, `t.TempDir` of the first caller or `os.UserCacheDir`) and returns the path; `Config.Binary`
  points at it, with `Config.Args` selecting `acp|claude|codex` (the fake acts as the CLI; subcommand/flags are
  ignored apart from `acp`/`app-server`/`-p`). Per-harness tests also assert the exact argv/env/config file the
  backend generates by letting the fake dump them (`FAKEAGENT_ARGV`).
- **Per-backend suites** (each in its package, external `_test` package): handshake ok / protocol version
  mismatch / handshake timeout (fake never answers; shorten via unexported test hook) / stderr tail in `Exited.Err`;
  text and thought streaming order; tool call lifecycle; permission round trip incl. reply shape bytes; cancel with a
  pending permission (permission answered cancelled BEFORE the cancel frame; turn ends Cancelled; next prompt works);
  prompt-while-running error; unknown inbound request gets `-32601` exactly once; malformed/blank lines ignored; kill
  on Close when the fake ignores stdin EOF; child crash mid-turn -> `Exited{Err}` (turn abandoned; Session methods then return `ErrClosed`); large line (1 MiB) survives; race detector clean (`-race`).
- **jsonrpc**: in-memory pipes; call/response correlation, id collision (inbound request id equals ours), CallAsync,
  writer never blocks when the peer stops reading (bounded by queue), `Done`.
- **mcp**: in-test `net/http` client (as the crate's `ureq` tests): initialize (version negotiation), tools/list,
  tools/call (ok, handler error in-band, unknown tool in-band, bad params -32602), notification -> 202 empty,
  malformed body -> -32700 id null, GET -> 405, missing/wrong token -> 401, bad Host/Origin -> 403, concurrent calls
  serialised, `Close` stops listener, client cancel cancels the handler context.
- **cadmcp**: `DirectDispatcher` + a real `cmdsys.Session` (like `cmdsys/commands/commands_test.go`): each tool's
  effect on the document and its JSON result; script leaves a command open -> escaped and reported; undo/redo flags.
  Plus one end-to-end test: real `mcp.Server` + HTTP client calling `add_line` while the drawing is mutated from a
  "UI goroutine" that drains a mailbox (`-race`).
- **ui**: `HeadlessWindow.Dispatch` clicks on the panel, a fake session feeding scripted events, assertions on
  transcript and permission answers; mailbox drain test; `-png` render of the panel.
- **Live smoke (gated)**: `AGENTMANAGER_LIVE=1 go test ./claude -run Live` uses the installed `claude` (needs auth;
  skip otherwise) to confirm every **(unverified)** item flagged in section 2: `system/init` timing, partial-message
  streaming and dedupe, deny message, `--permission-prompts`, `mcp__cad__*` naming through our MCP server. Equivalent
  gated tests for opencode/codex run wherever those CLIs exist; the PR description must say which were not run.

---------------------------------------------------------------------------------------------------

## 7. Work packages

Step 0 (coordinator or WP1, ~30 min, FIRST): create `agentmanager/go.mod`, root `replace`, and commit the
frozen declarations of section 4.3 (`types.go`, `jsonrpc` exported signatures with `panic("todo")` bodies,
`mcp` exported signatures, `transcript` signatures, `harnesses.Open` stub, fakeagent stubs) so that every WP compiles
against the same API. After step 0 the WPs below run in parallel with disjoint files. If a WP needs an API change it
stops and reports (AGENTS.md rule); the coordinator edits section 4.3 and `agentmanager/_docs/api-contract.md` first.

| WP | owns (files/dirs) | content | depends on |
|---|---|---|---|
| **WP1 core** | `agentmanager/*.go` (root), `go.mod`, `AGENTS.md`, `README.md`, `_docs/` skeleton + `api-contract.md`, `jsonrpc/`, `internal/proc/`, `internal/fakeagent/{main,scenario}.go` + stubs, `internal/testutil/`, `harnesses/` | neutral types, `AllowMCP`, unbounded event queue helper (`agentmanager.NewQueue`: used by every backend), jsonrpc Conn (+raw handler option), process spawn/close/kill/stderr ring/env hygiene, fake agent framework + scenario runner, harnesses.Open | step 0 |
| **WP2 mcp + transcript** | `agentmanager/mcp/`, `agentmanager/transcript/`, `_docs/protocols.md` section "MCP" | server per section 3 with tests; transcript model with tests (event sequences incl. permission resolve, plan, deduped tool updates) | step 0 |
| **WP3 acp/opencode** | `agentmanager/acp/`, `internal/fakeagent/acp.go`, `_docs/protocols.md` ACP section | client per 2.1: handshake, prompt slot, updates mapping, permissions table, cancel ordering, MCP wiring + env fallback, `OpenCode()` preset; tests with fake | WP1 (jsonrpc, proc, fake framework; can start against stubs and a local pipe pair) |
| **WP4 claude** | `agentmanager/claude/`, `internal/fakeagent/claude.go`, gated live test | argv/env/mcp-config file, stream parsing, partial-message dedupe, control_request/response, interrupt, resume | WP1 |
| **WP5 codex** | `agentmanager/codex/`, `internal/fakeagent/codex.go` | handshake, turn slot + late-ack cancel, notification mapping, approvals table, `-c` injection, resume | WP1 |
| **WP6 go-draw glue** | `cadmcp/`, `ui/agent*.go`, `cmd/cad/main.go` flags, `cmdsys/session.go` (`HistoryTotal` only), root `go.mod`/`justfile`, `AGENTS.md` verification rule, `_docs/features/agent-panel.md`, `_docs/tech/agent-integration.md`, INDEX/code-map/backlog/operations/command-system updates, `docs lint` | tools, dispatcher + mailbox, panel, CLI flags, docs | WP6a (cadmcp, only needs `mcp` + `cmdsys`) can start at step 0; WP6b (ui panel) needs WP2 transcript and WP1 types; end-to-end needs WP1 fake + any backend |

Order: step 0 -> {WP1, WP2, WP6a} -> {WP3, WP4, WP5} (need WP1's jsonrpc/proc/fake framework; they can begin on
frozen stubs and use `io.Pipe` pairs in their first tests) -> WP6b -> integration (WP6 wires `harnesses.Open`,
runs the end-to-end fake-agent test: fake claude calls `mcp__cad__add_line` over real HTTP, drawing changes) ->
live smoke where a CLI exists. Suggested first deliverable: WP1+WP2+WP4+WP6 give a working Claude path (the only CLI
installed); WP3/WP5 follow.

Definition of done for every WP: `gofmt -l .` empty, `go vet ./...` and `go test -race ./...` in both modules
(`agentmanager/` and root), doc comment on every exported identifier, docs updated in the same change (agentmanager's own
`_docs/` for the library, go-draw `_docs/` + `go run ./tools/docs lint` for glue; check whether the linter walks into
`agentmanager/_docs` and exclude or conform), and a statement of which behaviour was not run (no real opencode/codex,
no GUI).

Open questions for the coordinator: (1) nested module vs plain tree (recommended: nested); (2) default permission policy
(recommended: auto-allow `cad` MCP tools, ask for everything else); (3) whether `render_png` ships in the first
release; (4) whether the agent may run shell/file tools at all inside a CAD project (suggest default `--permission-mode`
unset and opencode/codex sandbox read-only, so only the `cad` tools can change the drawing).
