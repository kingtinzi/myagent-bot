#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/package-macos-dmg.sh <package-dir> [--output path] [--volume-name name] [--overwrite]

Example:
  ./scripts/package-macos-dmg.sh dist/PinchBot-1.0.0-Darwin-arm64 --overwrite
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
OUTPUT_DMG=""
VOLUME_NAME=""
OVERWRITE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      OUTPUT_DMG="${2:-}"
      shift 2
      ;;
    --volume-name)
      VOLUME_NAME="${2:-}"
      shift 2
      ;;
    --overwrite)
      OVERWRITE=true
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
if [[ ! -d "$PACKAGE_DIR" ]]; then
  echo "Package directory not found: $PACKAGE_DIR" >&2
  exit 1
fi
if ! command -v hdiutil >/dev/null 2>&1; then
  echo "Required command not found: hdiutil" >&2
  exit 1
fi

PACKAGE_BASENAME="$(basename "$PACKAGE_DIR")"
if [[ -z "$VOLUME_NAME" ]]; then
  VOLUME_NAME="$PACKAGE_BASENAME"
fi
if [[ -z "$OUTPUT_DMG" ]]; then
  OUTPUT_DMG="$(dirname "$PACKAGE_DIR")/${PACKAGE_BASENAME}.dmg"
fi

if [[ -e "$OUTPUT_DMG" && "$OVERWRITE" != true ]]; then
  echo "Output DMG already exists: $OUTPUT_DMG (pass --overwrite to replace it)" >&2
  exit 1
fi

echo "Creating DMG at $OUTPUT_DMG ..."
hdiutil create -volname "$VOLUME_NAME" -srcfolder "$PACKAGE_DIR" -ov -format UDZO "$OUTPUT_DMG"

echo "Verifying DMG ..."
hdiutil verify "$OUTPUT_DMG"

echo "DMG ready: $OUTPUT_DMG"
