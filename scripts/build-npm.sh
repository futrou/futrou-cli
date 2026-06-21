#!/bin/bash
source .env

set -euo pipefail

NPM_DIR="dist-npm"
BIN_DIR="$NPM_DIR/bin"

# Require dist/ to exist (run make build first)
if [ ! -d "$DIST_DIR" ]; then
    echo "❌  dist/ not found — run 'make build' first"
    exit 1
fi

echo "Packaging $NAME $VERSION for npm..."
echo "-------------------------"

# Clean and recreate dist-npm
rm -rf "$NPM_DIR"
mkdir -p "$BIN_DIR"

# Copy all platform binaries into bin/
for f in "$DIST_DIR"/$DIST_NAME-*; do
    name=$(basename "$f")
    cp "$f" "$BIN_DIR/$name"
    # Ensure executable bit survives (npm publish strips it otherwise we set it explicitly)
    chmod +x "$BIN_DIR/$name"
    echo "  + bin/$name"
done

# Copy all template files
cp -r templates/npm/. "$NPM_DIR/"
cp install.sh install.ps1 "$NPM_DIR/"

# Copy license
cp LICENSE "$NPM_DIR/LICENSE"

# Stamp version from .env into package.json
sed -i "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" "$NPM_DIR/package.json"

echo "-------------------------"
echo "✅ npm package ready in $NPM_DIR/"
echo ""
echo "To publish:"
echo "  cd $NPM_DIR && npm publish"
