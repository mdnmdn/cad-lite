# GPUI Kit integration investigation

Status: **investigation complete; implementation is a backlog proposal**.
Reviewed on 2026-10-13 against upstream commit
`c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2`
(committed 2026-10-13 15:33:41 UTC, “slider: Add accessibility labels”).
The shallow reference checkout is `_tmp/gpui-kit-upstream/`; it is not vendored.
No Rust build, dependency addition, component port or integration benchmark was
performed. The Go package suggestions below are proposals, not supported APIs.

## Recommendation and scope

Use [longbridge/gpui-kit](https://github.com/longbridge/gpui-kit) as a pinned
behavior and design reference for a **selective native Go component layer**.
Begin with a themed text button and controlled checkbox, rendered by the
existing Go pipeline. Keep the CPU renderer, no-cgo core and ebiten platform
boundary. There is no drop-in compatibility between these runtimes.

This route fits the project contract and provides a small test of the useful
parts of Kit: semantic theme tokens, consistent controls, stable IDs, focus,
keyboard activation and application-owned values. It does not commit the
project to porting the whole catalog. Kit's editor, dock, shell and platform
features remain separate future decisions.

Local baseline: L1 (absolute positioning), L2 (flex wrapping), and S1 (pressed
styles) are complete. I1 (scrollbar dragging/track clicks) is the concurrent
implementation wave and is not an integration prerequisite for the first
controls. Consult the [roadmap](roadmap.md) for its final status and commits;
this document does not replace the roadmap.

## What the upstream source actually contains

The inspected workspace declares version **0.7.3** and pins its default GPUI
family to **`gpui-pre-* =0.3.9`**. It also has an optional GPUI Fast backend.
These are source manifest facts, not claims about the latest published release.
See the [workspace manifest][workspace] and [Kit manifest][kit-manifest].

| Layer | Actual responsibility | Implication for this Go project |
|---|---|---|
| `crates/kit` / `gpui-kit` | Umbrella re-export of GPUI, Base, optional Component and assets; application/window initialization | Useful application convention, not a portable runtime or protocol |
| `crates/base` / `gpui-base` | Semantic controls, state, focus, overlays, positioning, motion, input, virtualization and dock behavior | Main source of behavioral reference; already more than a collection of empty elements |
| `crates/component` / `gpui-component` | Styled controls and presentation over Base; initialization of theme and window presentation | Main source of token/variant and visual reference |
| `crates/assets` | Icon names and selectable embedded SVG assets; native and WASM source implementations | Asset provenance and SVG support need explicit handling |
| `crates/shell` | Optional JavaScript runtime hosted by Rust | Not part of Kit's umbrella dependency; outside this integration proposal |
| `crates/story`, `story-web`, examples | Native/web showcases and fixtures | Reference for visible states and interactions, not Go acceptance evidence |

This structure is confirmed by [Kit exports/window helper][kit-source],
[Base exports][base-source] and [Component exports/init][component-source].
Upstream [architecture][architecture] separates reusable behavior and necessary
geometry from application presentation. The Go port should preserve that
seam without copying every crate or introducing Rust dependencies.

Kit's `open_window` wraps application content in Base `Root`. Root hosts content
and plugin overlays; Component registers window presentation. This differs from
the current Go `App.OpenWindow`, which directly accepts an `EntityRoot`.
Do not silently substitute Kit startup or root semantics. See [Root source][root].

## Integration approaches

| Approach | Cost and boundary impact | Decision proposed |
|---|---|---|
| Rust library in-process through an FFI shim | Must design an ABI for Rust entities, callbacks, windows, threads and drawing; the inspected Kit facade exposes Rust APIs, not a Go bridge. A cgo route violates the current core contract; another binding mechanism still needs a separately approved platform/runtime design | Reject for this scope |
| Rust process with IPC/offscreen output | Avoids a Go core link but adds process lifecycle, duplicate input/focus state, frame transport and packaging. A screenshot stream does not expose native Go components | No demonstrated need; do not implement |
| Full native Go translation | Preserves runtime boundaries but imports a very large behavior surface and dependency problem into a POC | Too broad for one milestone |
| Selective native Go controls | Build on existing `Div`, `Text`, focus, events and entities; translate specified behavior rather than Rust macros/types | Recommended first experiment |

These are engineering inferences from the manifests and interfaces. No bridge
prototype has established compatibility. Upstream's GPU performance claims,
WASM support and OS support do not transfer to the CPU Go implementation.

## Component and behavior dependency matrix

“Feasible” means a bounded subset can use the local primitives; it does not
mean upstream parity. Dependencies reference existing roadmap IDs. Gaps without
an ID must be recorded as new work before implementation, not hidden in a port.

| Component family / behavior | Local foundation | Remaining dependency or deliberate first-slice omission |
|---|---|---|
| Text button, badge, separator, form rows | `Div`/`Text`, L2 wrapping, S1, focus/tab/key events | Button keyboard activation and disabled propagation must be specified/tested; icon/loader/tooltip omissions need documentation |
| Checkbox, radio, toggle | Controlled root-view or entity state, focus, callbacks, simple quads/text | Tri-state and requested-change semantics; radio-group directional navigation later; accessibility bridge missing |
| Static tabs and tree disclosure | Actions, key contexts, `UniformList` | Selection/focus model and keyboard navigation are component work; variable-height trees need E7 |
| Input/number/OTP fields | Existing single-line `TextInput` | I2 selection, I3 multi-line, I4 IME, L7 scaled caret, E5 asynchronous validation; existing input is a plain struct, not an entity |
| Slider, resizable panels, scrollbars | Mouse events, L1 positioning, scroll handles | I1 only supplies scrollbar interaction; slider/panel capture and cancellation need their own contract; I6 cursor; motion E1/E2 optional |
| Select/combobox, menus, popovers, dialogs, tooltips | L1 and focus provide pieces | E6 overlay/anchor lifecycle, focus trap/restore and dismissal ordering; timers E1; searchable async sources E5 |
| Avatar/image/icon controls | Layout and rounded clips | E3 images, E4 SVG, asset-source/cache/lifetime policy; glyph checkmark must not be assumed portable |
| Progress/spinner/skeleton/toast | Static quads and labels | E1 clock, E2 transitions; notification stacking/dismissal E6 |
| Virtual list / table | `UniformList`, scroll containers, I1 when verified | E7 mixed-height list; table column measurement, sorting, selection and header behavior are additional component work |
| Markdown/HTML/chart/editor | Shaped text and scene primitives | L6 joint styled runs, S6 font variants, I2/I3/I4, E5, image support and custom drawing; parsing/highlighting/LSP dependencies need separate approval |
| Dock / detachable windows | Entities, L1/L2, events | I5 drag/drop, I6 resize cursors, E9 multi-window and persisted layout model; outside first phases |
| Accessible controls | Keyboard focus exists | No semantic accessibility tree/actions or platform bridge in the local contract; this gap has no roadmap ID yet |

Upstream implementation anchors include [Base button][button],
[checkbox][checkbox], [input modules][input], [positioner][positioner],
[virtual list][virtual-list] and [dock][dock]. The breadth of Base's dependencies
is explicit in its [manifest][base-manifest]; Go ports cannot inherit those
Rust libraries or add replacements inside `gpui` by implication.

## First vertical slice: theme + text button + checkbox

Proposed milestone K2: one example with light/dark semantic palettes, two button
variants and an application-controlled checkbox (unchecked/checked/mixed).
Use shape-based indicators that the existing rasterizer can draw; avoid SVG,
emoji, animation, remote assets and popups in this milestone. A check mark shape
may require a small custom element; do not replace it with a font-dependent
symbol and claim cross-platform visual parity.

Acceptance behavior should be decided in the contract before implementation:

- Stable IDs preserve focus through redraws; removing a control releases its
  ephemeral focus/press state. Distinct instances do not share keyed state.
- Primary pointer activation fires once; cancellation, leave and release follow
  documented semantics. Keyboard Enter/Space activation is scoped to the focused
  control and specifies key-down/key-up/repeat behavior and pressed visuals.
- Disabled controls cannot invoke callbacks or take focus through pointer/tab;
  a disabled child cannot accidentally trigger a parent activation handler.
- Checkbox activation requests the next controlled value; the application owns
  mutation. Mixed activates to checked, checked to unchecked, unchecked to checked.
- Theme switch redraws all affected controls; light/dark, hover, pressed,
  focus-visible and disabled appearances are deterministic and legible.

The proposed disabled/focus behavior follows the [Base button][button] model.
The styled [Component button][styled-button] includes legacy presentation and
loading semantics; these are not automatically interchangeable. The first Go
slice must say which behavior it implements. It must also explicitly state that
native screen-reader support is unavailable, rather than advertise Kit parity.

## Contract, packages and ownership

Proposed directory `components/` imports only `go-draw/internal/gpui` and stdlib and is
an optional layer above the core. Do not place controls under ebitenplatform or
make `gpui` depend on components. Start with one package; split an unstyled base
from styled presentation only when reuse justifies that extra public seam.
The new package boundary, tests location and exports require a project-contract
and structure update before files are introduced. Current rules place tests in
`*_test.go`; the coordinator must settle how they apply to a new package.

| Write owner for a future slice | Exclusive responsibility |
|---|---|
| Contract/documentation owner, first | `_docs/api-contract.md`, package/test boundary, theme/value/callback signatures and behavior; user concept/reference docs and roadmap |
| Core owner, only if a proven gap exists | Focus/event/state hooks inside `gpui`; separate brief and regression evidence |
| Component owner after contract | `components/` builders/adapters, behavior, themes and component checks; no core patches without reassignment |
| Example owner after component API | One new example and visual fixture preparation; shared docs stay with their existing owner |

Adapt Go's per-frame builder/adaptor pattern rather than translating `RenderOnce`
macros. A composed control may return a configured `Div` through `IntoElement`;
use a dedicated adapter if it implements the element pipeline itself. Do not
invent adapters solely to resemble Rust types.

Keep a small semantic theme value (surface, foreground, muted, accent,
danger, border/focus, spacing/radius/typography) in an application-owned object
or the existing `App.SetGlobal`/`Global` facility. Theme changes must explicitly
refresh affected windows: Go globals/notifications do not automatically do so.
Upstream [theme tokens][tokens] inform the vocabulary; do not copy its entire
config or syntax-highlight theme into the first controls.

Controlled values remain in application state; complex retained models can use
`Entity[T]`. Listeners upgrade weak handles at invocation and must not retain a
borrowed `*T` across reentry. Element state is for frame-local identity/focus,
not durable domain values: unused state is pruned. Avoid an implicit process
singleton that couples independent Apps or windows.

## Runtime and platform gaps

E5 is necessary before remote data, async search or background parsing.
`Context.Spawn` currently defers work in the effect cycle; it is not a worker
executor. Results must return to the UI goroutine through a designed API before
entities/windows are accessed. Define cancellation and released-owner behavior.

E3/E4 need decoding, caching, sizing and asset-source lifetime rules, not only
an icon enum. Keep optional loading and parsing outside the allowed core imports
unless the contract is explicitly revised. E6 must solve overlay paint/input
ordering, anchor bounds/clipping, collision handling, dismissal and focus restore;
L1 absolute positioning alone does not supply these semantics.

Accessibility needs its own product/platform milestone: roles, names, checked
and disabled states, focus relationships, actions, an inspectable headless tree
and OS adapters. Kit's AccessKit-backed behavior does not imply an ebiten bridge.
Do not add AccessKit/Rust or platform-specific packages to the Go core. This
investigation has not established whether the current backend can support the
required native adapters without a separate platform choice.

## Provenance and licensing

These findings come from the pinned source notices; importing/adapting individual
files still requires checking their headers and third-party origin.

| Material | Evidence and treatment before reuse |
|---|---|
| Kit/Base/Component software, website implementation and code snippets | [Apache-2.0 notice][apache] and package manifests. Preserve applicable copyright/license notices, identify modified files and retain any applicable NOTICE attribution; record source SHA/path |
| Rights-cleared site prose/original illustrations in the directories named by the notice | [Documentation notice][docs-license] additionally offers CC BY 4.0 from the revision adding that notice. If adapting protected prose, credit GPUI Kit, link the source/license and identify changes; this is not a blanket relicensing of third-party material or all internal `docs/` |
| Lucide icons and listed Feather-derived icons | [Asset notice][lucide-license] gives ISC for Lucide and MIT for the specified Feather-derived subset. Include the applicable copyright and permissions text; package Apache metadata alone is insufficient |
| Bundled story-web fonts | [OFL notice][font-license] names several font families and reserved names. Check each selected font and preserve its notice; do not silently import the showcase font directory |
| Other illustrations, theme JSON and retained custom icons | Verify file-level provenance before copying. No individual provenance audit was completed for the whole catalog; do not treat the repository license as proof for every asset |

This investigation paraphrases architecture and cites source; it copies no code,
prose, font or asset into tracked files. A future port should carry a small
provenance list (source commit/path, original owner, license, local adaptation)
for each reused implementation or asset, without vendoring the whole workspace.

## Phases and decision gates

| Proposed item | Status / prerequisites | Bounded output and exit gate |
|---|---|---|
| K1: investigation | Completed research only | This pinned source analysis and roadmap links; no implementation claim |
| K2: first controls | Backlog; L2/S1 done; package/test and activation contract first | Semantic theme, text button, controlled checkbox and one headless example; exclude assets/overlays/motion and document accessibility gap |
| K3: overlay control slice | Backlog; E6, and E1 for delayed tooltips | One popup or dialog with dismissal, focus restoration and edge positioning; choose the control before dispatch |
| K4: assets and richer data controls | Backlog; E3/E4 and applicable I2/E5/E7 | Select one named asset/data control after prerequisites; no automatic catalog-wide port |

The user requested this investigation and a roadmap entry. They have not chosen
package naming, full styled parity, a screen-reader product promise, a Rust ABI
or JavaScript extensions. Those choices are proposals; investigation completion
does not settle them. A coordinator may take K2 into contract review as the
smallest concrete implementation brief, while continuing already authorized
core milestones independently.

## Verification for any implementation phase

Use source-derived scenarios for behavioral acceptance, not Rust snapshots as
Go pixel goldens. Add meaningful headless dispatch tests for activation,
focus/tab, controlled values, disabled propagation and state cleanup; include
regressions that fail before a core fix. Compile published Go snippets in a
package under the repo root's `_tmp/` (see `docs/guides/contributing.md`).

Before/after affected-example PNGs belong under `_tmp/`. Inspect light/dark and
interaction states at scales 1 and 2; pin fonts or account for platform font
variation. Existing examples should remain pixel-identical unless an intentional
core change is recorded. Run `gofmt -l .`, `go vet ./...`, `go test ./...` and
`go mod tidy` after new imports. Compare paired `Draw` benchmarks and allocations
against the same machine/baseline; Kit's advertised FPS is not a local budget.

The new component example needs a meaningful frame benchmark when it exists.
A clear regression to the existing todo frame is a bug. Verify real-window
keyboard, pointer, HiDPI and any native accessibility behavior on a user machine;
this environment has no usable GPU/window server. Headless evidence must not be
reported as native-interaction or screen-reader verification.

[workspace]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/Cargo.toml
[kit-manifest]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/kit/Cargo.toml
[kit-source]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/kit/src/lib.rs
[base-source]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/lib.rs
[component-source]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/component/src/lib.rs
[architecture]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/docs/ARCHITECTURE.md
[root]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/root.rs
[button]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/button.rs
[checkbox]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/checkbox.rs
[input]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/input/mod.rs
[positioner]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/positioner.rs
[virtual-list]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/virtual_list.rs
[dock]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/dock/mod.rs
[base-manifest]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/Cargo.toml
[styled-button]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/component/src/button/button.rs
[tokens]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/base/src/theme_tokens.rs
[apache]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/LICENSE-APACHE
[docs-license]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/LICENSE-DOCS.md
[lucide-license]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/assets/LICENSE-LUCIDE
[font-license]: https://github.com/longbridge/gpui-kit/blob/c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2/crates/story-web/fonts/OFL.txt
