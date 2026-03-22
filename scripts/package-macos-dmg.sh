#!/usr/bin/env bash
# Create a customer-facing DMG from a dist package directory.
#
# Usage:
#   ./scripts/package-macos-dmg.sh dist/PinchBot-1.0.0-Darwin-arm64 --overwrite
# Optional:
#   --volname "PinchBot"
#   --codesign-identity "Developer ID Application: Your Company (TEAMID)"
#   --notarize-keychain-profile "notarytool-profile"
#   --bundle-id io.pinchbot.installer
#
# Output:
#   dist/<package-name>.dmg

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

PKG_DIR=""
OVERWRITE=false
VOLNAME="PinchBot"
CODESIGN_IDENTITY="${MAC_CODESIGN_IDENTITY:-}"
NOTARY_PROFILE=""
DMG_BUNDLE_ID="io.pinchbot.installer"

usage() {
	echo "Usage: $0 <dist-package-dir> [--overwrite] [--volname <name>] [--codesign-identity <identity>] [--notarize-keychain-profile <profile>] [--bundle-id <id>]"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--overwrite)
		OVERWRITE=true
		shift
		;;
	--volname)
		VOLNAME="${2:-}"
		shift 2
		;;
	--codesign-identity)
		CODESIGN_IDENTITY="${2:-}"
		shift 2
		;;
	--notarize-keychain-profile)
		NOTARY_PROFILE="${2:-}"
		shift 2
		;;
	--bundle-id)
		DMG_BUNDLE_ID="${2:-}"
		shift 2
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		if [[ -z "$PKG_DIR" ]]; then
			PKG_DIR="$1"
			shift
		else
			echo "Unknown argument: $1" >&2
			usage
			exit 1
		fi
		;;
	esac
done

if [[ -z "$PKG_DIR" ]]; then
	echo "ERROR: missing dist package dir." >&2
	usage
	exit 1
fi

PKG_DIR_ABS="$(cd "$(dirname "$PKG_DIR")" && pwd)/$(basename "$PKG_DIR")"
if [[ ! -d "$PKG_DIR_ABS" ]]; then
	echo "ERROR: package dir not found: $PKG_DIR_ABS" >&2
	exit 1
fi
if [[ ! -d "$PKG_DIR_ABS/launcher-chat.app" ]]; then
	echo "ERROR: launcher-chat.app not found in package dir." >&2
	exit 1
fi

BASENAME="$(basename "$PKG_DIR_ABS")"
DMG_PATH="$REPO_ROOT/dist/${BASENAME}.dmg"
STAGE="$(mktemp -d "${TMPDIR:-/tmp}/pinchbot-dmg.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

if [[ -f "$DMG_PATH" && "$OVERWRITE" != true ]]; then
	echo "ERROR: DMG already exists: $DMG_PATH (pass --overwrite to replace)" >&2
	exit 1
fi

echo "==> Stage DMG content"
cp -R "$PKG_DIR_ABS" "$STAGE/$BASENAME"
ln -sf /Applications "$STAGE/Applications"
xattr -cr "$STAGE/$BASENAME" 2>/dev/null || true

rm -f "$DMG_PATH"
echo "==> Build DMG: $DMG_PATH"
hdiutil create \
	-volname "$VOLNAME" \
	-srcfolder "$STAGE" \
	-ov \
	-format UDZO \
	-imagekey zlib-level=9 \
	-fs HFS+ \
	"$DMG_PATH"

echo "==> Verify DMG image structure"
hdiutil verify "$DMG_PATH"

if [[ -n "$CODESIGN_IDENTITY" ]]; then
	if ! command -v codesign >/dev/null 2>&1; then
		echo "ERROR: codesign not found." >&2
		exit 1
	fi
	echo "==> Sign DMG with identity: $CODESIGN_IDENTITY"
	codesign --force --timestamp --sign "$CODESIGN_IDENTITY" "$DMG_PATH"
	codesign --verify --verbose=2 "$DMG_PATH"
fi

if [[ -n "$NOTARY_PROFILE" ]]; then
	if ! command -v xcrun >/dev/null 2>&1; then
		echo "ERROR: xcrun not found." >&2
		exit 1
	fi
	echo "==> Submit DMG to Apple notary service"
	xcrun notarytool submit "$DMG_PATH" \
		--keychain-profile "$NOTARY_PROFILE" \
		--primary-bundle-id "$DMG_BUNDLE_ID" \
		--wait
	echo "==> Staple DMG ticket"
	xcrun stapler staple "$DMG_PATH"
	xcrun stapler validate "$DMG_PATH"
fi

echo "Done: $DMG_PATH"
