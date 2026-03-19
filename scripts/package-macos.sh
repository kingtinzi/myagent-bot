#!/usr/bin/env bash
# 将 macOS 相关编译产物清理后统一打包到 dist/PinchBot-<version>-macOS-<arch>/
# 用法:
#   ./scripts/package-macos.sh              # 清理中间产物 + 全量构建 + 写入 dist
#   ./scripts/package-macos.sh --skip-build # 仅整理 dist（需已手动构建过）
#   ./scripts/package-macos.sh --clean-only # 只清理，不构建
#
# 依赖: Go、Wails CLI（PATH）、可选 GOPROXY（默认尝试 goproxy.cn）

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

SKIP_BUILD=false
CLEAN_ONLY=false
for arg in "$@"; do
	case "$arg" in
	--skip-build) SKIP_BUILD=true ;;
	--clean-only) CLEAN_ONLY=true ;;
	-h|--help)
		echo "Usage: $0 [--skip-build | --clean-only]"
		exit 0
		;;
	esac
done

export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"
export PATH="$(go env GOPATH)/bin:$PATH"

MACHINE="$(uname -m)"
case "$MACHINE" in
x86_64) GOARCH_LABEL=amd64 ;;
arm64) GOARCH_LABEL=arm64 ;;
*) GOARCH_LABEL="$MACHINE" ;;
esac

VERSION="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
PKG_NAME="PinchBot-${VERSION}-macOS-${GOARCH_LABEL}"
OUT_DIR="$REPO_ROOT/dist/$PKG_NAME"

clean_intermediates() {
	echo "==> Cleaning intermediate build outputs ..."
	( cd "$REPO_ROOT/PinchBot" && make clean 2>/dev/null || true )
	rm -rf "$REPO_ROOT/Launcher/app-wails/build/bin" \
		"$REPO_ROOT/Launcher/app-wails/build/darwin"
	rm -f "$REPO_ROOT/Platform/platform-server"
	echo "    (PinchBot/build, Launcher/app-wails/build/bin|darwin, Platform/platform-server cleared)"
}

clean_dist_macos_packages() {
	echo "==> Removing previous dist/PinchBot-*-macOS-* ..."
	rm -rf "$REPO_ROOT"/dist/PinchBot-*-macOS-* 2>/dev/null || true
}

if [[ "$CLEAN_ONLY" == true ]]; then
	clean_intermediates
	clean_dist_macos_packages
	echo "Clean done."
	exit 0
fi

clean_dist_macos_packages
mkdir -p "$OUT_DIR"

if [[ "$SKIP_BUILD" != true ]]; then
	clean_intermediates

	echo "==> [1/4] PinchBot (make build) ..."
	( cd "$REPO_ROOT/PinchBot" && make build )

	echo "==> [2/4] Platform (platform-server) ..."
	( cd "$REPO_ROOT/Platform" && go build -o "$OUT_DIR/platform-server" ./cmd/platform-server )

	echo "==> [3/4] Launcher (wails build) ..."
	if ! command -v wails >/dev/null 2>&1; then
		echo "ERROR: wails not in PATH. Install: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
		exit 1
	fi
	( cd "$REPO_ROOT/Launcher/app-wails" && wails build -o launcher-chat )

	echo "==> [4/4] Assemble dist layout ..."
else
	echo "==> --skip-build: assembling from existing build/bin ..."
fi

# 从 wails 输出拷贝 .app（及 postBuild 生成的 config）
WAILS_BIN="$REPO_ROOT/Launcher/app-wails/build/bin"
if [[ ! -d "$WAILS_BIN/launcher-chat.app" ]]; then
	echo "ERROR: $WAILS_BIN/launcher-chat.app not found. Run without --skip-build." >&2
	exit 1
fi
cp -R "$WAILS_BIN/launcher-chat.app" "$OUT_DIR/"
mkdir -p "$OUT_DIR/config"
if [[ -d "$WAILS_BIN/config" ]]; then
	cp -R "$WAILS_BIN/config/." "$OUT_DIR/config/" 2>/dev/null || true
fi

if [[ ! -f "$OUT_DIR/platform-server" ]]; then
	if [[ -f "$REPO_ROOT/Platform/platform-server" ]]; then
		cp -f "$REPO_ROOT/Platform/platform-server" "$OUT_DIR/platform-server"
		chmod +x "$OUT_DIR/platform-server"
		echo "    (copied existing Platform/platform-server into dist)"
	fi
fi

# PinchBot 主程序（与 Windows 包名 pinchbot 对齐，无后缀）
PINCH_SRC="$REPO_ROOT/PinchBot/build/picoclaw-darwin-${GOARCH_LABEL}"
if [[ ! -f "$PINCH_SRC" ]]; then
	# Makefile 在部分环境可能用不同命名，取唯一匹配
	PINCH_SRC="$(ls "$REPO_ROOT/PinchBot/build"/picoclaw-darwin-* 2>/dev/null | head -1 || true)"
fi
if [[ -n "${PINCH_SRC:-}" && -f "$PINCH_SRC" ]]; then
	cp "$PINCH_SRC" "$OUT_DIR/pinchbot"
	chmod +x "$OUT_DIR/pinchbot"
else
	echo "WARNING: PinchBot binary not found under PinchBot/build/; skip pinchbot copy." >&2
fi

# 若 wails 未复制示例，则补全 config 模板
PB_CFG="$REPO_ROOT/PinchBot/config/config.example.json"
[[ -f "$PB_CFG" ]] && cp -f "$PB_CFG" "$OUT_DIR/config/config.example.json"
PF_EX="$REPO_ROOT/Platform/config/platform.example.env"
[[ -f "$PF_EX" ]] && cp -f "$PF_EX" "$OUT_DIR/config/platform.example.env"
RT_EX="$REPO_ROOT/Platform/config/runtime-config.example.json"
[[ -f "$RT_EX" ]] && cp -f "$RT_EX" "$OUT_DIR/config/runtime-config.example.json"

# 若 dist 里仍无 platform.env，而仓库 Platform 有 live 文件，则打入包（与 build-release -IncludeLivePlatformConfig 类似）
PF_LIVE="$REPO_ROOT/Platform/config/platform.env"
if [[ -f "$PF_LIVE" && ! -f "$OUT_DIR/config/platform.env" ]]; then
	cp -f "$PF_LIVE" "$OUT_DIR/config/platform.env"
	echo "    (bundled Platform/config/platform.env into dist config/)"
fi
RT_LIVE="$REPO_ROOT/Platform/config/runtime-config.json"
if [[ -f "$RT_LIVE" && ! -f "$OUT_DIR/config/runtime-config.json" ]]; then
	cp -f "$RT_LIVE" "$OUT_DIR/config/runtime-config.json"
	echo "    (bundled Platform/config/runtime-config.json into dist config/)"
fi

README_PATH="$OUT_DIR/README.txt"
cat >"$README_PATH" <<EOF
PinchBot — macOS 分发目录
========================================
Version: $VERSION
Arch: $GOARCH_LABEL
Output: $OUT_DIR

本目录即为可交给用户的完整布局（请整夹复制或打 zip）：

  launcher-chat.app   主程序（双击运行，勿只双击 Contents/MacOS 内二进制）
  pinchbot            可选独立网关二进制（调试/兼容；日常网关已在 launcher 进程内）
  platform-server     平台后端（存在 config/platform.env 时由 launcher 自动拉起）
  config/
    config.example.json
    platform.example.env
    platform.env        （若构建机存在 Platform/config/platform.env 会自动带入）
    runtime-config*.json
  README.txt          本说明

首次运行：双击 launcher-chat.app。请保持本目录内文件相对位置不变。

用户数据：未设置 PINCHBOT_HOME 时，从「访达双击 .app」启动会将配置与 workspace 写在
  ~/Library/Application Support/PinchBot/
（避免 App Translocation / 包旁只读导致秒退）。命令行直接跑二进制时仍可用当前目录下的 .pinchbot。

清理中间产物可执行:  ./scripts/package-macos.sh --clean-only
重新全量打包:        ./scripts/package-macos.sh

EOF

echo ""
echo "Done: $OUT_DIR"
ls -la "$OUT_DIR"
