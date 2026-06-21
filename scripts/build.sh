#!/bin/bash
set -euo pipefail

$(dirname "$0")/build-bin.sh
$(dirname "$0")/build-npm.sh
