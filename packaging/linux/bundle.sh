#!/usr/bin/env bash
# Build the Linux tar.gz: bin/cad-lite, share/applications/cad-lite.desktop, share/icons/.../cad-lite.png.
# Needs cgo plus the X11/GL dev headers (see packaging/README.md); build natively, not cross.
#   VERSION=1.2.3  OUT=_tmp/dist/linux
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/packaging/lib.sh"
resolve_version
resolve_feedback
resolve_analytics
OUT="${OUT:-$root/_tmp/dist/linux}"
arch="$(go env GOARCH)"
name="cad-lite-$VERSION-linux-$arch"
stage="$OUT/$name"
rm -rf "$stage"; mkdir -p "$stage/bin" "$stage/share/applications" "$stage/share/icons/hicolor/256x256/apps"
(cd "$root" && go build -trimpath -ldflags "-s -w $LDFLAGS_VERSION $LDFLAGS_FEEDBACK $LDFLAGS_ANALYTICS" -o "$stage/bin/cad-lite" ./cmd/cad)
cp "$root/packaging/linux/cad-lite.desktop" "$stage/share/applications/"
svg="$root/assets/appicon/appicon.svg"
png="$stage/share/icons/hicolor/256x256/apps/cad-lite.png"
if [ -f "$svg" ] && command -v rsvg-convert >/dev/null; then
  rsvg-convert -w 512 -h 512 "$svg" -o "$png"
elif [ -f "$root/assets/appicon/window-256.png" ]; then
  cp "$root/assets/appicon/window-256.png" "$png"
else
  echo "warning: no icon (need rsvg-convert with $svg, or assets/appicon/window-256.png)" >&2
fi
cat > "$stage/install.sh" <<'EOS'
#!/bin/sh
# Install under PREFIX (default ~/.local).
set -e
p="${PREFIX:-$HOME/.local}"; d="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$p/bin" "$p/share"
cp "$d/bin/cad-lite" "$p/bin/"
cp -r "$d/share/." "$p/share/"
echo "installed to $p"
EOS
chmod +x "$stage/install.sh"
tar -C "$OUT" -czf "$OUT/$name.tar.gz" "$name"
echo "$OUT/$name.tar.gz"
