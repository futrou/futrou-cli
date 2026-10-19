#!/bin/bash
# Every test file must be named after the source file it tests:
# src/commands/storage_test.go tests src/commands/storage.go. An optional
# category suffix is allowed (storages_logs_test.go); the prefix must match
# an existing source file in the same directory.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

status=0
while IFS= read -r test_file; do
    dir=$(dirname "$test_file")
    base=$(basename "$test_file" _test.go)
    candidate="$base"
    found=0
    while true; do
        if [ -f "$dir/$candidate.go" ]; then
            found=1
            break
        fi
        case "$candidate" in
            *_*) candidate="${candidate%_*}" ;;
            *) break ;;
        esac
    done
    if [ "$found" -eq 0 ]; then
        echo "test file $test_file does not match a source file in $dir" >&2
        status=1
    fi
done < <(find src scripts -name '*_test.go' | sort)

exit $status
