#!/usr/bin/env sh
# Cross-build linux/riscv64 and run self-check under qemu-user (binfmt).
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

LOG_DIR="${SELFCHECK_LOG_DIR:-.cache/selfcheck}"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/riscv64.log"
: >"$LOG"

BINARY="${RISCV64_SELFCHECK_BINARY:-$ROOT/bin/linux/riscv64/reticulum-go}"
SELFCHECK_ARGS="${SELFCHECK_ARGS:---json --full}"
CGO_ENABLED="${CGO_ENABLED:-0}"
export CGO_ENABLED
export GOOS=linux
export GOARCH=riscv64
export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

find_qemu() {
	for candidate in qemu-riscv64-static qemu-riscv64; do
		if command -v "$candidate" >/dev/null 2>&1; then
			echo "$candidate"
			return 0
		fi
	done
	return 1
}

selfcheck_fail() {
	echo "riscv64-self-check: $1" >&2
	if [ -f "$LOG" ]; then
		echo "riscv64-self-check: last output from $LOG:" >&2
		tail -80 "$LOG" >&2
	fi
	exit 1
}

QEMU="$(find_qemu)" || selfcheck_fail "install qemu-user-static (needs qemu-riscv64-static or qemu-riscv64)"

# Sandbox child re-exec uses os.Executable() without -exec. Prefer kernel binfmt.
if [ ! -d /proc/sys/fs/binfmt_misc ]; then
	selfcheck_fail "binfmt_misc unavailable (required for sandbox child re-exec under qemu-user)"
fi

echo "riscv64-self-check: qemu=$QEMU go=$(go version)"
echo "riscv64-self-check: building linux/riscv64 -> $BINARY"
mkdir -p "$(dirname "$BINARY")"
if ! go build -buildvcs=false -ldflags="-s -w" -o "$BINARY" ./cmd/reticulum-go >>"$LOG" 2>&1; then
	selfcheck_fail "build failed"
fi

echo "riscv64-self-check: package tests (selfcheck, sandbox)"
if ! go test -buildvcs=false -exec "$QEMU" -short -count=1 -timeout 15m ./pkg/selfcheck/ ./pkg/sandbox/ >>"$LOG" 2>&1; then
	selfcheck_fail "package tests failed"
fi

echo "riscv64-self-check: running self-check via binfmt"
# shellcheck disable=SC2086
if ! "$BINARY" self-check --binary "$BINARY" $SELFCHECK_ARGS >>"$LOG" 2>&1; then
	selfcheck_fail "self-check failed"
fi

echo "riscv64-self-check: ok"
if [ "${SELFCHECK_PRINT_LOG:-}" = "1" ]; then
	cat "$LOG"
fi
