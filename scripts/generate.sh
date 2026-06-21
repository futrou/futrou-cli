#!/bin/bash
set -euo pipefail
echo "Generating API types from OpenAPI spec..."
go run ./cmd/generate
