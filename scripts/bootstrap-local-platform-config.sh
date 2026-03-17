#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_DIR="$REPO_ROOT/Platform/config"
FORCE=false

for arg in "$@"; do
  case "$arg" in
    -f|--force)
      FORCE=true
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Usage: ./scripts/bootstrap-local-platform-config.sh [--force]" >&2
      exit 1
      ;;
  esac
done

copy_template() {
  local source_name="$1"
  local target_name="$2"
  local source_path="$CONFIG_DIR/$source_name"
  local target_path="$CONFIG_DIR/$target_name"

  if [[ ! -f "$source_path" ]]; then
    echo "Missing template: $source_path" >&2
    exit 1
  fi
  if [[ -f "$target_path" && "$FORCE" != true ]]; then
    echo "Skip existing $target_path (use --force to overwrite)"
    return
  fi
  cp "$source_path" "$target_path"
  echo "Wrote $target_path"
}

copy_template "platform.example.env" "platform.env"
copy_template "runtime-config.example.json" "runtime-config.json"

cat <<'EOF'

Local platform live config bootstrap complete.

Next steps:
  1. Edit Platform/config/platform.env and replace live values such as:
     - PLATFORM_DATABASE_URL
     - PLATFORM_SUPABASE_URL / PLATFORM_SUPABASE_ANON_KEY
     - PLATFORM_SUPABASE_JWKS_URL or PLATFORM_SUPABASE_JWT_SECRET
     - PLATFORM_EASYPAY_PID / PLATFORM_EASYPAY_KEY (if payment is enabled)
  2. Edit Platform/config/runtime-config.json and replace placeholders such as:
     - replace-with-your-upstream-api-key
     - example agreement URLs
     - official model pricing/version metadata
  3. Start the local stack with:
     - ./scripts/start-local-platform.sh

Do not ship platform.example.env or runtime-config.example.json as live config.
EOF
