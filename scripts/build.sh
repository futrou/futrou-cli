#!/bin/bash
source .env

# Fail on error
set -euo pipefail

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

# Clean up dist directory
rm -rf $DIST_DIR

# Create dist directory if it doesn't exist
mkdir -p $DIST_DIR

# Build for each platform
echo "Building $NAME $VERSION:"
echo "-------------------------"

for PLATFORM in $(echo $PLATFORMS | tr ',' '\n'); do
    echo "Building for $PLATFORM..."
    GOOS=$(echo $PLATFORM | cut -d '/' -f 1)
    GOARCH=$(echo $PLATFORM | cut -d '/' -f 2)

    if [ "$GOOS" == "windows" ]; then
        EXT=".exe"
    else
        EXT=""
    fi

    OUTPUT_FILE="$DIST_DIR/$DIST_NAME-$GOOS-$GOARCH$EXT"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -buildvcs=false \
        -trimpath \
        -tags netgo,osusergo \
        -ldflags "-s -w -extldflags '-static' -X 'futrou-cli/src/constants.Name=$NAME' -X 'futrou-cli/src/constants.Version=$VERSION' -X 'futrou-cli/src/constants.Mode=production'" \
        -o $OUTPUT_FILE ./src/
done

echo "-------------------------"

# Copy install scripts into dist/
cp install.sh install.ps1 "$DIST_DIR/"

echo "✅ Build complete!"