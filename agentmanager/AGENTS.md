# AGENTS.md (agentmanager)

Guidance for AI agents working in `agentmanager/`, a standalone Go library that launches coding
agents (opencode, Claude Code, Codex) and exposes one neutral session API. Plan:
`../_docs/inbox/completed/agent-manager-port.md`. Frozen API: `_docs/api-contract.md`. Start at `_docs/index.md`.

## Rules

1. **Stdlib only.** Module `agentmanager` has no requirements. Never import `go-draw/...`; the
   nested module makes that a compile error, keep it so. go-draw glue lives outside this tree.
2. **Contract first.** An exported signature change edits `_docs/api-contract.md` (and the plan's
   section 4.3 if still current) before the code. Work packages do not change another package's
   exports; stop and report instead.
3. **Disjoint files.** Stay inside the files your brief owns (see the plan's section 7). A transient
   build break from another agent's in-flight edit usually clears in a minute.
4. **Style.** Doc comment on every exported identifier; other comments only when non-obvious. Every
   inbound JSON-RPC request gets exactly one reply; every blocking wait has a timeout.
5. **No window, no real CLIs.** Test with the fake agent (`internal/fakeagent`, built by
   `internal/testutil.BuildFake`), `io.Pipe` pairs and `net/http` clients. Live tests are gated by
   `AGENTMANAGER_LIVE=1` and skip otherwise; say which were not run.
6. **Verification before done**, from `agentmanager/`: `gofmt -l .` prints nothing, `go vet ./...`,
   `go test -race ./...`. From the root: `just check-agentmanager` (the root `./...` skips this module).
7. **Bug fixes get a regression test** that fails first. Docs ride with the change (`_docs/`).
8. **Never log tokens.** Bearer tokens and MCP config files (0600) stay out of logs and argv.
