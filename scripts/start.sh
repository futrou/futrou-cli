#!/bin/bash
source .env

set -euo pipefail

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux)  GOOS="linux"  ;;
  darwin) GOOS="darwin" ;;
  mingw*|msys*|cygwin*) GOOS="windows" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64)          GOARCH="amd64" ;;
  aarch64|arm64)   GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

EXT=""
[ "$GOOS" = "windows" ] && EXT=".exe"

BIN="$DIST_DIR/$DIST_NAME-$GOOS-$GOARCH$EXT"

if [ ! -f "$BIN" ]; then
    echo "Binary not found: $BIN — run 'make build' first"
    exit 1
fi

exec "$BIN" "$@"
