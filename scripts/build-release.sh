#!/usr/bin/env bash
# OpenClaw release build for macOS - produces a folder (or tarball) to ship to customers.
# Usage: ./scripts/build-release.sh [version] [-z]
#   version: optional, default from git describe
#   -z: create .tar.gz after build
# Output: dist/OpenClaw-<version>-Darwin-<arch>/

set -e
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
PICOCLAW_DIR="$REPO_ROOT/PicoClaw"
LAUNCHER_DIR="$REPO_ROOT/Launcher/app-wails"

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

UNAME_M=$(uname -m)
if [[ "$UNAME_M" == "arm64" ]] || [[ "$UNAME_M" == "aarch64" ]]; then
  ARCH="arm64"
else
  ARCH="amd64"
fi
PLATFORM="Darwin-$ARCH"
PACKAGE_NAME="OpenClaw-$VERSION-$PLATFORM"
OUT_DIR="$DIST_DIR/$PACKAGE_NAME"
mkdir -p "$OUT_DIR"

echo "============================================="
echo "  OpenClaw Release Build (macOS)"
echo "  Version: $VERSION  Output: $OUT_DIR"
echo "============================================="

# 1. PicoClaw
echo ""
echo "[1/3] Building PicoClaw (picoclaw + picoclaw-launcher) ..."
cd "$PICOCLAW_DIR"
go generate ./... 2>/dev/null || true
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" go build -tags stdjson -ldflags "-s -w" -o "$OUT_DIR/picoclaw" ./cmd/picoclaw
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" go build -tags stdjson -ldflags "-s -w" -o "$OUT_DIR/picoclaw-launcher" ./cmd/picoclaw-launcher

# 2. Launcher (Wails) - use go build so we get a single binary in OUT_DIR (wails build produces .app bundle)
echo ""
echo "[2/3] Building Launcher (launcher-chat) ..."
cd "$LAUNCHER_DIR"
go build -tags "desktop,production" -ldflags "-s -w" -o "$OUT_DIR/launcher-chat" .

# 3. Config example, workspace example, README
echo ""
echo "[3/3] Copying config + workspace example and writing README ..."
CONFIG_EXAMPLE="$PICOCLAW_DIR/config/config.example.json"
if [[ -f "$CONFIG_EXAMPLE" ]]; then
  mkdir -p "$OUT_DIR/config"
  cp "$CONFIG_EXAMPLE" "$OUT_DIR/config/"
fi
if [[ -d "$PICOCLAW_DIR/workspace" ]]; then
  cp -R "$PICOCLAW_DIR/workspace" "$OUT_DIR/workspace-example"
fi

USER_HOME='$HOME'
README="$OUT_DIR/README.txt"
cat > "$README" << EOF
OpenClaw / PicoClaw - Usage (macOS)
========================================
Version: $VERSION
Platform: $PLATFORM

FOLDER STRUCTURE
-----------------
  launcher-chat              Main program (run this; it starts config UI + gateway)
  picoclaw-launcher          Config UI (port 18800, auto-started)
  picoclaw                   Gateway (port 18790, auto-started)
  config/
    config.example.json      Example config
  workspace-example/         Example workspace (USER.md, AGENTS.md, skills, etc.)
  README.txt                 This file

USER DATA (on this Mac)
-----------------------
  Config:     $USER_HOME/.picoclaw/config.json
  Auth:       $USER_HOME/.picoclaw/auth.json
  Workspace:  $USER_HOME/.picoclaw/workspace/

Copy config/config.example.json to $USER_HOME/.picoclaw/config.json or use
Settings (http://localhost:18800) in the browser. Copy workspace-example to
$USER_HOME/.picoclaw/workspace/ or run: ./picoclaw onboard

MAIN PROGRAM
------------
  ./launcher-chat

Or double-click launcher-chat in Finder (if built as .app, open launcher-chat.app).
EOF

chmod +x "$OUT_DIR/picoclaw" "$OUT_DIR/picoclaw-launcher" "$OUT_DIR/launcher-chat" 2>/dev/null || true

echo ""
echo "Build done: $OUT_DIR"
ls -la "$OUT_DIR"

if [[ "$MAKE_ZIP" == true ]]; then
  TARBALL="$DIST_DIR/$PACKAGE_NAME.tar.gz"
  echo ""
  echo "Creating $TARBALL ..."
  (cd "$DIST_DIR" && tar czf "$TARBALL" "$PACKAGE_NAME")
  echo "Created: $TARBALL"
fi

echo ""
echo "Ship the folder '$PACKAGE_NAME' or the tarball to customers."
echo ""
