# Protocols

One section per wire protocol or protocol-like component.

## MCP (package `mcp`)

In-process MCP server for injecting tools (the `cad` drawing tools) into agents. stdlib `net/http`,
streamable HTTP without SSE or sessions. One server per app, one token per server, shared by every
agent session.

- **Endpoint.** `Start()` binds `127.0.0.1:0` (tcp4) before returning and yields `Endpoint{URL, Token}`,
  `URL = http://127.0.0.1:PORT/mcp`, `Token` = 32 random bytes hex. Callers inject it as
  `agentmanager.MCPServer{Name, URL, Token}` (header `Authorization: Bearer <token>`). Tokens are never logged.
- **Checks, in order.** `Host` must be `127.0.0.1:PORT` or `localhost:PORT`, and any `Origin` must have a
  loopback host (else `403`); then bearer token via `subtle.ConstantTimeCompare` (else `401`, no OAuth
  metadata). Only `POST /mcp` is served: `GET`, `DELETE` and others get `405` (no server-initiated stream,
  no `Mcp-Session-Id`). Body limit 4 MiB (`413`).
- **Framing.** JSON-RPC 2.0 in the POST body, `application/json` reply. A single request gets one reply; a
  batch (JSON array) gets an array of the replies to its requests; notifications and client responses get no
  reply, and a body with nothing to reply to gets a bare `202`. Bad JSON is `-32700` (id `null`), missing method
  `-32600`, unknown method `-32601`, bad `tools/call` params `-32602`.
- **Methods.** `initialize` echoes the client's `protocolVersion` if it is `2025-06-18`, `2025-03-26` or
  `2024-11-05`, else `2025-06-18`; `capabilities:{tools:{listChanged:false}}`, `serverInfo{name,version}`
  and optional `instructions`. `notifications/*` -> 202. `ping` -> `{}`. `tools/list` (registration order,
  default schema `{"type":"object"}`). `tools/call {name, arguments}`.
- **Tool results.** `{content:[{type:"text",text}|{type:"image",data,mimeType}], isError, structuredContent?}`.
  Unknown tool, handler error, and handler panic are in-band `isError` results with the message as text,
  not JSON-RPC errors.
- **Concurrency.** Handlers run on the request goroutine with `r.Context()` (cancelled on client disconnect
  and on `Close`). `Options.Serialize` (default true) serialises handler calls with a context-aware semaphore: a call
  whose request context is done while waiting, or by the time it wins the slot, returns an error and never runs its handler.
- **Lifecycle.** `Close()` stops accepting, waits up to 2 s for in-flight requests, then cancels their
  contexts and force-closes. Idempotent; a closed server cannot be restarted.

## Codex (package `codex`, `codex app-server --listen stdio://`)

JSON-RPC over NDJSON stdio, no `jsonrpc` member required on frames. Needs codex >= 0.100. No `codex` CLI was
available when this was written: everything is verified against `internal/fakeagent` only (`RunCodex`); the wire
shapes are **unverified** until a gated live run: the `-c mcp_servers` override keys, `bearer_token_env_var`, the
approval request and `mcpToolCall` item shapes, `thread/resume` params.

- **Launch.** `codex -c k=v ... app-server --listen stdio://` then `Config.Args`; `-c` overrides precede the
  subcommand. Managed overrides: `sandbox_mode="read-only"`, `approval_policy="on-request"` (no permissive mode),
  `model`, `developer_instructions` (from `SystemPrompt`), and per MCP server
  `mcp_servers.<name>={url=..., bearer_token_env_var="CADLITE_MCP_TOKEN", default_tools_approval_mode="approve", tool_timeout_sec=120}`.
  The token travels only in the child env (`CADLITE_MCP_TOKEN`, then `CADLITE_MCP_TOKEN_1`, ... for further servers),
  never in argv.
- **Handshake** (10 s per call, reader running first): `initialize` (`experimentalApi`), `initialized`,
  `thread/start {cwd}` or `thread/resume {threadId,cwd,excludeTurns:true}`; `SessionStarted{thread.id, model}`.
  A failing handshake returns an error with the stderr tail.
- **Models.** After the handshake `model/list {}` is sent asynchronously; its first page (`data[]`: `model`, else
  `id`, and `displayName`) becomes `ModelsChanged{Current: thread model, Available}`. An error (an older
  app-server) is ignored. `SetModel` writes nothing: it stores the model, emits `ModelsChanged` and every later
  `turn/start` carries `"model"` (Codex keeps a turn override for the following turns). Unverified like the rest.
  A model's `inputModalities` (when the app-server sends it) sets `ModelInfo.Vision`: with `"image"` yes, without
  it no; absent is unknown.
- **Images.** `PromptImages` appends `{"type":"image","url":"data:<mime>;base64,<data>"}` to `turn/start`'s `input`
  after the text item (no temporary file, so `localImage` is not used). `AcceptsImages` is always true. Unverified.
- **Turns.** One in flight. `Prompt` sends `turn/start` and returns; the ack carries `turn.id`. A failed ack is
  reported as `TurnEnd{Failed}`. `turn/completed` is the only turn end (`interrupted` -> Cancelled,
  `failed` -> Failed with the error message).
- **Notifications** (only our `threadId`; others are subagents and dropped): agent-message and reasoning deltas;
  completed `agentMessage`/`reasoning` items only when nothing streamed for that id; `commandExecution`,
  `fileChange`, `mcpToolCall` items -> `ToolCallStart` / `ToolCallUpdate` (MCP: title `server: tool`,
  `MCPServer`/`MCPTool` set, output from the result text blocks); `turn/plan/updated` -> `PlanUpdate`; `error` ->
  `Log`; the rest ignored.
- **Approvals** (server requests, parked by request id): command/file/legacy exec and patch ask `accept`,
  `acceptForSession`, `decline` (reply shapes per the plan's table; legacy ones map to
  `approved`/`approved_for_session`/`denied`); `item/permissions/requestApproval`; elicitations that need no
  content (url, or a form with nothing required; `MCPServer` from `serverName`) with `accept`/`decline`, others are
  cancelled at once; `item/tool/requestUserInput` answered `{"answers":{}}`; any other request gets `-32601`
  once. `Config.AutoApprove` answers first; `serverRequest/resolved` drops a parked ask.
- **Cancel.** Every parked ask is answered cancel, then `turn/interrupt {threadId,turnId}`; if the `turn/start` ack
  is still pending the interrupt is sent when it arrives. The thread and process stay alive.
- **Exit.** Process gone -> `Exited` (Err carries the exit error and stderr tail unless `Close` was called), events
  closed, methods return `agentmanager.ErrClosed`. `Close` closes stdin, waits <= 10 s, kills the process group.
- **Fake scenarios** (`FAKEAGENT_SCENARIO` builtins): `echo-turn`, `tool-turn`, `mcp-turn`, `hold-turn`,
  `late-ack-turn`, `cancel-ask-turn`, `crash-turn`, `no-handshake`, and `mcp-call-turn` (a real MCP `tools/call`).

## Claude Code (package `claude`)

Backend for `claude -p` in stream-json mode: NDJSON on stdin/stdout, not JSON-RPC (the package reuses
`jsonrpc.Conn` only for line framing and the never-blocking writer, via `WithRawHandler`). Verified live
against Claude Code 2.1.299 (see "Live test").

- **Argv.** `-p --input-format stream-json --output-format stream-json --verbose --include-partial-messages
  --permission-prompt-tool stdio --disallowedTools AskUserQuestion --mcp-config <tmp> --strict-mcp-config`
  plus `--model`, `--resume <id>`, `--append-system-prompt` when set, then `Config.Args`. No permissive flags
  (`--dangerously-skip-permissions`, `--permission-mode`) are ever passed. `--permission-prompt-tool stdio` is what
  makes Claude ask instead of auto-denying; 2.1.299's default `--permission-prompts host` agrees.
- **MCP config.** `{"mcpServers":{"<name>":{"type":"http","url":URL,"headers":{"Authorization":"Bearer T"}}}}` in a
  0600 temp file (the token never appears in argv); removed on `Close` or process exit. Claude names the tools
  `mcp__<name>__<tool>`; the backend sets `ToolCall.MCPServer`/`MCPTool` and `ToolCallPatch.MCPServer` from that.
  Claude 2.1.299 defers MCP tools behind its `ToolSearch` tool (live finding), so the first MCP use in a session
  shows an extra `ToolSearch` tool call before the `mcp__cad__*` call; it is an ordinary tool block, not an error.
- **Env.** `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`, `CLAUDE_CODE_SESSION_ID`,
  `CLAUDE_CODE_SSE_PORT` are removed so a launch from inside Claude does not inherit the parent.
- **No handshake.** `Open` returns right after the spawn. Claude emits `system/init` only after the first user
  line; that yields one `SessionStarted` (a repeated init is ignored). `Config.ResumeID` becomes `--resume` and
  `ID()` returns it until init reports the id.
- **stdin.** User turn: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":T}]}}` (one turn at
  a time; a second `Prompt` errors). `PromptImages` adds one
  `{"type":"image","source":{"type":"base64","media_type":M,"data":B64}}` block per image after the text block
  (the Messages API image block; not run live with an image). Every listed model is `VisionYes`. Permission answer:
  `{"type":"control_response","response":{"subtype":"success","request_id":R,"response":{...}}}` where `response` is
  `{"behavior":"allow","updatedInput":<the request's original input>}` (plus `"updatedPermissions":[<suggestion>]` for
  an always-option) or `{"behavior":"deny","message":"Denied by user"|"Cancelled"}`. Interrupt:
  `{"type":"control_request","request_id":"gd-int-N","request":{"subtype":"interrupt","reason":"interrupt"}}`.
- **stdout mapping.**

  | line | event |
  |---|---|
  | `system/init` | `SessionStarted{SessionID, Model}` once |
  | `stream_event` `message_start` / `content_block_delta` (`text_delta`, `thinking_delta`) | `TextDelta` / `ThoughtDelta` with the message id; `input_json_delta` ignored |
  | `assistant` blocks `text`, `thinking` | `TextDelta` / `ThoughtDelta`, skipped when that message id was already streamed or the thinking is empty (a bare signature = withheld) |
  | `assistant` block `tool_use` | `ToolCallStart` (in_progress; never deduped away, announced once per id); `TodoWrite` also emits `PlanUpdate` |
  | `user` block `tool_result` | `ToolCallUpdate` (failed on `is_error`; `Output` = joined text; `RawOutput` = content; `async_launched` stays in_progress) |
  | `control_request` `can_use_tool` | `PermissionRequest` (options `allow`, `allow_always:<n>` per suggestion, `deny`); `Config.AutoApprove` answers first and emits nothing |
  | other `control_request` | `control_response` `{"subtype":"error","request_id":R,"error":"unsupported"}` |
  | `control_cancel_request` | forgets the pending request |
  | `control_response` | answers one of our control requests by `request_id` (`initialize`, `set_model`); others (the interrupt's) are ignored |
  | `result` | `TurnEnd`: cancelled when we interrupted, `error_max_turns` -> max_turn_requests, `is_error` or a non-success subtype -> failed (`Err` = result text), `stop_reason` max_tokens/refusal, else end_turn. Usage goes to `Logger.Debugf` only |
  | `log` | `Log` |

  Tool kinds: Read/NotebookRead/Glob/LS read; Edit/Write/NotebookEdit/MultiEdit edit; Bash/BashOutput/KillShell
  execute; Grep/WebSearch search; WebFetch fetch; TodoWrite/ExitPlanMode think; everything else other. Title is
  `"<Name> <file_path|path|command|pattern|url>"`, or the raw `mcp__srv__tool` name for MCP tools (what `AllowMCP`'s
  title fallback expects).
- **Models.** `Open` writes `{"type":"control_request","request_id":"gd-ctl-N","request":{"subtype":"initialize"}}`
  right after the spawn; Claude answers before any prompt with `response.models[]` (`value`, `displayName`), which
  becomes `ModelsChanged{Current: Config.Model, Available}` (2.1.299 lists aliases such as `default`, `opus`,
  `sonnet`, `haiku` and full ids). A CLI that does not answer reports nothing. `SetModel` sends
  `{"subtype":"set_model","model":M}` and waits for `control_response`: `success`, or `error` with `error`
  (`Model 'x' not found`). Both verified live on 2.1.299 (`TestLiveClaudeModels`, no prompt sent).
- **Cancel.** Every outstanding permission is denied first (an interrupt with an unanswered ask makes Claude fail the
  tool), then the interrupt is written. stdin stays open, the turn ends `TurnEnd{Cancelled}` and the next prompt
  reaches the same process. An unanswered permission stalls the turn, so `Close` denies first too.
- **Close / exit.** `Close` denies pending asks, interrupts a running turn, closes stdin, waits up to 10 s, then
  kills the process group, and removes the temp config. When the process ends without `Close`, `Exited{Err}` carries
  the exit error plus the stderr tail; the running turn is abandoned (no `TurnEnd`) and every method returns
  `agentmanager.ErrClosed`.
- **Live findings (2.1.299).** MCP tools are deferred behind `ToolSearch` (see MCP config above). Read-only `Bash`
  such as `echo` is auto-allowed by Claude itself and never reaches `can_use_tool`; use `touch` or similar to
  provoke an ask. `result.usage` is not forwarded as an event (`TurnEnd` has no usage field); it is written to
  `Logger.Debugf` only.
- **Live test.** `AGENTMANAGER_LIVE_CLAUDE=1 go test ./claude -run Live -v` (needs auth and network). Confirmed on
  2.1.299: init after the first line, partial deltas with no duplicated final text, `mcp__cad__ping` through our
  `mcp.Server` with bearer auth and `AllowMCP("cad")` auto-allow, allow and deny round trips (`Bash` needs a non-read-only
  command such as `touch`; `echo` is auto-allowed by Claude itself), interrupt then a fresh prompt on the same process.

## ACP (package `acp`)

Generic Agent Client Protocol v1 client (JSON-RPC 2.0, newline-delimited, stdio). `acp.New(Command)` serves any
ACP agent; `acp.OpenCode()` is the preset for `opencode acp`. `Config.Binary` overrides the executable and
`Config.Args` is appended after `acp`. There is no `opencode` CLI in the test environment: everything is verified
against `internal/fakeagent` (builtins `echo-turn`, `tool-turn`, see its `RunACP` comment); a live run is not done,
so the opencode wire details here (auth method ids, the `OPENCODE_CONFIG_CONTENT` shape, tool title formats for
MCP derivation, `session/load`) are **unverified**.

Decisions: (1) the history an agent replays during `session/load` is suppressed (`loading` flag in the session), so
resuming does not duplicate the transcript. (2) The opencode MCP env fallback restarts the process once with
`OPENCODE_CONFIG_CONTENT`, because that variable is read at startup and cannot be added to a running agent.

- **Handshake** (each step 30 s). `initialize {protocolVersion:1, clientInfo, clientCapabilities:{fs:{readTextFile:false,
  writeTextFile:false}, terminal:false}}`; a reply with another protocol version aborts. `authenticate` is sent only
  when `result._meta.defaultAuthMethodId` exists. `session/new {cwd, mcpServers}` (`mcpServers` is always an array).
  Error `-32000` becomes "authentication required" (OpenCode adds ``authenticate the CLI first (`opencode auth login`)``
  plus the advertised method ids). `Config.ResumeID` uses `session/load` (needs `loadSession`); the history the agent
  replays while loading is dropped. `Config.Model` is applied over the wire (`session/set_config_option` on the
  `model`-category option, else `session/set_model`), never argv; a failure is a warn `Log`. `SessionStarted` follows,
  then `ModelsChanged` when the result lists models: the model option's `options` (`value`, `name`; grouped
  options flattened), else `models.availableModels` (`modelId`, `name`). `SetModel` uses the same two methods
  and fails when the agent advertised neither.
- **MCP injection.** With `mcpCapabilities.http`, each `Config.MCPServers` entry is sent in `session/new` as
  `{type:"http", name, url, headers:[{name:"Authorization", value:"Bearer TOKEN"}]}`. Without it, `OpenCode()`
  restarts the process once with `OPENCODE_CONFIG_CONTENT={"mcp":{name:{type:"remote",url,headers,enabled:true}}}` and
  sends `mcpServers:[]`; a generic `New` agent gets a warn `Log` and no servers. Bearer tokens are redacted from
  `Logger` output.
- **Preset.** `opencode acp`, extra env `OPENCODE_DISABLE_CLAUDE_CODE=1`. No permissive flags or modes: permissions
  are real round trips.
- **Prompt slot.** `Prompt` sets a single turn flag, writes `session/prompt` with `CallAsync` and returns; a second
  prompt while running is an error. `Config.SystemPrompt` prefixes the first prompt of the session. The response ends
  the turn: `stopReason` maps to `TurnEnd.Reason`; a JSON-RPC error is `TurnEnd{Failed, message}` and the session stays
  usable. If the process dies mid-turn no `TurnEnd` is sent, `Exited{Err}` follows.
- **Images.** `AcceptsImages` is `initialize`'s `agentCapabilities.promptCapabilities.image`. `PromptImages` adds one
  `{"type":"image","mimeType":M,"data":B64}` content block per image after the text block; without the capability
  it returns `ErrNoImages` and writes nothing. ACP has no per-model modality, so models are `VisionUnknown`; opencode
  is assumed to tell the user itself when its model cannot read an image (unverified).
- **Updates.** `agent_message_chunk` -> `TextDelta`, `agent_thought_chunk` -> `ThoughtDelta` (text content only),
  `tool_call` -> `ToolCallStart`, `tool_call_update` -> `ToolCallUpdate` (absent field = unchanged; text of `content`
  items becomes `Output`, diffs as `diff <path>`), `plan` -> `PlanUpdate`. `user_message_chunk`,
  `available_commands_update` and the other update kinds are ignored. Unknown kinds map to `other`. `MCPServer`/`MCPTool`
  are derived from the title (`<server>_<tool>` with the tool in `[a-z0-9_]+`, or `mcp__server__tool`) and only for
  `other`/`read` kinds, against the configured servers and remembered per tool call id so later patches and permission requests carry it.
- **Permissions.** `session/request_permission` becomes `PermissionRequest` with `RequestID` `n:<num>` or `s:<str>` (the
  raw JSON-RPC id is echoed). `Config.AutoApprove` is consulted first; an answer is sent at once and the UI only sees an
  info `Log`. Otherwise the request is parked until `RespondPermission`; the reply is
  `{outcome:{outcome:"selected",optionId}}` or `{outcome:{outcome:"cancelled"}}`. A turn end drops outstanding requests
  without replying. `Cancel` replies `cancelled` to every outstanding request, then sends the `session/cancel`
  notification; the turn ends with `StopCancelled` when the agent says so.
- **Other inbound requests** (`terminal/*`, `fs/*`, `elicitation/*`, vendor methods) get `-32601` once.
- **Exit.** When stdout closes or the process exits, `Exited` is the last event (`Err` empty only after `Close`), the event
  channel closes and every method returns `agentmanager.ErrClosed`. `Close` answers pending permissions `cancelled`,
  cancels a running turn, closes stdin, waits up to 10 s, then kills the process group. Idempotent.
