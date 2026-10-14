#!/usr/bin/env bash
# Sign (and optionally notarize) go-draw.app, then write the zip and dmg. Runs on macOS only.
#   APP=path/to/go-draw.app   OUT=dir for the zip and dmg   VERSION=1.2.3
# Mode is picked by which environment variables are set:
#   MACOS_CERT_P12 (base64) + MACOS_CERT_PASSWORD + MACOS_SIGN_IDENTITY  -> Developer ID signed
#     + APPLE_ID, APPLE_TEAM_ID, APPLE_APP_PASSWORD                      -> also notarized, stapled
#   nothing set                                                           -> ad-hoc signed
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$root/packaging/lib.sh"
resolve_version
APP="${APP:-$root/_tmp/dist/macos/go-draw.app}"
OUT="${OUT:-$(dirname "$APP")}"
TMP="${RUNNER_TEMP:-$(mktemp -d)}"
zip="$OUT/go-draw-$VERSION-macos-universal.zip"
dmg="$OUT/go-draw-$VERSION-macos-universal.dmg"

make_dmg() {
  rm -rf "$TMP/dmg" "$dmg"; mkdir "$TMP/dmg"
  ditto "$APP" "$TMP/dmg/go-draw.app"
  ln -s /Applications "$TMP/dmg/Applications"
  hdiutil create -volname go-draw -srcfolder "$TMP/dmg" -ov -format UDZO "$dmg"
}

notarize() { # notarize FILE
  local json="$TMP/notary.json" id status
  xcrun notarytool submit "$1" --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" \
    --password "$APPLE_APP_PASSWORD" --wait --timeout 30m --output-format json > "$json" || true
  cat "$json"
  id="$(plutil -extract id raw "$json")"; status="$(plutil -extract status raw "$json")"
  if [ "$status" != "Accepted" ]; then
    xcrun notarytool log "$id" --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD"
    exit 1
  fi
}

if [ -z "${MACOS_CERT_P12:-}" ]; then
  echo "::warning::no signing certificate; ad-hoc signing go-draw.app (Gatekeeper will ask the user to Open Anyway)"
  codesign --force --deep --sign - "$APP"
  codesign --verify --deep --strict --verbose=2 "$APP"
  make_dmg
else
  kc="$TMP/sign.keychain-db"; pw="$(uuidgen)"
  echo "$MACOS_CERT_P12" | base64 --decode > "$TMP/cert.p12"
  security create-keychain -p "$pw" "$kc"
  security set-keychain-settings -lut 21600 "$kc"
  security unlock-keychain -p "$pw" "$kc"
  security import "$TMP/cert.p12" -k "$kc" -P "$MACOS_CERT_PASSWORD" -T /usr/bin/codesign
  security set-key-partition-list -S apple-tool:,apple: -s -k "$pw" "$kc"
  # shellcheck disable=SC2046
  security list-keychains -d user -s "$kc" $(security list-keychains -d user | tr -d '"')
  rm "$TMP/cert.p12"
  trap 'security delete-keychain "$kc"' EXIT

  notarizing=false
  if [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ] && [ -n "${APPLE_APP_PASSWORD:-}" ]; then notarizing=true; fi
  ts=--timestamp=none; if $notarizing; then ts=--timestamp; fi
  for target in "$APP/Contents/MacOS/go-draw" "$APP"; do
    codesign --force $ts --options runtime --keychain "$kc" --sign "$MACOS_SIGN_IDENTITY" "$target"
  done
  codesign --verify --deep --strict --verbose=2 "$APP"
  if $notarizing; then
    ditto -c -k --keepParent "$APP" "$TMP/notarize.zip"
    notarize "$TMP/notarize.zip"
    xcrun stapler staple "$APP"
    xcrun stapler validate "$APP"
  else
    echo "::warning::APPLE_* secrets not set; go-draw.app is signed but not notarized"
  fi
  make_dmg
  codesign --force $ts --keychain "$kc" --sign "$MACOS_SIGN_IDENTITY" "$dmg"
  if $notarizing; then notarize "$dmg"; xcrun stapler staple "$dmg"; fi
fi

# Zipped last, so a stapled ticket travels inside the archive.
rm -f "$zip"; ditto -c -k --keepParent "$APP" "$zip"
echo "$zip"; echo "$dmg"
