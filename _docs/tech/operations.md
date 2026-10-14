---
id: tech-operations
title: Operations
kind: tech
status: current
summary: How to build, run, test and render go-draw, build the browser binary, and run the documentation tooling.
read_when: you need a build, run, test or docs command, or you are changing one
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [justfile, cmd/cad/main.go, tools/docs/main.go, tools/icons/icons.py]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# Operations

## What it is

Everything runs with the Go toolchain named in `go.mod`, from the repository root. The `justfile`
wraps the common commands; each recipe is a plain command, so `just` itself is optional. Scratch
output (renders, binaries, experiments) goes in `_tmp/`, which is gitignored and skipped by `./...`.

## Commands

| Task | Command | `just` recipe |
|---|---|---|
| Verify a change | `gofmt -l .` (must print nothing), `go vet ./...`, `go test ./...` | `just check` |
| Run the desktop app | `go run ./cmd/cad` | `just run` |
| Run with a drawing | `go run ./cmd/cad -open plan.dxf` | `just run -open plan.dxf` |
| Render headlessly | `go run ./cmd/cad -png _tmp/cad.png` | `just png` |
| Build for the browser | `GOOS=js GOARCH=wasm go build -o _tmp/cad.wasm ./cmd/cad` | `just wasm` |
| Lint the docs | `go run ./tools/docs lint` | `just docs-lint` |
| Docs a change touches | `go run ./tools/docs touched [paths…]` | `just docs-touched` |
| Docs whose code moved | `go run ./tools/docs drift` | `just docs-drift` |
| Regenerate index and code map | `go run ./tools/docs index` | `just docs-index` |
| Check the icon registry and SVGs; fails if `names_gen.go` is stale | `uv run tools/icons/icons.py check` | `just icons-check` |
| Regenerate `assets/icons/names_gen.go` | `uv run tools/icons/icons.py codegen` | `just icons-gen` |
| Review sheet for some icons | `uv run tools/icons/icons.py sheet draw-line` | `just icons-sheet draw-line` (or `--category draw`) |
| Copy Lucide icons in as `adopted:` rows | `uv run tools/icons/icons.py adopt [NAME…]` | `just icons-adopt` |
| Compare competing takes in `assets/icons/variants/` | `uv run tools/icons/icons.py variants` | `just icons-variants --category draw` |
| One contact sheet per category | `uv run tools/icons/icons.py audit` | `just icons-audit` |
| List icons that look alike | `uv run tools/icons/icons.py dupes` | `just icons-dupes` |

The icon commands need [`uv`](https://docs.astral.sh/uv/), which installs the script's Python
dependencies on first run; sheets are rendered by resvg, not by the application's rasterizer.
`icons-adopt` downloads the pinned `lucide-static` package from `registry.npmjs.org` into `_tmp/`
(or reads the `icons/` directory named by `LUCIDE_DIR`), so it needs npm registry access. The
`preview/` and `variants/` directories under `assets/icons/` are gitignored. The icon workflow is
in `.claude/skills/go-draw-icons/SKILL.md`.

Run `go mod tidy` after adding an import.

## `cmd/cad` flags

| Flag | Meaning |
|---|---|
| `-open PATH` | Open a DXF file at startup |
| `-script TEXT` | Run a command script after opening (AutoCAD script rules) |
| `-demo` | Draw the demo plate at startup |
| `-png PATH` | Render one frame headlessly and exit; draws the demo plate unless `-open` or `-script` is given |
| `-hover X,Y` | With `-png`: move the pointer to the model point first, so snaps and previews show |
| `-width`, `-height` | Window size in logical pixels (default 1280×800) |
| `-scale` | Device scale for `-png` |
| `-smoke MS` | Quit the window after MS milliseconds |

A render that shows an active command with its snap marker:

```
go run ./cmd/cad -png _tmp/snap.png -demo -script "LINE 25,25" -hover 195.6,25.4
```

## Environment notes

- The interactive window needs a display and a GPU-capable ebiten backend. Agent shells usually have
  neither, and ebiten fails at startup there; verify with the headless tests and `-png` renders, and
  say which interactive behaviour was not run.
- Fonts come from the operating system (the library searches the standard font directories). A
  machine without them, a browser among them, renders no text.
- One `internal/gpui` test, `TestTextInputCaretUsesFullShapedLineAtBothScales`, depends on the
  installed fonts and fails on Linux font sets; it is tracked in the backlog.

## Related docs

- [Testing](testing.md) — what the test suites cover
- [Authoring](../_meta/authoring.md) — when to run the docs commands
