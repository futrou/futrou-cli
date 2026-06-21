#!/bin/bash

# Fail on error
set -euo pipefail

# Installs go packages
if command -v go &> /dev/null; then
    echo "Installing go packages..."
    go install ./src/
else
    echo "❌ Go is not installed"
    exit 1
fi

# Install garble if doesn't exist
if ! command -v garble &> /dev/null; then
    echo "Installing garble..."
    go install mvdan.cc/garble@v0.15.0
fi

echo "✅ Installation complete!"