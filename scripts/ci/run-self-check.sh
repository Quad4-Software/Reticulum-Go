#!/usr/bin/env sh
# Build the native reticulum-go binary and run host self-check.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

LOG_DIR="${SELFCHECK_LOG_DIR:-.cache/selfcheck}"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/run.log"
: >"$LOG"

CGO_ENABLED="${CGO_ENABLED:-0}"
SELFCHECK_ARGS="${SELFCHECK_ARGS:---json --full}"

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*)
	BINARY="$ROOT/bin/reticulum-go.exe"
	;;
*)
	BINARY="$ROOT/bin/reticulum-go"
	;;
esac

selfcheck_fail() {
	echo "self-check: $1" >&2
	if [ -f "$LOG" ]; then
		echo "self-check: last output from $LOG:" >&2
		tail -80 "$LOG" >&2
	fi
	exit 1
}

echo "self-check: building $(go env GOOS)/$(go env GOARCH) -> $BINARY"
if ! go build -ldflags="-s -w" -o "$BINARY" ./cmd/reticulum-go >>"$LOG" 2>&1; then
	selfcheck_fail "build failed"
fi

echo "self-check: running $BINARY self-check $SELFCHECK_ARGS"
# shellcheck disable=SC2086
if ! "$BINARY" self-check --binary "$BINARY" $SELFCHECK_ARGS >>"$LOG" 2>&1; then
	selfcheck_fail "self-check failed"
fi

echo "self-check: ok"
if [ "${SELFCHECK_PRINT_LOG:-}" = "1" ]; then
	cat "$LOG"
fi
