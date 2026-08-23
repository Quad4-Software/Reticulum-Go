#!/bin/sh
# Install staticcheck from a tagged module version (requires Go on PATH).
# Usage: setup-staticcheck.sh [module_version]
set -eu

. "$(dirname "$0")/priv.sh"

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v0.6.1}"
run_priv env PATH="$PATH" GOBIN=/usr/local/bin GOFLAGS= GOPROXY=https://proxy.golang.org,direct \
	go install "honnef.co/go/tools/cmd/staticcheck@${VER}"
command -v staticcheck
