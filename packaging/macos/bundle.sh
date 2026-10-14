#!/usr/bin/env bash
# Assemble Cad-lite.app (unsigned) from a pure-Go cross build; see packaging/README.md.
#   ARCHS="arm64 amd64"  architectures; two or more are merged with lipo (macOS only)
#   VERSION=1.2.3        defaults to the tag / git describe
#   OUT=_tmp/dist/macos  output directory; the bundle lands at $OUT/Cad-lite.app
# Signing, zip and dmg happen in the release workflow, never here.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/packaging/lib.sh"
resolve_version
ARCHS="${ARCHS:-arm64 amd64}"
OUT="${OUT:-$root/_tmp/dist/macos}"
app="$OUT/Cad-lite.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$OUT/build"

bins=()
for arch in $ARCHS; do
  bin="$OUT/build/cad-lite-$arch"
  (cd "$root" && GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w $LDFLAGS_VERSION" -o "$bin" ./cmd/cad)
  bins+=("$bin")
done
if [ "${#bins[@]}" -gt 1 ]; then
  command -v lipo >/dev/null || { echo "lipo not found: set ARCHS to a single arch off macOS" >&2; exit 1; }
  lipo -create -output "$app/Contents/MacOS/cad-lite" "${bins[@]}"
else
  cp "${bins[0]}" "$app/Contents/MacOS/cad-lite"
fi
chmod 755 "$app/Contents/MacOS/cad-lite"

sed -e "s/@VERSION@/$SHORT/" -e "s/@BUILD@/${GITHUB_RUN_NUMBER:-1}/" \
  "$root/packaging/macos/Info.plist" > "$app/Contents/Info.plist"
printf 'APPL????' > "$app/Contents/PkgInfo"

icon="$root/assets/appicon/AppIcon.icns"
if [ -f "$icon" ]; then
  cp "$icon" "$app/Contents/Resources/AppIcon.icns"
else
  echo "warning: $icon missing; bundle has no icon" >&2
fi
echo "$app (version $VERSION)"
