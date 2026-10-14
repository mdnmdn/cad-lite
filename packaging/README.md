# Packaging

How go-draw is built into downloadable bundles. CI runs the same scripts as the `just` recipes, so
a bundle built here matches a released one apart from signing.

| Platform | Script | Output (in `_tmp/dist/`) | `just` |
|---|---|---|---|
| macOS | `macos/bundle.sh`, then `macos/package.sh` (CI only) | `macos/go-draw.app`, `go-draw-<v>-macos-universal.{zip,dmg}` | `just bundle-mac` |
| Windows | `windows/bundle.sh`, `windows/go-draw.iss` | `windows/go-draw-<v>-windows-<arch>.zip`, `...-setup.exe` | `just bundle-win` |
| Linux | `linux/bundle.sh` | `linux/go-draw-<v>-linux-<arch>.tar.gz` | `just bundle-linux` |

Version: `$VERSION`, else the tag (`v1.2.3` gives `1.2.3`), else `git describe`. A non-semver value
(an untagged build) stamps `0.0.0` into the numeric fields. `-X main.version` is added only when
`cmd/cad/main.go` declares a `version` variable; it does not today.

## Pure-Go cross builds

macOS and Windows need no cgo: ebiten uses purego on both, so `CGO_ENABLED=0` cross-compiles all
four targets (`darwin/{arm64,amd64}`, `windows/{amd64,arm64}`) from any host. The macOS bundle
holds a universal binary (`lipo`, so it needs a Mac; elsewhere run `ARCHS=arm64 packaging/macos/bundle.sh`).
Linux needs cgo and the X11/GL headers, so it is built natively:
`libc6-dev libgl1-mesa-dev libx11-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev pkg-config`
(`librsvg2-bin` renders the icon from `assets/appicon/appicon.svg`; without it the committed
`window-256.png` is used).

## Windows resources

`windows/winres.json` holds the icon (`assets/appicon/AppIcon.ico`), the manifest (per-monitor-v2
DPI, long paths) and version info. `bundle.sh` runs `go-winres make` (install with
`go install github.com/tc-hib/go-winres@v0.3.3`), which writes `cmd/cad/rsrc_windows_<arch>.syso`;
Go links it automatically for that GOARCH. The `.syso` files are generated and deleted when the
script exits; do not commit them. The exe is linked with `-H=windowsgui`, so it has no console.

The installer (`go-draw.iss`) is per-user, no admin, with optional desktop icon and `.dxf`
association. Build by hand: `iscc /DMyAppVersion=1.2.3 /DMyArch=amd64 packaging\windows\go-draw.iss`.

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
and the `publish` job attaches everything plus `SHA256SUMS.txt` to a GitHub Release with generated
notes; a tag containing `-` is a prerelease. Run from the Actions tab (`workflow_dispatch`) to
build artifacts without a tag; on a tag ref a manual run creates a draft release.

## Local checks

```
go install github.com/tc-hib/go-winres@v0.3.3          # once, then put GOBIN on PATH
ARCHS=arm64 just bundle-mac                             # layout check off macOS
just bundle-win
just bundle-linux
```
