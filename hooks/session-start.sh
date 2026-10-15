#!/usr/bin/env bash
# neckbeard plugin — SessionStart context.
#
# Read-only by design: reports whether the `neckbeard` CLI is reachable and
# where the plugin's installer lives. It never downloads, installs, or writes
# anything; installing is an explicit step (/neckbeard:doctor or the script).
set -u

root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
installer="$root/scripts/install.sh"

if bin="$(command -v neckbeard 2>/dev/null)"; then
  ver="$("$bin" version 2>/dev/null || echo 'version unknown')"
  status="CLI: $ver at $bin."
else
  # Common install locations that may simply be missing from PATH.
  found=""
  for candidate in "$HOME/.local/bin/neckbeard" "${GOBIN:-}/neckbeard" "$(go env GOPATH 2>/dev/null)/bin/neckbeard"; do
    case "$candidate" in /*) [ -x "$candidate" ] && { found="$candidate"; break; } ;; esac
  done
  if [ -n "$found" ]; then
    status="CLI: installed at $found but that directory is not on PATH — tell the user to add it (or invoke the binary by that absolute path)."
  else
    status="CLI: NOT installed. Before the first neckbeard command, run: bash \"$installer\" (sha256-verified release binary, or go install when Go is present). /neckbeard:doctor does this and checks the other tools."
  fi
fi

# JSON string escaping for the two values we interpolate (paths may hold quotes).
esc() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"neckbeard plugin loaded (root: %s). %s Flow: /neckbeard:analyze → /neckbeard:estimate → /neckbeard:sync. /neckbeard:help prints the card."}}\n' \
  "$(esc "$root")" "$(esc "$status")"
exit 0
