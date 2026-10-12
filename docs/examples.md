# Examples

All 17 programs live under `examples/<name>/main.go`. Each one opens an
interactive window by default and accepts:

- `-png PATH` render one frame headlessly to PNG and exit (no window or GPU);
- `-width N`, `-height N` logical size;
- `-smoke MS` quit an interactive window after MS milliseconds.

Some add flags that seed state a headless render cannot reach (focus, scroll
offset); they are listed per example. The header doc comment of each
`main.go` is the authority on what was left out. "Ported from" paths are
relative to `crates/gpui/examples/` in gpui-ce; the three originals have no
upstream.

Run any of them with `go run ./examples/<name>`.

## Original

| Example | Shows | Run |
|---|---|---|
| `counter` | The minimal app: a count, +/- buttons with hover styles via `OnHoverStart`/`OnHoverEnd`, `up`/`down` key bindings through actions. | `go run ./examples/counter` |
| `todo` | `TextInput`, `OnSubmit`, click-to-toggle rows, strikethrough text, scrolling list, `enter`/`escape` bindings in the `textbox` context. | `go run ./examples/todo` |
| `gallery` | Static showcase of layout and styling (flex alignment, borders, radii, `StyledText` runs, overflow clip). | `go run ./examples/gallery` |

## Ported from gpui-ce

| Example | Ported from | Shows | Not ported | Run |
|---|---|---|---|---|
| `hello_world` | `legacy/hello_world.rs` | Greeting and colour swatches in a bordered, centred column. | `shadow_lg` on the root; dashed swatch borders (solid). | `go run ./examples/hello_world` |
| `actions_and_keybinds` | `learn/actions_and_keybinds.rs` | Typed actions bound to `space` and `backspace` in a key context on a focused view. | - | `go run ./examples/actions_and_keybinds` |
| `layout` | `learn/layout.rs` | Flex rows/columns, align/justify, grow/shrink, app shell, centring, scrolling regions. | CSS grid (flex rows instead); absolute positioning in the stack pattern (staggered padding, no overlap). | `go run ./examples/layout` |
| `styling` | `learn/styling.rs` | Hover, focus and focus-visible states, conditional styling, group-hover emulation, themed palette; Tab cycles three buttons. | Shadows; element opacity; cursor styles; `active()`; app menu; `flex_wrap` (two rows); native `focus_next`/`focus_prev`. | `go run ./examples/styling` |
| `text` | `learn/text.rs` | Sizes and weights, alignment, decoration, line height, `StyledText` runs, truncation, unicode grid. | Italic; underline/strikethrough colour; weights other than regular and bold; ellipsis, line-clamp, truncate, nowrap are emulated by pre-cutting strings; CSS grid; tofu for CJK/emoji missing from the system font. | `go run ./examples/text` |
| `interactive_elements` | `learn/interactive_elements.rs` | Click counts (single/double/triple), hover callbacks, raw mouse down/up/move, a drag-and-drop interaction built from mouse events. | `on_drag`/`on_drop` and the floating preview; dashed border; cursor styles; CSS grid; scrolling beyond the window. | `go run ./examples/interactive_elements` |
| `tree` | `legacy/tree.rs` | A deeply nested div hierarchy (a layout/paint stress test). `-depth N` or `GPUI_TREE_DEPTH` (default 50). | - | `go run ./examples/tree -depth 200` |
| `scrollable` | `legacy/scrollable.rs` | Two-way scrolling: a vertical page containing a horizontally scrolling strip (`TrackScroll`). `-vscroll`, `-hscroll` seed offsets. | The example font loader. | `go run ./examples/scrollable` |
| `uniform_list` | `learn/uniform_list.rs` | `UniformList`: 10,000 virtualized rows, only the visible range is rendered; `ShowScrollbar`. `-scroll` seeds the offset. | `Colors::for_appearance` (light palette hard-coded); the prelude. | `go run ./examples/uniform_list` |
| `opacity` | `legacy/opacity.rs` | Translucent panel with a shadow, nested content, per-item hover opacity, click to replay a fade-in (stepped in `Render` with `Refresh`; there is no animation API). `-opacity F` sets the initial level. | `img`/`svg` children; emoji row (letters); wavy decoration; absolute positioning (panel sits below the background text); asset source. | `go run ./examples/opacity` |
| `border_and_shadow_backgrounds` | `border_and_shadow_backgrounds.rs` | Coloured borders, outset and inset shadows on rounded cards, vertical scrolling. `-scroll` seeds the offset. | Gradient borders and shadows (first stop used); `rounded_smoothing`; `flex_wrap` (two rows of two). | `go run ./examples/border_and_shadow_backgrounds` |
| `tab_stop` | `legacy/tab_stop.rs` | Tab order with explicit `TabIndex`, a non-stop item, `Tab`/`Shift-Tab` via `FocusNext`/`FocusPrev`. `-focus N` seeds focus. | The example font loader. | `go run ./examples/tab_stop` |
| `focus_visible` | `legacy/focus_visible.rs` | `Focus` vs `FocusVisible` styles: click gives the focus border, Tab gives the keyboard one. `-focus N`, `-keyboard` seed focus. | `cursor_pointer`. (Headless cannot press keys, so `-focus` seeds focus and uses the keyboard border for `focus()`.) | `go run ./examples/focus_visible` |
| `gradient` | `legacy/gradient.rs` | Linear gradients at many angles and stop positions on rounded boxes; a button toggles Oklab/sRGB interpolation. `-oklab` seeds the space. | The canvas/`paint_path` trapezoid with gradient fill (a gradient box stands in); the button's `active()` style. | `go run ./examples/gradient` |

## Headless snapshots

```
go run ./examples/gradient -png gradient.png -oklab
go run ./examples/uniform_list -png list.png -scroll 4000
go run ./examples/focus_visible -png fv.png -focus 1
```

PNGs are gitignored (`*.png`).

## Common gaps

Across the ports the same missing features recur; they are tracked as open
follow-ups in `_docs/roadmap.md`: absolute positioning, `flex-wrap`, `active()`
styles, an animation API, `img`/`svg`, colour emoji, CSS grid, cursor styles,
drag and drop, dashed borders and italic faces.

See also: the [cookbook](guides/cookbook.md) for small compiling recipes, [contributing](guides/contributing.md#example-conventions) for example conventions, and the [API reference](reference/README.md).
