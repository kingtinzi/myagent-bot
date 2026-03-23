#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_DIR="$REPO_ROOT/Platform/config"
EXAMPLE_ENV="$CONFIG_DIR/platform.example.env"
LIVE_ENV="$CONFIG_DIR/platform.env"
EXAMPLE_RUNTIME="$CONFIG_DIR/runtime-config.example.json"
LIVE_RUNTIME="$CONFIG_DIR/runtime-config.json"

FORCE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)
      FORCE=true
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [--force]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 [--force]" >&2
      exit 1
      ;;
  esac
done

copy_if_needed() {
  local src="$1"
  local dst="$2"
  if [[ ! -f "$src" ]]; then
    echo "ERROR: missing template: $src" >&2
    exit 1
  fi
  if [[ -f "$dst" && "$FORCE" != true ]]; then
    echo "Skip existing file: $dst (use --force to overwrite)"
    return 0
  fi
  cp -f "$src" "$dst"
  echo "Wrote: $dst"
}

mkdir -p "$CONFIG_DIR"
copy_if_needed "$EXAMPLE_ENV" "$LIVE_ENV"
copy_if_needed "$EXAMPLE_RUNTIME" "$LIVE_RUNTIME"

echo
echo "Done."
echo "Templates:"
echo "  - platform.example.env -> platform.env"
echo "  - runtime-config.example.json -> runtime-config.json"
echo "Replace placeholder values, especially replace-with-your-upstream-api-key."
