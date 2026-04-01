#!/bin/sh
# Install gosec from a tagged module version (requires Go on PATH).
# Usage: setup-gosec.sh [module_version]
set -eu

. "$(dirname "$0")/priv.sh"

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v2.24.5}"
run_priv env PATH="$PATH" GOBIN=/usr/local/bin go install "github.com/securego/gosec/v2/cmd/gosec@${VER}"
command -v gosec
