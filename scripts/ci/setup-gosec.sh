#!/bin/sh
# Install gosec from a tagged module version (requires Go on PATH).
# Usage: setup-gosec.sh [module_version]
set -eu

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v2.24.5}"
sudo env PATH="$PATH" GOBIN=/usr/local/bin go install "github.com/securego/gosec/v2/cmd/gosec@${VER}"
command -v gosec
