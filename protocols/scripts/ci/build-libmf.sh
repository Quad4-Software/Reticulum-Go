#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
set -eu
ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
GOOS="${1:-$(go env GOOS)}"
GOARCH="${2:-$(go env GOARCH)}"
OUTDIR="${3:-bin}"
mkdir -p "$OUTDIR"
case "$GOOS" in linux) EXT=so;; darwin) EXT=dylib;; windows) EXT=dll;; *) EXT=so;; esac
OUT="$OUTDIR/libmf.$EXT"
CGO_ENABLED=1 GOOS="$GOOS" GOARCH="$GOARCH" go build -buildmode=c-shared -o "$OUT" ./cmd/libmf
cp include/mf.h "$OUTDIR/mf.h"
echo "built $OUT"
