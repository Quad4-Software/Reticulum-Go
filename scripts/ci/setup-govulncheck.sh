#!/bin/sh
# Install govulncheck from a tagged module version (requires Go on PATH).
# Usage: setup-govulncheck.sh [module_version]
set -eu

export PATH="/usr/local/go/bin:$PATH"
VER="${1:-v1.1.4}"
sudo env PATH="$PATH" GOBIN=/usr/local/bin go install "golang.org/x/vuln/cmd/govulncheck@${VER}"
command -v govulncheck
