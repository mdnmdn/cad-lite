# Packaging

How Cad-lite is built into downloadable bundles. CI runs the same scripts as the `just` recipes, so
a bundle built here matches a released one apart from signing.

| Platform | Script | Output (in `_tmp/dist/`) | `just` |
|---|---|---|---|
| macOS | `macos/bundle.sh`, then `macos/package.sh` (CI only) | `macos/Cad-lite.app`, `cad-lite-<v>-macos-universal.{zip,dmg}` | `just bundle-mac` |
| Windows | `windows/bundle.sh`, `windows/cad-lite.iss` | `windows/cad-lite-<v>-windows-amd64.zip`, `...-setup.exe` | `just bundle-win` |
| Linux | `linux/bundle.sh` | `linux/cad-lite-<v>-linux-amd64.tar.gz` | `just bundle-linux` |

Version: `$VERSION`, else the tag (`v1.2.3` gives `1.2.3`), else `git describe`. A non-semver value
(an untagged build) stamps `0.0.0` into the numeric fields. `-X main.version` is added when
`cmd/cad/main.go` declares a `version` variable, which it does; the in-app updater compares it with
the release tag. CI builds stamp `0.0.0-ci+<shortsha>`.

## Feedback configuration

Desktop bundle scripts accept optional `FEEDBACK_URL` and `FEEDBACK_KEY`. CI and release builds
read repository variable `FEEDBACK_URL` and secret `FEEDBACK_KEY`. Both must be present to enable
submission; missing values leave the build successful and the feedback dialog unconfigured.
`resolve_feedback` base64 encodes the values for `-X main.feedbackURLBase64` and
`-X main.feedbackKeyBase64`, avoiding shell-sensitive linker values. Base64 is transport encoding,
not encryption: desktop credentials are extractable and must be scoped to intake. They never enter
browser build flags. See [Feedback](../_docs/features/feedback.md) for the outbox and protocol.

## Pure-Go cross builds

macOS and Windows need no cgo: ebiten uses purego on both, so `CGO_ENABLED=0` cross-compiles all
release targets (`darwin/{arm64,amd64}`, `windows/amd64`) from any host. Windows bundles
default to `amd64`; the release workflow publishes no Windows or Linux ARM artifacts. The macOS bundle
holds a universal binary (`lipo`, so it needs a Mac; elsewhere run `ARCHS=arm64 packaging/macos/bundle.sh`).
Linux releases use an x64 runner and `GOARCH=amd64`. Linux needs cgo and the X11/GL headers,
so it is built natively:
`libc6-dev libgl1-mesa-dev libx11-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev pkg-config`
(`librsvg2-bin` renders the icon from `assets/appicon/appicon.svg`; without it the committed
`window-256.png` is used).

## Windows resources

`windows/winres.json` holds the icon (`assets/appicon/AppIcon.ico`), the manifest (per-monitor-v2
DPI, long paths) and version info. `bundle.sh` runs `go-winres make` (install with
`go install github.com/tc-hib/go-winres@v0.3.3`), which writes `cmd/cad/rsrc_windows_<arch>.syso`;
Go links it automatically for that GOARCH. The `.syso` files are generated and deleted when the
script exits; do not commit them. The exe is linked with `-H=windowsgui`, so it has no console.

The installer (`cad-lite.iss`) is per-user and needs no admin (`/ALLUSERS` on the command line
installs per machine into Program Files). It installs `cad-lite.exe` into `{autopf}\Cad-lite`, with
an optional desktop icon, and registers the shell integration: the `CadLite.Drawing` file type,
Cad-lite in "Open with" for `.dxf` and `.dwg`, the default program for `.dxf` (task, ticked) and `.dwg`
(task, unticked), and an "Open in Cad-lite" right-click entry (task, ticked). The registry keys, the
uninstall rules and the `cad-lite.exe "<path>"` contract are in `_docs/tech/operations.md`
(Windows installer). ISCC runs only on Windows, so a `.iss` change is checked by the `release`
workflow or by hand. Build by hand: `iscc /DMyAppVersion=1.2.3 /DMyArch=amd64 packaging\windows\cad-lite.iss`.

## macOS signing and notarization

`macos/package.sh` picks a mode from the repository secrets:

| Secrets set | Result |
|---|---|
| none | ad-hoc signed; users must right-click Open or "Open Anyway" once |
| `MACOS_CERT_P12` (base64 .p12), `MACOS_CERT_PASSWORD`, `MACOS_SIGN_IDENTITY` | Developer ID signed with hardened runtime, not notarized |
| the above plus `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_PASSWORD` (app-specific) | signed, notarized and stapled (app and dmg) |

Windows Authenticode signing is not wired; unsigned exes trigger SmartScreen. Add a signing step
before the Inno Setup step when a certificate exists.

## Releasing

Push a tag `v*` (`git tag v0.2.0 && git push origin v0.2.0`). `release.yml` builds all platforms,
and the `publish` job attaches everything plus `SHA256SUMS.txt` (the in-app updater verifies its
download against it and installs from the `.zip` and `.tar.gz`, never the `-setup.exe` or `.dmg`) to a GitHub Release with generated
notes; a tag containing `-` is a prerelease.

Or release from the Actions tab (`release` → Run workflow) with `version` set to `0.2.0` (no
leading `v`). The `macos`, `windows` and `linux` boxes select which platforms to build; all
are selected by default and at least one must remain selected. The run checks the selection,
that the version is well formed and its tag is new, builds the selected platforms, then creates the
tag `v0.2.0` on that commit and publishes their packages. Publishing requires all selected builds
to succeed; unselected builds are skipped. Tick `draft` to review it first (the tag is
created when you publish the draft). With `version` empty the run only builds and keeps the
selected packages as run artifacts, even when the chosen ref is a tag.

## Local checks

```
go install github.com/tc-hib/go-winres@v0.3.3          # once, then put GOBIN on PATH
ARCHS=arm64 just bundle-mac                             # layout check off macOS
just bundle-win
just bundle-linux
```
