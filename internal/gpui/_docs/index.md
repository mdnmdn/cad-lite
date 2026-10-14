# Documentation index

go-gpui is a proof-of-concept port of Zed's GPUI to Go: retained entities,
per-frame element trees, flexbox layout, keymap/actions and a text stack,
rendered by a CPU rasterizer to an ebiten window or straight to a PNG.

Audience: **user** (building apps), **contributor** (changing the library),
**agent** (AI working in the repo).

## Table of docs

| Doc | Audience | Purpose |
|-----|----------|---------|
| [../README.md](../README.md) | user | Overview, quick start, features, supported / unsupported list |
| [../AGENTS.md](../AGENTS.md) | agent, contributor | Rules, boundaries, layout, verification |
| [api-contract.md](api-contract.md) | contributor, agent | Normative API contract; change it before code |
| [roadmap.md](roadmap.md) | contributor, agent | Status, completed milestones, open work, limitations, process |
| [gpui-kit-integration.md](gpui-kit-integration.md) | contributor, agent | Pinned upstream investigation and phased native Go component proposal |
| [project-structure.md](project-structure.md) | contributor, agent | File tree, responsibilities, dependency layers, where to add things |
| [index.md](index.md) | all | This page |
| [../docs/README.md](../docs/README.md) | user | Index of user documentation under `docs/` |
| [../docs/getting-started.md](../docs/getting-started.md) | user | Tutorial from first window to headless tests |
| [../docs/examples.md](../docs/examples.md) | user | The 17 examples and what each shows |
| [../docs/architecture.md](../docs/architecture.md) | user, contributor | How the frame pipeline, entities, layout and painting fit together |
| [../docs/concepts/app-and-entities.md](../docs/concepts/app-and-entities.md) | user | App, entities, contexts, subscriptions |
| [../docs/concepts/elements.md](../docs/concepts/elements.md) | user, contributor | Element interface, Div, custom elements |
| [../docs/concepts/events-and-input.md](../docs/concepts/events-and-input.md) | user | Mouse/keyboard events, actions, keymap, focus |
| [../docs/concepts/layout.md](../docs/concepts/layout.md) | user | Flexbox layout model |
| [../docs/concepts/platform.md](../docs/concepts/platform.md) | user, contributor | Platform interfaces, headless, ebiten, clipboard |
| [../docs/concepts/styling.md](../docs/concepts/styling.md) | user | Style builders, backgrounds, shadows, state styles |
| [../docs/concepts/text.md](../docs/concepts/text.md) | user | Text, fonts, shaping, TextInput |
| [../docs/guides/cookbook.md](../docs/guides/cookbook.md) | user | Compiling recipes: buttons, lists, forms, overlays, shortcuts, tab navigation |
| [../docs/guides/testing.md](../docs/guides/testing.md) | user, contributor | Headless tests, synthetic events, pixel assertions, golden PNGs, benchmarks |
| [../docs/guides/porting-from-gpui.md](../docs/guides/porting-from-gpui.md) | user | Rust GPUI to Go mapping table and known differences |
| [../docs/guides/troubleshooting.md](../docs/guides/troubleshooting.md) | user, agent | Diagnosing missing text, slow frames, dead key bindings, focus and display errors |
| [../docs/guides/contributing.md](../docs/guides/contributing.md) | contributor, agent | Contract-first workflow, verification commands, test and example conventions |
| [../docs/reference/README.md](../docs/reference/README.md) | user | Index of the API reference pages and a name-to-page lookup |
| [../docs/reference/geometry-and-color.md](../docs/reference/geometry-and-color.md) | user | Reference: geometry, colour, fills, shadows |
| [../docs/reference/style-builders.md](../docs/reference/style-builders.md) | user | Reference: every fluent style builder on Div, Text, StyledMixin, UniformList |
| [../docs/reference/elements.md](../docs/reference/elements.md) | user, contributor | Reference: Div, Text, TextInput, UniformList, ScrollHandle, custom elements |
| [../docs/reference/events.md](../docs/reference/events.md) | user | Reference: event types, listeners, keystrokes, keymap, actions, focus |
| [../docs/reference/app-window.md](../docs/reference/app-window.md) | user | Reference: App, Entity, Context, Subscription, Window methods |
| [../docs/reference/platform.md](../docs/reference/platform.md) | user, contributor | Reference: Platform interfaces, headless, RenderToPNG, scene, ebitenplatform |
| [../docs/reference/text-and-fonts.md](../docs/reference/text-and-fonts.md) | user, contributor | Reference: FontManager, Shaper, TextStyle, metrics |

`docs/README.md` is the index of `docs/`. When adding a page under `docs/`,
list it there and in the table above; `find docs -name '*.md'` gives the
current set.

## Reading orders

I want to use it:

1. [../README.md](../README.md)
2. [../docs/getting-started.md](../docs/getting-started.md)
3. [../docs/examples.md](../docs/examples.md), then run `go run ./examples/<name>`
4. [../docs/README.md](../docs/README.md) and the concept pages you need
5. [roadmap.md](roadmap.md) section 3-4 to see what is missing

I want to contribute:

1. [../README.md](../README.md), [../AGENTS.md](../AGENTS.md)
2. [project-structure.md](project-structure.md)
3. [../docs/architecture.md](../docs/architecture.md)
4. [api-contract.md](api-contract.md) for the area you touch
5. [roadmap.md](roadmap.md): pick a milestone, follow Process (section 5)

I am an AI agent:

1. [../AGENTS.md](../AGENTS.md) (rules)
2. [project-structure.md](project-structure.md) (where things live)
3. [api-contract.md](api-contract.md) (edit first when signatures change)
4. [roadmap.md](roadmap.md) (what to work on, verification commands)
5. [../docs/architecture.md](../docs/architecture.md) and relevant `docs/concepts/` pages;
   update them with behaviour changes
