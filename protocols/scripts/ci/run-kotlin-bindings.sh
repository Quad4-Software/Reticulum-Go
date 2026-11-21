#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v kotlinc >/dev/null 2>&1; then
	echo "kotlinc not found on PATH, skipping kotlin bindings" >&2
	exit 0
fi

if ! command -v javac >/dev/null 2>&1; then
	echo "javac not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-liblxmf
else
	sh scripts/ci/build-liblxmf.sh
fi

make -C bindings/kotlin test
make -C bindings/kotlin examples
