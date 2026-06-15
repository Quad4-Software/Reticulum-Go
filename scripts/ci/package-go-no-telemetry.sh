#!/bin/sh
# Package an installed go-no-telemetry GOROOT for CI artifact upload.
# Usage: package-go-no-telemetry.sh <install_root> <output.tar.gz>
set -eu

INSTALL_ROOT="${1:?install root required}"
OUT="${2:?output archive required}"

if [ ! -x "${INSTALL_ROOT}/bin/go" ]; then
    echo "missing ${INSTALL_ROOT}/bin/go" >&2
    exit 1
fi

NAME="$(basename "$INSTALL_ROOT")"
PARENT="$(dirname "$INSTALL_ROOT")"

tar -C "$PARENT" -czf "$OUT" \
    --exclude='.git' \
    --exclude='test' \
    --exclude='_bootstrap' \
    --exclude='bootstrap-tars' \
    "$NAME"

sha256sum "$OUT" | awk '{print $1}' > "${OUT}.sha256"
