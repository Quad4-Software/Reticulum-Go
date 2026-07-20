#!/usr/bin/env sh
# Bounded protect flood soak for DoS/OOM gate leak detection.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SOAK_SECONDS="${SOAK_SECONDS:-90}"
export SOAK_SECONDS

TIMEOUT="${SOAK_TIMEOUT:-5m}"

echo "protect-soak: SOAK_SECONDS=${SOAK_SECONDS} timeout=${TIMEOUT}"
go run ./scripts/ci/testsummary -v ./pkg/protect -run 'TestProtectSoak' -count=1 -timeout "$TIMEOUT"
echo "protect-soak: done"
