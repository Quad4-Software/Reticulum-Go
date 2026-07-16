#!/usr/bin/env sh
# Push the Android self-check binary into a running emulator and execute it.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

BINARY="${ANDROID_SELFCHECK_BINARY:-bin/android/arm64/reticulum-go}"
REMOTE="/data/local/tmp/reticulum-go-selfcheck"
OUT_LOCAL="${ANDROID_SELFCHECK_OUT:-.cache/selfcheck/android.json}"

if [ ! -f "$BINARY" ]; then
	echo "android-self-check: missing binary $BINARY" >&2
	exit 1
fi

mkdir -p "$(dirname "$OUT_LOCAL")"

echo "android-self-check: waiting for device"
adb wait-for-device
adb shell 'while [ -z "$(getprop sys.boot_completed)" ]; do sleep 1; done'

echo "android-self-check: pushing $BINARY"
adb push "$BINARY" "$REMOTE"
adb shell chmod 755 "$REMOTE"

echo "android-self-check: running self-check"
# Sandbox child re-exec uses the same binary on device.
adb shell "cd /data/local/tmp && $REMOTE self-check --binary $REMOTE --json --quick" >"$OUT_LOCAL" 2>&1 || {
	echo "android-self-check: failed" >&2
	cat "$OUT_LOCAL" >&2
	exit 1
}

echo "android-self-check: ok"
cat "$OUT_LOCAL"
