#!/usr/bin/env bash
# Installs the neckbeard CLI for the plugin. Resolution order:
#   1. A GitHub Release binary for this OS/arch, sha256-verified against the
#      release's checksums.txt. Nothing is executed before verification.
#   2. `go install github.com/bealesh/neckbeard/cmd/neckbeard@<version>` when
#      Go is on PATH (source build; the module proxy verifies the checksum).
#   3. Otherwise: print what to do. Never curl|bash anything else.
#
# Usage: install.sh [-v vX.Y.Z|latest] [-d BIN_DIR] [-f]
#   -v  release tag (default: plugin manifest version, else latest stable)
#   -d  install directory (default: $NECKBEARD_BIN_DIR or ~/.local/bin; no sudo)
#   -f  reinstall even if `neckbeard` is already on PATH
set -euo pipefail

REPO="bealesh/neckbeard"
version="latest"
manifest="$(dirname "$0")/../.claude-plugin/plugin.json"
if [ -f "$manifest" ]; then
  plugin_version="$(sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' "$manifest" | head -n1)"
  if [ -n "$plugin_version" ]; then version="v${plugin_version#v}"; fi
fi
dir="${NECKBEARD_BIN_DIR:-$HOME/.local/bin}"
force=0

usage() { sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; }
while getopts "v:d:fh" opt; do
  case "$opt" in
    v) version="$OPTARG" ;;
    d) dir="$OPTARG" ;;
    f) force=1 ;;
    h) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

if [ "$force" = 0 ] && existing="$(command -v neckbeard 2>/dev/null)"; then
  echo "already installed: $("$existing" version) at $existing (pass -f to reinstall)"
  exit 0
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
esac
case "$os" in
  linux | darwin) ;;
  *) echo "no release binaries for $os; use: go install github.com/$REPO/cmd/neckbeard@$version" >&2; exit 1 ;;
esac

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

path_hint() {
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) echo "note: $dir is not on PATH — add it (e.g. export PATH=\"$dir:\$PATH\") or call $dir/neckbeard directly" ;;
  esac
}

# --- 1. release binary -------------------------------------------------------
tag="$version"
if [ "$version" = "latest" ]; then
  tag=""
  if command -v curl >/dev/null 2>&1; then
    tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
      | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true)"
  fi
fi

if [ -n "$tag" ] && command -v curl >/dev/null 2>&1; then
  asset="neckbeard_${tag#v}_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/$tag"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  if curl -fsSL -o "$tmp/$asset" "$base/$asset" && curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"; then
    # sha256sum lines are "<hash>  <name>"; tolerate a "./" prefix and the "*"
    # binary-mode marker so a differently generated checksums.txt still matches.
    want="$(grep -E "[[:space:]]\\*?(\\./)?$asset\$" "$tmp/checksums.txt" | awk '{print $1}' || true)"
    if [ -z "$want" ]; then
      echo "refusing: checksums.txt for $tag has no entry for $asset" >&2
      exit 1
    fi
    got="$(sha256 "$tmp/$asset")"
    if [ "$want" != "$got" ]; then
      echo "refusing: sha256 mismatch for $asset (want $want, got $got)" >&2
      exit 1
    fi
    tar -xzf "$tmp/$asset" -C "$tmp" neckbeard
    mkdir -p "$dir"
    install -m 0755 "$tmp/neckbeard" "$dir/neckbeard"
    echo "installed $("$dir/neckbeard" version) → $dir/neckbeard (release $tag, sha256 verified)"
    path_hint
    exit 0
  fi
  echo "no release asset $asset for $tag; falling back to go install" >&2
fi

# --- 2. go install -----------------------------------------------------------
if command -v go >/dev/null 2>&1; then
  mkdir -p "$dir"
  GOBIN="$dir" go install "github.com/$REPO/cmd/neckbeard@${tag:-latest}"
  echo "installed $("$dir/neckbeard" version) → $dir/neckbeard (go install)"
  path_hint
  exit 0
fi

# --- 3. nothing we can do safely --------------------------------------------
cat >&2 <<MSG
neckbeard is not installed and neither a release binary nor Go was available.
  - install Go (https://go.dev/dl) and rerun this script, or
  - download a release from https://github.com/$REPO/releases and put
    'neckbeard' on your PATH.
MSG
exit 1
