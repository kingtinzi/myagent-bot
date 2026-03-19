#!/usr/bin/env bash
# 将 dist 下的 macOS 分发目录打成客户可下载的 .dmg（只读压缩镜像）。
# 用法:
#   ./scripts/package-dmg.sh                          # 自动选 dist 下最新的 PinchBot-*-macOS-*
#   ./scripts/package-dmg.sh dist/PinchBot-xxx-macOS-amd64
#
# 产出: dist/<同名>.dmg（内含应用包、platform-server、config、README，并带「应用程序」快捷方式）
#
# 正式发给客户前建议:
#   - Apple 开发者账号下对 .app 及 DMG 做代码签名 + 公证 (notarytool)，否则客户可能遇 Gatekeeper 拦截。
#   - 见: https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	echo "Usage: $0 [path-to-dist-folder]"
	exit 0
fi

if [[ -n "${1:-}" ]]; then
	SRC="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
else
	SRC="$(ls -dt "$REPO_ROOT"/dist/PinchBot-*-macOS-* 2>/dev/null | head -1 || true)"
fi

if [[ -z "$SRC" || ! -d "$SRC" ]]; then
	echo "ERROR: 找不到分发目录。请先运行 ./scripts/package-macos.sh，或传入路径。" >&2
	exit 1
fi

BASENAME="$(basename "$SRC")"
DMG_PATH="$REPO_ROOT/dist/${BASENAME}.dmg"
STAGE="$(mktemp -d "${TMPDIR:-/tmp}/pinchbot-dmg.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

echo "==> Source: $SRC"
echo "==> Staging DMG contents ..."

# DMG 根目录展示名（客户打开磁盘后看到的文件夹）
VOL_ROOT="$STAGE/$BASENAME"
mkdir -p "$VOL_ROOT"
cp -R "$SRC"/. "$VOL_ROOT/"
# 方便用户把 .app 拖进「应用程序」
ln -sf /Applications "$STAGE/Applications"

# 去掉 quarantine 等扩展属性再打包（可选，减少客户侧异常）
xattr -cr "$VOL_ROOT" 2>/dev/null || true

rm -f "$DMG_PATH"
echo "==> Creating: $DMG_PATH"
# UDZO = 压缩只读，适合分发
hdiutil create -volname "$BASENAME" -srcfolder "$STAGE" -ov -format UDZO -imagekey zlib-level=9 -fs HFS+ "$DMG_PATH"

echo "Done: $DMG_PATH"
ls -lh "$DMG_PATH"
