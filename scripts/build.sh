#!/bin/bash
source .env

# Fail on error
set -euo pipefail

# The binaries are plain, standard Go builds: no obfuscation, stripping or
# other tricks. The only ldflags stamp the name, version and mode.

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

    # Determine the file extension based on the OS
    if [ "$GOOS" == "windows" ]; then
        EXT=".exe"
    else
        EXT=""
    fi

    OUTPUT_FILE="$DIST_DIR/$DIST_NAME-$GOOS-$GOARCH$EXT"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags "-X 'futrou-cli/src/constants.Name=$NAME' -X 'futrou-cli/src/constants.Version=$VERSION' -X 'futrou-cli/src/constants.Mode=production'" -o $OUTPUT_FILE ./src/
done

echo "-------------------------"

# Copy the installers next to the binaries
cp install.sh install.ps1 "$DIST_DIR/"

# The checksums (SHA256SUMS) are written and signed by scripts/sign.sh (make sign)

echo "✅ Build complete!"

# Generate the schema and docs, and package the npm module
"$(dirname "$0")/build-schema.sh"
"$(dirname "$0")/build-docs.sh"
"$(dirname "$0")/build-npm.sh"
