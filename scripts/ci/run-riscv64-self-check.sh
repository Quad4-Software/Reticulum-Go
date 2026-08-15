#!/usr/bin/env sh
# Compatibility wrapper for linux/riscv64 qemu-user self-check.
set -eu
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec sh "$ROOT/scripts/ci/run-qemu-arch-self-check.sh" riscv64
