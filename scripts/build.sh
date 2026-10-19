#!/bin/bash
source .env

# Fail on error
set -euo pipefail

# Why the binaries are obfuscated (garble) and stripped (-s -w, -trimpath):
# by default a Go binary ships a lot of information about the program itself:
# every package, function and type name, file paths of the build machine, line
# tables, DWARF debug info, build and VCS metadata, panic and print messages.
# Removing it makes the binaries smaller, and it also helps against false
# positives from dummy antiviruses and scanners, which flag unstripped Go
# binaries by matching that metadata and the well-known Go runtime strings.

# Assign default platforms if not set
if [ -z "$PLATFORMS" ]; then
    PLATFORMS="linux/amd64"
fi

# Assign default dist directory if not set
if [ -z "$DIST_DIR" ]; then
    DIST_DIR="dist"
fi

# Assign default dist name if not set
if [ -z "$DIST_NAME" ]; then
    DIST_NAME="futrou"
fi

# Generate the API types
"$(dirname "$0")/build-types.sh"

# garble patches the Go linker, which it cannot do for a toolchain Go downloaded
# into the module cache (GOTOOLCHAIN=auto). Require an installed Go matching go.mod.
export GOTOOLCHAIN=local

# Pinned garble version.
# We use garble (with -tiny) to make the binaries as small as possible and to avoid false-positive flagging by dummy scanners
# NOTE: Static seed so we can reuse the build cache between builds (garble only uses the first 8 bytes)
GARBLE_SEED="futrouCLI01"
GARBLE_VERSION="v0.18.0"
GARBLE_BIN="$(go env GOPATH)/bin/garble"
if [ ! -x "$GARBLE_BIN" ] \
    || ! go version -m "$GARBLE_BIN" | grep -qE "^\s*mod\s+mvdan.cc/garble\s+$GARBLE_VERSION\b" \
    || [ "$(go version -m "$GARBLE_BIN" | head -1 | awk '{print $2}')" != "$(go env GOVERSION)" ]; then
    echo "Installing garble $GARBLE_VERSION..."
    GOBIN="$(dirname "$GARBLE_BIN")" go install "mvdan.cc/garble@$GARBLE_VERSION"
fi

# Clean up dist directory
rm -rf $DIST_DIR

# Create dist directory if it doesn't exist
mkdir -p $DIST_DIR

# Build for each platform
echo "Building $NAME $VERSION:"
echo "Building $NAME with seed: $GARBLE_SEED"
echo "-------------------------"

for PLATFORM in $(echo $PLATFORMS | tr ',' '\n'); do
    echo "Building for $PLATFORM..."
    GOOS=$(echo $PLATFORM | cut -d '/' -f 1)
    GOARCH=$(echo $PLATFORM | cut -d '/' -f 2)

    # Determine the file extension based on the OS
    if [ "$GOOS" == "windows" ]; then
        EXT=".exe"
    else
        EXT=""
    fi

    OUTPUT_FILE="$DIST_DIR/$DIST_NAME-$GOOS-$GOARCH$EXT"
    GOGARBLE=futrou-cli CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH $GARBLE_BIN -seed=$GARBLE_SEED -tiny build -buildvcs=false -trimpath -tags netgo,osusergo -ldflags "-s -w -extldflags '-static' -X 'futrou-cli/src/constants.Name=$NAME' -X 'futrou-cli/src/constants.Version=$VERSION' -X 'futrou-cli/src/constants.Mode=production'" -o $OUTPUT_FILE ./src/
done

echo "-------------------------"

# Copy the installers next to the binaries
cp install.sh install.ps1 "$DIST_DIR/"

# Generate checksums for all binaries
echo "Generating checksums..."
(cd "$DIST_DIR" && sha256sum $DIST_NAME-* > checksums.txt)
echo "  + checksums.txt"

echo "✅ Build complete!"

# Generate the schema and docs, and package the npm module
"$(dirname "$0")/build-schema.sh"
"$(dirname "$0")/build-docs.sh"
"$(dirname "$0")/build-npm.sh"
