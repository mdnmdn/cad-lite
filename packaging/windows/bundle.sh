#!/usr/bin/env bash
# Build cad-lite.exe with the icon and version resources embedded; see packaging/README.md.
#   ARCHS=amd64          GOARCH values (release default: amd64)
#   VERSION=1.2.3        defaults to the tag / git describe
#   OUT=_tmp/dist/windows  output; per arch: $OUT/cad-lite-windows-<arch>/cad-lite.exe and a .zip
# Needs go-winres on PATH (go install github.com/tc-hib/go-winres@v0.3.3). Runs on any OS.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/packaging/lib.sh"
resolve_version
ARCHS="${ARCHS:-amd64}"
OUT="${OUT:-$root/_tmp/dist/windows}"
mkdir -p "$OUT"

icon="$root/assets/appicon/AppIcon.ico"
have_res=1
if [ ! -f "$icon" ]; then echo "warning: $icon missing; exe gets no icon/version resource" >&2; have_res=0; fi
command -v go-winres >/dev/null || { echo "warning: go-winres not on PATH; no resources" >&2; have_res=0; }

syso=()
cleanup() { [ "${#syso[@]}" -eq 0 ] || rm -f "${syso[@]}"; }
trap cleanup EXIT

for arch in $ARCHS; do
  dir="$OUT/cad-lite-windows-$arch"
  rm -rf "$dir"; mkdir -p "$dir"
  if [ "$have_res" = 1 ]; then
    # go-winres names the files rsrc_windows_<arch>.syso next to the package; Go links them
    # for the matching GOARCH only. They are generated, gitignored, and removed on exit.
    (cd "$root/packaging/windows" && go-winres make --in winres.json --arch "$arch" \
      --file-version "$NUMERIC" --product-version "$VERSION" --out "$root/cmd/cad/rsrc")
    syso+=("$root/cmd/cad/rsrc_windows_$arch.syso")
  fi
  (cd "$root" && GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-H=windowsgui -s -w $LDFLAGS_VERSION" -o "$dir/cad-lite.exe" ./cmd/cad)
  rm -f "$root/cmd/cad/rsrc_windows_$arch.syso"
  # Compress-Archive (Windows PowerShell 5) writes backslashes into entry names, so it is the last
  # resort; the GitHub Windows runner has 7-Zip but no zip.
  sevenzip="$(command -v 7z || true)"
  if [ -z "$sevenzip" ] && [ -x "/c/Program Files/7-Zip/7z.exe" ]; then sevenzip="/c/Program Files/7-Zip/7z.exe"; fi
  (cd "$OUT" && rm -f "cad-lite-$VERSION-windows-$arch.zip" &&
   if command -v zip >/dev/null; then zip -qr "cad-lite-$VERSION-windows-$arch.zip" "cad-lite-windows-$arch"
   elif [ -n "$sevenzip" ]; then "$sevenzip" a -tzip -bd -bso0 "cad-lite-$VERSION-windows-$arch.zip" "cad-lite-windows-$arch"
   else powershell -NoProfile -Command "Compress-Archive -Force cad-lite-windows-$arch cad-lite-$VERSION-windows-$arch.zip"; fi)
  echo "$dir/cad-lite.exe (version $VERSION)"
done
