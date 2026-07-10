#!/usr/bin/env sh
# OS-matrix runtime tests for CI (short mode, core native packages).
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

TIMEOUT="${OS_TEST_TIMEOUT:-20m}"

PKGS="./pkg/backbone/... ./pkg/interfaces/... ./pkg/node/... ./pkg/packet/... ./pkg/transport/... ./tests/crossref/..."

echo "os-matrix: running on $(go env GOOS)/$(go env GOARCH)"
# Route through testsummary so CI logs stay compact (TESTSUMMARY_QUIET=1 in workflows).
go run ./scripts/ci/testsummary -short -timeout "$TIMEOUT" $PKGS
