#!/usr/bin/env bash
# Start Platform server from a dev checkout with predictable local state.
# Sets PINCHBOT_HOME and PINCHBOT_CONFIG so `go run` uses this repo (not your global home).
# Usage (from repo root): ./scripts/start-local-platform.sh

set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PINCHBOT_HOME="${REPO_ROOT}/.openclaw-dev"
export PINCHBOT_CONFIG="${REPO_ROOT}/PinchBot/config.json"
mkdir -p "$PINCHBOT_HOME"
echo "PINCHBOT_HOME=$PINCHBOT_HOME"
echo "PINCHBOT_CONFIG=$PINCHBOT_CONFIG"
cd "${REPO_ROOT}/Platform"
exec go run ./cmd/platform-server
