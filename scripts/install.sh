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

echo "✅ Installation complete!"