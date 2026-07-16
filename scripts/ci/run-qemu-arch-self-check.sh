#!/usr/bin/env sh
# Cross-build a linux/$GOARCH binary and run self-check under qemu-user (binfmt).
# Usage: run-qemu-arch-self-check.sh <goarch>
# Supported: 386, arm, riscv64, ppc64le, ppc64
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOARCH_TARGET="${1:-${GOARCH:-}}"
if [ -z "$GOARCH_TARGET" ]; then
	echo "qemu-arch-self-check: usage: $0 <386|arm|riscv64|ppc64le|ppc64>" >&2
	exit 2
fi

case "$GOARCH_TARGET" in
386)
	QEMU_NAMES="qemu-i386-static qemu-i386"
	;;
arm)
	QEMU_NAMES="qemu-arm-static qemu-arm"
	export GOARM="${GOARM:-6}"
	;;
riscv64)
	QEMU_NAMES="qemu-riscv64-static qemu-riscv64"
	;;
ppc64le)
	QEMU_NAMES="qemu-ppc64le-static qemu-ppc64le"
	;;
ppc64)
	QEMU_NAMES="qemu-ppc64-static qemu-ppc64"
	;;
*)
	echo "qemu-arch-self-check: unsupported GOARCH $GOARCH_TARGET" >&2
	exit 2
	;;
esac

LOG_DIR="${SELFCHECK_LOG_DIR:-.cache/selfcheck}"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/${GOARCH_TARGET}.log"
: >"$LOG"

BINARY="${QEMU_ARCH_SELFCHECK_BINARY:-$ROOT/bin/linux/${GOARCH_TARGET}/reticulum-go}"
SELFCHECK_ARGS="${SELFCHECK_ARGS:---json --full}"
CGO_ENABLED="${CGO_ENABLED:-0}"
export CGO_ENABLED
export GOOS=linux
export GOARCH="$GOARCH_TARGET"
export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

TAG="qemu-${GOARCH_TARGET}-self-check"

find_qemu() {
	# shellcheck disable=SC2086
	for candidate in $QEMU_NAMES; do
		if command -v "$candidate" >/dev/null 2>&1; then
			echo "$candidate"
			return 0
		fi
	done
	return 1
}

selfcheck_fail() {
	echo "${TAG}: $1" >&2
	if [ -f "$LOG" ]; then
		echo "${TAG}: last output from $LOG:" >&2
		tail -80 "$LOG" >&2
	fi
	exit 1
}

QEMU="$(find_qemu)" || selfcheck_fail "install qemu-user-static (needs one of: $QEMU_NAMES)"

# Sandbox child re-exec uses os.Executable() without -exec. Prefer kernel binfmt.
if [ ! -d /proc/sys/fs/binfmt_misc ]; then
	selfcheck_fail "binfmt_misc unavailable (required for sandbox child re-exec under qemu-user)"
fi

ARCH_LABEL="linux/$GOARCH_TARGET"
if [ "$GOARCH_TARGET" = "arm" ]; then
	ARCH_LABEL="linux/arm GOARM=$GOARM"
fi

echo "${TAG}: qemu=$QEMU go=$(go version) target=$ARCH_LABEL"
echo "${TAG}: building $ARCH_LABEL -> $BINARY"
mkdir -p "$(dirname "$BINARY")"
if ! go build -buildvcs=false -ldflags="-s -w" -o "$BINARY" ./cmd/reticulum-go >>"$LOG" 2>&1; then
	selfcheck_fail "build failed"
fi

echo "${TAG}: package tests (selfcheck, sandbox)"
if ! go test -buildvcs=false -exec "$QEMU" -short -count=1 -timeout 15m ./pkg/selfcheck/ ./pkg/sandbox/ >>"$LOG" 2>&1; then
	selfcheck_fail "package tests failed"
fi

echo "${TAG}: running self-check via binfmt"
# shellcheck disable=SC2086
if ! "$BINARY" self-check --binary "$BINARY" $SELFCHECK_ARGS >>"$LOG" 2>&1; then
	selfcheck_fail "self-check failed"
fi

echo "${TAG}: ok"
if [ "${SELFCHECK_PRINT_LOG:-}" = "1" ]; then
	cat "$LOG"
fi
