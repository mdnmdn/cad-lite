---
id: tech-agent-integration
title: Agent integration
kind: tech
status: current
summary: How go-draw hosts AI agents: the separate agentmanager module, the in-process MCP server `cad`, the rule that only the UI goroutine touches the drawing, and the permission policy.
read_when: you are adding a drawing tool for agents, changing the agent threading or lifecycle, or touching the permission defaults
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/agent.go, ui/agent_store.go, agentmanager/store/store.go, ui/agent_e2e_test.go, cadmcp/dispatcher.go, cadmcp/tools.go, cadmcp/render.go, ui/snapshot.go, agentmanager/_docs/api-contract.md, go.mod, agentmanager/options.go, agentmanager/mcp/server.go, agentmanager/jsonrpc/conn.go, cmdsys/command.go, cmd/cad/main.go, cadmcp/safety_test.go]
depends_on: [tech-architecture, tech-ui-framework, tech-command-system, feat-agent-panel]
review_cycle: monthly
---

# Agent integration

## What it is

Three pieces connect an AI agent to a drawing:

- **`agentmanager/`**, a nested module with its own `go.mod`, stdlib only, importing nothing from
  go-draw. It launches opencode (over ACP), Claude Code (stream-json) and Codex (app-server) as child
  processes and exposes one backend-neutral `Session` (events in, prompts and permission answers out),
  an MCP server package and a transcript model. Like `internal/gpui`, it is a reusable library with its
  own docs, rules and API contract: start at `agentmanager/_docs/api-contract.md`, which is normative.
  go-draw reaches it through a `replace` directive in `go.mod`; `go test ./...` from the root does not
  enter it, so it has its own check (`just check-agentmanager`, see [Operations](operations.md)).
- **`cadmcp/`**, go-draw's glue: the drawing tools as MCP tools, and the `Dispatcher` that runs them on
  the UI goroutine. It imports `cmdsys`, `doc`, `geom` and `agentmanager/mcp`, never the UI.
- **`ui/agent*.go`**, the panel that starts sessions and pumps events; see the
  [agent panel](../features/agent-panel.md).

`agentmanager/store` (stdlib, in the nested module) persists conversations as JSONL per session plus an
open-tab set; go-draw keeps them in `<project>/.go-draw/agent/`. Nothing in this chain exists until the
panel or the `AI` command is first used.

## The `cad` MCP server

The first session start creates one `mcp.Server` named for go-draw, registers `cadmcp.Register` with an `Env`
(`Dispatch`, `Session`, `Snapshot`, `Changed`), binds it to `127.0.0.1` on a random port and passes its
URL and bearer token to the agent as `agentmanager.MCPServer{Name: "cad"}`. Every request needs the
token; requests with a foreign `Host` or `Origin` are refused. One server serves every live session of the workspace (the MCP server is stateless per request): it
starts with the first session and closes when the last one stops, exits or quits, so the port and token
never outlive the chats. Tool calls are serialized by a capacity-one semaphore that honours the request
context, so a cancelled request neither waits for nor runs after a busy handler. The HTTP server sets a
header timeout of 10 s and an `IdleTimeout` of 2 minutes; it sets no write timeout and no batch-length
cap (see the [backlog](../backlog.md)).

The tools (all coordinates in model units, Y up; one tool call is one undo step):

| Tool | Effect |
|---|---|
| `run_script`, `run_command` | Run AutoCAD script text, or one response line to the command line; return the history lines produced |
| `list_commands` | Command names the script accepts |
| `get_drawing`, `list_entities` | Layers, counts, extents, undo state; paginated entity list with geometry |
| `get_selection`, `set_selection` | Read or replace the selection |
| `add_line`, `add_circle`, `add_arc`, `add_polyline`, `add_rectangle` | Create entities on the current layer |
| `erase`, `move`, `rotate`, `scale`, `set_entity_layer` | Edit entities by handle |
| `set_layer`, `create_layer` | Current layer; layer table edits (not undoable) |
| `undo`, `redo`, `zoom_extents` | History and view |
| `measure` | Length, perimeter, area, distance |
| `render_png` | The drawing as a PNG, returned as MCP image content so a vision model can look at it, with a JSON block that maps pixels to model coordinates; views by extents, box, centre and scale, or the user's current view, with optional grid, rulers and selection highlight |

`render_png` is drawn by `ui.Snapshot`, headlessly, from the session; it never captures the window.

**Agents cannot run file or destructive commands.** `cadmcp` runs every tool body through `call`, which
sets `Session.SetAgentCaller(true)` for its duration. A command that implements `cmdsys.HumanOnly` (NEW,
OPEN and SAVE today) is refused with `<NAME> is not available to agents.`, whatever alias or script
reached it, and an agent cannot answer a file prompt that a human started. A future QUIT or SAVEAS opts
in by implementing the interface. `call` also recovers a panic in a tool body, aborts any open
transaction (`doc.Drawing.AbortOpen`) and returns an error, so a bad tool call leaves the drawing usable;
the session's own guard is in the [command system](command-system.md).

**Agents always see English.** While `SetAgentCaller(true)` is on, the session's history lines,
prompts and errors are English whatever the UI language, and keywords match in English only, so an
agent types the English keywords the tool documentation names. Italian command names still resolve
as aliases but are not needed.

## The UI-goroutine rule

gpui and `cmdsys.Session` are single threaded and gpui has no cross-goroutine post. MCP handlers run on
HTTP goroutines, so:

1. Every tool body runs inside `Dispatcher.Do`, which enqueues it in a mutex-guarded mailbox and waits.
   `Do` fails with `ErrUIBusy` after 15 s or when the dispatcher closes.
2. The UI goroutine drains the mailbox: `AgentPanel.pump` is a `Window.After` timer that calls
   `Dispatcher.Drain`, adopts finished session starts, and folds every tab's event channel into its
   transcript and stored record with non-blocking reads, background tabs included. No goroutine pumps
   events. Concurrent sessions therefore interleave only between timer ticks, and tool calls from all
   of them are serialized by the dispatcher and the server.
3. The timer re-arms every 16 ms while any session is starting, a turn runs or work is queued, every
   250 ms while only idle sessions are open (to notice a crashed agent), and not at all with no session.
4. Each session launch (`harnesses.Open`, which spawns a process) runs on its own goroutine and hands
   the result back through a per-tab mutex-guarded slot that the timer reads.
5. Stopping the last live session closes the dispatcher first, so waiting tool calls fail fast, then
   closes the session and the server in the background; quit waits for them. Quit also joins the
   session starts still in flight (bounded at 10 s), and a start that finishes after its tab has moved
   on closes its session instead of leaking the agent process. `cmd/cad` calls `Workspace.CloseAgent`
   before every fatal exit, because `log.Fatal` and `os.Exit` skip deferred calls.

## Persistence and resume

Each tab's events and user messages are appended to its `store.Record` as they are folded into the
transcript (debug log lines excepted); `TurnEnd` and `Exited` fsync. Every backend reports the agent's
own session id in `SessionStarted` (Claude only after the first prompt); the record keeps the latest.
Starting a tab whose record has an id for the same harness sets `Config.ResumeID`: Claude `--resume`,
Codex `thread/resume`, ACP `session/load`. ACP suppresses the history the agent replays on load, which
is why go-draw shows its own stored transcript. The open tabs and the active one are saved on every
change and at quit (`Store.SaveOpen`); restoring them reads records only, so no agent starts until the
user sends. Project switch and the `.go-draw/` layout are in the [agent panel](../features/agent-panel.md).

`Env.Changed` runs inside `Do`: it clears the render cache and refreshes the window, so the canvas shows
the agent's edits between frames.

## Permission policy

- The session config carries `agentmanager.AllowMCP("cad")`: requests for the `cad` tools are answered
  without a prompt, preferring allow-once. The match is strict: when the backend set
  `Call.MCPServer` only an exact match passes; otherwise only the exact title `mcp__cad__<tool>` with a
  bare tool name (`[A-Za-z0-9_-]+`) passes. A call whose kind is anything but other or read is never
  auto-allowed, so a title that merely starts with `cad` does not qualify. The ACP backend derives
  `MCPServer` by the same strict rule (`agentmanager/_docs/api-contract.md`).
- Everything else is a card in the panel with the agent's own options. The panel never answers for the
  user and has no allow-all control.
- go-draw requests no permissive harness mode (no bypass or full-auto flags). Where a protocol allows it,
  the backends run shell and file tools sandboxed read-only; see the agentmanager API contract.

## Verification status

| harness | status |
|---|---|
| Claude Code (`claude`, 2.1.299) | Verified live: start, MCP tool calls through the `cad` server with the auto-allow policy, permission round trips, interrupt. Claude defers MCP tools behind `ToolSearch`, so the first drawing call shows an extra `ToolSearch` tool card in the transcript. |
| opencode (ACP) | Not run live. Verified against the fake agent only; wire details are marked unverified in `agentmanager/_docs/protocols.md`. |
| Codex (`codex`) | Not run live. Verified against the fake agent only; wire details unverified, as above. |

`ui/agent_e2e_test.go` runs all three backends through the real panel, the real `cad` MCP server and the
real `Dispatcher`, against `agentmanager`'s fake agent (built with `go build` from the nested module). The
fake makes a real MCP `add_line` call; the test pumps the UI timer until the drawing has the line and the
transcript's tool card is completed. A second test calls `render_png` over HTTP and decodes the PNG. They
skip on `js` or when `go` is missing. `cadmcp/safety_test.go` holds the agent-safety tests: scripts cannot
reach NEW, OPEN or SAVE, an agent cannot answer a human's file prompt, and a panicking command does not
brick the drawing. Interactive behaviour in a window has not been run in the agent
environment (see [Operations](operations.md)).

ACP decisions that affect the panel: the history an agent replays during `session/load` is suppressed, so a
resumed session does not repeat old messages; when an opencode build lacks HTTP MCP support, its
environment-variable fallback restarts the agent process once, so the session may start slowly.

## Build constraint

`agentmanager` and `cadmcp` compile for `js/wasm` (the launch fails at run time there), so the browser
build keeps building and the panel reports the error on Start.

## Related docs

- [Agent panel](../features/agent-panel.md) — what the user sees
- [Architecture](architecture.md) — where `agentmanager`, `cadmcp` and `ui` sit
- [Command system](command-system.md) — the session the tools drive
- [Decision register](decisions.md) — why a nested module, loopback HTTP and these permission defaults
