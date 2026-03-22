#!/usr/bin/env bash
# Backward-compatible wrapper. Prefer:
#   ./scripts/package-macos-dmg.sh <dist-package-dir> [options]

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	echo "Deprecated wrapper. Use: ./scripts/package-macos-dmg.sh <dist-package-dir> [--overwrite]"
	exit 0
fi

if [[ -n "${1:-}" ]]; then
	PKG_DIR="$1"
	shift || true
else
	PKG_DIR="$(ls -1dt "$REPO_ROOT"/dist/PinchBot-*-Darwin-* 2>/dev/null | head -1 || true)"
fi

if [[ -z "$PKG_DIR" ]]; then
	echo "ERROR: no macOS package dir found in dist/." >&2
	exit 1
fi

"$REPO_ROOT/scripts/package-macos-dmg.sh" "$PKG_DIR" --overwrite "$@"
