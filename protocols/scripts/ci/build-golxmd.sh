#!/bin/sh
# Cross-compile golxmd. Usage: build-golxmd.sh <goos> <goarch> [outdir]
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOOS="${1:?goos required}"
GOARCH="${2:?goarch required}"
OUTDIR="${3:-bin}"
VERSION="${VERSION:-dev}"
GOARM="${GOARM:-}"

if [ "$GOARCH" = arm ] && [ -z "$GOARM" ]; then
	GOARM=6
fi

export GOOS GOARCH CGO_ENABLED=0
if [ -n "$GOARM" ]; then
	export GOARM
fi

mkdir -p "$OUTDIR"

name="golxmd-${GOOS}-${GOARCH}"
if [ "$GOARCH" = arm ]; then
	name="golxmd-${GOOS}-armv${GOARM}"
fi
if [ "$GOOS" = windows ]; then
	name="${name}.exe"
fi

ldflags="-s -w -X github.com/Quad4-Software/reticulum-go-protocols/internal/golxmd.Version=${VERSION}"
go build -trimpath -ldflags "$ldflags" -o "${OUTDIR}/${name}" ./cmd/golxmd
echo "${OUTDIR}/${name}"
