#!/usr/bin/env bash
# Copy Platform example env/runtime templates into live files for local development.
# Usage: ./scripts/bootstrap-local-platform-config.sh [--force]

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CFG="$REPO_ROOT/Platform/config"
force=false
for arg in "$@"; do
	if [[ "$arg" == "--force" ]]; then
		force=true
	fi
done

copy_if_allowed() {
	local src=$1
	local dst=$2
	if [[ -f "$dst" && "$force" != "true" ]]; then
		echo "refuse: $dst exists (pass --force)" >&2
		exit 1
	fi
	cp "$src" "$dst"
}

copy_if_allowed "$CFG/platform.example.env" "$CFG/platform.env"
copy_if_allowed "$CFG/runtime-config.example.json" "$CFG/runtime-config.json"
echo "Created platform.env and runtime-config.json from platform.example.env and runtime-config.example.json"
echo "Next: replace-with-your-upstream-api-key and other PLATFORM_* placeholders in both files."
