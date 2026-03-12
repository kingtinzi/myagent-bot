#!/usr/bin/env bash
# PinchBot release build for macOS - produces a .app bundle plus companion files.
# External distribution still requires Apple signing and notarization.
# Usage: ./scripts/build-release.sh [version] [-z]
#   version: optional, default from git describe
#   -z: create .tar.gz after build
# Output: dist/PinchBot-<version>-Darwin-<arch>/

set -e
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
PINCHBOT_DIR="$REPO_ROOT/PinchBot"
LAUNCHER_DIR="$REPO_ROOT/Launcher/app-wails"
PLATFORM_DIR="$REPO_ROOT/Platform"

resolve_go() {
  local candidate
  local -a path_candidates

  go_candidate_works() {
    local candidate_path="$1"
    [[ -n "$candidate_path" && -x "$candidate_path" ]] || return 1
    "$candidate_path" version >/dev/null 2>&1
  }

  while IFS= read -r candidate; do
    if go_candidate_works "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done < <(find "$REPO_ROOT/.tools" \( -path '*/bin/go' -o -path '*/bin/go.exe' \) -type f 2>/dev/null | sort)

  path_candidates=()
  if command -v go >/dev/null 2>&1; then
    path_candidates+=("$(command -v go)")
  fi
  if command -v go.exe >/dev/null 2>&1; then
    path_candidates+=("$(command -v go.exe)")
  fi
  for candidate in "${path_candidates[@]}"; do
    if go_candidate_works "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  echo "Go executable not found. Install Go or place a toolchain under .tools/go*/bin/go(.exe)" >&2
  return 1
}

GO_EXE="$(resolve_go)"

sanitize_bundle_version() {
  local raw="${1#v}"
  local parsed
  parsed=$(printf '%s' "$raw" | sed -E 's/^[^0-9]*([0-9]+(\.[0-9]+){0,2}).*/\1/')
  if [[ -z "$parsed" ]]; then
    parsed="0.0.0"
  fi
  IFS='.' read -r major minor patch _ <<< "$parsed"
  printf '%s.%s.%s\n' "${major:-0}" "${minor:-0}" "${patch:-0}"
}

MAKE_ZIP=false
VERSION=""
for arg in "$@"; do
  if [[ "$arg" == "-z" ]]; then
    MAKE_ZIP=true
  else
    VERSION="$arg"
  fi
done
if [[ -z "$VERSION" ]]; then
  VERSION=$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null) || VERSION="dev"
fi
BUNDLE_SHORT_VERSION="$(sanitize_bundle_version "$VERSION")"
BUNDLE_VERSION="$BUNDLE_SHORT_VERSION"

UNAME_M=$(uname -m)
if [[ "$UNAME_M" == "arm64" ]] || [[ "$UNAME_M" == "aarch64" ]]; then
  ARCH="arm64"
else
  ARCH="amd64"
fi
PLATFORM="Darwin-$ARCH"
PACKAGE_NAME="PinchBot-$VERSION-$PLATFORM"
OUT_DIR="$DIST_DIR/$PACKAGE_NAME"
TARBALL="$DIST_DIR/$PACKAGE_NAME.tar.gz"
rm -rf "$OUT_DIR" "$TARBALL"
mkdir -p "$OUT_DIR"
APP_DIR="$OUT_DIR/launcher-chat.app"
APP_CONTENTS_DIR="$APP_DIR/Contents"
APP_MACOS_DIR="$APP_CONTENTS_DIR/MacOS"
APP_RESOURCES_DIR="$APP_CONTENTS_DIR/Resources"
mkdir -p "$APP_MACOS_DIR" "$APP_RESOURCES_DIR"
CODESIGN_IDENTITY="${MAC_CODESIGN_IDENTITY:-}"
CODESIGN_STATUS="Unsigned bundle (internal QA only until you sign and notarize it)."

maybe_codesign() {
  if [[ -z "$CODESIGN_IDENTITY" ]]; then
    return 0
  fi
  if ! command -v codesign >/dev/null 2>&1; then
    echo "codesign not found but MAC_CODESIGN_IDENTITY is set." >&2
    return 1
  fi
  echo ""
  echo "[sign] Signing launcher-chat.app with identity: $CODESIGN_IDENTITY"
  codesign --force --options runtime --timestamp --sign "$CODESIGN_IDENTITY" "$APP_MACOS_DIR/pinchbot"
  codesign --force --options runtime --timestamp --sign "$CODESIGN_IDENTITY" "$APP_MACOS_DIR/pinchbot-launcher"
  codesign --force --options runtime --timestamp --sign "$CODESIGN_IDENTITY" "$APP_MACOS_DIR/platform-server"
  codesign --force --options runtime --timestamp --sign "$CODESIGN_IDENTITY" "$APP_MACOS_DIR/launcher-chat"
  codesign --force --options runtime --timestamp --sign "$CODESIGN_IDENTITY" "$APP_DIR"
  CODESIGN_STATUS="Signed with identity: $CODESIGN_IDENTITY"
}

write_readme() {
  local user_home='$HOME'
  local readme_path="$OUT_DIR/README.txt"
  cat > "$readme_path" << EOF
PinchBot - Usage (macOS)
========================================
Version: $VERSION
Platform: $PLATFORM

FOLDER STRUCTURE
-----------------
  launcher-chat.app/         Main desktop app bundle (double-click in Finder)
    Contents/
      Info.plist
      MacOS/
        launcher-chat        Main desktop binary
        pinchbot-launcher    Config UI backend (settings starts PinchBot-launcher on demand)
        pinchbot             Gateway (port 18790, auto-started by launcher-chat)
        platform-server      App account / official-model backend (auto-started after config/platform.env exists)
  config/
    config.example.json      Example config
    platform.example.env     Example platform env (copy to platform.env to enable local backend)
    runtime-config.example.json  Example official-model runtime config
  README.txt                 This file

USER DATA (created beside the app on first run)
-----------------------------------------------
  .pinchbot/
    config.json             Auto-created default config (workspace defaults to "workspace")
    auth.json               Local provider auth cache
    workspace/              Auto-created workspace with starter files on first gateway start

The settings page does NOT stay resident by default; settings starts PinchBot-launcher on demand.
Use PINCHBOT_HOME / PINCHBOT_CONFIG if you need to override the executable-local data directory.

MAIN PROGRAM
------------
  open ./launcher-chat.app

Or double-click launcher-chat.app in Finder.

PLATFORM BACKEND
----------------
  ./launcher-chat.app/Contents/MacOS/platform-server
  launcher-chat auto-starts this service from the package root after
  config/platform.env exists.
  The desktop chat window starts behind the auth gate, so launcher-chat itself,
  app account login, official-model billing, and recharge all require
  live config/platform.env plus this service.
  The release package ships example-only templates, so create live config first:
    1) copy config/platform.example.env to config/platform.env
    2) edit PLATFORM_* values for your environment
    3) optionally copy runtime-config.example.json to runtime-config.json as a starting point
       (or let the server create an empty runtime file on first bootstrap)
    4) then open launcher-chat.app (or run ./launcher-chat.app/Contents/MacOS/platform-server manually)

SIGNING
-------
  $CODESIGN_STATUS
  External customer distribution still requires Apple notarization after signing.
  Gatekeeper may block unsigned or unnotarized bundles.
EOF
}

echo "============================================="
echo "  PinchBot Release Build (macOS)"
echo "  Version: $VERSION  Output: $OUT_DIR"
echo "============================================="

# 1. PinchBot
echo ""
echo "[1/4] Building PinchBot (pinchbot + pinchbot-launcher) ..."
cd "$PINCHBOT_DIR"
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" "$GO_EXE" build -tags stdjson -ldflags "-s -w" -o "$APP_MACOS_DIR/pinchbot" ./cmd/picoclaw
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" "$GO_EXE" build -tags stdjson -ldflags "-s -w" -o "$APP_MACOS_DIR/pinchbot-launcher" ./cmd/picoclaw-launcher

# 2. Platform backend
echo ""
echo "[2/4] Building Platform backend (platform-server) ..."
if [[ -d "$PLATFORM_DIR" ]]; then
  cd "$PLATFORM_DIR"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" "$GO_EXE" build -ldflags "-s -w" -o "$APP_MACOS_DIR/platform-server" ./cmd/platform-server
fi

# 3. Launcher (Wails) - place the desktop binary inside a macOS .app bundle
echo ""
echo "[3/4] Building Launcher (launcher-chat) ..."
cd "$LAUNCHER_DIR"
"$GO_EXE" build -tags "desktop,production" -ldflags "-s -w -X main.Version=$VERSION" -o "$APP_MACOS_DIR/launcher-chat" .
cat > "$APP_CONTENTS_DIR/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>launcher-chat</string>
  <key>CFBundleIdentifier</key>
  <string>io.pinchbot.launcher</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>PinchBot</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$BUNDLE_SHORT_VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUNDLE_VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

# 4. Config examples and README
echo ""
echo "[4/4] Copying config + workspace example and writing README ..."
CONFIG_EXAMPLE="$PINCHBOT_DIR/config/config.example.json"
if [[ -f "$CONFIG_EXAMPLE" ]]; then
  mkdir -p "$OUT_DIR/config"
  cp "$CONFIG_EXAMPLE" "$OUT_DIR/config/"
fi
if [[ -f "$PLATFORM_DIR/config/platform.example.env" ]]; then
  mkdir -p "$OUT_DIR/config"
  cp "$PLATFORM_DIR/config/platform.example.env" "$OUT_DIR/config/"
fi
if [[ -f "$PLATFORM_DIR/config/runtime-config.example.json" ]]; then
  mkdir -p "$OUT_DIR/config"
  cp "$PLATFORM_DIR/config/runtime-config.example.json" "$OUT_DIR/config/"
fi
chmod +x "$APP_MACOS_DIR/pinchbot" "$APP_MACOS_DIR/pinchbot-launcher" "$APP_MACOS_DIR/launcher-chat" "$APP_MACOS_DIR/platform-server" 2>/dev/null || true
maybe_codesign
write_readme

echo ""
echo "Build done: $OUT_DIR"
ls -la "$OUT_DIR"

if [[ "$MAKE_ZIP" == true ]]; then
  echo ""
  echo "Creating $TARBALL ..."
  (cd "$DIST_DIR" && tar czf "$TARBALL" "$PACKAGE_NAME")
  echo "Created: $TARBALL"
fi

echo ""
if [[ -z "$CODESIGN_IDENTITY" ]]; then
  echo "Package '$PACKAGE_NAME' is ready for internal QA."
  echo "Sign and notarize it before external customer distribution."
else
  echo "Package '$PACKAGE_NAME' is signed."
  echo "Complete Apple notarization before external customer distribution."
fi
echo ""
