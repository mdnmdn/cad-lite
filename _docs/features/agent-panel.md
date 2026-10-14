---
id: feat-agent-panel
title: Agent panel
kind: feature
status: current
summary: The on-demand docked chat panel that runs AI coding agents (opencode, Claude Code or Codex) against the open drawing, as several tabs on a model the user picks (free models first on opencode), with images dropped or pasted into the prompt, whose conversations are stored with the drawing in its project, and asks the user before anything beyond drawing is allowed.
read_when: you are changing the agent panel, its model picker, image attachments, where its conversations are kept, its focus or permission behaviour, the agent flags, or what an agent may do to the drawing
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [ui/doctabs.go, ui/agent.go, ui/agent_markdown.go, ui/agent_markdown_view.go, ui/agent_message.go, ui/agent_queue.go, ui/agent_working.go, ui/agent_model.go, ui/agent_image.go, ui/agent_install.go, agentinstall/agentinstall.go, ui/discovery.go, ui/agent_store.go, ui/agent_view.go, ui/agent_demo.go, ui/workspace.go, ui/layout.go, cmdsys/commands/agent.go, agentmanager/store/store.go, cmd/cad/main.go, cmdsys/command.go]
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
  server, no process, no timer and no files. Three things ask: the `Agent` icon button at the right end of
  the title bar, **F2**, and the `AI` command (alias `AGENT`). The first two show and hide the panel, which
  docks to the right of the layers panel and runs from under the ribbon to the status bar. Its width defaults to a quarter of the window and the user drags it at its left edge ([Drawing canvas](canvas.md) has the limits); the `>` button at the top hides it. `AI` prompts `Ask the agent`, which takes free text (Space
  does not submit it): text opens the panel and sends it to the active tab, starting a session if the
  tab has none; an empty answer toggles the panel. A prompt sent while the tab's agent works is queued,
  as from the composer. Hiding the panel leaves running sessions alive; a dot on the `Agent` button shows that
  one is live (green) or that a background tab needs attention (amber).
- **Tabs.** Each conversation is a tab with its own transcript, stored record and agent session; any
  number run at once, each on any installed harness. `+` adds an empty tab, a tab's `x` closes its
  session and removes it (the record stays in the history), clicking a tab switches to it while the
  others keep running and are drained and stored in the background. A tab's dot is green while its
  session is live, blue while a turn runs and amber when a background tab asked for permission or
  finished a turn; switching to it clears the amber. The harness chips apply to a tab without a stored
  conversation; afterwards the tab is fixed to the harness it started with. The sessions of one drawing
  share one `cad` MCP server, started with its first session and closed with its last; tool calls still
  run one at a time on the UI goroutine.
- **History.** `History` lists the project's stored conversations, newest first, with title (the first
  prompt), harness and relative date, and the drawing's file name for a conversation of another drawing;
  choosing one opens it in a tab, or switches to it if it is open.
- **Model.** A chip after the harness chips (alone on the row once the harness is fixed) shows the active tab's model: the chosen model's name, else
  the harness default from the settings, else the model the agent reported, else `default`. Clicking it
  opens a menu: *Default*, the models the agent reported (Claude's `initialize` list, ACP's session
  models, Codex's `model/list`; the list arrives once a session runs and is stored with the
  conversation), a field for any other model id (Enter applies, Escape closes), and *Make default for
  <harness>*, which saves the tab's choice as `agent.model_<harness>` in the [settings](settings.md). The
  choice is per tab and stored with its conversation, so a restored tab keeps it. A tab without a session
  starts on it. A live session switches from the next turn when the harness can (Claude `set_model`, ACP
  `session/set_config_option` or `session/set_model`, Codex's `turn/start` model); otherwise, and when
  going back to *Default*, a note says so and the session ends (after the running turn), and the next
  prompt starts it again on the new model, resuming the conversation. A model the agent refuses is
  reported in the transcript and the previous choice comes back. The chip shows a model's name, else its
  id without the `<provider>/` prefix opencode ids carry.
- **Free models on opencode.** On the opencode harness the menu lists the free models first, each with a
  green `free` badge: `big-pickle`, then the ids ending in `-free`, in the agent's order (with or without
  the provider prefix, e.g. `opencode/big-pickle`, `opencode/grok-code-fast-1-free`), then the others. Until
  the agent has listed its models the menu offers `opencode/big-pickle`. A new opencode tab, with no model
  picked and no `agent.model_opencode` default, starts on `big-pickle` (the agent's own entry when it
  listed one, else `opencode/big-pickle`), or on the first free model the agent lists when big-pickle is
  not among them; the choice is stored with the conversation like a picked one. Picking *Default* means
  opencode's own default. The ids are opencode's `<provider>/<model>` as its ACP model list reports them;
  that `big-pickle` and the `-free` models are free on OpenCode Zen is an assumption, not checked live.
- **Images.** Image files dropped anywhere on the panel, or an image pasted (Cmd/Ctrl+V with no text on the
  clipboard) while the prompt field has the keyboard, wait above the prompt as chips: a preview, the file
  name (`pasted image` for a paste), the pixel size and an `x` that removes the image. They go with the
  next prompt, which may then be empty, and the sent message shows them as small previews. A drop on the
  panel never opens a drawing; files that are not PNG, JPEG, GIF or WebP are reported in the transcript.
  An image wider or taller than 2048 px is scaled down to fit (a JPEG stays JPEG, other types become PNG),
  and one still over 5 MB is refused, as is one whose header announces more than 50 megapixels (checked
  before decoding); at most six images go with one message. The tab's model must read
  images: Claude's always do; a model the agent lists as text-only (Codex's `inputModalities`), or an
  agent that takes no images at all (ACP without `promptCapabilities.image`), refuses the attachment with a
  note, and a prompt with images for such a model is refused, keeping text and images. Where the agent
  does not say (opencode's models over ACP), the images go and opencode answers for its model. Pasting a
  file path pastes the text; a file is attached by dropping it.
- **Storage with the drawing.** The project is the directory of the drawing file last given to `-open`,
  OPEN or SAVE, else the working directory. Conversations live in `<project>/.cad-lite/agent/` (copied once
  from the old `.go-draw/agent/` when only that exists), one JSONL file per conversation, created with the
  first prompt (directory 0700, files 0600) and tagged with the drawing's file name. A message's images
  are stored as their name, type, size and a PNG preview of at most 96 px inside its line, not the files;
  the agent's own session keeps what it was sent. Each drawing remembers
  its own open tabs; an untitled drawing has the project's untitled set. Opening the panel restores the
  tabs that were open when the drawing was last used, with their transcripts, read-only; no agent process
  starts. Sending to a restored tab starts its session with the stored agent session id, so the agent
  continues its own context (Claude `--resume`, Codex `thread/resume`, ACP `session/load`, whose replayed
  history is suppressed because the stored transcript is shown). A session the agent has lost ends with an
  error note; open a new tab to continue.
- **Document tabs.** The panel shows the conversations of the active [document tab](document-tabs.md):
  switching tabs shows the other drawing's conversations (restored from the store the first time, none
  for a drawing never used with the agent) while the sessions of the tab left keep running there. A
  session is bound to the document tab it was started in: its `cad` tools act on that drawing whichever
  tab is active (decision D32). Closing a document tab ends its sessions and keeps their records. OPEN
  into the untouched untitled tab closes that tab's live sessions and shows the opened drawing's tabs. SAVE or SAVEAS under a new name takes the tabs along: in the same
  directory the sessions keep running and reopening the old name shows no tabs; in another directory the open
  conversations are copied into that project's store, their sessions close and the tabs reappear there
  (the originals stay in the old project's History). The first save of an untitled drawing works the same
  way. A store from before per-drawing tabs gives its open tabs to the first drawing opened.
- **Default harness and model.** The `agent.default` [setting](settings.md) selects the harness at startup and when
  changed in the settings panel (`AgentPanel.Configure`); empty picks the first agent found (see PATH below).
  `agent.model_<harness>` is the model a tab without its own choice starts with on that harness.
- **Harness chips.** One chip per agent CLI found on the effective PATH (`opencode`, `claude`, `codex`): the app's own PATH plus the one the user's login shell reports, probed in the background at startup (chips appear when the probe ends) and again by the settings panel's Check button. An agent whose executable the user chose in the settings (*Executables*, see [Settings](settings.md)) gets a chip while that file exists, and starts from that file. Once the tab's
  harness is fixed (a session starting or open, or a stored conversation, which includes a restored or
  reopened one) the chips are hidden and the header names the harness after the title (`Agent · claude`,
  muted, with a tooltip); the model chip and the Start/Stop button stay on that row. With none found the panel says so; `-agent` with `-agent-cmd` offers the
  named harness regardless of `PATH`.
- **Install OpenCode.** With no agent found, the panel shows an *Install OpenCode* card under the
  warning (the settings panel's Agent discovery section shows the same card whenever OpenCode is not
  found). Nothing runs until the user clicks *Install OpenCode…* and then *Install* on the question that
  follows, which says what is downloaded (mise, about 50 MB, from github.com/jdx/mise, checked against
  the release's SHA256 list; then OpenCode's release binary, about 60 MB, from github.com/anomalyco/opencode; about 350 MB on disk in all; OpenCode's npm package, up to about 1 GB, only when the binary cannot be fetched and Node.js is installed), the folder it all goes into, that
  nothing is installed system-wide, and that a terminal opens for sign-in; the other button closes it without installing. While it
  runs, the card lists three steps (download and verify mise, with a bar and megabytes; install OpenCode;
  check the sign-in) and a *Cancel* button, which stops the running step and keeps nothing
  half-downloaded. On success the installed OpenCode becomes its executable (`agent.binaries`, as if
  chosen with Browse, so a restart keeps it), its chip appears, and when `opencode auth list` shows no
  credential a terminal opens running `opencode auth login`; the card says which terminal opened, and
  *Sign in…* opens it again. A failure shows the error with *Retry* and *Close*. Quitting the application cancels a running installation and waits for it to stop its processes and remove what it half-wrote.
- **Start and Stop.** Start launches the selected agent with the drawing's MCP server attached and
  shows `starting...`, then `ready`. **Stop** (a square icon on the harness row) ends the session; the transcript
  stays. Stop and Cancel differ: Stop = end the session, Cancel = cancel the running turn and keep the
  session. Stopping during a turn ends that turn as cancelled and drops the queue with a note.
  While a turn runs, the status pill shows `working...` with a small rubber-band animation (a dashed
  line following a sweeping crosshair, a few 1 px strokes) in place of its dot; it is drawn only while
  a turn runs. While nothing arrives the panel redraws at the animation's 10 frames a second (it
  polls the agent every 16 ms only right after events arrive); an idle open session polls every
  250 ms. The session also
  ends, with a note, when the agent process exits. Sending a prompt with no session starts one and
  sends the prompt once it is connected.
- **Tooltips and language.** Every button, tab, chip and history row has a tooltip, and every label,
  status line (`starting claude...`, `ready`), warning and relative date comes from the catalog
  (`ui.agent.*`), so the panel follows the interface language. Text sent to and received from the
  agent (prompts, tool names, permission option names) is not translated.
- **Prompt.** Multi-line: Enter or Send submits, Shift+Enter starts a new line, and pasted text
  keeps its line breaks. Long lines wrap at the panel's width; the field grows with its text up to
  eight lines and then scrolls, keeping the caret in view. Up/Down move between lines and Home/End
  go to the ends of the line. While a turn runs, **Send** becomes **Cancel** (a cross in a circle),
  which cancels the turn; the session stays usable. Once something is typed during a turn the button
  becomes **Queue**, with Cancel beside it as an icon button. While the session is still starting,
  Cancel abandons the start, so the session and its queue go.
- **Queue.** A prompt sent while a turn runs (Queue, Enter or the `AI` command) is queued with the
  images in the composer, and the composer clears. Queued prompts show above the composer as numbered
  chips, each with an `x` that removes it; they are sent one at a time, in order, each when the turn
  before it ends, also when a prompt fails to send. Cancel pauses the queue: the chips stay and a
  **Send next** chip appears, and nothing is sent until the user presses it or sends another prompt,
  which joins the end of the queue and resumes it (a prompt sent while anything is queued always
  goes behind it, so the order holds). Queued prompts are not merged, belong to their conversation
  tab, survive the restart a model switch causes with their paused state, and are dropped with a
  note when the session ends.
  A repeated turn-end from the agent moves the queue on once.
- **Transcript.** User messages, agent messages and notes in order; thoughts collapsed behind a
  `Thinking` line that expands on click; tool calls as cards with a status dot (pending, in progress,
  completed, failed) and a clipped result; the agent's plan with a mark per step; debug log lines are
  hidden. User messages are boxes inset 40 px from the left, as in a chat, and stay plain text.
  Agent messages and expanded thoughts render Markdown: headings, bold, italic (gpui has no italic
  faces, so emphasis shows in a softer colour), strikethrough, inline code and fenced code blocks
  (monospace on a tint; long lines wrap, since a sideways-scrolling block would take the wheel from the
  transcript), bullet and numbered lists (nested), block quotes, rules, tables (equal-width columns,
  header and alignment) and links. A link shows in the accent colour with its URL as tooltip; clicking
  opens it in the browser, https only (anything else is reported with a note). A URL holding bidi
  control characters is not a link; a bare URL keeps balanced parentheses (`Foo_(bar)`) and loses a
  closing parenthesis or punctuation that wraps it. Markdown still streaming in renders as far as it
  goes, re-parsed at most every 100 ms while the turn runs: an unclosed code fence is an open code
  block, unclosed emphasis or a half-written link shows as typed, and a table appears once its
  delimiter row arrives. Quotes and lists nest at most 16 deep; deeper text is a paragraph.
  Each user and agent message has a copy button that puts its source (the raw Markdown for the agent)
  on the clipboard and turns into a check mark for 1.5 s; a failed clipboard write is reported with a
  note. A tool card's parameters (its input, as indented JSON) are collapsed; clicking the card's
  header, which keeps the dot, title, status and a chevron, opens or closes them. The result stays
  clipped and visible, as the status it explains. The transcript follows its end whenever it changes. However long a conversation grows, only
  the blocks near the visible part are built and laid out; the others keep their last measured
  height (or an estimate until first shown), so the scroll range stays right.
- **Permissions.** The agent's drawing tools (MCP server `cad`) run without asking, matched strictly:
  by the server the backend reports, or by the exact title `mcp__cad__<tool>`, and only for calls of
  kind other or read. Everything else (shell, file edits, other MCP servers) shows a card with one
  button per option the agent offers; the card stays until answered. There is no switch that allows all.
- **Human-only commands.** An agent cannot run NEW, OPEN or SAVE, through a tool, a script or an alias;
  the history reads `<NAME> is not available to agents.` Only the user opens, saves or discards a drawing.
- **Focus.** The command line keeps the keyboard by default. Clicking the prompt field moves it to the
  field; **Escape** in the field, clicking the canvas or hiding the panel gives it back to the command
  line. Text typed into the prompt never reaches the command line.
- **Quit.** Closing the application stops every session of every document tab, the agent processes and the MCP server, and
  saves which tabs were open. A session still starting when the application quits is waited for (up to
  10 s) and closed, and a fatal startup error stops the agent before the process exits.

## Contract

- Flags `-agent opencode|claude|codex` (select and open), `-agent-cmd PATH` (executable instead of the
  one on `PATH`) and `-agent-demo` (open on a canned transcript, for `-png`) are listed in
  [Operations](../tech/operations.md).
- Panel API: `Workspace.AgentPanel` (creates the panel), `Workspace.HasAgentPanel`,
  `Workspace.SetProjectPath`, `AgentPanel.Configure`, `Start`, `Stop`, `Send`, `Cancel`, `Respond`,
  `SetOpener`, `SetClipboard`, `SetLinkOpener`, `Queued`, `Unqueue`, `Tabs` (`Tab.Model` is the chip's label), `NewTab`, `SwitchTab`, `CloseTab`, `OpenSession`,
  `Sessions`, `SetModel`, `SetDefaultModel`, `Models`, `AttachFiles`, `AttachImage`, `Attachments`,
  `RemoveAttachment`; `Workspace.CloseAgent`; `Workspace.SetAgentInstaller`
  (`ui.AgentInstaller`), `AgentInstalling`;
  action `cad-toggle-agent` bound to F2. Command `AI` reaches the panel through
  `cmdsys.Session.SetAgentHandler` (the `OPTIONS` command is wired the same way through `SetSettingsHandler`); OPEN and SAVE report the file through `cmdsys.Session.OnFilePath`.
- Files: `<project>/.cad-lite/agent/<id>.jsonl`, `open.json` (the untitled drawing's open tabs) and
  `open-<hash>.json` (one drawing's open tabs, keyed by its file name), format in the store's section of the
  agentmanager API contract. The design is [decision](../tech/decisions.md) D29; model switching is D30;
  images and the free opencode default are D33.
- Images reach the agent through `agentmanager.ImagePrompter`; drops through `gpui.Div.OnFileDrop` and
  pastes through `gpui.ImagePasteEvent` on the panel's root.
- What the agent may do to the drawing is the tool set in [Agent integration](../tech/agent-integration.md).

## Implementation

`ui/agent.go` holds `AgentPanel`, `agentSet` (one document tab's conversations, history state and MCP
server; the panel embeds the active tab's and `follow` swaps it) and `agentTab` (transcript, record,
session state per conversation): `ensureServer` (the set's MCP server, whose session is the document
tab's), `startTab` (`agentmanager.Config` with `ResumeID`, launch on a goroutine), the timer `pump` over
every tab of every set and `stopTab`. `ui/agent_store.go` holds the persistence glue: project directory,
`ensureLoaded` (open set restore), tabs, history and `apply`, which folds each event into the
transcript and `agentmanager/store`, and `drawingChanged`, which `Workspace.pathChanged` calls with whether
the drawing was saved (the same `doc.Drawing`) or replaced, for the active document tab only. `ui/agent_model.go` holds the model state per
tab (`tabModel`), `SetModel`/`applyModel` (a `ModelSetter` switch on a goroutine, or a restart), the chip
and the menu, with `orderModels`, `freeModel` and `suggestedModel`/`commitSuggestion` for opencode's free
models. `ui/agent_image.go` holds the attachments: `newAgentImage` (type check, downscale, preview),
`visionOf`, `attach`, the panel's `onFileDrop` and `onImagePaste`, the composer chips and the transcript
previews (painted with `gpui.Window.PaintImage`); `AgentPanel.Send` and `prompt` (`ui/agent.go`) send them
with `ImagePrompter.PromptImages`. `ui/agent_view.go` renders the header, tabs, history menu,
transcript (`agentmanager/transcript`) and prompt row; `transcriptList` makes the transcript a
`gpui.List` over the shown blocks, backed by the tab's `gpui.ListState` (`agentTab.list`) and keyed by
`transcript.T.Revision` plus the thought's expanded bit, so a block that changed off-screen is measured
again; `checkLook` drops every tab's cached heights when the language, theme or UI scale changes;
`composer` swaps Send for Cancel and Queue while a turn runs. `ui/agent_markdown.go` is the Markdown parser
(`parseMarkdown`, blocks and inline spans, lenient for streaming; `inlineInto` caches suffix
searches so unfinished link and autolink openers do not repeatedly scan the message;
`mergeSpans` builds each adjacent run of the same style once, and `bareURL` counts unmatched
parentheses once before trimming them) and `ui/agent_markdown_view.go` its
renderer (`mdView`: a styled paragraph is a wrapping row of words, because gpui's `StyledText` does not
flow runs). `ui/agent_message.go` caches each message's parse by block revision (`agentTab.markdown`),
opens links (`openLink`) and holds the copy buttons (`copyButton`, `copyText`, `reportClipboard`).
`ui/agent_queue.go` holds the queue (`enqueue`, `nextQueued`, called by `drainEvents` once per batch
that ended a running turn and after a failed prompt; `nextQueued` drains synchronous failures in
bounded iterative batches and `pumpTabs` continues any remainder, `ResumeQueue`, `dropQueue`, called by `stopTab`
after it ends a running turn, and the chips). The pump (`pump`, `pumpTabs`) picks its period from
what the tick found: `agentBusyPoll`, `agentTurnPoll` or `agentIdlePoll`. `ui/agent_working.go` is the working
animation (`workingIndicator`, painted with `gpui.Canvas` strokes; its pose is a function of the frame
time, so it adds no frames of its own). The prompt is a `gpui.TextInput` made
`Multiline(agentPromptLines)` in `newAgentPanel` (`ui/agent.go`). `ui/workspace.go` creates the panel lazily and
hosts the toggle and `agentCommand`; `cmdsys/commands/agent.go` is the `AI` command. `ui/agent_demo.go`
seeds the canned transcript. `ui/agent_install.go` is the Install OpenCode card and its flow
(`askInstall`, `startInstall` running the `AgentInstaller` on a goroutine, the poll `pollInstall`,
`cancelInstall`, `openLogin`); tests install fakes with `Workspace.SetAgentInstaller` and
`SetTerminalOpener`. Threading and lifecycle rules are in
[Agent integration](../tech/agent-integration.md).

## Failure

- A history directory that cannot be created or written shows one warning in the transcript; the
  conversation continues unsaved. A stored file with a truncated last line loses only that line.
- A harness that fails to start (missing binary, handshake error) ends the turn with the error as a note
  and leaves the panel idle; a queued prompt is dropped.
- A tool call that the UI goroutine cannot start within min(15 s, the request's deadline less 10 s)
  returns a busy error to the agent; one with less than 10 s left before its deadline is refused at once.
- A failed prompt write ends the turn with the error as a note.
- A model switch the agent refuses, or that does not answer within 15 s, shows a warning and restores the
  previous choice; a model id the CLI rejects at start ends the start with the error, as above.
- A conversation that cannot be copied on SAVE to another directory shows one warning; the others are copied.
- An image that cannot be read, is no PNG, JPEG, GIF or WebP, is over 5 MB once scaled, would be the
  seventh, or goes to a model without vision, shows a warning in the transcript (not stored) and is not
  attached. A session that turns out to take no images when a queued prompt with images reaches it ends
  that turn with `the agent does not accept images`.
- In the browser build no agent can start; the panel reports the launch error.
- An installation that fails (no network, a checksum mismatch, no mise build for the platform, `mise
  install` failing) shows the error, with the last lines mise printed, and stores nothing; a cancelled one
  goes back to the offer with a note. A sign-in check that cannot run, or a terminal that cannot be
  opened, keeps the installed OpenCode and says to run `opencode auth login` by hand.

## Related docs

- [Agent integration](../tech/agent-integration.md) — the agentmanager boundary, the MCP server and the threading rule
- [Command line](command-line.md) — the other keyboard input and the F-key shortcuts
- [UI framework and theme](../tech/ui-framework.md) — the status colours the panel uses

## Next steps

- A multi-line prompt field, once `TextInput` supports it.
- Delete or rename a stored conversation from the history menu.
- A live resume test against the real CLIs.
- Attach a file copied in the Finder or Explorer on paste (the clipboard's file list), not only image data.
- Per-model vision for opencode, once its ACP model list says which models read images.
