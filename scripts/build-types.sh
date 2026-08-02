#!/bin/bash
set -euo pipefail
echo "Generating API types from OpenAPI spec..."
tmp_file=$(mktemp)
trap 'rm -f "$tmp_file"' EXIT
go run ./scripts/build-types.go > "$tmp_file"
mv "$tmp_file" src/api/types.go
trap - EXIT
