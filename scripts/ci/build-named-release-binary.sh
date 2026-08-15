#!/bin/sh
# Build one GOOS/GOARCH release binary into DEST_DIR.
# linux/amd64 always emits v1 (unsuffixed and -v1) plus -v3. Never v3 alone.
# linux/386 is also published as linux-i686.
#
# Env: GOOS, GOARCH, GOARM, VERSION, DEST_DIR (default .), CGO_ENABLED (default 0)
set -eu

DEST_DIR="${DEST_DIR:-.}"
GOOS="${GOOS:?set GOOS}"
GOARCH="${GOARCH:?set GOARCH}"
VERSION="${VERSION:-}"
export CGO_ENABLED="${CGO_ENABLED:-0}"
export GOOS GOARCH
export GOFLAGS="${GOFLAGS:--mod=vendor}"
export GOPROXY="${GOPROXY:-off}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

if [ "$GOARCH" = arm ]; then
	export GOARM="${GOARM:-6}"
fi

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

mkdir -p "$DEST_DIR"

suffix=""
if [ "$GOOS" = windows ]; then
	suffix=".exe"
fi

ldflags="-s -w"
if [ -n "$VERSION" ]; then
	ldflags="$ldflags -X main.defaultVersion=${VERSION}"
fi

build_one() {
	out="$1"
	go build -ldflags="$ldflags" -o "$out" ./cmd/reticulum-go
}

if [ "$GOOS" = js ] && [ "$GOARCH" = wasm ]; then
	go build -ldflags="-s -w" -o "$DEST_DIR/reticulum-go-js-wasm.wasm" ./cmd/reticulum-wasm
	echo "built $DEST_DIR/reticulum-go-js-wasm.wasm"
	exit 0
fi

if [ "$GOOS" = linux ] && [ "$GOARCH" = amd64 ]; then
	export GOAMD64=v1
	build_one "$DEST_DIR/reticulum-go-linux-amd64-v1"
	cp "$DEST_DIR/reticulum-go-linux-amd64-v1" "$DEST_DIR/reticulum-go-linux-amd64"
	export GOAMD64=v3
	build_one "$DEST_DIR/reticulum-go-linux-amd64-v3"
	unset GOAMD64
	if [ ! -f "$DEST_DIR/reticulum-go-linux-amd64-v1" ] || [ ! -f "$DEST_DIR/reticulum-go-linux-amd64-v3" ]; then
		echo "build-named-release-binary: linux-amd64 v1 and v3 must both exist" >&2
		exit 1
	fi
	echo "built linux-amd64 v1 and v3"
	exit 0
fi

if [ "$GOOS" = linux ] && [ "$GOARCH" = 386 ]; then
	build_one "$DEST_DIR/reticulum-go-linux-386"
	cp "$DEST_DIR/reticulum-go-linux-386" "$DEST_DIR/reticulum-go-linux-i686"
	echo "built linux-386 and linux-i686"
	exit 0
fi

out="$DEST_DIR/reticulum-go-${GOOS}-${GOARCH}${suffix}"
build_one "$out"
echo "built $out"
