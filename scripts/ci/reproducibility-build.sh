#!/bin/sh
# Build the release binary twice with fixed flags and require identical SHA256.
# Uses GOOS/GOARCH when set; otherwise matches the current toolchain (go env).
#
# Env: CGO_ENABLED (default 0), GOOS, GOARCH, MAIN_PACKAGE (default ./cmd/reticulum-go)
set -eu

MAIN_PACKAGE="${MAIN_PACKAGE:-./cmd/reticulum-go}"
export CGO_ENABLED="${CGO_ENABLED:-0}"

if [ -z "${GOOS:-}" ]; then
	GOOS="$(go env GOOS)"
	export GOOS
fi
if [ -z "${GOARCH:-}" ]; then
	GOARCH="$(go env GOARCH)"
	export GOARCH
fi

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/repro-build.XXXXXX")"
cleanup() {
	rm -rf "$WORKDIR"
}
trap cleanup EXIT

DIR_A="$WORKDIR/a"
DIR_B="$WORKDIR/b"
mkdir -p "$DIR_A" "$DIR_B"
OUT_A="$DIR_A/reticulum-go"
OUT_B="$DIR_B/reticulum-go"

echo "reproducibility: go version: $(go version)"
echo "reproducibility: GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=$CGO_ENABLED"
echo "reproducibility: package=$MAIN_PACKAGE"

BUILD_FLAGS="-trimpath -buildvcs=false"
LDFLAGS="-s -w"

echo "reproducibility: build 1 -> $OUT_A"
go build $BUILD_FLAGS -ldflags="$LDFLAGS" -o "$OUT_A" "$MAIN_PACKAGE"

echo "reproducibility: build 2 -> $OUT_B"
go build $BUILD_FLAGS -ldflags="$LDFLAGS" -o "$OUT_B" "$MAIN_PACKAGE"

H1="$(sha256sum "$OUT_A" | awk '{print $1}')"
H2="$(sha256sum "$OUT_B" | awk '{print $1}')"
SZ1="$(wc -c < "$OUT_A" | tr -d ' ')"
SZ2="$(wc -c < "$OUT_B" | tr -d ' ')"

echo "reproducibility: sha256 build1 $H1 ($SZ1 bytes)"
echo "reproducibility: sha256 build2 $H2 ($SZ2 bytes)"

if [ "$SZ1" != "$SZ2" ]; then
	echo "reproducibility: FAILED - output sizes differ ($SZ1 vs $SZ2)" >&2
	exit 1
fi

if [ "$H1" != "$H2" ]; then
	echo "reproducibility: FAILED - SHA256 mismatch; release builds are not reproducible with current flags" >&2
	echo "reproducibility: hint: ensure -trimpath -buildvcs=false and stable -ldflags; check for CGO or build tags" >&2
	if command -v cmp >/dev/null 2>&1; then
		if ! cmp -s "$OUT_A" "$OUT_B"; then
			echo "reproducibility: cmp reports binaries differ" >&2
		fi
	fi
	exit 1
fi

echo "reproducibility: OK - two consecutive builds are byte-identical"
