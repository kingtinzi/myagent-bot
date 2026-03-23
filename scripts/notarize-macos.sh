#!/usr/bin/env bash
# Notarize a packaged launcher-chat.app and staple it.
#
# Usage:
#   ./scripts/notarize-macos.sh dist/PinchBot-1.0.0-Darwin-arm64 --keychain-profile "notarytool-profile"
# Optional:
#   --bundle-id io.pinchbot.launcher
#   --skip-staple
#
# Notes:
# - The app must already be codesigned with a Developer ID Application certificate.
# - This script submits a ZIP of launcher-chat.app to Apple notary service.
# - Optional: export MAC_NOTARYTOOL_PROFILE=my-profile and omit --keychain-profile to use that notarytool profile.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

BUNDLE_ID="io.pinchbot.launcher"
KEYCHAIN_PROFILE=""
SKIP_STAPLE=false
PKG_DIR=""

usage() {
	echo "Usage: $0 <dist-package-dir> --keychain-profile <profile> [--bundle-id <id>] [--skip-staple]"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--keychain-profile)
		KEYCHAIN_PROFILE="${2:-}"
		shift 2
		;;
	--bundle-id)
		BUNDLE_ID="${2:-}"
		shift 2
		;;
	--skip-staple)
		SKIP_STAPLE=true
		shift
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
if [[ -z "$KEYCHAIN_PROFILE" && -n "${MAC_NOTARYTOOL_PROFILE:-}" ]]; then
	KEYCHAIN_PROFILE="$MAC_NOTARYTOOL_PROFILE"
fi
if [[ -z "$KEYCHAIN_PROFILE" ]]; then
	echo "ERROR: --keychain-profile is required (or set MAC_NOTARYTOOL_PROFILE)." >&2
	usage
	exit 1
fi

PKG_DIR_ABS="$(cd "$(dirname "$PKG_DIR")" && pwd)/$(basename "$PKG_DIR")"
APP_PATH="$PKG_DIR_ABS/launcher-chat.app"
if [[ ! -d "$APP_PATH" ]]; then
	echo "ERROR: app not found at: $APP_PATH" >&2
	exit 1
fi

if ! command -v xcrun >/dev/null 2>&1; then
	echo "ERROR: xcrun not found. Install Xcode command line tools." >&2
	exit 1
fi
if ! command -v codesign >/dev/null 2>&1; then
	echo "ERROR: codesign not found." >&2
	exit 1
fi
if ! command -v spctl >/dev/null 2>&1; then
	echo "ERROR: spctl not found." >&2
	exit 1
fi
if ! command -v ditto >/dev/null 2>&1; then
	echo "ERROR: ditto not found." >&2
	exit 1
fi

echo "==> Verify codesign"
codesign --verify --deep --strict --verbose=2 "$APP_PATH"

echo "==> Gatekeeper local assess (pre-notarization may still show warnings)"
spctl --assess --type execute -vv "$APP_PATH" || true

ZIP_PATH="$REPO_ROOT/dist/$(basename "$PKG_DIR_ABS")-launcher-chat-notary.zip"
rm -f "$ZIP_PATH"
echo "==> Create ZIP for notarization"
ditto -c -k --keepParent "$APP_PATH" "$ZIP_PATH"

echo "==> Submit to notary service"
xcrun notarytool submit "$ZIP_PATH" \
	--keychain-profile "$KEYCHAIN_PROFILE" \
	--primary-bundle-id "$BUNDLE_ID" \
	--wait

if [[ "$SKIP_STAPLE" == false ]]; then
	echo "==> Staple app ticket"
	xcrun stapler staple "$APP_PATH"
	echo "==> Validate stapled ticket"
	xcrun stapler validate "$APP_PATH"
fi

echo "==> Final Gatekeeper assess"
spctl --assess --type execute -vv "$APP_PATH" || true

echo "Done: notarization completed for $APP_PATH"
