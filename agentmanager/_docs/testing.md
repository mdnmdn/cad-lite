# Testing

No real agent CLI is needed for the test suite. Every backend is exercised against `internal/fakeagent`,
a single program that impersonates `opencode` (ACP), `claude` (stream-json) and `codex` (app-server).

## Building the fake

`testutil.BuildFake(t)` runs `go build ./internal/fakeagent` once per test binary into a temp
directory and returns the path. Point `Config.Binary` at it and select the protocol with `Config.Args`:
`acp`, `-p` (or `claude`), `app-server` (or `codex`). Other flags the backend adds are ignored.

## Environment the fake reads

| variable | effect |
|---|---|
| `FAKEAGENT_SCENARIO` | path of a scenario JSON file (or pass `--scenario PATH` as an argument) |
| `FAKEAGENT_LOG` | every line received from the client is appended here, so tests assert what the backend wrote |
| `FAKEAGENT_MODELS` | the Claude fake answers `initialize` with two models and the Codex fake answers `model/list` with three (`inputModalities` text+image, text only, absent; method-not-found otherwise); the ACP fake's equivalent is the builtin option `models`. The Claude and ACP fakes refuse the model `bad-model` |
| `FAKEAGENT_ARGV` | the fake writes `ARGV`, the arguments, `ENV`, the sorted environment, `CWD`, the working directory, to this file at startup |

Pass them through `Config.Env`.

## Scenarios

```json
{
  "builtin": "echo-turn",
  "steps": [
    {"op": "expect", "match": {"method": "initialize"}, "var": "init"},
    {"op": "send", "frame": {"jsonrpc": "2.0", "id": "$init", "result": {"protocolVersion": 1}}},
    {"op": "sleep", "ms": 10},
    {"op": "stderr", "frame": {"text": "boom"}},
    {"op": "exit", "code": 1}
  ]
}
```

`builtin` (`echo-turn`: init, one text turn, end; `tool-turn`: tool call, permission, result, end) is
interpreted by the protocol file (`acp.go`, `claude.go`, `codex.go`); explicit `steps` run through the
shared `Runner` in `runner.go`. Every `echo-turn` answers each image it received with
` [image <mime> <n> bytes]` (`images.go`); the ACP option `noimage` advertises `promptCapabilities.image` false.

| op | meaning |
|---|---|
| `expect` | read frames until one contains `match` (recursive subset, `"*"` = any present value); `var` stores its `id` |
| `send` | write `frame`; string values equal to `"$name"` become variable `name` (any JSON type) |
| `wait-for-reply` | read until a response (no `method`) whose id equals variable `var` |
| `sleep` | pause `ms` |
| `stderr` | write `frame.text` to stderr (exercises the stderr tail in `Exited.Err`) |
| `hang` | keep running and ignore stdin EOF (exercises kill-on-Close) |
| `exit` | stop with `code`; finishing all steps exits 0, stdin EOF while a step waits exits 1 |

Protocol authors use `NewRunner(in, out)`: `ReadFrame`, `WriteFrame` (with `$var` substitution),
`WriteLine` (raw, for malformed-line tests), `Expect`, `Match`, `Vars`, `RunSteps`.

## Other helpers

- `jsonrpc` tests use `io.Pipe` pairs in memory.
- `internal/proc` tests re-exec the test binary as the child (`TestMain` keyed on `PROC_HELPER`).
- The race detector makes a child test binary take about a second to exit; that is not a bug.
- `jsonrpc` tests also cover the inbound line cap (`TestOverlongLineStopsReader`, `TestLineAtCapIsAccepted`) and a failed write stopping the writer (`TestWriteErrorStopsWriterAndFailsPending`); `proc` counts `/proc/self/fd` to prove `Close` releases the pipes (`TestCloseReleasesPipeDescriptors`); `mcp` checks that a cancelled call neither waits for nor runs after a busy handler (`TestCancelledCallDoesNotWaitForOrRunAfterBusyHandler`).

## Real MCP calls from the fakes

With `FAKEAGENT_MCP_TOOL=<tool>` (arguments in `FAKEAGENT_MCP_ARGS`, default `{}`) each fake makes a real MCP
`initialize` + `tools/call` over HTTP, so a host can test its MCP server end to end: ACP `tool-turn` (server from
`session/new` or `OPENCODE_CONFIG_CONTENT`), Claude `tool-turn` (tool `mcp__cad__<tool>`, server from the
`--mcp-config` file, called once the permission is allowed) and Codex `mcp-call-turn` (server and token from the
`-c mcp_servers` override and its token env var). `mcpcall.go` holds the shared client. Cad-lite's
`ui/agent_e2e_test.go` builds the fake with `go build` from this module and runs all three through the agent panel.

## Live tests

Gated by `AGENTMANAGER_LIVE=1`; they skip otherwise and need the real CLI and credentials.

## Commands

From `agentmanager/`: `gofmt -l .` (prints nothing), `go vet ./...`, `go test -race ./...`.
