#!/bin/sh
# Cross-compile gorrcd. Usage: build-gorrcd.sh <goos> <goarch> [outdir]
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOOS="${1:?goos required}"
GOARCH="${2:?goarch required}"
OUTDIR="${3:-bin}"
VERSION="${VERSION:-dev}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
GOARM="${GOARM:-}"

if [ "$GOARCH" = arm ] && [ -z "$GOARM" ]; then
	GOARM=6
fi

export GOOS GOARCH CGO_ENABLED=0
if [ -n "$GOARM" ]; then
	export GOARM
fi

mkdir -p "$OUTDIR"

name="gorrcd-${GOOS}-${GOARCH}"
if [ "$GOARCH" = arm ]; then
	name="gorrcd-${GOOS}-armv${GOARM}"
fi
if [ "$GOOS" = windows ]; then
	name="${name}.exe"
fi

ldflags="-s -w -X github.com/Quad4-Software/reticulum-go-protocols/internal/gorrcd.Version=${VERSION} -X github.com/Quad4-Software/reticulum-go-protocols/internal/gorrcd.BuildDate=${BUILD_DATE}"
go build -trimpath -ldflags "$ldflags" -o "${OUTDIR}/${name}" ./cmd/gorrcd
echo "${OUTDIR}/${name}"
