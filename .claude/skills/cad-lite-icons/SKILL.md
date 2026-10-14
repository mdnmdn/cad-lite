---
name: cad-lite-icons
description: Reference for Cad-lite's icon set — the registry in assets/icons/icons.yaml, the SVG spec every icon obeys, how the app rasterises and tints one, and the draw-render-review loop for adding icons without the set drifting. Use when adding, redrawing or renaming an icon, when a command needs a glyph it does not have, or when running just icons-check / icons-sheet / icons-audit.
---

# Cad-lite icons

The full-colour application icon (not part of this set) is documented in `assets/appicon/README.md`.

An icon is a **monochrome 24×24 SVG** in `assets/icons/`, listed in `assets/icons/icons.yaml`.
`just icons-check` is what a change has to pass. The Go package is `cad-lite/assets/icons`
(`icons.SVG(name)`, typed `icons.Name` constants in the generated `names_gen.go`); the element that
draws one is `ui.NewIcon` in `ui/icon.go`; the theme tokens live in `ui/theme.go`.

## Before you draw anything

1. **Does the concept need an icon?** A label a reader can read is not improved by a glyph beside
   it. State already carried by colour or by a word does not need one.
2. **Is it in Lucide?** Lucide (ISC, `assets/icons/LICENSE-lucide`) is the family the grid and the
   stroke come from. If a Lucide icon fits, write `source: adopted:<lucide-name>` in the row and run
   `just icons-adopt`: it fetches Lucide from the npm registry into `_tmp/` (or reads `LUCIDE_DIR`)
   and rewrites the geometry on our root, dropping `class`, `width`, `height`. The provenance stays
   in the row. `I11` fails an adopted row Lucide does not have.
3. **Is it already here under another name?** `grep` the registry's `keywords`, then
   `just icons-dupes` after drawing, which compares the rendered masks.
4. **Only then draw it**, and only if you can state its goal in one line. CAD-specific marks (snap
   markers, dimension lines, entity glyphs) are drawn, not adopted.

## How the app draws one — and why the spec is what it is

`ui.IconMask(name, px)` parses the SVG with `oksvg` (pure Go, so the wasm build keeps working),
rasterises it with `rasterx` at the **device** size (`size × Window.ScaleFactor()`), keeps **only the
alpha channel** as a `*gpui.GlyphMask`, and caches it by `name+px`. `ui.Icon` paints that mask with
`Window.PaintGlyph(gpui.GlyphSprite{Bounds, Mask, Color})` — the analogue of GPUI's `paint_svg`. The
origin is snapped to the device grid so the sprite is blitted 1:1. Consequences:

- **Colour in the file is discarded.** No gradients, no two-tone, no opacity. An icon that needs two
  colours is two `Icon` elements in Go with two tokens, never one clever file.
- **Theming is free.** The tint is one `gpui.Rgba` at the call site (`uiText`, `uiMuted`, `uiOn`);
  it is not in the cache key. Never encode a theme decision in an SVG.
- **Only what oksvg understands survives**: paths, basic shapes, stroke/fill. That is exactly what
  the spec allows.

`just icons-sheet` renders with resvg (via `resvg-py`), not oksvg. For the plain round-capped
strokes the spec allows they agree; when in doubt look at the real thing with
`go run ./cmd/cad -png _tmp/icons/app.png -scale 2` and read the toolbar.

## The spec

| Rule | Value |
|---|---|
| viewBox | exactly `0 0 24 24`, no `width`/`height` on the root |
| Stroke | `stroke="currentColor"`, `stroke-width="2"`, round caps and joins |
| Fill | `fill="none"` on the root; `currentColor` only for a deliberately solid shape |
| Live area | all ink inside `1.5 … 22.5` — 1.5px of optical padding (ink includes the stroke: circle r ≤ 9.5, a cap reaches 1px past its end) |
| Shapes | at most 4 top-level shapes |
| Forbidden | `<text> <image> <use> <defs> <style> <mask> <clipPath> <filter>`, gradients, `<animate*>`, and `style=` `class=` `opacity=` anywhere |
| Precision | two decimals |
| Name | kebab-case, `<category>-<thing>`, one spelling — file name = registry key = Go constant (`draw-line` → `icons.DrawLine`) |

Optical, not arithmetic: a circle fills the live square, a rectangle sits at ~18×14, a triangle needs
its centroid nudged. Balance by eye at 16–18px, which is where toolbar icons are read. A dot is
`<circle r=".5">` (the stroke makes it r=1.5) or `M12 12h.01`.

`just icons-check` enforces the mechanical half (`I01`–`I11`); a registry row with no file is a
worklist entry, not a failure. **Three rules apply only to what we draw** — shape count, ink envelope
and decimals. Adopted rows are exempt from those three and nothing else. Do not widen the exemption
to reach a drawing of your own that will not fit — that is the drawing telling you something.

Categories: `draw-` (entity creation: line, polyline, circle, arc, rectangle), `modify-` (move, copy,
rotate, scale, erase), `edit-` (undo, redo), `view-` (zoom, pan, grid), `snap-` (osnap, ortho),
`measure-` (distance, area, angle, radius, id-point), `layer-` (visible, hidden, current), `file-` (open, save), `app-` (settings, add, remove, agent, warning, history, prompt, chevron, send, key). A new
command goes into the category of what it does; a new category needs a line here and in the registry.

## The registry

`assets/icons/icons.yaml`. A key is the icon's one name; its value is the **goal** — what a reader has
to understand from it, in one line. Write the goal first.

```yaml
canon: [draw-line, draw-circle, draw-rectangle, snap-ortho]   # frozen reference — never redrawn

categories:
  snap:
    snap-ortho:
      goal: movement is held to right angles, horizontal and vertical only
      source: custom            # or adopted:<lucide-name>
      keywords: [ortho, orthogonal, right angle]
```

## The loop

Work **one category at a time, 6–8 icons per pass**.

1. `just icons-check` — find the rows with no file. Pick one category's worth.
2. Draw the SVGs obeying the spec by construction. When a metaphor is not obvious, draw **four takes**
   into `assets/icons/variants/` as `<name>-1.svg` … `-4.svg` and run
   `just icons-variants --category <category>`. Promote the winner to `assets/icons/<name>.svg` and
   delete the rest; takes are scratch and git-ignored.
3. `just icons-gen` (new rows or files change `names_gen.go`), then `just icons-check`. Fix everything
   before looking at a picture.
4. `just icons-sheet --category draw` and **read `assets/icons/preview/sheet.png`**. Every sheet carries
   the canon strip on top: judge coherence against that. Rows show 16/24/64px in the dark shell and a
   light variant.
5. Redraw what fails and go back to 3. Two passes without convergence means the goal is wrong, not the
   drawing — fix the registry line.
6. `just icons-dupes` before you finish. Every ~20 icons and once at the end, `just icons-audit`
   (one contact sheet per category: which one breaks the family?).

**What to look for in a sheet**, in order: (a) the 16px column — still readable, or mud; (b) weight
against the canon; (c) size — does the ink fill the same optical area as its siblings; (d) metaphor —
the same *kind* of drawing (outline, same corner radius, same level of abstraction).

Judging is better done by a subagent that sees the sheet and this spec but not the prompt that
produced the icons.

## Using an icon in Go

```go
import "cad-lite/assets/icons"

row.Child(ui.NewIcon(icons.ModifyMove, 18, uiText), gpui.NewText("Move"))
```

`ui.NewIcon(name, size, color)` is a fixed square of `size` logical px that does not flex-shrink; it
embeds `StyledMixin`, so margins etc. work. Never name a raw colour: use the `ui*` theme tokens
(active state `uiOn`, inactive `uiMuted`). Core packages (`geom doc spatial render cmdsys fileio`)
never import the icons or the renderer; only `ui` does. Tests: `ui/icon_test.go` rasterises every icon
in `icons.All`, so a broken file fails `go test ./ui`.

## Gotchas

- `names_gen.go` is generated; `just icons-check` fails if it is stale. Never edit it by hand.
- `assets/icons/preview/` and `variants/` are git-ignored.
- `just icons-*` need `uv` (deps are inline in `tools/icons/icons.py`: pyyaml, pillow, resvg-py, rich).
- Lucide is downloaded on first `check`/`adopt` into `_tmp/lucide-static/` (needs registry.npmjs.org).
