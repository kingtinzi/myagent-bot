#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLATFORM_DIR="$REPO_ROOT/Platform"
CONFIG_DIR="$PLATFORM_DIR/config"

PLATFORM_ENV=""
RUNTIME_CONFIG=""

usage() {
  cat <<'EOF'
Usage: ./scripts/start-local-platform.sh [--platform-env <path>] [--runtime-config <path>]
Starts Platform with a pinned local PinchBot state directory.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform-env)
      PLATFORM_ENV="${2:-}"
      shift 2
      ;;
    --runtime-config)
      RUNTIME_CONFIG="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$PLATFORM_ENV" ]]; then
  PLATFORM_ENV="$CONFIG_DIR/platform.env"
fi
if [[ "$PLATFORM_ENV" == *"platform.example.env" ]]; then
  echo "ERROR: platform.example.env is example-only; pass an explicit env file." >&2
  exit 1
fi
if [[ ! -f "$PLATFORM_ENV" ]]; then
  echo "ERROR: missing env file: $PLATFORM_ENV" >&2
  echo "Create one first with ./scripts/bootstrap-local-platform-config.sh" >&2
  exit 1
fi

if [[ -z "$RUNTIME_CONFIG" ]]; then
  RUNTIME_CONFIG="$CONFIG_DIR/runtime-config.json"
fi

export PINCHBOT_HOME="${PINCHBOT_HOME:-$REPO_ROOT/.openclaw}"
export PINCHBOT_CONFIG="${PINCHBOT_CONFIG:-$PINCHBOT_HOME/config.json}"
export PLATFORM_RUNTIME_CONFIG_PATH="${PLATFORM_RUNTIME_CONFIG_PATH:-$RUNTIME_CONFIG}"

set -a
source "$PLATFORM_ENV"
set +a

echo "PINCHBOT_HOME=$PINCHBOT_HOME"
echo "PINCHBOT_CONFIG=$PINCHBOT_CONFIG"
echo "PLATFORM_RUNTIME_CONFIG_PATH=$PLATFORM_RUNTIME_CONFIG_PATH"

cd "$PLATFORM_DIR"
exec go run ./cmd/platform-server
