---
id: tech-agent-integration
title: Agent integration
kind: tech
status: current
summary: How go-draw hosts AI agents: the separate agentmanager module, the in-process MCP server `cad`, the rule that only the UI goroutine touches the drawing, and the permission policy.
read_when: you are adding a drawing tool for agents, changing the agent threading or lifecycle, or touching the permission defaults
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/agent.go, ui/agent_e2e_test.go, cadmcp/dispatcher.go, cadmcp/tools.go, cadmcp/render.go, ui/snapshot.go, agentmanager/_docs/api-contract.md, go.mod]
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

## The `cad` MCP server

On Start the panel creates an `mcp.Server` named for go-draw, registers `cadmcp.Register` with an `Env`
(`Dispatch`, `Session`, `Snapshot`, `Changed`), binds it to `127.0.0.1` on a random port and passes its
URL and bearer token to the agent as `agentmanager.MCPServer{Name: "cad"}`. Every request needs the
token; requests with a foreign `Host` or `Origin` are refused. The server lives exactly as long as the
session: Stop, agent exit and quit close it, so the port and token never outlive a chat.

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

## The UI-goroutine rule

gpui and `cmdsys.Session` are single threaded and gpui has no cross-goroutine post. MCP handlers run on
HTTP goroutines, so:

1. Every tool body runs inside `Dispatcher.Do`, which enqueues it in a mutex-guarded mailbox and waits.
   `Do` fails with `ErrUIBusy` after 15 s or when the dispatcher closes.
2. The UI goroutine drains the mailbox: `AgentPanel.pump` is a `Window.After` timer that calls
   `Dispatcher.Drain`, adopts a finished session start, and folds the session's event channel into the
   transcript with non-blocking reads. No goroutine pumps events.
3. The timer re-arms every 16 ms while a session is starting, a turn runs or work is queued, every
   250 ms while an idle session is open (to notice a crashed agent), and not at all with no session.
4. The session launch (`harnesses.Open`, which spawns a process) runs on its own goroutine and hands
   the result back through a mutex-guarded slot that the timer reads.
5. Stop closes the dispatcher first, so waiting tool calls fail fast, then closes the session and the
   server in the background; quit waits for them.

`Env.Changed` runs inside `Do`: it clears the render cache and refreshes the window, so the canvas shows
the agent's edits between frames.

## Permission policy

- The session config carries `agentmanager.AllowMCP("cad")`: requests for the `cad` tools are answered
  without a prompt, preferring allow-once.
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
skip on `js` or when `go` is missing. Interactive behaviour in a window has not been run in the agent
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
