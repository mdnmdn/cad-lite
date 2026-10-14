# agentmanager

A small Go library that starts AI coding agents in structured I/O mode and presents them through
one session API: events out (text, thoughts, tool calls, permission asks, plan, turn end), and
prompt, cancel and permission answers in. It also contains an in-process MCP server so the host
application can give the agent tools.

Standard library only; imports nothing from Cad-lite.

## Packages

| package | role |
|---|---|
| `agentmanager` | neutral types: `Config`, `Session`, `Opener`, `Event` and friends, `AllowMCP`, `Queue` |
| `jsonrpc` | newline-delimited JSON-RPC 2.0 connection (also raw line mode) |
| `mcp` | loopback streamable-HTTP MCP server with bearer token |
| `transcript` | folds events into chat blocks (pure) |
| `store` | directory store of sessions (JSONL) and the open-tab set; replays into a transcript |
| `acp` | ACP client backend (opencode preset) |
| `claude` | Claude Code stream-json backend |
| `codex` | Codex app-server backend |
| `harnesses` | `Open` by harness id, `Available` |
| `internal/proc`, `internal/fakeagent`, `internal/testutil` | process spawning, fake CLIs, test helpers |

## Status

Implemented: root API, `jsonrpc`, `mcp`, `transcript`, `harnesses` and the three backends. Verified live against
Claude Code 2.1.299 only; opencode (ACP) and codex are verified against the fake agent and their wire details are
unverified until run live. See `_docs/roadmap.md` and `_docs/testing.md`.
