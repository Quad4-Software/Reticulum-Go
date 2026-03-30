#!/bin/sh
# Install revive from a tagged module version (requires Go on PATH).
# Usage: setup-revive.sh [module_version]
set -eu

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v1.15.0}"
sudo env PATH="$PATH" GOBIN=/usr/local/bin go install "github.com/mgechev/revive@${VER}"
command -v revive
