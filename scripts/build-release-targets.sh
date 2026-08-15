#!/bin/sh
# Cross-compile reticulum-go for every release GOOS/GOARCH pair.
# Usage: build-release-targets.sh [goos]
# Optional filter: linux, windows, darwin, freebsd, openbsd, netbsd,
# dragonfly, solaris, illumos, aix, android, js
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FILTER="${1:-}"
DEST_DIR="${DEST_DIR:-$ROOT/bin}"
export DEST_DIR
export VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

TARGETS="
linux/amd64
linux/386
linux/arm64
linux/arm
linux/riscv64
linux/ppc64le
linux/ppc64
linux/mips
linux/mipsle
linux/mips64
linux/mips64le
linux/s390x
windows/amd64
windows/arm64
windows/386
darwin/amd64
darwin/arm64
freebsd/amd64
freebsd/arm64
freebsd/arm
freebsd/386
freebsd/riscv64
openbsd/amd64
openbsd/arm64
openbsd/arm
openbsd/386
openbsd/ppc64
openbsd/riscv64
netbsd/amd64
netbsd/arm64
netbsd/arm
netbsd/386
dragonfly/amd64
solaris/amd64
illumos/amd64
aix/ppc64
android/arm64
js/wasm
"

failfile="$(mktemp "${TMPDIR:-/tmp}/rgo-build.XXXXXX")"
trap 'rm -f "$failfile"' EXIT INT

nproc="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
if [ "$nproc" -gt 6 ]; then
	nproc=6
fi
running=0

build_one_target() {
	goos="$1"
	goarch="$2"
	if ! GOOS="$goos" GOARCH="$goarch" sh "$ROOT/scripts/ci/build-named-release-binary.sh"; then
		echo "$goos/$goarch" >>"$failfile"
		return 1
	fi
}

for spec in $TARGETS; do
	goos="${spec%/*}"
	goarch="${spec#*/}"
	if [ -n "$FILTER" ] && [ "$goos" != "$FILTER" ]; then
		continue
	fi
	build_one_target "$goos" "$goarch" &
	running=$((running + 1))
	if [ "$running" -ge "$nproc" ]; then
		wait
		running=0
	fi
done
wait

if [ -s "$failfile" ]; then
	echo "build-release-targets: failed:" >&2
	sort -u "$failfile" >&2
	exit 1
fi

if [ -z "$FILTER" ] || [ "$FILTER" = linux ]; then
	sh "$ROOT/scripts/ci/verify-release-platform-assets.sh" "$DEST_DIR"
fi

echo "build-release-targets: ok dest=$DEST_DIR"
