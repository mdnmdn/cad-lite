# Roadmap

Status of the agent manager and what is left.

## Status

All packages are implemented: root API, `jsonrpc`, `mcp`, `transcript`, `harnesses`, `acp` (with the OpenCode
preset), `claude`, `codex`, and the fake agents. `go vet` and `go test -race ./...` pass. go-draw uses the
module through `ui/agent*.go`; its end-to-end test drives all three backends against the fake with a real MCP
call.

## Verified live

| harness | status |
|---|---|
| claude (2.1.299) | verified: init after the first line, partial deltas without duplicated text, `mcp__cad__*` through our `mcp.Server` with bearer auth and `AllowMCP` auto-allow, allow and deny round trips, interrupt then a new prompt. Findings in `protocols.md` |
| opencode (ACP) | not run live (no CLI available); wire details are marked unverified in `protocols.md` |
| codex | not run live (no CLI available); wire details are marked unverified in `protocols.md` |

## Next

1. A usage event: `TurnEnd` has no usage field today, so Claude's `result.usage` only reaches `Logger.Debugf`.
2. A live test of `Config.ResumeID` against the real CLI (claude `--resume`, ACP `session/load`).
3. A kill-on-close test for claude against the real process (the fake `hang` scenario covers it only in theory of the
   process group).
4. Live runs of opencode and codex, then remove the unverified marks.
