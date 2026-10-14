---
id: tech-ui-framework
title: UI framework and theme
kind: tech
status: current
summary: The boundary between go-draw and the internal/gpui library, what the application uses from it, the conventions of the shell, and the theme colours.
read_when: you are changing the UI shell, need something new from gpui, or are touching a theme colour
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/theme.go, ui/workspace.go, render/gpuirender/paint.go, internal/gpui/stroke.go, internal/gpui/canvas.go]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# UI framework and theme

## What it is

The UI shell (`ui`) is built on `internal/gpui`, a Go port of Zed's GPUI: retained state,
per-frame element trees, flexbox layout, a keymap with actions and key contexts, and a CPU
rasterizer with no cgo. The same view renders to an ebiten window (`internal/gpui/ebitenplatform`)
or headlessly to an image, which is how go-draw renders PNGs and runs UI tests.

`internal/gpui` is a separate library with its own documentation and rules: its entry point is
`internal/gpui/_docs/index.md`, its normative API is `internal/gpui/_docs/api-contract.md`, and its
agent rules are `internal/gpui/AGENTS.md`. This document states only the boundary.

## The boundary

What go-draw uses from the library:

| From gpui | Used for |
|---|---|
| `Div`, `Text`, flexbox builders, `Hover` styles | Every panel, button and label |
| `TextInput` with `OnSubmit`, `OnChange`, `OnEscape` | The command line |
| `Canvas` | The drawing area: a leaf that paints through a callback with its bounds |
| `Stroke` via `Window.PaintStroke` | Every line the canvas draws |
| `Quad` via `Window.PaintQuad` | The canvas background and box-selection rectangles |
| Keymap, `NewAction`, key context `cad` | Shortcuts (see the command line feature) |
| `HeadlessPlatform`, `HeadlessWindow.Dispatch` | `-png` renders and UI tests |
| `ebitenplatform.New` | The desktop window |

`Stroke`, `Canvas` and `TextInput.OnEscape` exist in the library for go-draw. A new need follows the
library's rule: its API contract changes first, then its code, then this table.

## Conventions

- `ui.Workspace` is the window's root view and the only `EntityRoot`. It owns the session, the view
  and the render builder, and builds the element tree from them each frame.
- All state the UI shows lives in the session or the workspace; elements are rebuilt every frame and
  hold none.
- The root element sets the key context `cad`; workspace shortcuts are bound in that context
  (`ui.Bindings`).
- The command line keeps keyboard focus: the workspace focuses it on the first frame and whenever the
  canvas is pressed, so typing always reaches it.
- A user action that changes what is shown calls `Window.Refresh`.

## Theme

The shell is dark, with the canvas in AutoCAD's dark model-space colour. Colours are defined once,
in `ui/theme.go`:

| Token | Use |
|---|---|
| `colCanvas` | Canvas background; ACI 7 contrasts with it |
| `colGrid`, `colGridMajor` | Grid lines, every tenth line stronger |
| `colCrosshair` | Crosshair and pickbox |
| `colSnap` | Snap markers |
| `colSelected`, `colHover` | Selection and hover highlights |
| `colWindowSel`/`colWindowOut`, `colCrossSel`/`colCrossOut` | Window (blue) and crossing (green) boxes |
| `colAxisX`, `colAxisY` | The coordinate-system icon |
| `uiBg`, `uiPanel`, `uiPanelAlt`, `uiBorder` | Shell backgrounds, hover fill, separators |
| `uiText`, `uiMuted`, `uiAccent`, `uiOn` | Text, secondary text, the active command, toggles that are on |

`TextInput` draws its own light field, so the command-line field is light on the dark shell; the
library has no styling hook for it (backlog).

## Rationale

**gpui instead of Dear ImGui.** The specification proposed Dear ImGui through cimgui-go. gpui needs
no cgo, so the whole application, UI included, builds for `js/wasm`; it renders headlessly, so UI
behaviour is tested and screenshotted without a display; and it lives in this repository, so the
canvas got the primitives it needed. The cost — widgets built by hand, a CPU rasterizer — is in the
decision register.

## Related docs

- [Architecture](architecture.md) — which packages may import gpui
- [Rendering](rendering.md) — how the canvas uses `Stroke` and `Quad`
- [Decision register](decisions.md) — gpui over ImGui, and the library's place in the module
