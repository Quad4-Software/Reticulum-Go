#!/bin/sh
# Install revive from a tagged module version (requires Go on PATH).
# Usage: setup-revive.sh [module_version]
set -eu

. "$(dirname "$0")/priv.sh"

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v1.15.0}"
run_priv env PATH="$PATH" GOBIN=/usr/local/bin go install "github.com/mgechev/revive@${VER}"
command -v revive
