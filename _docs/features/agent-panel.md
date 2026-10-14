---
id: feat-agent-panel
title: Agent panel
kind: feature
status: current
summary: The docked chat panel that runs an AI coding agent (opencode, Claude Code or Codex) against the open drawing, shows its transcript and asks the user before anything beyond drawing is allowed.
read_when: you are changing the agent panel, its focus or permission behaviour, the agent flags, or what an agent may do to the drawing
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/agent.go, ui/agent_view.go, ui/agent_demo.go, ui/workspace.go, cmd/cad/main.go]
depends_on: [tech-agent-integration, feat-command-line, tech-ui-framework]
review_cycle: monthly
---

# Agent panel

## Purpose

The user chats with an AI agent that draws for them. The agent edits the same drawing through the
same commands the command line runs, so every change is one undo step and shows on the canvas as it
happens. The panel is a transcript, a prompt field and the controls to start, stop and cancel.

## Behaviour

- **Toggle.** The `Agent` button at the right end of the toolbar and **F2** show and hide the panel,
  which docks to the right of the layers panel. Hiding it leaves a running session alive.
- **Picker.** One chip per agent CLI found on `PATH` (`opencode`, `claude`, `codex`). The chips lock
  while a session runs. With none found the panel says so; `-agent` with `-agent-cmd` offers the
  named harness regardless of `PATH`.
- **Start and Stop.** Start launches the selected agent with the drawing's MCP server attached and
  shows `starting...`, then `ready`. Stop closes the session; the transcript stays. The session also
  ends, with a note, when the agent process exits. Sending a prompt with no session starts one and
  sends the prompt once it is connected.
- **Prompt.** One line; Enter or Send submits. Only one turn runs at a time, so Send does nothing
  while the agent works. **Cancel** appears while a turn runs and stops it; the session stays usable.
- **Transcript.** User messages, agent messages and notes in order; thoughts collapsed behind a
  `Thinking` line that expands on click; tool calls as cards with a status dot (pending, in progress,
  completed, failed) and a clipped result; the agent's plan with a mark per step; debug log lines are
  hidden.
- **Permissions.** The agent's drawing tools (MCP server `cad`) run without asking. Everything else
  (shell, file edits, other MCP servers) shows a card with one button per option the agent offers;
  the card stays until answered. There is no switch that allows all.
- **Focus.** The command line keeps the keyboard by default. Clicking the prompt field moves it to the
  field; **Escape** in the field, clicking the canvas or hiding the panel gives it back to the command
  line. Text typed into the prompt never reaches the command line.
- **Quit.** Closing the application stops the session, the agent process and the MCP server.

## Contract

- Flags `-agent opencode|claude|codex` (select and open), `-agent-cmd PATH` (executable instead of the
  one on `PATH`) and `-agent-demo` (open on a canned transcript, for `-png`) are listed in
  [Operations](../tech/operations.md).
- Panel API: `Workspace.AgentPanel`, `AgentPanel.Configure`, `Start`, `Stop`, `Send`, `Cancel`,
  `Respond`, `SetOpener`; `Workspace.CloseAgent`; action `cad-toggle-agent` bound to F2.
- What the agent may do to the drawing is the tool set in [Agent integration](../tech/agent-integration.md).

## Implementation

`ui/agent.go` holds `AgentPanel`: state (idle, starting, ready), `Start` (MCP server, `agentmanager.Config`,
launch on a goroutine), the timer `pump` and `Stop`. `ui/agent_view.go` renders the header, the
transcript (`agentmanager/transcript`) and the prompt row. `ui/workspace.go` hosts the panel and the
toggle. `ui/agent_demo.go` seeds the canned transcript. Threading and lifecycle rules are in
[Agent integration](../tech/agent-integration.md).

## Failure

- A harness that fails to start (missing binary, handshake error) ends the turn with the error as a note
  and leaves the panel idle; a queued prompt is dropped.
- A tool call that the UI goroutine cannot run in 15 s returns a busy error to the agent.
- A failed prompt write ends the turn with the error as a note.
- In the browser build no agent can start; the panel reports the launch error.

## Related docs

- [Agent integration](../tech/agent-integration.md) — the agentmanager boundary, the MCP server and the threading rule
- [Command line](command-line.md) — the other keyboard input and the F-key shortcuts
- [UI framework and theme](../tech/ui-framework.md) — the status colours the panel uses

## Next steps

- A multi-line prompt field, once `TextInput` supports it.
- Resume a previous session and a model picker.
- Persist the transcript with the drawing.
