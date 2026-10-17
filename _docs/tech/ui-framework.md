---
id: tech-ui-framework
title: UI framework and theme
kind: tech
status: current
summary: The boundary between Cad-lite and the internal/gpui library, what the application uses from it, the conventions of the shell, the theme registry and tokens, and the UI scale.
read_when: you are changing the UI shell, need something new from gpui, or are touching a theme colour
updated: 2026-10-17
verified: 2026-10-17
code_anchors: [ui/theme.go, ui/themes.go, ui/settings.go, ui/slider.go, ui/workspace.go, ui/agent_view.go, ui/icon.go, cmdsys/sysvars.go, assets/icons/icons.yaml, render/gpuirender/paint.go, internal/gpui/stroke.go, internal/gpui/canvas.go, internal/gpui/list.go, ui/textedit_view.go, ui/hud.go, ui/hud_drag.go]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# UI framework and theme

## What it is

The UI shell (`ui`) is built on `internal/gpui`, a Go port of Zed's GPUI: retained state,
per-frame element trees, flexbox layout, a keymap with actions and key contexts, and a CPU
rasterizer with no cgo. The same view renders to an ebiten window (`internal/gpui/ebitenplatform`)
or headlessly to an image, which is how Cad-lite renders PNGs and runs UI tests.

Settings captures `Window.Snapshot` on opening and paints the owned image with
`Window.PaintImage` behind the modal. The workspace tree resumes when Settings closes;
scrolling only rebuilds the modal over the fixed background. Unscaled opaque image runs
copy directly in the CPU rasterizer.

`internal/gpui` is a separate library with its own documentation and rules: its entry point is
`internal/gpui/_docs/index.md`, its normative API is `internal/gpui/_docs/api-contract.md`, and its
agent rules are `internal/gpui/AGENTS.md`. This document states only the boundary.

## The boundary

What Cad-lite uses from the library:

| From gpui | Used for |
|---|---|
| `Div`, `Text`, flexbox builders, `Hover` styles | Every panel, button and label |
| `TextInput` with `OnSubmit`, `OnChange`, `OnEscape` | The command line (single-line) and the agent prompt (`Multiline`) |
| `TextInput.Multiline`, `FocusHandle`, `Div.OnAction` | TEXTEDIT's plain multiline field, trapped field/button navigation and drawing-action suppression while its modal is open |
| `List`, `ListState`, `ScrollHandle` | The agent transcript: a virtualized list of variable-height blocks; only those near the viewport are built |
| `Canvas` | The drawing area: a leaf that paints through a callback with its bounds |
| `Stroke` via `Window.PaintStroke` | Every line the canvas draws |
| `Quad` via `Window.PaintQuad` | The canvas background and box-selection rectangles |
| Keymap, `NewAction`, key context `cad` | Shortcuts (see the command line feature) |
| `HeadlessPlatform`, `HeadlessWindow.Dispatch` | `-png` renders and UI tests |
| `ebitenplatform.New` | The desktop window |
| `Window.PaintGlyph` | Every icon: an alpha mask tinted with one theme colour |
| `Div.Tooltip`, `App.SetTooltipStyle` | A hover tooltip on every clickable control, styled from the theme |
| `Div.OnFileDrop`, `App.OnOpenPaths` | Files dropped on the window and files the OS asks the app to open (macOS Finder and Dock), both into `Workspace.OpenPaths` |

`Stroke`, `Canvas`, `TextInput.OnEscape`, `TextInput.Multiline` and `List` exist in the library for Cad-lite. A new need follows the
library's rule: its API contract changes first, then its code, then this table.

## Conventions

- `ui.Workspace` is the window's root view and the only `EntityRoot`. It owns the open drawings as
  document tabs, each with its session, view and render builder, and builds the element tree from the
  active one each frame.
- All state the UI shows lives in the session or the workspace; elements are rebuilt every frame and
  hold none. A long scrolling list whose items differ in height is a `gpui.List` with a `ListState`
  kept by the view, keyed per item by its content revision and expanded state. The agent panel
  invalidates cached item heights when the language, theme or UI scale changes.
- The root element sets the key context `cad`; workspace shortcuts are bound in that context
  (`ui.Bindings`).
- The command line keeps keyboard focus: the workspace focuses it on the first frame and whenever the
  canvas is pressed, so typing always reaches it.
- A user action that changes what is shown calls `Window.Refresh`.

## Theme

A theme is a `Theme` value in `ui/themes.go`; the registry holds four, shown in the
[settings panel](../features/settings.md): `dark` (the default and the original colours, with the
canvas in AutoCAD's dark model-space colour), `light` (a light shell and a near-white canvas, where
ACI 7 draws black), `classic` (a near-black canvas in a grey shell, as in AutoCAD's classic
workspace) and `high-contrast` (black with white borders and saturated accents). `ApplyTheme`
copies a theme into the package-level variables of `ui/theme.go`, which every view reads while
rendering, so a redraw shows the new theme without any view being told. `Workspace.applyAppearance`
also updates every tab's builder's `Background` (which decides how ACI 7 is drawn) and invalidates
their caches. Views name a token, never a colour; the tokens, one `Theme` field each:

| Token | Use |
|---|---|
| `colCanvas` | Canvas background; ACI 7 contrasts with it |
| `colGrid`, `colGridMajor` | Grid lines, every `GRIDMAJOR`-th line stronger (five by default) |
| `colCrosshair` | Crosshair and pickbox |
| `colSnap` | Snap markers |
| `colSelected`, `colHover` | Selection and hover highlights |
| `colWindowSel`/`colWindowOut`, `colCrossSel`/`colCrossOut` | Window and crossing boxes |
| `colAxisX`, `colAxisY` | The coordinate-system icon |
| `uiBg`, `uiPanel`, `uiPanelAlt`, `uiBorder` | Shell backgrounds, hover fill, separators |
| `uiText`, `uiMuted`, `uiAccent`, `uiOn` | Text, secondary text, the active command (text in `uiText` must read on `uiAccent`), toggles that are on |
| `uiField`, `uiSelect` | Text field fill and selection highlight; the field border is `uiBorder`, turning `uiAccent` when focused, the placeholder `uiMuted` (all applied by `themeField`) |
| `uiOK`, `uiWarn`, `uiErr` | Status colours of the agent panel: completed, in progress or warning, failed or error |
| `Primary`, `OnPrimary` (`uiPrimary`, `uiOnPrimary`) | The call-to-action button (Send) and its text; the soft accent fill and border of active chips and buttons are `mix`es of `uiAccent` with the shell colours (`activeFill`, `activeBorder`) |
| `uiBackdrop` | The dim layer behind a modal panel |

The floating zoom and Properties HUDs use `uiPanel` with background alpha 200/255. Labels and icons
retain their theme colours without element-wide opacity. Properties header dragging uses a temporary
transparent input layer over the window; the root also routes move/release, with no gpui capture API.

The per-theme values are the literals in `ui/themes.go`; the canvas of each, for reference:

| Theme | Canvas | Shell (`uiBg` / `uiPanel`) | Accent |
|---|---|---|---|
| `dark` | 33, 40, 48 | 24, 28, 33 / 36, 41, 48 | 59, 130, 246 |
| `light` | 252, 252, 253 | 226, 230, 235 / 244, 246, 248 | 96, 165, 250 |
| `classic` | 6, 6, 8 | 58, 58, 60 / 76, 76, 80 | 70, 120, 210 |
| `high-contrast` | 0, 0, 0 | 0, 0, 0 / 10, 10, 10 | 0, 76, 178 |

The viewport label, command line, coordinates and status chips use the monospace family (`monoFamily`; the library bundles Go Mono, BSD-style licence, as the fallback for `monospace`). The command line and the agent prompt are `gpui.TextInput`s themed through `themeField` in
`ui/theme.go`, which uses the library's `Style`, `PlaceholderColor` and `SelectionColor`; a new
field gets the same look by passing through it, and `ApplyTheme` restyles every such field (the
registry holds them weakly).

## Tooltips

`Div.Tooltip(text)` is set on every button and clickable control; the text comes from the catalog
(`i18n.T("ui.*_tip")`) or, for a button that runs a command, is composed from the command's name
and description and its keyboard hint (`Workspace.commandTip` in `ui/tooltip.go`; the convention is in
the [canvas](../features/canvas.md) document). The bubble style is one function of the theme,
`tooltipStyle(Theme)` in `ui/theme.go`: `Field` fill, `Border` outline, `Text` colour, gpui's default
size and 500 ms delay. `Workspace.Render` hands it to `App.SetTooltipStyle` on the first frame and
again whenever the current theme differs from the one last applied, so a theme switch restyles the
bubbles. The style sets `MaxWidth` to 320: longer text wraps at that width and the bubble grows to the
wrapped height (`\n` is a hard break, as in "Name (CMD)\nDescription"); it is clamped to the window.

## UI scale

The whole UI, text included, is zoomed by `Window.SetUIScale` (0.75 to 2.0 in the settings; the
library accepts 0.25 to 4): layout runs at the platform size divided by the factor, the scene is
rasterized at the device scale times the factor, and the window divides pointer positions by the
factor, so no view and no platform does anything. It is a zoom of the finished layout, not a font
size: a narrower logical window reflows. The factor is 1 by default and renders are then identical to
before. `Window.ScaleFactor` is the effective raster scale, which `ui.Icon` uses to rasterize. The
contract is in the library's `_docs/api-contract.md`.

## Icons

Icons are monochrome SVG files that the shell rasterizes to an alpha mask and tints with a theme
colour, the analogue of GPUI's `paint_svg`:

1. `assets/icons/<name>.svg`, one per row of the registry `assets/icons/icons.yaml` (categories
   draw, modify, edit, view, snap, measure, layer, file and app; `source` is `custom` or `adopted:<lucide-name>`
   from Lucide under ISC, with the licence in `assets/icons/LICENSE-lucide`).
2. `icons.SVG(name)` returns the embedded source. The package is data only and imports `embed`;
   its `Name` constants are generated into `names_gen.go` by `just icons-gen`.
3. `ui.IconMask(name, px)` parses the file with oksvg, rasterizes it with rasterx at the device
   size, keeps only the alpha channel and caches the mask by name and device pixels. Both libraries
   are pure Go, so the `js/wasm` build keeps working.
4. `ui.NewIcon(name, size, color)` is the element: a square leaf that does not shrink in a flex
   row and paints the mask through `Window.PaintGlyph`, snapped to the device grid.

Colour inside a file is discarded, so a theme decision is a token at the call site (`uiText`,
`uiMuted`, `uiOn`), never part of an SVG. The icon spec (24-unit viewBox, stroke, ink envelope,
banned tags), the registry rules and the draw, render, review loop are owned by the skill
`.claude/skills/cad-lite-icons/SKILL.md`; the commands are in [Operations](operations.md).

Only `ui` imports `assets/icons`, oksvg and rasterx; the core packages never do
([Architecture](architecture.md)).

## Rationale

**gpui instead of Dear ImGui.** The specification proposed Dear ImGui through cimgui-go. gpui needs
no cgo, so the whole application, UI included, builds for `js/wasm`; it renders headlessly, so UI
behaviour is tested and screenshotted without a display; and it lives in this repository, so the
canvas got the primitives it needed. The cost — widgets built by hand, a CPU rasterizer — is in the
decision register.

**oksvg and rasterx for icons.** They are the one SVG rasterizer that is pure Go, so icons cost
the application no cgo and no wasm exclusion. The review sheets are rendered by resvg instead, so
the tool and the application differ slightly on edge pixels (decision register).

The title bar also hosts the [Feedback](../features/feedback.md) speech-balloon button. Its modal
uses a rendered image as the fixed background and a separate focus/action root; the screenshot
editor annotates image pixels without routing input to the drawing. The dialog and editor share a
local opposite light/dark palette, including text fields and the brush slider, while the application
and screenshot keep their original colours. `NewSlider` follows the application theme; the internal `newSliderWithTheme`
helper allows this editor to supply its own palette.

## Related docs

- [Architecture](architecture.md) — which packages may import gpui
- [Rendering](rendering.md) — how the canvas uses `Stroke` and `Quad`
- [Decision register](decisions.md) — gpui over ImGui, the library's place in the module, and the icon rasterizer
- [Operations](operations.md) — the `icons-*` commands
- [Settings](../features/settings.md) — the panel, the file, and what is persisted
