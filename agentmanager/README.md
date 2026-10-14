# agentmanager

A small Go library that starts AI coding agents in structured I/O mode and presents them through
one session API: events out (text, thoughts, tool calls, permission asks, plan, turn end), and
prompt, cancel and permission answers in. It also contains an in-process MCP server so the host
application can give the agent tools.

Standard library only; imports nothing from go-draw.

## Packages

| package | role |
|---|---|
| `agentmanager` | neutral types: `Config`, `Session`, `Opener`, `Event` and friends, `AllowMCP`, `Queue` |
| `jsonrpc` | newline-delimited JSON-RPC 2.0 connection (also raw line mode) |
| `mcp` | loopback streamable-HTTP MCP server with bearer token |
| `transcript` | folds events into chat blocks (pure) |
| `acp` | ACP client backend (opencode preset) |
| `claude` | Claude Code stream-json backend |
| `codex` | Codex app-server backend |
| `harnesses` | `Open` by harness id, `Available` |
| `internal/proc`, `internal/fakeagent`, `internal/testutil` | process spawning, fake CLIs, test helpers |

## Status

Core done: neutral types, `Queue`, `AllowMCP`, `jsonrpc`, `internal/proc`, fake-agent framework and `harnesses`. Backends (`acp`, `claude`, `codex`), `mcp` and `transcript` are owned by other work packages and may still be stubs. See `_docs/testing.md`.
