#!/usr/bin/env sh
# Build the native reticulum-go binary and smoke-test CLI + short daemon run.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

LOG_DIR="${SMOKE_LOG_DIR:-.cache/smoke}"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/run.log"
: >"$LOG"

RUN_TIMEOUT="${SMOKE_RUN_TIMEOUT:-12s}"
CGO_ENABLED="${CGO_ENABLED:-0}"

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*)
	BINARY="$ROOT/bin/reticulum-go.exe"
	;;
*)
	BINARY="$ROOT/bin/reticulum-go"
	;;
esac

smoke_fail() {
	echo "binary-smoke: $1" >&2
	if [ -f "$LOG" ]; then
		echo "binary-smoke: last output from $LOG:" >&2
		tail -80 "$LOG" >&2
	fi
	exit 1
}

echo "binary-smoke: building $(go env GOOS)/$(go env GOARCH) -> $BINARY"
if ! go build -ldflags="-s -w" -o "$BINARY" ./cmd/reticulum-go >>"$LOG" 2>&1; then
	smoke_fail "build failed"
fi

echo "binary-smoke: --version"
if ! "$BINARY" --version >>"$LOG" 2>&1; then
	smoke_fail "--version failed"
fi

echo "binary-smoke: --help"
if ! "$BINARY" --help >>"$LOG" 2>&1; then
	smoke_fail "--help failed"
fi

SMOKE_HOME="$(mktemp -d)"
trap 'rm -rf "$SMOKE_HOME"' EXIT INT HUP TERM
export HOME="$SMOKE_HOME"
mkdir -p "$SMOKE_HOME/.reticulum-go/storage"
cat >"$SMOKE_HOME/.reticulum-go/config" <<'EOF'
[reticulum]
enable_sandbox = yes
enable_control_api = no
panic_on_interface_err = no
EOF

run_daemon() {
	"$BINARY" >>"$LOG" 2>&1 &
	pid=$!
	trap 'kill "$pid" 2>/dev/null || true' EXIT INT HUP TERM
	case "$RUN_TIMEOUT" in
	*s)
		secs="${RUN_TIMEOUT%s}"
		;;
	*)
		secs=12
		;;
	esac
	i=0
	while [ "$i" -lt "$secs" ]; do
		if ! kill -0 "$pid" 2>/dev/null; then
			wait "$pid" || true
			return 1
		fi
		sleep 1
		i=$((i + 1))
	done
	kill "$pid" 2>/dev/null || taskkill //PID "$pid" //F >/dev/null 2>&1 || true
	wait "$pid" 2>/dev/null || true
	return 0
}

echo "binary-smoke: daemon run (${RUN_TIMEOUT})"
if ! run_daemon; then
	if grep -q 'doAllThreadsSyscall not supported' "$LOG" 2>/dev/null ||
		grep -q 'not supported with cgo enabled' "$LOG" 2>/dev/null; then
		smoke_fail "daemon panicked on AllThreadsSyscall (cgo/fakecgo vs Landlock)"
	fi
	smoke_fail "daemon exited early"
fi

echo "binary-smoke: ok"
