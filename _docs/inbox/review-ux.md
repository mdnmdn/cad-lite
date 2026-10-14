# UX review: go-draw interface and interaction design

Reviewed from commit c48c7ec (worktree), headless `-png` renders and the real window under Xvfb.
Shots are in `_tmp/review-ux-shots/` (gitignored); GUI-harness shots are under `gui/ux/`. The mid-drag
rubber-band box was not captured (the harness shots are taken after release), so the window/crossing
box styling is judged from `ui/theme.go` and `_docs/features/canvas.md`. Keyboard-only traversal and
text scaling were assessed from code (no tab stops or OS text-scale hook in `ui/`), not run.

## Executive summary

The shell is clean and coherent: a dark AutoCAD-like canvas, an icon+label toolbar, a prominent
command line with AutoCAD-style prompts, readable status toggles, and a tidy agent panel. Core CAD
conventions (crosshair, pickbox, snap markers, Space/Enter/right-click, F-keys, window vs crossing
colours) are right. The gaps are mostly discoverability and affordance, not structure:

1. **No tooltips, no shortcut hints, no disabled states** anywhere (toolbar, status flags, layer pills).
2. **Weak interactive affordances**: hover/button fills are 1.13:1 against their surroundings, the
   "Reject" permission button is effectively invisible, and the layer `on/off` pill reads as text.
3. **Feedback is quiet**: errors in history look identical to normal output, no snap tooltip, no
   dynamic input, no selection grips; empty/first-run state is the demo drawing or a blank canvas with no hint.
4. **Agent trust UI** is functional but under-signalled: permission cards look like tool cards, there is
   no running/progress indicator, Cancel is shown without a running turn in the demo, and the typed
   command shows the unsent command rather than any indication of where focus is.
5. **Layout**: the fixed 210 px layers panel plus a 340 px agent panel leave the canvas 360 px wide at
   900x560, and opening the panel crops the drawing instead of keeping it framed.
6. **Accessibility**: focus visibility is only the command field; colour is the only cue for the window
   (blue) vs crossing (green) boxes; several tokens are below 4.5:1 (see contrast table).

## Findings (ranked)

| ID | Sev | Area | Observation (evidence) | Why / best practice | Recommendation | Effort |
|---|---|---|---|---|---|---|
| UX-01 | P1 | Discoverability | Toolbar buttons and status flags have no tooltip; no shortcut or alias is shown anywhere (`10-hover-toolbar.png`: hover only tints the button). `internal/gpui/div.go` already supports tooltips; `ui/` does not use them. | Fluent/HIG: icon controls need a name + shortcut tooltip on hover/focus (~500 ms). AutoCAD shows command name, description and alias. | Add tooltip to every `button()`, `flag()`, layer pill, Agent: `"Line  (L)"`, `"Object snap  (F3)"`, `"Agent  (F2)"`, `"Undo  (Ctrl+Z)"`. Use `uiPanelAlt` bg, 1 px `uiBorder`, 12 px text, 500 ms delay. | M |
| UX-02 | P1 | States | Hover fill `uiPanelAlt` (44,50,58) on `uiPanel` (36,41,48) is 1.13:1 (`10-hover-toolbar.png` shows a barely visible tint). Same fill is used as the idle fill for buttons, the `on` pill, chips and cards, so hover cannot be distinguished by fill from rest. Pressed state does not exist. | WCAG 1.4.11 asks 3:1 for state-bearing component boundaries; at least hover must be clearly perceptible. | Add token `uiHover = rgb(58,66,78)` for hover and `uiPressed = rgb(30,35,41)`; add a 1 px `uiBorder` (or `rgb(88,98,112)`, 2.3:1) outline on hover for buttons. Give the Agent toggle and active tool a 1 px `uiOn` outline in addition to the fill. | S |
| UX-03 | P1 | Agent / trust | `Reject` is rendered as a `uiPanelAlt` button inside a `uiPanelAlt` card, so it has no visible shape (`03-agent-demo.png`, bottom card). The permission card is the same style as tool cards; only a 11 px amber title distinguishes it. | A destructive/consent decision must have two clearly formed, equally findable buttons and be visually distinct (HIG alerts). | Permission card: 2 px `uiWarn` left border, `rgb(58,46,20)` tint bg, heading 12 px bold. Buttons: Allow = `uiAccent`; Reject = 1 px `uiErr`-outline ghost button (text `uiErr`). Show what will run in a monospace block (`Run shell command: ls`). Add "Allow always" only as a separate, secondary option. Answered state: show which option was chosen, not just "answered". | S |
| UX-04 | P1 | Agent / feedback | No indicator that a turn is running (no spinner, elapsed time or status text besides `stopped` / `starting...` / `ready`); Cancel appears next to Send which is still shown and does nothing (`03-agent-demo.png`). Header status text is 11 px `uiMuted`. | Long async work needs visible progress and an obvious stop (Nielsen: visibility of system status). | While running: replace Send by Cancel (same slot, `uiErr`-ghost), show a pulsing 8 px dot (uiWarn) and "Working..." in the header, and keep the input enabled but queued. Status dot colour per state: stopped `uiMuted`, ready `uiOK`, working `uiWarn`. | M |
| UX-05 | P1 | Layout / responsiveness | Side panel is fixed at 210 px and agent panel at 340 px; at 900x560 the canvas is 360 px (`04-small-900x560.png`). Opening the agent panel crops the drawing and nothing re-frames it (`17-agent-typed.png`, plate clipped on both sides). Agent transcript scrolls with the top row cut off in the small render. | Docked panels should be collapsible/resizable; the canvas is the primary surface and needs a minimum (about 480 px). | Make the layers/properties panel and the agent panel resizable (180-320 / 300-480) with drag handles, collapse the layers panel automatically below 1100 px window width, keep the view centre fixed relative to the canvas when it resizes (or Zoom Extents if the user has not zoomed). Consider moving the agent into the same right column as a tab ("Layers / Agent"). | M |
| UX-06 | P1 | Accessibility | Only the command field has a focus ring (`16`/`17` shots). Toolbar, status flags, layer rows and agent controls are mouse-only; the command field steals focus on every canvas press. `internal/gpui/examples/tab_stop` and `focus_visible` exist but `ui/` does not use tab stops or focus-visible styling. | WCAG 2.1.1 keyboard, 2.4.7 focus visible, 2.4.11 focus appearance (2 px, 3:1). A CAD user can type every command, but flags, layer visibility and agent permissions cannot be reached by keyboard. | Add tab stops to toolbar buttons, status flags, layer rows/pills and agent buttons, with a focus-visible ring: 2 px `uiOn` outline, 2 px offset. Add shortcuts for the Agent permission buttons (Enter = default, Esc = reject) when the agent field has focus. Keep the command line as the default focus target. | M |
| UX-07 | P2 | CAD conventions / feedback | Snap marker is drawn but has no text label ("Endpoint", "Center") and no tooltip near the cursor (`02-line-snap.png`: yellow circle with the crosshair only). No dynamic input: the length/angle of the rubber band is not shown at the cursor (`12-line-rubberband.png` equivalent in `02`). Coordinates in the status bar are unrounded (`134.0486, 54.8767`). | AutoCAD/BricsCAD show a snap tooltip and dynamic dimensions; they reduce looking down to the command line. | Draw a 11 px label next to the marker (`uiText` on `uiPanel` 90% pill, 6 px padding, offset 14,14 px). Add an optional dynamic input pill: `length < angle` near the cursor, toggled by F12. Round status coordinates to the current display precision (4 decimals, trailing zeros trimmed). Snap marker 10 px, 2 px stroke. | M |
| UX-08 | P2 | Feedback | Errors, warnings and echoed input share one style: `Unknown command "FOO". Press F1 for help.` is plain `uiMuted` (`08-error.png`), and F1 help does not exist (command-line.md lists it as a next step). The history is 4 lines, not scrollable or expandable. | Errors must be distinguishable (not by colour alone) and must not point at nonexistent help. | Colour error lines `uiErr` with a leading `!`, warnings `uiWarn`; prompts `uiText`, echoed input `uiMuted`. Change the message to `Unknown command "FOO". Type HELP or press F2 for the agent.` until F1 exists. Make the history expandable (click or F2-style toggle, up to 12 lines) and add Up/Down recall. | S |
| UX-09 | P2 | Discoverability / empty state | A blank/first-run canvas gives no hint: `-script ""` still draws the demo (`06-empty.png` is identical to idle), and a drawing with no entities has no onboarding text; the command field placeholder is always "Type a command" even when a prompt is active (`07-circle-prompt.png`, prompt says "Specify radius..." yet placeholder says "Type a command"). | First-run needs an empty state; the placeholder should reflect context. | Empty drawing: centred 14 px `uiMuted` text over the canvas "Start with a tool above, or type LINE and press Enter. Ask the agent with F2." Placeholder: `Command` when idle, `Value, x,y or keyword` when a prompt is active. Show the default value in the prompt like AutoCAD `<10>`. | S |
| UX-10 | P2 | Selection | Selection is blue thicker line only; no grips, no selection count in the status bar, and the properties panel is read-only text (`15-selected-props.png`). Window (blue) vs crossing (green) differ by hue only; both fills are alpha 40. Crossing vs window is not also encoded by line style. | WCAG 1.4.1: do not use colour alone; AutoCAD uses dashed outline for crossing. Blue/green are separable for most CVD but low in luminance difference (outline 5.1:1 vs 8.5:1 helps). | Draw the crossing box with a 6/4 px dash and the window box solid. Add `uiSelect` fill `rgba(80,160,255,0.16)` / crossing `rgba(90,220,120,0.14)`. Add grips (6 px squares, `uiOn` fill, 1 px `uiBg` stroke) at endpoints/midpoints/centres. Status bar: `4 selected`. | M (grips L) |
| UX-11 | P2 | Visual design | Toolbar labels are 10 px (`10px label`); status flags 12 px; panel headings 11 px bold muted; agent meta 11 px. Several sizes (10, 11, 12, 13, 14) have no scale. Brand "go-draw" in the toolbar is accent bold but not a control. Icons are consistent (stroke ~1.5 px, `audit-draw.png`) but the `AI` toggle is text, not an icon, and Zoom shows only the window glyph. | Min 11-12 px for UI text; limit to a 4-step type scale. | Scale: 11 (meta/label) / 12 (body, controls) / 13 (command line, titles) / 14 (panel titles). Toolbar labels 11 px. Replace the `AI` glyph with a sparkle icon from the icon pipeline (`assets/icons/icons.yaml` new entry `app-agent`) and keep the "Agent" label. Spacing scale 4/8/12/16 (currently 2, 4, 6, 8, 10 mixed). | S |
| UX-12 | P2 | Layers panel | The `on`/`off` text pill (`11-hover-layer-on` area; `01-idle-demo.png`) is a text word, not a toggle; off layers are only shown by muted text and the swatch stays at full colour; layer 0 swatch is white on dark. No lock/freeze/new-layer or colour change, the panel has no empty space guidance. | Eye/visibility icons are the CAD convention; state must be shown by more than colour. | Replace the pill with an eye / eye-off icon (16 px, `uiText` / `uiMuted`), dim the swatch and name (opacity 0.45) for off layers, add `aria`-style label "Layer outline: visible". Current layer: left accent bar 2 px `uiOn` plus bold, not colour alone. | M |
| UX-13 | P2 | Toolbar / hierarchy | 16 tools plus the Agent toggle in one 48 px row; no disabled states (Undo/Redo are always active; Move/Copy/Erase work with no selection prompt flow). Active tool fill `uiAccent` with white 10 px text is 3.68:1 (below 4.5:1 for small text). Brand and toolbar share the row. | Disabled states communicate availability; contrast for small text. | Disable Undo/Redo (opacity 0.4, no hover) when the stack is empty. Active button: fill `rgb(37,99,235)` (white 5.2:1) with label 11 px bold. Group labels are not needed, but tool order matches AutoCAD ribbon (Draw / Modify / Annotate) so keep. | S |
| UX-14 | P3 | Canvas | Grid minor/major are 1.24:1 / 1.57:1 on the canvas, subtle but fine for a background; grips absent; pure-blue ACI 5 lines (0,0,255) are 1.73:1 on `colCanvas` (33,40,48) and ACI 1 red 3.72:1. | Entities must stay legible on the canvas; AutoCAD remaps dark colours on dark backgrounds. | Apply a legibility remap for entity colours below 3:1 (lighten in HSL to L>=55%) at draw time, keep the stored ACI. Crosshair at 7.5:1 is good. | M |
| UX-15 | P3 | HiDPI | The 2x render (`05-scale2.png`) is crisp: icons rasterise at device scale, text is clean, and spacing holds. Nothing to fix except that the 1 px hairlines (`uiBorder`, status flags) are 1 logical px = 2 physical, fine. | - | Keep. Add a test render at scale 1.5 and 3 to the review checklist. Honour OS text scaling by making `TextSize` multiples of a root size. | S |
| UX-16 | P3 | Agent transcript | User bubble `uiText` on `uiAccent` is 2.64:1 (`03-agent-demo.png`, blue "You" bubble), a contrast failure for 13 px text. Tool result JSON is raw (`{"new_handles":["2A"]}`), "Thinking" is a collapsed `> Thinking` text link without hover state, plan marks use `[x]`/`[~]`/`[ ]` text. | Transcript readability: AA contrast, humane tool summaries. | User bubble: `rgb(30,58,110)` bg with `uiText` (8.6:1) and a 1 px `uiAccent` border; align it right. Tool card: title "Add rectangle", result "1 entity created (2A)" and a "Show" link that selects the handles. Plan: 14 px check icons. Thinking: chevron icon + hover. | M |
| UX-17 | P3 | Window chrome | The app uses the full window with no menu bar, no file name / dirty marker, no title context, and `OPEN/SAVE` only by command. Command-first is acceptable but a first-time user has no visible way to open or save. | Desktop conventions: title shows document + modified dot; a File menu or toolbar entry. | Window title `name.dxf - go-draw` plus `*` when modified; add a compact File group (New, Open, Save) at the left of the toolbar, or a `...` menu listing commands with shortcuts (Ctrl+O, Ctrl+S, none are bound today). | M |

## Contrast table

Computed with the WCAG relative-luminance formula from the `ui/theme.go` tokens (alpha colours
flattened over their background).

| Pair | Ratio | Target | Verdict |
|---|---|---|---|
| uiText on uiPanel | 10.51 | 4.5 | pass |
| uiText on uiBg | 12.29 | 4.5 | pass |
| uiText on uiPanelAlt | 9.28 | 4.5 | pass |
| uiMuted on uiPanel | 4.88 | 4.5 | pass (close) |
| uiMuted on uiBg (status bar, placeholder) | 5.70 | 4.5 | pass |
| uiMuted on uiPanelAlt (off pill) | 4.31 | 4.5 | fail (small text) |
| uiOn on uiBg (status flags) | 6.73 | 4.5 | pass |
| uiOn on uiPanel (current layer) | 5.76 | 4.5 | pass |
| white on uiAccent (active tool, 10 px) | 3.68 | 4.5 | fail |
| uiText on uiAccent (You bubble) | 2.64 | 4.5 | fail |
| uiErr on uiPanel / uiPanelAlt | 4.28 / 3.78 | 4.5 | fail (11 px status text) |
| uiOK on uiPanelAlt | 5.47 | 4.5 | pass |
| uiWarn on uiPanelAlt | 6.61 | 4.5 | pass |
| uiAccent focus border on uiBg | 4.65 | 3.0 | pass |
| uiBorder on uiPanel (component boundary) | 1.42 | 3.0 | fail for interactive boundaries |
| uiPanelAlt on uiPanel (button fill / hover) | 1.13 | 3.0 | fail |
| Cyan (outline layer) on canvas | 11.87 | 3.0 | pass |
| ACI red (255,0,0) on canvas | 3.72 | 3.0 | pass |
| ACI blue (0,0,255) on canvas | 1.73 | 3.0 | fail (needs remap) |
| colSelected (80,160,255) on canvas | 5.54 | 3.0 | pass |
| Window outline (90,150,255) / crossing outline (90,220,120) on canvas | 5.13 / 8.47 | 3.0 | pass, but hue-only difference |
| colSnap yellow on canvas | 9.60 | 3.0 | pass |
| Crosshair (alpha 200) on canvas | 7.53 | 3.0 | pass |
| Grid minor / major on canvas | 1.24 / 1.57 | n/a | decorative, acceptable |
| UCS X axis red (230,80,80) on canvas | 3.99 | 3.0 | pass |

Suggested token fixes: `uiAccent` buttons with text use `rgb(37,99,235)` (white 5.2:1); `uiErr` text
`rgb(248,113,113)` (about 5.3:1 on uiPanelAlt); `uiMuted` `rgb(150,160,172)` to give the off pill 4.9:1;
new `uiHover` `rgb(58,66,78)`, `uiStroke` `rgb(88,98,112)` for interactive outlines.

## Quick wins (each S, under a day)

1. Tooltips with shortcuts on toolbar and status flags (UX-01); needs only existing gpui tooltip support.
2. `uiHover`/`uiPressed` tokens, visible Reject button, error/warn colours in history (UX-02, UX-03, UX-08).
3. Contrast token swaps above (active tool, You bubble, error text, off pill).
4. Context-sensitive command placeholder and an empty-canvas hint (UX-09).
5. Disable Undo/Redo when empty; round status coordinates (UX-13, UX-07).
6. Replace "Press F1 for help" until F1 exists (UX-08).
7. Dashed outline for the crossing box (UX-10).
8. Toolbar label size 10 to 11 px (UX-11).

## Proposed layout sketch (if layout changes are accepted)

Keep the AutoCAD-like frame; collapse the two docked panels into one resizable right dock with tabs and
move the history above the command line into a taller, expandable block.

```
+----------------------------------------------------------------------------------+
| go-draw  [File v] | Line Pline Circle Arc Rect | Move Copy Rot Scale Erase | ...  |
|                                             Dist Area | Undo Redo Zoom   [Agent] |
+-------------------------------------------------------------+--------------------+
|                                                             | [Layers][Agent]    |
|                                                             |--------------------|
|              canvas (min 480 px)                            |  o 0               |
|        crosshair + snap label "Endpoint"                    |  o outline  <cur>  |
|                  dyn input: 42.0 < 30                       |  o holes           |
|                                                             |--------------------|
|                                                             | PROPERTIES         |
+-------------------------------------------------------------+ 4 selected         |
| (expandable history, 4 to 12 lines, errors in red)          +--------------------+
| Specify next point or [Close/Undo]: [ Value, x,y or keyword ]                    |
+----------------------------------------------------------------------------------+
| 134.05, 54.88 | GRID OSNAP ORTHO DYN | 4 selected | Layer outline | 10 ent | 1:4.4 |
+----------------------------------------------------------------------------------+
```

The right dock is 220-480 px wide, resizable and collapsible (F2 for Agent, a Layers shortcut for the other tab).

## Not run / limits

Interactive pointer-driven behaviour beyond the scripted shots (drag rubber-band appearance, hover
snap tooltips, real tab order, OS text scaling) was not exercised. Contrast numbers are computed, not
measured from screenshots.
