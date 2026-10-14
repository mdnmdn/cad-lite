---
id: feat-updates
title: Updates
kind: feature
status: current
summary: How go-draw finds a newer GitHub release, shows it in the settings panel, and on one press downloads, verifies, installs and restarts into it.
read_when: you are changing the update check, the Update button, the release asset names or checksums, or how the installation is replaced
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [updater/check.go, updater/assets.go, updater/download.go, updater/install.go, updater/extract.go, updater/version.go, ui/update.go, ui/update_view.go, ui/settings_view.go, cmd/cad/main.go, .github/workflows/release.yml, packaging/lib.sh]
depends_on: [feat-settings, tech-operations, tech-architecture]
review_cycle: monthly
---

# Updates

## Purpose

go-draw keeps itself current from the GitHub releases of `mdnmdn/go-draw`, on a single stream (the
latest published release; there are no channels). It looks for a new release in the background and
on demand. When one exists the user presses **Update** once, and the download, the integrity check,
the installation and the restart follow without further action.

## Behaviour

- **Version.** The build stamps its version (`-X main.version=1.2.3`, tag without `v`); the
  settings panel shows it under *About and updates*, `go-draw -version` prints it, and the macOS
  `Info.plist` and the Windows version resource carry it. An unstamped build is `dev`; CI builds
  are `0.0.0-ci+<shortsha>`. A version with build metadata or that is not semver never updates itself
  (no automatic check, no Update button) but the *Check for updates* button still reports the latest release.
- **Checking.** One check runs about 10 seconds after start and then every 6 hours while
  `update.auto_check` is on. The *Check for updates* button runs one at once, ignoring the cached
  answer. Requests are conditional (`ETag` / `If-None-Match`), so an unchanged release costs a 304.
  Drafts and pre-releases are skipped. Nothing runs in `-png` renders, in tests, in the browser
  build, or when `GODRAW_NO_UPDATE=1`.
- **Notice.** While a release waits, the toolbar's gear button carries a dot and its tooltip names the
  version; the panel shows the status line, a release-notes excerpt and the Update button.
  Nothing modal interrupts the user.
- **Update.** The button downloads the archive for this platform, checks its sha256 against
  `SHA256SUMS.txt` of the same release (a missing list, a missing entry or a mismatch refuses to install),
  replaces the installation and restarts. The status line shows the percentage, then "installing".
- **Restart.** Without edits in the drawing the application quits and starts the new version at once.
  The application has no unsaved-changes flag, so a drawing whose undo history is not empty is taken as
  possibly unsaved: a dialog offers *Restart and update* or *Later* (the new version then starts at the next launch).
  Settings are saved and the agent closed before the new process starts, and the new process gets the
  arguments of the old one.
- **Installing.** Windows: the running `go-draw.exe` is renamed to `go-draw.exe.old`, the new one
  moved in, and the `.old` file deleted by the next start. macOS: the whole `.app` is extracted beside the
  old one, the old bundle is renamed `.app.old`, the new one renamed into place, and the old one deleted
  only after that worked (restored if it did not); a quarantine attribute on the new copy, if it has
  one, is cleared; the restart is `open -n`. Linux and a bare macOS binary: the binary is replaced the
  same way. The installation is found with `os.Executable` and symlinks resolved.
- **Assets.** The updater installs from `go-draw-<v>-windows-<amd64|arm64>.zip`,
  `go-draw-<v>-macos-universal.zip` and `go-draw-<v>-linux-amd64.tar.gz`; the `-setup.exe` and `.dmg`
  are for people installing by hand and are never chosen by the updater.

## Contract

Settings keys `update.auto_check` (boolean, default true) and `update.last_check` (timestamp, set by
the app); both are in [Settings](settings.md). Environment `GODRAW_NO_UPDATE=1` turns the machinery off.
Package `updater`: `Client` (`NewClient`, `Check`, `CheckForce`, `Download`, `Update`), `Release`, `Asset`,
`AssetName`, `ParseVersion`, `Updatable`, `Install`, `Extract`, `Relaunch`, `CleanupOld`, `OpenURL`, `Supported`,
`Disabled`. Workspace: `SetVersion`, `Version`, `EnableUpdates`, `SetUpdateClient`, `UpdatesEnabled`,
`UpdateAvailable`, `RelaunchIfPending`.

## Implementation

`updater` is stdlib only and never imports the UI. `Client.Check` reads
`/repos/mdnmdn/go-draw/releases/latest` (base URL, repository, platform and install path are fields,
which is how the tests drive it) and compares versions with `ParseVersion` and `Version.Compare`.
`Client.Update` calls `Download` (checksum first, then the archive into a temporary folder, hashing
while it streams), `Install` (extract into a staging folder beside the target, then `swap`) and
fills a `Plan` that `Relaunch` uses. `Extract` rejects paths and symlinks that leave the folder and
caps the unpacked size. Plain http is refused except to loopback.

`ui/update.go` keeps the state beside the workspace (`upd`). The worker goroutines write one
mutex-guarded result and two atomic progress counters; a 250 ms `Window.After` poll, armed only
while work runs, moves the result onto the UI goroutine, the same shape as the agent panel. The
6-hour timer is a `Window.After` armed by `startUpdates`, which `mountSettings` calls from every
`Render`. `ui/update_view.go` draws the section, the gear badge and the restart dialog.
`cmd/cad/main.go` calls `SetVersion`, `EnableUpdates` (not with `-png`) and defers `RelaunchIfPending`
before the agent's `CloseAgent` defer, so it runs after both the settings save and the agent close.

## Failure

- No network, a rate limit, a malformed answer: the status line shows the error; the next automatic check retries.
- A release without an archive for this platform shows an error and *Open release page*.
- A checksum problem, a failed download, or an installation folder that cannot be written (for
  example `/Applications` for a standard user) leaves the installed version untouched, shows the
  error and offers *Open release page*.
- A failed relaunch is logged; the new version is installed and starts at the next launch.

## Related docs

- [Settings](settings.md) — the panel and the `update` table
- [Operations](../tech/operations.md) — releases, ldflags, asset names and checksums
- [Architecture](../tech/architecture.md) — where `updater` sits

## Next steps

- Sign updates (checksums come from the same release, so they catch corruption, not a hijacked release).
- Delta updates; roll back to the `.old` copy; a real unsaved-changes flag.
