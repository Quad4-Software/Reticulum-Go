#!/bin/sh
# Build rnphone and rgesp-dial with CGO native audio libs.
set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOOS="${1:-$(go env GOOS)}"
GOARCH="${2:-$(go env GOARCH)}"
OUTDIR="${3:-bin}"

mkdir -p "$OUTDIR"

if [ ! -f third_party/opus/lib/libopus.a ] || [ ! -f third_party/codec2/lib/libcodec2.a ]; then
	sh "$ROOT/scripts/lxst/vendor-sync.sh"
fi

export CGO_ENABLED=1
export GOOS GOARCH

suffix=""
if [ "$GOOS" = "windows" ]; then
	suffix=".exe"
fi

go build -tags lxst_native -o "$OUTDIR/rnphone-${GOOS}-${GOARCH}${suffix}" ./cmd/rnphone
go build -tags lxst_native -o "$OUTDIR/rgesp-dial-${GOOS}-${GOARCH}${suffix}" ./cmd/rgesp-dial
