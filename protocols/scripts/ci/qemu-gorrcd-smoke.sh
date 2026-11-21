#!/bin/sh
# Run gorrcd --version under qemu-user for a linux GOARCH.
# Usage: qemu-gorrcd-smoke.sh <goarch> <qemu-bin>
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOARCH="${1:?goarch required}"
QEMU="${2:?qemu binary required}"
OUTDIR="${3:-bin}"

case "$GOARCH" in
arm)
	bin="${OUTDIR}/gorrcd-linux-armv${GOARM:-6}"
	;;
*)
	bin="${OUTDIR}/gorrcd-linux-${GOARCH}"
	;;
esac

if [ ! -f "$bin" ]; then
	sh "$ROOT/scripts/ci/build-gorrcd.sh" linux "$GOARCH" "$OUTDIR"
fi

command -v "$QEMU" >/dev/null
got="$("$QEMU" "$bin" --version)"
got="$(printf '%s' "$got" | tr -d '\r')"
if [ -z "$got" ]; then
	echo "empty --version from $bin" >&2
	exit 1
fi
echo "qemu ${GOARCH}: ${got}"
