#!/usr/bin/env sh
# Bounded transport fault-load soak for exploratory leak detection.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SOAK_SECONDS="${SOAK_SECONDS:-90}"
export SOAK_SECONDS

# Job timeout leaves headroom beyond SOAK_SECONDS for settle and GC checks.
TIMEOUT="${SOAK_TIMEOUT:-5m}"

echo "soak: SOAK_SECONDS=${SOAK_SECONDS} timeout=${TIMEOUT}"
go run ./scripts/ci/testsummary -v ./pkg/transport -run TestTransportSoakFaultLoad -count=1 -timeout "$TIMEOUT"
echo "soak: transport done"
sh "$(dirname "$0")/run-protect-soak.sh"
echo "soak: done"
