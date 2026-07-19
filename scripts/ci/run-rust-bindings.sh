#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Build librns and run Rust bindings tests plus examples.

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! command -v cargo >/dev/null 2>&1; then
	echo "cargo not found on PATH" >&2
	exit 1
fi

if command -v task >/dev/null 2>&1; then
	task build-librns
else
	mkdir -p bin
	CGO_ENABLED=1 go build -buildmode=c-shared -o bin/librns.so ./cmd/librns
	cp include/rns.h bin/rns.h
fi

make -C bindings/rust test
make -C bindings/rust examples
