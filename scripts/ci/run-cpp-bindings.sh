#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Build librns and run C++ bindings tests plus examples.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v cmake >/dev/null 2>&1; then
	echo "cmake not found on PATH" >&2
	exit 1
fi

if ! command -v c++ >/dev/null 2>&1 && ! command -v g++ >/dev/null 2>&1; then
	echo "C++ compiler not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-librns
else
	mkdir -p bin
	CGO_ENABLED=1 go build -buildmode=c-shared -o bin/librns.so ./cmd/librns
	cp include/rns.h bin/rns.h
fi

make -C bindings/cpp test
make -C bindings/cpp examples
