---
id: tech-operations
title: Operations
kind: tech
status: current
summary: How to build, run, test and render Cad-lite, build the browser binary, and run the documentation tooling.
read_when: you need a build, run, test or docs command, or you are changing one
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [settings/file.go, updater/assets.go, justfile, ui/agent_demo.go, cmd/cad/main.go, tools/docs/main.go, tools/icons/icons.py, tools/gui/main.go, packaging/lib.sh, packaging/windows/cad-lite.iss, .github/workflows/release.yml, .github/workflows/ci.yml]
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
| Run with a drawing (DXF or DWG) | `go run ./cmd/cad -open plan.dxf` | `just run -open plan.dxf` |
| Render headlessly | `go run ./cmd/cad -png _tmp/cad.png` | `just png` |
| Fuzz the DWG reader (seeded with the fixtures; a stall of tens of seconds is the fuzzer minimizing an input) | `go test ./fileio/dwg -run XXX -fuzz FuzzRead -fuzztime 60s -fuzzminimizetime 3s` | |
| Compare every DWG sample entity with ACadSharp's DXF export | `DWG_PARITY_DIR=…/ACadSharp/samples go test ./fileio/dwg -run TestParityWithDXF` | |
| Build for the browser | `GOOS=js GOARCH=wasm go build -o _tmp/cad.wasm ./cmd/cad` | `just wasm` |
| Run real-window scenarios under Xvfb (`just gui-run tools/gui/scenarios/line.gui`; `-video` records) | `go run ./tools/gui scenario.gui…` | `just gui-run` |
| Run every real-window scenario; fails on any assertion; output in `_tmp/gui/out` | `CADLITE_GUI_TEST=1 go test -count=1 ./tools/gui` | `just gui-test` |
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
in `.claude/skills/cad-lite-icons/SKILL.md`.

Run `go mod tidy` after adding an import.

## `cmd/cad` flags

| Flag | Meaning |
|---|---|
| `-open PATH` | Open a DXF or DWG file at startup; a bad file stops the program |
| `FILE ...` (after the flags) | Open the first readable drawing at startup as a dropped file is; problems go to the command history. Not together with `-open` |
| `-script TEXT` | Run a command script after opening (AutoCAD script rules) |
| `-demo` | Draw the demo plate at startup |
| `-png PATH` | Render one frame headlessly and exit; draws the demo plate unless `-open`, a file argument or `-script` is given |
| `-hover X,Y` | With `-png`: move the pointer to the model point first, so snaps and previews show |
| `-width`, `-height` | Window size in logical pixels; without them the size saved in the settings file, else 1280×800 |
| `-version` | Print the version (`dev` unless the build sets `-X main.version=...`) and exit |
| `-scale` | Device scale for `-png` |
| `-smoke MS` | Quit the window after MS milliseconds |
| `-save-on-exit PATH` | Write the drawing to this DXF path when the window closes or on SIGINT or SIGTERM; the real-window tests assert on that file |
| `-agent NAME` | Select the agent harness (`opencode`, `claude` or `codex`) and open the [agent panel](../features/agent-panel.md); Start launches it |
| `-agent-cmd PATH` | Run this executable for the selected harness instead of the one on `PATH` |
| `-agent-demo` | Open the agent panel on a canned transcript; for `-png` renders |

A render in Italian takes the language from the environment (`-png` does not read the settings file):

```
LANG=it_IT.UTF-8 go run ./cmd/cad -png _tmp/it.png
```

`CADLITE_THEME=dark|light|classic|high-contrast` picks the theme of a `-png` render the same way.

A render that shows an active command with its snap marker:

```
go run ./cmd/cad -png _tmp/snap.png -demo -script "LINE 25,25" -hover 195.6,25.4
```

## Settings file

The application keeps its [settings](../features/settings.md) in `settings.toml`, in `cad-lite/` under the
OS user config folder: `~/Library/Application Support` on macOS, `%AppData%` on Windows,
`$XDG_CONFIG_HOME` or `~/.config` on Linux. `CADLITE_CONFIG_DIR` replaces that whole directory (the
file is then `$CADLITE_CONFIG_DIR/settings.toml`); tests and renders that must not touch the real file
set it to a temporary directory. On the first start after the rename from go-draw, when
`cad-lite/settings.toml` does not exist and `go-draw/settings.toml` does, the old file is copied over (and
left in place).

Every `CADLITE_*` variable is read through `appenv`, which still accepts the old `GODRAW_*` name when the new one is unset. `cmd/cad` loads the file at startup unless `-png` is given (a failure is
logged and the defaults apply) and saves it, with the window size, when the window closes. The browser build has no file and keeps them in memory.

## Environment notes

- The interactive window needs a display and OpenGL. Agent shells have neither, and ebiten fails at
  startup there, so use the real-window harness `tools/gui`: it starts Xvfb, runs `cmd/cad` with
  `LIBGL_ALWAYS_SOFTWARE=1` (Mesa llvmpipe), drives it with xdotool and screenshots it. It needs
  `apt-get install -y xvfb xdotool x11-utils imagemagick ffmpeg` (`ffmpeg` only for `-video`). The
  harness does not exercise HiDPI scales, the clipboard, IME or window resizing; say which
  interactive behaviour was not run.
- The Open and Save dialogs (`filedialog`) run helper programs and need no cgo: `osascript` on macOS and
  PowerShell on Windows ship with the system; on Linux and the BSDs they need `zenity` or `kdialog` and a
  display, and without them the buttons fall back to the command line. Agent shells have neither, so the
  dialogs are tested through pure command builders and a fake dialog; a real dialog is never opened by
  tests.
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
macOS universal `.app` as `cad-lite-<v>-macos-universal.zip` and `.dmg`, the Windows `amd64` and
`arm64` zips with an Inno Setup `-setup.exe` each, and the Linux `cad-lite-<v>-linux-amd64.tar.gz`; a
`publish` job attaches them with `SHA256SUMS.txt` to a GitHub Release, on a tag or on a manual run that
names a version. The signing secrets
are optional (see `packaging/README.md`); without them the macOS app is ad-hoc signed. The scripts
share `packaging/lib.sh`, so a `just bundle-*` recipe builds what the workflow releases.

`.github/workflows/ci.yml` runs on pushes to `main` and on pull requests with three jobs: `linux`
(gofmt, vet, tests, the `agentmanager` module with `-race`, docs lint, the wasm build), `gui` (the
real-window scenarios under Xvfb) and `cross` (a `CGO_ENABLED=0` build and the pure-package tests on
Windows and macOS).

### Release steps and update assets

1. Either push a tag `vX.Y.Z`, or run the `release` workflow by hand (Actions → release → Run
   workflow) with `version` set to `X.Y.Z`: a `check` job first refuses a malformed version or a tag
   that exists, and `publish` creates the tag `vX.Y.Z` on the commit it built. The `draft` box keeps the
   release a draft; GitHub then creates the tag when the draft is published. A manual run with an empty
   `version` only builds the packages and keeps them as run artifacts. A version with `-` is a
   pre-release and is never offered by the [updater](../features/updates.md).
2. `packaging/lib.sh` turns the tag into the version and every bundle script links it with
   `-ldflags "-X main.version=X.Y.Z"` (also into `Info.plist` and the Windows version resource).
   CI stamps `0.0.0-ci+<shortsha>` so its binaries show where they came from.
3. The release carries, per platform: `cad-lite-X.Y.Z-windows-<amd64|arm64>.zip` and
   `cad-lite-X.Y.Z-windows-<arch>-setup.exe`, `cad-lite-X.Y.Z-macos-universal.zip` and `.dmg`,
   `cad-lite-X.Y.Z-linux-amd64.tar.gz`, and `SHA256SUMS.txt` (sha256sum format, every file). The
   updater installs from the zips and the tar.gz and verifies them against `SHA256SUMS.txt`; renaming
   an asset means changing `updater.AssetName` too.
4. `CADLITE_NO_UPDATE=1` turns the in-app update checks off.

### Windows installer

`packaging/windows/cad-lite.iss` (Inno Setup 6) installs `cad-lite.exe` into `{autopf}\Cad-lite`:
`%LOCALAPPDATA%\Programs\Cad-lite` per user (the default, `PrivilegesRequired=lowest`), Program Files
with `/ALLUSERS`. Every registry entry uses the root `HKA`, so it lands in `HKCU\Software\Classes` per
user and `HKLM\Software\Classes` per machine. Every verb runs `"{app}\cad-lite.exe" "%1"`, so the
contract with `cmd/cad` is **`cad-lite.exe "<path>"` opens that drawing**, the path being the first
argument.

| Key (under `Software\Classes` unless named) | Written | Removed on uninstall |
|---|---|---|
| `CadLite.Drawing` (`CAD drawing`, `DefaultIcon`, `shell\open\command`) | always | the key |
| `.dxf\OpenWithProgids`, `.dwg\OpenWithProgids`: value `CadLite.Drawing` | always | the value, the key when empty |
| `Applications\cad-lite.exe` (`FriendlyAppName`, `DefaultIcon`, `SupportedTypes` `.dxf` `.dwg`, `shell\open\command`) | always | the key |
| `Software\Synesthesia\Cad-lite\Capabilities` and the `Cad-lite` value of `Software\RegisteredApplications` (Settings → Default apps) | always | the key and the value |
| `.dxf` default value `CadLite.Drawing` | task `dxfassoc`, ticked | the value, only while it is still `CadLite.Drawing` |
| `.dwg` default value `CadLite.Drawing` | task `dwgassoc`, unticked | as `.dxf` |
| `SystemFileAssociations\.dxf\shell\CadLite.Open` and `...\.dwg\...` ("Open in Cad-lite", icon, command) | task `contextmenu`, ticked | the key |

`ChangesAssociations=yes` makes Setup and the uninstaller call `SHChangeNotify`. Re-running the
installer with a task unticked removes what that task wrote (a default only while it is still ours).
The `cad-lite.dxf` ProgID of earlier installers (`-open "%1"`) is deleted from `HKCU` on install. The
uninstaller also deletes `cad-lite.exe.old`. A default the user picked in Explorer (`UserChoice`)
outranks the installer's: Windows lets only the user change it, so on such a machine Cad-lite is
offered in "Open with" and Default apps instead. On Windows 11 "Open in Cad-lite" is under "Show more
options".

The paths stay valid across updates: the in-app updater replaces `{app}\cad-lite.exe` in place
(`updater.Install` renames it to `.old` and moves the new file to the same name), so no key needs
rewriting. A per-machine install in Program Files is not writable by the user, so the updater reports
the folder as not writable there; per-user installs update.

## Related docs

- [Testing](testing.md) — what the test suites cover
- [Authoring](../_meta/authoring.md) — when to run the docs commands
