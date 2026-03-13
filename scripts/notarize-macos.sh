#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/notarize-macos.sh <package-dir> [--app-name launcher-chat.app] [--keychain-profile profile] [--output-zip path] [--skip-spctl]

Examples:
  ./scripts/notarize-macos.sh dist/PinchBot-1.0.0-Darwin-arm64 --keychain-profile notarytool-password
  MAC_NOTARYTOOL_PROFILE=notarytool-password ./scripts/notarize-macos.sh dist/PinchBot-1.0.0-Darwin-arm64
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || $# -eq 0 ]]; then
  usage
  exit 0
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must run on macOS." >&2
  exit 1
fi

PACKAGE_DIR=""
APP_NAME="launcher-chat.app"
KEYCHAIN_PROFILE="${MAC_NOTARYTOOL_PROFILE:-}"
OUTPUT_ZIP=""
SKIP_SPCTL=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --app-name)
      APP_NAME="${2:-}"
      shift 2
      ;;
    --keychain-profile)
      KEYCHAIN_PROFILE="${2:-}"
      shift 2
      ;;
    --output-zip)
      OUTPUT_ZIP="${2:-}"
      shift 2
      ;;
    --skip-spctl)
      SKIP_SPCTL=true
      shift
      ;;
    -*)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
    *)
      if [[ -n "$PACKAGE_DIR" ]]; then
        echo "Only one package directory may be provided." >&2
        exit 1
      fi
      PACKAGE_DIR="$1"
      shift
      ;;
  esac
done

if [[ -z "$PACKAGE_DIR" ]]; then
  echo "Missing package directory." >&2
  usage
  exit 1
fi
if [[ -z "$KEYCHAIN_PROFILE" ]]; then
  echo "Missing notarytool keychain profile. Pass --keychain-profile or set MAC_NOTARYTOOL_PROFILE." >&2
  exit 1
fi
if [[ ! -d "$PACKAGE_DIR" ]]; then
  echo "Package directory not found: $PACKAGE_DIR" >&2
  exit 1
fi

for command_name in xcrun codesign ditto; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Required command not found: $command_name" >&2
    exit 1
  fi
done
if [[ "$SKIP_SPCTL" != true ]] && ! command -v spctl >/dev/null 2>&1; then
  echo "Required command not found: spctl" >&2
  exit 1
fi

APP_PATH="$PACKAGE_DIR/$APP_NAME"
if [[ ! -d "$APP_PATH" ]]; then
  echo "App bundle not found: $APP_PATH" >&2
  exit 1
fi

PACKAGE_BASENAME="$(basename "$PACKAGE_DIR")"
if [[ -z "$OUTPUT_ZIP" ]]; then
  OUTPUT_ZIP="$(dirname "$PACKAGE_DIR")/${PACKAGE_BASENAME}-notarize.zip"
fi

echo "[1/4] Verifying code signature..."
codesign --verify --deep --strict --verbose=2 "$APP_PATH"

if [[ "$SKIP_SPCTL" != true ]]; then
  echo "[2/4] Running Gatekeeper assessment before submit..."
  spctl --assess --type execute -vv "$APP_PATH"
else
  echo "[2/4] Skipping pre-submit Gatekeeper assessment."
fi

echo "[3/4] Creating notarization ZIP at $OUTPUT_ZIP ..."
rm -f "$OUTPUT_ZIP"
ditto -c -k --keepParent "$APP_PATH" "$OUTPUT_ZIP"

echo "[4/4] Submitting to Apple notary service..."
xcrun notarytool submit "$OUTPUT_ZIP" --keychain-profile "$KEYCHAIN_PROFILE" --wait

echo "Stapling notarization ticket..."
xcrun stapler staple "$APP_PATH"
xcrun stapler validate "$APP_PATH"

if [[ "$SKIP_SPCTL" != true ]]; then
  echo "Re-running Gatekeeper assessment after staple..."
  spctl --assess --type execute -vv "$APP_PATH"
fi

echo "Notarization complete for $APP_PATH"
