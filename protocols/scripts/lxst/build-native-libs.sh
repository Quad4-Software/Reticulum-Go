#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GOOS="${GOOS:-$(go env GOHOSTOS)}"
GOARCH="${GOARCH:-$(go env GOHOSTARCH)}"

case "$GOOS" in
	linux) CMAKE_SYSTEM_NAME=Linux ;;
	windows) CMAKE_SYSTEM_NAME=Windows ;;
	darwin) CMAKE_SYSTEM_NAME=Darwin ;;
	android) CMAKE_SYSTEM_NAME=Android ;;
	*) CMAKE_SYSTEM_NAME="" ;;
esac

case "$GOARCH" in
	amd64) CMAKE_SYSTEM_PROCESSOR=x86_64 ;;
	arm64) CMAKE_SYSTEM_PROCESSOR=aarch64 ;;
	*) CMAKE_SYSTEM_PROCESSOR="$GOARCH" ;;
esac

export CC CMAKE_SYSTEM_NAME CMAKE_SYSTEM_PROCESSOR

rm -f third_party/opus/lib/libopus.a third_party/codec2/lib/libcodec2.a
sh "$ROOT/scripts/vendor-sync.sh"

mkdir -p "third_party/opus/${GOOS}-${GOARCH}/lib" "third_party/codec2/${GOOS}-${GOARCH}/lib"
cp third_party/opus/lib/libopus.a "third_party/opus/${GOOS}-${GOARCH}/lib/"
cp third_party/codec2/lib/libcodec2.a "third_party/codec2/${GOOS}-${GOARCH}/lib/"
