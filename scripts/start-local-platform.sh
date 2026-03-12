#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${1:-$REPO_ROOT/Platform/config/platform.example.env}"

resolve_go() {
  local candidate
  while IFS= read -r candidate; do
    if [[ -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done < <(find "$REPO_ROOT/.tools" -path '*/bin/go' -type f 2>/dev/null | sort)
  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi
  echo "Go executable not found. Install Go or place a toolchain under .tools/go*/bin/go" >&2
  return 1
}

GO_EXE="$(resolve_go)"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

echo "Starting platform-server, picoclaw-launcher, and launcher-chat..."

(cd "$REPO_ROOT/Platform" && "$GO_EXE" run ./cmd/platform-server) &
PLATFORM_PID=$!
(cd "$REPO_ROOT/PicoClaw" && "$GO_EXE" run ./cmd/picoclaw-launcher) &
LAUNCHER_PID=$!
(cd "$REPO_ROOT/Launcher/app-wails" && "$GO_EXE" run -tags desktop,production .) &
CHAT_PID=$!

echo "platform-server PID=$PLATFORM_PID"
echo "picoclaw-launcher PID=$LAUNCHER_PID"
echo "launcher-chat PID=$CHAT_PID"
echo "Press Ctrl+C to stop all."

trap 'kill $PLATFORM_PID $LAUNCHER_PID $CHAT_PID 2>/dev/null || true' EXIT
wait
