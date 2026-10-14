---
id: wip-agent-api-waves
title: Agent API waves and transcript virtualization (progress)
kind: wip
status: draft
summary: Progress ledger for the transcript virtualization and the agent drawing API backlog items N9 to N20, wave by wave.
read_when: you are picking up or reviewing work on the agent transcript list or backlog items N9 to N20
updated: 2026-10-14
verified: 2026-10-14
code_anchors: []
depends_on: []
review_cycle: monthly
---

# Agent API waves and transcript virtualization (progress)

Coordinator ledger. Statuses: backlog, ready, in progress, in review, done, blocked, abandoned.
`done` means the combined checks passed (gofmt, go vet, go test, docs lint, agentmanager checks).

## Wave 1

| Item | Owner | Write scope | Status | Notes |
|---|---|---|---|---|
| Transcript virtualization: variable-height virtualized list in gpui, used by the agent transcript | agent "transcript" | `internal/gpui/`, `ui/agent_view.go`, `ui/agent.go`, `agentmanager/transcript` (minimal), `_docs/features/agent-panel.md`, `_docs/tech/ui-framework.md` | in review | `gpui.List`/`ListState` (height cache by index, ItemKey = block revision + expanded flag, 100 px overdraw, top-item anchoring); `transcript.T.Revision(i)`. gpui tests and ui agent tests green (coordinator re-run). Not run: real wheel/trackpad/thumb drag, hover in a live window. Review in progress |
| N10 MCP batch tool and G32 (unknown handles error) | agent "batch" | `cadmcp/`, `_docs/tech/agent-integration.md` | in review | `batch` tool: 1-500 ops, `$id` refs, all-or-nothing in one transaction (one UNDO); G32 now also refuses handles of unmodelled entities. cadmcp vet and tests re-run green by the coordinator; combined review pending |
| G49 JSON-RPC batch cap and WriteTimeout | agent "batch" | `agentmanager/mcp/server.go` + tests, `agentmanager/_docs/api-contract.md`, server sentence in `_docs/tech/agent-integration.md` | in review | 60 s WriteTimeout, 100-frame JSON-RPC batch cap, calls not started 40 s into a request are refused (beyond the brief: protects against lost replies after a drawing change). mcp tests green |
| N17 small MCP geometry: arc by centre/angles, bulges, rectangle width/height + fillet/chamfer | agent "batch" | `cadmcp/`, `_docs/tech/agent-integration.md` | in review | All forms also in `batch`. `add_polygon` blocked: needs `cmdsys/commands` to export the polygon vertex function (backlog G74); cmdsys is owned by "text" this wave |
| Review: transcript list | reviewer "review-list" | read-only | done | No medium+ bug. Confirmed low: anchor stored absolute when the list is empty (offset ends at pad.Top); mean estimate skewed by outliers (17x scroll range). Info: per-frame work still O(blocks) with small constants; follow-tail pulls a scrolled-up user down (as before, now backlog N21) |
| Review fixes: list anchor, robust estimate, InvalidateAll on language/theme, optional tops cache | agent "transcript" | same as its wave-1 scope | in review | All four done: empty frame drops the anchor; median estimate; `checkLook` re-measures on language/theme/scale change; tops buffer and `shown` cache. Tests failed first; gpui and ui agent tests re-run green by the coordinator. Waiting on the final combined checks |
| Review: batch, G32, N17, G49 | reviewer "review-mcp" | read-only | done | No atomicity hole. Confirmed: huge finite inputs give NaN/Inf geometry that breaks get_drawing; a failed batch still cancels the running command; G49 docs over-claim (Dispatcher.Do bounds only the queue wait, so a commit can lose its reply); a near-zero backwards segment on a half-side fillet; a -1e-20 start angle stored as 360 |
| Review fixes for the above | agent "batch" | `cadmcp/`, `agentmanager/mcp/`, mcp part of `agentmanager/_docs/api-contract.md`, `_docs/tech/agent-integration.md` | in review | All fixed with failing-first tests: MaxCoord 1e12 / MaxBulge 1e6 / MaxExtent 1e15 with a bounds check in the txn; batch validates before cancelling the command; budget 30+15+10+5 s from request arrival, `cadmcp.RunBudget`; a tool body over 10 s can still commit and lose its reply (documented). Open: `TestAgentToolsSeeADrawingOpenedFromDWG` expects 56 entities, gets 85 since "text" reads more DWG; that agent must update it. geom root causes logged as G75 |
| N9 native TEXT and MTEXT | agent "text" | `doc/`, `geom/` (if needed), `render/`, `fileio/`, `cmdsys/commands/` (new files + registration), `i18n/locales/`, `_docs/tech/document-model.md`, `_docs/tech/file-io.md`, `_docs/tech/rendering.md`, `_docs/features/drawing-and-editing.md`, `_docs/features/dxf-files.md` | in review | TEXT/DTEXT (15 justifications, TEXTSIZE/TEXTJUSTIFY/TEXTSTYLE, %% codes), MTEXT (wrap, \\P, attachment 1-9; saved to R12 as TEXT lines with a warning), Hershey romans vector font in doc (D34), DXF read/write, DWG TEXT read, MIRRTEXT. Core tests green (coordinator re-run); `_tmp/text/before.png` inspected: justifications and rotations sit on their reference points. Independent review in progress |

## Direct requests (2026-10-14, second batch)

| Item | Owner | Write scope | Status | Notes |
|---|---|---|---|---|
| Settings panel: measure and fix slowness; split into General, AI and Updates tabs | agent "settings" | `ui/settings*.go`, `ui/discovery_view.go`, `ui/update_view.go`, their tests, `_docs/features/settings.md` (+ `updates.md`); i18n keys added last | in review | Frame with panel open about 70 ms to 20-25 ms: the card's drop shadow (41 ms) and full-window translucent backdrop (17 ms) in the CPU rasterizer; shadow dropped, backdrop as four bands; only the active tab is built. Ctrl+Tab cycles tabs; the gear opens Updates when one is waiting. Settings tests re-run (only G73 failure); `_tmp/settings/` inspected. Follow-ups: gpui raster shadows over covered area and no fast path for translucent fills (G77); AI tab PATH lookups per frame (G78) |
| Measure tools snap to existing geometry (object snaps on their point prompts) | agent "cmd" | `cmdsys/`, `render/` if needed, `ui/canvas.go` (+tests), matching docs | in review | Root cause shared with LINE: in-flight transaction edits reached the index and render cache only at commit, so nothing placed mid-command could be seen or snapped to. Also: object-pick prompts snapped the click (MEASUREGEOM Angle picked the wrong line); new `Prompt.Pick` |
| LINE: segments placed so far stay visible while the command runs (they show only after Enter today) | agent "cmd" | same | in review | `cmdsys/inflight.go` publishes in-flight edits to the index and canvas after each step, still one undo step; `doc.Txn.TakeEdits()`. Fixes LINE, COPY, OFFSET, TEXT lines, TRIM/EXTEND, FILLET/CHAMFER. Tests fail first; `_tmp/line/*.png` inspected; cmdsys/doc/render and ui canvas tests re-run green |
| Review: N9 TEXT/MTEXT | reviewer "review-text" | read-only | done | High: DXF save of backward Fit/Aligned text writes group 10 at the second point; MTEXT command does not escape typed `\\`, `{`, `}`. Medium: text Bounds are ink-only, so INS snap and box picking miss (spatial index); a text-box hit beats a nearby line in Pick; literal prompts trim spaces. Low/plausible: MIRRTEXT Left becomes TL; oblique scaled by width factor; `\\U+` written to R12; MTEXT 72/73/210 dropped silently; greeking threshold 2 px; NOTICE not shipped; stale MIRRTEXT sysvar comment; TEXTSIZE/TEXTSTYLE not in header; cadmcp entityInfo lacks text fields |
| TEXT review fixes | agent "text" | `doc/`, `fileio/`, `render/`, `cmdsys/`, i18n insertions, TEXT docs | in review | All 5 confirmed and 3 plausible fixed with failing-first tests (R12 `\\U+` kept and documented; greeking at 3 px band floor, measured 0.64 s vs 0.35 s on 3000 labels). New `doc.MTextEscape`, `doc.TextStyles`, `doc.FindTextStyle`. Deferred: $TEXTSIZE/$TEXTSTYLE in the header (G79); NOTICE packaging (G80). Tests re-run green |
| Agent panel: hide the harness chips once the tab's harness is fixed (live session or stored conversation) and show the harness on the first line next to the title; model chip and Start/Stop stay | agent "transcript" | `ui/agent_view.go`, agent tests, `_docs/features/agent-panel.md` | in review | Done: header "Agent · <harness>" (live tab uses the session's harness); tests for fresh, live and restored tabs; `_tmp/harness/*.png` inspected. Open: the user's reading of "started" | Assumption: "started" = the existing `locked` condition |

## Wave 2

| Item | Owner | Write scope | Status | Notes |
|---|---|---|---|---|
| MCP text tools: add_text, add_mtext, update_text, text filter and text fields in list_entities, all also in `batch` | agent "batch" | `cadmcp/`, `_docs/tech/agent-integration.md` | in review | Line breaks in add_text are an error (one call, one entity); MTEXT escaping unless `raw`; batch pre-checks update_text against the in-batch state. cadmcp tests re-run green. Follow-up: share `findTextStyle` between cmdsys and cadmcp (copied today) |

## Direct requests (2026-10-15, UI batch)

Queued: all three slots are busy. Grouped by file ownership.

| Item | Planned owner | Write scope | Status | Notes |
|---|---|---|---|---|
| Rulers on the canvas viewport, reusing the ruler drawing of `ui/snapshot.go` (render_png); toggle chip in the status bar | agent "cmd" | `ui/canvas.go`, `ui/snapshot.go`, new `ui/ruler*.go`, tests, canvas docs | in review | Rulers overlay the drawing (view and coordinates unchanged), shared with render_png (pixel hashes pinned), on by default, not persisted (like GRID's live toggle); about +0.5 ms per frame. `_tmp/ruler/dark.png` inspected. RULER chip and HUD offset routed to "settings". Found: light-theme `-png` draws colour 7 near-white (G82) | Assumption: on by default, toggled like GRID |
| Status bar layer name opens a searchable layer popover with the layers panel's features | agent "settings" | `ui/statusbar.go`, `ui/sidepanel.go` (shared layer rows), new popover file, tests, `_docs/features/canvas.md` | in review | `ui/layerpop.go`: filter, keys, shared rows with the side panel (new lock switch), New layer row. Popover/HUD hang from the status bar (canvas.go not owned). PNGs inspected; tests re-run green | |
| Status bar switch for a transparent HUD properties panel over the drawing area | agent "settings" | same + overlay in `ui/layout.go` | in review | `ui/hud.go`: 90% opaque, +0.5-0.75 ms per frame (opaque would be +0.1 ms); off by default, not persisted; RULER chip and `ui.status.props` translations added | Assumption: shows the selection's properties like the side panel; off by default |
| POLYGON button in the ribbon's Draw group (icon `draw-polygon` exists) | agent "cmdline" | `ui/toolbar_layout.go` | in review | Rectangle over Polygon column; reflow checked at 640-1600 px. Follow-up: add POLYGON to `TestRibbonTooltipsExist` in i18n | |
| Command line autocomplete (most recently used match first) and Up/Down history recall (backlog G20); remove the prompt (terminal) icon | agent "cmdline" | `ui/cmdarea.go`, new suggestion files, `_docs/features/command-line.md`, G20 line | in review | `ui/cmdcomplete.go`: exact name or alias first, then MRU, prefix, substring; Up/Down history (100 lines, session only); prompt icon removed. Also edited `ui/workspace.go` (command-line wiring moved to `wireCommandLine`), out of scope but unowned. Tests re-run (only known G73 ribbon failure); `_tmp/cmdline/` inspected | |

| Markdown rendering of agent messages (headings, emphasis, inline code, code blocks, lists, links, tables as far as gpui allows) | agent "transcript" | new `ui/agent_markdown.go` + tests, `ui/agent_view.go`, `_docs/features/agent-panel.md` | in review | Own parser, no new dependency; streaming partials render sensibly; parse cached per block revision | Assumption: agent messages and thoughts only, user messages stay plain; text stays selectable/copyable as today |

| Agent chat polish (one brief with the markdown item, same files): copy icon per message; a small cheap CAD-themed working animation; tool-call parameters collapsed by default, expandable; user messages inset with a left margin; Send becomes Stop (cancel the turn) while a turn runs; a Queue button when the user writes while the agent works | agent "transcript" | `ui/agent_view.go`, `ui/agent.go`, new `ui/agent_*.go` files + tests, `cmdsys/commands/agent.go`, `_docs/features/agent-panel.md`, icons via the registry | in review | All 7 items done; icons app-copy/app-check/app-stop added via the skill; animation cost within noise (6.4 vs 6.1 ms per frame). `_tmp/agent-ui/dark.png` inspected; agent tests re-run green. Found: 16 ms event poll redraws the window about 60 times a second during a turn (for the review fixes). Open choice: Stop with a queue still sends the next queued prompt | Assumptions: Stop cancels the running turn (session stays); queued prompts are sent in order when the turn ends, shown above the composer and removable; the animation ticks only while a turn runs, at a low rate; copy copies the message source text |

## Final review (requested 2026-10-15)

| Part | Owner | Status |
|---|---|---|
| 1 core | reviewer "review-core" | done: no High; M1 MULTIPLE leaves stale index entries, M2 TRIM/EXTEND cut at glyph strokes, M3 quadratic MTEXT wrap (4.3 s for 100 KB); 6 Low. Fuzz: DXF 6.1M and DWG 248k runs, no crash |
| 1 core fixes (+ G74 polygon export, G36 Pick on TRIM/EXTEND/BREAK) | agent "cmd" | done: all fixed with tests; session syncs before every commit/abort (`commitOpen`/`abortOpen`); text is a TRIM/EXTEND edge by its box; MTEXT 100 KB wrap 33.8 s to 64 ms; `doc/textlimits.go` clamps extreme text; `PolygonFromCenter`/`PolygonFromEdge` exported. Coordinator re-run green |
| cadmcp: add_polygon (G74), shared text helpers (G81) | agent `mcp_followups` | done: standalone/batch validation, rollback and undo verified; helpers shared; TEXT/MTEXT keyword groups and POLYGON tooltips checked; docs and combined suite pass |
| 2 agent side | reviewer "review-agent" | done: H1 header Stop mid-turn wedges the tab (queue never drains); H2 failed prompt stops the queue and later Sends overtake; M1 markdown parser quadratic (0.5-1.2 s on 4-21 KB pathological input); M2 60 Hz redraw during turns; M3 two "Stop" buttons; M4 Stop then sends next queued; 7 Low. Security: permission policy unchanged, no new file or HumanOnly paths |
| 2 agent-side fixes | agent `agent_fixes` | in review: lifecycle, failed queue, linear parser, quiet polling and distinct icons verified; independent review fixes rechecked. User decision: Stop (header, ends the session) and Cancel (composer, replaces Send, cancels the turn) stay separate with distinct labels and icons. Assumption still pending: the queue pauses after a Cancel, with a resume control |
| 3 UI shell | reviewer "review-ui" | done: no High; M1 mouse release over the HUD leaves a drag running; M2 layer edits not marked modified (lock lost without a save prompt); 12 Low (ruler labels at max zoom, ruler layout 7x per frame, popover leaks and edge cases); bench numbers contradict two doc figures |
| 3 UI shell fixes | agent `ui_fixes` | done: HUD release, metadata dirty tracking, weak popover lifetime, ruler fixes and benchmark docs pass focused checks; G73 checks repaired. MRU-first retained per user request |


After rulers, status bar and agent chat land: a complete review of the whole uncommitted change set, split into up to three read-only reviewers by area (core: doc/geom/render/fileio/cmdsys; agent: cadmcp/agentmanager/ui agent panel; UI shell: gpui list, settings, status bar, command line, canvas, rulers), then fixes, then the full verification run (gofmt, go vet, go test ./..., docs lint, agentmanager checks).

## Next waves (planned)

- Wave 2: MCP text tools on top of N9 and N10 (`add_text`/`update_text`, also in `batch`, a text filter in `list_entities`); G74 `add_polygon`; i18n KeywordGroups for TEXT/MTEXT; N12 tags.
- Wave 3: N11 groups, N13 structured edits, N14 handle-based TRIM/EXTEND/OFFSET/FILLET/CHAMFER.
- Later: N15 to N20.

## Log

- 2026-10-14: suggestions file triaged into backlog N9 to N20; wave 1 dispatched.
- 2026-10-14: user direction: follow AutoCAD conventions where possible (commands, prompts, options, DXF semantics); relayed to the "text" and "batch" agents.
- 2026-10-14: agentmanager `claude` TestOpenMissingBinary and `harnesses` TestDiscoverReportsPathsAndVersions fail under parallel package runs on a clean HEAD worktree too (`_tmp/head-wt`): an existing flake, not a regression. Logged as backlog G72.
- 2026-10-14: ui TestCommandTipHeadlines, TestFileAndSettingsTips and TestRibbonReflowsWithWidth fail on a clean HEAD too (macOS shortcut labels, ribbon width): existing failures, logged as backlog G73.

- 2026-10-15: stopped final-review follow-ups resumed by coordinator Jake. Exclusive owners: `agent_fixes` — ui agent-panel code/tests and agent-panel docs; `ui_fixes` — UI shell review fixes and canvas/settings/command-line docs; `mcp_followups` — cadmcp polygon/shared-text helpers and agent-integration docs. All three in progress; existing core fixes remain in review. No commits authorized. Cancel queue policy awaits user response; combined verification follows the fixes.
- 2026-10-15: UI save-prompt blocker reproduced for panel/popover layer edits and New layer. `mcp_followups` owns minimal `Session.MarkModified` API, tests and core docs, plus G81 TEXT/MTEXT keyword registration; `ui_fixes` owns calls and UI regressions. Layer undo remains deferred. G73 verification repair tracked separately: platform-aware tooltip expectations and investigation of the documented two-row ribbon at 1100 px.
- 2026-10-15: resumed implementation briefs are in review. Layer save-prompt API/UI, keyword groups, polygon acceptance/docs, remaining markdown scans, bounded failed-queue drain, model-restart queue pause and distinct Stop icon are implemented with focused checks. Coordinator inspected whole-window/ribbon and Stop/Cancel PNGs. Global formatting and root vet pass; docs lint passes. `just check-agentmanager` passes on a separate retry after known discovery timeout failures under concurrent runs; nested race retry and root suite are pending. Independent read-only review remains in progress; Cancel queue choice remains unresolved.
- 2026-10-15: independent review found two remaining quadratic markdown paths (adjacent styled-span concatenation and repeated URL-parenthesis counts). `agent_fixes` owns the coherent parser repair with failing-first tests; agent-side work stays in review. Separate `go test -race ./...` retry in agentmanager passes; initial concurrent-run discovery timeout failures remain G72.
- 2026-10-15: independent review complete; two additional markdown performance findings repaired and rechecked. No remaining medium/high defect found within resumed areas. Original detailed historical Low findings are unavailable, so unnamed lows are not claimed resolved. Final root suite is running on settled production code; root vet/docs lint pass. A reviewer scratch probe needs formatting to satisfy literal global gofmt. Queue acceptance remains pending; no live UI/real-agent tests or commits.
- 2026-10-15: final combined verification passes: global `gofmt -l .` empty, root `go vet ./...` and `go test ./...` (UI 176.748 s), docs lint (34 documents), icon registry check (67 icons), `just check-agentmanager`, and nested `go test -race ./...`. Nested checks pass when retried separately; initial concurrent runs hit G72 discovery timeouts. Initial root run caught an in-flight failing-first Stop-icon test, then the settled full run passes. Before/after headless renders inspected; interactive window and real-agent tests are not run. G73/G74/G81 closed; G16 narrowed to remaining layer undo/typed-command metadata issues. User authorizes an end-of-work commit without attribution. Agent queue policy stays in review: draft Cancel pauses until Resume or a new Send, awaiting user choice.
