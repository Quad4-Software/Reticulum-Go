#!/bin/sh
# Run golxmd --version and --self-check under qemu-user for a linux GOARCH.
# Usage: qemu-golxmd-smoke.sh <goarch> <qemu-bin> [outdir]
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOARCH="${1:?goarch required}"
QEMU="${2:?qemu binary required}"
OUTDIR="${3:-bin}"

case "$GOARCH" in
arm)
	bin="${OUTDIR}/golxmd-linux-armv${GOARM:-6}"
	;;
*)
	bin="${OUTDIR}/golxmd-linux-${GOARCH}"
	;;
esac

if [ ! -f "$bin" ]; then
	sh "$ROOT/scripts/ci/build-golxmd.sh" linux "$GOARCH" "$OUTDIR"
fi

command -v "$QEMU" >/dev/null
got="$("$QEMU" "$bin" --version)"
got="$(printf '%s' "$got" | tr -d '\r')"
if [ -z "$got" ]; then
	echo "empty --version from $bin" >&2
	exit 1
fi
echo "qemu ${GOARCH} version: ${got}"

home="$(mktemp -d)"
trap 'rm -rf "$home"' EXIT
export GOLXMD_HOME="$home"
"$QEMU" "$bin" --self-check
echo "qemu ${GOARCH}: self-check ok"
