#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v cargo >/dev/null 2>&1; then
	echo "cargo not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-librrc
else
	sh scripts/ci/build-librrc.sh
fi

make -C bindings/rust test
make -C bindings/rust examples
