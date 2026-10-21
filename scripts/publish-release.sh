#!/usr/bin/env bash
set -euo pipefail
set -- "$GITHUB_REF_NAME" "$DIST"/* --verify-tag --generate-notes --title "$GITHUB_REF_NAME"
case "$GITHUB_REF_NAME" in
  *-*) set -- "$@" --prerelease ;;
esac
gh release create "$@"
