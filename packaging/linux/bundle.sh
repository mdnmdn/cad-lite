#!/usr/bin/env bash
# Build the Linux tar.gz: bin/go-draw, share/applications/go-draw.desktop, share/icons/.../go-draw.png.
# Needs cgo plus the X11/GL dev headers (see packaging/README.md); build natively, not cross.
#   VERSION=1.2.3  OUT=_tmp/dist/linux
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/packaging/lib.sh"
resolve_version
OUT="${OUT:-$root/_tmp/dist/linux}"
arch="$(go env GOARCH)"
name="go-draw-$VERSION-linux-$arch"
stage="$OUT/$name"
rm -rf "$stage"; mkdir -p "$stage/bin" "$stage/share/applications" "$stage/share/icons/hicolor/256x256/apps"
(cd "$root" && go build -trimpath -ldflags "-s -w $LDFLAGS_VERSION" -o "$stage/bin/go-draw" ./cmd/cad)
cp "$root/packaging/linux/go-draw.desktop" "$stage/share/applications/"
svg="$root/assets/appicon/appicon.svg"
png="$stage/share/icons/hicolor/256x256/apps/go-draw.png"
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
cp "$d/bin/go-draw" "$p/bin/"
cp -r "$d/share/." "$p/share/"
echo "installed to $p"
EOS
chmod +x "$stage/install.sh"
tar -C "$OUT" -czf "$OUT/$name.tar.gz" "$name"
echo "$OUT/$name.tar.gz"
