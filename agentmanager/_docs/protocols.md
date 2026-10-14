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
  and on `Close`). `Options.Serialize` (default true) serialises handler calls with a mutex.
- **Lifecycle.** `Close()` stops accepting, waits up to 2 s for in-flight requests, then cancels their
  contexts and force-closes. Idempotent; a closed server cannot be restarted.

## Codex (package `codex`, `codex app-server --listen stdio://`)

JSON-RPC over NDJSON stdio, no `jsonrpc` member required on frames. Needs codex >= 0.100. No `codex` CLI was
available when this was written: everything is verified against `internal/fakeagent` only (`RunCodex`); the wire
shapes marked unverified in the plan stay so until a gated live run.

- **Launch.** `codex -c k=v ... app-server --listen stdio://` then `Config.Args`; `-c` overrides precede the
  subcommand. Managed overrides: `sandbox_mode="read-only"`, `approval_policy="on-request"` (no permissive mode),
  `model`, `developer_instructions` (from `SystemPrompt`), and per MCP server
  `mcp_servers.<name>={url=..., bearer_token_env_var="GODRAW_MCP_TOKEN", default_tools_approval_mode="approve", tool_timeout_sec=120}`.
  The token travels only in the child env (`GODRAW_MCP_TOKEN`, then `GODRAW_MCP_TOKEN_1`, ... for further servers),
  never in argv.
- **Handshake** (10 s per call, reader running first): `initialize` (`experimentalApi`), `initialized`,
  `thread/start {cwd}` or `thread/resume {threadId,cwd,excludeTurns:true}`; `SessionStarted{thread.id, model}`.
  A failing handshake returns an error with the stderr tail.
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
  `late-ack-turn`, `cancel-ask-turn`, `crash-turn`, `no-handshake`.

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
  Claude 2.1.x defers MCP tools behind its `ToolSearch` tool, so the first MCP use shows an extra `ToolSearch` call.
- **Env.** `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`, `CLAUDE_CODE_SESSION_ID`,
  `CLAUDE_CODE_SSE_PORT` are removed so a launch from inside Claude does not inherit the parent.
- **No handshake.** `Open` returns right after the spawn. Claude emits `system/init` only after the first user
  line; that yields one `SessionStarted` (a repeated init is ignored). `Config.ResumeID` becomes `--resume` and
  `ID()` returns it until init reports the id.
- **stdin.** User turn: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":T}]}}` (one turn at
  a time; a second `Prompt` errors). Permission answer:
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
  | `control_response` | ignored (reply to our interrupt) |
  | `result` | `TurnEnd`: cancelled when we interrupted, `error_max_turns` -> max_turn_requests, `is_error` or a non-success subtype -> failed (`Err` = result text), `stop_reason` max_tokens/refusal, else end_turn. Usage goes to `Logger.Debugf` only |
  | `log` | `Log` |

  Tool kinds: Read/NotebookRead/Glob/LS read; Edit/Write/NotebookEdit/MultiEdit edit; Bash/BashOutput/KillShell
  execute; Grep/WebSearch search; WebFetch fetch; TodoWrite/ExitPlanMode think; everything else other. Title is
  `"<Name> <file_path|path|command|pattern|url>"`, or the raw `mcp__srv__tool` name for MCP tools (what `AllowMCP`'s
  title fallback expects).
- **Cancel.** Every outstanding permission is denied first (an interrupt with an unanswered ask makes Claude fail the
  tool), then the interrupt is written. stdin stays open, the turn ends `TurnEnd{Cancelled}` and the next prompt
  reaches the same process. An unanswered permission stalls the turn, so `Close` denies first too.
- **Close / exit.** `Close` denies pending asks, interrupts a running turn, closes stdin, waits up to 10 s, then
  kills the process group, and removes the temp config. When the process ends without `Close`, `Exited{Err}` carries
  the exit error plus the stderr tail; the running turn is abandoned (no `TurnEnd`) and every method returns
  `agentmanager.ErrClosed`.
- **Live test.** `AGENTMANAGER_LIVE_CLAUDE=1 go test ./claude -run Live -v` (needs auth and network). Confirmed on
  2.1.299: init after the first line, partial deltas with no duplicated final text, `mcp__cad__ping` through our
  `mcp.Server` with bearer auth and `AllowMCP("cad")` auto-allow, allow and deny round trips (`Bash` needs a non-read-only
  command such as `touch`; `echo` is auto-allowed by Claude itself), interrupt then a fresh prompt on the same process.
