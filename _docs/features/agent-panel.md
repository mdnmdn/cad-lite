---
id: feat-agent-panel
title: Agent panel
kind: feature
status: current
summary: The on-demand docked chat panel that runs AI coding agents (opencode, Claude Code or Codex) against the open drawing, as several tabs whose conversations are stored per project, and asks the user before anything beyond drawing is allowed.
read_when: you are changing the agent panel, its focus or permission behaviour, the agent flags, or what an agent may do to the drawing
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/agent.go, ui/agent_store.go, ui/agent_view.go, ui/agent_demo.go, ui/workspace.go, cmdsys/commands/agent.go, agentmanager/store/store.go, cmd/cad/main.go, cmdsys/command.go]
depends_on: [tech-agent-integration, feat-command-line, tech-ui-framework]
review_cycle: monthly
---

# Agent panel

## Purpose

The user chats with an AI agent that draws for them. The agent edits the same drawing through the
same commands the command line runs, so every change is one undo step and shows on the canvas as it
happens. The panel is a transcript, a prompt field and the controls to start, stop and cancel.

## Behaviour

- **On demand.** Nothing agent related runs or exists until the user asks: no panel object, no MCP
  server, no process, no timer and no files. Three things ask: the `Agent` button at the right end of
  the toolbar, **F2**, and the `AI` command (alias `AGENT`). The first two show and hide the panel, which
  docks to the right of the layers panel. `AI` prompts `Ask the agent`, which takes free text (Space
  does not submit it): text opens the panel and sends it to the active tab, starting a session if the
  tab has none; an empty answer toggles the panel. A prompt sent while the tab's agent works is refused
  with a note. Hiding the panel leaves running sessions alive; a dot on the `Agent` button shows that
  one is live (green) or that a background tab needs attention (amber).
- **Tabs.** Each conversation is a tab with its own transcript, stored record and agent session; any
  number run at once, each on any installed harness. `+` adds an empty tab, a tab's `x` closes its
  session and removes it (the record stays in the history), clicking a tab switches to it while the
  others keep running and are drained and stored in the background. A tab's dot is green while its
  session is live, blue while a turn runs and amber when a background tab asked for permission or
  finished a turn; switching to it clears the amber. The harness chips apply to a tab without a stored
  conversation; afterwards the tab is fixed to the harness it started with. All sessions share one `cad`
  MCP server, started with the first session and closed with the last; tool calls still run one at a
  time on the UI goroutine.
- **History.** `History` lists the project's stored conversations, newest first, with title (the first
  prompt), harness and relative date; choosing one opens it in a tab, or switches to it if it is open.
- **Per-project storage.** The project is the directory of the drawing file last given to `-open`,
  OPEN or SAVE, else the working directory. Conversations live in `<project>/.cad-lite/agent/` (copied once from the old `.go-draw/agent/` when only that exists), one
  JSONL file per conversation, created with the first prompt (directory 0700, files 0600). Opening the
  panel restores the tabs that were open when the project was last used, with their transcripts,
  read-only; no agent process starts. Sending to a restored tab starts its session with the stored agent
  session id, so the agent continues its own context (Claude `--resume`, Codex `thread/resume`, ACP
  `session/load`, whose replayed history is suppressed because the stored transcript is shown). A
  session the agent has lost ends with an error note; open a new tab to continue. OPEN or SAVE of a
  file in another directory closes every live session, keeps their records and switches to that project's
  tabs.
- **Default harness.** The `agent.default` [setting](settings.md) selects the harness at startup and when
  changed in the settings panel (`AgentPanel.Configure`); empty leaves the choice to `PATH`.
- **Harness chips.** One chip per agent CLI found on `PATH` (`opencode`, `claude`, `codex`). The chips lock
  while the tab has a session or a stored conversation. With none found the panel says so; `-agent` with `-agent-cmd` offers the
  named harness regardless of `PATH`.
- **Start and Stop.** Start launches the selected agent with the drawing's MCP server attached and
  shows `starting...`, then `ready`. Stop closes the session; the transcript stays. The session also
  ends, with a note, when the agent process exits. Sending a prompt with no session starts one and
  sends the prompt once it is connected.
- **Tooltips and language.** Every button, tab, chip and history row has a tooltip, and every label,
  status line (`starting claude...`, `ready`), warning and relative date comes from the catalog
  (`ui.agent.*`), so the panel follows the interface language. Text sent to and received from the
  agent (prompts, tool names, permission option names) is not translated.
- **Prompt.** One line; Enter or Send submits. Only one turn runs at a time, so Send does nothing
  while the agent works. **Cancel** appears while a turn runs and stops it; the session stays usable.
- **Transcript.** User messages, agent messages and notes in order; thoughts collapsed behind a
  `Thinking` line that expands on click; tool calls as cards with a status dot (pending, in progress,
  completed, failed) and a clipped result; the agent's plan with a mark per step; debug log lines are
  hidden.
- **Permissions.** The agent's drawing tools (MCP server `cad`) run without asking, matched strictly:
  by the server the backend reports, or by the exact title `mcp__cad__<tool>`, and only for calls of
  kind other or read. Everything else (shell, file edits, other MCP servers) shows a card with one
  button per option the agent offers; the card stays until answered. There is no switch that allows all.
- **Human-only commands.** An agent cannot run NEW, OPEN or SAVE, through a tool, a script or an alias;
  the history reads `<NAME> is not available to agents.` Only the user opens, saves or discards a drawing.
- **Focus.** The command line keeps the keyboard by default. Clicking the prompt field moves it to the
  field; **Escape** in the field, clicking the canvas or hiding the panel gives it back to the command
  line. Text typed into the prompt never reaches the command line.
- **Quit.** Closing the application stops every session, the agent processes and the MCP server, and
  saves which tabs were open. A session still starting when the application quits is waited for (up to
  10 s) and closed, and a fatal startup error stops the agent before the process exits.

## Contract

- Flags `-agent opencode|claude|codex` (select and open), `-agent-cmd PATH` (executable instead of the
  one on `PATH`) and `-agent-demo` (open on a canned transcript, for `-png`) are listed in
  [Operations](../tech/operations.md).
- Panel API: `Workspace.AgentPanel` (creates the panel), `Workspace.HasAgentPanel`,
  `Workspace.SetProjectPath`, `AgentPanel.Configure`, `Start`, `Stop`, `Send`, `Cancel`, `Respond`,
  `SetOpener`, `Tabs`, `NewTab`, `SwitchTab`, `CloseTab`, `OpenSession`, `Sessions`; `Workspace.CloseAgent`;
  action `cad-toggle-agent` bound to F2. Command `AI` reaches the panel through
  `cmdsys.Session.SetAgentHandler` (the `OPTIONS` command is wired the same way through `SetSettingsHandler`); OPEN and SAVE report the file through `cmdsys.Session.OnFilePath`.
- Files: `<project>/.cad-lite/agent/<id>.jsonl` and `open.json` (the open tabs), format in the store's
  section of the agentmanager API contract.
- What the agent may do to the drawing is the tool set in [Agent integration](../tech/agent-integration.md).

## Implementation

`ui/agent.go` holds `AgentPanel` and `agentTab` (transcript, record, session state per conversation):
`startTab` (shared MCP server, `agentmanager.Config` with `ResumeID`, launch on a goroutine), the timer
`pump` over every tab and `stopTab`. `ui/agent_store.go` holds the persistence glue: project directory,
`ensureLoaded` (open set restore), tabs, history and `apply`, which folds each event into the
transcript and `agentmanager/store`. `ui/agent_view.go` renders the header, tabs, history menu,
transcript (`agentmanager/transcript`) and prompt row. `ui/workspace.go` creates the panel lazily and
hosts the toggle and `agentCommand`; `cmdsys/commands/agent.go` is the `AI` command. `ui/agent_demo.go`
seeds the canned transcript. Threading and lifecycle rules are in
[Agent integration](../tech/agent-integration.md).

## Failure

- A history directory that cannot be created or written shows one warning in the transcript; the
  conversation continues unsaved. A stored file with a truncated last line loses only that line.
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
- A model picker.
- Delete or rename a stored conversation from the history menu.
- A live resume test against the real CLIs.
