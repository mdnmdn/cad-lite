# Contributing

How to change go-gpui without surprising the next person. This is the
user-docs view of the process; the authoritative internal documents are in
`_docs/`:

- [`_docs/index.md`](../../_docs/index.md): the internal docs index and reading
  orders.
- [`_docs/api-contract.md`](../../_docs/api-contract.md): the normative API
  contract.
- [`_docs/project-structure.md`](../../_docs/project-structure.md): file tree,
  responsibilities, dependency layers, where to add things.
- [`_docs/roadmap.md`](../../_docs/roadmap.md): status, open milestones,
  limitations, process.
- [`AGENTS.md`](../../AGENTS.md): the rules for AI agents working in the repo
  (humans should follow them too).

Read [architecture](../architecture.md) before touching the frame pipeline.

## Principles

1. **Contract first.** If an exported signature changes or is added, edit
   `_docs/api-contract.md` before the code, then the code, then the user docs.
2. **Docs describe the code.** `docs/` states what the code does today. When
   behaviour changes, update the page that describes it (reference, concept,
   guide). Where the docs and the contract disagree, the code wins and the page
   says so.
3. **Dependency boundaries.** Package `gpui` imports only the standard library,
   plus `golang.org/x/image` and `github.com/go-text/typesetting`, and only from
   `font.go` / `text.go`. Ebiten is imported only inside `ebitenplatform/`.
   The core reaches a backend only through `gpui.Platform` / `PlatformWindow`.
4. **Style.** Comments only where non-obvious, but every exported identifier
   gets a doc comment. `float32` geometry (`Pixels`); `int` only at raster
   boundaries. No cgo.
5. **Element pattern.** Elements are per-frame values implementing `Element`
   (`ID`, `RequestLayout`, `Prepaint`, `Paint`). Because Go has no overloading,
   the fluent builder `ID(string) *Div` and `Element.ID()` coexist through an
   adapter type (`divElement`): a builder type carrying `IntoElement()` that
   returns a small adapter. Follow it for new elements
   ([elements](../concepts/elements.md#the-divelement-adapter-pattern)).
6. **Entity safety.** Never hold a `*T` from an entity across a call that can
   re-enter the same entity; upgrade the weak handle at invoke time
   (`gpui.Listener`) ([re-entrancy rule](../concepts/app-and-entities.md#the-re-entrancy-rule-agentsmd-rule-7)).
7. **One goroutine.** The UI core is unsynchronised; only `App.Quit` and
   `IsQuitting` are goroutine-safe.

## Workflow

1. Pick a milestone from [`_docs/roadmap.md`](../../_docs/roadmap.md) section 3
   (prefer P1, then smaller). Note its ID in the commit or PR description and
   keep one milestone per commit where practical.
2. Change [`_docs/api-contract.md`](../../_docs/api-contract.md) if signatures
   change.
3. Write the code and tests in `internal/gpui/` (see [where things live](#where-things-live)).
4. Update `docs/`: reference pages for new API
   ([reference index](../reference/README.md)), the concept page for new
   behaviour, a cookbook recipe when it enables a common task, and
   `docs/examples.md` / `README.md` if an example changes.
5. Run the [verification](#verification) commands.
6. Move the finished milestone's row in `_docs/roadmap.md` to the Done section
   with the commit hash, and update its status summary.

## Verification

All of these must be clean before a change is done:

```
gofmt -l .                     # prints nothing
go vet ./...
go test ./...
go mod tidy                    # after adding or removing imports
go run ./examples/todo -png _tmp/todo.png    # pipeline smoke test
```

Also useful:

```
go test -race ./...                                   # the core is single-goroutine; this catches stray goroutines
go test . -run '^$' -bench . -benchmem           # frame-pipeline benchmarks in bench_test.go
go run ./examples/<name> -png _tmp/<name>.png         # render any example you touched and look at it
```

`*.png` and `/_tmp/` are gitignored. Never commit scratch output.

### Documentation snippets must compile

Every Go snippet under `docs/` is checked against the current API. Do the same
for your edits in a throwaway package under the repo root's `_tmp/`. The library
is an `internal` package, so the check must live inside the `go-draw` module
rather than in a separate module with a `replace` directive:

```
# from the repo root
mkdir -p _tmp/docchk/mysnippet
# put the snippet in _tmp/docchk/mysnippet/snippet.go (package snippet) and:
go vet ./_tmp/docchk/... && go test ./_tmp/docchk/...
```

Prefer snippets that are complete declarations. When a snippet shows a fragment
or a signature list rather than compilable code, mark the fence `text`, not
`go`. Where practical, add a quick runtime check (a headless test) for claims
about behaviour, not just for compilation. Grep every identifier you mention:
`grep -rn "func (d \*Div) OnClick" .`.

## Test conventions

- Tests live in `*_test.go` (and `ebitenplatform/*_test.go`), package
  `gpui_test` unless unexported access is required (then package `gpui`, an
  "internal" test; see `_docs/project-structure.md` for which files are which).
- Name tests for behaviour (`TestOverlayCoversEarlierText`), keep each small,
  and prefer table-driven tests for variations.
- Prefer exercising the real pipeline: open a headless window, `Dispatch`
  events, assert on state, pixels (`Image().NRGBAAt`) or the scene. Helpers like
  `openHeadless` and `click` already exist in `dispatch_test.go`; reuse them.
  Details and recipes: [testing](testing.md).
- Layout tests can call `gpui.Layout` on a hand-built `LayoutNode` tree.
- Regression tests for a bug go next to the area they cover; the
  `review_fixes*_test.go` files hold fixes found during review.
- Do not depend on a GPU or a display. Tests that need text must not depend on
  specific glyph pixels; tolerate a machine without fonts (skip rather than
  fail) where the feature is font-dependent.
- Run `go test -race ./...` for anything that could involve goroutines
  (clipboard, snapshot, quit).
- Add a benchmark to `bench_test.go` when you change a hot path
  (rasteriser, layout, shaping).

## Example conventions

Each example is `examples/<name>/main.go`, `package main`, and:

- starts with a header doc comment: what it shows, `Ported from gpui-ce
  crates/gpui/examples/<path>` when applicable, and a `Not ported:` paragraph
  listing what was left out (that comment is the authority and
  [examples](../examples.md) summarises it);
- supports the common flags `-png PATH` (render one frame headlessly with
  `gpui.RenderToPNG` and exit), `-width`, `-height` and `-smoke MS` (quit an
  interactive window after MS milliseconds via `time.AfterFunc(d, cx.Quit)`);
- seeds state that a headless render cannot reach (focus, scroll offset,
  hover) from extra flags, such as `-focus` or `-scroll`;
- keeps the interactive path and the `-png` path on the same root view;
- uses only public API (it is a user of the library, not part of it).

A skeleton that compiles:

```go
// Command hello shows the example skeleton: -png renders headlessly, otherwise
// an ebiten window opens.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go-draw/internal/gpui"
	"go-draw/internal/gpui/ebitenplatform"
)

// View is the root view.
type View struct{}

// Render builds the element tree.
func (View) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().SizeFull().FlexCol().ItemsCenter().JustifyCenter().Bg(gpui.Gray50).
		Child(gpui.NewText("Hello").TextSize(32).TextColor(gpui.Gray900))
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 480, "window width in logical pixels")
	height := flag.Int("height", 320, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, View{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Hello", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return View{} }); err != nil {
			log.Fatal(err)
		}
		if *smoke > 0 {
			time.AfterFunc(time.Duration(*smoke)*time.Millisecond, cx.Quit)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

When you add an example also update `docs/examples.md`, the example list in
`README.md` and `AGENTS.md`, and the count (17 today) wherever it appears.

## Commits

- Small commits with an imperative subject, in the style of `git log --oneline`
  (for example "Add absolute positioning" or "Fix hover text colour in custom
  containers"). One milestone or one logical fix per commit.
- Mention the roadmap ID (`L1`, `S4`, ...) in the commit or PR description.
- Do not commit until you have run the verification commands, and never commit
  `_tmp/`, PNG output or editor files.
- Contract and docs changes may share a commit with the code they describe.

## Where things live

[`_docs/project-structure.md`](../../_docs/project-structure.md) has the full
tree and a "where to add things" guide; the short version:

| You are adding | Put it in |
|---|---|
| A style builder | `styled.go` (the mixin), plus the forwarders in `div.go`, `text_elem.go`, `styled_layout.go` so `*Div` / `*Text` keep their chain; add it to the [style reference](../reference/style-builders.md) |
| A new element | a new file beside `div.go`, following the builder-plus-adapter pattern ([elements](../concepts/elements.md)) |
| A layout feature | `layout.go` (engine) and `style.go` (fields), tests in `layout*_test.go` |
| An input event or dispatch behaviour | `events.go`, `window.go`; the platform side in `ebitenplatform/input.go` |
| Painting | `scene.go` (primitives), `raster.go` (rasteriser), `background.go` / `shadow.go` |
| Text or fonts | `font.go` / `text.go` (the only files allowed to import `x/image` and go-text) |
| A platform backend | a new package implementing `gpui.Platform`; see [platforms](../concepts/platform.md#writing-a-new-backend) |
| A user-facing explanation | the right page under `docs/` and the index in [docs/README.md](../README.md) |

## Reviewing

Look for: a contract edit alongside any signature change; doc comments on every
new exported identifier; tests that run headless; no new imports outside the
allowed files; docs updated and snippets compiling; no `*T` retained across
re-entrant calls; and, for painting changes, a `-png` render attached or
described.
