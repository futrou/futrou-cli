#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "Generating CLI reference in SKILL.md and README.md..."
go run ./scripts/build-docs
