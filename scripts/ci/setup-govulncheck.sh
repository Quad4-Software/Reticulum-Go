#!/bin/sh
# Install govulncheck from a tagged module version (requires Go on PATH).
# Usage: setup-govulncheck.sh [module_version]
set -eu

. "$(dirname "$0")/priv.sh"

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v1.7.0}"
run_priv env PATH="$PATH" GOBIN=/usr/local/bin GOFLAGS= GOPROXY=https://proxy.golang.org,direct go install "golang.org/x/vuln/cmd/govulncheck@${VER}"
command -v govulncheck
