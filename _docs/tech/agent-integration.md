---
id: tech-agent-integration
title: Agent integration
kind: tech
status: current
summary: How Cad-lite hosts AI agents: the separate agentmanager module, the in-process MCP server `cad`, the rule that only the UI goroutine touches the drawing, and the permission policy.
read_when: you are adding a drawing tool for agents, changing the agent threading or lifecycle, or touching the permission defaults
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [ui/agent.go, ui/agent_model.go, ui/agent_image.go, agentmanager/session.go, ui/discovery.go, agentinstall/agentinstall.go, agentinstall/mise.go, agentinstall/opencode.go, agentinstall/auth.go, ui/agent_install.go, ui/terminal.go, ui/agent_binary.go, agentmanager/shellenv/shellenv.go, agentmanager/harnesses/harnesses.go, ui/agent_store.go, agentmanager/store/store.go, ui/agent_e2e_test.go, cadmcp/dispatcher.go, cadmcp/tools.go, cadmcp/edit.go, cadmcp/batch.go, cadmcp/text.go, cadmcp/render.go, ui/snapshot.go, agentmanager/_docs/api-contract.md, go.mod, agentmanager/options.go, agentmanager/mcp/server.go, agentmanager/jsonrpc/conn.go, cmdsys/command.go, cmd/cad/main.go, cadmcp/safety_test.go]
depends_on: [tech-architecture, tech-ui-framework, tech-command-system, feat-agent-panel]
review_cycle: monthly
---

# Agent integration

## What it is

Three pieces connect an AI agent to a drawing:

- **`agentmanager/`**, a nested module with its own `go.mod`, stdlib only, importing nothing from
  Cad-lite. It launches opencode (over ACP), Claude Code (stream-json) and Codex (app-server) as child
  processes and exposes one backend-neutral `Session` (events in, prompts and permission answers out),
  an MCP server package and a transcript model. Like `internal/gpui`, it is a reusable library with its
  own docs, rules and API contract: start at `agentmanager/_docs/api-contract.md`, which is normative.
  Cad-lite reaches it through a `replace` directive in `go.mod`; `go test ./...` from the root does not
  enter it, so it has its own check (`just check-agentmanager`, see [Operations](operations.md)).
- **`cadmcp/`**, Cad-lite's glue: the drawing tools as MCP tools, and the `Dispatcher` that runs them on
  the UI goroutine. It imports `cmdsys`, `doc`, `geom` and `agentmanager/mcp`, never the UI.
- **`ui/agent*.go`**, the panel that starts sessions and pumps events; see the
  [agent panel](../features/agent-panel.md).

`agentmanager/store` (stdlib, in the nested module) persists conversations as JSONL per session plus an
open-tab set per drawing; Cad-lite keeps them in `<project>/.cad-lite/agent/`. Nothing in this chain exists until the
panel or the `AI` command is first used.

## PATH discovery

A GUI app often has a shorter PATH than the user's terminal: macOS launchd gives apps launched from the
Finder or Dock no `/opt/homebrew/bin`, `~/.local/bin`, `~/.bun/bin`, npm global bins or mise/asdf shims; a
Windows GUI keeps the PATH from login time; some Linux launchers skip `~/.bashrc`. `agentmanager/shellenv`
asks for the real one: on Unix `$SHELL -l -i -c` (fallback `/bin/zsh` on macOS, `/bin/bash` elsewhere) prints
`$PATH` between unique markers, so banners and rc noise are ignored; stdin is closed, there is no
terminal and no window, and it gives up after 5 s. On Windows it reads the Machine and User `Path` with `reg query`
and expands `%VARS%`. `shellenv.Merge` puts the login entries first and drops repeats; `Set` stores the result
for the process. `harnesses.Available`, `harnesses.Discover` (path and `--version` per agent, 3 s each) and
`harnesses.Open` use the merged PATH: `Open` resolves the executable to its full path and gives the child
`PATH=<merged>`, so the agent's own subprocesses (node, bun) are found. An executable the user chose in the settings
(`agent.binaries`) is handed to `harnesses.SetBinary` by `ui/agent_binary.go`; it is process-wide and wins
over the PATH lookup in `Available`, `Discover` (which marks it `Override`) and `Open`, while an explicit
`Config.Binary` (`-agent-cmd`) still wins over it. `harnesses.Check` validates a chosen file (exists, not a
folder, executable) and reads its `--version`. `ui/discovery.go` runs the probe once in
the background at startup (not for `-png`), stores the result in memory only and shows it in the settings
panel ([Settings](../features/settings.md)). Not covered by tests here: real macOS and Windows probes.

## Installing OpenCode

`agentinstall` (a Cad-lite package, not part of `agentmanager`, because it needs the app's folders and
reuses `updater`'s archive and checksum helpers) installs OpenCode for a user who has no agent, only after
the user confirms it in the UI. `Installer.Install` runs three steps, each cancellable through its context:

1. **mise.** `EnsureMise` reuses `<tools>/mise/bin/mise` when it answers `--version`; otherwise it reads
   `SHA256SUMS.txt` from `https://github.com/jdx/mise/releases/latest/download/`, picks the archive for the
   platform (`AssetSuffix`: Linux the static `-musl.tar.gz`, macOS `.tar.gz`, Windows `.zip`) and the version
   from its name, downloads `/download/v<version>/<archive>` over HTTPS (plain HTTP only to loopback, for
   tests), refuses it unless its sha256 matches (`ErrChecksum`, `ErrNoChecksum`), unpacks it with
   `updater.Extract` in a temporary folder and moves the executable into place, mode 0755. No shell
   installer runs.
2. **OpenCode.** `InstallTool` installs OpenCode's release binary, never needing node for it, trying in
   order: `aqua:anomalyco/opencode@latest` (`BinarySpec`; mise's aqua backend asks the GitHub API for the
   release, unauthenticated, 60 requests an hour); then the same archive through mise's `http` backend from a
   direct URL, `http:opencode[url=https://github.com/anomalyco/opencode/releases/download/v{{version}}/<asset>]@<version>`,
   where `<asset>` is `ReleaseAsset` (`opencode-linux-x64.tar.gz`, `opencode-darwin-arm64.zip`,
   `opencode-windows-x64.zip` and the arm64 forms) and the version comes from the redirect of
   `releases/latest/download/<asset>`, so no API call is made (github.com/sst/opencode redirects to that
   repository); and only when `node` is on the merged PATH, mise's registry entry `opencode@latest` (`NpmSpec`,
   npm, about 1 GB). Without node the error says so (`ErrNoNode`) with why the binary attempts failed; a
   GitHub refusal (403, 429, rate limit) is marked `ErrRateLimited`, "try again later". After a success
   `mise which --tool <spec> opencode` gives the executable and `mise cache clear` drops the second copy; a
   failed or cancelled attempt removes the install folders it created. mise is confined to the tools
   folder: `MISE_DATA_DIR`, `MISE_CONFIG_DIR`, `MISE_CACHE_DIR`, `MISE_STATE_DIR`, `MISE_GLOBAL_CONFIG_FILE`
   and `MISE_SYSTEM_CONFIG_FILE` point inside it, `MISE_CEILING_PATHS` stops it reading project files above
   it, `MISE_YES=1` answers its prompts, npm's cache is redirected there too, and the working directory is
   `<tools>/work`. Every program runs in a process group of its own (Windows: `taskkill /T` on cancel), so a
   cancel kills mise with the downloads and npm processes it started. mise plus the binary take about
   320 MB on disk (mise about 140 MB, OpenCode about 180 MB) from about 110 MB of downloads.
3. **Sign-in.** `CheckAuth` runs `opencode auth list` (30 s) and `HasCredentials` reads the layouts opencode
   prints: the framed one with `N credentials` counts, in Unicode or with its ASCII fallback (`T`, `|`, `—`),
   or one line per credential (2.x: `No authenticated integrations` when empty). Credentials given by
   environment variables count. OpenCode 1.x leaves no process behind; OpenCode 2.x starts a detached
   background service on this call (its own session, recorded in `~/.local/state/opencode/service.json` with
   pid, URL and password) that would keep running. When that file appears only during the check, the
   service is ours and `opencode service stop` ends it (else the recorded pid is killed); a service running
   before the check is left alone. (`auth list --standalone` would avoid the service on 2.x, but 1.x rejects the
   flag.)

The tools folder is `cad-lite/tools` under `%LocalAppData%`, `~/Library/Application Support` or
`$XDG_DATA_HOME` (`~/.local/share`), or `$CADLITE_TOOLS_DIR` ([Operations](operations.md)). `ui/agent_install.go`
runs `Install` on a goroutine and adopts the result through a poll timer, like the discovery probe; the
installed executable is stored as `agent.binaries.opencode`, which `applyAgentBinaries` hands to
`harnesses.SetBinary`. `Workspace.CloseAgent` (quit) cancels a running installation and waits up to 10 s for
it to kill its processes and remove what it half-wrote. When there is no credential, `OpenTerminalCommand`
opens a terminal running `opencode auth login` (`agentinstall.LoginArgs`). Tests serve a fake release (a
tarball holding a shell-script mise that writes a fake opencode) from `httptest`; covered are checksum
refusal, cancellation during the download and during `mise install` (child process killed, partial folder
removed), each fallback, the node rule, the service stop, and the isolation variables. Run for real once on
Linux: the mise download, the aqua attempt failing on a blocked API, the direct-download fallback installing
OpenCode 1.19.0 without node, and the sign-in check with no process left. A real sign-in, the aqua backend
with a reachable API, and the macOS and Windows terminals were not run.

## The `cad` MCP server

The first session start of a document tab creates that tab's `mcp.Server` named for Cad-lite, registers
`cadmcp.Register` with an `Env` (`Dispatch`, `Session`, which returns that document tab's session whichever tab
is active, `Snapshot`, `Changed`), binds it to `127.0.0.1` on a random port and passes its
URL and bearer token to the agent as `agentmanager.MCPServer{Name: "cad"}`. Every request needs the
token; requests with a foreign `Host` or `Origin` are refused. One server serves every live session of its document tab (the MCP server is stateless per request), so a
session edits the drawing it was started from (decision D32): it
starts with the tab's first session and closes when its last one stops, exits or quits, so the port and token
never outlive the chats. Tool calls are serialized by a capacity-one semaphore that honours the request
context, so a cancelled request neither waits for nor runs after a busy handler. The HTTP server sets a
header timeout of 10 s, an `IdleTimeout` of 2 minutes and a 60 s write timeout (handler time counts), caps
a request body at 4 MB and a JSON-RPC batch array at 100 frames (a longer one is refused whole with
`-32600` and runs nothing). The 60 s are budgeted from the request's arrival, and the budgets add up:
a tool call must take the call slot within 30 s, or it is answered with an error and never runs; its
handler's context expires at 55 s; `Dispatcher.Do` starts the tool body only if it can do so within
15 s of being asked and at least `cadmcp.RunBudget` (10 s) before that deadline, else it returns
`ErrUIBusy` and the body never runs; the last 5 s are for writing the reply (30 + 15 + 10 + 5 = 60).
The body itself cannot be interrupted once it runs on the UI goroutine: one that runs longer than its
10 s still commits its change, and its reply is then lost to the write timeout, so the agent sees a
failed call that did take effect. The caps on scripts, polylines and batches keep normal bodies far
below 10 s; nothing enforces it. The limits are part of the agentmanager API contract. The `batch` tool
below has caps of its own.

The tools (all coordinates in model units, Y up; one tool call is one undo step):

| Tool | Effect |
|---|---|
| `run_script`, `run_command` | Run AutoCAD script text, or one response line to the command line; return the history lines produced |
| `list_commands` | Command names the script accepts |
| `get_drawing`, `list_entities` | Layers, counts, extents, undo state; paginated entity list with geometry, filtered by layer, class, box or shown text |
| `get_selection`, `set_selection` | Read or replace the selection |
| `add_line`, `add_circle`, `add_arc`, `add_polyline`, `add_rectangle`, `add_polygon` | Create entities on the current layer (or `layer`); the shape forms are below |
| `erase`, `move`, `rotate`, `scale`, `set_entity_layer` | Edit entities by handle |
| `add_text`, `add_mtext`, `update_text` | Create TEXT and MTEXT; change a text by handle (below) |
| `batch` | Many add and edit tools above in one call and one undo step, all or nothing (below) |
| `set_layer`, `create_layer` | Current layer; layer table edits (not undoable) |
| `undo`, `redo`, `zoom_extents` | History and view |
| `measure` | Length, perimeter, area, distance |
| `render_png` | The drawing as a PNG, returned as MCP image content so a vision model can look at it, with a JSON block that maps pixels to model coordinates; views by extents, box, centre and scale, or the user's current view, with optional grid, rulers (bottom and left; drawn by `ui/ruler.go`, which the canvas rulers share) and selection highlight |

**Shape forms.** Each `add_*` tool validates and builds its entity in a pure builder in
`cadmcp/edit.go` (`arcEntity`, `polylineEntity`, `rectEntity` and the rest), which `batch` calls too,
so every form below works inside a batch. A tool that takes alternative forms accepts exactly one,
given in full, and names the missing or conflicting arguments otherwise.

- `add_arc`: three points `p1`, `p2`, `p3` (start, a point on the arc, end), or `center`, `radius`,
  `start_angle`, `end_angle`, AutoCAD's Center form: degrees, counter-clockwise from +X, the arc
  running counter-clockwise from start to end, as DXF stores it. Equal angles (modulo 360) are an
  error; a full circle is `add_circle`.
- `add_polyline`: optional `bulges`, one per point as DXF group 42: the bulge of the segment leaving
  that point, tan(included angle/4), positive counter-clockwise; the last one counts only when
  `closed`. All-zero bulges are stored as none.
- `add_rectangle`: opposite corners `x1`, `y1`, `x2`, `y2`, or `origin` plus a signed `width` (along
  X) and `height` (along Y); optionally `fillet` (a radius) or `chamfer` (`[d1, d2]`, d1 off the edges
  along X and d2 off those along Y, as RECTANG applies them), never both. The corners are cut by
  `geom.CutPolyline` with `geom.FilletCut` or `geom.ChamferCut`, so a rounded corner is a bulged
  vertex.
- `add_polygon`: `sides` (3–1024) plus `center` and positive `radius`, or `edge` (two different
  points). The centre form defaults to `mode: inscribed` (radius to the vertices); `circumscribed`
  uses the radius to the edge midpoints. At `rotation: 0` the bottom edge is horizontal, as with
  POLYGON's typed radius; rotation is degrees counter-clockwise. The Edge form places the polygon
  to the left of the directed edge and takes no mode or rotation. Both forms build a closed
  LWPOLYLINE through `commands.PolygonFromCenter` and `commands.PolygonFromEdge`.

**Text.** `add_text` places single-line TEXT as AutoCAD's TEXT command does: `point` is the
insertion point its `justify` refers to (AutoCAD's option names through `doc.ParseJustify`: Left, the
default, Center, Right, Align, Middle, Fit and TL … BR), or for Align and Fit the baseline start, with
`second_point` its end; `height` is the capital height, `rotation` degrees counter-clockwise, `style` an
existing text style (the default style, the STYLE records kept from the file, or one some text
uses; matched case-insensitively and stored with the drawing's spelling), `width_factor` 0.01–100.
Align derives the height and Fit the width factor, and both take the rotation from their two points,
so giving those is an error. The value keeps AutoCAD's `%%` codes and must be one line: a line break is
an error rather than several TEXT entities, so one call is one entity and one handle (use `add_mtext`, or
one `add_text` per line at 5/3 of the height apart, as TEXT continues under the last line).
`add_mtext` places MTEXT by its `attachment` (TL, the default, … BR) at `point`, wrapping at `width` (0
does not wrap); line breaks become `\P`, and `\`, `{` and `}` are escaped so the text shows as typed,
unless `raw` is set, which passes MTEXT format codes through. Values are capped at `cadmcp.MaxTextLen`
(1000 bytes) and `cadmcp.MaxMTextLen` (20000 after escaping), and control characters other than tab are
refused. `update_text` changes any subset of `value`, `point`, `second_point`, `height`, `rotation`,
`justify`, `style` and `width_factor` (TEXT) or `width` and `attachment` (MTEXT) of one text by
`handle`; a field of the other class, or of another justification (rotation on Align text), is an
error. A new `justify` keeps the insertion point, so the text moves around it, as TEXT's Justify option
does (`doc.Text.SetJustify`). `update_text` checks the change on a copy first and inside `batch` against
the text as earlier ops leave it, so a refused update changes nothing. `list_entities` takes `text`, a
case-insensitive substring of the shown text (`doc.MTextPlain` for MTEXT), and the classes TEXT and
MTEXT; a text's entry has `value` (for MTEXT the plain text, with the stored value in `raw`), `height`,
`rotation` (degrees), `justify` or `attachment`, `point` (the insertion point; Align and Fit add
`second_point`), `style`, `width_factor` or `width`, and `box`, the four corners of the text box
counter-clockwise. Text deviations: JUSTIFYTEXT (changing the justification without moving the text) has
no structured form, oblique angles and the backward and upside-down flags cannot be set, and the
line spacing of MTEXT is always the default.
The text tools share `doc.MTextEscape`, `doc.FindTextStyle` and `doc.TextStyles` with the command
system; `cadmcp/text.go`, `styleResolver` caches style lookups within a batch.

Deviations from RECTANG and ARC: a fillet or chamfer that does not fit is an error, where RECTANG
silently draws square corners; the size form uses signed lengths where RECTANG's Dimensions option
picks the quadrant with a point; RECTANG's Rotation, Width, Elevation and Thickness options and
ARC's angle, length and direction variants have no structured form (`run_script` reaches them); and
RECTANG's fillet or chamfer are not remembered between calls, whereas the command keeps them as
session settings. `add_polygon` also keeps no session defaults; its rotation takes an angle instead
of POLYGON's picked radius point.

**Handles.** Every handle-taking tool (`erase`, `move`, `rotate`, `scale`, `set_entity_layer`,
`update_text`, `set_selection`, `measure`, `render_png`'s `highlight`, and `batch`) returns an error, and changes nothing,
for a handle that is not hexadecimal, names no entity, or names an entity of a class Cad-lite does not
model (`doc.Unknown`, reported by `list_entities` with `unknown: true`); `existingHandle` in
`cadmcp/tools.go` is the one check. An edit also refuses an entity on a locked layer.

**Number ranges.** Every number that places or sizes geometry (coordinates, lengths, radii, offsets,
angles in degrees, scale factors) must be finite and within ±`cadmcp.MaxCoord` (1e12), and a polyline
bulge within ±`cadmcp.MaxBulge` (1e6). Inside the transaction each created or changed entity's bounds
must be finite and within ±`cadmcp.MaxExtent` (1e15), which catches results that in-range inputs can
still reach (a large scale far from the centre); otherwise the transaction aborts with an error.
`run_script` is not covered: a command fed huge numbers can still store unencodable geometry.

**The `batch` tool.** `batch` takes `ops`, a list of `{op, id?, args}`. `op` is one of `add_line`,
`add_circle`, `add_arc`, `add_polyline`, `add_rectangle`, `add_polygon`, `add_text`, `add_mtext`, `erase`, `move`,
`rotate`, `scale`, `set_entity_layer` and `update_text`, and `args` are exactly that tool's
arguments (`update_text`'s `handle` may be a `"$<id>"` too). An `add_*` op may carry a client `id`
(1–64 letters, digits, `_`, `.`, `-`, unique in the call); a later op names the entity it creates as
`"$<id>"` in its `handles`, beside ordinary hexadecimal handles (which never start with `$`). It runs in
three stages, each op through the same builder as its single tool (`lineEntity`, `moveEdit` and the rest
in `cadmcp/edit.go`):

1. `batchArgs.UnmarshalJSON`, on the MCP handler's goroutine, checks what needs no drawing: op names,
   ids, arguments and numbers, and that every `$id` names an earlier `add_*` op; it builds the entities.
2. `resolveBatch`, on the UI goroutine, checks layers and existing handles, that no earlier op of
   the batch erased a target, and that no edit touches an entity on a locked layer, following the
   layer changes of earlier ops. Only then does the batch cancel a running command, as every tool
   does, and it checks again if that changed anything.
3. `runBatch` applies every op in one `doc.Txn`, checking each op's result bounds and, again, locked
   layers. Any failure aborts the transaction and restores the handle seed (in a deferred call, so a
   panic restores it too).

A refused batch leaves a running command alone. A failure while applying, which the checks before
make rare, comes after the command was cancelled. Otherwise a failure at any stage changes nothing (entities, handle seed, undo and redo stacks) and names the
failing op, as in `ops[3] (move $door1): no entity with handle FFFF; nothing was changed`. Success
returns `ops`, `handles` (client id to handle), `created` (handle, class, layer, id), `modified` and
`erased`; the last three are the transaction's `doc.ChangeSet`, so an entity created and then erased
in the same call appears nowhere. A call holds at most `cadmcp.MaxBatchOps` (500) ops and
`cadmcp.MaxBatchPoints` (200000) polyline points, under the server's 4 MB body cap. A batch cannot run
scripts or commands, so NEW, OPEN and SAVE stay out of reach.

A batch mirrors AutoCAD's `UNDO BEGIN` … `UNDO END`: one `UNDO` reverts all of it, and its ops are
named after the single tools, which mirror LINE, CIRCLE, ARC, PLINE, RECTANG, ERASE, MOVE, ROTATE,
SCALE and a CHPROP layer change. It differs from AutoCAD in three ways: it is atomic, whereas an
AutoCAD undo group keeps whatever ran before an error; it is one transaction rather than a journal
group (`Drawing.BeginGroup`), which is what makes the rollback possible; and a failed batch gives
its unused handles back (the seed is restored), where AutoCAD never reissues a handle; no handle of
a failed batch was ever reported, so nothing can refer to one. Client ids live only for the call;
they are not stored in the drawing.

`render_png` is drawn by `ui.SnapshotImage`, headlessly, from the session; it never captures the window.
The image holds at most `cadmcp.MaxSnapshotPixels` (2 megapixels, e.g. 2048x1024): a larger
width x height is scaled down uniformly, keeping the aspect ratio and the model window the arguments
asked for, and the JSON block then reports the size drawn plus `requested` and `downscaled`. The render
runs on the UI goroutine (it reads the session); the PNG encoding runs afterwards on the MCP handler's
goroutine, so the window stalls only for the render.

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
   `Do` fails with `ErrUIBusy`, and the body never runs, when the UI goroutine does not start it within
   15 s, or within the context's deadline less `RunBudget` (10 s), or when the dispatcher closes.
2. The UI goroutine drains the mailbox: `AgentPanel.pump` is a `Window.After` timer that calls
   `Dispatcher.Drain`, adopts finished session starts, and folds every tab's event channel into its
   transcript and stored record with non-blocking reads, background tabs included. No goroutine pumps
   events. Concurrent sessions therefore interleave only between timer ticks, and tool calls from all
   of them are serialized by the dispatcher and the server.
3. `ui/agent.go`, `pumpTabs` re-arms the timer every 16 ms while a session starts, a model switch
   runs, events arrive, dispatcher work is pending or runnable queued prompts remain. A quiet running
   turn uses 100 ms ticks (10 animation frames per second); idle live sessions use 250 ms ticks to
   notice a crashed agent. Paused queues do not request busy ticks, and with no live session or
   pending work the timer stops. Each tick also tries `nextQueued` for ready tabs.
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
is why Cad-lite shows its own stored transcript. The open tabs and the active one are saved for the
current drawing (its file name, "" when untitled) on every change and at quit (`Store.SaveOpenFor`);
restoring them reads records only, so no agent starts until the user sends. Each record also keeps the
drawing (`Record.SetDrawing`) and the user's model choice (`Record.SetModel`), and stores the
`ModelsChanged` events, so a restored tab shows its model and the list the agent offered. Drawing and
project switches and the `.cad-lite/` layout are in the [agent panel](../features/agent-panel.md).

## Models

`Config.Model` is the tab's choice, else `agent.model_<harness>` from the settings. Every backend reports
what the agent offers as `ModelsChanged` and implements `agentmanager.ModelSetter`; the panel calls
`SetModel` on a goroutine (it blocks on the agent's answer, up to 15 s) and folds the result back on the
timer like a session start. A session without a setter, or a change back to the default, is closed and
restarted with `ResumeID` on the next prompt. Wire details per harness are in `agentmanager/_docs/protocols.md`.
On opencode a tab nobody picked a model for starts on a free one (`ui/agent_model.go`, `commitSuggestion`,
decision D33).

## Images

A prompt's images go through `agentmanager.ImagePrompter.PromptImages`, which all three backends implement
(ACP image content blocks, Claude base64 image blocks, Codex `image` items with a data URL). The panel
decides whether to allow them from `ImagePrompter.AcceptsImages` of the live session and
`ModelInfo.Vision` of the tab's model (`ui/agent_image.go`, `visionOf`): no when either says so, yes for
Claude, otherwise unknown, which allows them. The UI goroutine decodes, scales and encodes the image when it
is attached (up to 2048 px and 5 MB), so a large file costs one stall of the panel then. The record keeps a
preview, not the image (`store.Record.AddUserWith`, D33). Not run live with an image on any harness.

`Env.Changed` runs inside `Do` after every mutating tool (`AgentPanel.agentChanged`) and refreshes the
window, so the canvas shows the agent's edits between frames. It does not clear the render cache: entity
edits reach it through the drawing's change sets, so it calls `Builder.SyncLayers`, which drops
the cache only when a layer table edit (not journalled) changed a layer's colour or lineweight or the
table itself.

## Permission policy

- The session config carries `agentmanager.AllowMCP("cad")`: requests for the `cad` tools are answered
  without a prompt, preferring allow-once. The match is strict: when the backend set
  `Call.MCPServer` only an exact match passes; otherwise only the exact title `mcp__cad__<tool>` with a
  bare tool name (`[A-Za-z0-9_-]+`) passes. A call whose kind is anything but other or read is never
  auto-allowed, so a title that merely starts with `cad` does not qualify. The ACP backend derives
  `MCPServer` by the same strict rule (`agentmanager/_docs/api-contract.md`).
- Everything else is a card in the panel with the agent's own options. The panel never answers for the
  user and has no allow-all control.
- Cad-lite requests no permissive harness mode (no bypass or full-auto flags). Where a protocol allows it,
  the backends run shell and file tools sandboxed read-only; see the agentmanager API contract.

## Verification status

| harness | status |
|---|---|
| Claude Code (`claude`, 2.1.299) | Verified live: start, MCP tool calls through the `cad` server with the auto-allow policy, permission round trips, interrupt; the model list (`initialize`) and `set_model`, accepted and refused, without a prompt (`TestLiveClaudeModels`). Claude defers MCP tools behind `ToolSearch`, so the first drawing call shows an extra `ToolSearch` tool card in the transcript. |
| opencode (ACP) | Not run live (models included). Verified against the fake agent only; wire details are marked unverified in `agentmanager/_docs/protocols.md`. |
| Codex (`codex`) | Not run live (`model/list` and the `turn/start` model included). Verified against the fake agent only; wire details unverified, as above. |

`ui/agent_e2e_test.go` runs all three backends through the real panel, the real `cad` MCP server and the
real `Dispatcher`, against `agentmanager`'s fake agent (built with `go build` from the nested module). The
fake makes a real MCP `add_line` call; the test pumps the UI timer until the drawing has the line and the
transcript's tool card is completed. A second test calls `render_png` over HTTP and decodes the PNG. They
skip on `js` or when `go` is missing. `cadmcp/batch_test.go` proves a mixed batch with client-id references is one undo step and that a
batch failing at any op, while checking or while applying, leaves the drawing and both undo stacks
unchanged. `cadmcp/safety_test.go` holds the agent-safety tests: scripts cannot
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
