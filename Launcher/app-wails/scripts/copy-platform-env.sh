#!/usr/bin/env bash
# Wails postBuildHook：工作目录常为 build/bin，也可用任意 cwd 调用本脚本。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_WAILS="$(cd "$SCRIPT_DIR/.." && pwd)"
# Launcher/app-wails -> Launcher -> 仓库根
REPO_ROOT="$(cd "$APP_WAILS/../.." && pwd)"
SRC="$REPO_ROOT/Platform/config/platform.env"
DST="$APP_WAILS/build/bin/config/platform.env"

if [[ -f "$SRC" ]]; then
	mkdir -p "$(dirname "$DST")"
	cp "$SRC" "$DST"
	echo "[postbuild] Copied Platform/config/platform.env -> build/bin/config/platform.env"
else
	echo "[postbuild] Skipped: $SRC not found (copy Platform/config/platform.example.env to platform.env and fill in values)"
fi
