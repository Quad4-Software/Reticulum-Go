#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Run Dart bindings tests (Control API client and librns FFI) plus examples.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v dart >/dev/null 2>&1; then
	echo "dart not found on PATH" >&2
	exit 1
fi

if [ ! -f bin/librns.so ]; then
	echo "==> building librns.so for FFI tests"
	task build-librns
fi

export RNS_ROOT="$ROOT"
export RNS_LIB_PATH="$ROOT/bin/librns.so"

cd "$ROOT/bindings/dart"
dart pub get
dart analyze --fatal-infos
dart test

cd "$ROOT"
make -C bindings/dart examples
