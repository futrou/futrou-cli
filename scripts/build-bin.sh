#!/bin/bash
source .env

set -euo pipefail

if [ -z "$PLATFORMS" ]; then
    PLATFORMS="linux/amd64"
fi

if [ -z "$DIST_DIR" ]; then
    DIST_DIR="dist"
fi

if [ -z "$DIST_NAME" ]; then
    DIST_NAME="futrou"
fi

rm -rf $DIST_DIR
mkdir -p $DIST_DIR

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

cp install.sh install.ps1 "$DIST_DIR/"
sed -i "s/__FUTROU_VERSION__/$VERSION/g" "$DIST_DIR/install.sh"

# Generate checksums for all binaries
echo "Generating checksums..."
(cd "$DIST_DIR" && sha256sum $DIST_NAME-* > checksums.txt)
echo "  + checksums.txt"

echo "✅ Build complete!"
