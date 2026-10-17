# Shared helpers for packaging/*/bundle.sh. Source this file; it is not executable.

# repo_root prints the repository root (the directory above packaging/).
repo_root() { cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd; }

# resolve_version sets SHORT (major.minor.patch), VERSION (semver without a leading v, may carry a -suffix),
# NUMERIC (major.minor.patch.0 for Windows resources and CFBundleVersion) and
# LDFLAGS_VERSION (an -X flag, empty when main has no `version` variable).
# Input: $VERSION, else the tag in $GITHUB_REF_NAME, else `git describe`, else 0.0.0-dev.
resolve_version() {
  local root v
  root="$(repo_root)"
  v="${VERSION:-}"
  if [ -z "$v" ] && [[ "${GITHUB_REF_TYPE:-}" == tag ]]; then v="${GITHUB_REF_NAME:-}"; fi
  if [ -z "$v" ]; then v="$(git -C "$root" describe --tags --always --dirty 2>/dev/null || true)"; fi
  v="${v#v}"
  [ -n "$v" ] || v="0.0.0-dev"
  VERSION="$v"
  local core="${v%%-*}"
  if [[ "$core" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then NUMERIC="$core.0"; SHORT="$core"; else NUMERIC="0.0.0.0"; SHORT="0.0.0"; fi
  LDFLAGS_VERSION=""
  if grep -Eq '^\s*(var\s+)?version\s*(=|string)' "$root/cmd/cad/main.go" 2>/dev/null; then
    LDFLAGS_VERSION="-X main.version=$VERSION"
  fi
}

# resolve_feedback supplies optional desktop intake settings without shell-sensitive linker values.
# These credentials are extractable from a desktop executable; never inject them in browser builds.
resolve_feedback() {
  LDFLAGS_FEEDBACK=""
  if [ -n "${FEEDBACK_URL:-}" ] && [ -n "${FEEDBACK_KEY:-}" ]; then
    local feedback_url_encoded feedback_key_encoded
    feedback_url_encoded="$(printf '%s' "$FEEDBACK_URL" | base64 | tr -d '\r\n')"
    feedback_key_encoded="$(printf '%s' "$FEEDBACK_KEY" | base64 | tr -d '\r\n')"
    LDFLAGS_FEEDBACK="-X main.feedbackURLBase64=$feedback_url_encoded -X main.feedbackKeyBase64=$feedback_key_encoded"
  fi
}
