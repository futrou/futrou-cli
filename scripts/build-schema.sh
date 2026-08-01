#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "Generating futrou.schema.json..."
go run ./src config schema > futrou.schema.json
echo "  + futrou.schema.json"
