---
id: tech-operations
title: Operations
kind: tech
status: current
summary: How to build, run, test and render go-draw, build the browser binary, and run the documentation tooling.
read_when: you need a build, run, test or docs command, or you are changing one
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [settings/file.go, justfile, ui/agent_demo.go, cmd/cad/main.go, tools/docs/main.go, tools/icons/icons.py, tools/gui/main.go, packaging/lib.sh, .github/workflows/release.yml, .github/workflows/ci.yml]
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
| Verify the nested `agentmanager` module (not covered by `./...`) | `cd agentmanager && gofmt -l . && go vet ./... && go test ./...` | `just check-agentmanager` |
| Run the desktop app | `go run ./cmd/cad` | `just run` |
| Run with a drawing | `go run ./cmd/cad -open plan.dxf` | `just run -open plan.dxf` |
| Render headlessly | `go run ./cmd/cad -png _tmp/cad.png` | `just png` |
| Build for the browser | `GOOS=js GOARCH=wasm go build -o _tmp/cad.wasm ./cmd/cad` | `just wasm` |
| Run real-window scenarios under Xvfb (`just gui-run tools/gui/scenarios/line.gui`; `-video` records) | `go run ./tools/gui scenario.gui…` | `just gui-run` |
| Run every real-window scenario; fails on any assertion; output in `_tmp/gui/out` | `GODRAW_GUI_TEST=1 go test -count=1 ./tools/gui` | `just gui-test` |
| One screenshot of the real window (`just gui-shot -demo`) | `go run ./tools/gui` on a one-step scenario | `just gui-shot` |
| Build the application icon (`.icns`, `.ico`, window PNGs) from `assets/appicon/*.svg` | `uv run tools/icons/appicon.py build` | `just appicon` |
| Review sheet of the application icon at every size beside UI icons | `uv run tools/icons/appicon.py sheet` | `just appicon-sheet` |
| Assemble the macOS `.app`, the Windows exe and zip, the Linux tar.gz in `_tmp/dist/` | `packaging/{macos,windows,linux}/bundle.sh` | `just bundle-mac`, `just bundle-win`, `just bundle-linux` |
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
| `-save-on-exit PATH` | Write the drawing to this DXF path when the window closes or on SIGINT or SIGTERM; the real-window tests assert on that file |
| `-agent NAME` | Select the agent harness (`opencode`, `claude` or `codex`) and open the [agent panel](../features/agent-panel.md); Start launches it |
| `-agent-cmd PATH` | Run this executable for the selected harness instead of the one on `PATH` |
| `-agent-demo` | Open the agent panel on a canned transcript; for `-png` renders |

A render that shows an active command with its snap marker:

```
go run ./cmd/cad -png _tmp/snap.png -demo -script "LINE 25,25" -hover 195.6,25.4
```

## Settings file

The application keeps its [settings](../features/settings.md) in `settings.toml`, in `go-draw/` under the
OS user config folder: `~/Library/Application Support` on macOS, `%AppData%` on Windows,
`$XDG_CONFIG_HOME` or `~/.config` on Linux. `GODRAW_CONFIG_DIR` replaces that whole directory (the
file is then `$GODRAW_CONFIG_DIR/settings.toml`); tests and renders that must not touch the real file
set it to a temporary directory. The browser build has no file and keeps them in memory.

## Environment notes

- The interactive window needs a display and OpenGL. Agent shells have neither, and ebiten fails at
  startup there, so use the real-window harness `tools/gui`: it starts Xvfb, runs `cmd/cad` with
  `LIBGL_ALWAYS_SOFTWARE=1` (Mesa llvmpipe), drives it with xdotool and screenshots it. It needs
  `apt-get install -y xvfb xdotool x11-utils imagemagick ffmpeg` (`ffmpeg` only for `-video`). The
  harness does not exercise HiDPI scales, the clipboard, IME or window resizing; say which
  interactive behaviour was not run.
- Fonts come from the operating system (the library searches the standard font directories). Where
  there are none, in a minimal container or the browser (`js/wasm`, always), the library uses its
  bundled Go Regular and Go Bold faces (Latin, Greek and Cyrillic only), so text still renders.
  `GPUI_NO_SYSTEM_FONTS=1` forces that on any machine, for renders that must not depend on the
  installed fonts. The bundled fonts add about 0.3 MB to the wasm binary.
- `TestTextInputCaretUsesFullShapedLineAtBothScales` samples one caret pixel, which depends on the
  font's fractional advances; it runs on the bundled font (`GPUI_NO_SYSTEM_FONTS`) and so does not
  vary with the installed fonts.

## Releases

`.github/workflows/release.yml` runs on a pushed `v*` tag and on manual dispatch. It builds the
macOS universal `.app` as `go-draw-<v>-macos-universal.zip` and `.dmg`, the Windows `amd64` and
`arm64` zips with an Inno Setup `-setup.exe` each, and the Linux `go-draw-<v>-linux-amd64.tar.gz`; a
`publish` job on a tag attaches them with `SHA256SUMS.txt` to a GitHub Release. The signing secrets
are optional (see `packaging/README.md`); without them the macOS app is ad-hoc signed. The scripts
share `packaging/lib.sh`, so a `just bundle-*` recipe builds what the workflow releases.

`.github/workflows/ci.yml` runs on pushes to `main` and on pull requests with three jobs: `linux`
(gofmt, vet, tests, the `agentmanager` module with `-race`, docs lint, the wasm build), `gui` (the
real-window scenarios under Xvfb) and `cross` (a `CGO_ENABLED=0` build and the pure-package tests on
Windows and macOS).

## Related docs

- [Testing](testing.md) — what the test suites cover
- [Authoring](../_meta/authoring.md) — when to run the docs commands
