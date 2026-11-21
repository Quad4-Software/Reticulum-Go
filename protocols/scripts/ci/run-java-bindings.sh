#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v javac >/dev/null 2>&1; then
	echo "javac not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-librrc
	task build-liblxmf
else
	sh scripts/ci/build-librrc.sh
	sh scripts/ci/build-liblxmf.sh
fi

make -C bindings/java test
make -C bindings/java examples
